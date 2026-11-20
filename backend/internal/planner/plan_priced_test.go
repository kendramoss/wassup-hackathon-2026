package planner

import (
	"context"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
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
		shown = append(shown, priced(t, p))
	}
	for _, p := range res.Rejected {
		if p.Move.Key() != "" { // moves searched; the "keep the current setup" row is not a move
			rejected = append(rejected, priced(t, p))
		}
	}
	return shown, rejected, res
}

// priced reads the flag; a searched plan must carry it explicitly (true or
// false), never leave it absent, which is reserved for plans stored earlier.
func priced(t *testing.T, p domain.Plan) bool {
	t.Helper()
	if p.NetPriced == nil {
		t.Errorf("searched plan %q has no net_priced flag", p.Move.Key())
		return false
	}
	return *p.NetPriced
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
	t.Run("duty known but a moved group has a cost at only one plant: not priced", func(t *testing.T) {
		shown, rejected, res := pricedFlags(t, func(e *Economics) {
			for g, byPlant := range e.GroupCosts {
				kept := map[string]float64{}
				for pl, c := range byPlant {
					if pl != "th" { // the source plant's cost stays, the destination's is blank
						kept[pl] = c
					}
				}
				e.GroupCosts[g] = kept
			}
		})
		if !allEqual(shown, false) || !allEqual(rejected, false) {
			t.Errorf("shown=%v rejected=%v, want none priced", shown, rejected)
		}
		// The arithmetic and ranking are unchanged: blanks still read as 0 there.
		if res.Plans[0].AddedCostYear >= 0 {
			t.Errorf("planner arithmetic changed: added cost %v", res.Plans[0].AddedCostYear)
		}
	})
	t.Run("costs entered only for an unrelated group: not priced", func(t *testing.T) {
		shown, _, _ := pricedFlags(t, func(e *Economics) { e.GroupCosts = map[string]map[string]float64{"other": {"cn": 1, "th": 2}} })
		if !allEqual(shown, false) {
			t.Errorf("shown=%v, want not priced", shown)
		}
	})
	t.Run("no group costs at all (intentional no-cost mode): priced on duty alone", func(t *testing.T) {
		shown, rejected, res := pricedFlags(t, func(e *Economics) { e.GroupCosts = nil })
		if !allEqual(shown, true) || !allEqual(rejected, true) || res.Plans[0].AddedCostYear != 0 || res.Plans[0].NetYear != 500000 {
			t.Errorf("shown=%v rejected=%v added=%v net=%v", shown, rejected, res.Plans[0].AddedCostYear, res.Plans[0].NetYear)
		}
	})
	t.Run("complete costs that net to a real zero: priced zero", func(t *testing.T) {
		shown, _, res := pricedFlags(t, func(e *Economics) {
			e.DutyRates = map[string]float64{"China": 0.2, "Thailand": 0.2}
			for g, byPlant := range e.GroupCosts {
				byPlant["th"] = byPlant["cn"]
				e.GroupCosts[g] = byPlant
			}
		})
		p := res.Plans[0]
		if !allEqual(shown, true) || p.DutyEffectYear != 0 || p.AddedCostYear != 0 || p.NetYear != 0 {
			t.Errorf("shown=%v duty=%v added=%v net=%v, want a fully known zero", shown, p.DutyEffectYear, p.AddedCostYear, p.NetYear)
		}
	})
}
