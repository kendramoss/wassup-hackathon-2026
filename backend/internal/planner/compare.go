package planner

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
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
		if len(e.GroupCosts) > 0 {
			_, added := economics(Request{Economics: e, Product: *current}, c.Moved, "", "")
			c.AddedCostUSD = ptr(added)
		}
	}
	if c.DutySavingsUSD != nil && c.AddedCostUSD != nil {
		c.NetUSD = ptr(round2(*c.DutySavingsUSD - *c.AddedCostUSD))
	}
	return c
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
