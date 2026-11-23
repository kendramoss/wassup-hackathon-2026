package planner

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner/opcost"
)

// Comparison is the yearly money view of one production setup against the
// current one, from the user's economics (illustrative unless sourced).
type Comparison struct {
	// DutyRate and AnnualDutyUSD are for the setup's predicted origin; nil
	// when the economics have no rate for that origin.
	DutyRate      *float64 `json:"duty_rate"`
	AnnualDutyUSD *float64 `json:"annual_duty_usd"`
	// DutySavingsUSD is the current setup's duty minus this setup's duty.
	DutySavingsUSD *float64 `json:"duty_savings_usd"`
	// AddedCostUSD is the yearly cost of moving operation groups to other
	// plants (only for structured products with group costs).
	AddedCostUSD *float64 `json:"added_cost_usd"`
	// NetUSD is savings minus added cost, when both are known.
	NetUSD       *float64    `json:"net_usd"`
	Moved        domain.Move `json:"moved"`
	Illustrative bool        `json:"illustrative"`
	// OriginDiffersFromCurrent is true when the setup's predicted origin is not
	// the current setup's origin. For a setup that moves nothing this is the
	// engine disagreeing with the current origin, not a duty change.
	OriginDiffersFromCurrent bool `json:"origin_differs_from_current"`
	// The fields below are present only when the economics carry an operations
	// block (#281). AddedCostUSD is then the operating cost difference
	// (nullable: unknown or not modelled), NetUSD the modelled net (non-null
	// only when every financial input is known), and Operations the full
	// result with the absolute baseline and proposed sides.
	AmortizedOneTimeUSD *float64       `json:"amortized_one_time_usd,omitempty"`
	Feasibility         string         `json:"feasibility,omitempty"`
	Operations          *opcost.Result `json:"operations,omitempty"`
}

// Compare prices a setup whose predicted origin is origin against the current
// setup (product current, origin currentOrigin). variant is the setup's
// structured product, if it has one; groups it places at a different plant
// count as moves.
//
// A structured setup that is materially equivalent to the current one (no
// group move, same component origins, plant countries, operations and
// capabilities) is the current setup: its duty stays at the current origin's
// rate and it saves nothing, even when the engine predicts a different origin
// for it. Any material difference, with or without a group move, is priced
// at the scenario's origin.
func Compare(e Economics, current *domain.Product, variant *domain.Product, currentOrigin, origin string) Comparison {
	c := Comparison{Illustrative: e.Illustrative, Moved: domain.Move{},
		OriginDiffersFromCurrent: domain.NormalizeCountry(origin) != domain.NormalizeCountry(currentOrigin)}
	if current != nil && variant != nil && len(movesBetween(*current, *variant)) == 0 && materiallyEquivalent(*current, *variant) {
		origin = currentOrigin
	}
	r2, ok2 := e.DutyRate(origin)
	if ok2 {
		c.DutyRate = ptr(r2)
		c.AnnualDutyUSD = ptr(round2(e.AnnualImportValueUSD * r2))
		if r1, ok1 := e.DutyRate(currentOrigin); ok1 {
			c.DutySavingsUSD = ptr(round2(e.AnnualImportValueUSD * (r1 - r2)))
		}
	}
	if current != nil && variant != nil {
		c.Moved = movesBetween(*current, *variant)
		if e.Operations.Active() { // one calculator for plan, scenario and target comparison
			c.priceOps(e, *current, variant, currentOrigin, origin)
			return c
		}
		if len(e.GroupCosts) > 0 {
			if added, known := knownAddedCost(e, *current, c.Moved); known {
				c.AddedCostUSD = ptr(added)
			}
		}
	}
	if c.DutySavingsUSD != nil && c.AddedCostUSD != nil {
		c.NetUSD = ptr(round2(*c.DutySavingsUSD - *c.AddedCostUSD))
	}
	return c
}

// priceOps fills the money of a comparison from opcost.Evaluate, the same
// arithmetic the plan search uses for the same move.
func (c *Comparison) priceOps(e Economics, current domain.Product, variant *domain.Product, currentOrigin, origin string) {
	res := opsEvaluate(e, current, variant, c.Moved, currentOrigin, origin)
	c.Operations = &res
	// Dollar duty is unknown, not 0, when there is no annual import value or the
	// duty effect could not be worked out. A known rate stays.
	if e.AnnualImportValueUSD <= 0 {
		c.AnnualDutyUSD = nil
	}
	if e.AnnualImportValueUSD <= 0 || res.Components.DutyEffectPerYear == nil {
		c.DutySavingsUSD = nil
	}
	c.AddedCostUSD = res.Components.OperatingDelta
	c.AmortizedOneTimeUSD = res.Components.AmortizedOneTimePerYear
	c.NetUSD = res.NetModelledPerYear
	c.Feasibility = res.Feasibility
}

// moneyBaselinePlant is the one plant a moved group is priced from in the
// legacy money paths. A group performed at several plants (CurrentPlant "", the
// same rule the operations block uses) has no single baseline cost, so it is
// not found rather than picked from the order of the operations list (#716).
// Only money uses it; legal facts and enumeration keep groupPlant.
func moneyBaselinePlant(p domain.Product, group string) (domain.Plant, bool) {
	id := CurrentPlant(p, group)
	if id == "" {
		return domain.Plant{}, false
	}
	return p.PlantByID(id)
}

// knownAddedCost is the yearly cost of the moves, only when every moved group
// has a cost entered at both its current plant and its destination. A missing
// entry is unknown, not zero (D131); an explicit 0 is a known cost. A setup
// that moves nothing costs nothing extra. The plan search keeps its own
// semantics for blank costs (economics in plan.go).
func knownAddedCost(e Economics, current domain.Product, m domain.Move) (float64, bool) {
	var added float64
	for _, gm := range m {
		from, ok := moneyBaselinePlant(current, gm.Group)
		if !ok {
			return 0, false
		}
		costs := e.GroupCosts[gm.Group]
		fromCost, haveFrom := costs[from.ID]
		toCost, haveTo := costs[gm.ToPlant]
		if !haveFrom || !haveTo {
			return 0, false
		}
		added += toCost - fromCost
	}
	return round2(added), true
}

// materiallyEquivalent reports whether two structured products describe the
// same production setup for origin purposes. Order of lists and display-only
// labels (product and plant names, description) are ignored; component name,
// origin country, plant, group, operations, plant country and capabilities
// are compared (names case/space-folded, countries normalised).
func materiallyEquivalent(a, b domain.Product) bool {
	return setupSignature(a) == setupSignature(b)
}

func setupSignature(p domain.Product) string {
	fold := func(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
	var parts []string
	for _, pl := range p.Plants {
		caps := make([]string, 0, len(pl.Capabilities))
		for _, c := range pl.Capabilities {
			caps = append(caps, fold(c))
		}
		sort.Strings(caps)
		parts = append(parts, fmt.Sprintf("P%q|%q|%q", pl.ID, domain.NormalizeCountry(pl.Country), strings.Join(caps, ",")))
	}
	for _, c := range p.Components {
		parts = append(parts, fmt.Sprintf("C%q|%q|%q|%q", fold(c.Name), domain.NormalizeCountry(c.MadeIn), c.Plant, fold(c.Group)))
	}
	for _, o := range p.Operations {
		parts = append(parts, fmt.Sprintf("O%q|%q|%q", fold(o.Name), o.Plant, fold(o.Group)))
	}
	sort.Strings(parts)
	return strings.Join(parts, "\n")
}

// movesBetween lists the operation groups the variant performs at a different
// plant than the current product, in group order.
func movesBetween(current, variant domain.Product) domain.Move {
	groups := map[string]bool{}
	for _, comp := range current.Components {
		groups[comp.Group] = true
	}
	for _, op := range current.Operations {
		groups[op.Group] = true
	}
	names := make([]string, 0, len(groups))
	for g := range groups {
		if g != "" {
			names = append(names, g)
		}
	}
	sort.Strings(names)
	m := domain.Move{}
	for _, g := range names {
		from, ok1 := groupPlant(current, g)
		to, ok2 := groupPlant(variant, g)
		if ok1 && ok2 && from.ID != to.ID {
			m = append(m, domain.GroupMove{Group: g, ToPlant: to.ID})
		}
	}
	return m
}

func ptr(f float64) *float64 { return &f }

func round2(f float64) float64 { return math.Round(f*100) / 100 }

// MatchMove is the plan search's row for the move that turns current into
// variant: the same groups to the same plants, in any order. Nil when the
// variant moves nothing or the search did not try that move (D220).
func MatchMove(r *Result, current, variant domain.Product) *domain.Plan {
	if r == nil {
		return nil
	}
	want := movesBetween(current, variant)
	if len(want) == 0 {
		return nil
	}
	key := sortedKey(want)
	for _, list := range [][]domain.Plan{r.Plans, r.Rejected} {
		for i := range list {
			if len(list[i].Move) == len(want) && sortedKey(list[i].Move) == key {
				return &list[i]
			}
		}
	}
	return nil
}

func sortedKey(m domain.Move) string {
	parts := make([]string, len(m))
	for i, g := range m {
		parts[i] = g.Group + "@" + g.ToPlant
	}
	sort.Strings(parts)
	return strings.Join(parts, "+")
}

// MovedItems names what a move takes with it in this product (D220): the
// components of each moved group that are not already at the plant the group
// moves to, in product order, each once. A group's operations are listed only
// when none of its components move, so the list names parts, not process
// sentences (judge panel round 3, D270: the gas grill listed 18 items).
func MovedItems(p domain.Product, m domain.Move) []string {
	var out []string
	seen := map[string]bool{}
	hasParts := map[string]bool{}
	add := func(name, group, plant string) bool {
		added := false
		for _, gm := range m {
			if gm.Group == group && plant != gm.ToPlant && name != "" {
				added = true
				if !seen[name] {
					seen[name] = true
					out = append(out, name)
				}
			}
		}
		return added
	}
	for _, c := range p.Components {
		if add(c.Name, c.Group, c.Plant) {
			hasParts[c.Group] = true
		}
	}
	for _, o := range p.Operations {
		if !hasParts[o.Group] {
			add(o.Name, o.Group, o.Plant)
		}
	}
	return out
}
