package rcep

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

//go:embed data/psr_hs2022.json
var psrJSON []byte

// Range is an "except from" exclusion: HS codes from From to To inclusive at
// Level ("chapter", "heading" or "subheading"), written as in the Annex
// ("72.08", "8504.31").
type Range struct {
	Level string `json:"level"`
	From  string `json:"from"`
	To    string `json:"to"`
}

// Alternative is one way to meet a rule, e.g. "CTH except from heading
// 72.08" or "RVC40". Criterion is WO, CC, CTH, CTSH, RVC or CR.
type Alternative struct {
	Criterion string  `json:"criterion"`
	Threshold int     `json:"threshold,omitempty"`
	Except    []Range `json:"except,omitempty"`
	// Condition names an extra requirement written in words in the Annex;
	// only "active_ingredient_50pct" (heading 38.08) exists.
	Condition string `json:"condition,omitempty"`
}

// Variant is one branch of a row whose rule depends on what the product is
// (e.g. "1. CTH for carp ... 2. CC for any other good").
type Variant struct {
	ID           string        `json:"id"`
	Label        string        `json:"label"`
	Alternatives []Alternative `json:"alternatives"`
}

// Rule is one row of Annex 3A.
type Rule struct {
	Code        string `json:"code"`
	Level       string `json:"level"`
	Heading     string `json:"heading,omitempty"`
	Description string `json:"description"`
	Text        string `json:"text"`
	// Status is "parsed" (alternatives read from the text), "manual" (the
	// parser could not read it; see manual.go), or "split" (the row has no
	// rule of its own; its subheadings do).
	Status       string        `json:"status"`
	Alternatives []Alternative `json:"alternatives,omitempty"`
	Variants     []Variant     `json:"variants,omitempty"`
	Pages        []int         `json:"pages"`
}

// Coverage is the parser's own count, kept with the table.
type Coverage struct {
	Rows             int `json:"rows"`
	RowsWithRule     int `json:"rows_with_rule"`
	Parsed           int `json:"parsed"`
	Manual           int `json:"manual"`
	SplitWithoutRule int `json:"split_rows_without_rule"`
	Unreadable       int `json:"unreadable_rows"`
	Chapters         int `json:"chapters"`
}

// Table is the Annex 3A rule table.
type Table struct {
	Version  string          `json:"version"`
	Source   json.RawMessage `json:"source"`
	Coverage Coverage        `json:"coverage"`
	Rules    []Rule          `json:"rules"`

	byChapter, byHeading, bySub map[string]*Rule
}

var (
	defaultOnce  sync.Once
	defaultTable *Table
	defaultErr   error
)

// Default returns the embedded HS 2022 table with the hand-encoded rows
// filled in. It panics if the embedded file is broken, which a test catches.
func Default() *Table {
	defaultOnce.Do(func() { defaultTable, defaultErr = Load(psrJSON) })
	if defaultErr != nil {
		panic(defaultErr)
	}
	return defaultTable
}

// Load parses a table and applies the hand encodings in manual.go.
func Load(b []byte) (*Table, error) {
	var t Table
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("rcep: rule table: %w", err)
	}
	t.byChapter, t.byHeading, t.bySub = map[string]*Rule{}, map[string]*Rule{}, map[string]*Rule{}
	for i := range t.Rules {
		r := &t.Rules[i]
		if r.Status == "manual" {
			m, ok := manualRules[r.Code]
			if !ok {
				return nil, fmt.Errorf("rcep: row %s is not parsed and has no hand encoding", r.Code)
			}
			if norm(r.Text) != norm(m.text) {
				return nil, fmt.Errorf("rcep: row %s text changed; re-check its hand encoding", r.Code)
			}
			r.Alternatives, r.Variants = m.alternatives, m.variants
		}
		d := digits(r.Code)
		switch r.Level {
		case "chapter":
			t.byChapter[d] = r
		case "heading":
			t.byHeading[d] = r
		case "subheading":
			t.bySub[d] = r
			if r.Heading != "" {
				t.byHeading[digits(r.Heading)] = r
			}
		}
	}
	return &t, nil
}

// Lookup returns the rule that applies to a good of HS code hs (at least
// four digits; six for headings whose rules are set per subheading). The
// most specific row with a rule wins: subheading, then heading, then
// chapter. ok is false when no row applies; need says why.
func (t *Table) Lookup(hs string) (r *Rule, ok bool, need string) {
	d := digits(hs)
	if len(d) < 4 {
		return nil, false, "the good's HS code needs at least 4 digits"
	}
	if len(d) >= 6 {
		if r := t.bySub[d[:6]]; r != nil && r.Status != "split" {
			return r, true, ""
		}
	}
	if r := t.byHeading[d[:4]]; r != nil {
		if r.Status != "split" {
			return r, true, ""
		}
		if len(d) < 6 {
			return nil, false, fmt.Sprintf("heading %s has rules per subheading; give 6 digits", dotted(d[:4]))
		}
		return nil, false, fmt.Sprintf("no rule found for subheading %s", dotted(d[:6]))
	}
	if r := t.byChapter[d[:2]]; r != nil && r.Status != "split" {
		return r, true, ""
	}
	return nil, false, fmt.Sprintf("no rule found for %s", dotted(d))
}

// digits keeps the digits of an HS code ("8504.31" -> "850431").
func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// dotted writes 4 or 6 digits the way the Annex does ("8504", "8504.31").
func dotted(d string) string {
	switch {
	case len(d) <= 4:
		return d
	default:
		return d[:4] + "." + d[4:]
	}
}

func norm(s string) string { return strings.Join(strings.Fields(s), " ") }
