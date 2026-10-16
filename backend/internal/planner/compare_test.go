package planner

import (
	"testing"
)

func TestCompareScenarioAgainstCurrentSetup(t *testing.T) {
	r, _ := fixture(t, "heatgun_2022.json")
	variant := r.Product
	variant.Components = append(variant.Components[:0:0], r.Product.Components...)
	for i, c := range variant.Components {
		if c.Group == "motor_fan_assembly" {
			variant.Components[i].Plant, variant.Components[i].MadeIn = "th", "Thailand"
		}
	}
	c := Compare(r.Economics, &r.Product, &variant, "China", "Thailand")
	// $2M × (41.7% − 16.7%) = $500k saved; motor-fan assembly th − cn = +$120k.
	if c.DutySavingsUSD == nil || *c.DutySavingsUSD != 500000 || c.AddedCostUSD == nil || *c.AddedCostUSD != 120000 ||
		c.NetUSD == nil || *c.NetUSD != 380000 || *c.AnnualDutyUSD != 334000 || c.Moved.Key() != "motor_fan_assembly@th" || !c.Illustrative {
		t.Errorf("comparison = %+v", c)
	}
	same := Compare(r.Economics, &r.Product, &r.Product, "China", "China")
	if *same.DutySavingsUSD != 0 || *same.AddedCostUSD != 0 || len(same.Moved) != 0 || same.OriginDiffersFromCurrent {
		t.Errorf("current setup vs itself = %+v", same)
	}
	// Heat gun "as today": CBP held China, the engine predicts Thailand for the
	// unchanged setup. Moving nothing cannot save duty.
	asToday := Compare(r.Economics, &r.Product, &r.Product, "China", "Thailand")
	if *asToday.DutySavingsUSD != 0 || *asToday.NetUSD != 0 || *asToday.AnnualDutyUSD != 834000 || *asToday.DutyRate != 0.417 || !asToday.OriginDiffersFromCurrent {
		t.Errorf("as today = %+v", asToday)
	}
	if !c.OriginDiffersFromCurrent {
		t.Error("moved setup with a new origin should report the difference")
	}
	unknown := Compare(r.Economics, &r.Product, nil, "China", "Vietnam")
	if unknown.DutyRate != nil || unknown.DutySavingsUSD != nil || unknown.AddedCostUSD != nil || unknown.NetUSD != nil {
		t.Errorf("no rate for Vietnam, no product: %+v", unknown)
	}
}
