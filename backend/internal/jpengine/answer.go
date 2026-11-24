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
		case r == 0x200c || r == 0x200d: // joiners are real text in some scripts
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

// originAliases are spellings of one country that contain a comma, brackets
// or an abbreviation; they are read before the multi-country check.
var originAliases = map[string]string{
	"korea, republic of": "South Korea", "korea (republic of)": "South Korea", "korea, rep.": "South Korea", "korea (south)": "South Korea",
	"lao pdr": "Laos", "lao people's democratic republic": "Laos", "lao": "Laos",
	"hong kong, china": "Hong Kong", "hong kong (china)": "Hong Kong", "china, hong kong": "Hong Kong",
	"myanmar (burma)": "Myanmar", "burma (myanmar)": "Myanmar", "burma": "Myanmar",
	"brunei darussalam": "Brunei Darussalam",
	"viet nam":          "Vietnam",
	// Single countries whose official names contain "and" or a comma.
	"trinidad and tobago": "Trinidad and Tobago", "bosnia and herzegovina": "Bosnia and Herzegovina",
	"antigua and barbuda": "Antigua and Barbuda", "saint kitts and nevis": "Saint Kitts and Nevis",
	"sao tome and principe": "Sao Tome and Principe", "saint vincent and the grenadines": "Saint Vincent and the Grenadines",
	"congo, democratic republic of": "Democratic Republic of the Congo", "congo, republic of": "Republic of the Congo",
	"tanzania, united republic of": "Tanzania", "iran, islamic republic of": "Iran", "moldova, republic of": "Moldova",
	"korea, democratic people's republic of": "North Korea", "taiwan, province of china": "Taiwan",
	"micronesia, federated states of": "Micronesia", "bolivia (plurinational state of)": "Bolivia",
}

// notOrigins are whole answers that name a region or no country at all.
var notOrigins = map[string]bool{
	"asia": true, "southeast asia": true, "south east asia": true, "east asia": true, "south asia": true, "asean": true,
	"asia pacific": true, "europe": true, "africa": true, "america": true, "americas": true, "north america": true,
	"south america": true, "latin america": true, "oceania": true, "middle east": true, "various": true, "multiple": true,
	"several": true, "mixed": true, "unknown": true, "none": true, "n/a": true, "na": true, "undetermined": true,
	"unspecified": true, "other": true, "others": true, "global": true, "worldwide": true, "tbd": true, "any": true,
	"not applicable": true, "no origin": true, "not specified": true,
}

var notOriginPrefixes = []string{"various ", "multiple ", "several ", "not ", "no ", "unknown ", "mixed "}

var multiCountry = regexp.MustCompile(`[,;/|&+]|\b(?:and|or|and/or|not|except|excluding|plus|with|from)\b`)

// singleName is a permissive shape for one country name.
var singleName = regexp.MustCompile(`^\p{L}[\p{L}\p{M} .'\x{2019}()\-]*$`)

// originCountry returns the canonical origin for a qualifying answer, which
// must name exactly one country other than Japan, or the EU. Names the shared
// country table lacks are accepted when they look like one country name.
func originCountry(raw string) (string, bool) {
	s := strings.Join(strings.Fields(raw), " ")
	l := strings.ToLower(s)
	if l == "eu" || l == "european union" {
		return "EU", true
	}
	if a, ok := originAliases[l]; ok {
		s, l = a, strings.ToLower(a)
	}
	if l == "" || notOrigins[l] || l == "japan" || strings.Contains(" "+l+" ", " japan ") {
		return "", false
	}
	for _, p := range notOriginPrefixes {
		if strings.HasPrefix(l, p) {
			return "", false
		}
	}
	if _, aliased := originAliases[l]; !aliased && multiCountry.MatchString(l) {
		return "", false
	}
	if n, ok := domain.KnownCountry(s); ok && n != "Japan" && len(domain.CountryParts(n)) == 1 {
		return n, true
	}
	if n := domain.NormalizeCountry(s); extraParties[n] {
		return n, true
	}
	if utf8.RuneCountInString(s) > 60 || utf8.RuneCountInString(s) < 3 || !singleName.MatchString(s) {
		return "", false
	}
	if isASCII(s) {
		return domain.NormalizeCountry(s), true
	}
	return s, true
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// rulingMention finds a Japan ruling number as written in free text: JP, any
// case or fullwidth, optional space, dash or underscore, then five or more
// digits (a real id has seven).
var rulingMention = regexp.MustCompile(`[JjＪｊ][PpＰｐ][\p{Z}\s_\-\x{2010}-\x{2015}\x{2212}\x{FF0D}\x{FF3F}]{0,3}[0-9０-９]{5,}`)

// maskUnknownRulings replaces ruling numbers that are not in allowed (ids of
// precedents the model was shown) with a neutral phrase. A match directly
// after an ASCII letter is part of another word and stays. With a separator
// only seven digits (a real id) count, so "JP 20250101" stays.
func maskUnknownRulings(s string, allowed map[string]bool) string {
	var b strings.Builder
	last := 0
	for _, loc := range rulingMention.FindAllStringIndex(s, -1) {
		start, end := loc[0], loc[1]
		if r, _ := utf8.DecodeLastRuneInString(s[:start]); start > 0 && r < utf8.RuneSelf && unicode.IsLetter(r) {
			continue
		}
		if m := s[start:end]; strings.ContainsAny(m, " _-\u2010\u2011\u2012\u2013\u2014\u2015\u2212\uff0d\uff3f\u3000") {
			if digitCount(m) != 7 {
				continue // a spaced "JP 20250101" is not a ruling id
			}
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

func isDigit(r rune) bool { return r >= '0' && r <= '9' || r >= '０' && r <= '９' }

func digitCount(s string) int {
	n := 0
	for _, r := range s {
		if isDigit(r) {
			n++
		}
	}
	return n
}
