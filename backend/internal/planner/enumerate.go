package planner

import (
	"sort"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// option is one movable group and a plant in the target country that can
// perform it.
type option struct{ group, plant string }

// options lists, in template group order then plant order, each movable group
// not yet in the target country with each target plant that can perform it.
func options(p domain.Product, t Template, target string) []option {
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
	return opts
}

// Enumerate lists every single and double move of movable operation groups to
// a plant in the target country that can perform them. Groups already
// performed in the target country are not moved. Order is deterministic:
// template group order, then plant order.
func Enumerate(p domain.Product, t Template, target string) []domain.Move {
	opts := options(p, t, domain.NormalizeCountry(target))
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

// EnumerateUpTo is Enumerate plus moves of three distinct groups, added only
// while the total stays at most maxMoves (D84). CBP's real change can move
// three groups at once (the heat gun's N357771), which pairs never cover.
// When not every triple fits, the triples whose groups cost the most where
// they are made now come first (costs: the request's group costs by plant),
// ties in template order. Singles and pairs keep Enumerate's order, so their
// cached verdicts are reused. maxMoves <= 0 adds no triples.
func EnumerateUpTo(p domain.Product, t Template, target string, costs map[string]map[string]float64, maxMoves int) []domain.Move {
	moves := Enumerate(p, t, target)
	room := maxMoves - len(moves)
	if room <= 0 {
		return moves
	}
	opts := options(p, t, domain.NormalizeCountry(target))
	type triple struct {
		move domain.Move
		cost float64
	}
	current := func(group string) float64 {
		from, ok := groupPlant(p, group)
		if !ok {
			return 0
		}
		return costs[group][from.ID]
	}
	var ts []triple
	for i := 0; i < len(opts); i++ {
		for j := i + 1; j < len(opts); j++ {
			for k := j + 1; k < len(opts); k++ {
				a, b, c := opts[i], opts[j], opts[k]
				if a.group == b.group || a.group == c.group || b.group == c.group {
					continue
				}
				ts = append(ts, triple{
					move: domain.Move{{Group: a.group, ToPlant: a.plant}, {Group: b.group, ToPlant: b.plant}, {Group: c.group, ToPlant: c.plant}},
					cost: current(a.group) + current(b.group) + current(c.group),
				})
			}
		}
	}
	sort.SliceStable(ts, func(i, j int) bool { return ts[i].cost > ts[j].cost })
	for i := 0; i < len(ts) && i < room; i++ {
		moves = append(moves, ts[i].move)
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
