package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/metering"
)

type meteredKeyEngine struct {
	*fakeEngine
	meter *metering.Meter
}

func (e meteredKeyEngine) Predict(ctx context.Context, c domain.Case) (engine.Result, error) {
	e.meter.Charge(ctx, 0.25)
	return e.fakeEngine.Predict(ctx, c)
}

func keyMeteringEnv(t *testing.T) (*roleEnv, *metering.Meter) {
	t.Helper()
	m := metering.New(metering.NewMemory())
	e := newRoleEnv(t, true, func(d *Deps) {
		d.AdminConsole, d.Metering = true, m
		d.Engine = meteredKeyEngine{fakeEngine: &fakeEngine{conf: 0.99}, meter: m}
		d.Planner = budgetPlanner{fakePlanner: &fakePlanner{}, max: 3}
	})
	return e, m
}

func TestWorkspaceKeyMeteringCountsAndEnforcesHardCap(t *testing.T) {
	e, m := keyMeteringEnv(t)
	secret, _ := e.wsKey(e.wsID, "carol", "editor")
	h := map[string]string{"X-API-Key": secret}
	e.hs.newCase(t, h)
	st, err := m.Status(t.Context(), e.wsID, m.Month())
	if err != nil || st.Totals.Predictions != 1 || st.Totals.ModelUSD != 0.25 {
		t.Fatalf("workspace key usage = %+v, %v; want one prediction and its model cost", st, err)
	}
	if _, err := m.SetBudget(t.Context(), e.wsID, "carol@example.com", metering.Budget{MonthlyUSD: 0.25, HardCap: true}); err != nil {
		t.Fatal(err)
	}
	before := e.snapshot()
	code, out := e.hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), h)
	if code != http.StatusPaymentRequired || out["code"] != "workspace_budget_reached" {
		t.Fatalf("workspace key above cap = %d %v, want 402", code, out)
	}
	if e.snapshot() != before {
		t.Fatal("blocked workspace key created a case")
	}
	if st, err := m.Status(t.Context(), e.zed, m.Month()); err != nil || st.Totals.Predictions != 0 || st.Totals.ModelUSD != 0 {
		t.Fatalf("other workspace was charged: %+v %v", st, err)
	}
	if _, err := m.SetBudget(t.Context(), e.wsID, "carol@example.com", metering.Budget{MonthlyUSD: 0.25}); err != nil {
		t.Fatal(err)
	}
	e.hs.newCase(t, h)
	if st, err := m.Status(t.Context(), e.wsID, m.Month()); err != nil || st.Totals.Predictions != 2 || st.Totals.ModelUSD != 0.5 {
		t.Fatalf("soft-budget usage = %+v %v", st, err)
	}
}

func TestWorkspaceKeyMeteringPlanEstimateAndSearch(t *testing.T) {
	e, m := keyMeteringEnv(t)
	id := e.wsCase()
	secret, _ := e.wsKey(e.wsID, "carol", "editor")
	h := map[string]string{"X-API-Key": secret}
	if _, err := m.SetBudget(t.Context(), e.wsID, "carol@example.com", metering.Budget{MonthlyUSD: 2, HardCap: true}); err != nil {
		t.Fatal(err)
	}
	path := "/v1/cases/" + id + "/plan"
	code, out := e.hs.doAs(t, "POST", path+"/estimate", `{"target":"Thailand"}`, h)
	b, _ := out["workspace_budget"].(map[string]any)
	if code != http.StatusOK || b["fits"] != false || b["remaining_usd"] != 1.75 {
		t.Errorf("workspace key estimate = %d %v", code, out)
	}
	code, out = e.hs.doAs(t, "POST", path, `{"target":"Thailand"}`, h)
	if code != http.StatusPaymentRequired || out["code"] != "workspace_budget_reached" {
		t.Fatalf("workspace key plan beyond remaining budget = %d %v", code, out)
	}
	if _, err := m.SetBudget(t.Context(), e.wsID, "carol@example.com", metering.Budget{MonthlyUSD: 10, HardCap: true}); err != nil {
		t.Fatal(err)
	}
	if code, out = e.hs.doAs(t, "POST", path, `{"target":"Thailand"}`, h); code != http.StatusOK {
		t.Fatalf("workspace key plan with room = %d %v", code, out)
	}
	if st, err := m.Status(t.Context(), e.wsID, m.Month()); err != nil || st.Totals.PlanSearches != 1 {
		t.Fatalf("workspace key plan count = %+v %v", st, err)
	}
}

func TestWorkspaceKeyMeteringOverMCP(t *testing.T) {
	e, m := keyMeteringEnv(t)
	secret, _ := e.wsKey(e.wsID, "carol", "editor")
	cs := mcpClient(t, e.hs, map[string]string{"X-API-Key": secret})
	if res, _ := callTool(t, cs, "predict_origin", heatGunArgs(t)); res.IsError {
		t.Fatalf("workspace key prediction: %s", text(res))
	}
	if st, err := m.Status(t.Context(), e.wsID, m.Month()); err != nil || st.Totals.Predictions != 1 || st.Totals.ModelUSD != 0.25 {
		t.Fatalf("MCP workspace key usage = %+v %v", st, err)
	}
	if _, err := m.SetBudget(t.Context(), e.wsID, "carol@example.com", metering.Budget{MonthlyUSD: 0.25, HardCap: true}); err != nil {
		t.Fatal(err)
	}
	if res, _ := callTool(t, cs, "predict_origin", heatGunArgs(t)); !res.IsError || !strings.Contains(text(res), "workspace_budget_reached") {
		t.Fatalf("MCP workspace key above cap = %s", text(res))
	}
}

func TestWorkspaceKeyMeteringPreservesLegacyAndIdentityPrecedence(t *testing.T) {
	e, m := keyMeteringEnv(t)
	editor, _ := e.wsKey(e.wsID, "carol", "editor")
	legacy, _ := e.wsKey(e.wsID, "carol", "")
	if _, err := m.SetBudget(t.Context(), e.wsID, "carol@example.com", metering.Budget{MonthlyUSD: 0.01, HardCap: true}); err != nil {
		t.Fatal(err)
	}
	m.Charge(metering.WithWorkspace(t.Context(), e.wsID), 0.01)
	for name, headers := range map[string]map[string]string{
		"legacy":     {"X-API-Key": legacy},
		"standalone": {"X-API-Key": e.keys["acme"]},
		"operator":   {"X-API-Key": editor, "X-Admin-Token": testAdmin},
		"signed-in":  {"X-API-Key": editor, "X-User-Token": "tok-zed"},
	} {
		t.Run(name, func(t *testing.T) { e.hs.newCase(t, headers) })
	}
	if st, err := m.Status(t.Context(), e.wsID, m.Month()); err != nil || st.Totals.Predictions != 0 || st.Totals.ModelUSD != 0.01 {
		t.Fatalf("legacy/operator/other identity charged key workspace: %+v %v", st, err)
	}
	if st, err := m.Status(t.Context(), e.zed, m.Month()); err != nil || st.Totals.Predictions != 1 || st.Totals.ModelUSD != 0.25 {
		t.Fatalf("signed-in identity did not take precedence: %+v %v", st, err)
	}
}
