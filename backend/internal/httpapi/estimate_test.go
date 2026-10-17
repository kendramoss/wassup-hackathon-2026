package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/spend"
)

// estimatingPlanner is the fake planner plus a fixed estimate.
type estimatingPlanner struct {
	*fakePlanner
	got planner.Request
}

func (e *estimatingPlanner) Estimate(_ context.Context, r planner.Request) (planner.Estimate, error) {
	e.got = r
	return planner.Estimate{Target: r.Target, Moves: 6, EngineCalls: 7, ChecksFresh: planner.Range{Min: 3, Max: 6},
		CostUSD: planner.Range{Min: 0.1, Max: 0.22}, WallSeconds: planner.Range{Min: 26, Max: 78}}, nil
}

func TestPlanEstimate(t *testing.T) {
	ep := &estimatingPlanner{fakePlanner: &fakePlanner{}}
	meter := spend.New(spend.NewMemory(), 0.01)
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		d.Planner, d.CaseOwnership, d.Spend = ep, true, meter
	})
	alice := map[string]string{"X-Visitor-ID": "alice-visitor-id-0001"}
	bob := map[string]string{"X-Visitor-ID": "bob-visitor-id-000002"}
	_, c := hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), alice)
	id := c["id"].(string)

	meter.Add(context.Background(), 0.02) // over the day's budget: the estimate spends nothing, so it still answers
	code, out := hs.doAs(t, "POST", "/v1/cases/"+id+"/plan/estimate", `{"target":"Thailand"}`, alice)
	if code != http.StatusOK {
		t.Fatalf("estimate = %d %v", code, out)
	}
	e := out["estimate"].(map[string]any)
	if e["moves"] != 6.0 || e["cost_usd"].(map[string]any)["max"] != 0.22 || e["wall_seconds"].(map[string]any)["min"] != 26.0 {
		t.Errorf("estimate = %v", e)
	}
	if ep.got.Target != "Thailand" || ep.got.CurrentOrigin == nil || ep.got.Product.Name == "" {
		t.Errorf("planner request = %+v", ep.got)
	}
	if code, _ := hs.doAs(t, "POST", "/v1/cases/"+id+"/plan/estimate", `{"target":"Thailand"}`, bob); code != http.StatusNotFound {
		t.Errorf("bob estimating alice's case = %d, want 404", code)
	}
	if code, _ := hs.doAs(t, "POST", "/v1/cases/case_nope/plan/estimate", `{"target":"Thailand"}`, alice); code != http.StatusNotFound {
		t.Errorf("unknown case = %d, want 404", code)
	}
	if code, _ := hs.doAs(t, "POST", "/v1/cases/"+id+"/plan/estimate", `{}`, alice); code != http.StatusBadRequest {
		t.Errorf("no target = %d, want 400", code)
	}
}

func TestPlanEstimateNeedsAnEstimator(t *testing.T) {
	hs := newHarness(t, "substantial_transformation") // fakePlanner has no Estimate
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	if code, _ := hs.do(t, "POST", "/v1/cases/"+c["id"].(string)+"/plan/estimate", `{"target":"Thailand"}`); code != http.StatusNotFound && code != http.StatusMethodNotAllowed {
		t.Errorf("estimate without an estimator = %d, want no route", code)
	}
}
