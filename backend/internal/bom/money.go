package bom

import (
	"fmt"
	"strconv"
	"strings"
)

// Money is what a BOM's optional money columns say (all optional):
//
//	unit_cost_usd    cost of one of this row's parts (or of the operation) per finished unit
//	qty              parts per finished unit (default 1)
//	annual_units     finished units imported a year (the first non-empty cell)
//	unit_price_usd   import price of one finished unit (the first non-empty cell)
//	hts              the product's HTS subheading (the first non-empty cell)
//
// A group's yearly cost at a plant is the sum of its rows' unit cost × qty ×
// annual units; the yearly import value is annual units × unit price.
type Money struct {
	HTS                  string                        `json:"hts,omitempty"`
	AnnualUnits          float64                       `json:"annual_units,omitempty"`
	UnitPriceUSD         float64                       `json:"unit_price_usd,omitempty"`
	AnnualImportValueUSD float64                       `json:"annual_import_value_usd,omitempty"`
	GroupCosts           map[string]map[string]float64 `json:"group_costs,omitempty"`
}

var moneyAliases = map[string][]string{
	"unit_cost":    {"unit_cost_usd", "unit cost usd", "unit cost (usd)", "unit cost", "unit_cost", "cost per unit", "cost (usd)", "cost"},
	"qty":          {"qty", "quantity", "qty per unit", "quantity per unit"},
	"annual_units": {"annual_units", "annual units", "units per year", "units a year", "annual volume", "yearly units"},
	"unit_price":   {"unit_price_usd", "unit price usd", "unit price (usd)", "unit price", "unit_price", "import price", "fob price"},
	"hts":          {"hts", "hts code", "hts_code", "hts number", "hs code", "tariff code"},
}

// ParseMoney reads the money columns of rows (header first) for the product
// Parse built from the same rows. A sheet without them gives an empty Money
// and no warnings.
func ParseMoney(rows [][]string, opts Options) (Money, []string) {
	var m Money
	for len(rows) > 0 && blank(rows[0]) {
		rows = rows[1:]
	}
	if len(rows) < 2 {
		return m, nil
	}
	col, err := columns(rows[0])
	if err != nil {
		return m, nil
	}
	for i, h := range rows[0] {
		h = strings.ToLower(strings.TrimSpace(h))
		for key, names := range moneyAliases {
			if _, done := col[key]; done {
				continue
			}
			for _, n := range names {
				if h == n {
					col[key] = i
				}
			}
		}
	}
	cell := func(row []string, key string) string {
		if i, ok := col[key]; ok && i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}
	var warnings []string
	type cost struct {
		group, plant string
		perUnit      float64
	}
	var costs []cost
	for n, row := range rows[1:] {
		line := n + 2
		if blank(row) {
			continue
		}
		if v := cell(row, "hts"); v != "" && m.HTS == "" {
			m.HTS = v
		}
		if v := cell(row, "annual_units"); v != "" && m.AnnualUnits == 0 {
			if f, ok := amount(v); ok {
				m.AnnualUnits = f
			} else {
				warnings = append(warnings, fmt.Sprintf("row %d: annual units %q is not a number; ignored", line, v))
			}
		}
		if v := cell(row, "unit_price"); v != "" && m.UnitPriceUSD == 0 {
			if f, ok := amount(v); ok {
				m.UnitPriceUSD = f
			} else {
				warnings = append(warnings, fmt.Sprintf("row %d: unit price %q is not a number; ignored", line, v))
			}
		}
		v := cell(row, "unit_cost")
		if v == "" || strings.ToLower(cell(row, "type")) == "plant" {
			continue
		}
		f, ok := amount(v)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("row %d: unit cost %q is not a number; ignored", line, v))
			continue
		}
		g, ok := matchGroup(cell(row, "group"), opts.Template)
		if !ok || g == "" {
			continue // Parse already warned about the group
		}
		qty := 1.0
		if q := cell(row, "qty"); q != "" {
			if qf, ok := amount(q); ok {
				qty = qf
			}
		}
		plantName := cell(row, "plant")
		if plantName == "" {
			plantName = cell(row, "country")
		}
		costs = append(costs, cost{group: g, plant: plantID(plantName), perUnit: f * qty})
	}
	if m.AnnualUnits > 0 && m.UnitPriceUSD > 0 {
		m.AnnualImportValueUSD = m.AnnualUnits * m.UnitPriceUSD
	}
	if len(costs) > 0 {
		if m.AnnualUnits == 0 {
			warnings = append(warnings, "unit costs need annual_units to become yearly group costs; add an annual_units value")
		} else {
			m.GroupCosts = map[string]map[string]float64{}
			for _, c := range costs {
				if m.GroupCosts[c.group] == nil {
					m.GroupCosts[c.group] = map[string]float64{}
				}
				m.GroupCosts[c.group][c.plant] += c.perUnit * m.AnnualUnits
			}
		}
	}
	return m, warnings
}

// amount reads a non-negative number, allowing "$", "," and spaces.
func amount(s string) (float64, bool) {
	s = strings.NewReplacer("$", "", ",", "", " ", "", "USD", "", "usd", "").Replace(s)
	f, err := strconv.ParseFloat(s, 64)
	return f, err == nil && f >= 0
}
