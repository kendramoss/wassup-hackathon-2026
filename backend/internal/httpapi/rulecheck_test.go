package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

func ruleCheckProduct(partHTS string) string {
	p := map[string]any{
		"name": "Thermal module", "hts": "8473.30.5100",
		"plants": []map[string]any{{"id": "mx1", "country": "Mexico"}, {"id": "cn1", "country": "China"}},
		"components": []map[string]any{
			{"name": "aluminum fins", "made_in": "China", "plant": "cn1", "hts": partHTS},
			{"name": "copper heat pipe", "made_in": "China", "plant": "cn1", "hts": "7411.10"},
		},
		"operations": []map[string]any{{"name": "solder and braze the module", "plant": "mx1", "group": "final_assembly"}},
	}
	b, _ := json.Marshal(map[string]any{"date": "2026-03-01", "family": "pcba_electronic_devices", "product": p})
	return string(b)
}

// The rule check rides along with a Part 102 case that gives codes, and
// changes neither the prediction nor the triage (D363).
func TestRuleCheckShownBesideTheModel(t *testing.T) {
	rules, err := tariffshift.Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, regime, partHTS, applies, status, origin string
		on, present, review                            bool
	}{
		{name: "part 102, decided, agrees", regime: "usmca_part102", partHTS: "7616.99", on: true, present: true, applies: "part102", status: tariffshift.Decided, origin: "Mexico"},
		{name: "part 102, a part already of 8473: open on de minimis, no flag", regime: "usmca_part102", partHTS: "8473.30.11", on: true, present: true, applies: "part102", status: tariffshift.Undetermined, review: false},
		{name: "substantial transformation: informational, and the general engine (China) differs", regime: "substantial_transformation", partHTS: "7616.99", on: true, present: true, applies: "informational", status: tariffshift.Decided, origin: "Mexico", review: true},
		{name: "switched off", regime: "usmca_part102", partHTS: "7616.99", on: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hs := newHarnessWith(t, tt.regime, func(d *Deps) {
				d.USMCA, d.USMCAFastPath = usmcaEngine{conf: 0.97}, 0
				if tt.on {
					d.RuleCheck = rules
				}
			})
			code, out := hs.do(t, "POST", "/v1/cases/predict", ruleCheckProduct(tt.partHTS))
			if code != http.StatusCreated {
				t.Fatalf("predict = %d %v", code, out)
			}
			rc, ok := out["rule_check"].(map[string]any)
			if ok != tt.present {
				t.Fatalf("rule_check present %v, want %v: %v", ok, tt.present, out["rule_check"])
			}
			if !tt.present {
				return
			}
			res := rc["result"].(map[string]any)
			if rc["applies"] != tt.applies || res["status"] != tt.status || rc["country"] != "Mexico" || rc["country_from"] != "final assembly plant" {
				t.Fatalf("rule_check %v", rc)
			}
			if tt.origin != "" && res["origin"] != tt.origin {
				t.Fatalf("origin %v", res["origin"])
			}
			if rc["review"] != tt.review {
				t.Fatalf("review %v", rc["review"])
			}
			// The same case read back carries the same check.
			code, got := hs.do(t, "GET", "/v1/cases/"+out["id"].(string), "")
			if code != http.StatusOK || got["rule_check"] == nil {
				t.Fatalf("GET = %d, rule_check %v", code, got["rule_check"])
			}
		})
	}
}

// A decided rule check that differs from the model is flagged for review,
// and the triage route is left as the engine's bar set it.
func TestRuleCheckDisagreementIsOnlyAFlag(t *testing.T) {
	rules, _ := tariffshift.Default()
	hs := newHarnessWith(t, "usmca_part102", func(d *Deps) {
		d.USMCA, d.USMCAFastPath = usmcaEngine{conf: 0.97}, 0.9
		d.RuleCheck = rules
	})
	// Both parts already of heading 8473 and from China: no shift, and both
	// candidates for essential character are Chinese, so the rule says China.
	body := ruleCheckProduct("8473.30.11")
	var m map[string]any
	_ = json.Unmarshal([]byte(body), &m)
	comps := m["product"].(map[string]any)["components"].([]any)
	comps[1].(map[string]any)["hts"] = "8473.30.20"
	b, _ := json.Marshal(m)
	code, out := hs.do(t, "POST", "/v1/cases/predict", string(b))
	if code != http.StatusCreated {
		t.Fatalf("predict = %d %v", code, out)
	}
	rc := out["rule_check"].(map[string]any)
	res := rc["result"].(map[string]any)
	// No value shares, so de minimis keeps it open; the answer if de
	// minimis does not apply is China.
	if res["status"] != tariffshift.Undetermined || res["if_not_de_minimis"].(map[string]any)["origin"] != "China" {
		t.Fatalf("rule_check %v", rc)
	}
	if tr := out["triage"].(map[string]any); tr["route"] != "auto" {
		t.Fatalf("the rule check must not change the route: %v", tr)
	}
}

// A case without codes reads exactly as before: no field.
func TestRuleCheckAbsentWithoutCodes(t *testing.T) {
	rules, _ := tariffshift.Default()
	hs := newHarnessWith(t, "usmca_part102", func(d *Deps) { d.RuleCheck = rules })
	code, out := hs.do(t, "POST", "/v1/cases/predict", `{"date":"2026-03-01","facts":"Steel bolt seals stamped and assembled in Mexico from Chinese steel wire."}`)
	if code != http.StatusCreated {
		t.Fatalf("predict = %d", code)
	}
	if _, ok := out["rule_check"]; ok {
		t.Fatal("rule_check on a case without codes")
	}
}
