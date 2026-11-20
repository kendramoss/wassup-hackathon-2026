// Package opcost is the pure calculator for modelled operational cost and
// feasibility (#281). It holds the optional "operations" inputs, their
// validation and the single Evaluate function that plan search, scenarios and
// target comparisons are meant to share. It imports only the standard library:
// no HTTP, store, config or planner types. Countries arrive already
// canonicalised by the caller (the existing domain.NormalizeCountry at the
// adapter boundary); nothing here converts units or invents defaults or
// estimates. Every figure is derived from user-entered inputs.
package opcost

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
)

// Limits (contract section 3). MaxMagnitude keeps every USD figure below
// 2^53/100 so cents stay exact in float64; anything larger is refused.
const (
	MaxGroups     = 64
	MaxPlants     = 32
	MaxHorizon    = 30.0
	MaxOrders     = 1e6
	MaxValue      = 1e12
	MaxLeadDays   = 3650
	MaxMagnitude  = 9e13
	MaxLabel      = 24
	MaxText       = 120
	MaxNote       = 500
	MaxSource     = 200
	BasisIncluded = "included"
	BasisAdd      = "additional"
)

// Ref binds an entry to a plant by name and (canonical) country, so an entry
// keyed by a plant ID that was later reused for another plant is not read.
type Ref struct {
	Name    string `json:"name"`
	Country string `json:"country"`
}

// Operations is the optional operations block. A nil pointer or absent field is
// NOT ENTERED; an explicit 0 is a known zero and is kept.
type Operations struct {
	HorizonYears    *float64            `json:"horizon_years,omitempty"`
	MaxLeadTimeDays *int                `json:"max_lead_time_days,omitempty"`
	Note            string              `json:"note,omitempty"`
	Groups          map[string]GroupOps `json:"groups,omitempty"`
}

// GroupOps is what the user entered for one operation group.
type GroupOps struct {
	CostBasis         string              `json:"cost_basis,omitempty"`
	Unit              string              `json:"unit,omitempty"`
	AnnualDemandUnits *float64            `json:"annual_demand_units,omitempty"`
	OrdersPerYear     *float64            `json:"orders_per_year,omitempty"`
	Plants            map[string]PlantOps `json:"plants,omitempty"`
}

// PlantOps is what the user entered for one group at one plant.
type PlantOps struct {
	Ref          *Ref              `json:"ref,omitempty"`
	Labour       *float64          `json:"labour_usd_per_year,omitempty"`
	Logistics    *float64          `json:"logistics_usd_per_year,omitempty"`
	Capex        *float64          `json:"capex_usd,omitempty"`
	Retooling    *float64          `json:"retooling_usd,omitempty"`
	LeadTimeDays *int              `json:"lead_time_days,omitempty"`
	MOQUnits     *float64          `json:"moq_units,omitempty"`
	MOQUnit      string            `json:"moq_unit,omitempty"`
	Sources      map[string]string `json:"sources,omitempty"`
}

// Decode reads exactly one operations block, strictly. Each failure carries a
// slug: unknown_field (a field this API does not have), trailing_data (anything
// after the one JSON value) or invalid_json (syntax, wrong type, out of range
// number). "null" decodes to nil. This is the public pure decoder: callers
// must not assume an outer HTTP decoder has already checked the block.
func Decode(data []byte) (*Operations, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var o *Operations
	if err := dec.Decode(&o); err != nil {
		slug := "invalid_json"
		if strings.Contains(err.Error(), "unknown field") {
			slug = "unknown_field"
		}
		return nil, fmt.Errorf("operations.%s: %w", slug, err)
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("operations.trailing_data: only one JSON value is allowed")
	}
	return o, nil
}

// Active reports whether the block carries any input. nil, {}, an empty groups
// map and entries holding only a ref or sources are inactive (legacy mode).
// The note alone is not an input.
func (o *Operations) Active() bool {
	if o == nil {
		return false
	}
	if o.HorizonYears != nil || o.MaxLeadTimeDays != nil {
		return true
	}
	for _, g := range o.Groups {
		if g.CostBasis != "" || g.Unit != "" || g.AnnualDemandUnits != nil || g.OrdersPerYear != nil {
			return true
		}
		for _, p := range g.Plants {
			if p.Labour != nil || p.Logistics != nil || p.Capex != nil || p.Retooling != nil ||
				p.LeadTimeDays != nil || p.MOQUnits != nil || p.MOQUnit != "" {
				return true
			}
		}
	}
	return false
}

// Normalize returns the block to store: nil when inactive, else a deep copy in
// which every entered number is canonical (-0 becomes 0; an entered 0 stays
// non-nil, an absent field stays nil). The caller's data is never changed.
func Normalize(o *Operations) *Operations {
	if !o.Active() {
		return nil
	}
	return clean(o)
}

// nz canonicalises negative zero. It is the one place that does so.
func nz(f float64) float64 {
	if f == 0 {
		return 0
	}
	return f
}

func nzp(p *float64) *float64 {
	if p == nil {
		return nil
	}
	v := nz(*p)
	return &v
}

func nip(p *int) *int {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func clean(o *Operations) *Operations {
	c := &Operations{HorizonYears: nzp(o.HorizonYears), MaxLeadTimeDays: nip(o.MaxLeadTimeDays), Note: o.Note}
	if o.Groups == nil {
		return c
	}
	c.Groups = make(map[string]GroupOps, len(o.Groups))
	for gid, g := range o.Groups {
		cg := GroupOps{CostBasis: g.CostBasis, Unit: g.Unit, AnnualDemandUnits: nzp(g.AnnualDemandUnits), OrdersPerYear: nzp(g.OrdersPerYear)}
		if g.Plants != nil {
			cg.Plants = make(map[string]PlantOps, len(g.Plants))
			for pid, p := range g.Plants {
				cp := PlantOps{Labour: nzp(p.Labour), Logistics: nzp(p.Logistics), Capex: nzp(p.Capex), Retooling: nzp(p.Retooling),
					LeadTimeDays: nip(p.LeadTimeDays), MOQUnits: nzp(p.MOQUnits), MOQUnit: p.MOQUnit}
				if p.Ref != nil {
					r := *p.Ref
					cp.Ref = &r
				}
				if p.Sources != nil {
					cp.Sources = make(map[string]string, len(p.Sources))
					for k, v := range p.Sources {
						cp.Sources[k] = v
					}
				}
				cg.Plants[pid] = cp
			}
		}
		c.Groups[gid] = cg
	}
	return c
}

func trim(s string) string { return strings.TrimSpace(s) }

func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }

// round2 rounds to cents (half away from zero in float64; not a decimal-exact
// claim) and never returns negative zero.
func round2(f float64) float64 {
	return nz(math.Round(f*100) / 100)
}

func fold(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

// sameRef reports whether an entry's ref still describes the product's plant:
// equal canonical country, and name equal after space and case folding.
func sameRef(entry *Ref, plant Ref) bool {
	return entry != nil && entry.Country == plant.Country && fold(entry.Name) == fold(plant.Name)
}
