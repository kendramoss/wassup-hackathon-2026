package httpapi

import (
	"context"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

// #368 through the real handlers: plan search without money, then a money PUT,
// then what get_case tells the Ask agent. The plan keeps what the search knew.
type onePlanPlanner struct{ priced bool }

func (p onePlanPlanner) Plan(_ context.Context, r planner.Request) (planner.Result, error) {
	return planner.Result{CaseID: r.CaseID, Target: r.Target, Rejected: []domain.Plan{}, Plans: []domain.Plan{{
		Description: "Make PCBA in Thailand", Prediction: domain.Prediction{Origin: "Thailand"}, Shown: true, NetPriced: p.priced,
		Verdict: &domain.Verdict{Confidence: 0.9}}}}, nil
}

func (p onePlanPlanner) PlanWithProgress(ctx context.Context, r planner.Request, _ func(planner.Event)) (planner.Result, error) {
	return p.Plan(ctx, r)
}

func agentSees(t *testing.T, hs harness, id, question string) string {
	t.Helper()
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/ask", `{"question":"`+question+`"}`); code != 200 {
		t.Fatalf("ask = %d %v", code, out)
	}
	return hs.agent.last.CaseSummary
}

func TestGetCaseStaysNotPricedAfterLaterMoneyPUT(t *testing.T) {
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.Planner = onePlanPlanner{priced: false} })
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand"}`); code != 200 {
		t.Fatalf("plan = %d %v", code, out)
	}
	if s := agentSees(t, hs, id, "first?"); !strings.Contains(s, "net not priced") || strings.Contains(s, "$0/yr") {
		t.Fatalf("before money: %s", s)
	}
	if code, out := hs.do(t, "PUT", "/v1/cases/"+id+"/economics", `{"illustrative":true,"annual_import_value_usd":2000000,"duty_rates":{"China":0.417,"Thailand":0.167}}`); code != 200 {
		t.Fatalf("put economics = %d %v", code, out)
	}
	if s := agentSees(t, hs, id, "second?"); !strings.Contains(s, "net not priced") || strings.Contains(s, "$0/yr") {
		t.Errorf("after the money PUT the earlier plan was reported as priced: %s", s)
	}
}

func TestGetCaseReportsAPricedZero(t *testing.T) {
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.Planner = onePlanPlanner{priced: true} })
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand"}`); code != 200 {
		t.Fatalf("plan = %d %v", code, out)
	}
	if s := agentSees(t, hs, id, "zero?"); !strings.Contains(s, "net $0/yr") {
		t.Errorf("a priced zero was hidden: %s", s)
	}
}
