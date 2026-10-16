package planner

import (
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// Enumerate lists every single and double move of movable operation groups to
// a plant in the target country that can perform them. Groups already
// performed in the target country are not moved. Order is deterministic:
// template group order, then plant order.
func Enumerate(p domain.Product, t Template, target string) []domain.Move {
	target = domain.NormalizeCountry(target)
	type option struct{ group, plant string }
	var opts []option
	for _, g := range t.Groups {
		if !g.Movable {
			continue
		}
		cur, present := groupPlant(p, g.ID)
		if !present && len(groupComponents(p, g.ID)) == 0 {
			continue // the product has no such group
		}
		if present && domain.NormalizeCountry(cur.Country) == target {
			continue
		}
		if !present && allComponentsIn(p, g.ID, target) {
			continue
		}
		for _, pl := range p.Plants {
			if domain.NormalizeCountry(pl.Country) == target && pl.Can(g.ID) {
				opts = append(opts, option{g.ID, pl.ID})
			}
		}
	}
	var moves []domain.Move
	for _, o := range opts {
		moves = append(moves, domain.Move{{Group: o.group, ToPlant: o.plant}})
	}
	for i := 0; i < len(opts); i++ {
		for j := i + 1; j < len(opts); j++ {
			if opts[i].group == opts[j].group {
				continue
			}
			moves = append(moves, domain.Move{{Group: opts[i].group, ToPlant: opts[i].plant}, {Group: opts[j].group, ToPlant: opts[j].plant}})
		}
	}
	return moves
}

func allComponentsIn(p domain.Product, group, country string) bool {
	for _, c := range p.Components {
		if c.Group == group && domain.NormalizeCountry(c.MadeIn) != country {
			return false
		}
	}
	return true
}

// Describe is a one-line description of a move for verifier prompts and UI,
// e.g. "Make Motor-fan assembly and PCBA (SMT) in Thailand".
func Describe(p domain.Product, t Template, m domain.Move) string {
	if len(m) == 0 {
		return "No change: keep the current setup"
	}
	names := make([]string, 0, len(m))
	country := ""
	sameCountry := true
	for _, gm := range m {
		name := gm.Group
		if g, ok := t.Group(gm.Group); ok {
			name = g.Name
		}
		pl, _ := p.PlantByID(gm.ToPlant)
		if country != "" && pl.Country != country {
			sameCountry = false
		}
		country = pl.Country
		names = append(names, name)
	}
	if sameCountry {
		return "Make " + joinAnd(names) + " in " + country
	}
	parts := make([]string, len(m))
	for i, gm := range m {
		pl, _ := p.PlantByID(gm.ToPlant)
		parts[i] = names[i] + " in " + pl.Country
	}
	return "Make " + joinAnd(parts)
}
