package tariffshift

import (
	"fmt"
	"html"
	"regexp"
	"slices"
	"strings"
)

var (
	reSection  = regexp.MustCompile(`(?s)<DIV8 N="102\.20"[^>]*>(.*?)</DIV8>`)
	reRow      = regexp.MustCompile(`(?s)<TR>(.*?)</TR>`)
	reCell     = regexp.MustCompile(`(?s)<T[DH][^>]*>(.*?)</T[DH]>`)
	reTag      = regexp.MustCompile(`<[^>]+>`)
	reParaCell = regexp.MustCompile(`^\(([a-z])\)$`)
)

// BuildRules reads the eCFR XML of 19 CFR Part 102 and returns the 102.20
// rows whose HTS column starts with one of the chapters, each split into
// alternatives. Rows with an empty HTS column continue the row above.
func BuildRules(xml []byte, chapters []string) ([]Rule, error) {
	sec := reSection.FindSubmatch(xml)
	if sec == nil {
		return nil, fmt.Errorf("tariffshift: no section 102.20 in the XML")
	}
	var (
		rules             []Rule
		para, section     string
		cur               *Rule
		inScope, anyTable bool
		notes             []note
	)
	flush := func() {
		if cur != nil && inScope {
			rules = append(rules, *cur)
		}
		cur = nil
	}
	for _, row := range reRow.FindAllSubmatch(sec[1], -1) {
		var cells []string
		for _, c := range reCell.FindAllSubmatch(row[1], -1) {
			t := reTag.ReplaceAllString(string(c[1]), " ")
			cells = append(cells, clean(html.UnescapeString(t)))
		}
		switch len(cells) {
		case 0:
			continue
		case 1:
			// A note row spanning both columns. Notes in scope are kept and,
			// where one bars a change to a heading, applied by applyNotes.
			flush()
			if para != "" && inSection(section, chapters) && noteInScope(cells[0], chapters) {
				notes = append(notes, note{para: para, text: cells[0]})
			}
			continue
		case 2:
		default:
			return nil, fmt.Errorf("tariffshift: row with %d cells", len(cells))
		}
		anyTable = true
		hts, text := cells[0], cells[1]
		switch {
		case hts == "HTSUS":
			continue
		case reParaCell.MatchString(hts):
			flush()
			para, section = reParaCell.FindStringSubmatch(hts)[1], text
			continue
		case hts == "":
			if cur != nil {
				cur.Text += "\n" + text
			}
			continue
		}
		flush()
		inScope = len(hts) >= 2 && slices.Contains(chapters, hts[:2])
		if !inScope {
			cur = &Rule{}
			continue
		}
		r, err := ParseRange(columnRange(hts))
		if err != nil {
			return nil, err
		}
		cur = &Rule{
			ID:        fmt.Sprintf("19 CFR 102.20(%s), %s", para, hts),
			Paragraph: para,
			Section:   section,
			HTS:       hts,
			Applies:   r,
			Text:      text,
		}
	}
	flush()
	if !anyTable {
		return nil, fmt.Errorf("tariffshift: no table rows")
	}
	for i := range rules {
		rules[i].Alternatives = ParseRule(rules[i].Text)
	}
	if err := applyNotes(rules, notes); err != nil {
		return nil, err
	}
	return rules, nil
}

type note struct{ para, text string }

// columnErrata are HTS columns the eCFR prints with an error that the row's
// own text settles. Rule.HTS and the ID keep the column as published; only
// the range the row applies to is read from here.
//
// "4441-4421" (lo above hi) follows the row "4413-4421" and repeats its
// text with subheading 4418.73 for 4418.71; each of its sentences changes
// to heading 4413 through 4421 (or a subheading of it). It is read as
// 4413-4421, so two rows cover those headings and Lookup leaves a good
// there undetermined ("more than one rule covers") rather than pick one.
var columnErrata = map[string]string{"4441-4421": "4413-4421"}

func columnRange(hts string) string {
	if r, ok := columnErrata[hts]; ok {
		return r
	}
	return hts
}

var reChapterNote = regexp.MustCompile(`^Chapter (\d+) Note:`)

// noteInScope drops a "Chapter NN Note:" for a chapter out of scope.
func noteInScope(text string, chapters []string) bool {
	m := reChapterNote.FindStringSubmatch(text)
	return m == nil || slices.Contains(chapters, fmt.Sprintf("%02s", m[1]))
}

var reSectionChapters = regexp.MustCompile(`Chapters? (\d+)(?: through (\d+))?`)

// inSection says whether a section heading ("Section XVII: Chapters 86
// through 89") covers any chapter in scope.
func inSection(section string, chapters []string) bool {
	m := reSectionChapters.FindStringSubmatch(section)
	if m == nil {
		return false
	}
	lo, hi := fmt.Sprintf("%02s", m[1]), fmt.Sprintf("%02s", m[2])
	if m[2] == "" {
		hi = lo
	}
	for _, c := range chapters {
		if c >= lo && c <= hi {
			return true
		}
	}
	return false
}

// reFallbackNote is a chapter note that replaces 102.11(b) to (d) for goods
// that fail their tariff shift (the chapter 94 bedding note).
var reFallbackNote = regexp.MustCompile(`^Chapter \d+ Note: For a good classifiable in subheadings (\d{4}\.\d{2}) through (\d{4}\.\d{2}) which does not meet the appropriate tariff shift rule`)

// reTextileNote sends goods of listed subheadings to 102.21 (the textile
// rules), which this package does not encode ("Chapter 39 Note: The country
// of origin of goods classified in subheadings 3921.12.15, 3921.13.15, and
// 3921.90.2550 shall be determined under the provisions of § 102.21.").
var reTextileNote = regexp.MustCompile(`^Chapter \d+ Note: The country of origin of goods classified in subheadings? (.+?),? (?:shall|will) be determined under the provisions of § 102\.21\.$`)

var reBarNote = regexp.MustCompile(`^Note: Any change to heading (\d{4}) from (.+?) shall not be considered to satisfy a required change in tariff classification\.?$`)

// applyNotes attaches every in-scope note to the rules of its paragraph. A
// note that bars changes to a heading from listed codes becomes an
// unconditional exception on every alternative of the rules for that
// heading. Any other note in scope is an error, so a new note is read by a
// person before it can be ignored.
func applyNotes(rules []Rule, notes []note) error {
	for _, n := range notes {
		if f := reFallbackNote.FindStringSubmatch(n.text); f != nil {
			span, err := ParseRange(f[1] + "-" + f[2])
			if err != nil {
				return err
			}
			hit := false
			for i := range rules {
				r := &rules[i]
				if r.Paragraph == n.para && span.contains(r.Applies.Lo) == yes && span.contains(r.Applies.Hi) == yes {
					r.Notes = append(r.Notes, n.text)
					r.FallbackNote = true
					hit = true
				}
			}
			if !hit {
				return fmt.Errorf("tariffshift: note matches no rule: %q", n.text)
			}
			continue
		}
		if t := reTextileNote.FindStringSubmatch(n.text); t != nil {
			codes, described, err := parseList("subheading " + t[1])
			if err != nil || described || len(codes) == 0 {
				return fmt.Errorf("tariffshift: note list not read: %q", t[1])
			}
			hit := false
			for i := range rules {
				r := &rules[i]
				if r.Paragraph != n.para || !overlaps(r.Applies, codes) {
					continue
				}
				hit = true
				r.Notes = append(r.Notes, n.text)
				r.Textile = append(r.Textile, codes...)
			}
			if !hit {
				return fmt.Errorf("tariffshift: note matches no rule: %q", n.text)
			}
			continue
		}
		m := reBarNote.FindStringSubmatch(n.text)
		if m == nil {
			return fmt.Errorf("tariffshift: note in scope not understood: %q", n.text)
		}
		from, described, err := parseList(m[2])
		if err != nil || described || len(from) == 0 {
			return fmt.Errorf("tariffshift: note list not read: %q", m[2])
		}
		head := Range{Lo: m[1], Hi: m[1]}
		hit := false
		for i := range rules {
			r := &rules[i]
			if r.Paragraph != n.para || head.contains(r.Applies.Lo) != yes {
				continue
			}
			hit = true
			r.Notes = append(r.Notes, n.text)
			for j := range r.Alternatives {
				a := &r.Alternatives[j]
				if a.Encoded {
					a.Except = append(a.Except, Exception{From: from, Text: n.text})
				}
			}
		}
		if !hit {
			return fmt.Errorf("tariffshift: note matches no rule: %q", n.text)
		}
	}
	return nil
}

// overlaps says whether any code in cs falls in r.
func overlaps(r Range, cs []Range) bool {
	for _, c := range cs {
		if r.contains(c.Lo) != no || r.contains(c.Hi) != no {
			return true
		}
	}
	return false
}

// ParseRule splits a rule's verbatim text and parses each alternative.
func ParseRule(text string) []Alternative {
	var out []Alternative
	text = strings.ReplaceAll(text, "\nA change", "; A change")
	for _, s := range SplitAlternatives(strings.ReplaceAll(text, "\n", " ")) {
		out = append(out, ParseAlternative(s))
	}
	return out
}

// Reparse parses a table's committed verbatim rule text again with the
// current parser and re-applies its notes. It is how rules.json is
// regenerated after a parser fix without fetching the eCFR again (#550,
// #551); TestRulesJSONMatchesParser checks the committed file equals it.
func Reparse(tab Table) (Table, error) {
	var notes []note
	seen := map[string]bool{}
	for _, r := range tab.Rules {
		for _, n := range r.Notes {
			if k := r.Paragraph + n; !seen[k] {
				seen[k] = true
				notes = append(notes, note{para: r.Paragraph, text: n})
			}
		}
	}
	out := tab
	out.Rules = make([]Rule, len(tab.Rules))
	for i, r := range tab.Rules {
		out.Rules[i] = r
		out.Rules[i].Notes, out.Rules[i].FallbackNote, out.Rules[i].Textile = nil, false, nil
		out.Rules[i].Alternatives = ParseRule(r.Text)
	}
	if err := applyNotes(out.Rules, notes); err != nil {
		return Table{}, err
	}
	return out, nil
}
