package corpus

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// The facts splitter is ported from
// hackathon-research/experiments/origin-proof/scripts/split_facts.py; the
// patterns are copied verbatim (Python re and Go RE2 agree on these). Offsets
// that Python measures in characters (3000, 600, 40) are measured in runes
// here, so the cut lands at the same place on non-ASCII text.

// startPatterns locate where the facts begin: a FACTS: heading or the
// salutation, if found in the first 3,000 characters. Tried in order.
var startPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)FACTS\s*:`),
	regexp.MustCompile(`Dear\s+[^:]{1,80}?:`),
	regexp.MustCompile(`Dear\s+[^,]{1,80}?,`),
}

// cuePatterns mark where the facts end: the first legal-standard sentence or
// heading, or a holding-like sentence.
var cuePatterns = []string{
	`ISSUES?\s*:`, `LAW AND ANALYSIS\s*:?`, `\bANALYSIS\s*:`, `\bHOLDING\s*:`, `CLASSIFICATION\s*:`,
	`When determining (the )?(country of )?origin`, `When determining the country`, `The test for determining`,
	`In determining whether (a )?substantial transformation`, `In determining (the )?country of origin`,
	`Section 102\.\d`, `\bPart 102\b`, `19 C\.?F\.?R\.? (§ ?)?1?02\.`, `19 C\.?F\.?R\.? (§ ?)?134`, `Part 134 of the`,
	`Section 304 of the Tariff Act`, `19 U\.S\.C\.? (§ ?)?1304`, `Section 134\.1`, `Pursuant to (the )?(General Note|section|19)`,
	`The applicable subheading`, `The applicable tariff provision`, `In your (letter|request), you (suggest|propose)ed classification`,
	`Classification of (merchandise|goods) under the (HTSUS|Harmonized)`, `\bGRI\s*1\b`, `General Rules? of Interpretation`,
	`The (rules? of origin|country of origin rules)`, `Country of Origin\s+(Part|Section|When|The|In|Pursuant|CBP|For|Regarding)`,
	`(The )?Trade Agreements Act`, `Title III`, `For purposes of (the )?(Section 301|trade remedies|marking|government procurement)`,
	`Concerning the country of origin`, `Regarding the (country of )?origin`, `The country of origin of (the|a|an|this|these|each|your)\b`,
	`Under 19 C`, `U\.S\. Note`, `General Note \d`, `substantial transformation (occurs|analysis)`, `A substantial transformation occurs`,
	`CBP has (consistently|previously|long) held`, `Generally,? (the )?country of origin`, `Section 2518`, `19 U\.S\.C\.? (§ ?)?25`,
	`The marking statute`, `Heading \d{4}`, `heading \d{4}, HTSUS`, `In (HQ|NY|Headquarters Ruling)`,
	// holding-ish sentences that sometimes precede any legal-standard sentence
	`does not (effect|result in|constitute) a substantial transformation`, `(is|are) (not )?substantially transformed`,
	`[Ii]t is (the|our) opinion`, `[Ii]n our opinion`, `\bTherefore,`, `\bAccordingly,`, `[Tt]he last substantial transformation`, `It has been CBP.s position`, `(NY|HQ) [NH]?\d{5,6} states as follows`,
}

var cue = regexp.MustCompile("(?:" + strings.Join(cuePatterns, ")|(?:") + ")")

// redactPattern finds sentences that state a requester's proposed origin or a
// substantial-transformation conclusion for the finished good.
var redactPattern = regexp.MustCompile(`[^.]*(?:(?:you|You|requester|client) (?:suggest|propose|believe|contend|assert|claim|maintain|state that the (?:country of )?origin)[^.]*(?:origin|substantial)|substantially transformed|substantial transformation|(?:country of origin|origin) of the (?:finished|final|imported|completed|subject)[^.]{0,80}(?:is|will be|would be|should be))[^.]*\.`)

const redaction = " [REDACTED: origin-conclusion sentence] "

var spaceRun = regexp.MustCompile(`[ \t]+`)

// SplitMeta records how a ruling was split.
type SplitMeta struct {
	Start     int    `json:"start"` // rune offset of the facts start in the normalised text
	End       int    `json:"end"`   // rune offset of the cut
	Cue       string `json:"cue"`   // matched cut cue, empty if the whole text was kept
	Redacted  int    `json:"redacted_sentences"`
	TextRunes int    `json:"len"`
}

// SplitFacts returns the facts section of a ruling's full text, with
// origin-conclusion sentences redacted, plus how it was cut.
func SplitFacts(text string) (string, SplitMeta) {
	t := spaceRun.ReplaceAllString(strings.ReplaceAll(text, "\u00a0", " "), " ")

	start := 0 // byte offset
	for _, p := range startPatterns {
		if loc := p.FindStringIndex(t); loc != nil && runes(t, loc[0]) < 3000 {
			start = loc[1]
			break
		}
	}
	startRunes := runes(t, start)

	// Skip cues inside the opening "you requested a ruling under Section 304
	// ..." sentence.
	firstEnd := start
	if i := strings.Index(t[start:], ". "); i >= 0 {
		if d := runes(t, start+i) - startRunes; d < 600 {
			firstEnd = start + i
		}
	}

	from := byteAt(t, startRunes+40)
	end, cueText := len(t), ""
	// Match on the whole text and filter by position, so \b sees the character
	// before the search start as Python's finditer(pos) does.
	for _, loc := range cue.FindAllStringIndex(t, -1) {
		if loc[0] < from || loc[0] <= firstEnd {
			continue
		}
		end, cueText = loc[0], t[loc[0]:loc[1]]
		break
	}

	facts := strings.TrimSpace(t[start:end])
	n := len(redactPattern.FindAllStringIndex(facts, -1))
	facts = redactPattern.ReplaceAllLiteralString(facts, redaction)
	return facts, SplitMeta{Start: startRunes, End: runes(t, end), Cue: cueText, Redacted: n, TextRunes: utf8.RuneCountInString(t)}
}

// runes converts a byte offset in s to a rune offset.
func runes(s string, byteOffset int) int { return utf8.RuneCountInString(s[:byteOffset]) }

// byteAt converts a rune offset in s to a byte offset (len(s) if past the end).
func byteAt(s string, runeOffset int) int {
	i := 0
	for b := range s {
		if i == runeOffset {
			return b
		}
		i++
	}
	return len(s)
}

// Truncate returns the first n runes of s, like Python's s[:n].
func Truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	return s[:byteAt(s, n)]
}
