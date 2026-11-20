package httpapi

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// #365 (was #357): a streamed plan search must store exactly the economics the
// user sent, like the JSON plan path. Table, live and date-derived rates stay
// derived: the planner is priced with them and economics_priced shows them,
// but the stored case holds only the raw inputs.
func planThenRead(t *testing.T, stream bool, body string) (hs harness, stored, priced map[string]any) {
	t.Helper()
	hs = newHarness(t, "substantial_transformation")
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t)) // product has hts 8419.89.95
	id := c["id"].(string)
	req := httptest.NewRequest("POST", "/v1/cases/"+id+"/plan", strings.NewReader(body))
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("plan (stream=%v) = %d %s", stream, rec.Code, rec.Body.String())
	}
	if stream && !strings.Contains(rec.Body.String(), "event: done") {
		t.Fatalf("stream did not finish: %s", rec.Body.String())
	}
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	stored, _ = got["economics"].(map[string]any)
	priced, _ = got["economics_priced"].(map[string]any)
	return hs, stored, priced
}

func TestStreamedPlanStoresRawEconomicsLikeJSON(t *testing.T) {
	body := `{"target":"Thailand","economics":{"illustrative":true,"annual_import_value_usd":2000000,"duty_rates":{"Thailand":0.2}}}`
	jsonHS, jsonStored, jsonPriced := planThenRead(t, false, body)
	streamHS, streamStored, streamPriced := planThenRead(t, true, body)

	if !reflect.DeepEqual(jsonStored, streamStored) {
		t.Fatalf("stored economics differ\n json:   %v\n stream: %v", jsonStored, streamStored)
	}
	if r, _ := streamStored["duty_rates"].(map[string]any); len(r) != 1 || r["Thailand"] != 0.2 {
		t.Errorf("stored rates are not the one the user entered: %v", r)
	}
	for _, k := range []string{"duty_rates_from_table", "duty_rates_live_mfn", "duty_rates_note"} {
		if v, ok := streamStored[k]; ok && v != nil {
			t.Errorf("derived field %s was stored: %v", k, v)
		}
	}
	// The search itself still ran on priced economics, on both paths.
	for name, hs := range map[string]harness{"json": jsonHS, "stream": streamHS} {
		e := hs.planner.got.Economics
		if e.DutyRates["China"] != 0.417 || e.DutyRates["Thailand"] != 0.2 || len(e.DutyRatesFromTable) != 3 {
			t.Errorf("%s: planner was not priced: %+v", name, e)
		}
	}
	// And the case still shows the derived rates for display.
	for name, p := range map[string]map[string]any{"json": jsonPriced, "stream": streamPriced} {
		if r, _ := p["duty_rates"].(map[string]any); r["China"] != 0.417 || r["Thailand"] != 0.2 {
			t.Errorf("%s: economics_priced lost the derived rates: %v", name, p)
		}
	}
}

func TestStreamedPlanWithoutMoneyKeepsStoredEconomics(t *testing.T) {
	// control: a body with no amount leaves what the case already holds.
	hs := newHarness(t, "substantial_transformation")
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	if code, _ := hs.do(t, "PUT", "/v1/cases/"+id+"/economics", `{"illustrative":true,"annual_import_value_usd":1500000}`); code != http.StatusOK {
		t.Fatalf("put economics = %d", code)
	}
	req := httptest.NewRequest("POST", "/v1/cases/"+id+"/plan", strings.NewReader(`{"target":"Thailand"}`))
	req.Header.Set("Accept", "text/event-stream")
	hs.h.ServeHTTP(httptest.NewRecorder(), req)
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	st := got["economics"].(map[string]any)
	if st["annual_import_value_usd"] != 1500000.0 {
		t.Errorf("stored economics changed: %v", st)
	}
	if r, _ := st["duty_rates"].(map[string]any); len(r) != 0 {
		t.Errorf("table rates leaked into the stored case: %v", r)
	}
}
