package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestPlanFillsDutyRatesFromTheTable(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t)) // product has hts 8419.89.95
	id := c["id"].(string)
	body, _ := json.Marshal(map[string]any{"target": "Thailand", "economics": map[string]any{
		"illustrative": true, "annual_import_value_usd": 2000000, "duty_rates": map[string]float64{"Thailand": 0.2}}})
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/plan", string(body)); code != http.StatusOK {
		t.Fatalf("plan = %d %v", code, out)
	}
	e := hs.planner.got.Economics
	if e.DutyRates["China"] != 0.417 || e.DutyRates["Thailand"] != 0.2 || len(e.DutyRatesFromTable) != 3 || !strings.Contains(e.DutyRatesNote, "8419.89.95") {
		t.Errorf("economics = %+v", e)
	}
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	if r := got["economics"].(map[string]any)["duty_rates"].(map[string]any); len(r) != 1 {
		t.Errorf("table rates were stored on the case: %v", r)
	}
}

func TestDutyRatesEndpoint(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	code, out := hs.do(t, "GET", "/v1/duty-rates?hts=8419.89.9585", "")
	if code != http.StatusOK || out["hts"] != "8419.89.95" || len(out["rates"].([]any)) != 4 {
		t.Fatalf("line = %d %v", code, out)
	}
	if code, out := hs.do(t, "GET", "/v1/duty-rates?hts=6505.00.90", ""); code != http.StatusNotFound || !strings.Contains(out["message"].(string), "6505.00.90") {
		t.Errorf("unknown = %d %v", code, out)
	}
	if code, out := hs.do(t, "GET", "/v1/duty-rates", ""); code != http.StatusOK || len(out["lines"].([]any)) != 1 {
		t.Errorf("all = %d %v", code, out)
	}
}
