package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// driftRouter labels the first call with regime and every later call as
// general, so a watch that routes again instead of using the stored label
// shows up as a regime change.
type driftRouter struct {
	regime string
	calls  atomic.Int32
}

func (r *driftRouter) Classify(context.Context, string) (router.Label, error) {
	regime := RegimeGeneral
	if r.calls.Add(1) == 1 {
		regime = r.regime
	}
	return router.Label{Route: "assembly", Subfamily: "motorised", Regime: regime, Reason: "r", ModelID: "gemini-3.8-flash"}, nil
}

// watchRegimeHarness predicts dispatchBody on a regime wired to spec and
// returns the harness, the case id and the underlying store.
func watchRegimeHarness(t *testing.T, rc regimeCase, spec Predictor, adjust func(*Deps)) (harness, string, store.Cases, *driftRouter) {
	t.Helper()
	var cases store.Cases
	dr := &driftRouter{regime: rc.regime}
	hs := newHarnessWith(t, rc.regime, func(d *Deps) {
		d.Router = dr
		rc.wire(d, spec)
		if adjust != nil {
			adjust(d)
		}
		cases = d.Cases
	})
	code, out := hs.do(t, "POST", "/v1/cases/predict", dispatchBody)
	if code != http.StatusCreated {
		t.Fatalf("predict = %d %v", code, out)
	}
	return hs, out["id"].(string), cases, dr
}

func doWatch(t *testing.T, hs harness, id string) (map[string]any, map[string]any) {
	t.Helper()
	code, out := hs.do(t, "POST", "/v1/cases/"+id+"/watch", "")
	if code != http.StatusOK {
		t.Fatalf("watch = %d %v", code, out)
	}
	return out, out["watch"].(map[string]any)
}

func triageOf(w map[string]any) map[string]any {
	tr, _ := w["triage"].(map[string]any)
	return tr
}

// An immediate watch with unchanged facts keeps a healthy specialist case on
// its regime: no re-routing, the specialist answers, nothing reads as changed.
func TestWatchKeepsHealthySpecialistRegime(t *testing.T) {
	for _, rc := range regimeCases {
		t.Run(rc.name, func(t *testing.T) {
			spec := &specialistFake{version: rc.version, origin: rc.origin}
			hs, id, _, dr := watchRegimeHarness(t, rc, spec, nil)
			hs.engine.err = errGeneralDown // a healthy specialist never needs the general engine
			out, w := doWatch(t, hs, id)
			p := w["prediction"].(map[string]any)
			if w["status"] != "unchanged" || p["origin_country"] != rc.origin {
				t.Errorf("status %v origin %v, want unchanged %s", w["status"], p["origin_country"], rc.origin)
			}
			if w["basis"] != "precedent" || w["regime"] != rc.regime || w["original_regime"] != rc.regime || w["scope"] != "confirmed" {
				t.Errorf("provenance = %v", w)
			}
			if tr := triageOf(w); tr["route"] != "auto" || !strings.HasPrefix(tr["reason"].(string), rc.prefix) {
				t.Errorf("triage = %v", tr)
			}
			if n := spec.calls.Load(); n != 2 {
				t.Errorf("specialist called %d times, want 2 (predict + watch)", n)
			}
			if n := dr.calls.Load(); n != 1 {
				t.Errorf("router called %d times, want 1: the watch must use the stored label", n)
			}
			if out["prediction"].(map[string]any)["origin_country"] != rc.origin {
				t.Errorf("watch changed the case's own prediction")
			}
		})
	}
}

// The scheduler run does not list an unchanged specialist case as changed.
func TestWatchRunKeepsHealthySpecialistUnchanged(t *testing.T) {
	rc := regimeCases[0]
	spec := &specialistFake{version: rc.version, origin: rc.origin}
	hs, _, _, _ := watchRegimeHarness(t, rc, spec, nil)
	_, run := hs.doAs(t, "POST", "/v1/watch/run", "", map[string]string{"X-Admin-Token": testAdmin})
	if run["checked"] != 1.0 || len(run["changed"].([]any)) != 0 || len(run["failed"].([]any)) != 0 {
		t.Errorf("watch run = %v", run)
	}
}

// A specialist that fails during the watch is explicit: review, a failed
// specialist and a general method, never a quiet change to general.
func TestWatchSpecialistFailureIsExplicit(t *testing.T) {
	for _, rc := range regimeCases {
		t.Run(rc.name, func(t *testing.T) {
			spec := &specialistFake{version: rc.version, origin: rc.origin}
			hs, id, _, _ := watchRegimeHarness(t, rc, spec, nil)
			spec.err = errGeneralDown
			out, w := doWatch(t, hs, id)
			tr := triageOf(w)
			if tr["route"] != "needs_review" || !strings.HasPrefix(tr["reason"].(string), "specialist unavailable") {
				t.Errorf("triage = %v", tr)
			}
			if w["specialist"] != "failed" || w["basis"] != "specialist_unavailable" || w["regime"] != RegimeGeneral || w["original_regime"] != rc.regime || w["scope"] != "confirmed" {
				t.Errorf("provenance = %v", w)
			}
			if out["prediction"].(map[string]any)["origin_country"] != rc.origin {
				t.Errorf("the case's own prediction changed")
			}
		})
	}
}

// A specialist case that first fell back to a provisional general answer and
// is now answered by its specialist changed regime, not precedent.
func TestWatchRegimeChangeIsNotPrecedentChange(t *testing.T) {
	rc := regimeCases[2]
	spec := &specialistFake{version: rc.version, origin: rc.origin, err: errGeneralDown}
	hs, id, _, _ := watchRegimeHarness(t, rc, spec, nil)
	spec.err = nil
	_, w := doWatch(t, hs, id)
	if w["status"] != "changed" || w["basis"] != "regime" || w["regime"] != rc.regime || w["original_regime"] != RegimeGeneral {
		t.Errorf("watch = %v", w)
	}
}

// A different engine method (prompt version) on the same regime is not a
// precedent change either.
func TestWatchMethodChangeIsNotPrecedentChange(t *testing.T) {
	hs, id, cases, _ := watchRegimeHarness(t, regimeCase{name: "general", regime: "substantial_transformation", prefix: "confidence", wire: func(*Deps, Predictor) {}}, nil, nil)
	if _, err := cases.Update(context.Background(), id, func(c *store.Case) error {
		c.Prediction.Trace.PromptVersion = "engine-v1+rules-v1"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	hs.engine.later = "Thailand"
	_, w := doWatch(t, hs, id)
	if w["status"] != "changed" || w["basis"] != "method" || w["regime"] != RegimeGeneral || w["original_regime"] != RegimeGeneral {
		t.Errorf("watch = %v", w)
	}
}

// A legacy case with no stored label is a review outcome with the label
// missing: never confirmed general, and a specialist is never guessed.
func TestWatchLegacyCaseWithoutLabelNeedsReview(t *testing.T) {
	t.Run("general answer", func(t *testing.T) {
		var cases store.Cases
		hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { cases = d.Cases })
		_, c := hs.do(t, "POST", "/v1/cases/predict", dispatchBody)
		id := c["id"].(string)
		if _, err := cases.Update(context.Background(), id, func(c *store.Case) error { c.Router = nil; return nil }); err != nil {
			t.Fatal(err)
		}
		_, w := doWatch(t, hs, id)
		tr := triageOf(w)
		if w["scope"] != "label_missing" || w["basis"] != "unverified" || tr["route"] != "needs_review" || tr["reason"] != LabelMissingReason {
			t.Errorf("watch = %v", w)
		}
		if w["status"] != "unchanged" {
			t.Errorf("status %v: facts and origin are the same", w["status"])
		}
	})
	t.Run("specialist case", func(t *testing.T) {
		rc := regimeCases[0]
		spec := &specialistFake{version: rc.version, origin: rc.origin}
		hs, id, cases, _ := watchRegimeHarness(t, rc, spec, nil)
		if _, err := cases.Update(context.Background(), id, func(c *store.Case) error { c.Router = nil; return nil }); err != nil {
			t.Fatal(err)
		}
		_, w := doWatch(t, hs, id)
		tr := triageOf(w)
		if w["scope"] != "label_missing" || w["basis"] != "unverified" || tr["route"] != "needs_review" || w["original_regime"] != rc.regime || w["regime"] != RegimeGeneral {
			t.Errorf("watch = %v", w)
		}
		if n := spec.calls.Load(); n != 1 {
			t.Errorf("specialist called %d times: a missing label must not guess a regime", n)
		}
	})
}

// A stored prediction whose trace names no known engine has an unknown
// original regime: review, never read as general or as the label.
func TestWatchUnknownOriginalRegimeNeedsReview(t *testing.T) {
	for _, pv := range []string{"custom-v9", ""} {
		t.Run("version "+pv, func(t *testing.T) {
			rc := regimeCases[1]
			spec := &specialistFake{version: rc.version, origin: rc.origin}
			hs, id, cases, _ := watchRegimeHarness(t, rc, spec, nil)
			if _, err := cases.Update(context.Background(), id, func(c *store.Case) error {
				c.Prediction.Trace.PromptVersion = pv
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			_, w := doWatch(t, hs, id)
			tr := triageOf(w)
			if w["basis"] != "unverified" || w["original_regime"] != nil || tr["route"] != "needs_review" || tr["reason"] != WatchOriginalRegimeUnknownReason {
				t.Errorf("watch = %v", w)
			}
		})
	}
}

// No stored label and no router configured: a general prediction keeps the
// old no-router behaviour, but a specialist or unknown prediction must not be
// answered by the general engine and reported as an ordinary precedent
// result. It is a label-missing review with an unverified basis.
func TestWatchNoLabelNoRouter(t *testing.T) {
	for _, tt := range []struct {
		name, version string
		unverified    bool
	}{
		{"general trace: legacy control", "engine-v1", false},
		{"specialist trace", "textile-v2", true},
		{"unknown trace", "custom-v9", true},
		{"no trace version", "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var cases store.Cases
			hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.Router = nil; cases = d.Cases })
			_, c := hs.do(t, "POST", "/v1/cases/predict", dispatchBody)
			id := c["id"].(string)
			if _, err := cases.Update(context.Background(), id, func(c *store.Case) error {
				c.Prediction.Trace.PromptVersion = tt.version
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			_, w := doWatch(t, hs, id)
			tr := triageOf(w)
			if !tt.unverified {
				if w["basis"] != "precedent" || w["scope"] != "no_router" || tr["route"] != "auto" || w["status"] != "unchanged" {
					t.Errorf("legacy control changed: %v", w)
				}
				return
			}
			if w["basis"] != "unverified" || w["scope"] != "label_missing" || tr["route"] != "needs_review" || tr["reason"] != LabelMissingReason {
				t.Errorf("watch = %v", w)
			}
			if w["regime"] != RegimeGeneral {
				t.Errorf("regime %v: the engine that answered was general", w["regime"])
			}
			if tt.version == "textile-v2" && w["original_regime"] != router.RegimeTextile {
				t.Errorf("original_regime %v, want the regime from the stored trace", w["original_regime"])
			}
			if tt.version != "textile-v2" && w["original_regime"] != nil {
				t.Errorf("original_regime %v, want unknown (absent)", w["original_regime"])
			}
		})
	}
}

// Controls: a general case, and a deployment with no router at all, stay as
// they were: the label-missing review applies only where a router exists.
func TestWatchGeneralControls(t *testing.T) {
	for _, tt := range []struct {
		name   string
		adjust func(*Deps)
		scope  string
	}{
		{"general label", nil, "confirmed"},
		{"no router configured", func(d *Deps) { d.Router = nil }, "no_router"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hs := newHarnessWith(t, "substantial_transformation", tt.adjust)
			_, c := hs.do(t, "POST", "/v1/cases/predict", dispatchBody)
			_, w := doWatch(t, hs, c["id"].(string))
			tr := triageOf(w)
			if w["status"] != "unchanged" || w["basis"] != "precedent" || w["scope"] != tt.scope || w["regime"] != RegimeGeneral || tr["route"] != "auto" {
				t.Errorf("watch = %v", w)
			}
		})
	}
}

// D97 holds with a specialist: a watch overtaken by new answers is refused
// with 409 and not saved, and the scheduler counts it as failed.
func TestWatchSpecialistStaleSaveRefused(t *testing.T) {
	rc := regimeCases[0]
	for _, viaScheduler := range []bool{false, true} {
		name := "single"
		if viaScheduler {
			name = "scheduler"
		}
		t.Run(name, func(t *testing.T) {
			spec := &specialistFake{version: rc.version, origin: rc.origin}
			gate := &watchGate{inner: spec}
			hs, id, _, _ := watchRegimeHarness(t, rc, gate, nil)
			gate.arm()
			done := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				rec := httptest.NewRecorder()
				req := httptest.NewRequest("POST", "/v1/cases/"+id+"/watch", strings.NewReader(`{}`))
				if viaScheduler {
					req = httptest.NewRequest("POST", "/v1/watch/run", nil)
					req.Header.Set("X-Admin-Token", testAdmin)
				}
				hs.h.ServeHTTP(rec, req)
				done <- rec
			}()
			select {
			case <-gate.entered:
			case <-time.After(3 * time.Second):
				t.Fatal("the watch never called the stored regime's specialist")
			}
			if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/answers", answerBody); code != http.StatusOK {
				t.Fatalf("answers %d %v", code, out)
			}
			close(gate.release)
			rec := <-done
			if viaScheduler {
				if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"failed":["`+id+`"]`) || !strings.Contains(rec.Body.String(), `"checked":0`) {
					t.Errorf("scheduler: %d %s, want the superseded case counted as failed", rec.Code, rec.Body.String())
				}
			} else if rec.Code != http.StatusConflict {
				t.Errorf("stale watch: %d %s, want 409", rec.Code, rec.Body.String())
			}
			_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
			if got["watch"] != nil {
				t.Errorf("a stale watch was attached: %v", got["watch"])
			}
		})
	}
}
