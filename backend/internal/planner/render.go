package planner

import (
	"fmt"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// Render writes a product's facts in the style of a CBP ruling request, so the
// engine reads plans the way it reads real cases: which components are
// produced in which country, processing detail only for the operation groups
// in detail (the groups a plan moves; CBP restates the new in-house work in
// that detail, e.g. N357771), and the final assembly. Output depends only on
// its inputs: groups in template order, components in product order.
//
// The first heat-gun run rendered processing detail for every group, which
// made the engine call the 2022 setup Thai (CBP held China); see
// docs/DECISIONS.md D16.
func Render(p domain.Product, t Template, detail ...string) string {
	var b strings.Builder
	final := finalAssemblyCountry(p, t)
	fmt.Fprintf(&b, "The merchandise under consideration is a %s.", p.Name)
	if p.Description != "" {
		fmt.Fprintf(&b, " %s", strings.TrimSpace(p.Description))
	}
	if final != "" {
		fmt.Fprintf(&b, " The %s is assembled in %s using components sourced from %s.", p.Name, final, joinAnd(componentCountries(p)))
	}
	b.WriteString("\n\n")

	// Where each component is made, grouped by country in first-seen order.
	byCountry, order := map[string][]string{}, []string{}
	for _, c := range p.Components {
		if _, ok := byCountry[c.MadeIn]; !ok {
			order = append(order, c.MadeIn)
		}
		byCountry[c.MadeIn] = append(byCountry[c.MadeIn], c.Name)
	}
	for i, country := range order {
		verb := "are produced in"
		if i > 0 {
			verb = "are sourced from"
		}
		if country == final {
			verb = "are produced in"
		}
		fmt.Fprintf(&b, "The %s %s %s. ", joinAnd(byCountry[country]), verb, country)
	}

	// Processing detail for the groups in detail, in template order.
	want := map[string]bool{}
	for _, d := range detail {
		want[d] = true
	}
	for _, g := range t.Groups {
		if !want[g.ID] || g.ID == "final_assembly" {
			continue
		}
		plant, ok := groupPlant(p, g.ID)
		if !ok {
			continue
		}
		subject := strings.ToLower(g.Name[:1]) + g.Name[1:]
		if comps := groupComponents(p, g.ID); len(comps) > 0 {
			subject = joinAnd(comps)
		}
		fmt.Fprintf(&b, "\n\nIn %s, the %s will be produced in-house: %s.", plant.Country, subject, joinOps(g.Operations))
	}
	if g, ok := t.Group("final_assembly"); ok && final != "" {
		ops := g.Operations
		if custom := groupOperations(p, "final_assembly"); len(custom) > 0 {
			ops = custom
		}
		fmt.Fprintf(&b, "\n\nFinal assembly in %s: %s.", final, joinOps(ops))
	}
	return strings.TrimSpace(b.String()) + "\n"
}

// Apply returns a copy of p with each moved group's components and operations
// at the destination plant (components then count as made in its country).
func Apply(p domain.Product, m domain.Move) domain.Product {
	out := p
	out.Components = append([]domain.Component(nil), p.Components...)
	out.Operations = append([]domain.Operation(nil), p.Operations...)
	for _, gm := range m {
		plant, ok := p.PlantByID(gm.ToPlant)
		if !ok {
			continue
		}
		for i := range out.Components {
			if out.Components[i].Group == gm.Group {
				out.Components[i].Plant, out.Components[i].MadeIn = plant.ID, plant.Country
			}
		}
		for i := range out.Operations {
			if out.Operations[i].Group == gm.Group {
				out.Operations[i].Plant = plant.ID
			}
		}
	}
	return out
}

// groupPlant is where a group is performed now: the plant of its operations,
// else of its components.
func groupPlant(p domain.Product, group string) (domain.Plant, bool) {
	for _, o := range p.Operations {
		if o.Group == group {
			return p.PlantByID(o.Plant)
		}
	}
	for _, c := range p.Components {
		if c.Group == group && c.Plant != "" {
			return p.PlantByID(c.Plant)
		}
	}
	return domain.Plant{}, false
}

func groupComponents(p domain.Product, group string) []string {
	var out []string
	for _, c := range p.Components {
		if c.Group == group {
			out = append(out, c.Name)
		}
	}
	return out
}

func groupOperations(p domain.Product, group string) []string {
	var out []string
	for _, o := range p.Operations {
		if o.Group == group {
			out = append(out, o.Name)
		}
	}
	return out
}

func finalAssemblyCountry(p domain.Product, t Template) string {
	if pl, ok := groupPlant(p, "final_assembly"); ok {
		return pl.Country
	}
	return ""
}

func componentCountries(p domain.Product) []string {
	var out []string
	seen := map[string]bool{}
	for _, c := range p.Components {
		if !seen[c.MadeIn] {
			seen[c.MadeIn] = true
			out = append(out, c.MadeIn)
		}
	}
	return out
}

func joinAnd(xs []string) string {
	switch len(xs) {
	case 0:
		return ""
	case 1:
		return xs[0]
	case 2:
		return xs[0] + " and " + xs[1]
	}
	return strings.Join(xs[:len(xs)-1], ", ") + " and " + xs[len(xs)-1]
}

func joinOps(ops []string) string { return strings.Join(ops, "; ") }
