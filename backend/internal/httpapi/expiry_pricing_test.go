package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/duty"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

// 8483.50.90 is on China's List 1 (9903.88.01) with a described-product
// exclusion 8483.50.9040 whose stored end date is 2026-11-09.
func expiryHTS(t *testing.T) *duty.Live {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("keyword") == "8483.50.90" {
			_, _ = w.Write([]byte(`[{"htsno":"8483.50.90","description":"Other","general":"2.5%"}]`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(srv.Close)
	l := duty.NewLive("test", time.Hour, 2*time.Second)
	l.BaseURL = srv.URL
	return l
}

func planNote(t *testing.T, now func() time.Time, mutate func(*Deps)) (string, planner.Economics) {
	t.Helper()
	live := expiryHTS(t)
	add, err := duty.LoadAdditional()
	if err != nil {
		t.Fatal(err)
	}
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		d.LiveMFN, d.DutyAdd, d.Now = live, add, now
		if mutate != nil {
			mutate(d)
		}
	})
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunWithHTS(t, "8483.50.90"))
	body := `{"target":"Vietnam","economics":{"illustrative":true,"annual_import_value_usd":1000000}}`
	if code, out := hs.do(t, "POST", "/v1/cases/"+c["id"].(string)+"/plan", body); code != http.StatusOK {
		t.Fatalf("plan = %d %v", code, out)
	}
	e := hs.planner.got.Economics
	return e.DutyRatesNote, e
}

func fixedClock(s string) (func() time.Time, *atomic.Int32) {
	var n atomic.Int32
	d, _ := time.Parse(time.DateOnly, s)
	return func() time.Time { n.Add(1); return d.Add(12 * time.Hour) }, &n
}

func TestPlanExclusionExpiryByInjectedClock(t *testing.T) {
	for _, c := range []struct {
		day     string
		valid   bool
		expired bool
	}{{"2026-11-08", true, false}, {"2026-11-09", true, false}, {"2026-11-10", false, true}, {"2025-01-01", true, false}} {
		clk, n := fixedClock(c.day)
		note, e := planNote(t, clk, nil)
		if n.Load() != 1 {
			t.Errorf("%s: clock called %d times, want 1", c.day, n.Load())
		}
		mayApply := strings.Contains(note, "A product exclusion may apply for China (9903.88.01")
		expired := strings.Contains(note, "not applicable as of "+c.day) && strings.Contains(note, "per the embedded end date (2026-11-09)")
		if mayApply != c.valid || expired != c.expired {
			t.Errorf("%s: mayApply=%v expired=%v note=%q", c.day, mayApply, expired, note)
		}
		if strings.Contains(strings.ToLower(note), "extended") && !strings.Contains(note, "no statement about any later extension") {
			t.Errorf("%s: extension claim in %q", c.day, note)
		}
		// Never auto-applied: China rate = 2.5% MFN + 25% List 1 + 12.5% country.
		if got := e.DutyRates["China"]; got != 0.4 {
			t.Errorf("%s: China rate %v, want 0.4 (exclusion not applied)", c.day, got)
		}
	}
}

func TestPlanCaseDateDoesNotDriveExpiry(t *testing.T) {
	// The heat-gun case date is 2022; the as-of is the request clock.
	clk, _ := fixedClock("2026-11-20")
	note, _ := planNote(t, clk, nil)
	if strings.Contains(note, "may apply for China (9903.88.01") || !strings.Contains(note, "not applicable as of 2026-11-20") {
		t.Errorf("note = %q", note)
	}
}

func TestPlanClockCapturedOncePerRequestAcrossMidnight(t *testing.T) {
	var n atomic.Int32
	base, _ := time.Parse(time.DateOnly, "2026-11-09")
	clk := func() time.Time { // first call 23:59:59 on the 9th, later calls cross midnight
		if n.Add(1) == 1 {
			return base.Add(24*time.Hour - time.Second)
		}
		return base.Add(24*time.Hour + time.Second)
	}
	note, _ := planNote(t, clk, nil)
	if n.Load() != 1 {
		t.Errorf("clock called %d times in one plan request", n.Load())
	}
	if !strings.Contains(note, "may apply for China (9903.88.01") || strings.Contains(note, "not applicable as of") {
		t.Errorf("note = %q", note)
	}
}

func TestPlanNilClockFallsBackToRealUTC(t *testing.T) {
	note, _ := planNote(t, nil, nil)
	// Real "now" is past 2026-11-09: never present-tense applicability.
	if strings.Contains(note, "may apply for China (9903.88.01") || !strings.Contains(note, "not applicable as of ") {
		t.Errorf("nil clock note = %q", note)
	}
}

func TestAdditionalNoteWording(t *testing.T) {
	add, _ := duty.LoadAdditional()
	asOf := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	extras := map[string][]duty.Extra{"China": {{Name: "S301", Heading: "9903.88.01", Rate: 0.25}}}
	check := map[string][]duty.Exclusion{"China": {
		{Heading: "9903.88.01", Description: "bad date (end date unreadable or no as-of date: confirm this exclusion's dates before relying on it)", EndDate: "9 Nov 2026", Status: duty.StatusReview},
		{Heading: "9903.05.31", HTS: "8483.50.90", Description: "52(d): civil aircraft only"},
	}}
	expired := map[string][]duty.Exclusion{"China": {{Heading: "9903.88.01", EndDate: "2026-11-09", Description: "8483.50.9040", Status: duty.StatusExpired}}}
	n := additionalNote(add, extras, check, expired, asOf)
	for _, want := range []string{"confirm", "civil aircraft only. Not applied", "not applicable as of 2026-11-20 per the embedded end date (2026-11-09)", "no statement about any later extension"} {
		if !strings.Contains(n, want) {
			t.Errorf("missing %q in %q", want, n)
		}
	}
	if strings.Contains(n, "until )") || strings.Contains(n, "until ,") || strings.Contains(n, "is current") {
		t.Errorf("assurance/empty until in %q", n)
	}
	if additionalNote(add, nil, nil, expired, asOf) == "" {
		t.Error("expired-only note dropped")
	}
}
