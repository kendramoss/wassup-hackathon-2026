package domain

import (
	"strings"
	"time"
)

// Plant is a site where operations can be performed.
type Plant struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Country string `json:"country"`
}

// Component is a part of the product, made (or bought) in a country.
// Plant is the ID of the product's own plant that makes it, or empty when the
// component is bought in from a supplier in MadeIn.
type Component struct {
	Name   string `json:"name"`
	MadeIn string `json:"made_in"`
	Plant  string `json:"plant,omitempty"`
	// Group is the operation group that produces this component (e.g.
	// "motor_fan_assembly"), so moving the group moves the component.
	Group string `json:"group,omitempty"`
}

// Operation is a processing step performed at one of the product's plants.
type Operation struct {
	Name  string `json:"name"`
	Plant string `json:"plant"`
	Group string `json:"group,omitempty"`
}

// Product is the structured description of a case: what is made, from which
// components, with which operations at which plants.
type Product struct {
	Name        string      `json:"name"`
	Family      string      `json:"family,omitempty"`
	Description string      `json:"description,omitempty"`
	Plants      []Plant     `json:"plants"`
	Components  []Component `json:"components"`
	Operations  []Operation `json:"operations"`
}

// PlantByID returns the plant with the given ID.
func (p Product) PlantByID(id string) (Plant, bool) {
	for _, pl := range p.Plants {
		if pl.ID == id {
			return pl, true
		}
	}
	return Plant{}, false
}

// Validate checks that the product is complete and that every plant reference
// resolves.
func (p Product) Validate() error {
	var errs problems
	if strings.TrimSpace(p.Name) == "" {
		errs.add("product name is required")
	}
	if len(p.Components) == 0 {
		errs.add("at least one component is required")
	}
	seen := map[string]bool{}
	for i, pl := range p.Plants {
		switch {
		case pl.ID == "":
			errs.add("plants[%d]: id is required", i)
		case seen[pl.ID]:
			errs.add("plants[%d]: duplicate id %q", i, pl.ID)
		}
		seen[pl.ID] = true
		if strings.TrimSpace(pl.Country) == "" {
			errs.add("plants[%d]: country is required", i)
		}
	}
	for i, c := range p.Components {
		if strings.TrimSpace(c.Name) == "" {
			errs.add("components[%d]: name is required", i)
		}
		if strings.TrimSpace(c.MadeIn) == "" {
			errs.add("components[%d]: made_in is required", i)
		}
		if c.Plant != "" && !seen[c.Plant] {
			errs.add("components[%d]: unknown plant %q", i, c.Plant)
		}
	}
	for i, o := range p.Operations {
		if strings.TrimSpace(o.Name) == "" {
			errs.add("operations[%d]: name is required", i)
		}
		if !seen[o.Plant] {
			errs.add("operations[%d]: unknown plant %q", i, o.Plant)
		}
	}
	return errs.err()
}

// Case is one origin question: a product as of a date. Facts is the free-text
// description the engine reads; when Product is set, Facts may be rendered
// from it. Date bounds precedent retrieval: only rulings dated strictly before
// it may be used.
type Case struct {
	ID      string    `json:"id"`
	Date    time.Time `json:"date"`
	Facts   string    `json:"facts"`
	Product *Product  `json:"product,omitempty"`
}

// MaxFactsRunes caps free-text facts so one request can't send an unbounded
// prompt. The research harness truncated facts at 12,000 characters.
const MaxFactsRunes = 12000

// Validate checks the case has a date and either facts or a valid product.
func (c Case) Validate() error {
	var errs problems
	if c.Date.IsZero() {
		errs.add("date is required (it bounds which earlier rulings may be cited)")
	}
	if strings.TrimSpace(c.Facts) == "" && c.Product == nil {
		errs.add("facts or product is required")
	}
	if n := len([]rune(c.Facts)); n > MaxFactsRunes {
		errs.add("facts is %d characters; max %d", n, MaxFactsRunes)
	}
	if c.Product != nil {
		if err := c.Product.Validate(); err != nil {
			errs = append(errs, err.(*ValidationError).Problems...)
		}
	}
	return errs.err()
}
