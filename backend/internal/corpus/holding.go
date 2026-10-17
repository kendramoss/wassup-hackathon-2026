package corpus

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The holding labeller is ported from
// hackathon-research/experiments/origin-proof/scripts/label.py (extract and
// choose), which produced the "regex" rows of eval/data/labels.jsonl. It is
// used for the Part 102 rulings collected for the USMCA regime (D61). Python
// lookarounds are not available in RE2: "(?![a-z])" after a country becomes
// a consumed "[^a-z]|$", and the sentence splitter's abbreviation
// lookbehinds are checked in code.

var (
	surfaceToCountry = map[string]string{}
	countryAlt       string
)

func init() {
	var forms []string
	for c, fs := range countrySurfaces {
		for _, f := range fs {
			surfaceToCountry[f] = c
			forms = append(forms, f)
		}
	}
	sort.Strings(forms)
	sort.SliceStable(forms, func(i, j int) bool { return len(forms[i]) > len(forms[j]) })
	q := make([]string, len(forms))
	for i, f := range forms {
		q[i] = regexp.QuoteMeta(f)
	}
	countryAlt = strings.Join(q, "|")
	holdingPatterns = []holdingPattern{
		{"A", regexp.MustCompile(`(?s)(?:country[- ]of[- ]origin|origin)(?:\s*,?\s*(?:of|for)\s*(?P<prod>.{0,220}?))?\s*,?\s*(?:is|are|will be|would be|shall be|should be|remains|will remain|is considered to be|will be considered(?: to be)?|is deemed to be)\s*(?:the\s*)?(?:same as [^.]{0,80}?i\.e\.,\s*)?(?P<c>` + countryAlt + `)(?:[^a-z]|$)`)},
		{"B", regexp.MustCompile(`(?s)(?:the\s+)?(?P<c>` + countryAlt + `)\s+(?:is|will be|would be)\s+the\s+country\s+of\s+origin(?:\s+(?:of|for)\s+(?P<prod>.{0,150}?))?[.,]`)},
		{"C", regexp.MustCompile(`(?:substantially transformed (?:in|into [^.]{0,80}? in)|origin is conferred (?:by|in)|(?:is|are|will be|would be) (?:considered )?(?:a )?products? of|origin is)\s+(?:the\s+)?(?P<c>` + countryAlt + `)(?:[^a-z]|$)`)},
		{"D", regexp.MustCompile(`(?:is|are|will be|would be) (?:of )?(?:the )?(?P<c>` + countryAlt + `)(?:n)? origin\b`)},
	}
	altCountry = regexp.MustCompile(`^\s*,?\s*(?:or|and)?\s*(?:the\s+)?(` + countryAlt + `)(?:[^a-z]|$)`)
	countryMention = regexp.MustCompile(`(` + countryAlt + `)`)
}

type holdingPattern struct {
	name string
	re   *regexp.Regexp
}

var (
	holdingPatterns []holdingPattern
	altCountry      *regexp.Regexp
	countryMention  *regexp.Regexp

	sentenceBreak = regexp.MustCompile(`[.!?]\s+[A-Z“"(]`)
	// Abbreviations a sentence never ends on (ABBR in label.py).
	abbreviations = []string{"no.", "No.", "U.S.", "v.", "Cir.", "Inc.", "Co.", "Mr.", "Ms.", "Nos.", "St.", "U.K.", "e.g.", "i.e.", "Corp.", "Ltd.", "F.", "Supp.", "pp.", "Fed.", "Dr.", "Jr."}

	negation      = regexp.MustCompile(`\b(not|no longer|cannot)\b`)
	negationTail  = regexp.MustCompile(`\b(not|no)\s*$`)
	hypothetical  = regexp.MustCompile(`would (be|have been) |\bif |assum|whether|claim|you (state|assert|contend|believe)|requester|propose`)
	agreement     = regexp.MustCompile(`we agree|agree with`)
	definition    = regexp.MustCompile(`defines|means the country|is defined|country of manufacture, production, or growth`)
	conclusionCue = regexp.MustCompile(`(HOLDING|Therefore|therefore|Accordingly|Based (up)?on|[Ii]t is (the|our) opinion|[Ii]n our opinion|we (find|hold|conclude|are of the opinion)|[Cc]onsequently|As such|Thus|As a result|conclude|we believe|[Hh]ence|will be|is the opinion)`)
	nonWord       = regexp.MustCompile(`\W+`)
)

// holdingHit is one origin statement found in a ruling's analysis.
type holdingHit struct {
	product, country, sentence, pattern string
	concl                               bool
}

// sentences splits text at ". X" boundaries that do not follow a listed
// abbreviation (SENT in label.py).
func sentences(t string) []string {
	var out []string
	last := 0
	for _, loc := range sentenceBreak.FindAllStringIndex(t, -1) {
		end := loc[0] + 1 // after the punctuation
		if endsWithAbbreviation(t[:end]) {
			continue
		}
		_, w := utf8.DecodeLastRuneInString(t[loc[0]:loc[1]])
		out = append(out, t[last:end])
		last = loc[1] - w // the next sentence starts at its first letter
	}
	return append(out, t[last:])
}

func endsWithAbbreviation(s string) bool {
	for _, a := range abbreviations {
		if strings.HasSuffix(s, a) {
			before := s[:len(s)-len(a)]
			r, _ := utf8.DecodeLastRuneInString(before)
			if before == "" || !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_') {
				return true
			}
		}
	}
	return false
}

// lastRunes returns the last n runes of s.
func lastRunes(s string, n int) string {
	if c := utf8.RuneCountInString(s); c > n {
		return s[byteAt(s, c-n):]
	}
	return s
}

// extractHoldings finds origin statements in the analysis part of a
// normalised ruling text (extract in label.py).
func extractHoldings(analysis string) []holdingHit {
	var hits []holdingHit
	for _, s := range sentences(analysis) {
		s1 := space.ReplaceAllString(s, " ")
		for _, p := range holdingPatterns {
			ci, pi := p.re.SubexpIndex("c"), p.re.SubexpIndex("prod")
			for _, m := range p.re.FindAllStringSubmatchIndex(s1, -1) {
				pre := lastRunes(s1[:m[0]], 25)
				if negation.MatchString(s1[m[0]:m[1]]) || negationTail.MatchString(pre) {
					continue
				}
				pre120 := strings.ToLower(lastRunes(s1[:m[0]], 120))
				if hypothetical.MatchString(pre120) && !agreement.MatchString(pre120) {
					continue
				}
				if strings.Contains(pre120, "suggest") && !strings.Contains(strings.ToLower(s1), "agree") {
					continue
				}
				if definition.MatchString(s1) {
					continue
				}
				prod := ""
				if pi >= 0 && m[2*pi] >= 0 {
					prod = strings.TrimSpace(s1[m[2*pi]:m[2*pi+1]])
				}
				country := surfaceToCountry[s1[m[2*ci]:m[2*ci+1]]]
				tail := s1[m[2*ci+1]:]
				tail = tail[:byteAt(tail, 160)]
				for {
					am := altCountry.FindStringSubmatchIndex(tail)
					if am == nil {
						break
					}
					country += " | " + surfaceToCountry[tail[am[2]:am[3]]]
					tail = tail[am[3]:]
				}
				hits = append(hits, holdingHit{product: prod, country: country, sentence: Truncate(s1, 400),
					pattern: p.name, concl: conclusionCue.MatchString(s1)})
			}
		}
	}
	return hits
}

// chooseHoldings keeps the conclusions, preferring HOLDING sentences and
// patterns A and B, one per product, the last statement winning (choose in
// label.py).
func chooseHoldings(hits []holdingHit) []holdingHit {
	if len(hits) == 0 {
		return nil
	}
	pick := func(in []holdingHit, keep func(holdingHit) bool) []holdingHit {
		var out []holdingHit
		for _, h := range in {
			if keep(h) {
				out = append(out, h)
			}
		}
		if len(out) == 0 {
			return in
		}
		return out
	}
	pool := pick(hits, func(h holdingHit) bool { return h.concl })
	pool = pick(pool, func(h holdingHit) bool { return strings.Contains(h.sentence, "HOLDING") })
	pool = pick(pool, func(h holdingHit) bool { return h.pattern == "A" || h.pattern == "B" })
	var order []string
	byKey := map[string]holdingHit{}
	for _, h := range pool {
		key := Truncate(strings.TrimSpace(nonWord.ReplaceAllString(strings.ToLower(h.product), " ")), 80)
		if _, ok := byKey[key]; !ok {
			order = append(order, key)
		}
		byKey[key] = h
	}
	out := make([]holdingHit, len(order))
	for i, k := range order {
		out[i] = byKey[k]
	}
	return out
}

// LabelHolding labels a ruling from its full text the way label.py did:
// CBP's origin statements after the facts (meta.End, from SplitFacts), the
// primary origin when every product got the same country, and up to four
// holding sentences as evidence. The regime is part102_tariff_shift (the
// caller has checked USMCARegime).
func LabelHolding(id, rulingDate, collection, subject, text string, meta SplitMeta) Label {
	t := spaceRun.ReplaceAllString(strings.ReplaceAll(text, "\u00a0", " "), " ")
	facts := t[byteAt(t, meta.Start):byteAt(t, meta.End)]
	hits := chooseHoldings(extractHoldings(t[byteAt(t, meta.End):]))
	l := Label{
		Ruling: id, Date: rulingDate[:min(10, len(rulingDate))], Collection: strings.ToUpper(collection), Subject: subject,
		Regime: "part102_tariff_shift", MultiCountry: len(factsCountries(facts)) >= 2, Method: "regex", Evidence: []string{},
	}
	same := len(hits) > 0
	for _, h := range hits {
		same = same && h.country == hits[0].country
		if len(l.Evidence) < 4 {
			l.Evidence = append(l.Evidence, h.sentence)
		}
	}
	if same {
		o := hits[0].country
		l.PrimaryOrigin = &o
	}
	return l
}

// factsCountries returns the distinct countries named in the facts, without
// "United States" as the import destination (facts_countries in label.py,
// simplified: only the "to/into the United States" context is skipped).
func factsCountries(facts string) map[string]bool {
	out := map[string]bool{}
	for _, m := range countryMention.FindAllStringSubmatchIndex(facts, -1) {
		s, e := m[2], m[3]
		if s > 0 {
			if r, _ := utf8.DecodeLastRuneInString(facts[:s]); unicode.IsUpper(r) {
				continue
			}
		}
		if e < len(facts) {
			if r, _ := utf8.DecodeRuneInString(facts[e:]); r >= 'a' && r <= 'z' {
				continue
			}
		}
		c := surfaceToCountry[facts[s:e]]
		if c == "United States" && importContext.MatchString(strings.ToLower(lastRunes(facts[:s], 40))) {
			continue
		}
		out[c] = true
	}
	return out
}

var importContext = regexp.MustCompile(`(import|export|ship|sent|sold|sale|into|to) (in)?(to )?(the )?$`)

// USMCARegime reports whether a ruling applies the Part 102 general rules
// (19 CFR 102.11, with 102.20 tariff shifts) to a non-textile good: it cites
// 102.11 or 102.20, applies a 102.11 paragraph or a tariff shift, and does not
// apply a paragraph of the textile rule 102.21.
func USMCARegime(text string) bool {
	return part102Cite.MatchString(text) && part102Applied.MatchString(text) && !textileApplied.MatchString(text)
}

var (
	part102Cite    = regexp.MustCompile(`102\.(?:20|11)\b`)
	part102Applied = regexp.MustCompile(`(?i)102\.11\s*\(\s*[a-d]\s*\)|tariff shift|change in tariff classification`)
	textileApplied = regexp.MustCompile(`102\.21\s*\(`)
	tariffChapter  = regexp.MustCompile(`\b(\d{2})\d{2}\.\d{2}`)
)

// TextileTariffs reports whether a CROSS tariffs field ("6302.21.9020,
// 6307.90") lists only textile chapters (50-63), whose goods follow 102.21.
func TextileTariffs(tariffs string) bool {
	ms := tariffChapter.FindAllStringSubmatch(tariffs, -1)
	if len(ms) == 0 {
		return false
	}
	for _, m := range ms {
		if m[1] < "50" || m[1] > "63" {
			return false
		}
	}
	return true
}
