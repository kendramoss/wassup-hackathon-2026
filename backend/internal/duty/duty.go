// Package duty holds sourced U.S. duty rates by HTS subheading and origin
// (D52). Each total is the sum of its components (MFN plus additional duties
// such as Section 301), every component cites its HTS heading and source,
// and unconfirmed components are marked. The table covers only lines that
// were researched (docs/DUTY-SOURCES.md); anything else is the user's input.
package duty

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

//go:embed rates.json
var ratesJSON []byte

// Component is one duty that applies to a line and origin.
type Component struct {
	Name     string  `json:"name"`
	Rate     float64 `json:"rate"` // ad valorem fraction: 0.25 = 25%
	Heading  string  `json:"heading"`
	Source   string  `json:"source"`
	Verified bool    `json:"verified"`
	Note     string  `json:"note,omitempty"`
}

// Rate is the total for one origin with its breakdown.
type Rate struct {
	Origin     string      `json:"origin"`
	Total      float64     `json:"total"`
	Components []Component `json:"components"`
	// Unverified is true when any component is not confirmed from a primary source.
	Unverified bool `json:"unverified"`
}

// Line is one HTS subheading's rates.
type Line struct {
	HTS         string `json:"hts"`
	Description string `json:"description"`
	Checked     string `json:"checked"`
	Revision    string `json:"hts_revision"`
	SourcesDoc  string `json:"sources_doc"`
	Rates       []Rate `json:"rates"`
}

// Table is the sourced rate table.
type Table struct{ lines map[string]Line }

type file struct {
	Checked    string `json:"checked"`
	Revision   string `json:"hts_revision"`
	SourcesDoc string `json:"sources_doc"`
	Lines      []struct {
		HTS         string                 `json:"hts"`
		Description string                 `json:"description"`
		Checked     string                 `json:"checked"` // overrides the file's date
		Origins     map[string][]Component `json:"origins"`
	} `json:"lines"`
}

// Load reads the embedded table.
func Load() (*Table, error) { return parse(ratesJSON) }

func parse(b []byte) (*Table, error) {
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("duty: decode rates: %w", err)
	}
	t := &Table{lines: map[string]Line{}}
	for _, l := range f.Lines {
		line := Line{HTS: l.HTS, Description: l.Description, Checked: f.Checked, Revision: f.Revision, SourcesDoc: f.SourcesDoc}
		if l.Checked != "" {
			line.Checked = l.Checked
		}
		for origin, cs := range l.Origins {
			r := Rate{Origin: domain.NormalizeCountry(origin), Components: cs}
			for _, c := range cs {
				if c.Rate < 0 || c.Rate > 1 {
					return nil, fmt.Errorf("duty: %s %s %s: rate %v out of range", l.HTS, origin, c.Name, c.Rate)
				}
				r.Total += c.Rate
				r.Unverified = r.Unverified || !c.Verified
			}
			r.Total = math.Round(r.Total*1e4) / 1e4
			line.Rates = append(line.Rates, r)
		}
		sort.Slice(line.Rates, func(i, j int) bool { return line.Rates[i].Origin < line.Rates[j].Origin })
		t.lines[Normalize(l.HTS)] = line
	}
	return t, nil
}

// ErrInvalidHTS means a string is not a supported HTS number (see ParseHTS).
// It also matches ErrNoRate, so a caller that treats "no line" as unpriced
// treats a malformed number the same way.
var ErrInvalidHTS = invalidHTSError{}

type invalidHTSError struct{}

func (invalidHTSError) Error() string { return "duty: not a supported HTS number" }
func (invalidHTSError) Is(target error) bool {
	return target == ErrNoRate
}

// HTSCode is a validated HTS number.
type HTSCode struct {
	Subheading  string // NNNN.NN.NN, the table and live-lookup key
	Statistical string // the two statistical digits of a 10-digit number, else ""
}

// ParseHTS is the one strict HTS syntax check; Table.Line and LiveHTS both
// use it. After trimming surrounding whitespace the number must be either
//
//	8 digits  NNNNNNNN, or NNNN<s>NN<s>NN
//	10 digits NNNNNNNNNN, NNNN<s>NN<s>NNNN, or NNNN<s>NN<s>NN<s>NN
//
// where <s> is a single '.', ' ' or '-', the same one throughout. Digits are
// ASCII 0-9 only. Letters, other characters, other lengths (6, 9, 11+),
// doubled, leading or trailing separators and mixed separators are
// rejected, never stripped or truncated. A 10-digit number keeps its
// statistical suffix and maps to its 8-digit subheading.
func ParseHTS(s string) (HTSCode, error) {
	s = strings.TrimSpace(s)
	bad := func() (HTSCode, error) {
		return HTSCode{}, fmt.Errorf("%w: %q (want 8 or 10 digits, e.g. 8419.89.95 or 8419.89.9500)", ErrInvalidHTS, s)
	}
	var sep byte
	if i := strings.IndexAny(s, ". -"); i >= 0 {
		sep = s[i]
	}
	var d string
	if sep == 0 {
		d = s
	} else {
		parts := strings.Split(s, string(sep))
		switch len(parts) {
		case 3:
			if len(parts[0]) != 4 || len(parts[1]) != 2 || (len(parts[2]) != 2 && len(parts[2]) != 4) {
				return bad()
			}
		case 4:
			if len(parts[0]) != 4 || len(parts[1]) != 2 || len(parts[2]) != 2 || len(parts[3]) != 2 {
				return bad()
			}
		default:
			return bad()
		}
		d = strings.Join(parts, "")
	}
	if len(d) != 8 && len(d) != 10 {
		return bad()
	}
	for i := 0; i < len(d); i++ {
		if d[i] < '0' || d[i] > '9' {
			return bad()
		}
	}
	return HTSCode{Subheading: d[:4] + "." + d[4:6] + "." + d[6:8], Statistical: d[8:]}, nil
}

// Normalize keeps the digits of an HTS number and formats them as
// NNNN.NN.NN (extra statistical digits are dropped). It is a lenient
// formatter for already-trusted text (table data, numbers matched by a
// pattern), not a validator: untrusted input must go through ParseHTS first.
func Normalize(hts string) string {
	var d []rune
	for _, r := range hts {
		if r >= '0' && r <= '9' {
			d = append(d, r)
		}
	}
	if len(d) < 8 {
		return string(d)
	}
	s := string(d[:8])
	return s[:4] + "." + s[4:6] + "." + s[6:8]
}

// Line returns the rates for an HTS subheading (8 digits; a 10-digit
// statistical number matches its subheading). A number ParseHTS rejects has
// no line.
func (t *Table) Line(hts string) (Line, bool) {
	c, err := ParseHTS(hts)
	if err != nil {
		return Line{}, false
	}
	l, ok := t.lines[c.Subheading]
	return l, ok
}

// Lines lists every subheading in the table.
func (t *Table) Lines() []Line {
	out := make([]Line, 0, len(t.lines))
	for _, l := range t.lines {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].HTS < out[j].HTS })
	return out
}

// Fill returns rates with every origin the table has for hts added; a rate
// the user gave is kept. It reports the origins it filled.
func (t *Table) Fill(rates map[string]float64, hts string) (map[string]float64, []string) {
	l, ok := t.Line(hts)
	if !ok {
		return rates, nil
	}
	out := map[string]float64{}
	given := map[string]bool{}
	for k, v := range rates {
		out[k] = v
		given[domain.NormalizeCountry(k)] = true
	}
	var filled []string
	for _, r := range l.Rates {
		if !given[r.Origin] {
			out[r.Origin] = r.Total
			filled = append(filled, r.Origin)
		}
	}
	return out, filled
}

// Note describes filled rates for Economics.DutyRatesNote.
func (t *Table) Note(hts string, filled []string) string {
	l, _ := t.Line(hts)
	var unverified []string
	for _, r := range l.Rates {
		for _, f := range filled {
			if r.Origin == f && r.Unverified {
				unverified = append(unverified, f)
			}
		}
	}
	n := fmt.Sprintf("Duty rates for %s from the sourced table (HTS %s, %s, checked %s; %s): %s.", strings.Join(filled, ", "), l.HTS, l.Revision, l.Checked, l.SourcesDoc, l.Description)
	if len(unverified) > 0 {
		n += " Not fully confirmed from a primary source: " + strings.Join(unverified, ", ") + "."
	}
	return n
}
