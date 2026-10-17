package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/triage"
)

// usmcaEngine is a fake Part 102 predictor; err makes it fail.
type usmcaEngine struct {
	conf  float64
	err   error
	calls *int
	cites []string // precedents shown; nil means N350002
}

func (e usmcaEngine) Predict(context.Context, domain.Case) (engine.Result, error) {
	if e.calls != nil {
		*e.calls++
	}
	if e.err != nil {
		return engine.Result{}, e.err
	}
	cites := e.cites
	if cites == nil {
		cites = []string{"N350002"}
	}
	return engine.Result{Prediction: domain.Prediction{Origin: "Mexico", Confidence: e.conf, Precedents: cites,
		Trace: domain.Trace{ModelID: "gemini-4-argon", PromptVersion: engine.USMCAVersion}}}, nil
}

func TestUSMCARegime(t *testing.T) {
	seal := `{"date":"2026-03-01","facts":"Steel bolt seals stamped and assembled in Mexico from Chinese steel wire (7217) and US plastic housings."}`
	for _, tt := range []struct {
		name    string
		regime  string
		eng     *usmcaEngine
		fast    float64
		origin  string
		route   string
		prefix  string
		engRuns int
	}{
		{name: "no engine: out of scope as before", regime: "usmca_part102", fast: 0, origin: "China", route: "needs_review", prefix: "out of scope"},
		{name: "fast path 0: always review", regime: "usmca_part102", eng: &usmcaEngine{conf: 0.97}, origin: "Mexico", route: "needs_review", prefix: triage.USMCAPrefix, engRuns: 1},
		{name: "above the bar: auto", regime: "usmca_part102", eng: &usmcaEngine{conf: 0.97}, fast: 0.9, origin: "Mexico", route: "auto", prefix: triage.USMCAPrefix, engRuns: 1},
		{name: "below the bar: review", regime: "usmca_part102", eng: &usmcaEngine{conf: 0.97}, fast: 0.99, origin: "Mexico", route: "needs_review", prefix: triage.USMCAPrefix, engRuns: 1},
		{name: "engine error: out of scope", regime: "usmca_part102", eng: &usmcaEngine{err: errors.New("boom")}, fast: 0.9, origin: "China", route: "needs_review", prefix: "out of scope", engRuns: 1},
		{name: "other regimes never reach it", regime: "substantial_transformation", eng: &usmcaEngine{conf: 0.97}, fast: 0.9, origin: "China", prefix: "confidence"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			hs := newHarnessWith(t, tt.regime, func(d *Deps) {
				if tt.eng != nil {
					e := *tt.eng
					e.calls = &calls
					d.USMCA, d.USMCAFastPath = e, tt.fast
				}
			})
			code, out := hs.do(t, "POST", "/v1/cases/predict", seal)
			if code != http.StatusCreated {
				t.Fatalf("predict = %d %v", code, out)
			}
			p, tr := out["prediction"].(map[string]any), out["triage"].(map[string]any)
			if p["origin_country"] != tt.origin || !strings.HasPrefix(tr["reason"].(string), tt.prefix) || (tt.route != "" && tr["route"] != tt.route) {
				t.Errorf("prediction %v triage %v", p["origin_country"], tr)
			}
			if calls != tt.engRuns {
				t.Errorf("USMCA engine ran %d times, want %d", calls, tt.engRuns)
			}
			if tt.prefix == triage.USMCAPrefix && (out["clarify"] != nil || len(hs.clar.facts) != 0) {
				t.Errorf("clarify ran on a USMCA case")
			}
		})
	}
}

// mapPrecedents is a precedent pool holding only the given rulings.
type mapPrecedents map[string]domain.Ruling

func (m mapPrecedents) Get(id string) (domain.Ruling, bool) {
	r, ok := m[id]
	return r, ok
}

// A ruling only in the USMCA pool keeps its date, subject and held origin
// when the API looks precedents up in both pools, as the server does with
// USMCA_ENGINE=on; the main pool wins for a ruling in both.
func TestUSMCAPrecedentsDetail(t *testing.T) {
	main := mapPrecedents{"N350001": {ID: "N350001", Date: time.Date(2025, 3, 25, 0, 0, 0, 0, time.UTC), Subject: "Air fryer", Origin: "Vietnam"}}
	usmca := mapPrecedents{
		"N260108": {ID: "N260108", Date: time.Date(2015, 1, 12, 0, 0, 0, 0, time.UTC), Subject: "Insulating glass panels", Origin: "Mexico"},
		"N350001": {ID: "N350001", Subject: "shadowed by the main pool", Origin: "Canada"},
	}
	for _, tt := range []struct {
		name       string
		precedents PrecedentStore
		date       any // N260108's ref and ruling-view fields; origin nil when not found
		subject    any
		origin     any
	}{
		{name: "main pool only: bare ref", precedents: main, date: "", subject: ""},
		{name: "both pools: filled", precedents: PrecedentStores{main, usmca}, date: "2015-01-12", subject: "Insulating glass panels", origin: "Mexico"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hs := newHarnessWith(t, "usmca_part102", func(d *Deps) {
				d.USMCA = usmcaEngine{conf: 0.97, cites: []string{"N260108", "N350001"}}
				d.Precedents = tt.precedents
			})
			code, out := hs.do(t, "POST", "/v1/cases/predict", `{"date":"2026-03-01","facts":"Insulating glass panels assembled in Mexico from Chinese float glass."}`)
			if code != http.StatusCreated {
				t.Fatalf("predict = %d %v", code, out)
			}
			refs, _ := out["precedents_detail"].([]any)
			if len(refs) != 2 {
				t.Fatalf("precedents_detail = %v", out["precedents_detail"])
			}
			ref, other := refs[0].(map[string]any), refs[1].(map[string]any)
			if ref["id"] != "N260108" || ref["date"] != tt.date || ref["subject"] != tt.subject || ref["origin"] != tt.origin {
				t.Errorf("N260108 ref = %v, want date %v subject %v origin %v", ref, tt.date, tt.subject, tt.origin)
			}
			if other["origin"] != "Vietnam" || other["subject"] != "Air fryer" {
				t.Errorf("N350001 ref = %v, want the main pool's", other)
			}
			code, ruling := hs.do(t, "GET", "/v1/rulings/N260108", "")
			if code != http.StatusOK || ruling["origin"] != tt.origin {
				t.Errorf("ruling view = %d origin %v, want %v", code, ruling["origin"], tt.origin)
			}
		})
	}
}
