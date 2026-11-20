package opcost

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// Basis is the canonical form of the operations inputs that change a result:
// the numbers, the cost basis, the units and the plant refs. The note and the
// sources are provenance text, not inputs, and are left out.
type Basis struct {
	HorizonYears    *float64              `json:"h,omitempty"`
	MaxLeadTimeDays *int                  `json:"l,omitempty"`
	Groups          map[string]BasisGroup `json:"g,omitempty"`
}

// BasisGroup is a group in a Basis.
type BasisGroup struct {
	CostBasis string                `json:"b,omitempty"`
	Unit      string                `json:"u,omitempty"`
	Demand    *float64              `json:"d,omitempty"`
	Orders    *float64              `json:"o,omitempty"`
	Plants    map[string]BasisPlant `json:"p,omitempty"`
}

// BasisPlant is a plant entry in a Basis.
type BasisPlant struct {
	Ref       *Ref     `json:"r,omitempty"`
	Labour    *float64 `json:"a,omitempty"`
	Logistics *float64 `json:"c,omitempty"`
	Capex     *float64 `json:"x,omitempty"`
	Retooling *float64 `json:"t,omitempty"`
	Lead      *int     `json:"e,omitempty"`
	MOQ       *float64 `json:"m,omitempty"`
	MOQUnit   string   `json:"n,omitempty"`
}

// CanonicalOps returns the basis of an operations block, nil when inactive,
// so a case without operations has the same basis bytes as before this field.
func CanonicalOps(o *Operations) *Basis {
	if !o.Active() {
		return nil
	}
	b := &Basis{HorizonYears: o.HorizonYears, MaxLeadTimeDays: o.MaxLeadTimeDays}
	for gid, g := range o.Groups {
		bg := BasisGroup{CostBasis: g.CostBasis, Unit: trim(g.Unit), Demand: g.AnnualDemandUnits, Orders: g.OrdersPerYear}
		for pid, p := range g.Plants {
			bp := BasisPlant{Labour: p.Labour, Logistics: p.Logistics, Capex: p.Capex, Retooling: p.Retooling,
				Lead: p.LeadTimeDays, MOQ: p.MOQUnits, MOQUnit: trim(p.MOQUnit)}
			if p.Ref != nil {
				bp.Ref = &Ref{Name: fold(p.Ref.Name), Country: p.Ref.Country}
			}
			if bp == (BasisPlant{}) {
				continue
			}
			if bg.Plants == nil {
				bg.Plants = map[string]BasisPlant{}
			}
			bg.Plants[pid] = bp
		}
		if b.Groups == nil {
			b.Groups = map[string]BasisGroup{}
		}
		b.Groups[gid] = bg
	}
	return b
}

// MoneyBasis is the RAW money a result is priced from: what the user entered,
// before the tariff table or the live lookup fills any rate. Derived notes and
// provenance lists are not part of it.
type MoneyBasis struct {
	Illustrative         bool
	AnnualImportValueUSD float64
	DutyRates            map[string]float64
	GroupCosts           map[string]map[string]float64
	Operations           *Operations
}

// BasisHash is the one definition of "the money inputs" for the plan stamp.
// Take it from the raw economics before pricing; the stored case holds the
// same raw economics, so a fresh hash later is unchanged until the user edits
// a cost or constraint.
func BasisHash(m MoneyBasis) string {
	var rates map[string]float64
	if len(m.DutyRates) > 0 {
		rates = m.DutyRates
	}
	var groups map[string]map[string]float64
	keys := make([]string, 0, len(m.GroupCosts))
	for g := range m.GroupCosts {
		keys = append(keys, g)
	}
	sort.Strings(keys)
	for _, g := range keys {
		if len(m.GroupCosts[g]) == 0 {
			continue
		}
		if groups == nil {
			groups = map[string]map[string]float64{}
		}
		groups[g] = m.GroupCosts[g]
	}
	b, err := json.Marshal(struct {
		Illustrative bool
		Value        float64
		Rates        map[string]float64
		Groups       map[string]map[string]float64
		Ops          *Basis
	}{m.Illustrative, m.AnnualImportValueUSD, rates, groups, CanonicalOps(m.Operations)})
	if err != nil { // non-finite input: validation refuses it; never hash a constant for it
		return "unhashable"
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
