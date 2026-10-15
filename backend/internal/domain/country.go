package domain

import (
	"regexp"
	"strings"
)

// countryAliases maps lower-cased spellings to the canonical English name used
// in CBP labels. Ported from ALIASES in
// hackathon-research/experiments/origin-proof/predict.py.
var countryAliases = map[string]string{
	"usa": "United States", "us": "United States", "u.s.": "United States", "united states of america": "United States",
	"us virgin islands": "US Virgin Islands", "united states virgin islands": "US Virgin Islands",
	"prc": "China", "people's republic of china": "China", "mainland china": "China",
	"viet nam": "Vietnam", "republic of korea": "South Korea", "korea": "South Korea", "s. korea": "South Korea",
	"taiwan (roc)": "Taiwan", "uk": "United Kingdom", "great britain": "United Kingdom", "england": "United Kingdom",
	"the netherlands": "Netherlands", "holland": "Netherlands", "czechia": "Czech Republic", "turkiye": "Turkey",
	"türkiye": "Turkey", "hong kong sar": "Hong Kong",
}

var nonCountryChars = regexp.MustCompile(`[^a-z .'()]`)

// NormalizeCountry returns the canonical name for a country as written by a
// model or a ruling, so predictions and labels compare equal. It mirrors norm()
// in predict.py so Go eval numbers are comparable with the research runs:
// aliases are matched after removing digits and punctuation except ". ' ( )",
// and unknown names are title-cased.
func NormalizeCountry(s string) string {
	// "Indonesia/Malaysia", "Indonesia or Malaysia": normalise each part
	// rather than gluing them into one word (D23).
	if parts := altSplit.Split(strings.TrimSpace(s), -1); len(parts) > 1 {
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if n := normalizeOne(p); n != "" {
				out = append(out, n)
			}
		}
		return strings.Join(out, "/")
	}
	return normalizeOne(s)
}

var altSplit = regexp.MustCompile(`\s*(?:/|\||\band/or\b|\bor\b)\s*`)

// CountryParts splits a normalised "A/B" origin into its countries.
func CountryParts(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "/")
}

func normalizeOne(s string) string {
	c := strings.ToLower(strings.TrimSpace(s))
	if c == "" {
		return ""
	}
	// predict.py looks up "türkiye" before its regex would strip the "ü"; keep
	// that behaviour by checking the raw lower-cased form first.
	if v, ok := countryAliases[c]; ok && strings.ContainsRune(c, 'ü') {
		return v
	}
	c = nonCountryChars.ReplaceAllString(c, "")
	c = strings.TrimSpace(strings.ReplaceAll(c, ".", ""))
	c = strings.TrimPrefix(c, "the ")
	if v, ok := countryAliases[c]; ok {
		return v
	}
	return strings.ReplaceAll(titleCase(c), "Of", "of")
}

// titleCase upper-cases the first letter of each run of letters, like
// Python's str.title() on ASCII input.
func titleCase(s string) string {
	b := []byte(s)
	prevLetter := false
	for i, ch := range b {
		isLetter := ch >= 'a' && ch <= 'z'
		if isLetter && !prevLetter {
			b[i] = ch - 'a' + 'A'
		}
		prevLetter = isLetter
	}
	return string(b)
}
