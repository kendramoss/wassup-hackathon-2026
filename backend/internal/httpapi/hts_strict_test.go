package httpapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

var malformedHTS = []string{"garbage84198995", "841989950000000", "8419.89.95x", "8419.89.95; drop", "8419", "8419.89.9", "8419.89 95"}

func TestDutyRatesRejectsMalformedHTS(t *testing.T) {
	live, calls := fakeHTS(t)
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.LiveMFN = live })
	for _, in := range malformedHTS {
		code, out := hs.do(t, "GET", "/v1/duty-rates?hts="+url.QueryEscape(in), "")
		if code != http.StatusBadRequest || out["code"] != "invalid_input" {
			t.Errorf("%q = %d %v, want 400 invalid_input", in, code, out)
		}
		if msg, _ := out["message"].(string); strings.Contains(msg, in) && len(in) > 10 {
			t.Errorf("%q echoed in %q", in, msg)
		}
	}
	long := strings.Repeat("9", 5000)
	code, out := hs.do(t, "GET", "/v1/duty-rates?hts="+long, "")
	if msg, _ := out["message"].(string); code != http.StatusBadRequest || len(msg) > 300 {
		t.Errorf("long input = %d, message length %d", code, len(msg))
	}
	if calls.Load() != 0 {
		t.Errorf("%d live calls for malformed HTS", calls.Load())
	}
}

func TestDutyRatesValidFormsStillServed(t *testing.T) {
	live, calls := fakeHTS(t)
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.LiveMFN = live })
	for _, in := range []string{"8419.89.95", "84198995", "8419%2089%2095", "8419-89-95", "8419.89.9500", "8419899500"} {
		if code, out := hs.do(t, "GET", "/v1/duty-rates?hts="+in, ""); code != http.StatusOK || out["hts"] != "8419.89.95" {
			t.Errorf("%q = %d %v", in, code, out)
		}
	}
	for _, in := range []string{"8516.79.00", "8516.79.0000", "8516-79-00"} {
		if code, out := hs.do(t, "GET", "/v1/duty-rates?hts="+in, ""); code != http.StatusOK || out["live_mfn"] == nil {
			t.Errorf("live %q = %d %v", in, code, out)
		}
	}
	if calls.Load() != 1 {
		t.Errorf("live calls = %d, want 1 (one cached subheading)", calls.Load())
	}
}

func TestPlanLeavesMalformedHTSUnpriced(t *testing.T) {
	for _, bad := range malformedHTS {
		live, calls := fakeHTS(t)
		hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.LiveMFN = live })
		code, c := hs.do(t, "POST", "/v1/cases/predict", heatGunWithHTS(t, bad))
		if code != http.StatusCreated {
			t.Fatalf("predict %q = %d %v", bad, code, c)
		}
		id := c["id"].(string)
		body := `{"target":"Thailand","economics":{"illustrative":true,"annual_import_value_usd":2000000}}`
		if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/plan", body); code != http.StatusOK {
			t.Fatalf("%q plan = %d %v", bad, code, out)
		}
		e := hs.planner.got.Economics
		if len(e.DutyRates) != 0 || len(e.DutyRatesFromTable) != 0 || len(e.DutyRatesLiveMFN) != 0 {
			t.Errorf("%q priced: %+v", bad, e)
		}
		if !strings.Contains(e.DutyRatesNote, "not a valid 8- or 10-digit HTS number") || strings.Contains(e.DutyRatesNote, bad) {
			t.Errorf("%q note = %q", bad, e.DutyRatesNote)
		}
		if calls.Load() != 0 {
			t.Errorf("%q: %d live calls", bad, calls.Load())
		}
	}
}

func TestPlanPricesValidHTSForms(t *testing.T) {
	for _, good := range []string{"8419.89.95", "84198995", "8419 89 95", "8419-89-95", "8419.89.9500", "8419899585"} {
		hs := newHarness(t, "substantial_transformation")
		_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunWithHTS(t, good))
		id := c["id"].(string)
		body := `{"target":"Thailand","economics":{"illustrative":true,"annual_import_value_usd":2000000}}`
		if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/plan", body); code != http.StatusOK {
			t.Fatalf("%q plan = %d %v", good, code, out)
		}
		if e := hs.planner.got.Economics; e.DutyRates["China"] != 0.417 || len(e.DutyRatesFromTable) == 0 {
			t.Errorf("%q not priced: %+v", good, e)
		}
	}
}

func TestMoneyPrefillMalformedHTSNoLiveCall(t *testing.T) {
	live, calls := fakeHTS(t)
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.LiveMFN = live })
	prod := func(h string) string {
		return `{"product":{"name":"Heat gun","hts":"` + h + `","plants":[{"id":"cn","country":"China"}],"components":[{"name":"motor","made_in":"China","plant":"cn"}],"operations":[]}}`
	}
	for _, bad := range append([]string{"8516.79.00x"}, malformedHTS...) {
		code, out := hs.do(t, "POST", "/v1/products/money-prefill", prod(bad))
		if code != http.StatusOK || out["hts"] != nil || out["hts_source"] != nil || len(out["economics"].(map[string]any)["duty_rates"].(map[string]any)) != 0 {
			t.Errorf("%q = %d %v", bad, code, out)
		}
		if n, _ := out["hts_note"].(string); !strings.Contains(n, "not a valid") {
			t.Errorf("%q: no explicit note: %v", bad, out)
		}
	}
	if calls.Load() != 0 {
		t.Errorf("%d live calls for malformed HTS", calls.Load())
	}
	for _, good := range []string{"8516.79.00", "8516.79.0000", "8516-79-00"} {
		code, out := hs.do(t, "POST", "/v1/products/money-prefill", prod(good))
		if code != http.StatusOK || out["hts"] != "8516.79.00" || out["economics"].(map[string]any)["duty_rates"].(map[string]any)["China"] != 0.027 {
			t.Errorf("%q = %d %v", good, code, out)
		}
	}
}

func TestImportMalformedHTSIgnored(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	row := func(h string) string {
		csv := "Part,Made in,Factory,Operation group,Plant capabilities,Qty,Unit cost (USD),Annual units,Unit price (USD),HTS\n" +
			"main housing,Thailand,Rayong plant,housing_moulding,motor_fan_assembly,1,2,\"10,000\",$30," + h + "\n"
		b, _ := json.Marshal(map[string]any{"name": "Heat gun", "family": "motorised_tools_small_appliances", "csv": csv})
		return string(b)
	}
	for _, bad := range []string{"garbage84198995", "841989950000000"} {
		code, out := hs.do(t, "POST", "/v1/products/import", row(bad))
		if code != http.StatusOK {
			t.Fatalf("%q = %d %v", bad, code, out)
		}
		if h := out["product"].(map[string]any)["hts"]; h != nil && h != "" {
			t.Errorf("%q kept as %v", bad, h)
		}
		if src, _ := out["economics_sources"].(map[string]any); src["hts"] != nil {
			t.Errorf("%q sourced: %v", bad, src)
		}
		if b, _ := json.Marshal(out["warnings"]); strings.Contains(string(b), bad) {
			t.Errorf("warning echoes input: %s", b)
		}
	}
	for _, good := range []string{"8419.89.95", "8419 89 95", "8419899500"} {
		if _, out := hs.do(t, "POST", "/v1/products/import", row(good)); out["product"].(map[string]any)["hts"] != "8419.89.95" {
			t.Errorf("%q = %v", good, out["product"])
		}
	}
}
