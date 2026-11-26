// Package erpimport reads bills of materials exported from an ERP (#880) and
// turns them into products, one per assembly, with a row-level error report.
//
// Three CSV layouts are read, all one row per BOM line (a component of an
// assembly), with headers matched case-insensitively:
//
//   - generic: product_id, product_name, product_hts (optional),
//     assembled_in (the country of final assembly), component, made_in,
//     component_hts (optional), description (optional).
//   - sap_s4: an SAP S/4HANA BOM explosion (CS11/CS12 or the
//     API_BILL_OF_MATERIAL_SRV export) joined to the material master:
//     Material, Material Description, Plant Country, Component, Component
//     Description, Country of Origin, Commodity Code; Header Commodity Code
//     optional.
//   - netsuite: a NetSuite saved search on assembly items with member lines:
//     Assembly Item (or Internal ID), Assembly Description, Location
//     Country, Member Item, Member Description, Country of Manufacture,
//     HS Tariff Number (member); Assembly HS Tariff Number optional.
//
// A JSON layout (items with components) serves REST callers. Every layout
// ends in the same Item: an external ID (the ERP's own key for the
// assembly), a name, the product's HTS and a domain.Product whose final
// assembly plant is in the assembly country. Nothing here calls a model.
package erpimport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// Formats.
const (
	Generic  = "generic"
	SAPS4    = "sap_s4"
	NetSuite = "netsuite"
	JSON     = "json"
)

// Formats lists the accepted formats.
var Formats = []string{Generic, SAPS4, NetSuite, JSON}

// Limits.
const (
	MaxRows     = 1000 // BOM lines in one import (the sheet reader stops at 1000 rows)
	MaxItems    = 200  // assemblies in one import
	MaxParts    = 200  // components of one assembly
	maxFieldLen = 2000
	maxIDLen    = 120
)

// Item is one assembly read from the export.
type Item struct {
	// ExternalID is the ERP's key for the assembly (material number, item
	// internal ID or product_id): re-importing the same ID updates the same
	// case.
	ExternalID string         `json:"external_id"`
	Name       string         `json:"name"`
	HTS        string         `json:"hts,omitempty"`
	Product    domain.Product `json:"product"`
	// Rows are the source line numbers (1-based, header = 1) the item came from.
	Rows []int `json:"rows"`
}

// Problem is one thing the caller must fix, tied to a line or an assembly.
type Problem struct {
	Row        int    `json:"row,omitempty"`
	ExternalID string `json:"external_id,omitempty"`
	Message    string `json:"message"`
}

// Result is a parsed import: the items that are complete, and the problems
// that kept rows or items out. One bad assembly never drops another.
type Result struct {
	Format   string    `json:"format"`
	Items    []Item    `json:"items"`
	Problems []Problem `json:"problems"`
	// Lines is how many non-blank BOM lines were read.
	Lines int `json:"lines"`
}

// columns maps the logical fields to each format's header aliases.
var columns = map[string]map[string][]string{
	Generic: {
		"id":          {"product_id", "product id", "assembly_id", "item_id", "sku"},
		"name":        {"product_name", "product name", "assembly", "product"},
		"hts":         {"product_hts", "product hts", "hts", "hs_code"},
		"assembled":   {"assembled_in", "assembled in", "assembly_country", "country_of_assembly", "made_in_country"},
		"description": {"description", "product_description"},
		"part":        {"component", "component_name", "part", "part_name"},
		"made_in":     {"made_in", "component_origin", "origin", "country_of_origin", "coo"},
		"part_hts":    {"component_hts", "part_hts", "component hts"},
	},
	SAPS4: {
		"id":          {"material", "material number", "header material", "billofmaterialmaterial", "matnr"},
		"name":        {"material description", "header description", "materialdescription", "maktx"},
		"hts":         {"header commodity code", "header comm./imp. code no.", "material commodity code"},
		"assembled":   {"plant country", "plantcountry", "country of plant", "plant country key"},
		"description": {"bom header text", "billofmaterialheadertext"},
		"part":        {"component description", "componentdescription", "object description", "component text"},
		"part_id":     {"component", "component number", "billofmaterialcomponent", "idnrk"},
		"made_in":     {"country of origin", "countryoforigin", "ctry of origin", "herkl"},
		"part_hts":    {"commodity code", "comm./imp. code no.", "importexportcommoditycode", "stawn"},
	},
	NetSuite: {
		"id":          {"assembly item", "assembly internal id", "internal id", "assembly"},
		"name":        {"assembly description", "assembly display name", "display name"},
		"hts":         {"assembly hs tariff number", "assembly harmonized code"},
		"assembled":   {"location country", "location : country", "build location country"},
		"description": {"assembly purchase description", "assembly sales description"},
		"part":        {"member description", "member item : description", "member display name"},
		"part_id":     {"member item", "member item : name", "member"},
		"made_in":     {"country of manufacture", "member item : country of manufacture", "member country of manufacture"},
		"part_hts":    {"hs tariff number", "member item : hs tariff number", "harmonized code", "member hs tariff number"},
	},
}

var required = map[string][]string{
	Generic:  {"id", "name", "assembled", "part", "made_in"},
	SAPS4:    {"id", "name", "assembled", "made_in"},
	NetSuite: {"id", "name", "assembled", "made_in"},
}

// htsish accepts a heading to a 10-digit code with dots or spaces.
var htsish = regexp.MustCompile(`^[0-9]{4}(?:[. ]?[0-9]{2}){0,3}$`)

// cleanHTS normalises an HTS/commodity code from an ERP ("8504409580",
// "8504.40.9580", "8504 40 95") to the dotted form the rule check reads; ok
// is false when it is not a code.
func cleanHTS(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", true
	}
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		if r == '.' || r == ' ' || r == '-' {
			return -1
		}
		return 'x'
	}, s)
	if strings.Contains(digits, "x") || len(digits) < 4 || len(digits) > 10 || len(digits)%2 != 0 {
		return "", false
	}
	out := digits[:4]
	if len(digits) > 4 {
		out += "." + digits[4:6]
	}
	if len(digits) > 6 {
		out += "." + digits[6:]
	}
	if !htsish.MatchString(out) {
		return "", false
	}
	return out, true
}

// Parse reads rows (header first) in format.
func Parse(format string, rows [][]string) (Result, error) {
	res := Result{Format: format, Items: []Item{}, Problems: []Problem{}}
	cols, ok := columns[format]
	if !ok {
		return res, fmt.Errorf("unknown format %q; use one of %s", format, strings.Join(Formats, ", "))
	}
	for len(rows) > 0 && blank(rows[0]) {
		rows = rows[1:]
	}
	if len(rows) < 2 {
		return res, fmt.Errorf("the file needs a header row and at least one BOM line")
	}
	if len(rows)-1 > MaxRows {
		return res, fmt.Errorf("%d BOM lines; the limit is %d", len(rows)-1, MaxRows)
	}
	idx := map[string]int{}
	for i, h := range rows[0] {
		h = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")))
		for key, names := range cols {
			if _, done := idx[key]; done {
				continue
			}
			for _, n := range names {
				if h == n {
					idx[key] = i
				}
			}
		}
	}
	var missing []string
	for _, k := range required[format] {
		if _, ok := idx[k]; !ok {
			missing = append(missing, cols[k][0])
		}
	}
	if format != Generic {
		if _, ok1 := idx["part"]; !ok1 {
			if _, ok2 := idx["part_id"]; !ok2 {
				missing = append(missing, cols["part"][0]+" or "+cols["part_id"][0])
			}
		}
	}
	if len(missing) > 0 {
		return res, fmt.Errorf("the header row needs columns %s (got %q)", strings.Join(missing, ", "), rows[0])
	}
	cell := func(row []string, key string) string {
		if i, ok := idx[key]; ok && i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}

	type draft struct {
		item     Item
		country  string
		bad      bool
		partSeen map[string]bool
	}
	drafts := map[string]*draft{}
	var order []string
	for n, row := range rows[1:] {
		line := n + 2
		if blank(row) {
			continue
		}
		res.Lines++
		if long := tooLong(row); long >= 0 {
			res.Problems = append(res.Problems, Problem{Row: line, Message: fmt.Sprintf("column %d is longer than %d characters", long+1, maxFieldLen)})
			continue
		}
		id := cell(row, "id")
		if id == "" {
			res.Problems = append(res.Problems, Problem{Row: line, Message: "the assembly key (" + cols["id"][0] + ") is empty"})
			continue
		}
		if len(id) > maxIDLen {
			res.Problems = append(res.Problems, Problem{Row: line, Message: fmt.Sprintf("the assembly key is longer than %d characters", maxIDLen)})
			continue
		}
		d, ok := drafts[id]
		if !ok {
			if len(drafts) == MaxItems {
				res.Problems = append(res.Problems, Problem{Row: line, ExternalID: id, Message: fmt.Sprintf("more than %d assemblies; the rest are not read", MaxItems)})
				break
			}
			d = &draft{item: Item{ExternalID: id}, partSeen: map[string]bool{}}
			drafts[id] = d
			order = append(order, id)
		}
		d.item.Rows = append(d.item.Rows, line)
		add := func(msg string) {
			res.Problems = append(res.Problems, Problem{Row: line, ExternalID: id, Message: msg})
			d.bad = true
		}
		// Header fields may repeat on every line; the first non-empty value
		// wins and a different later value is an error.
		for _, f := range []struct {
			key  string
			dst  *string
			norm func(string) string
		}{
			{"name", &d.item.Name, nil},
			{"description", &d.item.Product.Description, nil},
			{"assembled", &d.country, nil},
			{"hts", &d.item.HTS, nil},
		} {
			v := cell(row, f.key)
			if v == "" {
				continue
			}
			if f.key == "assembled" {
				c, ok := country(v)
				if !ok {
					add(fmt.Sprintf("country %q is not a known country code", v))
					continue
				}
				v = c
			}
			if f.key == "hts" {
				h, ok := cleanHTS(v)
				if !ok {
					add(fmt.Sprintf("product code %q is not an HTS number", v))
					continue
				}
				v = h
			}
			if f.norm != nil {
				v = f.norm(v)
			}
			switch {
			case *f.dst == "":
				*f.dst = v
			case *f.dst != v && f.key != "description":
				add(fmt.Sprintf("%s is %q here but %q on an earlier line of the same assembly", cols[f.key][0], v, *f.dst))
			}
		}
		part := cell(row, "part")
		if part == "" {
			part = cell(row, "part_id")
		}
		if part == "" {
			add("the component name is empty")
			continue
		}
		madeIn, ok := country(cell(row, "made_in"))
		if !ok {
			add(fmt.Sprintf("component %q: country %q is not a known country code", part, cell(row, "made_in")))
			continue
		}
		if madeIn == "" {
			add(fmt.Sprintf("component %q has no country of origin", part))
			continue
		}
		hts, ok2 := cleanHTS(cell(row, "part_hts"))
		ok = ok2
		if !ok {
			add(fmt.Sprintf("component %q: code %q is not an HTS number", part, cell(row, "part_hts")))
			continue
		}
		key := strings.ToLower(part) + "\x00" + madeIn
		if d.partSeen[key] {
			continue // the same part from the same country on two BOM lines (e.g. two positions): listed once
		}
		d.partSeen[key] = true
		if len(d.item.Product.Components) == MaxParts {
			add(fmt.Sprintf("more than %d components", MaxParts))
			continue
		}
		d.item.Product.Components = append(d.item.Product.Components, domain.Component{Name: part, MadeIn: madeIn, HTS: hts})
	}
	for _, id := range order {
		d := drafts[id]
		if d.bad {
			continue
		}
		it := d.item
		if it.Name == "" {
			res.Problems = append(res.Problems, Problem{Row: it.Rows[0], ExternalID: id, Message: "the assembly has no name"})
			continue
		}
		if d.country == "" {
			res.Problems = append(res.Problems, Problem{Row: it.Rows[0], ExternalID: id, Message: "the country of final assembly is empty"})
			continue
		}
		it.Product = build(it.Name, it.Product.Description, it.HTS, d.country, it.Product.Components)
		if err := it.Product.Validate(); err != nil {
			res.Problems = append(res.Problems, Problem{Row: it.Rows[0], ExternalID: id, Message: err.Error()})
			continue
		}
		res.Items = append(res.Items, it)
	}
	return res, nil
}

// assemblyPlant is the ID of the final-assembly plant every import gets.
const assemblyPlant = "final_assembly"

// build makes the product: one plant in the assembly country, with the
// final assembly there; components made in that country are made at it.
func build(name, desc, hts, country string, parts []domain.Component) domain.Product {
	p := domain.Product{Name: name, Description: desc, HTS: hts,
		Plants:     []domain.Plant{{ID: assemblyPlant, Name: "Final assembly", Country: country}},
		Components: make([]domain.Component, len(parts)),
		Operations: []domain.Operation{{Name: "Final assembly", Plant: assemblyPlant}},
	}
	copy(p.Components, parts)
	return p
}

// JSONItem is one assembly in the REST (json) format.
type JSONItem struct {
	ExternalID  string `json:"external_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	HTS         string `json:"hts"`
	AssembledIn string `json:"assembled_in"`
	Components  []struct {
		Name   string `json:"name"`
		MadeIn string `json:"made_in"`
		HTS    string `json:"hts"`
	} `json:"components"`
}

// FromJSON checks REST items like the CSV layouts. Item problems carry the
// item's index as Row (0-based index + 1).
func FromJSON(items []JSONItem) Result {
	res := Result{Format: JSON, Items: []Item{}, Problems: []Problem{}}
	seen := map[string]bool{}
	for i, in := range items {
		n := i + 1
		res.Lines += len(in.Components)
		bad := func(msg string) {
			res.Problems = append(res.Problems, Problem{Row: n, ExternalID: in.ExternalID, Message: msg})
		}
		id := strings.TrimSpace(in.ExternalID)
		switch {
		case n > MaxItems:
			bad(fmt.Sprintf("more than %d items", MaxItems))
			continue
		case id == "" || len(id) > maxIDLen:
			bad(fmt.Sprintf("external_id is required, at most %d characters", maxIDLen))
			continue
		case seen[id]:
			bad("external_id appears twice")
			continue
		case strings.TrimSpace(in.Name) == "":
			bad("name is required")
			continue
		case jsonCountry(in.AssembledIn) == "":
			bad("assembled_in is required")
			continue
		case len(in.Components) == 0 || len(in.Components) > MaxParts:
			bad(fmt.Sprintf("1 to %d components are required", MaxParts))
			continue
		}
		seen[id] = true
		hts, ok := cleanHTS(in.HTS)
		if !ok {
			bad(fmt.Sprintf("hts %q is not an HTS number", in.HTS))
			continue
		}
		var parts []domain.Component
		okAll := true
		for j, c := range in.Components {
			ch, ok := cleanHTS(c.HTS)
			mi := jsonCountry(c.MadeIn)
			if strings.TrimSpace(c.Name) == "" || mi == "" || !ok {
				bad(fmt.Sprintf("components[%d] needs a name, made_in and a valid hts if given", j))
				okAll = false
				break
			}
			parts = append(parts, domain.Component{Name: strings.TrimSpace(c.Name), MadeIn: mi, HTS: ch})
		}
		if !okAll {
			continue
		}
		it := Item{ExternalID: id, Name: strings.TrimSpace(in.Name), HTS: hts, Rows: []int{n},
			Product: build(strings.TrimSpace(in.Name), strings.TrimSpace(in.Description), hts, jsonCountry(in.AssembledIn), parts)}
		if err := it.Product.Validate(); err != nil {
			bad(err.Error())
			continue
		}
		res.Items = append(res.Items, it)
	}
	return res
}

func jsonCountry(s string) string {
	c, ok := country(s)
	if !ok {
		return ""
	}
	return c
}

// Facts renders the product as the plain facts the engine reads, for an
// import with no family template: what it is, where it is assembled, and
// where each component is made.
func Facts(p domain.Product) string {
	var b strings.Builder
	assembled := ""
	if len(p.Plants) > 0 {
		assembled = p.Plants[0].Country
	}
	fmt.Fprintf(&b, "The merchandise under consideration is %s.", p.Name)
	if d := strings.TrimSpace(p.Description); d != "" {
		fmt.Fprintf(&b, " %s", d)
	}
	if assembled != "" {
		fmt.Fprintf(&b, " It is assembled in %s.", assembled)
	}
	b.WriteString("\n\n")
	byCountry, order := map[string][]string{}, []string{}
	for _, c := range p.Components {
		if _, ok := byCountry[c.MadeIn]; !ok {
			order = append(order, c.MadeIn)
		}
		byCountry[c.MadeIn] = append(byCountry[c.MadeIn], c.Name)
	}
	for _, country := range order {
		fmt.Fprintf(&b, "Made in %s: %s. ", country, strings.Join(byCountry[country], ", "))
	}
	return strings.TrimSpace(b.String()) + "\n"
}

// Fingerprint is a stable hash of the product, so a re-import can tell an
// unchanged assembly from a changed one.
func Fingerprint(p domain.Product) string {
	q := p
	q.Components = append([]domain.Component(nil), p.Components...)
	sort.SliceStable(q.Components, func(i, j int) bool {
		a, b := q.Components[i], q.Components[j]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.MadeIn < b.MadeIn
	})
	raw, _ := json.Marshal(q)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

func blank(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func tooLong(row []string) int {
	for i, c := range row {
		if len(c) > maxFieldLen {
			return i
		}
	}
	return -1
}
