package main

import (
	"math"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/adcvd"
)

func TestWilson(t *testing.T) {
	r := wilson(8, 10)
	if math.Abs(r.P-0.8) > 1e-9 || math.Abs(r.Lo-0.4902) > 1e-3 || math.Abs(r.Hi-0.9433) > 1e-3 {
		t.Errorf("wilson(8,10) = %+v", r)
	}
	if r := wilson(0, 0); r.N != 0 || r.P != 0 {
		t.Errorf("wilson(0,0) = %+v", r)
	}
	if r := wilson(5, 5); r.Hi != 1 || r.Lo < 0.5 {
		t.Errorf("wilson(5,5) = %+v", r)
	}
}

func TestScoreCountsOwnOrderFlags(t *testing.T) {
	tb := adcvd.NewTable("t", "2024-12-31", []adcvd.Order{
		{Case: "A-570-900", Type: "AD", Country: "The People's Republic of China", Product: "Diamond Sawblades", Status: "active", HTS: []string{"8202.39.00"}},
		{Case: "A-570-111", Type: "AD", Country: "The People's Republic of China", Product: "Other", Status: "active", HTS: []string{"7326.90"}},
	})
	rulings := []ruling{
		{ID: "r1", Cases: []string{"A-570-900"}, OrderCountry: "China"},   // within, flagged: TP
		{ID: "r2", Cases: []string{"A-570-900"}, OrderCountry: "China"},   // outside, flagged: FP
		{ID: "r3", Cases: []string{"A-570-900"}, OrderCountry: "China"},   // within, other code: FN
		{ID: "r4", Cases: []string{"A-570-900"}, OrderCountry: "China"},   // outside, other code: TN
		{ID: "r5", Cases: []string{"A-570-999"}, OrderCountry: "China"},   // within, order not in snapshot, code of another order: FN, gap; flagged for the other order only
		{ID: "r6", Cases: []string{"A-570-900"}, OrderCountry: "China"},   // mixed: skipped
		{ID: "r7", Cases: []string{"A-570-900"}, OrderCountry: "China"},   // no hts: skipped
		{ID: "r8", Cases: []string{"A-570-900"}, OrderCountry: "Germany"}, // outside, wrong country: TN
	}
	labels := map[string]label{
		"r1": {ID: "r1", Outcome: "within", HTS: "8202.39.0000"},
		"r2": {ID: "r2", Outcome: "outside", HTS: "8202.39.0040"},
		"r3": {ID: "r3", Outcome: "within", HTS: "9999.00.0000"},
		"r4": {ID: "r4", Outcome: "outside", HTS: "9999.00.0000"},
		"r5": {ID: "r5", Outcome: "within", HTS: "7326.90.8688"},
		"r6": {ID: "r6", Outcome: "mixed", HTS: "8202.39.0000"},
		"r7": {ID: "r7", Outcome: "within", HTS: ""},
		"r8": {ID: "r8", Outcome: "outside", HTS: "8202.39.0000"},
	}
	rows, s := score(tb, rulings, labels)
	if s.Scored != 6 || s.Within != 3 || s.Outside != 3 || s.Skipped["outcome_mixed"] != 1 || s.Skipped["no_hts"] != 1 {
		t.Fatalf("summary = %+v", s)
	}
	if s.Own.TP != 1 || s.Own.FP != 1 || s.Own.FN != 2 || s.Own.TN != 2 {
		t.Errorf("own = %+v", s.Own)
	}
	// r5 is flagged for another order: any-order counts it, own-order does not.
	if s.Any.TP != 2 || s.Any.FN != 1 {
		t.Errorf("any = %+v", s.Any)
	}
	if s.OrderNotInSnap != 1 {
		t.Errorf("within rulings on orders not in the snapshot = %d, want 1", s.OrderNotInSnap)
	}
	if s.FlagEveryRuling.Recall.P != 1 || math.Abs(s.FlagEveryRuling.Precision.P-0.5) > 1e-9 {
		t.Errorf("flag-every baseline = %+v", s.FlagEveryRuling)
	}
	if len(rows) != 6 {
		t.Errorf("rows = %d", len(rows))
	}
}
