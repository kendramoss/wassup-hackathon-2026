package hts

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Part classifications (D380). Many CBP rulings classify the parts of a good
// in a sentence of their own ("The aluminum pegs will be classified in
// subheading 8302.50, HTSUS."). PartStatements finds those sentences; "corpus
// part-hts-index" lists them for the whole corpus, so a part of a user's
// product can be matched to the parts CBP classified before. The same
// function reads the gold codes of the evaluation (D381) from the rulings
// being scored, which the suggester never sees for that ruling.

// Statement is one sentence of a ruling that classifies a named thing.
type Statement struct {
	// Part is the thing classified, as the sentence names it (lower case,
	// a few words: "aluminum pegs").
	Part string `json:"part"`
	// HTS is the code as stated, normalised to 4, 6 or 8 digits
	// ("8302", "8302.50", "8302.50.00"); chapters 98 and 99 are skipped.
	HTS string `json:"hts"`
	// Sentence is the sentence, whitespace collapsed, at most sentenceRunes.
	Sentence string `json:"sentence"`
}

const sentenceRunes = 320

// partClass matches "<determiner> <thing> [is|are|will be ...] [properly]
// classified|classifiable|provided for in|under [sub]heading <code>".
var partClass = regexp.MustCompile(`(?i)\b(?:the|its|a|an|each|these|those|such|all)\s+([a-z][a-z0-9 ,/'&()\-]{1,90}?)\s+(?:(?:is|are|was|were|would be|will be|would|will|remains?|should be|must be)\s+)?(?:properly\s+|correctly\s+|also\s+)?(?:classified|classifiable|provided for)\s+(?:in|under|within)\s+(?:sub)?heading\s+(\d{4}(?:\.\d{2}){0,2}(?:\.?\d{2,4})?)`)

// genericPart are names that say nothing about what the thing is.
var genericPart = map[string]bool{
	"merchandise": true, "goods": true, "good": true, "product": true, "products": true, "item": true, "items": true,
	"article": true, "articles": true, "subject merchandise": true, "subject product": true, "subject item": true,
	"subject goods": true, "subject article": true, "subject articles": true, "instant product": true, "finished product": true,
	"finished good": true, "finished article": true, "imported product": true, "imported merchandise": true,
}

// PartStatements returns the classifying sentences of a ruling text, in
// order, without exact duplicates. The thing classified is cut at a clause
// ("the varistor, which as noted above, is classified" → "varistor") and
// leading "subject"/"imported" words are dropped; generic names (merchandise,
// product, item) and names longer than eight words are skipped.
func PartStatements(text string) []Statement {
	text = strings.NewReplacer("\r", " ", "\n", " ", " ", " ").Replace(text)
	var out []Statement
	seen := map[string]bool{}
	for _, m := range partClass.FindAllStringSubmatchIndex(text, -1) {
		part := cleanPart(text[m[2]:m[3]])
		code := statedCode(text[m[4]:m[5]])
		if part == "" || code == "" {
			continue
		}
		sent := sentenceAround(text, m[0], m[1])
		key := part + "|" + code
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, Statement{Part: part, HTS: code, Sentence: sent})
	}
	return out
}

func cleanPart(p string) string {
	p = strings.ToLower(strings.Join(strings.Fields(p), " "))
	for _, cut := range []string{", which", " which", ", that", " that ", ", when", " when ", ", as ", ", once", " once "} {
		if i := strings.Index(p, cut); i >= 0 {
			p = p[:i]
		}
	}
	// A match can start at an earlier determiner ("the only foreign
	// component is the varistor"): keep what follows the last verb.
	for _, verb := range []string{" is ", " are ", " was ", " were "} {
		if i := strings.LastIndex(p, verb); i >= 0 {
			p = p[i+len(verb):]
		}
	}
	p = strings.Trim(p, " ,()-'")
	for _, lead := range []string{"the ", "a ", "an ", "its ", "subject ", "imported ", "individual ", "instant "} {
		p = strings.TrimPrefix(p, lead)
	}
	for _, w := range []string{"origin", "change to", "chapter", "note", "heading", "rule", "tariff", "ruling", "classif"} {
		if strings.Contains(p, w) {
			return "" // legal text ("the country of origin of a good classified in ..."), not a thing
		}
	}
	if p == "" || genericPart[p] || len(strings.Fields(p)) > 8 || !strings.ContainsAny(p, "abcdefghijklmnopqrstuvwxyz") {
		return ""
	}
	return p
}

// statedCode normalises a stated code to 4, 6 or 8 digits (a 10-digit
// statistical number is cut to 8). Chapters 98 and 99 are not
// classifications of a thing; "" for them and for malformed numbers.
func statedCode(s string) string {
	d := digitsOf(s)
	if strings.HasPrefix(d, "98") || strings.HasPrefix(d, "99") {
		return ""
	}
	return FormatCode(d)
}

// FormatCode formats the digits of an HTS code at its stated level: 4
// ("8501"), 6 ("8501.31") or 8 ("8501.31.40"); longer numbers are cut to 8.
// Odd lengths below 8 are cut to the level below; under 4 digits is "".
func FormatCode(digits string) string {
	d := digitsOf(digits)
	switch {
	case len(d) >= 8:
		return d[:4] + "." + d[4:6] + "." + d[6:8]
	case len(d) >= 6:
		return d[:4] + "." + d[4:6]
	case len(d) >= 4:
		return d[:4]
	}
	return ""
}

func digitsOf(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// sentenceAround returns the sentence holding text[i:j], whitespace
// collapsed and cut to sentenceRunes.
func sentenceAround(text string, i, j int) string {
	start := strings.LastIndex(text[:i], ". ")
	if start < 0 {
		start = 0
	} else {
		start += 2
	}
	end := strings.Index(text[j:], ". ")
	if end < 0 {
		end = len(text)
	} else {
		end += j + 1
	}
	s := strings.Join(strings.Fields(text[start:end]), " ")
	if r := []rune(s); len(r) > sentenceRunes {
		s = string(r[:sentenceRunes]) + "…"
	}
	return s
}

// PartEntry is one part classification in the index.
type PartEntry struct {
	Ruling string `json:"ruling"`
	Date   string `json:"date"` // YYYY-MM-DD
	Statement
}

// PartIndex is data/part_hts_index.json, written by "corpus part-hts-index".
type PartIndex struct {
	Note    string      `json:"note"`
	Entries []PartEntry `json:"entries"`
}

// LoadPartIndex reads a part index and checks its codes.
func LoadPartIndex(path string) (*PartIndex, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("hts: read %s: %w", path, err)
	}
	var x PartIndex
	if err := json.Unmarshal(b, &x); err != nil {
		return nil, fmt.Errorf("hts: decode %s: %w", path, err)
	}
	if len(x.Entries) == 0 {
		return nil, fmt.Errorf("hts: %s lists no part classifications", path)
	}
	for _, e := range x.Entries {
		if e.Ruling == "" || e.Part == "" || FormatCode(e.HTS) != e.HTS {
			return nil, fmt.Errorf("hts: %s: bad entry %+v", path, e)
		}
	}
	return &x, nil
}
