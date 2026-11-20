package planner

import "testing"

// #366 (was #358): a moved group's added cost needs a cost entered at both its
// current plant and its destination. A missing entry is unknown, not zero, so
// the added cost and net are null; an explicit 0 is a real cost.
func movedAssembly() (Economics, Comparison, func(map[string]map[string]float64) Comparison) {
	e, cur := materialBase()
	v := cloneProduct(cur)
	v.Components[0].Plant = "th"
	v.Operations[0].Plant = "th"
	compare := func(costs map[string]map[string]float64) Comparison {
		e.GroupCosts = costs
		return Compare(e, &cur, &v, "China", "Thailand")
	}
	return e, compare(nil), compare
}

func TestCompareAddedCostNeedsBothPlantCosts(t *testing.T) {
	_, _, compare := movedAssembly()
	// duty saving is 1,000,000 x (0.417 - 0.167) = 250,000
	for _, tc := range []struct {
		name  string
		costs map[string]map[string]float64
		added *float64
		net   *float64
	}{
		{"both known", map[string]map[string]float64{"assembly": {"cn": 1000, "th": 51000}}, ptr(50000), ptr(200000)},
		{"explicit zero cost at both", map[string]map[string]float64{"assembly": {"cn": 0, "th": 0}}, ptr(0), ptr(250000)},
		{"explicit zero at the destination", map[string]map[string]float64{"assembly": {"cn": 1000, "th": 0}}, ptr(-1000), ptr(251000)},
		{"missing source", map[string]map[string]float64{"assembly": {"th": 50000}}, nil, nil},
		{"missing destination", map[string]map[string]float64{"assembly": {"cn": 1000}}, nil, nil},
		{"costs only for an unrelated group", map[string]map[string]float64{"other": {"cn": 1, "th": 2}}, nil, nil},
		{"empty cost row for the group", map[string]map[string]float64{"assembly": {}}, nil, nil},
	} {
		c := compare(tc.costs)
		if pv(c.AddedCostUSD) != pv(tc.added) || pv(c.NetUSD) != pv(tc.net) {
			t.Errorf("%s: added=%s net=%s, want added=%s net=%s", tc.name, pv(c.AddedCostUSD), pv(c.NetUSD), pv(tc.added), pv(tc.net))
		}
		if c.DutySavingsUSD == nil || *c.DutySavingsUSD != 250000 {
			t.Errorf("%s: unknown cost must not hide the known duty saving: %s", tc.name, pv(c.DutySavingsUSD))
		}
	}
}

func TestCompareWithoutMovesOrCostsKeepsSupportedModes(t *testing.T) {
	e, cur := materialBase()
	e.GroupCosts = map[string]map[string]float64{"other": {"cn": 5}}
	// Nothing moves: nothing is added, even though no cost is entered for the group.
	same := Compare(e, &cur, &cur, "China", "China")
	if same.AddedCostUSD == nil || *same.AddedCostUSD != 0 || same.NetUSD == nil {
		t.Errorf("no moves: added=%s net=%s", pv(same.AddedCostUSD), pv(same.NetUSD))
	}
	// No costs at all stays the supported no-cost mode: added and net are null.
	e.GroupCosts = nil
	v := cloneProduct(cur)
	v.Components[0].Plant, v.Operations[0].Plant = "th", "th"
	if c := Compare(e, &cur, &v, "China", "Thailand"); c.AddedCostUSD != nil || c.NetUSD != nil {
		t.Errorf("no cost mode: added=%s net=%s", pv(c.AddedCostUSD), pv(c.NetUSD))
	}
}
