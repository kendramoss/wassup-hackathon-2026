package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/metering"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

// budgetPlanner is the fake planner with a fixed estimate.
type budgetPlanner struct {
	*fakePlanner
	max float64
}

func (p budgetPlanner) Estimate(context.Context, planner.Request) (planner.Estimate, error) {
	return planner.Estimate{CostUSD: planner.Range{Min: p.max / 2, Max: p.max}}, nil
}

func meteringHarness(t *testing.T) (harness, *metering.Meter, string) {
	t.Helper()
	m := metering.New(metering.NewMemory())
	hs, _, _ := teamsHarness(t, func(d *Deps) { d.Metering = m })
	code, ws := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Acme"}`, user("alice"))
	if code != http.StatusCreated {
		t.Fatalf("create workspace = %d %v", code, ws)
	}
	wsID := ws["id"].(string)
	hs.addToWorkspace(t, wsID, "alice", "bob")
	return hs, m, wsID
}

func TestWorkspaceUsageCountsAndBudgetBlocks(t *testing.T) {
	hs, m, wsID := meteringHarness(t)
	usage, budget := "/v1/workspaces/"+wsID+"/usage", "/v1/workspaces/"+wsID+"/budget"

	caseID := hs.newCase(t, user("bob"))
	hs.newCase(t, nil) // anonymous: not the workspace's
	code, out := hs.doAs(t, "GET", usage, "", user("alice"))
	if code != http.StatusOK {
		t.Fatalf("usage = %d %v", code, out)
	}
	u := out["usage"].(map[string]any)
	if tot := u["totals"].(map[string]any); tot["predictions"] != 1.0 || tot["plan_searches"] != 0.0 || u["budget"] != nil || u["blocked"] != false {
		t.Fatalf("usage after one member prediction: %v", u)
	}

	// Only an admin sets the budget; a bad budget is 400.
	if code, out := hs.doAs(t, "PUT", budget, `{"monthly_usd":5,"hard_cap":true}`, user("bob")); code != http.StatusForbidden {
		t.Fatalf("editor sets budget = %d %v", code, out)
	}
	for _, bad := range []string{`{"monthly_usd":0}`, `{"monthly_usd":5,"alert_percent":120}`, `{"monthly_usd":5,"extra":1}`} {
		if code, _ := hs.doAs(t, "PUT", budget, bad, user("alice")); code != http.StatusBadRequest {
			t.Errorf("budget %s = %d, want 400", bad, code)
		}
	}
	code, out = hs.doAs(t, "PUT", budget, `{"monthly_usd":5,"hard_cap":true}`, user("alice"))
	if b, _ := out["budget"].(map[string]any); code != http.StatusOK || b["monthly_usd"] != 5.0 || b["updated_by"] != "alice@example.com" {
		t.Fatalf("set budget = %d %v", code, out)
	}

	// The month's spend reaches the cap: members' model calls are refused.
	m.Charge(metering.WithWorkspace(context.Background(), wsID), 5)
	req := httptest.NewRequest("POST", "/v1/cases/predict", strings.NewReader(heatGunBody(t)))
	req.Header.Set("X-User-Token", "tok-bob")
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusPaymentRequired || !strings.Contains(rec.Body.String(), "workspace_budget_reached") || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("predict over the cap = %d %s", rec.Code, rec.Body.String())
	}
	if code, _ := hs.doAs(t, "POST", "/v1/cases/"+caseID+"/plan", `{"target":"Thailand"}`, user("bob")); code != http.StatusPaymentRequired {
		t.Fatalf("plan over the cap = %d", code)
	}
	hs.newCase(t, nil) // others are not affected
	code, out = hs.doAs(t, "GET", usage, "", user("bob"))
	u = out["usage"].(map[string]any)
	if code != http.StatusOK || u["blocked"] != true || u["alert"] != true || u["remaining_usd"] != 0.0 {
		t.Fatalf("usage at the cap = %d %v", code, u)
	}

	// A soft budget reports and never blocks; removing it unblocks too.
	if code, _ := hs.doAs(t, "PUT", budget, `{"monthly_usd":5}`, user("alice")); code != http.StatusOK {
		t.Fatal("soft budget")
	}
	hs.newCase(t, user("bob"))
	if code, _ := hs.doAs(t, "DELETE", budget, "", user("bob")); code != http.StatusForbidden {
		t.Fatal("editor deletes budget")
	}
	if code, _ := hs.doAs(t, "DELETE", budget, "", user("alice")); code != http.StatusNoContent {
		t.Fatal("delete budget")
	}
	code, out = hs.doAs(t, "GET", usage, "", user("alice"))
	if u = out["usage"].(map[string]any); code != http.StatusOK || u["budget"] != nil || u["totals"].(map[string]any)["predictions"] != 2.0 {
		t.Fatalf("usage after delete = %d %v", code, u)
	}
}

func TestWorkspaceUsageIsMembersOnly(t *testing.T) {
	hs, _, wsID := meteringHarness(t)
	if code, _ := hs.doAs(t, "GET", "/v1/workspaces/"+wsID+"/usage", "", user("carol")); code != http.StatusNotFound {
		t.Errorf("non-member usage = %d, want 404", code)
	}
	if code, _ := hs.doAs(t, "GET", "/v1/workspaces/"+wsID+"/usage", "", nil); code != http.StatusUnauthorized {
		t.Errorf("anonymous usage = %d, want 401", code)
	}
	if code, _ := hs.doAs(t, "GET", "/v1/workspaces/"+wsID+"/usage?month=2026-13", "", user("bob")); code != http.StatusBadRequest {
		t.Errorf("bad month = %d, want 400", code)
	}
	if code, out := hs.doAs(t, "GET", "/v1/workspaces/"+wsID+"/usage?month=2026-01", "", user("bob")); code != http.StatusOK || out["usage"].(map[string]any)["totals"].(map[string]any)["month"] != "2026-01" {
		t.Errorf("past month = %d %v", code, out)
	}
}

func TestPlanSearchRefusedWhenEstimateWouldPassHardCap(t *testing.T) {
	m := metering.New(metering.NewMemory())
	hs, _, _ := teamsHarness(t, func(d *Deps) {
		d.Metering = m
		d.Planner = budgetPlanner{fakePlanner: &fakePlanner{}, max: 3}
	})
	_, ws := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Acme"}`, user("alice"))
	wsID := ws["id"].(string)
	id := hs.newCase(t, user("alice"))
	if code, _ := hs.doAs(t, "PUT", "/v1/workspaces/"+wsID+"/budget", `{"monthly_usd":10,"hard_cap":true}`, user("alice")); code != http.StatusOK {
		t.Fatal("budget")
	}
	m.Charge(metering.WithWorkspace(context.Background(), wsID), 8) // 2 left; the search may cost 3
	code, est := hs.doAs(t, "POST", "/v1/cases/"+id+"/plan/estimate", `{"target":"Thailand"}`, user("alice"))
	b, _ := est["workspace_budget"].(map[string]any)
	if code != http.StatusOK || b["fits"] != false || b["remaining_usd"] != 2.0 || b["hard_cap"] != true {
		t.Fatalf("estimate = %d %v", code, est)
	}
	code, out := hs.doAs(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand"}`, user("alice"))
	if code != http.StatusPaymentRequired || out["details"].(map[string]any)["estimate_usd"] != 3.0 {
		t.Fatalf("plan = %d %v", code, out)
	}
	// With room for the estimate it runs and is counted.
	if code, _ := hs.doAs(t, "PUT", "/v1/workspaces/"+wsID+"/budget", `{"monthly_usd":20,"hard_cap":true}`, user("alice")); code != http.StatusOK {
		t.Fatal("raise budget")
	}
	if code, out := hs.doAs(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand"}`, user("alice")); code != http.StatusOK {
		t.Fatalf("plan with room = %d %v", code, out)
	}
	_, us := hs.doAs(t, "GET", "/v1/workspaces/"+wsID+"/usage", "", user("alice"))
	if us["usage"].(map[string]any)["totals"].(map[string]any)["plan_searches"] != 1.0 {
		t.Fatalf("plan searches not counted: %v", us)
	}
}

func TestMeteringRoutesOffByDefault(t *testing.T) {
	hs, _, _ := teamsHarness(t, nil)
	_, ws := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Acme"}`, user("alice"))
	if code, _ := hs.doAs(t, "GET", "/v1/workspaces/"+ws["id"].(string)+"/usage", "", user("alice")); code != http.StatusNotFound {
		t.Errorf("usage with metering off = %d, want 404", code)
	}
}
