package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/duty"
)

// fakeHTS answers GET /reststop/search like hts.usitc.gov (D65): 8516.79.00
// at 2.7% (not in the sourced table), 6505.00.90 compound, 5555.55.55 down.
func fakeHTS(t *testing.T) (*duty.Live, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch r.URL.Query().Get("keyword") {
		case "8516.79.00":
			_, _ = w.Write([]byte(`[{"htsno":"8516.79.00","description":"Other","general":"2.7%"}]`))
		case "6505.00.90":
			_, _ = w.Write([]byte(`[{"htsno":"6505.00.90","description":"Other","general":"20.7¢/kg + 7.5%"}]`))
		case "5555.55.55":
			http.Error(w, "down", http.StatusBadGateway)
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	t.Cleanup(srv.Close)
	l := duty.NewLive("test", time.Hour, 2*time.Second)
	l.BaseURL = srv.URL
	return l, &calls
}

// heatGunWithHTS is the heat-gun case body with another HTS number.
func heatGunWithHTS(t *testing.T, hts string) string {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(heatGunBody(t)), &body); err != nil {
		t.Fatal(err)
	}
	body["product"].(map[string]any)["hts"] = hts
	b, _ := json.Marshal(body)
	return string(b)
}

func TestDutyRatesLiveMFN(t *testing.T) {
	live, calls := fakeHTS(t)
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		d.LiveMFN = live
		d.Limits = Limits{LightPerMin: 100, HeavyPerMin: 100, GlobalPerMin: 1000, LookupPerMin: 2}
	})
	// The sourced table wins: no fetch.
	if code, out := hs.do(t, "GET", "/v1/duty-rates?hts=8419.89.95", ""); code != http.StatusOK || len(out["rates"].([]any)) != 4 || out["live_mfn"] != nil {
		t.Fatalf("table line = %d %v", code, out)
	}
	if calls.Load() != 0 {
		t.Errorf("table line fetched the HTS")
	}
	code, out := hs.do(t, "GET", "/v1/duty-rates?hts=8516.79.0000", "")
	if code != http.StatusOK || out["hts"] != "8516.79.00" {
		t.Fatalf("live line = %d %v", code, out)
	}
	m := out["live_mfn"].(map[string]any)
	if m["rate"] != 0.027 || m["general"] != "2.7%" || m["supported"] != true || m["mfn_only"] != true || m["live"] != true || m["fetched_at"] == "" ||
		!strings.Contains(m["source"].(string), "/reststop/search?keyword=8516.79.00") || !strings.Contains(m["note"].(string), "Section 301, forced-labor, 232") {
		t.Errorf("live_mfn = %v", m)
	}
	// Compound: answered, but with no rate.
	if code, out := hs.do(t, "GET", "/v1/duty-rates?hts=6505.00.90", ""); code != http.StatusOK || out["live_mfn"].(map[string]any)["rate"] != nil ||
		out["live_mfn"].(map[string]any)["supported"] != false {
		t.Errorf("compound = %d %v", code, out)
	}
	// The lookup bucket (2/min) is spent; a cached answer is still free.
	if code, _ := hs.do(t, "GET", "/v1/duty-rates?hts=8516.79.00", ""); code != http.StatusOK {
		t.Errorf("cached answer = %d", code)
	}
	if code, _ := hs.do(t, "GET", "/v1/duty-rates?hts=9999.99.99", ""); code != http.StatusTooManyRequests {
		t.Errorf("uncached past the lookup limit = %d, want 429", code)
	}
	if code, _ := hs.doAs(t, "GET", "/v1/duty-rates?hts=9999.99.99", "", map[string]string{"X-Admin-Token": testAdmin}); code != http.StatusNotFound {
		t.Errorf("missing line = %d, want 404", code)
	}
	if code, _ := hs.doAs(t, "GET", "/v1/duty-rates?hts=5555.55.55", "", map[string]string{"X-Admin-Token": testAdmin}); code != http.StatusNotFound {
		t.Errorf("HTS down = %d, want 404 (no rate, not an error)", code)
	}
}

func TestDutyRatesLiveOffByDefault(t *testing.T) {
	hs := newHarness(t, "substantial_transformation") // LiveMFN nil: DUTY_LIVE_MFN=off
	if code, _ := hs.do(t, "GET", "/v1/duty-rates?hts=8516.79.00", ""); code != http.StatusNotFound {
		t.Errorf("live off = %d, want 404", code)
	}
}

func TestPlanFillsLiveMFN(t *testing.T) {
	live, calls := fakeHTS(t)
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.LiveMFN = live })
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunWithHTS(t, "8516.79.00"))
	id := c["id"].(string)
	if c["economics_priced"] != nil {
		t.Errorf("economics_priced without economics = %v", c["economics_priced"])
	}
	body := `{"target":"Vietnam","economics":{"illustrative":true,"annual_import_value_usd":1000000,"duty_rates":{"Thailand":0.2}}}`
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/plan", body); code != http.StatusOK {
		t.Fatalf("plan = %d %v", code, out)
	}
	e := hs.planner.got.Economics
	// The user's Thailand rate wins; China (plants, components, current
	// origin) and the target Vietnam get the MFN rate.
	if e.DutyRates["Thailand"] != 0.2 || e.DutyRates["China"] != 0.027 || e.DutyRates["Vietnam"] != 0.027 || len(e.DutyRates) != 3 {
		t.Errorf("rates = %v", e.DutyRates)
	}
	if strings.Join(e.DutyRatesLiveMFN, ",") != "China,Vietnam" || len(e.DutyRatesFromTable) != 0 {
		t.Errorf("live = %v table = %v", e.DutyRatesLiveMFN, e.DutyRatesFromTable)
	}
	if !strings.Contains(e.DutyRatesNote, "live HTS MFN rate for 8516.79.00, 2.7%") || !strings.Contains(e.DutyRatesNote, "Section 301, forced-labor, 232") {
		t.Errorf("note = %q", e.DutyRatesNote)
	}
	// Live rates are not stored; economics_priced shows them from the cache.
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	if r := got["economics"].(map[string]any)["duty_rates"].(map[string]any); len(r) != 1 {
		t.Errorf("live rates were stored on the case: %v", r)
	}
	priced := got["economics_priced"].(map[string]any)
	if r := priced["duty_rates"].(map[string]any); r["China"] != 0.027 || len(priced["duty_rates_live_mfn"].([]any)) != 1 {
		t.Errorf("economics_priced = %v", priced) // Vietnam was the plan's target, not one of the case's own countries
	}
	if calls.Load() != 1 {
		t.Errorf("%d HTS fetches, want 1 (cached)", calls.Load())
	}
}

func TestPlanLiveMFNFallsBackQuietly(t *testing.T) {
	live, calls := fakeHTS(t)
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.LiveMFN = live })
	for _, tt := range []struct{ hts, note string }{
		{"5555.55.55", ""},                     // HTS down: no rate, no error
		{"6505.00.90", "specific or compound"}, // compound: no rate, says why
		{"8419.89.95", "sourced table"},        // the table wins
	} {
		_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunWithHTS(t, tt.hts))
		body := `{"target":"Thailand","economics":{"illustrative":true,"annual_import_value_usd":1000000,"duty_rates":{"Thailand":0.2}}}`
		if code, out := hs.do(t, "POST", "/v1/cases/"+c["id"].(string)+"/plan", body); code != http.StatusOK {
			t.Fatalf("%s: plan = %d %v", tt.hts, code, out)
		}
		e := hs.planner.got.Economics
		if len(e.DutyRatesLiveMFN) != 0 || e.DutyRates["Thailand"] != 0.2 {
			t.Errorf("%s: rates = %v live = %v", tt.hts, e.DutyRates, e.DutyRatesLiveMFN)
		}
		if tt.hts == "8419.89.95" {
			if e.DutyRates["China"] != 0.417 {
				t.Errorf("table rate lost: %v", e.DutyRates)
			}
		} else if len(e.DutyRates) != 1 {
			t.Errorf("%s: filled %v", tt.hts, e.DutyRates)
		}
		if !strings.Contains(e.DutyRatesNote, tt.note) {
			t.Errorf("%s: note = %q", tt.hts, e.DutyRatesNote)
		}
	}
	// Every origin already priced: the HTS is not asked.
	n := calls.Load()
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunWithHTS(t, "8516.79.00"))
	body := `{"target":"Thailand","economics":{"illustrative":true,"annual_import_value_usd":1000000,"duty_rates":{"Thailand":0.2,"China":0.3}}}`
	if code, _ := hs.do(t, "POST", "/v1/cases/"+c["id"].(string)+"/plan", body); code != http.StatusOK || calls.Load() != n {
		t.Errorf("fully priced plan = %d, %d new fetches", code, calls.Load()-n)
	}
}
