package httpapi

import (
	"context"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/triage"
)

// #300: the fallback-model review guard runs before the family bars, so a
// family bar can never accept what the fallback model answered while
// FALLBACK_FAST_PATH does not allow it. Family bars stay default-off (D63).
func TestFallbackGuardPrecedesFamilyBars(t *testing.T) {
	bars := &triage.FamilyBars{Bars: map[string]float64{"motorised": 0.80}}
	const fb = "gemini-fallback"
	tests := []struct {
		name     string
		bars     *triage.FamilyBars
		model    string // "" = primary engine
		fast     float64
		route    string
		contains string
	}{
		{"bars off, primary", nil, "", 0, "auto", "confidence 0.97 ≥ 0.92"},
		{"bars off, fallback, fast path 0", nil, fb, 0, "needs_review", triage.FallbackReason(fb)},
		{"bars off, fallback, fast path enabled", nil, fb, 0.5, "auto", "answered by the fallback model (" + fb + "); confidence 0.97 ≥ 0.50"},
		{"bars on, primary", bars, "", 0, "auto", "motorised family bar"},
		{"bars on, primary, fast path enabled is irrelevant", bars, "", 0.5, "auto", "motorised family bar"},
		{"bars on, fallback, fast path 0", bars, fb, 0, "needs_review", triage.FallbackReason(fb)},
		{"bars on, fallback, fast path enabled", bars, fb, 0.5, "auto", "answered by the fallback model (" + fb + "); confidence 0.97 ≥ 0.50"},
		{"bars on, fallback, fast path above confidence", bars, fb, 0.99, "needs_review", triage.FallbackReason(fb)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d Deps
			newHarnessWith(t, "substantial_transformation", func(dd *Deps) {
				dd.FamilyBars, dd.FallbackFastPath = tt.bars, tt.fast
				dd.Engine = &fakeEngine{conf: 0.97, modelID: tt.model}
				d = *dd
			})
			cl, err := NewCasePredictor(d).Classify(context.Background(), domain.Case{ID: "x", Facts: "heat gun"})
			if err != nil {
				t.Fatal(err)
			}
			if string(cl.Triage.Route) != tt.route || !strings.Contains(cl.Triage.Reason, tt.contains) {
				t.Errorf("triage = %+v, want %s containing %q", cl.Triage, tt.route, tt.contains)
			}
		})
	}
}

// Controls: with family bars on, the specialist and scope-unavailable
// branches stay ahead of the bars and are untouched by the fallback guard.
func TestFallbackGuardLeavesEarlierBranches(t *testing.T) {
	bars := &triage.FamilyBars{Bars: map[string]float64{"motorised": 0.80}}
	// Healthy specialist answered by a non-primary model: textile keeps its
	// own fallback check, usmca/pharma keep their own policy.
	for _, rc := range regimeCases {
		t.Run("specialist "+rc.name, func(t *testing.T) {
			spec := &specialistFake{version: rc.version, origin: rc.origin}
			var d Deps
			newHarnessWith(t, rc.regime, func(dd *Deps) {
				dd.FamilyBars = bars
				rc.wire(dd, spec)
				d = *dd
			})
			cl, err := NewCasePredictor(d).Classify(context.Background(), domain.Case{ID: "x", Facts: "gaiters"})
			if err != nil || cl.Triage.Route != domain.RouteAuto || !strings.HasPrefix(cl.Triage.Reason, rc.prefix) {
				t.Errorf("triage = %+v err %v", cl.Triage, err)
			}
		})
	}
	t.Run("router failure D98", func(t *testing.T) {
		var d Deps
		newHarnessWith(t, "substantial_transformation", func(dd *Deps) {
			dd.FamilyBars, dd.Router = bars, failingRouter{}
			dd.Engine = &fakeEngine{conf: 0.97, modelID: "gemini-fallback"}
			dd.FallbackFastPath = 0.5
			d = *dd
		})
		cl, err := NewCasePredictor(d).Classify(context.Background(), domain.Case{ID: "x", Facts: "heat gun"})
		if err != nil || cl.Triage.Route != domain.RouteNeedsReview || cl.Triage.Reason != ScopeUnavailableReason {
			t.Errorf("triage = %+v err %v", cl.Triage, err)
		}
	})
	t.Run("label missing", func(t *testing.T) {
		var d Deps
		newHarnessWith(t, "substantial_transformation", func(dd *Deps) {
			dd.FamilyBars = bars
			dd.Engine = &fakeEngine{conf: 0.97, modelID: "gemini-fallback"}
			dd.FallbackFastPath = 0.5
			d = *dd
		})
		cp := NewCasePredictor(d)
		got, err := cp.a.dispatch(context.Background(), domain.Case{ID: "x", Facts: "gaiters"}, nil, &pinned{})
		if err != nil {
			t.Fatal(err)
		}
		tr, in := cp.a.decide(got)
		if tr.Route != domain.RouteNeedsReview || tr.Reason != LabelMissingReason || in {
			t.Errorf("decide = %+v inScope %v", tr, in)
		}
	})
	t.Run("out of scope keeps scope reason", func(t *testing.T) {
		var d Deps
		newHarnessWith(t, router.RegimeUSMCA, func(dd *Deps) {
			dd.FamilyBars = bars
			dd.Engine = &fakeEngine{conf: 0.97, modelID: "gemini-fallback"}
			d = *dd
		})
		cl, err := NewCasePredictor(d).Classify(context.Background(), domain.Case{ID: "x", Facts: "gaiters"})
		if err != nil || !strings.HasPrefix(cl.Triage.Reason, "out of scope") {
			t.Errorf("triage = %+v err %v", cl.Triage, err)
		}
	})
}
