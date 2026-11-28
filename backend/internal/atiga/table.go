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

// Range is a span of chapters, headings or subheadings at Level ("chapter",
// "heading" or "subheading"), written as digits ("72", "7213", "721931"). It
// is an "except from" exclusion on a change of tariff classification, or the
// scope of a condition on materials. Partial says only part of the span is
// meant (the text names a product within it), so a material in it cannot be
// told from its code.
type Range struct {
	Level   string `json:"level"`
	From    string `json:"from"`
	To      string `json:"to"`
	Partial string `json:"partial,omitempty"`
}

// Requirement is one condition of an alternative. Kind is "rvc"
// (Threshold), "ctc" (Level, Except), "process" (Process), "wo" (wholly
// obtained in the exporting Party), "wo_any" (wholly obtained in any Party),
// "materials" (Scope, Origin: the materials classified there must be
// wholly obtained or originating in a Party) or "value_cap" (Scope,
// Percent: the non-originating materials classified there stay within a
// share of the FOB value).
type Requirement struct {
	Kind      string  `json:"kind"`
	Threshold int     `json:"threshold,omitempty"`
	Level     string  `json:"level,omitempty"`
	Except    []Range `json:"except,omitempty"`
	Process   string  `json:"process,omitempty"`
	Scope     []Range `json:"scope,omitempty"`
	// Origin is wo_any, wo_exporting, orig_any or orig_exporting.
	Origin  string  `json:"origin,omitempty"`
	Percent float64 `json:"percent,omitempty"`
	// Within: the change may be inside the subheading (national lines),
	// which six digits cannot show.
	Within bool `json:"within,omitempty"`
	// NoDeMinimisSameSubheading: de minimis may not cover a material of the
	// good's own subheading.
	NoDeMinimisSameSubheading bool `json:"no_de_minimis_same_subheading,omitempty"`
	// NotOnlyPackaging: a change that comes only from packaging or
	// presenting goods for sale does not count.
	NotOnlyPackaging bool `json:"not_only_packaging,omitempty"`
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
	// Note is a sentence of the rule that the alternatives do not carry.
	Note string `json:"note,omitempty"`
}

// Table is a product-specific rule table.
type Table struct {
	Version string          `json:"version"`
	Source  json.RawMessage `json:"source"`
	Rules   []Rule          `json:"rules"`

	profile *Profile
	bySub   map[string]*Rule
	byHead  map[string]*Rule   // rows written at heading level
	byChap  map[string]*Rule   // rows written at chapter level
	subsOf  map[string][]*Rule // sub-heading rows under a heading
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

// Parse reads an ATIGA rule table.
func Parse(b []byte) (*Table, error) { return ParseFor(ATIGAProfile, b) }

// ParseFor reads a rule table for the agreement of p.
func ParseFor(p *Profile, b []byte) (*Table, error) {
	var t Table
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("atiga: %w", err)
	}
	t.profile = p
	for i := range t.Rules {
		r := &t.Rules[i]
		padAlternatives(r.Alternatives)
		for j := range r.Variants {
			padAlternatives(r.Variants[j].Alternatives)
		}
	}
	t.bySub = map[string]*Rule{}
	t.byHead = map[string]*Rule{}
	t.byChap = map[string]*Rule{}
	t.subsOf = map[string][]*Rule{}
	for i := range t.Rules {
		r := &t.Rules[i]
		d := digits(r.Code)
		switch {
		case len(d) == 6:
			if _, dup := t.bySub[d]; dup {
				return nil, fmt.Errorf("atiga: row %d repeats code %q", r.Row, r.Code)
			}
			t.bySub[d] = r
			t.subsOf[d[:4]] = append(t.subsOf[d[:4]], r)
		case len(d) == 4 && p.AllowShortCodes:
			t.byHead[d] = r
		case len(d) == 2 && p.AllowShortCodes:
			t.byChap[d] = r
		default:
			return nil, fmt.Errorf("atiga: row %d has code %q", r.Row, r.Code)
		}
		if len(r.Alternatives) == 0 && len(r.Variants) == 0 {
			return nil, fmt.Errorf("atiga: row %d (%s) has no rule", r.Row, r.Code)
		}
	}
	return &t, nil
}

// padAlternatives writes every range bound at the width of its level. A
// proviso on "Chapter 8" arrives as "8", which text comparison with the
// two-digit "08" of a material's code would never place in the range.
func padAlternatives(alts []Alternative) {
	for i := range alts {
		for j := range alts[i].Requires {
			q := &alts[i].Requires[j]
			for k := range q.Except {
				padRange(&q.Except[k])
			}
			for k := range q.Scope {
				padRange(&q.Scope[k])
			}
		}
	}
}

func padRange(g *Range) {
	w := levelDigits[g.Level]
	if w == 0 {
		return
	}
	pad := func(s string) string {
		d := digits(s)
		for len(d) < w {
			d = "0" + d
		}
		return d
	}
	g.From, g.To = pad(g.From), pad(g.To)
}

// Profile is the agreement the table is read for.
func (t *Table) Profile() *Profile {
	if t.profile == nil {
		return ATIGAProfile
	}
	return t.profile
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
// the table (false: the agreement's general rule). A rule written at heading
// or chapter level covers every subheading under it that has no row of its
// own. When the code is too short to tell, ok is false and need says what is
// missing.
func (t *Table) Lookup(hs string) (r *Rule, listed, ok bool, need string) {
	p := t.Profile()
	d := digits(hs)
	if len(d) < 4 {
		return nil, false, false, "give at least the 4-digit heading"
	}
	if len(d) >= 6 {
		if r, found := t.bySub[d[:6]]; found {
			return r, true, true, ""
		}
	} else if rows := t.subsOf[d[:4]]; len(rows) > 0 {
		if r, found := t.byHead[d[:4]]; found {
			return r, true, true, ""
		}
		return nil, false, false, fmt.Sprintf("heading %s has subheadings with their own %s rule; give the 6-digit subheading", dotted(d[:4]), p.Cites.Annex)
	}
	if r, found := t.byHead[d[:4]]; found {
		return r, true, true, ""
	}
	if r, found := t.byChap[d[:2]]; found {
		return r, true, true, ""
	}
	return p.General(d), false, true, ""
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
