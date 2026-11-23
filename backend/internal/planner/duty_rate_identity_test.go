package planner

import "testing"

func TestDutyRateMatchesAliasAndCanonical(t *testing.T) {
	for _, p := range [][2]string{{"UAE", "United Arab Emirates"}, {"Chinese Taipei", "Taiwan"}, {"Korea, South", "South Korea"}, {"DPRK", "North Korea"}} {
		for _, key := range p {
			e := Economics{DutyRates: map[string]float64{key: 0.2}}
			for _, ask := range p {
				if r, ok := e.DutyRate(ask); !ok || r != 0.2 {
					t.Errorf("rate under %q, asked %q: %v %v", key, ask, r, ok)
				}
			}
		}
	}
	if _, ok := (Economics{DutyRates: map[string]float64{"Taiwan": 0.1}}).DutyRate("China"); ok {
		t.Error("unrelated country matched")
	}
}

// Alias and canonical keys together resolve deterministically, to the exact
// canonical key, and an explicit zero is a rate, not a miss.
func TestDutyRateCoexistingKeysDeterministic(t *testing.T) {
	e := Economics{DutyRates: map[string]float64{"UAE": 0.5, "United Arab Emirates": 0.2, "Uae": 0.7}}
	for i := 0; i < 200; i++ {
		if r, ok := e.DutyRate("UAE"); !ok || r != 0.2 {
			t.Fatalf("canonical key must win: %v %v", r, ok)
		}
	}
	e = Economics{DutyRates: map[string]float64{"Chinese Taipei": 0.4, "Taiwan, China": 0.3}}
	for i := 0; i < 200; i++ {
		if r, _ := e.DutyRate("Taiwan"); r != 0.4 { // no canonical key: first in sorted key order
			t.Fatalf("sorted-first key must win: %v", r)
		}
	}
	z := Economics{DutyRates: map[string]float64{"UAE": 0}}
	if r, ok := z.DutyRate("United Arab Emirates"); !ok || r != 0 {
		t.Errorf("explicit zero lost: %v %v", r, ok)
	}
	if len(e.DutyRates) != 2 {
		t.Error("rates were mutated")
	}
}
