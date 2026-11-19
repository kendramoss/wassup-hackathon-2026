package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/triage"
)

// specialistFake is a specialist engine that counts its calls.
type specialistFake struct {
	version string
	origin  string
	err     error
	calls   atomic.Int32
}

func (s *specialistFake) Predict(context.Context, domain.Case) (engine.Result, error) {
	s.calls.Add(1)
	if s.err != nil {
		return engine.Result{}, s.err
	}
	return engine.Result{Prediction: domain.Prediction{Origin: s.origin, Confidence: 0.97, Precedents: []string{"N350001"},
		Trace: domain.Trace{ModelID: "gemini-4-argon", PromptVersion: s.version}}}, nil
}

const dispatchBody = `{"date":"2026-03-01","facts":"Knit polyester neck gaiters, knit in Vietnam from Chinese yarn."}`

// regimeCase wires one specialist regime into Deps.
type regimeCase struct {
	name    string
	regime  string
	version string
	origin  string
	prefix  string
	wire    func(d *Deps, s Predictor)
}

var regimeCases = []regimeCase{
	{"textile", router.RegimeTextile, engine.TextileVersion, "Vietnam", triage.TextilePrefix,
		func(d *Deps, s Predictor) { d.Textile, d.TextileFastPath = s, 0.9 }},
	{"usmca", router.RegimeUSMCA, engine.USMCAVersion, "Mexico", triage.USMCAPrefix,
		func(d *Deps, s Predictor) { d.USMCA, d.USMCAFastPath = s, 0.9 }},
	{"pharma", router.RegimePharma, engine.PharmaVersion, "India", triage.PharmaPrefix,
		func(d *Deps, s Predictor) { d.Pharma, d.PharmaFastPath = s, 0.9 }},
}

var errGeneralDown = errors.New("general engine down")

// A healthy selected specialist answers even when the general engine fails.
func TestHealthySpecialistIgnoresGeneralFailure(t *testing.T) {
	for _, rc := range regimeCases {
		t.Run(rc.name, func(t *testing.T) {
			spec := &specialistFake{version: rc.version, origin: rc.origin}
			hs := newHarnessWith(t, rc.regime, func(d *Deps) {
				d.Engine = &fakeEngine{err: errGeneralDown}
				rc.wire(d, spec)
			})
			code, out := hs.do(t, "POST", "/v1/cases/predict", dispatchBody)
			if code != http.StatusCreated {
				t.Fatalf("predict = %d %v", code, out)
			}
			p, tr := out["prediction"].(map[string]any), out["triage"].(map[string]any)
			if p["origin_country"] != rc.origin || tr["route"] != "auto" || !strings.HasPrefix(tr["reason"].(string), rc.prefix) {
				t.Errorf("prediction %v triage %v", p["origin_country"], tr)
			}
			if n := spec.calls.Load(); n != 1 {
				t.Errorf("specialist called %d times, want 1", n)
			}
			if out["clarify"] != nil {
				t.Errorf("clarify ran on a specialist case")
			}
		})
	}
}

// Controls: a healthy general engine does not change a healthy specialist's
// answer, and the specialist is still called once.
func TestHealthySpecialistWithHealthyGeneral(t *testing.T) {
	for _, rc := range regimeCases {
		t.Run(rc.name, func(t *testing.T) {
			spec := &specialistFake{version: rc.version, origin: rc.origin}
			hs := newHarnessWith(t, rc.regime, func(d *Deps) { rc.wire(d, spec) })
			code, out := hs.do(t, "POST", "/v1/cases/predict", dispatchBody)
			p, tr := out["prediction"].(map[string]any), out["triage"].(map[string]any)
			if code != http.StatusCreated || p["origin_country"] != rc.origin || tr["route"] != "auto" || spec.calls.Load() != 1 {
				t.Errorf("code %d prediction %v triage %v calls %d", code, p["origin_country"], tr, spec.calls.Load())
			}
		})
	}
}

// A general engine error stays an error wherever the general engine is the
// only one that can answer.
func TestGeneralFailureStaysAnErrorOffSpecialistRoute(t *testing.T) {
	for _, tt := range []struct {
		name   string
		regime string
		adjust func(d *Deps)
	}{
		{"general route", "substantial_transformation", nil},
		{"no router", "substantial_transformation", func(d *Deps) { d.Router = nil }},
		{"configured router failed (D98)", "substantial_transformation", func(d *Deps) { d.Router = failingRouter{} }},
		{"unclear regime", "unclear", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hs := newHarnessWith(t, tt.regime, func(d *Deps) {
				d.Engine = &fakeEngine{err: errGeneralDown}
				if tt.adjust != nil {
					tt.adjust(d)
				}
			})
			if code, out := hs.do(t, "POST", "/v1/cases/predict", dispatchBody); code != http.StatusInternalServerError {
				t.Errorf("predict = %d %v, want 500", code, out)
			}
		})
	}
}

// A failed specialist never auto-accepts and never claims its regime: the
// healthy general answer is kept, explicitly provisional, for review.
func TestSpecialistFailureKeepsProvisionalGeneralForReview(t *testing.T) {
	for _, rc := range regimeCases {
		t.Run(rc.name, func(t *testing.T) {
			spec := &specialistFake{version: rc.version, origin: rc.origin, err: errors.New("specialist down")}
			hs := newHarnessWith(t, rc.regime, func(d *Deps) { rc.wire(d, spec) })
			code, out := hs.do(t, "POST", "/v1/cases/predict", dispatchBody)
			if code != http.StatusCreated {
				t.Fatalf("predict = %d %v", code, out)
			}
			p, tr := out["prediction"].(map[string]any), out["triage"].(map[string]any)
			if tr["route"] != "needs_review" || tr["reason"] != SpecialistUnavailableReason(rc.regime) {
				t.Errorf("triage = %v", tr)
			}
			if p["origin_country"] != "China" {
				t.Errorf("provisional general prediction not kept: %v", p["origin_country"])
			}
			if out["clarify"] != nil || len(hs.clar.facts) != 0 {
				t.Errorf("clarify ran for a specialist-unavailable case")
			}
			if spec.calls.Load() != 1 {
				t.Errorf("specialist called %d times, want 1", spec.calls.Load())
			}
			// The stored prediction tells the truth about the engine that made it.
			_, got := hs.do(t, "GET", "/v1/cases/"+out["id"].(string), "")
			sp := got["prediction"].(map[string]any)
			var pred domain.Prediction
			pred.Trace.PromptVersion = sp["trace"].(map[string]any)["prompt_version"].(string)
			if PredictionRegime(pred) != RegimeGeneral {
				t.Errorf("stored prediction regime = %q, want general (prompt %q)", PredictionRegime(pred), pred.Trace.PromptVersion)
			}
			if got["router"].(map[string]any)["regime"] != rc.regime {
				t.Errorf("router label not kept: %v", got["router"])
			}
		})
	}
}

// Both engines failing is an error, not a review.
func TestSpecialistAndGeneralFailureIsAnError(t *testing.T) {
	for _, rc := range regimeCases {
		t.Run(rc.name, func(t *testing.T) {
			spec := &specialistFake{version: rc.version, err: errors.New("specialist down")}
			hs := newHarnessWith(t, rc.regime, func(d *Deps) {
				d.Engine = &fakeEngine{err: errGeneralDown}
				rc.wire(d, spec)
			})
			if code, out := hs.do(t, "POST", "/v1/cases/predict", dispatchBody); code != http.StatusInternalServerError {
				t.Errorf("predict = %d %v, want 500", code, out)
			}
		})
	}
}

// A confirmed special regime without an enabled engine goes to review with
// the out-of-scope reason; it never auto-accepts, even above the bar.
func TestUnconfiguredSpecialistReviewsPerRegime(t *testing.T) {
	for _, rc := range regimeCases {
		t.Run(rc.name, func(t *testing.T) {
			hs := newHarnessWith(t, rc.regime, nil)
			code, out := hs.do(t, "POST", "/v1/cases/predict", dispatchBody)
			tr := out["triage"].(map[string]any)
			if code != http.StatusCreated || tr["route"] != "needs_review" || !strings.HasPrefix(tr["reason"].(string), "out of scope") {
				t.Errorf("code %d triage %v", code, tr)
			}
			if out["prediction"].(map[string]any)["confidence"].(float64) < 0.92 {
				t.Errorf("control is not above the bar")
			}
		})
	}
}

// The exported adapter runs the same dispatch and triage (for #319).
func TestCasePredictorAdapter(t *testing.T) {
	for _, rc := range regimeCases {
		t.Run(rc.name, func(t *testing.T) {
			spec := &specialistFake{version: rc.version, origin: rc.origin}
			var d Deps
			hs := newHarnessWith(t, rc.regime, func(dd *Deps) {
				dd.Engine = &fakeEngine{err: errGeneralDown}
				rc.wire(dd, spec)
				d = *dd
			})
			_ = hs
			cp := NewCasePredictor(d)
			c := domain.Case{ID: "x", Facts: "gaiters"}
			var _ interface {
				Predict(context.Context, domain.Case) (engine.Result, error)
			} = cp
			cl, err := cp.Classify(context.Background(), c)
			if err != nil {
				t.Fatal(err)
			}
			if cl.Result.Prediction.Origin != rc.origin || cl.Regime() != rc.regime || cl.Scope != ScopeConfirmed ||
				cl.Triage.Route != domain.RouteAuto || cl.Specialist != SpecialistOK {
				t.Errorf("classification = %+v", cl)
			}
			res, err := cp.Predict(context.Background(), c)
			if err != nil || res.Prediction.Origin != rc.origin {
				t.Errorf("Predict = %v %v", res.Prediction.Origin, err)
			}
		})
	}
}

// Router states are distinct, and an empty label is never "confirmed general".
func TestDispatchScopeStates(t *testing.T) {
	c := domain.Case{ID: "x", Facts: "gaiters"}
	build := func(adjust func(d *Deps)) *api {
		var d Deps
		newHarnessWith(t, "substantial_transformation", func(dd *Deps) { adjust(dd); d = *dd })
		return NewCasePredictor(d).a
	}
	for _, tt := range []struct {
		name  string
		a     *api
		pin   *pinned
		want  ScopeState
		label bool
	}{
		{"no router", build(func(d *Deps) { d.Router = nil }), nil, ScopeNoRouter, false},
		{"configured failed", build(func(d *Deps) { d.Router = failingRouter{} }), nil, ScopeConfiguredFailed, false},
		{"confirmed", build(func(d *Deps) {}), nil, ScopeConfirmed, true},
		{"pinned label", build(func(d *Deps) {}), &pinned{label: &router.Label{Regime: "substantial_transformation"}}, ScopeConfirmed, true},
		{"pinned label missing", build(func(d *Deps) {}), &pinned{}, ScopeLabelMissing, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d, err := tt.a.dispatch(context.Background(), c, nil, tt.pin)
			if err != nil || d.Scope != tt.want || (d.Label != nil) != tt.label {
				t.Errorf("scope %q label %v err %v, want %q", d.Scope, d.Label, err, tt.want)
			}
		})
	}
	// A pinned specialist label dispatches to the specialist without calling the router.
	spec := &specialistFake{version: engine.TextileVersion, origin: "Vietnam"}
	a := build(func(d *Deps) { d.Router = failingRouter{}; d.Textile = spec })
	d, err := a.dispatch(context.Background(), c, nil, &pinned{label: &router.Label{Regime: router.RegimeTextile}})
	if err != nil || d.Regime() != router.RegimeTextile || d.Scope != ScopeConfirmed {
		t.Errorf("pinned textile: %+v %v", d, err)
	}
}

func TestPredictionRegime(t *testing.T) {
	for v, want := range map[string]string{
		"": "", engine.PromptVersion: RegimeGeneral, engine.PromptVersion + "+" + engine.RulesVersion: RegimeGeneral,
		engine.PromptVersion + "+" + engine.RulesVersion + "+" + engine.ReasoningVersion: RegimeGeneral,
		engine.TextileVersion: router.RegimeTextile, engine.TextileVersion2: router.RegimeTextile,
		engine.TextileVersion + "+" + engine.RulesVersion:  router.RegimeTextile,
		engine.TextileVersion2 + "+" + engine.RulesVersion: router.RegimeTextile,
		engine.USMCAVersion: router.RegimeUSMCA, engine.USMCAVersion + "+" + engine.RulesVersion: router.RegimeUSMCA,
		engine.PharmaVersion: router.RegimePharma, engine.PharmaVersion + "+" + engine.RulesVersion: router.RegimePharma,
		// Not in the known namespace: never guessed as a regime or as general.
		"textile-v3": "", "textile-v10": "", "textile-v1x": "", "usmca-v2": "", "pharma-v2": "", "custom": "",
		"engine-v10": "", "engine-v1x": "",
	} {
		var p domain.Prediction
		p.Trace.PromptVersion = v
		if got := PredictionRegime(p); got != want {
			t.Errorf("PredictionRegime(%q) = %q, want %q", v, got, want)
		}
	}
}

// A label-missing (legacy) pinned dispatch never auto-accepts, whatever the
// confidence, and never claims a regime or runs clarify.
func TestLabelMissingNeverAutoAccepts(t *testing.T) {
	var d Deps
	newHarnessWith(t, "substantial_transformation", func(dd *Deps) { d = *dd })
	cp := NewCasePredictor(d)
	got, err := cp.a.dispatch(context.Background(), domain.Case{ID: "x", Facts: "gaiters"}, nil, &pinned{})
	if err != nil || got.Result.Prediction.Confidence < d.FastPathConf {
		t.Fatalf("control not above the bar: %v %+v", err, got.Result.Prediction)
	}
	tr, in := cp.a.decide(got)
	if tr.Route != domain.RouteNeedsReview || tr.Reason != LabelMissingReason || in {
		t.Errorf("decide = %+v inScope %v", tr, in)
	}
}

// Streaming on a specialist route with a failed general engine still ends in
// a finished case, and every stage that started is closed.
func TestPredictStreamSpecialistWithGeneralFailure(t *testing.T) {
	spec := &specialistFake{version: engine.TextileVersion, origin: "Vietnam"}
	hs := newHarnessWith(t, router.RegimeTextile, func(d *Deps) {
		d.StreamPredict = true
		d.Engine = &fakeEngine{err: errGeneralDown}
		d.Textile, d.TextileFastPath = spec, 0.9
	})
	evs := readEvents(t, streamPredict(t, hs, dispatchBody).Body.String())
	if len(evs) == 0 || evs[len(evs)-1].name != "done" {
		t.Fatalf("last event = %+v", evs)
	}
	got := stageList(evs)
	for _, s := range got {
		if name, ok := strings.CutSuffix(s, " started"); ok && indexOf(got, name+" done") < 0 {
			t.Errorf("stage %q never closed: %q", name, got)
		}
	}
}
