package export

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// requestBoilerplate are phrases of CBP's reply to a ruling request ("In your
// letter dated…, you requested…", "With regard to your request for…,"). A
// case's facts are often pasted from a CBP ruling, so they carry CBP's voice;
// a request letter from the requester must not (D180).
var requestBoilerplate = []string{
	"in your letter dated",
	"in your letter of",
	"this is in response to your letter",
	"this is in reply to your letter",
	"this is in response to your request",
	"this is in reply to your request",
	"you requested a",
	"you have requested a",
	"your request was",
	"with regard to your request",
	"in regard to your request",
	"with respect to your request",
	"in response to your request",
}

// NoFactsPlaceholder stands in for facts that were only CBP's reply wording,
// so the letter never speaks in CBP's voice and is never left without a
// facts line (D250).
const NoFactsPlaceholder = "[FILL IN: a description of the goods and how they are produced]."

// CleanFacts drops the sentences of facts that are CBP's reply boilerplate
// rather than facts about the goods, and a dangling last fragment that ends
// in a comma (where the ruling text was cut before CBP's analysis). Every
// other sentence is kept word for word, in order (D180, D250).
//
// Line endings are normalised first (CRLF, CR, vertical tab and form feed
// to LF). A blank line, or a
// line break after the end of a sentence, starts a new paragraph; a line
// break inside a sentence (a wrapped line, a date on its own line) does not.
// Sentences are not cut at common abbreviations (Mfg., Co., Inc., S.A.,
// de C.V., U.S.A., No., e.g., single initials), so a dropped sentence never
// leaves a fragment and never takes a fact sentence with it.
//
// If nothing would be left, the facts are returned with only the
// boilerplate sentences removed; if they were nothing but boilerplate, a
// [FILL IN] placeholder is returned.
func CleanFacts(facts string) string {
	sents := splitFacts(facts)
	keep := make([]bool, len(sents))
	kept := false
	for i, s := range sents {
		keep[i] = !boilerplate(s.text)
		kept = kept || keep[i]
	}
	if !kept {
		return NoFactsPlaceholder
	}
	noBoiler := joinKept(sents, keep)
	last := -1
	for i := range sents {
		if keep[i] {
			last = i
		}
	}
	if strings.HasSuffix(sents[last].text, ",") {
		keep[last] = false
		if out := joinKept(sents, keep); out != "" {
			return out
		}
	}
	return noBoiler
}

func boilerplate(s string) bool {
	l := strings.ToLower(strings.Join(strings.Fields(s), " "))
	for _, b := range requestBoilerplate {
		if strings.Contains(l, b) {
			return true
		}
	}
	return false
}

// sentence is one sentence of the facts, as written (trimmed), and how it is
// separated from the next one.
type sentence struct {
	text string
	sep  int
}

const (
	sepNone  = iota // "ruling.The": no space in the source
	sepSpace        // the next sentence follows on the same line
	sepPara         // a blank line, or a line break after a sentence's end
)

var blankLine = regexp.MustCompile(`\n[^\S\n]*\n\s*`)

// splitFacts normalises line endings and splits facts into sentences.
func splitFacts(facts string) []sentence {
	s := normaliseBreaks(facts)
	var out []sentence
	for _, p := range blankLine.Split(strings.TrimSpace(s), -1) {
		ps := splitSentences(p)
		if len(ps) == 0 {
			continue
		}
		if n := len(out); n > 0 {
			out[n-1].sep = sepPara
		}
		out = append(out, ps...)
	}
	return out
}

// lineBreaks are the line and paragraph breaks facts come with: CRLF, CR,
// vertical tab, form feed and the Unicode separators, all made LF.
var lineBreaks = strings.NewReplacer("\r\n", "\n", "\r", "\n", "\v", "\n", "\f", "\n", "\u2028", "\n", "\u2029", "\n\n")

func normaliseBreaks(s string) string { return lineBreaks.Replace(s) }

// splitSentences splits one paragraph after each sentence's end.
func splitSentences(p string) []sentence {
	var out []sentence
	start := 0
	i := 0
	for i < len(p) {
		c := p[i]
		if c != '.' && c != '!' && c != '?' {
			i++
			continue
		}
		// The run of end marks and closing quotes or brackets.
		j := i
		for j < len(p) && strings.IndexByte(".!?", p[j]) >= 0 {
			j++
		}
		marks := p[i:j]
		for j < len(p) && strings.IndexByte(`)]"'`, p[j]) >= 0 {
			j++
		}
		for strings.HasPrefix(p[j:], "”") || strings.HasPrefix(p[j:], "’") {
			j += len("”")
		}
		k := j
		for k < len(p) {
			r, n := utf8.DecodeRuneInString(p[k:])
			if !unicode.IsSpace(r) {
				break
			}
			k += n
		}
		word := wordBefore(p, i)
		var end bool
		sep := sepSpace
		switch {
		case k == len(p):
			end = true
		case k == j: // no space: "ruling.The"
			end = marks == "." && startsUpper(p[k:]) && joinedEnd(word)
			sep = sepNone
		default:
			end = sentenceEnd(word, marks, p[k:])
			if strings.Contains(p[j:k], "\n") {
				sep = sepPara
			}
		}
		if !end {
			i = j
			continue
		}
		if t := strings.TrimSpace(p[start:j]); t != "" {
			out = append(out, sentence{text: t, sep: sep})
		}
		start, i = k, k
	}
	if t := strings.TrimSpace(p[start:]); t != "" {
		out = append(out, sentence{text: t, sep: sepSpace})
	}
	return out
}

// joinKept writes the kept sentences back. Where sentences were dropped
// between two kept ones, the strongest break in the gap is used.
func joinKept(sents []sentence, keep []bool) string {
	var b strings.Builder
	pending := -1
	for i, s := range sents {
		if keep[i] {
			if b.Len() > 0 {
				switch pending {
				case sepPara:
					b.WriteString("\n\n")
				case sepSpace:
					b.WriteString(" ")
				}
			}
			b.WriteString(s.text)
			pending = s.sep
			continue
		}
		if b.Len() > 0 && s.sep > pending {
			pending = s.sep
		}
	}
	return b.String()
}

// Abbreviations, lower case, without the final period. neverEnd never ends a
// sentence; mayEnd ends one only when an ordinary capitalised word follows
// ("…of Acme Co. The motor…"), not another abbreviation ("Mfg. Co. Inc.").
var (
	neverEnd = set("mr", "mrs", "ms", "messrs", "dr", "prof", "st", "no", "nos", "e.g", "i.e", "eg", "ie",
		"vs", "cf", "approx", "fig", "figs", "ref", "pp", "incl", "u.s.c", "c.f.r", "jan", "feb",
		"apr", "jun", "jul", "aug", "sept", "oct", "nov", "mt", "mfr", "nr", "tel")
	mayEnd = set("inc", "co", "corp", "ltd", "llc", "mfg", "s.a", "c.v", "u.s", "u.s.a", "u.k",
		"s.p.a", "b.v", "n.v", "s.r.l", "s.a.s", "s.a.c", "l.l.c", "l.p", "pte", "pty", "plc", "bros",
		"intl", "int'l", "ind", "assn", "dept", "etc", "s.l", "a.g", "k.k", "sdn", "bhd", "cia", "cía")
	// companySuffix after a period continues a company's name.
	companySuffix = set("inc", "co", "corp", "ltd", "llc", "l.l.c", "mfg", "plc", "pte", "pty", "s.a", "l.p")
	// usNouns follow "U.S." used as an adjective, not as the end of a sentence.
	usNouns = set("customs", "border", "government", "department", "court", "patent", "international",
		"trade", "food", "federal", "code", "dollars", "dollar", "mint", "army", "navy", "air", "army's",
		"environmental", "consumer", "fish", "district", "supreme", "postal", "coast", "census", "fda",
		"cbp", "epa", "usda", "fcc", "importer", "importers", "market", "manufacturer", "company",
		"subsidiary", "affiliate", "port", "ports", "plant", "facility", "facilities", "origin",
		"content", "parts", "components", "materials", "steel", "producer", "producers", "customer",
		"customers", "retail", "retailer", "retailers", "distributor", "distributors", "military",
		"patent", "copyright", "trademark", "and", "or")
)

// starters are words that usually open a sentence in a ruling's facts.
var starters = set("The", "This", "These", "That", "Those", "It", "Its", "They", "There", "You", "Your",
	"We", "Our", "In", "Each", "A", "An", "All", "After", "Before", "Then", "Finally", "Next", "Both",
	"When", "While", "As", "At", "For", "From", "On", "Once", "Upon", "During", "Since", "If", "Because",
	"Following", "Prior", "Under", "With", "Based", "According", "Additionally", "Also", "However",
	"Further", "Furthermore", "Moreover", "Subsequently", "Thereafter", "Lastly", "First", "Second",
	"Third", "No", "Neither", "None", "Some", "Such", "To", "Once", "One", "Two", "Three")

func set(ws ...string) map[string]bool {
	m := make(map[string]bool, len(ws))
	for _, w := range ws {
		m[w] = true
	}
	return m
}

// sentenceEnd says whether end marks after word, followed by space and then
// next, end a sentence.
func sentenceEnd(word, marks, next string) bool {
	r, _ := utf8.DecodeRuneInString(next)
	if unicode.IsLower(r) {
		return false // "Inc. of Detroit", "e.g. the motor"
	}
	if !unicode.IsUpper(r) && !unicode.IsDigit(r) && !strings.ContainsRune(`"'“‘([`, r) {
		return false // "Inc. , the importer"
	}
	if marks != "." {
		return true // "!", "?", "C.V..", "..."
	}
	if nw := nextWord(next); strings.HasSuffix(nw, ".") && companySuffix[strings.ToLower(strings.TrimSuffix(nw, "."))] {
		return false // "Technology, US. LLC."
	}
	w := strings.ToLower(strings.Trim(word, `"'“‘(`))
	switch {
	case w == "":
		return true
	case utf8.RuneCountInString(w) == 1 && unicode.IsLetter([]rune(w)[0]):
		// An initial ("J. Smith") unless a usual sentence opening follows
		// ("…for Tesla Model Y. The panel…").
		return starters[nextWord(next)]
	case neverEnd[w]:
		return false
	case mayEnd[w] || isDotted(w):
		nw := nextWord(next)
		if nw == "" {
			return false // "Ltd. (Acme)", "Co. “Brand”"
		}
		lw := strings.ToLower(strings.TrimSuffix(nw, "."))
		if mayEnd[lw] || neverEnd[lw] || isDotted(lw) || strings.HasSuffix(nw, ".") && len(lw) <= 4 {
			return false // "Mfg. Co. Inc.", "S.A. de C.V."
		}
		if (w == "u.s" || w == "u.s.a") && usNouns[lw] {
			return false // "U.S. Customs and Border Protection"
		}
		return true
	}
	return true
}

// wordBefore is the word that ends at p[i] (the end mark itself excluded).
func wordBefore(p string, i int) string {
	s := strings.LastIndexFunc(p[:i], unicode.IsSpace)
	if s < 0 {
		return p[:i]
	}
	_, n := utf8.DecodeRuneInString(p[s:])
	return p[s+n : i]
}

// nextWord is the word that starts next, with its own end mark if any; ""
// when next does not start with a letter.
func nextWord(next string) string {
	r, _ := utf8.DecodeRuneInString(next)
	if !unicode.IsLetter(r) {
		return ""
	}
	e := strings.IndexFunc(next, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(`,;:)]"”`, r)
	})
	if e < 0 {
		return next
	}
	return next[:e]
}

// isDotted reports a dotted abbreviation of single letters: "u.s", "s.a".
func isDotted(w string) bool {
	if !strings.Contains(w, ".") {
		return false
	}
	for _, part := range strings.Split(w, ".") {
		if utf8.RuneCountInString(part) > 2 {
			return false
		}
	}
	return true
}

// joinedEnd says whether a period with no space after it ends a sentence:
// "ruling.The", "(Quebec, Canada).You", but not "U.S.A" or "Co.Ltd".
func joinedEnd(word string) bool {
	w := strings.TrimRight(word, `)]"'”’`)
	lw := strings.ToLower(strings.TrimLeft(w, `(["'“‘`))
	if utf8.RuneCountInString(lw) < 2 || neverEnd[lw] || mayEnd[lw] || isDotted(lw) {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(w)
	return unicode.IsLower(r) || len(w) < len(word) // "(USMCA).The"
}

func startsUpper(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsUpper(r)
}
