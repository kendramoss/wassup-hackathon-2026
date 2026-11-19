package planner

import (
	"fmt"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

func materialBase() (Economics, domain.Product) {
	e := Economics{AnnualImportValueUSD: 1000000, Illustrative: true,
		DutyRates: map[string]float64{"China": 0.417, "Thailand": 0.167}}
	p := domain.Product{Name: "widget",
		Plants: []domain.Plant{
			{ID: "cn", Name: "Shenzhen", Country: "China", Capabilities: []string{"assembly"}},
			{ID: "th", Name: "Rayong", Country: "Thailand", Capabilities: []string{"assembly"}},
		},
		Components: []domain.Component{
			{Name: "housing", MadeIn: "China", Plant: "cn", Group: "assembly"},
			{Name: "sensor", MadeIn: "China"},
		},
		Operations: []domain.Operation{{Name: "final assembly", Plant: "cn", Group: "assembly"}},
	}
	return e, p
}

func cloneProduct(p domain.Product) domain.Product {
	p.Plants = append([]domain.Plant(nil), p.Plants...)
	for i := range p.Plants {
		p.Plants[i].Capabilities = append([]string(nil), p.Plants[i].Capabilities...)
	}
	p.Components = append([]domain.Component(nil), p.Components...)
	p.Operations = append([]domain.Operation(nil), p.Operations...)
	return p
}

// assertPricedAtScenarioOrigin: the scenario is priced at Thailand's rate,
// saving 1,000,000 x (0.417-0.167) = 250,000, with no group move.
func assertPricedAtScenarioOrigin(t *testing.T, c Comparison) {
	t.Helper()
	if c.DutyRate == nil || *c.DutyRate != 0.167 || c.AnnualDutyUSD == nil || *c.AnnualDutyUSD != 167000 ||
		c.DutySavingsUSD == nil || *c.DutySavingsUSD != 250000 || len(c.Moved) != 0 || !c.OriginDiffersFromCurrent {
		t.Errorf("want Thailand pricing with 250000 savings, got rate=%s duty=%s savings=%s moved=%v", pv(c.DutyRate), pv(c.AnnualDutyUSD), pv(c.DutySavingsUSD), c.Moved)
	}
}

func assertSuppressed(t *testing.T, c Comparison) {
	t.Helper()
	if c.DutyRate == nil || *c.DutyRate != 0.417 || c.DutySavingsUSD == nil || *c.DutySavingsUSD != 0 ||
		*c.AnnualDutyUSD != 417000 || !c.OriginDiffersFromCurrent {
		t.Errorf("want suppressed current-origin pricing, got rate=%s duty=%s savings=%s", pv(c.DutyRate), pv(c.AnnualDutyUSD), pv(c.DutySavingsUSD))
	}
}

func TestComparePurchasedComponentOriginChangeIsMaterial(t *testing.T) {
	e, cur := materialBase()
	v := cloneProduct(cur)
	v.Components[1].MadeIn = "Thailand" // purchased, no plant
	assertPricedAtScenarioOrigin(t, Compare(e, &cur, &v, "China", "Thailand"))
	// spelling variants of the same country are not a change
	v2 := cloneProduct(cur)
	v2.Components[1].MadeIn = " china "
	assertSuppressed(t, Compare(e, &cur, &v2, "China", "Thailand"))
}

func TestComparePlantCountryChangeIsMaterial(t *testing.T) {
	e, cur := materialBase()
	v := cloneProduct(cur)
	v.Plants[0].Country = "Thailand"
	assertPricedAtScenarioOrigin(t, Compare(e, &cur, &v, "China", "Thailand"))
}

func TestCompareOperationsChangeIsMaterial(t *testing.T) {
	e, cur := materialBase()
	v := cloneProduct(cur)
	v.Operations = append(v.Operations, domain.Operation{Name: "final test", Plant: "th"})
	assertPricedAtScenarioOrigin(t, Compare(e, &cur, &v, "China", "Thailand"))
	v2 := cloneProduct(cur)
	v2.Operations[0].Name = "different process"
	assertPricedAtScenarioOrigin(t, Compare(e, &cur, &v2, "China", "Thailand"))
	v3 := cloneProduct(cur)
	v3.Plants[0].Capabilities = []string{"assembly", "casting"}
	assertPricedAtScenarioOrigin(t, Compare(e, &cur, &v3, "China", "Thailand"))
}

func TestCompareEquivalentSetupsStaySuppressed(t *testing.T) {
	e, cur := materialBase()
	// identical
	assertSuppressed(t, Compare(e, &cur, &cur, "China", "Thailand"))
	// reordered lists, renamed plant/product labels, reordered capabilities
	v := cloneProduct(cur)
	v.Name, v.Description = "widget (scenario copy)", "same thing"
	v.Components[0], v.Components[1] = v.Components[1], v.Components[0]
	v.Plants[0], v.Plants[1] = v.Plants[1], v.Plants[0]
	v.Plants[0].Name = "Renamed site"
	assertSuppressed(t, Compare(e, &cur, &v, "China", "Thailand"))
	// no-move baseline against nil variant is unchanged behaviour (no suppression info)
	// and a product with no plants/components touched stays suppressed
	cap2 := cloneProduct(cur)
	cap2.Plants[0].Capabilities = []string{"assembly"}
	assertSuppressed(t, Compare(e, &cur, &cap2, "China", "Thailand"))
}

func TestCompareMaterialChangeKeepsCostsAndMovesHonest(t *testing.T) {
	e, cur := materialBase()
	e.GroupCosts = map[string]map[string]float64{"assembly": {"cn": 100, "th": 150}}
	v := cloneProduct(cur)
	v.Components[1].MadeIn = "Thailand"
	c := Compare(e, &cur, &v, "China", "Thailand")
	if c.AddedCostUSD == nil || *c.AddedCostUSD != 0 || c.NetUSD == nil || *c.NetUSD != 250000 || len(c.Moved) != 0 {
		t.Errorf("sourcing change: no moves, no added cost: added=%s net=%s moved=%v", pv(c.AddedCostUSD), pv(c.NetUSD), c.Moved)
	}
	// real relocation still prices the move
	m := cloneProduct(cur)
	m.Operations[0].Plant = "th"
	m.Components[0].Plant = "th"
	mc := Compare(e, &cur, &m, "China", "Thailand")
	if mc.Moved.Key() != "assembly@th" || *mc.AddedCostUSD != 50 || *mc.DutySavingsUSD != 250000 {
		t.Errorf("relocation = %+v", mc)
	}
	// current inputs are not mutated
	if cur.Components[1].MadeIn != "China" {
		t.Error("current product mutated")
	}
}

func pv(f *float64) string {
	if f == nil {
		return "nil"
	}
	return fmt.Sprint(*f)
}
