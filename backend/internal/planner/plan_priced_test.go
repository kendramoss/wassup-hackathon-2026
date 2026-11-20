package planner

import (
	"context"
	"testing"
)

// #368: a plan records at search time whether its money could be worked out.
// Run through the real planner with the fixture, varying only the economics.
func pricedFlags(t *testing.T, mutate func(*Economics)) (shown, rejected []bool, res Result) {
	t.Helper()
	r, _ := fixture(t, "heatgun_2022.json")
	mutate(&r.Economics)
	tmpls, _ := LoadTemplates()
	res, err := New(tmpls, &fakeEngine{}, &fakeVerifier{supported: true, confidence: 0.97}, Options{PlanConfidence: 0.95, Concurrency: 3}).Plan(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range res.Plans {
		shown = append(shown, p.NetPriced)
	}
	for _, p := range res.Rejected {
		if p.Move.Key() != "" { // moves searched; the "keep the current setup" row is not a move
			rejected = append(rejected, p.NetPriced)
		}
	}
	return shown, rejected, res
}

func allEqual(bs []bool, want bool) bool {
	for _, b := range bs {
		if b != want {
			return false
		}
	}
	return len(bs) > 0
}

func TestPlanRecordsWhetherItWasPriced(t *testing.T) {
	t.Run("fixture economics: priced", func(t *testing.T) {
		shown, rejected, _ := pricedFlags(t, func(*Economics) {})
		if !allEqual(shown, true) || !allEqual(rejected, true) {
			t.Errorf("shown=%v rejected=%v, want all priced", shown, rejected)
		}
	})
	t.Run("no money at all: not priced", func(t *testing.T) {
		shown, rejected, res := pricedFlags(t, func(e *Economics) { *e = Economics{} })
		if !allEqual(shown, false) || !allEqual(rejected, false) {
			t.Errorf("shown=%v rejected=%v, want none priced", shown, rejected)
		}
		for _, p := range res.Plans {
			if p.NetYear != 0 {
				t.Errorf("an unpriced plan carries net %v", p.NetYear)
			}
		}
	})
	t.Run("import value but no duty rate for the origins: not priced", func(t *testing.T) {
		shown, rejected, _ := pricedFlags(t, func(e *Economics) { e.DutyRates = nil })
		if !allEqual(shown, false) || !allEqual(rejected, false) {
			t.Errorf("shown=%v rejected=%v, want none priced", shown, rejected)
		}
	})
	t.Run("only the destination rate known: not priced", func(t *testing.T) {
		shown, _, _ := pricedFlags(t, func(e *Economics) { e.DutyRates = map[string]float64{"Thailand": 0.167} })
		if !allEqual(shown, false) {
			t.Errorf("shown=%v, want not priced", shown)
		}
	})
	t.Run("equal rates: a real zero duty effect is priced", func(t *testing.T) {
		shown, _, res := pricedFlags(t, func(e *Economics) {
			e.DutyRates = map[string]float64{"China": 0.2, "Thailand": 0.2}
			e.GroupCosts = nil // the supported no-cost mode keeps pricing the duty side
		})
		if !allEqual(shown, true) || res.Plans[0].DutyEffectYear != 0 || res.Plans[0].NetYear != 0 {
			t.Errorf("shown=%v duty=%v net=%v, want a priced zero", shown, res.Plans[0].DutyEffectYear, res.Plans[0].NetYear)
		}
	})
}
