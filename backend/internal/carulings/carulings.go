// Package carulings holds the CBSA's published origin and marking rulings
// (data/ca, D923) and picks the ones related to a Canada case. They are
// shown beside the SOR/94-23 rules check and never change its answer: 14
// rulings are too few to test anything against.
package carulings

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strings"
	"unicode"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

//go:embed rulings.json
var raw []byte

// Ruling is one row of data/ca/rulings.jsonl.
type Ruling struct {
	ID        string   `json:"id"`
	Date      string   `json:"date"`
	Type      string   `json:"type"`
	Product   string   `json:"product"`
	HS        []string `json:"hs"`
	Agreement string   `json:"agreement"`
	Holding   string   `json:"holding"`
	Facts     string   `json:"facts"`
	Reasoning string   `json:"reasoning"`
	URL       string   `json:"url"`
}

// MaxShown caps the rulings shown on a case.
const MaxShown = 3

var all = mustLoad()

func mustLoad() []Ruling {
	var rs []Ruling
	if err := json.Unmarshal(raw, &rs); err != nil {
		panic("carulings: " + err.Error())
	}
	return rs
}

// All returns every ruling, oldest first.
func All() []Ruling { return append([]Ruling(nil), all...) }

// For picks at most MaxShown rulings related to the product: same
// subheading, then heading, then chapter of the good's code; without a code
// match, a ruling needs two shared words with the product, one of them in
// the ruling's product name. Ties go to the newer ruling. Nil when none
// relate.
func For(p *domain.Product) []domain.CanadaRuling {
	if p == nil {
		return nil
	}
	good := digits(p.HTS)
	words := []string{p.Name, p.Description}
	for _, c := range p.Components {
		words = append(words, c.Name)
	}
	mine := tokens(strings.Join(words, " "))
	type scored struct {
		r       Ruling
		hs, txt int
	}
	var picked []scored
	for _, r := range all {
		s := scored{r: r}
		for _, c := range r.HS {
			s.hs = max(s.hs, codeMatch(good, digits(c)))
		}
		name, text := tokens(r.Product), tokens(r.Product+" "+r.Facts)
		inName := false
		for w := range mine {
			if text[w] {
				s.txt++
			}
			if name[w] {
				inName = true
			}
		}
		if s.hs > 0 || (s.txt >= 2 && inName) {
			picked = append(picked, s)
		}
	}
	sort.SliceStable(picked, func(i, j int) bool {
		a, b := picked[i], picked[j]
		if a.hs != b.hs {
			return a.hs > b.hs
		}
		if a.txt != b.txt {
			return a.txt > b.txt
		}
		return a.r.Date > b.r.Date
	})
	var out []domain.CanadaRuling
	for i, s := range picked {
		if i == MaxShown {
			break
		}
		out = append(out, domain.CanadaRuling{ID: s.r.ID, Date: s.r.Date, Kind: s.r.Type, Product: s.r.Product,
			Agreement: s.r.Agreement, Holding: s.r.Holding, URL: s.r.URL})
	}
	return out
}

// codeMatch: 3 same subheading, 2 same heading, 1 same chapter, 0 otherwise.
func codeMatch(a, b string) int {
	switch {
	case len(a) >= 6 && len(b) >= 6 && a[:6] == b[:6]:
		return 3
	case len(a) >= 4 && len(b) >= 4 && a[:4] == b[:4]:
		return 2
	case len(a) >= 2 && len(b) >= 2 && a[:2] == b[:2]:
		return 1
	}
	return 0
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

var stop = map[string]bool{"with": true, "from": true, "made": true, "other": true, "part": true, "parts": true,
	"product": true, "products": true, "goods": true, "used": true, "into": true, "that": true, "this": true,
	"which": true, "their": true, "have": true, "will": true, "under": true, "were": true, "also": true}

// tokens: lower-case words of four or more letters, without common words; a
// trailing "s" is dropped so "pillows" meets "pillow".
func tokens(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) }) {
		t := strings.TrimSuffix(w, "s")
		if len(t) >= 4 && !stop[w] && !stop[t] {
			out[t] = true
		}
	}
	return out
}
