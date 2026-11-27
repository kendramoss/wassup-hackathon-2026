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

// labeller is the holding labeller over one table of country surface forms.
// The legacy table (countrySurfaces) writes Hong Kong as China, as the table
// of label.py did; the textile table (textileSurfaces) keeps Hong Kong a
// separate origin, because CBP holds it separately for 19 CFR 102.21 (D957).
type labeller struct {
	// textile reads every "The country of origin of X is Y" sentence as a
	// holding, and a scenario holding that names two places as no single
	// origin (D957).
	textile          bool
	surfaceToCountry map[string]string
	patterns         []holdingPattern
	altCountry       *regexp.Regexp
	countryMention   *regexp.Regexp
	scenarioPlace    *regexp.Regexp
}

func newLabeller(surfaces map[string][]string, extraVerbs string) *labeller {
	l := &labeller{surfaceToCountry: map[string]string{}}
	var forms []string
	for c, fs := range surfaces {
		for _, f := range fs {
			l.surfaceToCountry[f] = c
			forms = append(forms, f)
		}
	}
	sort.Strings(forms)
	sort.SliceStable(forms, func(i, j int) bool { return len(forms[i]) > len(forms[j]) })
	q := make([]string, len(forms))
	for i, f := range forms {
		q[i] = regexp.QuoteMeta(f)
	}
	countryAlt := strings.Join(q, "|")
	l.patterns = []holdingPattern{
		{"A", regexp.MustCompile(`(?s)(?:country[- ]of[- ]origin|origin)(?:\s*,?\s*(?:of|for)\s*(?P<prod>.{0,220}?))?\s*,?\s*(?:is|are|will be|would be|shall be|should be|remains|will remain|is considered to be|will be considered(?: to be)?|is deemed to be` + extraVerbs + `)\s*(?:the\s*)?(?:same as [^.]{0,80}?i\.e\.,\s*)?(?P<c>` + countryAlt + `)(?:[^a-z]|$)`)},
		{"B", regexp.MustCompile(`(?s)(?:the\s+)?(?P<c>` + countryAlt + `)\s+(?:is|will be|would be)\s+the\s+country\s+of\s+origin(?:\s+(?:of|for)\s+(?P<prod>.{0,150}?))?[.,]`)},
		{"C", regexp.MustCompile(`(?:substantially transformed (?:in|into [^.]{0,80}? in)|origin is conferred (?:by|in)|(?:is|are|will be|would be) (?:considered )?(?:a )?products? of|origin is)\s+(?:the\s+)?(?P<c>` + countryAlt + `)(?:[^a-z]|$)`)},
		{"D", regexp.MustCompile(`(?:is|are|will be|would be) (?:of )?(?:the )?(?P<c>` + countryAlt + `)(?:n)? origin\b`)},
	}
	l.altCountry = regexp.MustCompile(`^\s*,?\s*(?:or|and)?\s*(?:the\s+)?(` + countryAlt + `)(?:[^a-z]|$)`)
	l.countryMention = regexp.MustCompile(`(` + countryAlt + `)`)
	l.scenarioPlace = regexp.MustCompile(`(` + countryAlt + `)\s+under\s+[Ss]cenarios?\s+\d`)
	return l
}

func newTextileLabeller() *labeller {
	l := newLabeller(textileSurfaces(), "|is conferred (?:in|by|upon)")
	l.textile = true
	return l
}

var (
	// holdingStart is a sentence that opens as CBP's holding paragraph does:
	// "The country of origin of the pillow shams is China" with or without a
	// "HOLDING:" lead. The legacy cue list misses the second and later
	// sentences of a multi-good holding, which carry no "Therefore".
	holdingStart = regexp.MustCompile(`^\s*(?:HOLDING\s*:\s*)?(?:[Tt]he )?[Cc]ountry of origin (?:of|for|is)\b`)
	// scenarioCue marks a sentence that opens with a condition: "When the
	// foundation cover is made from fabrics wholly manufactured in China, the
	// country of origin is China".
	scenarioCue = regexp.MustCompile(`^\s*(?:when|where)\b|(?i:\bscenarios?\s+\d)`)
)

// textileSurfaces is countrySurfaces with Hong Kong (and Macau, which already
// is) a place of its own: for 19 CFR 102.21 CBP rules on the country where a
// knit-to-shape garment was knit, and that can be Hong Kong while the garment
// is assembled in China.
func textileSurfaces() map[string][]string {
	out := make(map[string][]string, len(countrySurfaces)+1)
	for c, fs := range countrySurfaces {
		out[c] = fs
	}
	var china []string
	for _, f := range countrySurfaces["China"] {
		if f != "Hong Kong" {
			china = append(china, f)
		}
	}
	out["China"] = china
	out["Hong Kong"] = []string{"Hong Kong"}
	out["United States"] = append(append([]string{}, countrySurfaces["United States"]...), "US")
	return out
}

var (
	legacyLabeller  = newLabeller(countrySurfaces, "")
	textileLabeller = newTextileLabeller()
)

type holdingPattern struct {
	name string
	re   *regexp.Regexp
}

var (
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
	// holding: the sentence is CBP's HOLDING paragraph. The legacy labeller
	// asks for the word in the sentence; the textile labeller also takes the
	// sentences after it, since a holding on several goods is one sentence each.
	holding bool
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
func (lb *labeller) extractHoldings(analysis string) []holdingHit {
	var hits []holdingHit
	inHolding := false
	for _, s := range sentences(analysis) {
		s1 := space.ReplaceAllString(s, " ")
		if strings.Contains(s1, "HOLDING") {
			inHolding = true
		}
		for _, p := range lb.patterns {
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
				country := lb.surfaceToCountry[s1[m[2*ci]:m[2*ci+1]]]
				tail := s1[m[2*ci+1]:]
				tail = tail[:byteAt(tail, 160)]
				for {
					am := lb.altCountry.FindStringSubmatchIndex(tail)
					if am == nil {
						break
					}
					country += " | " + lb.surfaceToCountry[tail[am[2]:am[3]]]
					tail = tail[am[3]:]
				}
				hits = append(hits, holdingHit{product: prod, country: country, sentence: Truncate(s1, 400),
					pattern: p.name, holding: strings.Contains(s1, "HOLDING") || (lb.textile && inHolding), concl: conclusionCue.MatchString(s1) || (lb.textile && (holdingStart.MatchString(s1) || scenarioCue.MatchString(s1)))})
			}
		}
	}
	return hits
}

// chooseHoldings keeps the conclusions, preferring HOLDING sentences and
// patterns A and B, one per product, the last statement winning (choose in
// label.py).
func chooseHoldings(hits []holdingHit, textile bool) []holdingHit {
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
	pool = pick(pool, func(h holdingHit) bool { return h.holding })
	pool = pick(pool, func(h holdingHit) bool { return h.pattern == "A" || h.pattern == "B" })
	var order []string
	byKey := map[string]holdingHit{}
	for _, h := range pool {
		key := Truncate(strings.TrimSpace(nonWord.ReplaceAllString(strings.ToLower(h.product), " ")), 80)
		if key == "" && textile {
			// Without "of <good>" the sentence is the holding: two goods with
			// different origins must not collapse into one (D957).
			key = "sentence:" + Truncate(strings.TrimSpace(nonWord.ReplaceAllString(strings.ToLower(h.sentence), " ")), 160)
		}
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
	return legacyLabeller.label(id, rulingDate, collection, subject, text, meta)
}

// LabelHoldingTextile labels a 19 CFR 102.21 ruling: LabelHolding with Hong
// Kong as a place of its own (D957). The legacy labeller wrote China for
// 2019-2020 knit-to-shape sweaters whose holding says Hong Kong, 34 of the 122
// decided rulings of D956.
func LabelHoldingTextile(id, rulingDate, collection, subject, text string, meta SplitMeta) Label {
	l := textileLabeller.label(id, rulingDate, collection, subject, text, meta)
	l.Regime = ClassifyRegime(text, l.MultiCountry)
	return l
}

func (lb *labeller) label(id, rulingDate, collection, subject, text string, meta SplitMeta) Label {
	t := spaceRun.ReplaceAllString(strings.ReplaceAll(text, "\u00a0", " "), " ")
	facts := t[byteAt(t, meta.Start):byteAt(t, meta.End)]
	hits := chooseHoldings(lb.extractHoldings(t[byteAt(t, meta.End):]), lb.textile)
	l := Label{
		Ruling: id, Date: rulingDate[:min(10, len(rulingDate))], Collection: strings.ToUpper(collection), Subject: subject,
		Regime: "part102_tariff_shift", MultiCountry: len(lb.factsCountries(facts)) >= 2, Method: "regex", Evidence: []string{},
	}
	same := len(hits) > 0
	for _, h := range hits {
		same = same && h.country == hits[0].country
		if lb.textile && (len(lb.scenarioPlaces(h.sentence)) >= 2 || lb.conditionalPlaces(h.sentence)) {
			same = false // "Vietnam under Scenario 1 and China under Scenarios 2 and 3"
		}
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

var conditionCue = regexp.MustCompile(`(?i)\b(?:when|if|in the event)\b`)

// conditionalPlaces reports a holding that names two places and ties them to a
// condition: "is conferred in India when the fabric is woven in India and in
// Pakistan when the fabric is woven in Pakistan".
func (lb *labeller) conditionalPlaces(s string) bool {
	i := strings.Index(s, "origin")
	if i < 0 {
		return false
	}
	tail := s[i:]
	if !conditionCue.MatchString(tail) {
		return false
	}
	places := map[string]bool{}
	for _, m := range lb.countryMention.FindAllString(tail, -1) {
		places[lb.surfaceToCountry[m]] = true
	}
	return len(places) >= 2
}

// scenarioPlaces returns the distinct places a sentence ties to a numbered
// scenario ("Vietnam under Scenario 1"). One place "under both scenarios" is
// one holding, not a scenario-dependent one.
func (lb *labeller) scenarioPlaces(s string) map[string]bool {
	out := map[string]bool{}
	for _, m := range lb.scenarioPlace.FindAllStringSubmatch(s, -1) {
		out[lb.surfaceToCountry[m[1]]] = true
	}
	return out
}

// factsCountries returns the distinct countries named in the facts, without
// "United States" as the import destination (facts_countries in label.py,
// simplified: only the "to/into the United States" context is skipped).
func (lb *labeller) factsCountries(facts string) map[string]bool {
	out := map[string]bool{}
	for _, m := range lb.countryMention.FindAllStringSubmatchIndex(facts, -1) {
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
		c := lb.surfaceToCountry[facts[s:e]]
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

// CountryMentions counts the countries named in a facts section, with the
// same matching as factsCountries ("United States" as the import destination
// is skipped), and returns them in order of first mention. It feeds the
// rule baselines in `eval baselines` (most-mentioned, first and last country).
func CountryMentions(facts string) (counts map[string]int, order []string) {
	counts = map[string]int{}
	for _, m := range legacyLabeller.countryMention.FindAllStringSubmatchIndex(facts, -1) {
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
		c := legacyLabeller.surfaceToCountry[facts[s:e]]
		if c == "United States" && importContext.MatchString(strings.ToLower(lastRunes(facts[:s], 40))) {
			continue
		}
		if counts[c] == 0 {
			order = append(order, c)
		}
		counts[c]++
	}
	return counts, order
}
