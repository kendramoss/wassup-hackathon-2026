package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

// heatGunWithCodes is the heat gun with an (illustrative) HTS code on every
// part, so the per-move rule check runs on a substantial-transformation case.
func heatGunWithCodes(t *testing.T) string {
	t.Helper()
	var m map[string]any
	_ = json.Unmarshal([]byte(heatGunBody(t)), &m)
	codes := map[string]string{"housing_moulding": "3926.90", "heating_element": "8516.80", "motor_fan_assembly": "8501.10",
		"pcba": "8537.10", "switch_cord_minor_parts": "8536.50", "cord_knobs_covers": "3926.90"}
	for _, c := range m["product"].(map[string]any)["components"].([]any) {
		c := c.(map[string]any)
		c["hts"] = codes[c["group"].(string)]
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// Every plan of a search on a product with codes carries the 102.20 check of
// the product after its move; it is added when returned or read, never
// stored, and changes no verdict, Shown or order (D441).
func TestPlanMovesCarryTheRuleCheck(t *testing.T) {
	rules, err := tariffshift.Default()
	if err != nil {
		t.Fatal(err)
	}
	cases := store.NewMemory()
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.RuleCheck, d.Cases = rules, cases })
	code, out := hs.do(t, "POST", "/v1/cases/predict", heatGunWithCodes(t))
	if code != http.StatusCreated {
		t.Fatalf("predict %d %v", code, out)
	}
	id := out["id"].(string)
	code, pl := hs.do(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand"}`)
	if code != http.StatusOK {
		t.Fatalf("plan %d %v", code, pl)
	}
	check := func(where string, res map[string]any) {
		t.Helper()
		plans, rej := res["plans"].([]any), res["rejected"].([]any)
		if len(plans) != 1 || len(rej) != 2 {
			t.Fatalf("%s: plans changed: %d shown, %d rejected", where, len(plans), len(rej))
		}
		for _, x := range append(append([]any{res["baseline"]}, plans...), rej...) {
			rc, ok := x.(map[string]any)["rule_check"].(map[string]any)
			if !ok {
				t.Fatalf("%s: a plan without rule_check: %v", where, x)
			}
			if rc["applies"] != "informational" || rc["country"] != "Thailand" || rc["label"] == "" {
				t.Fatalf("%s: %v", where, rc)
			}
		}
		p0 := plans[0].(map[string]any)
		if p0["shown"] != true || p0["verdict"].(map[string]any)["confidence"] != 0.96 {
			t.Fatalf("%s: the verdict changed: %v", where, p0)
		}
	}
	check("plan response", pl["plan"].(map[string]any))
	code, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	if code != http.StatusOK {
		t.Fatalf("get %d", code)
	}
	check("case read", got["plan"].(map[string]any))

	stored, err := cases.Get(context.Background(), id)
	if err != nil || stored.Plan == nil {
		t.Fatalf("stored %v %v", stored.Plan, err)
	}
	for _, p := range append(append(stored.Plan.Plans, stored.Plan.Rejected...), stored.Plan.Baseline) {
		if p.RuleCheck != nil {
			t.Fatal("the check was stored")
		}
	}
}

// A substantial-transformation case without part codes reads as before.
func TestPlanRuleCheckAbsentWithoutCodes(t *testing.T) {
	rules, _ := tariffshift.Default()
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.RuleCheck = rules })
	code, out := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	if code != http.StatusCreated {
		t.Fatalf("predict %d", code)
	}
	code, pl := hs.do(t, "POST", "/v1/cases/"+out["id"].(string)+"/plan", `{"target":"Thailand"}`)
	if code != http.StatusOK {
		t.Fatalf("plan %d", code)
	}
	for _, x := range pl["plan"].(map[string]any)["plans"].([]any) {
		if _, ok := x.(map[string]any)["rule_check"]; ok {
			t.Fatal("rule_check on a case without part codes")
		}
	}
}
