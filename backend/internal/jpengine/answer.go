package jpengine

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// Caps on the free text a model answer may put on the result. The prompt asks
// for a few words and at most 80 words.
const (
	maxReasoningRunes    = 2000
	maxDecidingRuleRunes = 200
)

// cleanText drops control and format characters (NUL, bidi overrides, zero
// width marks) except newline and tab, then trims space.
func cleanText(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r):
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

// capRunes keeps at most n runes; a longer text ends in an ellipsis within n.
func capRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

// sanitized returns the answer with its free text cleaned and capped.
func (a modelAnswer) sanitized() modelAnswer {
	a.Reasoning = capRunes(cleanText(a.Reasoning), maxReasoningRunes)
	a.DecidingRule = capRunes(cleanText(a.DecidingRule), maxDecidingRuleRunes)
	return a
}

// extraParties are parties of the agreements in jpcorpus.AgreementNames that
// the shared country table lacks.
var extraParties = map[string]bool{"Brunei": true, "Brunei Darussalam": true, "Myanmar": true}

// originCountry returns the canonical origin for a qualifying answer, which
// must name exactly one country other than Japan, or the EU.
func originCountry(raw string) (string, bool) {
	if strings.EqualFold(strings.TrimSpace(raw), "EU") || strings.EqualFold(strings.TrimSpace(raw), "European Union") {
		return "EU", true
	}
	n, ok := domain.KnownCountry(raw)
	if ok && n != "Japan" && len(domain.CountryParts(n)) == 1 {
		return n, true
	}
	if n = domain.NormalizeCountry(raw); extraParties[n] {
		return n, true
	}
	return "", false
}

// rulingMention finds a Japan ruling number as written in free text: JP, any
// case or fullwidth, optional space, dash or underscore, then five or more
// digits (a real id has seven).
var rulingMention = regexp.MustCompile(`[JjＪｊ][PpＰｐ][\p{Z}\s_\-\x{2010}-\x{2015}\x{2212}\x{FF0D}\x{FF3F}]{0,3}[0-9０-９]{5,}`)

// maskUnknownRulings replaces ruling numbers that are not in allowed (ids of
// precedents the model was shown) with a neutral phrase. A match directly
// after a letter is part of another word and stays.
func maskUnknownRulings(s string, allowed map[string]bool) string {
	var b strings.Builder
	last := 0
	for _, loc := range rulingMention.FindAllStringIndex(s, -1) {
		start, end := loc[0], loc[1]
		if r, _ := utf8.DecodeLastRuneInString(s[:start]); start > 0 && unicode.IsLetter(r) {
			continue
		}
		if allowed[canonicalRulingID(s[start:end])] {
			continue
		}
		b.WriteString(s[last:start])
		b.WriteString("an unverified reference")
		last = end
	}
	b.WriteString(s[last:])
	return b.String()
}

// canonicalRulingID folds a mention to "JP" plus ASCII digits.
func canonicalRulingID(mention string) string {
	var b strings.Builder
	for _, r := range norm.NFKC.String(mention) {
		if r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
			b.WriteRune(unicode.ToUpper(r))
		}
	}
	return b.String()
}
