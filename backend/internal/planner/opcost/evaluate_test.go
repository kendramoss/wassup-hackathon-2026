package opcost

import (
	"encoding/json"
	"math"
	"reflect"
	"regexp"
	"testing"
)

func f(x float64) *float64 { return &x }
func n(x int) *int         { return &x }

var (
	rp1 = Ref{"Dongguan factory", "china"}
	rp2 = Ref{"Chonburi works", "thailand"}
	rp3 = Ref{"Gdansk works", "poland"}
)

// base is a synthetic one-move input: fa moves p1 -> p2, import value 1,000,000,
// rates 25% -> 5% (duty effect 200,000). All entries empty.
func base() Input {
	return Input{
		AnnualImportValueUSD: 1_000_000, FromRate: f(0.25), ToRate: f(0.05),
		Plants: map[string]Ref{"p1": rp1, "p2": rp2, "p3": rp3},
		Moves:  []Move{{"fa", "p1", "p2"}},
		Ops: &Operations{Groups: map[string]GroupOps{"fa": {Plants: map[string]PlantOps{
			"p1": {Ref: &rp1}, "p2": {Ref: &rp2}}}}},
	}
}

func (i *Input) plant(g, p string, fn func(*PlantOps)) {
	gr := i.Ops.Groups[g]
	pl := gr.Plants[p]
	fn(&pl)
	gr.Plants[p] = pl
	i.Ops.Groups[g] = gr
}

func (i *Input) group(g string, fn func(*GroupOps)) {
	gr := i.Ops.Groups[g]
	fn(&gr)
	i.Ops.Groups[g] = gr
}

func val(t *testing.T, name string, got *float64, want float64) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s = nil, want %v", name, want)
	}
	if *got != want {
		t.Fatalf("%s = %v, want %v", name, *got, want)
	}
}

func isNil(t *testing.T, name string, got *float64) {
	t.Helper()
	if got != nil {
		t.Fatalf("%s = %v, want nil (unknown)", name, *got)
	}
}

func hasUnknown(r Result, cat, factor, reason string) bool {
	for _, u := range r.Unknown {
		if u.Category == cat && u.Factor == factor && u.Reason == reason {
			return true
		}
	}
	return false
}

func has(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func eval(t *testing.T, in Input) Result {
	t.Helper()
	r, active := Evaluate(in)
	if !active {
		t.Fatal("expected active operations")
	}
	return r
}

// costly is the contract's worked example 13.1 (hand-computed).
func costly(h float64) Input {
	in := base()
	in.GroupCosts = map[string]map[string]float64{"fa": {"p1": 300_000, "p2": 320_000}}
	in.Ops.HorizonYears = f(h)
	in.group("fa", func(g *GroupOps) { g.CostBasis = BasisAdd })
	in.plant("fa", "p1", func(p *PlantOps) { p.Labour, p.Logistics = f(100_000), f(10_000) })
	in.plant("fa", "p2", func(p *PlantOps) {
		p.Labour, p.Logistics, p.Capex, p.Retooling = f(60_000), f(45_000), f(900_000), f(100_000)
	})
	return in
}

func TestWorkedExampleNetCostlier(t *testing.T) {
	tests := []struct {
		h        float64
		amort    float64
		net      float64
		decision string
	}{
		{4, 250_000, -65_000, DecisionCostlier},
		{5, 200_000, -15_000, DecisionCostlier},
		{10, 100_000, 85_000, ""},
		{3, 333_333.33, -148_333.33, DecisionCostlier},
	}
	for _, tc := range tests {
		r := eval(t, costly(tc.h))
		c := r.Components
		val(t, "duty", c.DutyEffectPerYear, 200_000)
		val(t, "group", c.GroupCostDelta, 20_000)
		val(t, "labour", c.LabourDelta, -40_000)
		val(t, "logistics", c.LogisticsDelta, 35_000)
		val(t, "operating", c.OperatingDelta, 15_000)
		val(t, "one-time", c.OneTimeTotal, 1_000_000)
		val(t, "amortized", c.AmortizedOneTimePerYear, tc.amort)
		val(t, "net", r.NetModelledPerYear, tc.net)
		if !r.Priced || r.Decision != tc.decision {
			t.Fatalf("h=%v priced=%v decision=%q, want true %q", tc.h, r.Priced, r.Decision, tc.decision)
		}
		if r.NetYear != tc.net || r.AddedCostYear != 15_000 || r.DutyEffectYear != 200_000 {
			t.Fatalf("legacy fields %v %v %v", r.DutyEffectYear, r.AddedCostYear, r.NetYear)
		}
		if !has(r.Assumptions, AssumptionParallel) || !has(r.Assumptions, AssumptionStraightLine) {
			t.Fatalf("assumptions missing: %v", r.Assumptions)
		}
	}
}

func TestIncludedNeverAdds(t *testing.T) {
	in := costly(10)
	in.group("fa", func(g *GroupOps) { g.CostBasis = BasisIncluded })
	in.plant("fa", "p1", func(p *PlantOps) { p.Capex, p.Retooling = nil, nil })
	in.plant("fa", "p2", func(p *PlantOps) { p.Capex, p.Retooling = nil, nil })
	r := eval(t, in)
	val(t, "operating", r.Components.OperatingDelta, 20_000) // group costs only
	isNil(t, "labour (inside the group cost)", r.Components.LabourDelta)
	val(t, "net", r.NetModelledPerYear, 180_000)
	// a breakdown missing at one plant never nulls a known group total
	in.plant("fa", "p2", func(p *PlantOps) { p.Labour, p.Logistics = nil, nil })
	r = eval(t, in)
	val(t, "operating", r.Components.OperatingDelta, 20_000)
	if !r.Priced {
		t.Fatal("included with missing breakdown must stay priced")
	}
	// but included still needs the actual group costs at both plants
	in.GroupCosts = map[string]map[string]float64{"fa": {"p1": 300_000}}
	r = eval(t, in)
	if r.Priced || !hasUnknown(r, "financial", "group_cost", "missing_target") {
		t.Fatalf("included needs both group costs: %+v", r.Unknown)
	}
	in.GroupCosts = nil
	r = eval(t, in)
	if r.Priced || !hasUnknown(r, "financial", "group_cost", "missing_group_cost") {
		t.Fatalf("included without a table is unknown: %+v", r.Unknown)
	}
}

func TestAdditionalDeltaAndMissing(t *testing.T) {
	in := costly(4)
	in.plant("fa", "p2", func(p *PlantOps) { p.Labour = nil })
	r := eval(t, in)
	if r.Priced || r.NetModelledPerYear != nil || !hasUnknown(r, "financial", "labour", "missing_target") {
		t.Fatalf("labour at p1 only must be unknown: %+v", r)
	}
	isNil(t, "labour", r.Components.LabourDelta)
	isNil(t, "operating", r.Components.OperatingDelta)
	val(t, "group", r.Components.GroupCostDelta, 20_000) // other parts stay known
	if r.Decision != "" {
		t.Fatalf("unknown pricing must not reject: %q", r.Decision)
	}
	in = costly(4)
	in.plant("fa", "p1", func(p *PlantOps) { p.Labour = nil })
	if r = eval(t, in); !hasUnknown(r, "financial", "labour", "missing_baseline") {
		t.Fatal("missing baseline labour must be unknown")
	}
	// logistics entered nowhere: not modelled, total still known
	in = costly(4)
	in.plant("fa", "p1", func(p *PlantOps) { p.Logistics = nil })
	in.plant("fa", "p2", func(p *PlantOps) { p.Logistics = nil })
	r = eval(t, in)
	val(t, "operating", r.Components.OperatingDelta, -20_000)
	if !r.Priced || !has(r.NotModelled, "logistics:fa") {
		t.Fatalf("logistics not modelled expected: %v", r.NotModelled)
	}
	// explicit zero is a known zero
	in = costly(4)
	in.plant("fa", "p1", func(p *PlantOps) { p.Logistics = f(0) })
	in.plant("fa", "p2", func(p *PlantOps) { p.Logistics = f(0) })
	r = eval(t, in)
	val(t, "logistics", r.Components.LogisticsDelta, 0)
	if !r.Priced {
		t.Fatal("known zero must stay priced")
	}
	// basis missing while labour entered
	in = costly(4)
	in.group("fa", func(g *GroupOps) { g.CostBasis = "" })
	if r = eval(t, in); r.Priced || !hasUnknown(r, "financial", "labour", "cost_basis_missing") {
		t.Fatalf("missing basis: %+v", r.Unknown)
	}
}

func TestGroupCostRuleC(t *testing.T) {
	// table present, destination missing: unknown, legacy figure reads blank as 0
	in := base()
	in.GroupCosts = map[string]map[string]float64{"fa": {"p1": 300_000}}
	in.Ops.HorizonYears = f(5)
	r := eval(t, in)
	if r.Priced || !hasUnknown(r, "financial", "group_cost", "missing_target") {
		t.Fatalf("%+v", r.Unknown)
	}
	if r.AddedCostYear != -300_000 || r.NetYear != 500_000 {
		t.Fatalf("legacy blank-as-0 reading: added %v net %v", r.AddedCostYear, r.NetYear)
	}
	// table present but this group has no entry at all: also unknown
	in.GroupCosts = map[string]map[string]float64{"other": {"p1": 1}}
	if r = eval(t, in); r.Priced {
		t.Fatal("a table without this group is unknown")
	}
	// table absent: group cost not modelled (no known-zero claim), labour still priced
	in = base()
	in.group("fa", func(g *GroupOps) { g.CostBasis = BasisAdd })
	in.plant("fa", "p1", func(p *PlantOps) { p.Labour = f(100_000) })
	in.plant("fa", "p2", func(p *PlantOps) { p.Labour = f(60_000) })
	r = eval(t, in)
	if !r.Priced || !has(r.NotModelled, "group_cost") {
		t.Fatalf("no-table mode: %+v", r)
	}
	isNil(t, "group (not modelled)", r.Components.GroupCostDelta)
	val(t, "operating", r.Components.OperatingDelta, -40_000)
	val(t, "net", r.NetModelledPerYear, 240_000)
	// explicit zero group costs with labour-only: known
	in.GroupCosts = map[string]map[string]float64{"fa": {"p1": 0, "p2": 0}}
	r = eval(t, in)
	val(t, "net", r.NetModelledPerYear, 240_000)
	if has(r.NotModelled, "group_cost") {
		t.Fatal("explicit zero is modelled")
	}
}

func TestOnlyHorizonActiveMatchesPlainGroupCosts(t *testing.T) {
	// hand-computed: duty 200,000 - added 20,000 = 180,000, nothing else modelled
	in := base()
	in.GroupCosts = map[string]map[string]float64{"fa": {"p1": 300_000, "p2": 320_000}}
	in.Ops.HorizonYears = f(5)
	r := eval(t, in)
	val(t, "net", r.NetModelledPerYear, 180_000)
	isNil(t, "amortized (not modelled)", r.Components.AmortizedOneTimePerYear)
	if r.Assessed || r.Feasibility != "" || r.Decision != "" {
		t.Fatalf("no constraint is active: %+v", r)
	}
	for _, w := range []string{"labour:fa", "logistics:fa", "capex:fa", "retooling:fa", "lead_time_constraint", "moq_constraint:fa",
		"subsidies_incentives", "capacity_utilisation", "discounting_time_value", "taxes_residual_value", "ramp_up_yield"} {
		if !has(r.NotModelled, w) {
			t.Fatalf("not_modelled lacks %s: %v", w, r.NotModelled)
		}
	}
}

func TestDutyUnknown(t *testing.T) {
	in := costly(5)
	in.ToRate = nil
	r := eval(t, in)
	if r.Priced || !hasUnknown(r, "financial", "duty", "no_duty_rate") || r.NetModelledPerYear != nil {
		t.Fatalf("%+v", r.Unknown)
	}
	isNil(t, "duty", r.Components.DutyEffectPerYear)
	in = costly(5)
	in.AnnualImportValueUSD = 0
	if r = eval(t, in); r.Priced || !hasUnknown(r, "financial", "duty", "no_import_value") {
		t.Fatal("no import value is unknown")
	}
}

func TestSunkBaselineCapital(t *testing.T) {
	clean := base()
	clean.GroupCosts = map[string]map[string]float64{"fa": {"p1": 300_000, "p2": 320_000}}
	clean.Ops.HorizonYears = f(5)
	want := eval(t, clean)
	in := base()
	in.GroupCosts = clean.GroupCosts
	in.plant("fa", "p1", func(p *PlantOps) { p.Capex, p.Retooling = f(5_000_000), f(1_000_000) })
	in.Ops.HorizonYears = f(5)
	r := eval(t, in)
	if !r.Priced {
		t.Fatalf("baseline-only capex must not null the target: %+v", r.Unknown)
	}
	val(t, "net", r.NetModelledPerYear, 180_000)
	isNil(t, "amortized (baseline capital only)", r.Components.AmortizedOneTimePerYear)
	if len(r.SunkIgnored) != 2 || r.SunkIgnored[0] != (Item{"fa", "p1", "capex_usd"}) || r.SunkIgnored[1] != (Item{"fa", "p1", "retooling_usd"}) {
		t.Fatalf("sunk_ignored: %+v", r.SunkIgnored)
	}
	if !reflect.DeepEqual(r.Components, want.Components) || *r.NetModelledPerYear != *want.NetModelledPerYear {
		t.Fatalf("baseline capital changed the result")
	}
	// no horizon needed either
	in.Ops.HorizonYears = nil
	if r = eval(t, in); !r.Priced || hasUnknown(r, "financial", "horizon", "missing_horizon") {
		t.Fatal("baseline-only one-time must not require a horizon")
	}
}

func TestDestinationInvestmentActive(t *testing.T) {
	in := costly(4)
	in.plant("fa", "p2", func(p *PlantOps) { p.Capex = nil })
	in.plant("fa", "p3", func(p *PlantOps) { p.Ref = &rp3; p.Capex = f(1) }) // another potential destination
	r := eval(t, in)
	if r.Priced || !hasUnknown(r, "financial", "capex", "missing_target") {
		t.Fatalf("active capex missing at the destination must be unknown: %+v", r.Unknown)
	}
	isNil(t, "one-time", r.Components.OneTimeTotal)
	isNil(t, "amortized", r.Components.AmortizedOneTimePerYear)
	// horizon missing while active
	in = costly(4)
	in.Ops.HorizonYears = nil
	if r = eval(t, in); r.Priced || !hasUnknown(r, "financial", "horizon", "missing_horizon") {
		t.Fatalf("%+v", r.Unknown)
	}
	// known zero capex is entered: modelled, zero
	in = costly(4)
	in.plant("fa", "p2", func(p *PlantOps) { p.Capex, p.Retooling = f(0), f(0) })
	r = eval(t, in)
	val(t, "amortized", r.Components.AmortizedOneTimePerYear, 0)
	if !r.Priced {
		t.Fatal("known zero one-time stays priced")
	}
}

func TestAmbiguousBaseline(t *testing.T) {
	in := costly(4)
	in.Moves = []Move{{"fa", "", "p2"}}
	r := eval(t, in)
	if r.Priced || !hasUnknown(r, "financial", "operating", "ambiguous_baseline") {
		t.Fatalf("%+v", r.Unknown)
	}
	isNil(t, "operating", r.Components.OperatingDelta)
	isNil(t, "one-time", r.Components.OneTimeTotal)
	val(t, "duty", r.Components.DutyEffectPerYear, 200_000)
	if len(r.SunkIgnored) != 0 {
		t.Fatal("no arbitrary source plant may be picked")
	}
}

func TestStaleRef(t *testing.T) {
	// a re-added plant under the same ID but another country/name never inherits
	for _, ref := range []Ref{{"Chonburi works", "poland"}, {"Another works", "thailand"}} {
		in := costly(4)
		rr := ref
		in.plant("fa", "p2", func(p *PlantOps) { p.Ref = &rr })
		r := eval(t, in)
		if r.Priced || !hasUnknown(r, "financial", "labour", "missing_target") {
			t.Fatalf("stale destination must be unknown: %+v", r.Unknown)
		}
		if len(r.StaleRefs) == 0 || r.StaleRefs[0].Plant != "p2" {
			t.Fatalf("stale_refs: %+v", r.StaleRefs)
		}
	}
	// name folding: case and spacing are not a different plant
	in := costly(4)
	in.plant("fa", "p2", func(p *PlantOps) { p.Ref = &Ref{" chonburi   WORKS", "thailand"} })
	if r := eval(t, in); !r.Priced || len(r.StaleRefs) != 0 {
		t.Fatalf("folded name should match: %+v", r.Unknown)
	}
	// missing ref and unknown plant ID are stale
	in = costly(4)
	in.plant("fa", "p2", func(p *PlantOps) { p.Ref = nil })
	if r := eval(t, in); r.Priced {
		t.Fatal("entry without ref is stale")
	}
	in = costly(4)
	delete(in.Plants, "p2")
	if r := eval(t, in); r.Priced {
		t.Fatal("entry for a plant no longer in the product is stale")
	}
}

func TestLeadTime(t *testing.T) {
	mk := func(max int, lead ...*int) Input {
		in := base()
		in.Ops.MaxLeadTimeDays = n(max)
		in.Moves = nil
		for i, l := range lead {
			g := []string{"g1", "g2"}[i]
			in.Moves = append(in.Moves, Move{g, "p1", "p2"})
			in.Ops.Groups[g] = GroupOps{Plants: map[string]PlantOps{"p1": {Ref: &rp1}, "p2": {Ref: &rp2, LeadTimeDays: l}}}
		}
		return in
	}
	tests := []struct {
		name string
		in   Input
		want string
	}{
		{"tie feasible", mk(90, n(90)), Feasible},
		{"one over", mk(90, n(91)), Infeasible},
		{"max over groups", mk(90, n(60), n(120)), Infeasible},
		{"violation dominates unknown", mk(90, n(120), nil), Infeasible},
		{"unknown when one missing", mk(90, n(60), nil), StatusUnknown},
		{"all known ok", mk(90, n(60), n(0)), Feasible},
	}
	for _, tc := range tests {
		r := eval(t, tc.in)
		if r.Feasibility != tc.want || !r.Assessed {
			t.Fatalf("%s: feasibility %q, want %q", tc.name, r.Feasibility, tc.want)
		}
	}
	// reason + values for the infeasible case
	r := eval(t, mk(90, n(120)))
	c := r.Checks[0]
	if c.Kind != "lead_time" || c.Reason != "lead_time_exceeds_max" || c.Unit != "days" {
		t.Fatalf("%+v", c)
	}
	val(t, "value", c.Value, 120)
	val(t, "limit", c.Limit, 90)
	// no max entered: no check, listed, not unknown
	in := mk(90, n(500))
	in.Ops.MaxLeadTimeDays = nil
	in.Ops.HorizonYears = f(1)
	r = eval(t, in)
	if r.Assessed || len(r.Checks) != 0 || !has(r.NotModelled, "lead_time_constraint") {
		t.Fatalf("lead check must be inactive: %+v", r)
	}
}

func TestMOQ(t *testing.T) {
	mk := func(q *float64, unit string, d, o *float64) Input {
		in := base()
		in.Ops.Groups["fa"] = GroupOps{Unit: "pcs", AnnualDemandUnits: d, OrdersPerYear: o, Plants: map[string]PlantOps{
			"p1": {Ref: &rp1}, "p2": {Ref: &rp2, MOQUnits: q, MOQUnit: unit}}}
		return in
	}
	tests := []struct {
		name   string
		in     Input
		want   string
		reason string
	}{
		{"below moq", mk(f(5000), "pcs", f(24000), f(6)), Infeasible, "order_below_moq"},
		{"tie", mk(f(4000), "pcs", f(24000), f(6)), Feasible, ""},
		{"just below", mk(f(4000.01), "pcs", f(24000), f(6)), Infeasible, "order_below_moq"},
		{"moq zero needs no demand", mk(f(0), "pcs", nil, nil), Feasible, ""},
		{"unit differs", mk(f(100), "sets", f(24000), f(6)), StatusUnknown, "unit_mismatch"},
		{"unit case matters", mk(f(100), "Pcs", f(24000), f(6)), StatusUnknown, "unit_mismatch"},
		{"missing demand", mk(f(100), "pcs", nil, f(6)), StatusUnknown, "missing_demand"},
		{"missing orders", mk(f(100), "pcs", f(24000), nil), StatusUnknown, "missing_orders_per_year"},
		{"zero demand", mk(f(1), "pcs", f(0), f(6)), Infeasible, "order_below_moq"},
		{"fractional orders", mk(f(48000), "pcs", f(24000), f(0.5)), Feasible, ""},
	}
	for _, tc := range tests {
		r := eval(t, tc.in)
		if len(r.Checks) != 1 || r.Checks[0].Status != tc.want || r.Checks[0].Reason != tc.reason || r.Feasibility != tc.want {
			t.Fatalf("%s: %+v feasibility %q", tc.name, r.Checks, r.Feasibility)
		}
	}
	// a trailing space in the label is trimmed, nothing else
	if r := eval(t, mk(f(100), " pcs ", f(24000), f(6))); r.Checks[0].Status != Feasible {
		t.Fatalf("trim: %+v", r.Checks)
	}
	// MOQ entered only at the baseline plant is not a constraint
	in := mk(nil, "", f(24000), f(6))
	in.plant("fa", "p1", func(p *PlantOps) { p.MOQUnits, p.MOQUnit = f(99999), "pcs" })
	in.Ops.HorizonYears = f(1)
	r := eval(t, in)
	if r.Assessed || !has(r.NotModelled, "moq_constraint:fa") {
		t.Fatalf("baseline MOQ must be ignored: %+v", r.Checks)
	}
	// active through another plant but missing at the destination
	in = mk(nil, "", f(24000), f(6))
	in.plant("fa", "p3", func(p *PlantOps) { p.Ref = &rp3; p.MOQUnits, p.MOQUnit = f(10), "pcs" })
	if r = eval(t, in); r.Feasibility != StatusUnknown || r.Checks[0].Reason != "missing_moq" {
		t.Fatalf("%+v", r.Checks)
	}
	// two groups: one infeasible makes the move infeasible
	in = mk(f(100), "pcs", f(24000), f(6))
	in.Moves = append(in.Moves, Move{"g2", "p1", "p2"})
	in.Ops.Groups["g2"] = GroupOps{Unit: "pcs", AnnualDemandUnits: f(10), OrdersPerYear: f(1), Plants: map[string]PlantOps{
		"p1": {Ref: &rp1}, "p2": {Ref: &rp2, MOQUnits: f(11), MOQUnit: "pcs"}}}
	if r = eval(t, in); r.Feasibility != Infeasible || len(r.Checks) != 2 {
		t.Fatalf("%+v", r.Checks)
	}
}

func TestFinancialAndFeasibilityAreSeparate(t *testing.T) {
	// a fully known negative net with an unknown MOQ is net_costlier, and stays priced
	in := costly(4)
	in.Ops.Groups["fa"] = withMOQ(in.Ops.Groups["fa"], f(100), "pcs", nil, nil)
	r := eval(t, in)
	if !r.Priced || r.Feasibility != StatusUnknown || r.Decision != DecisionCostlier {
		t.Fatalf("priced=%v feasibility=%q decision=%q", r.Priced, r.Feasibility, r.Decision)
	}
	val(t, "net", r.NetModelledPerYear, -65_000)
	if !hasUnknown(r, "feasibility", "moq", "missing_demand") {
		t.Fatalf("%+v", r.Unknown)
	}
	// a known infeasibility dominates the decision code
	in.Ops.MaxLeadTimeDays = n(10)
	in.plant("fa", "p2", func(p *PlantOps) { p.LeadTimeDays = n(20) })
	if r = eval(t, in); r.Decision != DecisionInfeasible {
		t.Fatalf("decision %q", r.Decision)
	}
	// feasible and positive: no decision; positive with unknown feasibility: no decision
	in = costly(10)
	in.Ops.Groups["fa"] = withMOQ(in.Ops.Groups["fa"], f(100), "pcs", nil, nil)
	if r = eval(t, in); r.Decision != "" || r.Feasibility != StatusUnknown {
		t.Fatalf("decision %q feasibility %q", r.Decision, r.Feasibility)
	}
	// an unpriced move is never rejected on money
	in = costly(4)
	in.plant("fa", "p2", func(p *PlantOps) { p.Labour = nil })
	if r = eval(t, in); r.Decision != "" {
		t.Fatalf("decision %q", r.Decision)
	}
}

func withMOQ(g GroupOps, q *float64, unit string, d, o *float64) GroupOps {
	g.Unit, g.AnnualDemandUnits, g.OrdersPerYear = "pcs", d, o
	p := g.Plants["p2"]
	p.MOQUnits, p.MOQUnit = q, unit
	g.Plants["p2"] = p
	return g
}

func TestNetExactlyZeroStaysShown(t *testing.T) {
	in := base()
	in.AnnualImportValueUSD = 1000
	in.FromRate, in.ToRate = f(0.30), f(0.20) // duty effect 100
	in.GroupCosts = map[string]map[string]float64{"fa": {"p1": 0, "p2": 100}}
	in.Ops.HorizonYears = f(5)
	r := eval(t, in)
	val(t, "net", r.NetModelledPerYear, 0)
	if math.Signbit(*r.NetModelledPerYear) || r.Decision != "" {
		t.Fatalf("zero net: signbit/decision %q", r.Decision)
	}
	// net equal to -0.01 is rejected
	in.GroupCosts["fa"]["p2"] = 100.01
	if r = eval(t, in); r.Decision != DecisionCostlier {
		t.Fatalf("decision %q net %v", r.Decision, *r.NetModelledPerYear)
	}
}

func TestRoundingAndRange(t *testing.T) {
	// 1/3 of a cent boundaries: 1,000,000 / 3
	in := costly(3)
	val(t, "amortized", eval(t, in).Components.AmortizedOneTimePerYear, 333_333.33)
	// tiny positive horizon cannot produce Inf/NaN
	in = costly(1e-300)
	r := eval(t, in)
	if r.Priced || !hasUnknown(r, "financial", "amortized_one_time", "arithmetic_range") {
		t.Fatalf("%+v", r.Unknown)
	}
	if _, err := json.Marshal(r); err != nil {
		t.Fatal(err)
	}
	// huge inputs are refused, not summed
	in = costly(4)
	in.plant("fa", "p2", func(p *PlantOps) { p.Capex = f(math.MaxFloat64) })
	if r = eval(t, in); r.Priced || !hasUnknown(r, "financial", "capex", "arithmetic_range") {
		t.Fatalf("%+v", r.Unknown)
	}
	in = costly(4)
	in.AnnualImportValueUSD = math.Inf(1)
	r = eval(t, in)
	if _, err := json.Marshal(r); err != nil || r.Priced {
		t.Fatalf("inf import value: %v", err)
	}
	in = costly(4)
	in.FromRate = f(math.NaN())
	if r = eval(t, in); r.Priced {
		t.Fatal("NaN rate")
	}
}

func TestInactive(t *testing.T) {
	for _, o := range []*Operations{nil, {}, {Groups: map[string]GroupOps{}}, {Note: "x"},
		{Groups: map[string]GroupOps{"fa": {Plants: map[string]PlantOps{"p1": {Ref: &rp1, Sources: map[string]string{"capex_usd": "q"}}}}}}} {
		in := base()
		in.Ops = o
		if _, active := Evaluate(in); active || o.Active() || Normalize(o) != nil {
			t.Fatalf("%+v must be inactive", o)
		}
	}
	// explicit zeros are inputs
	for _, o := range []*Operations{{HorizonYears: f(0)}, {MaxLeadTimeDays: n(0)},
		{Groups: map[string]GroupOps{"fa": {Plants: map[string]PlantOps{"p1": {Labour: f(0)}}}}}} {
		if !o.Active() {
			t.Fatalf("%+v must be active", o)
		}
	}
}

func TestDecode(t *testing.T) {
	for _, s := range []string{`null`, `{}`, `{"groups":{}}`} {
		o, err := Decode([]byte(s))
		if err != nil || Normalize(o) != nil {
			t.Fatalf("%s: %v %+v", s, err, o)
		}
	}
	if _, err := Decode([]byte(`{"groups":{"fa":{"cost_bais":"included"}}}`)); err == nil {
		t.Fatal("unknown field must be rejected")
	}
	o, err := Decode([]byte(`{"groups":{"fa":{"plants":{"p1":{"labour_usd_per_year":0}}}}}`))
	if err != nil || o.Groups["fa"].Plants["p1"].Labour == nil {
		t.Fatal("explicit zero must decode to a non-nil pointer")
	}
	b, _ := json.Marshal(o)
	if !regexp.MustCompile(`"labour_usd_per_year":0`).Match(b) {
		t.Fatalf("explicit zero must survive marshal: %s", b)
	}
}

var negZero = regexp.MustCompile(`:-0([,}\]])`)

func TestPropertyNoNaNInfNegZero(t *testing.T) {
	vals := []float64{0, 0.004, 1, 333.335, 1e6, 1e12, 9e13, 1e300}
	hs := []float64{1e-300, 1e-9, 0.5, 3, 30}
	count := 0
	for _, v := range vals {
		for _, h := range hs {
			for _, c := range vals {
				in := costly(h)
				in.AnnualImportValueUSD = v
				in.GroupCosts["fa"]["p2"] = c
				in.plant("fa", "p2", func(p *PlantOps) { p.Capex = f(c); p.Retooling = f(v) })
				r, _ := Evaluate(in)
				b, err := json.Marshal(r)
				if err != nil || negZero.Match(b) {
					t.Fatalf("v=%v h=%v c=%v: err=%v %s", v, h, c, err, b)
				}
				for _, x := range []float64{r.DutyEffectYear, r.AddedCostYear, r.NetYear} {
					if !finite(x) || math.Signbit(x) && x == 0 {
						t.Fatalf("legacy figure %v", x)
					}
				}
				if r.Priced != (r.NetModelledPerYear != nil) {
					t.Fatal("priced iff net known")
				}
				count++
			}
		}
	}
	if count == 0 {
		t.Fatal("no cases")
	}
}

func TestDeterministicAcrossMapOrder(t *testing.T) {
	in := costly(4)
	for _, g := range []string{"g1", "g2", "g3"} {
		in.Ops.Groups[g] = GroupOps{CostBasis: BasisAdd, Plants: map[string]PlantOps{
			"p1": {Ref: &rp1, Labour: f(1), Capex: f(7)}, "p2": {Ref: &rp2, Labour: f(2)}, "p3": {Ref: &rp3, Capex: f(1)}}}
		in.Moves = append(in.Moves, Move{g, "p1", "p2"})
	}
	in.GroupCosts["g1"], in.GroupCosts["g2"] = map[string]float64{"p1": 1, "p2": 2}, map[string]float64{"p1": 1, "p2": 2}
	first := eval(t, in)
	for i := 0; i < 50; i++ {
		if !reflect.DeepEqual(first, eval(t, in)) {
			t.Fatal("result depends on map order")
		}
	}
}
