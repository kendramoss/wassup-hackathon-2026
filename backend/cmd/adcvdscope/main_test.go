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
}

func scoreTable() *adcvd.Table {
	return adcvd.NewTable("t", "2016-12-31", []adcvd.Order{
		{Case: "A-570-082", Type: "AD", Country: "The People's Republic of China", Product: "Steel Wheels", Status: "active", ScopeText: "Steel wheels for trailers, 12 to 16.5 inches."},
		{Case: "A-570-106", Type: "AD", Country: "The People's Republic of China", Product: "Wooden Cabinets", Status: "active", ScopeText: "Wooden cabinets and vanities for kitchens."},
	})
}

func TestScorePoliciesFollowTheReader(t *testing.T) {
	tb := scoreTable()
	rulings := []ruling{
		{ID: "r1", Cases: []string{"A-570-082"}, OrderCountry: "China"}, // within, reader in: TP everywhere
		{ID: "r2", Cases: []string{"A-570-082"}, OrderCountry: "China"}, // outside, reader unclear: FP for in+unclear, TN for in
		{ID: "r3", Cases: []string{"A-570-106"}, OrderCountry: "China"}, // within, reader out: FN
		{ID: "r4", Cases: []string{"A-570-999"}, OrderCountry: "China"}, // within, order not in the list: FN, a gap
		{ID: "r5", Cases: []string{"A-570-082"}, OrderCountry: "China"}, // mixed: skipped
	}
	labels := map[string]label{"r1": {ID: "r1", Outcome: "within"}, "r2": {ID: "r2", Outcome: "outside"}, "r3": {ID: "r3", Outcome: "within"}, "r4": {ID: "r4", Outcome: "within"}, "r5": {ID: "r5", Outcome: "mixed"}}
	descs := map[string]desc{
		"r1": {ID: "r1", Description: "steel trailer wheel"}, "r2": {ID: "r2", Description: "steel wheel for trailers"},
		"r3": {ID: "r3", Description: "wooden kitchen vanity cabinet"}, "r4": {ID: "r4", Description: "wooden cabinet"}, "r5": {ID: "r5", Description: "steel wheel"},
	}
	fr := &fileReader{verdicts: map[string]string{}, missing: map[string]bool{}}
	key := func(product, scope, d string) string {
		return adcvd.PairKey(adcvd.Order{Product: product, Country: "The People's Republic of China", ScopeText: scope}, d)
	}
	wheel, cab := "Steel wheels for trailers, 12 to 16.5 inches.", "Wooden cabinets and vanities for kitchens."
	fr.verdicts[key("Steel Wheels", wheel, "steel trailer wheel")] = "in"
	fr.verdicts[key("Steel Wheels", wheel, "steel wheel for trailers")] = "unclear"
	fr.verdicts[key("Wooden Cabinets", cab, "wooden kitchen vanity cabinet")] = "out"
	fr.verdicts[key("Wooden Cabinets", cab, "wooden cabinet")] = "out"

	_, s, pairs := score(tb, rulings, labels, descs, 5, false, fr)
	if s.Scored != 4 || s.Within != 3 || s.Outside != 1 || s.Skipped["outcome_mixed"] != 1 || s.OrderNotInSnap != 1 {
		t.Fatalf("counts: %+v", s)
	}
	if len(pairs) == 0 || s.ReaderMissing != 0 && len(fr.missing) != 0 {
		t.Errorf("pairs %d, missing %d", len(pairs), len(fr.missing))
	}
	in, iu := s.Policies["in"].Own, s.Policies["in+unclear"].Own
	if in.TP != 1 || in.FP != 0 || in.FN != 2 || in.TN != 1 {
		t.Errorf("in: %+v", in)
	}
	if iu.TP != 1 || iu.FP != 1 || iu.FN != 2 || iu.TN != 0 {
		t.Errorf("in+unclear: %+v", iu)
	}
	if got := s.OwnInTopK; got.K != 2 || got.N != 3 {
		t.Errorf("own retrieved %+v, want 2 of 3 (the order outside the list is never retrieved)", got)
	}
}

func TestUnansweredCandidatesAreNeverFlaggedByAReaderPolicy(t *testing.T) {
	tb := scoreTable()
	fr := &fileReader{verdicts: map[string]string{}, missing: map[string]bool{}}
	rows, s, _ := score(tb, []ruling{{ID: "r1", Cases: []string{"A-570-082"}, OrderCountry: "China"}}, map[string]label{"r1": {ID: "r1", Outcome: "within"}},
		map[string]desc{"r1": {ID: "r1", Description: "steel trailer wheel"}}, 5, false, fr)
	if rows[0].Flags["in+unclear"].Own || !rows[0].Flags["retrieve_only"].Own {
		t.Errorf("flags %+v", rows[0].Flags)
	}
	if len(fr.missing) == 0 {
		t.Error("the unanswered question was not recorded")
	}
	_ = s
}
