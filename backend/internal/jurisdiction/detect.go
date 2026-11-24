package jurisdiction

import (
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// DetectorVersion names the cue lists below; a change to them needs a bump
// and a re-run of the D680 checks.
const DetectorVersion = "market-detect-v3"

// places maps a place a question can name as its destination to its market.
// Lower case; matched on word boundaries.
var places = map[string][]string{
	US: {"the united states", "united states", "the u.s.", "the us", "the usa", "usa", "u.s.", "america"},
	EU: {"the european union", "european union", "the eu", "eu", "the eu customs territory", "europe",
		"germany", "france", "italy", "spain", "the netherlands", "netherlands", "belgium", "poland", "czechia", "the czech republic", "czech republic",
		"austria", "sweden", "denmark", "finland", "ireland", "portugal", "greece", "hungary", "romania", "bulgaria", "croatia",
		"slovakia", "slovenia", "lithuania", "latvia", "estonia", "luxembourg", "malta", "cyprus",
		"paris", "berlin", "hamburg", "munich", "frankfurt", "rotterdam", "amsterdam", "antwerp", "brussels", "madrid", "barcelona",
		"milan", "rome", "warsaw", "vienna", "dublin", "lisbon", "stockholm", "copenhagen", "prague", "athens", "budapest",
		"valencia", "genoa", "le havre", "marseille", "gdansk", "piraeus", "lyon", "turin", "naples", "bremen", "cologne", "düsseldorf"},
	UK: {"the united kingdom", "united kingdom", "the uk", "uk", "great britain", "britain", "england", "scotland", "wales",
		"london", "birmingham", "manchester", "felixstowe", "southampton", "glasgow", "edinburgh", "liverpool", "leeds", "dover", "bristol"},
	CA: {"canada", "toronto", "vancouver", "montreal", "calgary", "ottawa", "edmonton", "winnipeg", "halifax", "prince rupert"},
	JP: {"japan", "tokyo", "osaka", "yokohama", "nagoya", "kobe", "kyoto", "fukuoka"},
	KR: {"south korea", "korea", "the republic of korea", "republic of korea", "seoul", "busan", "incheon"},
	AU: {"australia", "sydney", "melbourne", "brisbane", "perth", "adelaide", "fremantle"},
	MX: {"mexico", "méxico", "mexico city", "ciudad de méxico", "guadalajara", "monterrey", "tijuana", "manzanillo", "veracruz", "nuevo laredo", "lázaro cárdenas"},
	Other: {"china", "india", "brazil", "singapore", "saudi arabia", "the united arab emirates", "united arab emirates", "the uae", "uae",
		"switzerland", "norway", "turkey", "türkiye", "israel", "south africa", "new zealand", "indonesia", "malaysia", "thailand",
		"vietnam", "the philippines", "philippines", "taiwan", "hong kong", "chile", "colombia", "peru", "argentina", "egypt",
		"nigeria", "kenya", "pakistan", "bangladesh", "russia", "ukraine", "qatar", "kuwait", "dubai", "mumbai", "shanghai", "são paulo", "sao paulo",
		"durban", "johannesburg", "cape town", "santos", "jebel ali", "riyadh", "jeddah", "chennai", "delhi", "jakarta", "bangkok",
		"ho chi minh city", "hanoi", "manila", "kuala lumpur", "istanbul", "tel aviv", "auckland", "zurich", "oslo"},
}

// northernIreland are places in Northern Ireland. Under the Windsor
// Framework the EU's customs rules apply to many goods entering Northern
// Ireland, so naming it never picks the UK (or the EU): the case asks the
// user to pick the market (D767).
var northernIreland = []string{"northern ireland", "belfast", "londonderry", "derry"}

// CueNorthernIreland is the cue of a case read as Northern Ireland.
const CueNorthernIreland = "Northern Ireland"

// NorthernIrelandReason is the triage reason for such a case.
const NorthernIrelandReason = "Goods entering Northern Ireland can fall under EU or UK rules. Pick the market to get an answer"

// adjectives maps a market adjective ("German customs") to its market.
var adjectives = map[string][]string{
	US:    {"u.s.", "us", "united states", "american"},
	EU:    {"eu", "european", "union", "german", "french", "dutch", "italian", "spanish", "polish", "belgian", "irish", "swedish", "danish", "austrian", "portuguese", "czech", "finnish", "greek"},
	UK:    {"uk", "british"},
	CA:    {"canadian"},
	JP:    {"japanese", "japan"},
	KR:    {"korean", "south korean", "korea"},
	AU:    {"australian"},
	MX:    {"mexican"},
	Other: {"indian", "brazilian", "chinese", "singapore", "singaporean", "saudi", "emirati", "swiss", "norwegian", "turkish", "israeli", "south african", "new zealand", "indonesian", "malaysian", "thai", "vietnamese", "philippine", "taiwanese"},
}

// authorities are names of an origin authority, law or tariff: a strong cue
// on their own. Case-sensitive patterns are marked (?-i).
var authorities = []struct {
	market, re, cue string
}{
	{US, `(?-i:\bCBP\b)|customs and border protection`, "CBP"},
	{US, `\b19 ?c\.? ?f\.? ?r\b`, "19 CFR"},
	{US, `(?-i:\bHTSUS?\b)|harmonized tariff schedule of the united states`, "HTSUS"},
	{US, `\bsection (?:301|232)\b`, "Section 301/232"},
	{EU, `binding origin information|(?-i:\bBOIs?\b)`, "BOI"},
	{EU, `union customs code|(?-i:\bTARIC\b)|combined nomenclature|free circulation|common customs tariff|(?:european |eu )?single market|\beu[- ]\w+ customs union`, "EU customs law"},
	{EU, `agencia tributaria|agenzia delle dogane|\bdouanes?\b|(?-i:\bZoll(?:amt)?\b)`, "an EU customs agency"},
	{UK, `(?-i:\bHMRC\b)|hm revenue|uk global tariff`, "HMRC"},
	{CA, `(?-i:\bCBSA\b)|canada border services|(?-i:\bCARM\b)`, "CBSA"},
	{JP, `japan customs`, "Japan Customs"},
	{KR, `korea customs service|(?-i:\bKCS\b)`, "Korea Customs Service"},
	{AU, `australian border force|(?-i:\bABF\b)`, "Australian Border Force"},
	{MX, `(?-i:\bSAT\b)|(?-i:\bANAM\b)|servicio de administración tributaria|importaci[oó]n (?:a|en) m[eé]xico|aduanas? de m[eé]xico|ley aduanera`, "Mexican customs"},
	{Other, `(?-i:\bCBIC\b)|receita federal|singapore customs|(?-i:\bZATCA\b)|(?-i:\bGACC\b)|south african revenue service`, "a non-U.S. customs authority"},
}

// currencies are weak cues: a price alone does not say where goods enter.
// A code needs an amount beside it or "in" before it ("CAD drawings" is not
// a price).
var currencies = []struct{ market, re string }{
	{EU, `€|` + code("EUR") + `|\d\s?euros?\b|\bin euros\b`},
	{UK, `£|` + code("GBP") + `|pounds? sterling`},
	{CA, `(?-i:\bC\$\s?\d)|` + code("CAD") + `|canadian dollars?`},
	{JP, code("JPY") + `|\d\s?yen\b|\bin yen\b`},
	{KR, `₩|` + code("KRW") + `|korean won|\bin won\b`},
	{AU, `(?-i:\bA\$\s?\d)|` + code("AUD") + `|australian dollars?`},
	{MX, code("MXN") + `|mexican pesos?|\d\s?pesos\b`},
}

// code matches a currency code next to an amount, or after "in".
func code(c string) string {
	return `(?-i:\d[\d,.]*\s?` + c + `\b|\b` + c + `\s?\d|\bin ` + c + `\b)`
}

// Destination verbs. Entry verbs say the goods enter the place; send verbs
// say they go or sell there, which a CBP ruling also says of a stop on the
// way. Either is dropped when the place is a processing stop (stop).
const (
	entryVerb = `(?:(?:import(?:ed|ing|s)?|importation|entry|entered|release[sd]?(?: for free circulation)?)\s+(?:\w+\s+){0,2}?(?:into|to|in)\s+` +
		`|(?:bring(?:s|ing)?|brought)\s+(?:\S+\s+){0,6}?into\s+|enter(?:s|ing)?\s+(?:into\s+)?|landing\s+(?:\w+\s+){0,3}?in\s+` +
		`|arriv(?:e|es|ed|ing)\s+(?:\w+\s+){0,2}?(?:in|at)\s+|customs\s+(?:port\s+)?(?:in|at|of)\s+|port\s+of\s+)`
	sendVerb = `(?:ship(?:ped|ping|s)?|shipment|export(?:ed|ing|s)?|sell(?:ing|s)?|sold|sale|distribut(?:e|ed|ing))\s+(?:\w+\s+){0,3}?(?:into|to|in)\s+`
	// "for UK customs", "Japanese customs rules", "Korean import".
	customsNoun = `\s+(?:customs|import|imports|border|authorities|authority|tariff)\b`
)

// Cue kinds, strongest first.
const (
	kindAuthority = iota // an authority, a law or "<market> customs"
	kindEntry            // entry into a place
	kindWeak             // a price, or goods sent or sold to a place
)

type cueRE struct {
	market, cue string
	re          *regexp.Regexp
	kind        int
	// dest is set for "<verb> into <place>" cues; group 1 is the place.
	dest bool
	// notBefore drops a match followed by these words ("Malaysian customs
	// declaration" is the export side).
	notBefore *regexp.Regexp
}

var cues = buildCues()

func alt(names []string) string {
	s := slices.Clone(names)
	sort.Slice(s, func(i, j int) bool { return len(s[i]) > len(s[j]) }) // longest first
	for i, n := range s {
		s[i] = regexp.QuoteMeta(n)
	}
	return strings.Join(s, "|")
}

var exportSide = regexp.MustCompile(`(?i)^\s*(?:declaration|export|form|document|record|invoice|clearance)`)

func buildCues() []cueRE {
	var out []cueRE
	for _, a := range authorities {
		out = append(out, cueRE{market: a.market, cue: a.cue, re: regexp.MustCompile(`(?i)` + a.re), kind: kindAuthority})
	}
	end := `(?:[^\p{L}\p{N}]|$)`
	for _, c := range Codes() {
		p := alt(places[c])
		send := kindWeak
		if c == US {
			// "shipped to the United States" names U.S. entry: the United
			// States is never a processing stop in a CBP ruling's facts.
			send = kindEntry
		}
		out = append(out,
			cueRE{market: c, cue: "entry into " + Name(c), re: regexp.MustCompile(`(?i)\b` + entryVerb + `(` + p + `)` + end), kind: kindEntry, dest: true},
			cueRE{market: c, cue: "goods sent to " + Name(c), re: regexp.MustCompile(`(?i)\b` + sendVerb + `(` + p + `)` + end), kind: send, dest: true},
			cueRE{market: c, cue: Name(c) + " customs", re: regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}])(?:` + alt(adjectives[c]) + `)` + customsNoun), kind: kindAuthority, notBefore: exportSide},
		)
	}
	ni := alt(northernIreland)
	out = append(out,
		cueRE{market: UK, cue: CueNorthernIreland, re: regexp.MustCompile(`(?i)\b` + entryVerb + `(` + ni + `)` + end), kind: kindEntry, dest: true},
		cueRE{market: UK, cue: CueNorthernIreland, re: regexp.MustCompile(`(?i)\b` + sendVerb + `(` + ni + `)` + end), kind: kindWeak, dest: true},
		cueRE{market: UK, cue: CueNorthernIreland, re: regexp.MustCompile(`(?i)\b(?:to|into|in|at)\s+(?:our\s+|a\s+|the\s+)?(` + ni + `)` + end), kind: kindWeak, dest: true},
	)
	for _, c := range currencies {
		out = append(out, cueRE{market: c.market, cue: "a price in " + Name(c.market) + " currency", re: regexp.MustCompile(`(?i)` + c.re), kind: kindWeak})
	}
	return out
}

type hit struct {
	market, cue string
	kind, pos   int
}

// Detect reads a case's market from its text, without a model call.
//
//   - Authority cues (a customs authority, a law, "<market> customs") decide
//     first. Non-U.S. ones alone give that market; U.S. ones alone give the
//     United States; both give the United States with Conflict set.
//   - Otherwise entry cues ("imported into Canada", and for the United
//     States also "shipped to the United States") decide: the market of the
//     last one in the text, since a ruling's facts name stops on the way
//     before the destination. A different market's entry cue is a Hint.
//   - Weak cues (a price, "sold in", "shipped to") never set a market; a
//     non-U.S. one is a Hint.
//   - No cue: the United States.
func Detect(text string) domain.Jurisdiction {
	j := detect(text)
	// Northern Ireland (D767): EU or UK rules can apply, so a reading of the
	// UK, or no market, that rests on a Northern Ireland place asks the
	// user instead.
	if j.Code == UK || j.Source == SourceDefault && j.Hint == UK {
		for _, c := range cues {
			if c.cue == CueNorthernIreland && len(c.find(text)) > 0 {
				return domain.Jurisdiction{Code: Other, Source: SourceDetected, Cue: CueNorthernIreland, Hint: UK, Conflict: true}
			}
		}
	}
	return j
}

func detect(text string) domain.Jurisdiction {
	var hits []hit
	for _, c := range cues {
		for _, loc := range c.find(text) {
			hits = append(hits, hit{c.market, c.cue, c.kind, loc[0]})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].pos < hits[j].pos })
	byKind := func(k int) (us, other []hit) {
		for _, h := range hits {
			if h.kind != k {
				continue
			}
			if h.market == US {
				us = append(us, h)
			} else {
				other = append(other, h)
			}
		}
		return us, other
	}
	weakHint := func() string {
		_, o := byKind(kindWeak)
		return top(o)
	}
	if us, other := byKind(kindAuthority); len(us)+len(other) > 0 {
		switch {
		case len(other) == 0:
			j := domain.Jurisdiction{Code: US, Source: SourceDetected, Cue: us[0].cue}
			if _, oe := byKind(kindEntry); len(oe) > 0 {
				j.Hint = top(oe)
			} else {
				j.Hint = weakHint()
			}
			return j
		case len(us) == 0:
			m := top(other)
			return domain.Jurisdiction{Code: m, Source: SourceDetected, Cue: firstCue(other, m)}
		default:
			return domain.Jurisdiction{Code: US, Source: SourceDetected, Cue: us[0].cue, Hint: top(other), Conflict: true}
		}
	}
	if us, other := byKind(kindEntry); len(us)+len(other) > 0 {
		all := append(append([]hit(nil), us...), other...)
		sort.SliceStable(all, func(i, j int) bool { return all[i].pos < all[j].pos })
		last := all[len(all)-1]
		j := domain.Jurisdiction{Code: last.market, Source: SourceDetected, Cue: last.cue}
		if last.market == US {
			j.Hint = top(other)
			if j.Hint == "" {
				j.Hint = weakHint()
			}
		} else if len(us) > 0 {
			j.Hint = US
		}
		return j
	}
	return domain.Jurisdiction{Code: US, Source: SourceDefault, Hint: weakHint()}
}

// top is the market with the most hits (ties: the one named first); "" for
// none.
func top(hs []hit) string {
	n := map[string]int{}
	best := ""
	for _, h := range hs {
		n[h.market]++
		if best == "" || n[h.market] > n[best] {
			best = h.market
		}
	}
	return best
}

func firstCue(hs []hit, market string) string {
	for _, h := range hs {
		if h.market == market {
			return h.cue
		}
	}
	return ""
}

// Match is one cue found in a text, for error analysis (cmd/eval market).
type Match struct {
	Market, Cue, Snippet string
	Strong               bool
}

// Matches lists every cue Detect sees in text, with 60 characters around it.
func Matches(text string) []Match {
	var out []Match
	for _, c := range cues {
		for _, loc := range c.find(text) {
			lo, hi := max(0, loc[0]-60), min(len(text), loc[1]+30)
			out = append(out, Match{c.market, c.cue, strings.ReplaceAll(text[lo:hi], "\n", " "), c.kind != kindWeak})
		}
	}
	return out
}

// find returns the cue's matches, without the destination matches whose
// place is a processing stop.
func (c cueRE) find(text string) [][]int {
	if !c.dest {
		var out [][]int
		for _, m := range c.re.FindAllStringIndex(text, -1) {
			if c.notBefore == nil || !c.notBefore.MatchString(text[m[1]:]) {
				out = append(out, m)
			}
		}
		return out
	}
	var out [][]int
	for _, m := range c.re.FindAllStringSubmatchIndex(text, -1) {
		if c.market == US || !stop(text, m[0], m[1], text[m[2]:m[3]]) {
			out = append(out, m[:2])
		}
	}
	return out
}

// stopAfter matches the words after "imported into <place>" that make the
// place a processing stop: "for further processing", "where they are
// assembled", "to be assembled".
var stopAfter = regexp.MustCompile(`(?i)^[^.;]{0,40}?\b(?:where|for (?:further |final )?(?:\w+ing|processing|assembly|inspection|manufactur\w*)|in \w+ (?:\w+ )?form|to be \w+ed|to make|to produce|and (?:then )?(?:assembled|processed|made|tested|cut|woven|packed)|by (?:inserting|installing|assembling))\b`)

// stopBefore matches "Once imported into", "After importation into",
// "initially imported into": a step in a production history.
var stopBefore = regexp.MustCompile(`(?i)\b(?:once|after|initially|first|originally|then|subsequently|later)\s*$`)

// processVerb is work done at a place ("assembled in Thailand", "In
// Thailand, the parts are ...").
const processVerb = `(?:assembl|process|manufactur|produc|made|make|finish|convert|mill|weav|knit|sew|cut|mount|install|test|inspect|pack|blend|refin|undergo|machin|weld|mold|mould|print|bottl|fill)\w*`

// stop reports whether place, named as a destination ending at end, is
// rather a processing stop: the next words say work is done there, or the
// text says work is done in it elsewhere. A CBP ruling's facts often say
// parts are "imported into Thailand for further processing".
func stop(text string, start, end int, place string) bool {
	if stopAfter.MatchString(text[end:]) || stopBefore.MatchString(text[:start]) {
		return true
	}
	q := regexp.QuoteMeta(strings.TrimPrefix(strings.ToLower(place), "the "))
	re := regexp.MustCompile(`(?i)\b` + processVerb + `\s+(?:\w+\s+){0,3}?in\s+(?:the\s+)?` + q + `\b|(?:^|[.;]\s*)(?:also\s+)?in\s+(?:the\s+)?` + q + `\s*,`)
	return re.MatchString(text)
}
