package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/textilerules"
)

// A knit cotton-free dress: cut and sewn in Vietnam from fabric knitted in
// China, which 19 CFR 102.21(e) for 6101-6117 (1) gives to the country of
// assembly.
func textileProduct(hts string) string {
	p := map[string]any{
		"name": "Knit dress", "hts": hts,
		"plants": []map[string]any{{"id": "cn1", "country": "China"}, {"id": "vn1", "country": "Vietnam"}},
		"components": []map[string]any{
			{"name": "knit fabric", "made_in": "China", "plant": "cn1"},
			{"name": "thread", "made_in": "China"},
		},
		"operations": []map[string]any{
			{"name": "knit the fabric", "plant": "cn1"},
			{"name": "cut and sew", "plant": "vn1", "group": "final_assembly"},
		},
		"textile": map[string]any{"knit_to_shape": "no", "two_or_more_components": "yes", "cotton_wool_16": "no"},
	}
	b, _ := json.Marshal(map[string]any{"date": "2026-03-01", "family": "furniture", "product": p})
	return string(b)
}

// With TEXTILE_RULES on, a textile product carries textile_rule_check; the
// answer and triage are what they would be without it, and off or without a
// textile code the case reads as before (D951).
func TestTextileRuleCheckIsDisplayOnly(t *testing.T) {
	rules, err := textilerules.Default()
	if err != nil {
		t.Fatal(err)
	}
	run := func(t *testing.T, on bool, hts string) map[string]any {
		hs := newHarnessWith(t, "textile_102_21", func(d *Deps) {
			if on {
				d.TextileRules = rules
			}
		})
		code, out := hs.do(t, "POST", "/v1/cases/predict", textileProduct(hts))
		if code != http.StatusCreated {
			t.Fatalf("predict = %d %v", code, out)
		}
		return out
	}
	base := run(t, false, "6104.43.2020")
	if _, has := base["textile_rule_check"]; has {
		t.Fatal("textile_rule_check present with the flag off")
	}
	out := run(t, true, "6104.43.2020")
	tc, ok := out["textile_rule_check"].(map[string]any)
	if !ok {
		t.Fatalf("no textile_rule_check: %v", out)
	}
	res := tc["result"].(map[string]any)
	if res["status"] != "decided" || res["origin"] != "Vietnam" || res["para"] != "(c)(2)" {
		t.Fatalf("result %v", res)
	}
	// Display only: the prediction and the route do not change.
	for _, k := range []string{"prediction", "triage"} {
		a, _ := json.Marshal(base[k])
		b, _ := json.Marshal(out[k])
		if string(a) != string(b) {
			t.Errorf("%s changed:\n%s\n%s", k, a, b)
		}
	}
	// The fake engine answers China, so the decided check differs: a flag only.
	if tc["review"] != true {
		t.Errorf("review = %v, want true (fake engine says China)", tc["review"])
	}
	// A code outside 102.21(b)(5): nothing is added.
	if _, has := run(t, true, "8473.30.5100")["textile_rule_check"]; has {
		t.Error("textile_rule_check on a non-textile code")
	}
}

func TestProductTextileFactsAreValidated(t *testing.T) {
	hs := newHarnessWith(t, "textile_102_21", nil)
	var m map[string]any
	_ = json.Unmarshal([]byte(textileProduct("6104.43.2020")), &m)
	m["product"].(map[string]any)["textile"] = map[string]any{"knit_to_shape": "maybe"}
	b, _ := json.Marshal(m)
	if code, _ := hs.do(t, "POST", "/v1/cases/predict", string(b)); code != http.StatusBadRequest && code != http.StatusUnprocessableEntity {
		t.Errorf("bad textile fact accepted: %d", code)
	}
}
