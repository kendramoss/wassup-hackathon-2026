package engine

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

// MaxRuleRows caps the 102.20 rows quoted to the Part 102 engine (D550).
const MaxRuleRows = 4

// htsInFacts finds tariff codes written in facts: dotted codes
// ("8414.30", "8473.30.5100") and four-digit headings named as such
// ("heading 8482", "subheadings 8414"). Bare four-digit numbers are not
// read (years, model numbers).
var htsInFacts = regexp.MustCompile(`\b((?:sub)?headings? )?(\d{4}(?:\.\d{2}(?:\.\d{2,4}){0,2})?)\b`)

// CodesIn returns the tariff codes c names, in order, without repeats: the
// product's own HTS first when the case has a structured product, then the
// codes written in the facts.
func CodesIn(c domain.Case) []string {
	var out []string
	seen := map[string]bool{}
	add := func(code string) {
		d := tariffshift.Digits(code)
		if d == "" || seen[d] {
			return
		}
		seen[d] = true
		out = append(out, code)
	}
	if c.Product != nil {
		add(c.Product.HTS)
	}
	for _, m := range htsInFacts.FindAllStringSubmatch(c.Facts, -1) {
		if m[1] != "" || strings.Contains(m[2], ".") {
			add(m[2])
		}
	}
	return out
}

// Part102RuleText quotes the 19 CFR 102.20 rows (verbatim, with the notes
// that govern them) for the tariff codes c names, at most MaxRuleRows rows,
// so the Part 102 engine applies the regulation's own words (D550). It is
// "" when c names no code in an encoded chapter. Deterministic: the same
// case always gets the same text.
func Part102RuleText(rules *tariffshift.Rules) func(domain.Case) string {
	return func(c domain.Case) string {
		if rules == nil {
			return ""
		}
		var lines []string
		seen := map[string]bool{}
		for _, code := range CodesIn(c) {
			for _, row := range rules.RowsFor(code) {
				if seen[row.ID] || len(seen) >= MaxRuleRows {
					continue
				}
				seen[row.ID] = true
				l := fmt.Sprintf("- %s (code named: %s): %s", row.ID, code, row.Text)
				for _, n := range row.Notes {
					l += "\n  " + n
				}
				lines = append(lines, l)
			}
		}
		if len(lines) == 0 {
			return ""
		}
		src := rules.Table().Source
		return fmt.Sprintf("19 CFR 102.20 RULE TEXT for the tariff codes named in the facts (verbatim, eCFR as of %s; a code named in the facts may be the good's or a material's):\n%s",
			src.AsOf, strings.Join(lines, "\n"))
	}
}
