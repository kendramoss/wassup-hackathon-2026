package opcost

import (
	"math"
	"sort"
)

type readState int

const (
	rMissing readState = iota
	rOK
	rBad
)

// read looks at an entered number: nil is missing, a non-finite or oversized
// one is bad (refused, never summed).
func read(p *float64) (float64, readState) {
	if p == nil {
		return 0, rMissing
	}
	return readVal(*p)
}

func readVal(v float64) (float64, readState) {
	if !finite(v) || math.Abs(v) > MaxMagnitude {
		return 0, rBad
	}
	return v, rOK
}

func inRange(f float64) bool { return finite(f) && math.Abs(f) <= MaxMagnitude }

type calc struct {
	in    Input
	res   Result
	unk   map[Unknown]bool
	nm    map[string]bool
	stale map[Item]bool

	uGroup, uLabour, uLog, uOne bool
	gAcc, lAcc, loAcc, otAcc    float64 // known sums
	gLeg, lLeg, loLeg, otLeg    float64 // legacy sums: an unknown side reads 0
	oneActive                   bool
}

func (c *calc) unknown(cat, factor, group, reason string) {
	c.unk[Unknown{cat, factor, group, reason}] = true
}

func (c *calc) fin(factor, group, reason string) { c.unknown("financial", factor, group, reason) }

func (c *calc) notModelled(s string) { c.nm[s] = true }

// entry resolves the user's entry for group g at plant p. usable is false when
// there is no entry, or its ref no longer describes the product's plant.
func (c *calc) entry(g, p string) (pl PlantOps, exists, usable bool) {
	pl, exists = c.in.Ops.Groups[g].Plants[p]
	if !exists {
		return pl, false, false
	}
	if plant, ok := c.in.Plants[p]; ok && sameRef(pl.Ref, plant) {
		return pl, true, true
	}
	c.stale[Item{g, p, "ref"}] = true
	return pl, true, false
}

// Evaluate prices and checks the moves together as one plan move. The bool is
// false when the operations block is inactive: the caller then runs the
// legacy calculation unchanged.
func Evaluate(in Input) (Result, bool) {
	if !in.Ops.Active() {
		return Result{}, false
	}
	c := &calc{in: in, unk: map[Unknown]bool{}, nm: map[string]bool{}, stale: map[Item]bool{}}
	for _, s := range []string{"subsidies_incentives", "capacity_utilisation", "discounting_time_value", "taxes_residual_value", "ramp_up_yield"} {
		c.notModelled(s)
	}
	c.res.Assumptions = []string{AssumptionParallel, AssumptionStraightLine}
	duty, dutyOK := c.duty()
	for _, m := range in.Moves {
		c.cost(m)
	}
	c.finish(duty, dutyOK)
	c.feasibility()
	c.collect()
	return c.res, true
}

func (c *calc) duty() (float64, bool) {
	v, st := readVal(c.in.AnnualImportValueUSD)
	switch {
	case st == rBad:
		c.fin("duty", "", "arithmetic_range")
		return 0, false
	case v <= 0:
		c.fin("duty", "", "no_import_value")
		return 0, false
	case c.in.FromRate == nil || c.in.ToRate == nil:
		c.fin("duty", "", "no_duty_rate")
		return 0, false
	}
	r1, s1 := read(c.in.FromRate)
	r2, s2 := read(c.in.ToRate)
	d := round2(v * (r1 - r2))
	if s1 != rOK || s2 != rOK || !inRange(d) {
		c.fin("duty", "", "arithmetic_range")
		return 0, false
	}
	return d, true
}

func validBasis(b string) bool { return b == BasisIncluded || b == BasisAdd }

// cost adds one moved group's operating and one-time cost.
func (c *calc) cost(m Move) {
	g := c.in.Ops.Groups[m.Group]
	if m.FromPlant == "" { // ambiguous baseline: never pick a source plant
		c.fin("operating", m.Group, "ambiguous_baseline")
		c.uGroup, c.uLabour, c.uLog, c.uOne = true, true, true, true
		return
	}
	// Group cost: knownAddedCost semantics when any group cost was entered.
	if len(c.in.GroupCosts) > 0 {
		from, okF := c.in.GroupCosts[m.Group][m.FromPlant]
		to, okT := c.in.GroupCosts[m.Group][m.ToPlant]
		fv, sF := readVal(from)
		tv, sT := readVal(to)
		if !okF {
			sF = rMissing
		}
		if !okT {
			sT = rMissing
		}
		c.part(&c.uGroup, &c.gAcc, &c.gLeg, "group_cost", m.Group, sF, fv, sT, tv)
	} else {
		c.notModelled("group_cost")
		if g.CostBasis == BasisIncluded {
			c.fin("group_cost", m.Group, "missing_group_cost")
			c.uGroup = true
		}
	}
	// Labour and logistics: in scope when entered at any plant of the group.
	for _, f := range []struct {
		name string
		get  func(PlantOps) *float64
		u    *bool
		a, l *float64
	}{
		{"labour", func(p PlantOps) *float64 { return p.Labour }, &c.uLabour, &c.lAcc, &c.lLeg},
		{"logistics", func(p PlantOps) *float64 { return p.Logistics }, &c.uLog, &c.loAcc, &c.loLeg},
	} {
		entered := false
		for _, p := range g.Plants {
			if f.get(p) != nil {
				entered = true
			}
		}
		switch {
		case !entered:
			c.notModelled(f.name + ":" + m.Group)
		case !validBasis(g.CostBasis):
			c.fin(f.name, m.Group, "cost_basis_missing")
			*f.u = true
		case g.CostBasis == BasisIncluded:
			// a breakdown of the group cost: never added
		default:
			fp, _, fu := c.entry(m.Group, m.FromPlant)
			tp, _, tu := c.entry(m.Group, m.ToPlant)
			var fv, tv float64
			sF, sT := rMissing, rMissing
			if fu {
				fv, sF = read(f.get(fp))
			}
			if tu {
				tv, sT = read(f.get(tp))
			}
			c.part(f.u, f.a, f.l, f.name, m.Group, sF, fv, sT, tv)
		}
	}
	c.oneTime(m, g)
}

// part records a "to minus from" difference.
func (c *calc) part(u *bool, acc, leg *float64, factor, group string, sF readState, fv float64, sT readState, tv float64) {
	*leg += tv - fv
	if sF == rOK && sT == rOK {
		*acc += tv - fv
		return
	}
	*u = true
	for _, s := range []struct {
		st     readState
		reason string
	}{{sF, "missing_baseline"}, {sT, "missing_target"}} {
		switch s.st {
		case rMissing:
			c.fin(factor, group, s.reason)
		case rBad:
			c.fin(factor, group, "arithmetic_range")
		}
	}
}

// oneTime adds capex and retooling at the destination. A value at the group's
// current plant is sunk baseline capital: listed, never credited, and it never
// activates the factor.
func (c *calc) oneTime(m Move, g GroupOps) {
	for _, f := range []struct {
		name, key string
		get       func(PlantOps) *float64
	}{
		{"capex", "capex_usd", func(p PlantOps) *float64 { return p.Capex }},
		{"retooling", "retooling_usd", func(p PlantOps) *float64 { return p.Retooling }},
	} {
		active := false
		for id, p := range g.Plants {
			if f.get(p) == nil {
				continue
			}
			if id == m.FromPlant {
				c.res.SunkIgnored = append(c.res.SunkIgnored, Item{m.Group, id, f.key})
			} else {
				active = true
			}
		}
		if !active {
			c.notModelled(f.name + ":" + m.Group)
			continue
		}
		c.oneActive = true
		tp, _, tu := c.entry(m.Group, m.ToPlant)
		st := rMissing
		var v float64
		if tu {
			v, st = read(f.get(tp))
		}
		c.otLeg += v
		switch st {
		case rOK:
			c.otAcc += v
		case rMissing:
			c.uOne = true
			c.fin(f.name, m.Group, "missing_target")
		case rBad:
			c.uOne = true
			c.fin(f.name, m.Group, "arithmetic_range")
		}
	}
}

func ptr(f float64) *float64 { r := round2(f); return &r }

// finish assembles components, the net and the legacy figures.
func (c *calc) finish(duty float64, dutyOK bool) {
	r := &c.res
	h, hs := read(c.in.Ops.HorizonYears)
	if hs == rOK && h > 0 {
		r.HorizonYears = c.in.Ops.HorizonYears
	}
	var cm Components
	if dutyOK {
		cm.DutyEffectPerYear = ptr(duty)
	}
	set := func(dst **float64, unknown bool, v float64, name string) {
		if unknown {
			return
		}
		if !inRange(v) || !inRange(round2(v)) {
			c.fin(name, "", "arithmetic_range")
			return
		}
		*dst = ptr(v)
	}
	set(&cm.GroupCostDelta, c.uGroup, c.gAcc, "group_cost")
	set(&cm.LabourDelta, c.uLabour, c.lAcc, "labour")
	set(&cm.LogisticsDelta, c.uLog, c.loAcc, "logistics")
	if cm.GroupCostDelta != nil && cm.LabourDelta != nil && cm.LogisticsDelta != nil {
		set(&cm.OperatingDelta, false, *cm.GroupCostDelta+*cm.LabourDelta+*cm.LogisticsDelta, "operating")
	}
	set(&cm.OneTimeTotal, c.uOne, c.otAcc, "one_time")
	legAmort := 0.0
	switch {
	case c.uOne:
	case !c.oneActive:
		cm.AmortizedOneTimePerYear = ptr(0)
	case hs != rOK || h <= 0:
		c.fin("horizon", "", "missing_horizon")
	default:
		a := c.otAcc / h
		if !inRange(a) {
			c.fin("amortized_one_time", "", "arithmetic_range")
		} else {
			cm.AmortizedOneTimePerYear = ptr(a)
		}
	}
	if hs == rOK && h > 0 {
		if a := c.otLeg / h; inRange(a) {
			legAmort = a
		}
	}
	r.Components = cm
	if cm.DutyEffectPerYear != nil && cm.OperatingDelta != nil && cm.AmortizedOneTimePerYear != nil {
		n := round2(*cm.DutyEffectPerYear - *cm.OperatingDelta - *cm.AmortizedOneTimePerYear)
		if inRange(n) {
			r.NetModelledPerYear, r.Priced = &n, true
		} else {
			c.fin("net", "", "arithmetic_range")
		}
	}
	if dutyOK {
		r.DutyEffectYear = duty
	}
	if r.Priced {
		r.AddedCostYear, r.NetYear = *cm.OperatingDelta, *r.NetModelledPerYear
	} else {
		if add := round2(c.gLeg + c.lLeg + c.loLeg); inRange(add) {
			r.AddedCostYear = add
		}
		if n := round2(r.DutyEffectYear - r.AddedCostYear - round2(legAmort)); inRange(n) {
			r.NetYear = n
		}
	}
	switch {
	case r.Priced && *r.NetModelledPerYear < 0:
		r.Decision = DecisionCostlier
	}
}

// feasibility runs the lead-time and MOQ checks. They only decide the
// feasibility state; they never null a fully known financial net.
func (c *calc) feasibility() {
	r := &c.res
	if len(c.in.Moves) > 0 {
		if lim := c.in.Ops.MaxLeadTimeDays; lim != nil {
			c.leadTime(*lim)
		} else {
			c.notModelled("lead_time_constraint")
		}
	}
	for _, m := range c.in.Moves {
		c.moq(m)
	}
	if len(r.Checks) > 0 {
		r.Assessed = true
		r.Feasibility = Feasible
		for _, ch := range r.Checks {
			if ch.Status == Infeasible {
				r.Feasibility = Infeasible
				break
			}
			if ch.Status == StatusUnknown {
				r.Feasibility = StatusUnknown
			}
		}
	}
	if r.Feasibility == Infeasible {
		r.Decision = DecisionInfeasible
	}
}

func (c *calc) leadTime(limit int) {
	ch := Check{Kind: "lead_time", Unit: "days", Limit: ptrRaw(float64(limit))}
	longest, haveLongest, missing := 0, false, false
	for _, m := range c.in.Moves {
		p, _, usable := c.entry(m.Group, m.ToPlant)
		if !usable || p.LeadTimeDays == nil {
			missing = true
			c.unknown("feasibility", "lead_time", m.Group, "missing_lead_time")
			continue
		}
		if !haveLongest || *p.LeadTimeDays > longest {
			longest, haveLongest = *p.LeadTimeDays, true
		}
	}
	if haveLongest {
		ch.Value = ptrRaw(float64(longest))
	}
	switch {
	case haveLongest && longest > limit:
		ch.Status, ch.Reason = Infeasible, "lead_time_exceeds_max"
	case missing:
		ch.Status, ch.Reason = StatusUnknown, "missing_lead_time"
	default:
		ch.Status = Feasible
	}
	c.res.Checks = append(c.res.Checks, ch)
}

func ptrRaw(f float64) *float64 { return &f }

// moq checks the group's order size (demand / orders per year) against the
// destination plant's minimum order quantity. Units must match exactly.
func (c *calc) moq(m Move) {
	g := c.in.Ops.Groups[m.Group]
	active := false
	for id, p := range g.Plants {
		if p.MOQUnits != nil && id != m.FromPlant {
			active = true
		}
	}
	if !active {
		c.notModelled("moq_constraint:" + m.Group)
		return
	}
	unit := trim(g.Unit)
	ch := Check{Kind: "moq", Group: m.Group, Unit: unit}
	unk := func(reason string) {
		ch.Status, ch.Reason = StatusUnknown, reason
		c.unknown("feasibility", "moq", m.Group, reason)
		c.res.Checks = append(c.res.Checks, ch)
	}
	p, _, usable := c.entry(m.Group, m.ToPlant)
	var q float64
	qs := rMissing
	if usable {
		q, qs = read(p.MOQUnits)
	}
	if qs == rMissing {
		unk("missing_moq")
		return
	}
	if qs == rBad {
		unk("arithmetic_range")
		return
	}
	ch.Limit = ptrRaw(q)
	if q == 0 {
		ch.Status = Feasible
		c.res.Checks = append(c.res.Checks, ch)
		return
	}
	if unit == "" || trim(p.MOQUnit) != unit {
		unk("unit_mismatch")
		return
	}
	d, ds := read(g.AnnualDemandUnits)
	if ds != rOK {
		unk(map[readState]string{rMissing: "missing_demand", rBad: "arithmetic_range"}[ds])
		return
	}
	o, os := read(g.OrdersPerYear)
	if os != rOK || o <= 0 {
		unk(map[bool]string{true: "arithmetic_range", false: "missing_orders_per_year"}[os == rBad])
		return
	}
	if size := d / o; finite(size) {
		ch.Value = ptrRaw(size)
	}
	if d >= q*o {
		ch.Status = Feasible
	} else {
		ch.Status, ch.Reason = Infeasible, "order_below_moq"
	}
	c.res.Checks = append(c.res.Checks, ch)
}

// collect turns the sets into sorted, deterministic slices.
func (c *calc) collect() {
	r := &c.res
	for u := range c.unk {
		r.Unknown = append(r.Unknown, u)
	}
	sort.Slice(r.Unknown, func(i, j int) bool {
		a, b := r.Unknown[i], r.Unknown[j]
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		if a.Factor != b.Factor {
			return a.Factor < b.Factor
		}
		if a.Group != b.Group {
			return a.Group < b.Group
		}
		return a.Reason < b.Reason
	})
	for s := range c.nm {
		r.NotModelled = append(r.NotModelled, s)
	}
	sort.Strings(r.NotModelled)
	for it := range c.stale {
		r.StaleRefs = append(r.StaleRefs, it)
	}
	sortItems(r.StaleRefs)
	sortItems(r.SunkIgnored)
	if r.Unknown == nil {
		r.Unknown = []Unknown{}
	}
	if r.SunkIgnored == nil {
		r.SunkIgnored = []Item{}
	}
	if r.StaleRefs == nil {
		r.StaleRefs = []Item{}
	}
	if r.Checks == nil {
		r.Checks = []Check{}
	}
}

func sortItems(s []Item) {
	sort.Slice(s, func(i, j int) bool {
		a, b := s[i], s[j]
		if a.Group != b.Group {
			return a.Group < b.Group
		}
		if a.Plant != b.Plant {
			return a.Plant < b.Plant
		}
		return a.Factor < b.Factor
	})
}
