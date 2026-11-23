package planner

import (
	"fmt"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// splitGroupProduct puts group pcba at plants sp1 (cn) and sp2 (th) when split,
// or both operations at sp1 when not; flip reverses the operations list.
func splitGroupProduct(split, flip bool) domain.Product {
	a := domain.Operation{Name: "SMT line", Plant: "sp1", Group: "pcba"}
	b := domain.Operation{Name: "Reflow", Plant: "sp2", Group: "pcba"}
	if !split {
		b.Plant = "sp1"
	}
	ops := []domain.Operation{a, b}
	if flip {
		ops = []domain.Operation{b, a}
	}
	return domain.Product{Name: "x", Plants: []domain.Plant{
		{ID: "sp1", Name: "A", Country: "China"}, {ID: "sp2", Name: "B", Country: "Thailand"},
		{ID: "sp3", Name: "C", Country: "Vietnam", Capabilities: []string{"pcba"}}},
		Components: []domain.Component{{Name: "board", MadeIn: "China", Group: "pcba", Plant: "sp1"}}, Operations: ops}
}

func splitGroupEcon(costs map[string]float64) Economics {
	e := Economics{AnnualImportValueUSD: 1e6, DutyRates: map[string]float64{"China": 0.25, "Vietnam": 0.05}}
	if costs != nil {
		e.GroupCosts = map[string]map[string]float64{"pcba": costs}
	}
	return e
}

var splitGroupMove = domain.Move{{Group: "pcba", ToPlant: "sp3"}}

func TestSplitGroupEconomicsIgnoresOperationOrder(t *testing.T) {
	for _, costs := range []map[string]float64{
		{"sp1": 100, "sp2": 900, "sp3": 500},
		{"sp1": 100, "sp3": 500}, // th cost missing
	} {
		e := splitGroupEcon(costs)
		var got [2][3]any
		for i, flip := range []bool{false, true} {
			d, a, priced := economics(Request{Product: splitGroupProduct(true, flip), Economics: e}, splitGroupMove, "China", "Vietnam")
			got[i] = [3]any{d, a, priced}
			if priced {
				t.Errorf("costs %v flip=%v: split group must be unpriced", costs, flip)
			}
		}
		if got[0] != got[1] {
			t.Errorf("costs %v: economics depends on operation order: %v vs %v", costs, got[0], got[1])
		}
		if got[0][0] != 200000.0 {
			t.Errorf("duty effect changed: %v", got[0][0])
		}
	}
}

func TestSplitGroupCompareNetsUnknown(t *testing.T) {
	e := splitGroupEcon(map[string]float64{"sp1": 100, "sp2": 900, "sp3": 500})
	for _, flip := range []bool{false, true} {
		cur := splitGroupProduct(true, flip)
		v := Apply(cur, splitGroupMove)
		c := Compare(e, &cur, &v, "China", "Vietnam")
		if len(c.Moved) != 1 {
			t.Fatalf("flip=%v moved=%v", flip, c.Moved)
		}
		if c.AddedCostUSD != nil || c.NetUSD != nil {
			t.Errorf("flip=%v: split group must have nil added/net, got %v %v", flip, c.AddedCostUSD, c.NetUSD)
		}
		if c.DutySavingsUSD == nil || *c.DutySavingsUSD != 200000 {
			t.Errorf("flip=%v: duty savings must stay known", flip)
		}
	}
}

func TestSplitGroupApplyMoneyUnpriced(t *testing.T) {
	e := splitGroupEcon(map[string]float64{"sp1": 100, "sp2": 900, "sp3": 500})
	var nets [2]float64
	for i, flip := range []bool{false, true} {
		pl := domain.Plan{Prediction: domain.Prediction{Origin: "Vietnam"}, Shown: true}
		applyMoney(Request{Product: splitGroupProduct(true, flip), Economics: e}, Template{}, &pl, splitGroupMove, "China")
		if pl.NetPriced == nil || *pl.NetPriced {
			t.Errorf("flip=%v: net_priced must be false", flip)
		}
		nets[i] = pl.NetYear
	}
	if nets[0] != nets[1] {
		t.Errorf("net depends on order: %v", nets)
	}
}

func TestSingleGroupMoneyUnchangedControls(t *testing.T) {
	for _, flip := range []bool{false, true} {
		cur := splitGroupProduct(false, flip)
		// single-plant, costs known
		e := splitGroupEcon(map[string]float64{"sp1": 100, "sp2": 900, "sp3": 500})
		d, a, priced := economics(Request{Product: cur, Economics: e}, splitGroupMove, "China", "Vietnam")
		if d != 200000 || a != 400 || !priced {
			t.Errorf("single plant: %v %v %v", d, a, priced)
		}
		v := Apply(cur, splitGroupMove)
		c := Compare(e, &cur, &v, "China", "Vietnam")
		if c.AddedCostUSD == nil || *c.AddedCostUSD != 400 || c.NetUSD == nil || *c.NetUSD != 199600 {
			t.Errorf("single plant compare: %v %v", c.AddedCostUSD, c.NetUSD)
		}
		// explicit zero cost at the current plant is a known cost
		ez := splitGroupEcon(map[string]float64{"sp1": 0, "sp3": 500})
		_, a, priced = economics(Request{Product: cur, Economics: ez}, splitGroupMove, "China", "Vietnam")
		if a != 500 || !priced {
			t.Errorf("explicit zero: %v %v", a, priced)
		}
	}
}

// With no group costs at all (supported duty-only mode) a split group keeps
// pricing on duty alone, in both orders.
func TestSplitGroupDutyOnlyModeUnchanged(t *testing.T) {
	e := splitGroupEcon(nil)
	for _, flip := range []bool{false, true} {
		cur := splitGroupProduct(true, flip)
		d, a, priced := economics(Request{Product: cur, Economics: e}, splitGroupMove, "China", "Vietnam")
		if d != 200000 || a != 0 || !priced {
			t.Errorf("flip=%v duty-only: %v %v %v", flip, d, a, priced)
		}
		v := Apply(cur, splitGroupMove)
		c := Compare(e, &cur, &v, "China", "Vietnam")
		if c.AddedCostUSD != nil {
			t.Errorf("duty-only compare added must stay nil: %v", *c.AddedCostUSD)
		}
	}
}

// An operation at one plant with a component at another is not a split group:
// operations take precedence (groupPlant's rule), so the money is unchanged and
// does not depend on list order.
func TestOperationAndComponentAtDifferentPlantsStillPriced(t *testing.T) {
	e := splitGroupEcon(map[string]float64{"sp1": 100, "sp2": 900, "sp3": 500})
	for _, flip := range []bool{false, true} {
		cur := splitGroupProduct(false, flip)
		cur.Components = append(cur.Components, domain.Component{Name: "alt", MadeIn: "Thailand", Group: "pcba", Plant: "sp2"})
		d, a, priced := economics(Request{Product: cur, Economics: e}, splitGroupMove, "China", "Vietnam")
		if d != 200000 || a != 400 || !priced {
			t.Errorf("flip=%v: %v %v %v", flip, d, a, priced)
		}
	}
}

// With no operations, components at two plants are a split group too.
func TestComponentOnlySplitGroupUnpriced(t *testing.T) {
	e := splitGroupEcon(map[string]float64{"sp1": 100, "sp2": 900, "sp3": 500})
	comps := []domain.Component{{Name: "a", Group: "pcba", Plant: "sp1"}, {Name: "b", Group: "pcba", Plant: "sp2"}}
	var got [2]float64
	for i, flip := range []bool{false, true} {
		cur := splitGroupProduct(false, false)
		cur.Operations = nil
		cur.Components = []domain.Component{comps[0], comps[1]}
		if flip {
			cur.Components = []domain.Component{comps[1], comps[0]}
		}
		_, a, priced := economics(Request{Product: cur, Economics: e}, splitGroupMove, "China", "Vietnam")
		if priced {
			t.Errorf("flip=%v: component-split group must be unpriced", flip)
		}
		got[i] = a
	}
	if got[0] != got[1] {
		t.Errorf("order dependent: %v", got)
	}
}

// withPcbaOps returns p with the pcba group's operations at the given plants,
// in the given order.
func withPcbaOps(p domain.Product, plants ...string) domain.Product {
	p.Operations = nil
	for i, pl := range plants {
		p.Operations = append(p.Operations, domain.Operation{Name: []string{"SMT line", "Reflow"}[i%2], Plant: pl, Group: "pcba"})
	}
	return p
}

// A variant that consolidates an already split group changes the group's plant
// set. The legacy money must be unknown for every current operation order and
// every destination, including a destination equal to the plant movesBetween
// happens to pick first (#716 review finding 1).
func TestConsolidatingSplitGroupMoneyUnknownInBothOrders(t *testing.T) {
	e := splitGroupEcon(map[string]float64{"sp1": 100, "sp2": 900, "sp3": 500})
	for _, dest := range []string{"sp1", "sp2", "sp3"} {
		for _, flip := range []bool{false, true} {
			cur := splitGroupProduct(true, flip)
			v := withPcbaOps(cur, dest, dest)
			c := Compare(e, &cur, &v, "China", "Vietnam")
			if c.AddedCostUSD != nil || c.NetUSD != nil {
				t.Errorf("dest=%s flip=%v: money must be unknown, got added=%v net=%v moved=%v", dest, flip, c.AddedCostUSD, c.NetUSD, c.Moved)
			}
			if c.DutySavingsUSD == nil || *c.DutySavingsUSD != 200000 {
				t.Errorf("dest=%s flip=%v: duty savings must stay known", dest, flip)
			}
		}
	}
}

// Same legal prediction, only the variant's operation order differs: the money
// must not depend on either list order.
func TestConsolidatingSplitGroupVariantOrderIndependent(t *testing.T) {
	e := splitGroupEcon(map[string]float64{"sp1": 100, "sp2": 900, "sp3": 500})
	cur := splitGroupProduct(true, false)
	var got []string
	for _, v := range []domain.Product{withPcbaOps(cur, "sp1", "sp1"), withPcbaOps(cur, "sp1", "sp1")} {
		c := Compare(e, &cur, &v, "China", "Vietnam")
		got = append(got, fmt.Sprint(c.AddedCostUSD == nil, c.NetUSD == nil))
	}
	if got[0] != got[1] {
		t.Errorf("order dependent: %v", got)
	}
}

// An unchanged or merely reordered split setup is not a plant-set change and
// keeps its old (zero added cost) money; a split variant of a single-plant
// current group is outside this guard (legacy baseline is single-plant).
func TestSplitGroupUnchangedAndReorderedKeepMoney(t *testing.T) {
	e := splitGroupEcon(map[string]float64{"sp1": 100, "sp2": 900, "sp3": 500})
	cur := splitGroupProduct(true, false)
	for _, v := range []domain.Product{cur, splitGroupProduct(true, true)} {
		c := Compare(e, &cur, &v, "China", "China")
		if c.AddedCostUSD == nil || *c.AddedCostUSD != 0 || c.NetUSD == nil || *c.NetUSD != 0 {
			t.Errorf("unchanged split must stay zero/known: %v %v", c.AddedCostUSD, c.NetUSD)
		}
	}
}

// Single-plant and explicit-zero controls through Compare keep their money when
// the variant is a plain single-plant move, and the duty-only mode stays nil.
func TestConsolidationGuardControls(t *testing.T) {
	for _, flip := range []bool{false, true} {
		cur := splitGroupProduct(false, flip)
		v := withPcbaOps(cur, "sp3", "sp3")
		e := splitGroupEcon(map[string]float64{"sp1": 0, "sp3": 500})
		c := Compare(e, &cur, &v, "China", "Vietnam")
		if c.AddedCostUSD == nil || *c.AddedCostUSD != 500 {
			t.Errorf("explicit zero control: %v", c.AddedCostUSD)
		}
		ed := splitGroupEcon(nil)
		sp := splitGroupProduct(true, flip)
		vd := withPcbaOps(sp, "sp1", "sp1")
		if cd := Compare(ed, &sp, &vd, "China", "Vietnam"); cd.AddedCostUSD != nil || cd.NetUSD != nil {
			t.Errorf("duty-only mode has no group-cost money: %v %v", cd.AddedCostUSD, cd.NetUSD)
		}
	}
}
