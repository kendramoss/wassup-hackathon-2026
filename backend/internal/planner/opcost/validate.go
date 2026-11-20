package opcost

import (
	"fmt"
	"math/big"
	"sort"
	"unicode/utf8"
)

// ValidateEnv is what validation knows about the case. Without a product
// (HasProduct false) the group and plant references are not checked.
type ValidateEnv struct {
	HasProduct bool
	Groups     map[string]bool // the product's group IDs
	Plants     map[string]Ref  // the product's plants by ID, canonical
	// CurrentPlant is each group's current plant. A group missing here (or ""
	// for a multi-plant group) has an ambiguous baseline: every one-time
	// entry then counts as a potential destination.
	CurrentPlant map[string]string
	GroupCosts   map[string]map[string]float64
}

// Problem is one validation failure; Slug is the stable machine code
// (operations.<slug>), reported under the existing invalid_input error.
type Problem struct {
	Slug, Path, Message string
}

func (p Problem) Error() string { return "operations." + p.Slug + ": " + p.Path + ": " + p.Message }

var sourceKeys = map[string]bool{"labour_usd_per_year": true, "logistics_usd_per_year": true, "capex_usd": true,
	"retooling_usd": true, "lead_time_days": true, "moq_units": true}

type validator struct {
	ps []Problem
}

func (v *validator) add(slug, path, format string, a ...any) {
	v.ps = append(v.ps, Problem{slug, path, fmt.Sprintf(format, a...)})
}

func (v *validator) money(path string, p *float64) {
	if p != nil && (!finite(*p) || *p < 0 || *p > MaxValue) {
		v.add("value_out_of_range", path, "must be a finite number from 0 to %g", MaxValue)
	}
}

func (v *validator) days(path string, p *int) {
	if p != nil && (*p < 0 || *p > MaxLeadDays) {
		v.add("lead_time_out_of_range", path, "must be whole days from 0 to %d", MaxLeadDays)
	}
}

func (v *validator) text(path, s string, max int) bool {
	if utf8.RuneCountInString(s) > max {
		v.add("string_too_long", path, "at most %d characters", max)
		return false
	}
	return true
}

// Validate checks an operations block. An inactive block is valid. Problems
// are in a stable order.
func Validate(o *Operations, env ValidateEnv) []Problem {
	if !o.Active() {
		return nil
	}
	v := &validator{}
	if o.HorizonYears != nil && (!finite(*o.HorizonYears) || *o.HorizonYears <= 0 || *o.HorizonYears > MaxHorizon) {
		v.add("horizon_out_of_range", "horizon_years", "must be greater than 0 and at most %g years", MaxHorizon)
	}
	v.days("max_lead_time_days", o.MaxLeadTimeDays)
	v.text("note", o.Note, MaxNote)
	if len(o.Groups) > MaxGroups {
		v.add("too_many_groups", "groups", "at most %d groups", MaxGroups)
	}
	ids := make([]string, 0, len(o.Groups))
	for id := range o.Groups {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	oneTimeTotal := 0.0
	oneTimeEntered := false
	for _, gid := range ids {
		g := o.Groups[gid]
		gp := "groups." + gid
		if gid == "" || (env.HasProduct && !env.Groups[gid]) {
			v.add("unknown_group", gp, "not an operation group of this product")
		}
		maxOne, entered := v.group(gid, g, env)
		if entered {
			oneTimeEntered = true
		}
		oneTimeTotal += maxOne
	}
	switch {
	case oneTimeEntered && o.HorizonYears == nil:
		v.add("horizon_required", "horizon_years", "needed to spread one-time costs entered for a plant a group could move to")
	case oneTimeEntered && o.HorizonYears != nil && finite(*o.HorizonYears) && *o.HorizonYears > 0 && *o.HorizonYears <= MaxHorizon:
		if a := oneTimeTotal / *o.HorizonYears; !finite(a) || a > MaxMagnitude {
			v.add("horizon_too_small", "horizon_years", "the yearly share of the one-time costs would exceed the supported range")
		}
	}
	return v.ps
}

// group validates one group. It returns the largest one-time total a single
// move of the group could carry and whether any potential-destination one-time
// cost was entered.
func (v *validator) group(gid string, g GroupOps, env ValidateEnv) (maxOne float64, entered bool) {
	gp := "groups." + gid
	if g.CostBasis != "" && !validBasis(g.CostBasis) {
		v.add("cost_basis_invalid", gp+".cost_basis", "must be %q or %q", BasisIncluded, BasisAdd)
	}
	g.Unit = trim(g.Unit)
	v.text(gp+".unit", g.Unit, MaxLabel)
	v.money(gp+".annual_demand_units", g.AnnualDemandUnits)
	if g.AnnualDemandUnits != nil && g.Unit == "" {
		v.add("unit_required", gp+".unit", "name the unit the demand is in")
	}
	if g.OrdersPerYear != nil && (!finite(*g.OrdersPerYear) || *g.OrdersPerYear <= 0 || *g.OrdersPerYear > MaxOrders) {
		v.add("orders_per_year_out_of_range", gp+".orders_per_year", "must be greater than 0 and at most %g", MaxOrders)
	}
	if len(g.Plants) > MaxPlants {
		v.add("too_many_plants", gp+".plants", "at most %d plants per group", MaxPlants)
	}
	pids := make([]string, 0, len(g.Plants))
	labourEntered := false
	for pid, p := range g.Plants {
		pids = append(pids, pid)
		if p.Labour != nil || p.Logistics != nil {
			labourEntered = true
		}
	}
	sort.Strings(pids)
	if labourEntered && g.CostBasis == "" {
		v.add("cost_basis_required", gp+".cost_basis", "say whether labour and logistics are included in the group cost or additional to it")
	}
	for _, pid := range pids {
		p := g.Plants[pid]
		pp := gp + ".plants." + pid
		if pid == "" || (env.HasProduct && !hasPlant(env.Plants, pid)) {
			v.add("unknown_plant", pp, "not a plant of this product")
		}
		if p.Ref == nil || p.Ref.Name == "" || p.Ref.Country == "" {
			v.add("plant_ref_required", pp+".ref", "name and country of the plant are required")
		} else {
			v.text(pp+".ref.name", p.Ref.Name, MaxText)
			v.text(pp+".ref.country", p.Ref.Country, MaxText)
			if plant, ok := env.Plants[pid]; env.HasProduct && ok && !sameRef(p.Ref, plant) {
				v.add("plant_ref_mismatch", pp+".ref", "does not match the product's plant %q", pid)
			}
		}
		v.money(pp+".labour_usd_per_year", p.Labour)
		v.money(pp+".logistics_usd_per_year", p.Logistics)
		v.money(pp+".capex_usd", p.Capex)
		v.money(pp+".retooling_usd", p.Retooling)
		v.days(pp+".lead_time_days", p.LeadTimeDays)
		v.money(pp+".moq_units", p.MOQUnits)
		if p.MOQUnits != nil {
			u := trim(p.MOQUnit)
			if u == "" {
				v.add("moq_unit_required", pp+".moq_unit", "name the unit of the minimum order")
			}
			v.text(pp+".moq_unit", u, MaxLabel)
		}
		for k, s := range p.Sources {
			if !sourceKeys[k] {
				v.add("unknown_source_key", pp+".sources."+k, "not a factor name")
			}
			v.text(pp+".sources."+k, s, MaxSource)
		}
		if g.CostBasis == BasisIncluded && (p.Labour != nil || p.Logistics != nil) {
			gc, ok := env.GroupCosts[gid][pid]
			switch {
			case !ok:
				v.add("included_requires_group_cost", pp, "an included breakdown needs the group cost entered for this plant")
			case cents(new(big.Rat).Add(dec(deref(p.Labour)), dec(deref(p.Logistics)))).Cmp(cents(dec(gc))) > 0:
				v.add("components_exceed_group_cost", pp, "labour plus logistics exceed the group cost")
			}
		}
		// one-time costs a move could carry: potential destinations only
		if p.Capex != nil || p.Retooling != nil {
			if cur := env.CurrentPlant[gid]; cur == "" || cur != pid {
				entered = true
				if t := deref(p.Capex) + deref(p.Retooling); finite(t) && t > maxOne {
					maxOne = t
				}
			}
		}
	}
	return maxOne, entered
}

func deref(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func hasPlant(m map[string]Ref, id string) bool { _, ok := m[id]; return ok }
