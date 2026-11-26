package atiga

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

//go:embed data/psr_hs2022.json
var psrJSON []byte

// Range is an "except from" exclusion at Level ("heading" or
// "subheading"), written as in the Annex ("7213", "7219.31").
type Range struct {
	Level string `json:"level"`
	From  string `json:"from"`
	To    string `json:"to"`
}

// Requirement is one condition of an alternative. Kind is "rvc"
// (Threshold), "ctc" (Level, Except), "process" (Process) or "wo".
type Requirement struct {
	Kind      string  `json:"kind"`
	Threshold int     `json:"threshold,omitempty"`
	Level     string  `json:"level,omitempty"`
	Except    []Range `json:"except,omitempty"`
	Process   string  `json:"process,omitempty"`
}

// Alternative is one way to meet a rule; every requirement must hold
// ("RVC35+CTSH", "CC and cut and sewn in any AMS").
type Alternative struct {
	Name     string        `json:"name"`
	Requires []Requirement `json:"requires"`
}

// Variant is one branch of a row whose rule depends on the product ("Of
// steel", "For products from 7208").
type Variant struct {
	ID           string        `json:"id"`
	Label        string        `json:"label"`
	Alternatives []Alternative `json:"alternatives"`
}

// Rule is one row of Annex 3, kept verbatim with the rule read from it.
type Rule struct {
	Row          int           `json:"row"`
	Code         string        `json:"code"`
	Description  string        `json:"description"`
	Concise      string        `json:"concise"`
	Full         string        `json:"full"`
	Alternatives []Alternative `json:"alternatives,omitempty"`
	Variants     []Variant     `json:"variants,omitempty"`
}

// Table is the Annex 3 rule table.
type Table struct {
	Version string          `json:"version"`
	Source  json.RawMessage `json:"source"`
	Rules   []Rule          `json:"rules"`

	bySub     map[string]*Rule
	byHeading map[string][]*Rule
}

var (
	defaultOnce  sync.Once
	defaultTable *Table
)

// Default is the embedded table.
func Default() *Table {
	defaultOnce.Do(func() {
		t, err := Parse(psrJSON)
		if err != nil {
			panic(err)
		}
		defaultTable = t
	})
	return defaultTable
}

// Parse reads a rule table.
func Parse(b []byte) (*Table, error) {
	var t Table
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("atiga: %w", err)
	}
	t.bySub = map[string]*Rule{}
	t.byHeading = map[string][]*Rule{}
	for i := range t.Rules {
		r := &t.Rules[i]
		d := digits(r.Code)
		if len(d) != 6 {
			return nil, fmt.Errorf("atiga: row %d has code %q", r.Row, r.Code)
		}
		if len(r.Alternatives) == 0 && len(r.Variants) == 0 {
			return nil, fmt.Errorf("atiga: row %d (%s) has no rule", r.Row, r.Code)
		}
		t.bySub[d] = r
		t.byHeading[d[:4]] = append(t.byHeading[d[:4]], r)
	}
	return &t, nil
}

// GeneralRule is Art. 28.1: RVC40 or CTH, for goods not listed in Annex 3.
var GeneralRule = Rule{
	Code:        "Art. 28.1",
	Description: "Goods not listed in Annex 3",
	Concise:     "RVC40 or CTH",
	Full:        "A regional value content of not less than forty percent (40%) calculated using the formula set out in Article 29; or all non-originating materials have undergone a change in tariff classification at four-digit level (a change in tariff heading)",
	Alternatives: []Alternative{
		{Name: "RVC40", Requires: []Requirement{{Kind: "rvc", Threshold: 40}}},
		{Name: "CTH", Requires: []Requirement{{Kind: "ctc", Level: "heading"}}},
	},
}

// Lookup finds the rule for an HS code. listed says whether it came from
// Annex 3 (false: the Art. 28.1 general rule). When the code is too short
// to tell, ok is false and need says what is missing.
func (t *Table) Lookup(hs string) (r *Rule, listed, ok bool, need string) {
	d := digits(hs)
	if len(d) < 4 {
		return nil, false, false, "give at least the 4-digit heading"
	}
	if len(d) >= 6 {
		if r, found := t.bySub[d[:6]]; found {
			return r, true, true, ""
		}
		return &GeneralRule, false, true, ""
	}
	if rows := t.byHeading[d[:4]]; len(rows) > 0 {
		return nil, false, false, fmt.Sprintf("heading %s has subheadings with their own Annex 3 rule; give the 6-digit subheading", dotted(d[:4]))
	}
	return &GeneralRule, false, true, ""
}

func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// dotted writes 2, 4 or 6 digits the way the Annex does ("72", "72.08",
// "7219.31").
func dotted(d string) string {
	switch len(d) {
	case 4:
		return d[:2] + "." + d[2:]
	case 6:
		return d[:4] + "." + d[4:]
	}
	return d
}
