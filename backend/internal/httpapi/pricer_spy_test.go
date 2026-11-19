package httpapi

import (
	"net/http"
	"testing"
)

// Invalid economics are rejected (#293) before pricing: the counting HTS
// fake sees no lookup for a live-only HTS number.
func TestInvalidEconomicsReachNoLiveLookup(t *testing.T) {
	live, calls := fakeHTS(t)
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.LiveMFN = live })
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunWithHTS(t, "8516.79.00"))
	id := c["id"].(string)
	body := `{"target":"Vietnam","economics":{"illustrative":true,"annual_import_value_usd":-5,"duty_rates":{"Thailand":2}}}`
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/plan", body); code != http.StatusBadRequest {
		t.Fatalf("plan = %d %v, want 400", code, out)
	}
	if n := calls.Load(); n != 0 {
		t.Errorf("%d live lookups for invalid economics", n)
	}
}
