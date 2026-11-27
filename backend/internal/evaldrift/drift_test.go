package evaldrift

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func base() Point { return Point{ID: "cbp_final", Kind: KindBaseline, Correct: 340, Cases: 377} }

func hist(months ...Point) []Point { return append([]Point{base()}, months...) }

func month(id string, k, n int) Point { return Point{ID: id, Kind: KindMonthly, Correct: k, Cases: n} }

func only(t *testing.T, r []Result) Result {
	t.Helper()
	if len(r) != 1 {
		t.Fatalf("results = %d, want 1: %+v", len(r), r)
	}
	return r[0]
}

// A month that matches the earlier accuracy raises nothing.
func TestNoDropNoAlert(t *testing.T) {
	r := only(t, Evaluate(hist(month("2026-12", 36, 40)), DefaultRule))
	if r.Alert || r.Status != StatusOK {
		t.Fatalf("90%% against 90%% alerted: %+v", r)
	}
	// Better than before is not an alert either.
	if r := only(t, Evaluate(hist(month("2026-12", 40, 40)), DefaultRule)); r.Alert {
		t.Fatalf("improvement alerted: %+v", r)
	}
}

// A real fall alerts; a fall that clears the earlier interval entirely is severe.
func TestRealDropAlerts(t *testing.T) {
	r := only(t, Evaluate(hist(month("2026-12", 28, 40)), DefaultRule)) // 70%
	if !r.Alert || r.Severity != SeverityBelowInterval {
		t.Fatalf("70%% on 40 cases: %+v", r)
	}
	if r.PValue > DefaultRule.Alpha {
		t.Fatalf("p = %v", r.PValue)
	}
	// 83.75% on 400: a small fall, but certain; the month's interval still reaches the earlier one.
	r = only(t, Evaluate(hist(month("2026-12", 335, 400)), DefaultRule))
	if !r.Alert || r.Severity != SeverityDrop {
		t.Fatalf("83.75%% on 400 cases: %+v", r)
	}
	// 31 of 40 (77.5%) is not yet evidence at alpha 0.01.
	if r := only(t, Evaluate(hist(month("2026-12", 31, 40)), DefaultRule)); r.Alert {
		t.Fatalf("77.5%% on 40 cases alerted: %+v", r)
	}
}

// Small months are noise: too few cases never alert, and a middling dip does not.
func TestSmallNIsNotAnAlert(t *testing.T) {
	r := only(t, Evaluate(hist(month("2026-12", 5, 10)), DefaultRule)) // 50% on 10
	if r.Alert || r.Status != StatusTooFew {
		t.Fatalf("10 cases: %+v", r)
	}
	r = only(t, Evaluate(hist(month("2026-12", 16, 20)), DefaultRule)) // 80% on 20: ordinary variation
	if r.Alert {
		t.Fatalf("80%% on 20 cases alerted: %+v", r)
	}
}

// With no earlier figure there is nothing to fall below.
func TestNeedsABaseline(t *testing.T) {
	r := only(t, Evaluate([]Point{month("2026-12", 5, 40)}, DefaultRule))
	if r.Alert || r.Status != StatusNoBaseline {
		t.Fatalf("no baseline: %+v", r)
	}
	small := []Point{{ID: "x", Kind: KindBaseline, Correct: 9, Cases: 10}, month("2026-12", 5, 40)}
	if r := only(t, Evaluate(small, DefaultRule)); r.Alert || r.Status != StatusNoBaseline {
		t.Fatalf("10-case baseline: %+v", r)
	}
}

// Each month is judged against the months before it only, pooled; a month
// already alerted on drags the later baseline down, and a later good month is not blamed.
func TestMonthsAreJudgedAgainstEarlierOnes(t *testing.T) {
	rs := Evaluate(hist(month("2026-12", 36, 40), month("2027-01", 28, 40), month("2027-02", 37, 40)), DefaultRule)
	if len(rs) != 3 || rs[0].Alert || !rs[1].Alert || rs[2].Alert {
		t.Fatalf("results: %+v", rs)
	}
	if rs[1].Baseline.Cases != 377+40 || rs[1].Baseline.Correct != 340+36 {
		t.Fatalf("pooled baseline of the second month: %+v", rs[1].Baseline)
	}
	// Order in the file does not matter: months are taken in calendar order.
	rs2 := Evaluate(hist(month("2027-02", 37, 40), month("2026-12", 36, 40), month("2027-01", 28, 40)), DefaultRule)
	for i := range rs {
		if rs[i].Month != rs2[i].Month || rs[i].Alert != rs2[i].Alert || rs[i].PValue != rs2[i].PValue {
			t.Fatalf("order changed the result: %+v vs %+v", rs, rs2)
		}
	}
}

func TestFisherOneSidedKnownValues(t *testing.T) {
	// 0 of 5 against 5 of 5: C(5,5)/C(10,5) = 1/252.
	if p := fisherLower(0, 5, 5, 5); abs(p-1.0/252) > 1e-12 {
		t.Fatalf("p = %v", p)
	}
	// Equal proportions: about half or more.
	if p := fisherLower(5, 10, 5, 10); p < 0.5 {
		t.Fatalf("p = %v", p)
	}
	// Sum of the whole lower tail is 1.
	if p := fisherLower(10, 10, 5, 10); abs(p-1) > 1e-12 {
		t.Fatalf("p = %v", p)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// Persisted history: the file round-trips, rejects duplicates and a changed month.
func TestRecordIsIdempotentAndRefusesChange(t *testing.T) {
	h := &History{Series: map[string][]Point{"cbp": {base()}}}
	if added, err := h.Record("cbp", month("2026-12", 36, 40)); err != nil || !added {
		t.Fatalf("record: %v %v", added, err)
	}
	if added, err := h.Record("cbp", month("2026-12", 36, 40)); err != nil || added {
		t.Fatalf("again: %v %v", added, err)
	}
	if _, err := h.Record("cbp", month("2026-12", 30, 40)); err == nil || !strings.Contains(err.Error(), "2026-12") {
		t.Fatalf("a changed month was accepted: %v", err)
	}
	if _, err := h.Record("cbp", Point{ID: "2027-01", Kind: KindMonthly, Correct: 41, Cases: 40}); err == nil {
		t.Fatal("more correct than cases accepted")
	}
	if _, err := h.Record("cbp", month("Dec", 1, 2)); err == nil {
		t.Fatal("bad month id accepted")
	}
}

func TestFileRoundTripAndRecompute(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "history.json")
	h := &History{Series: map[string][]Point{"cbp": hist(month("2026-12", 28, 40)), "japan": {{ID: "jp_fresh_1126", Kind: KindBaseline, Correct: 37, Cases: 44}}}}
	if err := h.Save(p); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	a, b := Compute(h, DefaultRule), Compute(got, DefaultRule)
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Fatalf("recompute differs:\n%s\n%s", ja, jb)
	}
	if len(a.Alerts()) != 1 || a.Alerts()[0].Series != "cbp" {
		t.Fatalf("alerts: %+v", a.Alerts())
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a.Alerts()[0].Line(), "2026-12") {
		t.Fatalf("summary line: %q", a.Alerts()[0].Line())
	}
}
