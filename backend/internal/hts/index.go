// Package hts suggests HTSUS subheadings for a product (D64). Many corpus
// rulings are "tariff classification and country of origin" rulings that
// state the subheading CBP applied; the index lists those rulings with their
// subheadings, BM25 finds the ones most like the product, and one model call
// explains each candidate citing only the rulings retrieved for it. It is a
// suggestion for the duty table (D52), never a classification.
package hts

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/duty"
)

// Where a ruling's subheading was found, most reliable first.
const (
	SourceTariffNo   = "tariff_no"             // the "TARIFF NO.:" header line
	SourceHolding    = "holding"               // the last HOLDING section
	SourceApplicable = "applicable_subheading" // "the applicable subheading ... is NNNN.NN.NN"
)

// Entry is one ruling that states an HTS subheading.
type Entry struct {
	Ruling  string   `json:"ruling"`
	Date    string   `json:"date"` // YYYY-MM-DD
	Subject string   `json:"subject"`
	HTS     []string `json:"hts"` // 8-digit subheadings, NNNN.NN.NN, in the order stated
	Source  string   `json:"source"`
}

// Index lists the rulings that state a subheading, as written by
// "corpus hts-index" (data/hts_index.json).
type Index struct {
	Note    string  `json:"note"`
	Rulings []Entry `json:"rulings"`
}

// Subheadings counts the distinct subheadings in the index.
func (x *Index) Subheadings() int {
	seen := map[string]bool{}
	for _, e := range x.Rulings {
		for _, h := range e.HTS {
			seen[h] = true
		}
	}
	return len(seen)
}

// LoadIndex reads an index file and checks every subheading is 8 digits.
func LoadIndex(path string) (*Index, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("hts: read %s: %w", path, err)
	}
	var x Index
	if err := json.Unmarshal(b, &x); err != nil {
		return nil, fmt.Errorf("hts: decode %s: %w", path, err)
	}
	if len(x.Rulings) == 0 {
		return nil, fmt.Errorf("hts: %s lists no rulings", path)
	}
	for _, e := range x.Rulings {
		if len(e.HTS) == 0 {
			return nil, fmt.Errorf("hts: %s: %s has no subheading", path, e.Ruling)
		}
		for _, h := range e.HTS {
			if !subheading.MatchString(h) {
				return nil, fmt.Errorf("hts: %s: %s: %q is not NNNN.NN.NN", path, e.Ruling, h)
			}
		}
	}
	return &x, nil
}

var (
	subheading = regexp.MustCompile(`^\d{4}\.\d{2}\.\d{2}$`)
	// htsNumber is an HTSUS number of 8 or 10 digits; boundaries are checked
	// in codes (RE2 has no lookbehind).
	htsNumber  = regexp.MustCompile(`\d{4}\.\d{2}\.\d{2}(?:\.?\d{2})?`)
	tariffNo   = regexp.MustCompile(`TARIFF NO\.?:?([^\r\n]*)`)
	holding    = regexp.MustCompile(`\bHOLDINGS?:?[ \t]*[\r\n]`)
	applicable = regexp.MustCompile(`(?i)applicable (?:tariff )?(?:sub)?heading[^.]{0,150}?(\d{4}\.\d{2}\.\d{2}(?:\.?\d{2})?)`)
)

// holdingSpan is how much of the HOLDING section is searched.
const holdingSpan = 3000

// codes returns the 8-digit subheadings in s, in order, without duplicates.
// Chapters 98 and 99 (special provisions such as 9802 and the Section 301
// headings in 9903) are not product classifications and are skipped.
func codes(s string) []string {
	var out []string
	for _, m := range htsNumber.FindAllStringIndex(s, -1) {
		if m[0] > 0 && (isDigit(s[m[0]-1]) || s[m[0]-1] == '.') || m[1] < len(s) && isDigit(s[m[1]]) {
			continue // part of a longer number
		}
		h := duty.Normalize(s[m[0]:m[1]])
		if strings.HasPrefix(h, "98") || strings.HasPrefix(h, "99") || slices.Contains(out, h) {
			continue
		}
		out = append(out, h)
	}
	return out
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// Extract finds the subheadings a ruling's text says CBP applied, and where:
// the TARIFF NO. header when it names a product subheading, else the last
// HOLDING section, else "applicable subheading ... NNNN.NN.NN" phrases. It
// returns nil when the ruling states none.
func Extract(text string) ([]string, string) {
	if m := tariffNo.FindStringSubmatch(text); m != nil {
		if cs := codes(m[1]); len(cs) > 0 {
			return cs, SourceTariffNo
		}
	}
	if locs := holding.FindAllStringIndex(text, -1); len(locs) > 0 {
		start := locs[len(locs)-1][1]
		if cs := codes(text[start:min(len(text), start+holdingSpan)]); len(cs) > 0 {
			return cs, SourceHolding
		}
	}
	var cs []string
	for _, m := range applicable.FindAllStringSubmatch(text, -1) {
		for _, c := range codes(m[1]) {
			if !slices.Contains(cs, c) {
				cs = append(cs, c)
			}
		}
	}
	if len(cs) > 0 {
		return cs, SourceApplicable
	}
	return nil, ""
}
