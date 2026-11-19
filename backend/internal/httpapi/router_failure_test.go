package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/triage"
)

// failingRouter is a configured scope router whose call always errors.
type failingRouter struct{}

func (failingRouter) Classify(context.Context, string) (router.Label, error) {
	return router.Label{}, errors.New("router unavailable")
}

const routerFailureBody = `{"date":"2026-03-01","facts":"Cotton shirt cut and sewn in Vietnam from fabric woven in China."}`

// A configured router that errors establishes no scope: the provisional
// prediction stays, the case goes to review with a scope-unavailable reason
// (never "out of scope", never "in scope"), and nothing auto-accepts (D7).
func TestRouterFailureForcesReview(t *testing.T) {
	bars := &triage.FamilyBars{Bars: map[string]float64{"motorised": 0.50}}
	for _, tt := range []struct {
		name   string
		adjust func(d *Deps)
	}{
		{"global bar", func(d *Deps) {}},
		{"family bars on", func(d *Deps) { d.FamilyBars = bars }},
		{"specialist engines configured but not dispatched", func(d *Deps) {
			d.USMCA, d.USMCAFastPath = usmcaEngine{conf: 0.99}, 0.5
		}},
		{"fallback fast path open", func(d *Deps) { d.FallbackFastPath = 0.5 }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
				d.Router = failingRouter{}
				tt.adjust(d)
			})
			code, out := hs.do(t, "POST", "/v1/cases/predict", routerFailureBody)
			if code != http.StatusCreated {
				t.Fatalf("predict = %d %v", code, out)
			}
			tr := out["triage"].(map[string]any)
			reason := tr["reason"].(string)
			if tr["route"] != "needs_review" {
				t.Errorf("route = %v, want needs_review (reason %q)", tr["route"], reason)
			}
			if reason != ScopeUnavailableReason || !strings.HasPrefix(reason, "scope unavailable") {
				t.Errorf("reason = %q, want a scope-unavailable reason", reason)
			}
			if out["router"] != nil {
				t.Errorf("router = %v, want null", out["router"])
			}
			if p, _ := out["prediction"].(map[string]any); p == nil || p["origin_country"] != "China" || p["confidence"].(float64) != 0.97 {
				t.Errorf("provisional prediction not retained: %v", out["prediction"])
			}
			if out["clarify"] != nil || len(hs.clar.facts) != 0 {
				t.Errorf("clarify ran for a scope-unavailable case")
			}
		})
	}
}

// The same holds for an answered fallback: no auto-accept while scope is unknown.
func TestRouterFailureFallbackNotAutoAccepted(t *testing.T) {
	eng := &fakeEngine{conf: 0.99, modelID: "projects/p/locations/us/endpoints/9"}
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		d.Router, d.Engine, d.FallbackFastPath = failingRouter{}, eng, 0.5
	})
	_, out := hs.do(t, "POST", "/v1/cases/predict", routerFailureBody)
	tr := out["triage"].(map[string]any)
	if tr["route"] != "needs_review" || !strings.HasPrefix(tr["reason"].(string), "scope unavailable") {
		t.Errorf("triage = %v", tr)
	}
}

// A specialist-eligible case with a failed router must not reach the
// specialist engine (no label) and must not auto-accept.
func TestRouterFailureSkipsSpecialist(t *testing.T) {
	calls := 0
	hs := newHarnessWith(t, "usmca_part102", func(d *Deps) {
		d.Router = failingRouter{}
		d.USMCA, d.USMCAFastPath = usmcaEngine{conf: 0.99, calls: &calls}, 0.5
	})
	_, out := hs.do(t, "POST", "/v1/cases/predict", routerFailureBody)
	if calls != 0 {
		t.Errorf("specialist ran %d times without a router label", calls)
	}
	if tr := out["triage"].(map[string]any); tr["route"] != "needs_review" {
		t.Errorf("triage = %v", tr)
	}
}

// Controls: working router (general, specialist) and no router configured.
func TestRouterFailureControls(t *testing.T) {
	t.Run("router ok, general: auto and in scope", func(t *testing.T) {
		hs := newHarnessWith(t, "substantial_transformation", nil)
		_, out := hs.do(t, "POST", "/v1/cases/predict", routerFailureBody)
		tr := out["triage"].(map[string]any)
		if tr["route"] != "auto" || !strings.Contains(tr["reason"].(string), "and in scope") || out["router"] == nil {
			t.Errorf("triage = %v router = %v", tr, out["router"])
		}
	})
	t.Run("router ok, specialist: specialist path", func(t *testing.T) {
		calls := 0
		hs := newHarnessWith(t, "usmca_part102", func(d *Deps) {
			d.USMCA, d.USMCAFastPath = usmcaEngine{conf: 0.97, calls: &calls}, 0.9
		})
		_, out := hs.do(t, "POST", "/v1/cases/predict", routerFailureBody)
		tr := out["triage"].(map[string]any)
		if calls != 1 || tr["route"] != "auto" || !strings.HasPrefix(tr["reason"].(string), triage.USMCAPrefix) {
			t.Errorf("calls = %d triage = %v", calls, tr)
		}
	})
	t.Run("no router configured: unchanged", func(t *testing.T) {
		hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.Router = nil })
		_, out := hs.do(t, "POST", "/v1/cases/predict", routerFailureBody)
		tr := out["triage"].(map[string]any)
		if tr["route"] != "auto" || !strings.Contains(tr["reason"].(string), "and in scope") || out["router"] != nil {
			t.Errorf("triage = %v router = %v", tr, out["router"])
		}
	})
}
