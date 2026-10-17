package bom

import (
	"fmt"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// Change kinds reported by Diff.
const (
	ComponentAdded   = "component_added"
	ComponentRemoved = "component_removed"
	CountryChanged   = "country_changed" // made_in changed (the plant may have changed too)
	PlantChanged     = "plant_changed"   // same country, another plant
	OperationAdded   = "operation_added"
	OperationRemoved = "operation_removed"
	OperationMoved   = "operation_moved" // performed at another plant or in another country
)

// Change is one origin-relevant difference between two versions of a
// product (D67). Plants are named by plant name, not ID: a BOM upload derives
// IDs from names, so the stored product and the upload can use different IDs
// for the same plant.
type Change struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	FromCountry string `json:"from_country,omitempty"`
	ToCountry   string `json:"to_country,omitempty"`
	FromPlant   string `json:"from_plant,omitempty"`
	ToPlant     string `json:"to_plant,omitempty"`
	Summary     string `json:"summary"`
}

// Diff lists what changed from before to after that can change the origin:
// components added or removed, a component made in another country or at
// another plant, operations added, removed or moved. Components and
// operations are matched by name (case and spacing ignored; repeated names
// pair in order). Groups, capabilities and plants that make nothing are not
// compared: they change the plan search, not the origin.
func Diff(before, after domain.Product) []Change {
	out := []Change{}
	newComps := index(len(after.Components), func(i int) string { return after.Components[i].Name })
	matched := make([]bool, len(after.Components))
	for _, oc := range before.Components {
		j, ok := newComps.take(oc.Name)
		if !ok {
			out = append(out, Change{Kind: ComponentRemoved, Name: oc.Name, FromCountry: oc.MadeIn, FromPlant: plantName(before, oc.Plant),
				Summary: fmt.Sprintf("%s (made in %s) is no longer in the BOM", oc.Name, oc.MadeIn)})
			continue
		}
		matched[j] = true
		nc := after.Components[j]
		fromPlant, toPlant := plantName(before, oc.Plant), plantName(after, nc.Plant)
		// A bought-in component has no plant; only compare plants both name.
		plantMoved := fromPlant != "" && toPlant != "" && plantID(fromPlant) != plantID(toPlant)
		switch {
		case !sameCountry(oc.MadeIn, nc.MadeIn):
			c := Change{Kind: CountryChanged, Name: nc.Name, FromCountry: oc.MadeIn, ToCountry: nc.MadeIn,
				Summary: fmt.Sprintf("%s: made in %s, was %s", nc.Name, nc.MadeIn, oc.MadeIn)}
			if plantMoved {
				c.FromPlant, c.ToPlant = fromPlant, toPlant
				c.Summary += fmt.Sprintf(" (plant %s, was %s)", toPlant, fromPlant)
			}
			out = append(out, c)
		case plantMoved:
			out = append(out, Change{Kind: PlantChanged, Name: nc.Name, FromCountry: oc.MadeIn, ToCountry: nc.MadeIn, FromPlant: fromPlant, ToPlant: toPlant,
				Summary: fmt.Sprintf("%s: made at %s, was %s", nc.Name, toPlant, fromPlant)})
		}
	}
	for j, nc := range after.Components {
		if !matched[j] {
			out = append(out, Change{Kind: ComponentAdded, Name: nc.Name, ToCountry: nc.MadeIn, ToPlant: plantName(after, nc.Plant),
				Summary: fmt.Sprintf("%s (made in %s) is new in the BOM", nc.Name, nc.MadeIn)})
		}
	}
	newOps := index(len(after.Operations), func(i int) string { return after.Operations[i].Name })
	matched = make([]bool, len(after.Operations))
	for _, oo := range before.Operations {
		fromPlant, fromCountry := plantName(before, oo.Plant), plantCountry(before, oo.Plant)
		j, ok := newOps.take(oo.Name)
		if !ok {
			out = append(out, Change{Kind: OperationRemoved, Name: oo.Name, FromCountry: fromCountry, FromPlant: fromPlant,
				Summary: fmt.Sprintf("operation no longer in the BOM: %s (%s)", oo.Name, fromCountry)})
			continue
		}
		matched[j] = true
		no := after.Operations[j]
		toPlant, toCountry := plantName(after, no.Plant), plantCountry(after, no.Plant)
		if !sameCountry(fromCountry, toCountry) || plantID(fromPlant) != plantID(toPlant) {
			out = append(out, Change{Kind: OperationMoved, Name: no.Name, FromCountry: fromCountry, ToCountry: toCountry, FromPlant: fromPlant, ToPlant: toPlant,
				Summary: fmt.Sprintf("operation moved to %s (%s), was %s (%s): %s", toPlant, toCountry, fromPlant, fromCountry, no.Name)})
		}
	}
	for j, no := range after.Operations {
		if !matched[j] {
			out = append(out, Change{Kind: OperationAdded, Name: no.Name, ToCountry: plantCountry(after, no.Plant), ToPlant: plantName(after, no.Plant),
				Summary: fmt.Sprintf("operation new in the BOM: %s (%s)", no.Name, plantCountry(after, no.Plant))})
		}
	}
	return out
}

// names maps a normalised name to the unmatched indices with that name.
type names map[string][]int

func index(n int, name func(int) string) names {
	m := names{}
	for i := 0; i < n; i++ {
		k := nameKey(name(i))
		m[k] = append(m[k], i)
	}
	return m
}

// take returns the first unmatched index with the name.
func (m names) take(name string) (int, bool) {
	k := nameKey(name)
	if len(m[k]) == 0 {
		return 0, false
	}
	i := m[k][0]
	m[k] = m[k][1:]
	return i, true
}

func nameKey(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

func sameCountry(a, b string) bool {
	return domain.NormalizeCountry(a) == domain.NormalizeCountry(b)
}

func plantName(p domain.Product, id string) string {
	if id == "" {
		return ""
	}
	if pl, ok := p.PlantByID(id); ok && pl.Name != "" {
		return pl.Name
	}
	return id
}

func plantCountry(p domain.Product, id string) string {
	pl, _ := p.PlantByID(id)
	return pl.Country
}

// KeepPlantIDs returns after with each plant that has the same name as a
// plant in before (compared as plant IDs are made: case and punctuation
// ignored) given before's ID, and component and operation references
// rewritten to match. A BOM upload names plants by ID from their names
// ("rayong_plant"), while a stored product may use its own ("th"); keeping
// the stored IDs keeps the case's economics (group costs by plant ID) and
// current origin pointing at the same plants. A rename that would clash
// with another plant's ID is skipped.
func KeepPlantIDs(before, after domain.Product) domain.Product {
	byName := map[string]string{}
	for _, pl := range before.Plants {
		byName[plantID(pl.Name)] = pl.ID
	}
	rename := map[string]string{}
	used := map[string]bool{}
	for _, pl := range after.Plants {
		used[pl.ID] = true
	}
	for _, pl := range after.Plants {
		id, ok := byName[plantID(pl.Name)]
		if !ok || id == pl.ID || used[id] {
			continue
		}
		rename[pl.ID], used[id] = id, true
	}
	if len(rename) == 0 {
		return after
	}
	out := after
	out.Plants = append([]domain.Plant(nil), after.Plants...)
	out.Components = append([]domain.Component(nil), after.Components...)
	out.Operations = append([]domain.Operation(nil), after.Operations...)
	for i := range out.Plants {
		if id, ok := rename[out.Plants[i].ID]; ok {
			out.Plants[i].ID = id
		}
	}
	for i := range out.Components {
		if id, ok := rename[out.Components[i].Plant]; ok {
			out.Components[i].Plant = id
		}
	}
	for i := range out.Operations {
		if id, ok := rename[out.Operations[i].Plant]; ok {
			out.Operations[i].Plant = id
		}
	}
	return out
}
