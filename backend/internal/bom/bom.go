// Package bom turns an uploaded bill of materials (CSV or Excel) into the
// structured product the engine and planner use.
//
// One row per component (or per operation, with type "operation"). Headers are
// matched case-insensitively, with aliases:
//
//	name                 component / part / item / operation name (required)
//	country              made_in / origin / country of origin: where the component is made,
//	                     or where the plant is for an operation (required)
//	plant                plant / factory / site / supplier (optional; defaults to the country)
//	group                operation group / process group (optional; matched to the family template)
//	type                 component (default) or operation
//	plant_capabilities   groups this row's plant could also perform, separated by ";" (optional)
package bom

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

// MaxRows caps a BOM so an upload cannot be unbounded.
const MaxRows = 500

// ErrInvalid wraps domain.ErrInvalid for BOM problems the user must fix.
var ErrInvalid = fmt.Errorf("bom: %w", domain.ErrInvalid)

// ReadRows reads the first sheet of an Excel file (.xlsx) or a CSV file. The
// format is detected from the content (an .xlsx file is a zip archive).
func ReadRows(r io.Reader) ([][]string, error) {
	b, err := io.ReadAll(io.LimitReader(r, 5<<20))
	if err != nil {
		return nil, fmt.Errorf("bom: read: %w", err)
	}
	if bytes.HasPrefix(b, []byte("PK\x03\x04")) {
		f, err := excelize.OpenReader(bytes.NewReader(b))
		if err != nil {
			return nil, fmt.Errorf("%w: not a readable Excel file: %v", ErrInvalid, err)
		}
		defer f.Close()
		sheets := f.GetSheetList()
		if len(sheets) == 0 {
			return nil, fmt.Errorf("%w: the Excel file has no sheets", ErrInvalid)
		}
		rows, err := f.GetRows(sheets[0])
		if err != nil {
			return nil, fmt.Errorf("%w: read sheet %q: %v", ErrInvalid, sheets[0], err)
		}
		return rows, nil
	}
	cr := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")))) // Excel's CSV export adds a BOM
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%w: not a readable CSV file: %v", ErrInvalid, err)
	}
	return rows, nil
}

var aliases = map[string][]string{
	"name":         {"name", "component", "component name", "part", "part name", "item", "operation", "operation name", "description"},
	"country":      {"country", "made in", "made_in", "origin", "country of origin", "coo"},
	"plant":        {"plant", "factory", "site", "supplier", "plant name"},
	"group":        {"group", "operation group", "operation_group", "process group", "process"},
	"type":         {"type", "row type", "kind"},
	"capabilities": {"plant capabilities", "plant_capabilities", "capabilities", "can also make"},
}

func columns(header []string) (map[string]int, error) {
	idx := map[string]int{}
	for i, h := range header {
		h = strings.ToLower(strings.TrimSpace(h))
		for key, names := range aliases {
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
	for _, k := range []string{"name", "country"} {
		if _, ok := idx[k]; !ok {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%w: header row needs columns %s (got %q)", ErrInvalid, strings.Join(missing, " and "), header)
	}
	return idx, nil
}

var nonID = regexp.MustCompile(`[^a-z0-9]+`)

func plantID(name string) string {
	id := strings.Trim(nonID.ReplaceAllString(strings.ToLower(name), "_"), "_")
	if id == "" {
		id = "plant"
	}
	return id
}

// Options are the product fields the sheet does not carry.
type Options struct {
	Name        string
	Description string
	Family      string
	// Template, when set, maps group cells to template group IDs (by ID or
	// name) and reports which groups the plan search needs.
	Template *planner.Template
}

// Parse builds a product from rows (header first). Warnings explain what the
// plan search will lack; errors are problems the user must fix.
func Parse(rows [][]string, opts Options) (domain.Product, []string, error) {
	var warnings []string
	p := domain.Product{Name: strings.TrimSpace(opts.Name), Family: opts.Family, Description: strings.TrimSpace(opts.Description),
		Plants: []domain.Plant{}, Components: []domain.Component{}, Operations: []domain.Operation{}}
	for len(rows) > 0 && blank(rows[0]) {
		rows = rows[1:]
	}
	if len(rows) < 2 {
		return p, nil, fmt.Errorf("%w: the sheet needs a header row and at least one component row", ErrInvalid)
	}
	if len(rows)-1 > MaxRows {
		return p, nil, fmt.Errorf("%w: %d rows; the limit is %d", ErrInvalid, len(rows)-1, MaxRows)
	}
	col, err := columns(rows[0])
	if err != nil {
		return p, nil, err
	}
	cell := func(row []string, key string) string {
		if i, ok := col[key]; ok && i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}
	plants := map[string]*domain.Plant{} // by id
	var order []string
	caps := map[string]map[string]bool{}
	var problems []string
	for n, row := range rows[1:] {
		line := n + 2
		if blank(row) {
			continue
		}
		name, country := cell(row, "name"), domain.NormalizeCountry(cell(row, "country"))
		if name == "" || country == "" {
			problems = append(problems, fmt.Sprintf("row %d: name and country are required", line))
			continue
		}
		plantName := cell(row, "plant")
		if plantName == "" {
			plantName = country
		}
		id := plantID(plantName)
		pl, ok := plants[id]
		switch {
		case !ok:
			pl = &domain.Plant{ID: id, Name: plantName, Country: country}
			plants[id], caps[id] = pl, map[string]bool{}
			order = append(order, id)
		case pl.Country != country:
			problems = append(problems, fmt.Sprintf("row %d: plant %q is in %s on an earlier row, not %s", line, plantName, pl.Country, country))
			continue
		}
		group := ""
		if g := cell(row, "group"); g != "" {
			if group, ok = matchGroup(g, opts.Template); !ok {
				warnings = append(warnings, fmt.Sprintf("row %d: group %q is not in the %s template; the row is kept without a group", line, g, opts.Family))
			}
		}
		if group != "" {
			caps[id][group] = true
		}
		for _, c := range strings.Split(cell(row, "capabilities"), ";") {
			if c = strings.TrimSpace(c); c == "" {
				continue
			}
			if g, ok := matchGroup(c, opts.Template); ok {
				caps[id][g] = true
			} else {
				warnings = append(warnings, fmt.Sprintf("row %d: capability %q is not in the %s template; ignored", line, c, opts.Family))
			}
		}
		if strings.EqualFold(cell(row, "type"), "operation") {
			p.Operations = append(p.Operations, domain.Operation{Name: name, Plant: id, Group: group})
		} else {
			p.Components = append(p.Components, domain.Component{Name: name, MadeIn: country, Plant: id, Group: group})
		}
	}
	if len(problems) > 0 {
		return p, warnings, fmt.Errorf("%w: %s", ErrInvalid, strings.Join(problems, "; "))
	}
	for _, id := range order {
		pl := plants[id]
		for g := range caps[id] {
			pl.Capabilities = append(pl.Capabilities, g)
		}
		sort.Strings(pl.Capabilities)
		p.Plants = append(p.Plants, *pl)
	}
	warnings = append(warnings, planWarnings(p, opts.Template)...)
	if err := p.Validate(); err != nil {
		return p, warnings, errors.Join(ErrInvalid, err)
	}
	return p, warnings, nil
}

// matchGroup maps a cell to a template group ID by ID or name. Without a
// template any non-empty value is kept as given.
func matchGroup(v string, t *planner.Template) (string, bool) {
	if t == nil {
		return v, true
	}
	key := plantID(v)
	for _, g := range t.Groups {
		if plantID(g.ID) == key || plantID(g.Name) == key {
			return g.ID, true
		}
	}
	return "", false
}

// planWarnings says what the plan search will be unable to do with p.
func planWarnings(p domain.Product, t *planner.Template) []string {
	if t == nil {
		return []string{"no product family given: the product can be analysed, but the plan search needs a family with a template"}
	}
	var w []string
	grouped := 0
	for _, c := range p.Components {
		if c.Group != "" {
			grouped++
		}
	}
	if grouped == 0 {
		w = append(w, "no component has an operation group: the plan search cannot move anything; add a group column")
	}
	if len(p.Plants) < 2 {
		w = append(w, "only one plant: the plan search needs at least two plants to move operations between")
	}
	extra := false
	for _, pl := range p.Plants {
		for _, g := range pl.Capabilities {
			if !doesNow(p, pl.ID, g) {
				extra = true
			}
		}
	}
	if !extra {
		w = append(w, "no plant lists a capability beyond what it makes now: add a plant_capabilities column (e.g. \"motor_fan_assembly; pcba\") so the plan search has moves to try")
	}
	return w
}

func doesNow(p domain.Product, plant, group string) bool {
	for _, c := range p.Components {
		if c.Plant == plant && c.Group == group {
			return true
		}
	}
	for _, o := range p.Operations {
		if o.Plant == plant && o.Group == group {
			return true
		}
	}
	return false
}

func blank(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}
