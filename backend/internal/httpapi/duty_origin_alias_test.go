package httpapi

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/duty"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

// Regression for #718. Local fake HTS (3.7% general rate, a synthetic fixture),
// the repository's shipped additional.json as app data, and a fixed clock.

func aliasHarness(t *testing.T) harness {
	t.Helper()
	live, _ := fakeHTS(t)
	add, err := duty.LoadAdditional()
	if err != nil {
		t.Fatal(err)
	}
	now, _ := fixedClock("2026-11-23")
	return newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.LiveMFN, d.DutyAdd, d.Now = live, add, now })
}

func aliasProduct(country string) string {
	b, _ := json.Marshal(country)
	return `{"name":"Heater","hts":"8516.21.00","plants":[{"id":"p1","name":"Plant","country":` + string(b) + `}],` +
		`"components":[{"name":"body","made_in":` + string(b) + `,"plant":"p1"}],"operations":[]}`
}

func aliasPrefill(t *testing.T, hs harness, country, rates string) map[string]any {
	t.Helper()
	econ := ""
	if rates != "" {
		econ = `,"economics":{"annual_import_value_usd":1000000,"duty_rates":` + rates + `}`
	}
	code, out := hs.do(t, "POST", "/v1/products/money-prefill", `{"product":`+aliasProduct(country)+econ+`}`)
	if code != http.StatusOK {
		t.Fatalf("money-prefill %q = %d %v", country, code, out)
	}
	e, _ := out["economics"].(map[string]any)
	return e
}

func aliasPlanEconomics(t *testing.T, hs harness, country string) planner.Economics {
	t.Helper()
	var body map[string]any
	_ = json.Unmarshal([]byte(heatGunWithHTS(t, "8516.21.00")), &body)
	body["product"].(map[string]any)["plants"].([]any)[0].(map[string]any)["country"] = country
	b, _ := json.Marshal(body)
	code, c := hs.do(t, "POST", "/v1/cases/predict", string(b))
	if code != http.StatusCreated {
		t.Fatalf("predict %q = %d %v", country, code, c)
	}
	pc, _ := hs.do(t, "POST", "/v1/cases/"+c["id"].(string)+"/plan", `{"target":"Vietnam","economics":{"illustrative":true,"annual_import_value_usd":1000000}}`)
	if pc != http.StatusOK {
		t.Fatalf("plan %q = %d", country, pc)
	}
	return hs.planner.got.Economics
}

var column2Aliases = [][2]string{
	{"DPRK", "North Korea"}, {"Democratic People's Republic of Korea", "North Korea"}, {"Korea, North", "North Korea"},
	{"Republic of Cuba", "Cuba"}, {"Byelorussia", "Belarus"},
}

var extrasAliases = [][2]string{
	{"UAE", "United Arab Emirates"}, {"Chinese Taipei", "Taiwan"}, {"Taiwan, China", "Taiwan"}, {"Korea, South", "South Korea"},
}

func TestMoneyPrefillColumn2AliasesAreNotFilled(t *testing.T) {
	hs := aliasHarness(t)
	for _, p := range column2Aliases {
		for _, name := range []string{p[0], p[1]} {
			if e := aliasPrefill(t, hs, name, ""); len(e["duty_rates"].(map[string]any)) != 0 {
				t.Errorf("%q must not get a live rate, got %v", name, e["duty_rates"])
			}
		}
	}
}

func TestMoneyPrefillAliasMatchesCanonical(t *testing.T) {
	hs := aliasHarness(t)
	for _, p := range extrasAliases {
		a, c := aliasPrefill(t, hs, p[0], ""), aliasPrefill(t, hs, p[1], "")
		rates := c["duty_rates"].(map[string]any)
		if len(rates) != 1 || c["duty_additional"] == nil {
			t.Fatalf("canonical %q control: rates %v additional %v", p[1], rates, c["duty_additional"])
		}
		for _, k := range []string{"duty_rates", "duty_additional", "duty_rates_live_mfn"} {
			if !reflect.DeepEqual(a[k], c[k]) {
				t.Errorf("%q vs %q: %s = %v, want %v", p[0], p[1], k, a[k], c[k])
			}
		}
		if n, _ := a["duty_rates_note"].(string); strings.Contains(n, p[0]) {
			t.Errorf("note names the raw spelling %q: %s", p[0], n)
		}
	}
}

func TestPlanColumn2AndExtrasAliasesPriceAsCanonical(t *testing.T) {
	hs := aliasHarness(t)
	for _, p := range append(append([][2]string{}, column2Aliases...), extrasAliases...) {
		a, c := aliasPlanEconomics(t, hs, p[0]), aliasPlanEconomics(t, hs, p[1])
		if !reflect.DeepEqual(a.DutyRates, c.DutyRates) {
			t.Errorf("%q vs %q: plan rates %v, want %v", p[0], p[1], a.DutyRates, c.DutyRates)
		}
		if !reflect.DeepEqual(a.DutyAdditional, c.DutyAdditional) {
			t.Errorf("%q vs %q: plan extras differ: %v, want %v", p[0], p[1], a.DutyAdditional, c.DutyAdditional)
		}
	}
}

// A manual rate given under an alias is kept as given; the canonical origin is
// not filled over it and no extras are layered on it.
func TestPrefillKeepsManualAliasRate(t *testing.T) {
	hs := aliasHarness(t)
	e := aliasPrefill(t, hs, "UAE", `{"UAE":0.5}`)
	rates := e["duty_rates"].(map[string]any)
	if len(rates) != 1 || rates["UAE"] != 0.5 {
		t.Errorf("manual alias rate not preserved: %v", rates)
	}
	if e["duty_additional"] != nil {
		t.Errorf("extras layered over a manual rate: %v", e["duty_additional"])
	}
	if s, _ := e["duty_rates_live_mfn"].([]any); len(s) != 0 {
		t.Errorf("live fill despite a manual rate: %v", s)
	}
}

func TestPrefillManualZeroAliasRateIsKept(t *testing.T) {
	hs := aliasHarness(t)
	e := aliasPrefill(t, hs, "Taiwan", `{"Chinese Taipei":0}`)
	if r := e["duty_rates"].(map[string]any); len(r) != 1 || r["Chinese Taipei"] != 0.0 {
		t.Errorf("explicit zero not preserved: %v", r)
	}
}
