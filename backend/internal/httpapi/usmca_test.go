package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/triage"
)

// usmcaEngine is a fake Part 102 predictor; err makes it fail.
type usmcaEngine struct {
	conf  float64
	err   error
	calls *int
}

func (e usmcaEngine) Predict(context.Context, domain.Case) (engine.Result, error) {
	if e.calls != nil {
		*e.calls++
	}
	if e.err != nil {
		return engine.Result{}, e.err
	}
	return engine.Result{Prediction: domain.Prediction{Origin: "Mexico", Confidence: e.conf, Precedents: []string{"N350002"},
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
