package jpengine

import (
	_ "embed"
	"regexp"
	"strings"
	"sync"
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

// originAliases maps other spellings of one country to its canonical name.
// Keys are matched after foldName. A target that is a table name of the shared
// country table or a UN M49 name is used as written.
var originAliases = map[string]string{
	"korea, republic of": "South Korea", "korea (republic of)": "South Korea", "korea, rep.": "South Korea", "korea (south)": "South Korea",
	"korea, democratic people's republic of": "North Korea", "democratic people's republic of korea": "North Korea", "north korea": "North Korea",
	"lao pdr": "Laos", "lao people's democratic republic": "Laos", "lao": "Laos",
	"hong kong, china": "Hong Kong", "hong kong (china)": "Hong Kong", "china, hong kong": "Hong Kong", "hong kong sar": "Hong Kong",
	"china, hong kong special administrative region": "Hong Kong",
	"macau": "Macao", "macao": "Macao", "macao sar": "Macao", "china, macao": "Macao", "china, macao special administrative region": "Macao",
	"myanmar (burma)": "Myanmar", "burma (myanmar)": "Myanmar", "burma": "Myanmar",
	"brunei": "Brunei", "brunei darussalam": "Brunei",
	"viet nam": "Vietnam", "turkiye": "Turkey", "czechia": "Czech Republic",
	"russia": "Russian Federation", "iran": "Iran (Islamic Republic of)", "syria": "Syrian Arab Republic",
	"bolivia": "Bolivia (Plurinational State of)", "venezuela": "Venezuela (Bolivarian Republic of)",
	"tanzania": "United Republic of Tanzania", "tanzania, united republic of": "United Republic of Tanzania",
	"moldova": "Republic of Moldova", "moldova, republic of": "Republic of Moldova",
	"iran, islamic republic of": "Iran (Islamic Republic of)", "congo, democratic republic of": "Democratic Republic of the Congo",
	"congo, republic of": "Congo", "drc": "Democratic Republic of the Congo", "dr congo": "Democratic Republic of the Congo",
	"palestine": "State of Palestine", "swaziland": "Eswatini", "cape verde": "Cabo Verde", "east timor": "Timor-Leste",
	"ivory coast": "C\u00f4te d\u2019Ivoire", "micronesia": "Micronesia (Federated States of)", "the bahamas": "Bahamas", "the gambia": "Gambia",
	"netherlands": "Netherlands", "united kingdom of great britain and northern ireland": "United Kingdom",
	"united states of america": "United States", "taiwan, province of china": "Taiwan", "republic of korea": "South Korea",
}

// foldName makes country names comparable: NFKC, lower case, no diacritics,
// straight apostrophes, single spaces.
func foldName(s string) string {
	s = norm.NFD.String(norm.NFKC.String(s))
	s = strings.Map(func(r rune) rune {
		switch {
		case unicode.Is(unicode.Mn, r):
			return -1
		case r == '\u2019' || r == '\u2018' || r == '\u02bc':
			return '\''
		}
		return unicode.ToLower(r)
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// countryIndex maps a folded country name to its canonical name. It is closed:
// the shared country table, the aliases above, and the English country and
// area names of the UN Statistics Division M49 list (see
// docs/japan/answer-validation.md for source and hash).
var countryIndex = sync.OnceValue(func() map[string]string {
	m := map[string]string{}
	add := func(name, canon string) {
		if k := foldName(name); k != "" && m[k] == "" {
			m[k] = canon
		}
	}
	for _, n := range domain.CountryTable() {
		if n == "European Union" {
			continue
		}
		c := domain.NormalizeCountry(n)
		add(n, c)
		add(c, c)
	}
	for alias, canon := range originAliases {
		add(alias, canon)
		add(canon, canon)
	}
	for _, line := range strings.Split(strings.TrimSpace(unCountryTSV), "\n") {
		if f := strings.Split(line, "\t"); len(f) == 4 {
			add(f[3], f[3])
		}
	}
	return m
})

// originCountry returns the canonical origin for a qualifying answer, which
// must name exactly one country other than Japan, or the EU. Anything not in
// the closed country list is rejected.
func originCountry(raw string) (string, bool) {
	k := foldName(raw)
	if k == "eu" || k == "european union" {
		return "EU", true
	}
	n, ok := countryIndex()[k]
	if !ok || n == "Japan" {
		return "", false
	}
	return n, true
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

//go:embed countries_un_m49.tsv
var unCountryTSV string
