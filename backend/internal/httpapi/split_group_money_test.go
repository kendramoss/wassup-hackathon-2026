package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/verifier"
)

// Regression for #716: a group performed at two plants has no single baseline
// plant, so the legacy plan money must not depend on the operations order.

type splitGroupVerifier struct{}

func (splitGroupVerifier) Verify(_ context.Context, t verifier.Task) (verifier.Result, error) {
	return verifier.Result{OriginA: "China", OriginB: "Vietnam", ChangesOrigin: true,
		Verdict: domain.Verdict{PrecedentSupported: true, SupportingRuling: "N325810", Confidence: 0.97}, Proof: verifier.ProofFor(t, "N325810")}, nil
}

func splitGroupBody(t *testing.T, split, flip bool) string {
	var m map[string]any
	_ = json.Unmarshal([]byte(heatGunBody(t)), &m)
	p := m["product"].(map[string]any)
	p["plants"] = append(p["plants"].([]any), map[string]any{"id": "vn", "name": "Hanoi plant", "country": "Vietnam", "capabilities": []string{"pcba"}})
	a := map[string]any{"name": "SMT line", "plant": "cn", "group": "pcba"}
	b := map[string]any{"name": "Reflow", "plant": "th", "group": "pcba"}
	if !split {
		b["plant"] = "cn"
	}
	ops := []any{a, b}
	if flip {
		ops = []any{b, a}
	}
	p["operations"] = append(ops, p["operations"].([]any)...)
	out, _ := json.Marshal(m)
	return string(out)
}

// splitGroupPlan runs predict then plan over the real handlers with the fake
// engine and verifier, and returns the shown pcba plan row.
func splitGroupPlan(t *testing.T, split, flip bool, groupCosts string) map[string]any {
	t.Helper()
	hs := bomHarness(t, func(d *Deps) {
		eng := d.Engine.(*fakeEngine)
		eng.byProduct = func(p domain.Product) string {
			for _, c := range p.Components {
				if strings.Contains(c.Name, "PCBA") && c.MadeIn == "Vietnam" {
					return "Vietnam"
				}
			}
			return ""
		}
		eng.byFacts = func(f string) string {
			if strings.Contains(f, "In Vietnam, the printed circuit board assembly") {
				return "Vietnam"
			}
			return ""
		}
		d.Planner = planner.New(d.Templates, eng, splitGroupVerifier{}, planner.Options{PlanConfidence: 0.9, Concurrency: 1, MaxMoves: planner.DefaultMaxMoves})
	})
	c, cs := hs.do(t, "POST", "/v1/cases/predict", splitGroupBody(t, split, flip))
	if c != 201 {
		t.Fatalf("predict %d %v", c, cs)
	}
	econ := `{"illustrative":true,"annual_import_value_usd":1000000,"duty_rates":{"China":0.25,"Vietnam":0.05,"Thailand":0.2}` + groupCosts + `}`
	code, out := hs.do(t, "POST", "/v1/cases/"+cs["id"].(string)+"/plan", `{"target":"Vietnam","economics":`+econ+`}`)
	if code != 200 {
		t.Fatalf("plan %d %v", code, out)
	}
	pl, _ := out["plan"].(map[string]any)
	rows, _ := pl["plans"].([]any)
	if len(rows) != 1 {
		t.Fatalf("want one shown plan, got %d", len(rows))
	}
	return rows[0].(map[string]any)
}

func TestPlanSplitGroupMoneyIsOrderIndependentAndUnpriced(t *testing.T) {
	for name, costs := range map[string]string{
		"full costs":      `,"group_costs":{"pcba":{"cn":100,"th":900,"vn":500}}`,
		"th cost missing": `,"group_costs":{"pcba":{"cn":100,"vn":500}}`,
	} {
		a := splitGroupPlan(t, true, false, costs)
		b := splitGroupPlan(t, true, true, costs)
		for _, k := range []string{"duty_effect_per_year", "added_cost_per_year", "net_per_year", "net_priced"} {
			if a[k] != b[k] {
				t.Errorf("%s: %s depends on operation order: %v vs %v", name, k, a[k], b[k])
			}
		}
		if a["net_priced"] != false {
			t.Errorf("%s: net_priced = %v, want false", name, a["net_priced"])
		}
		if a["duty_effect_per_year"] != 200000.0 {
			t.Errorf("%s: duty effect %v", name, a["duty_effect_per_year"])
		}
	}
}

func TestPlanSingleGroupMoneyUnchanged(t *testing.T) {
	costs := `,"group_costs":{"pcba":{"cn":100,"th":900,"vn":500}}`
	for _, flip := range []bool{false, true} {
		r := splitGroupPlan(t, false, flip, costs)
		if r["added_cost_per_year"] != 400.0 || r["net_per_year"] != 199600.0 || r["net_priced"] != true {
			t.Errorf("flip=%v single plant money changed: %v %v %v", flip, r["added_cost_per_year"], r["net_per_year"], r["net_priced"])
		}
	}
	// duty-only (no group costs): split group still prices on duty alone.
	for _, flip := range []bool{false, true} {
		r := splitGroupPlan(t, true, flip, "")
		if r["net_per_year"] != 200000.0 || r["net_priced"] != true {
			t.Errorf("flip=%v duty-only changed: %v %v", flip, r["net_per_year"], r["net_priced"])
		}
	}
}
