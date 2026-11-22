package tariffshift

import (
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Canada's marking rules (SOR/94-23, Schedule III) from the Justice Laws
// XML (D720). The rows are written like 19 CFR 102.20, with two changes of
// form that BuildCanadaRules undoes before ParseRule reads them: headings
// are written "84.73" (not "8473"), and ranges in the code column use an en
// dash ("01.01 – 01.06").

var (
	reCASchedule = regexp.MustCompile(`(?s)<Label>SCHEDULE III</Label>(.*?)</Schedule>`)
	reCABlock    = regexp.MustCompile(`(?s)<Heading[^>]*>(.*?)</Heading>|<TableGroup[^>]*>(.*?)</TableGroup>|<Provision[^>]*>(.*?)</Provision>`)
	reCALabel    = regexp.MustCompile(`(?s)<Label>(.*?)</Label>`)
	reCATitle    = regexp.MustCompile(`(?s)<TitleText>(.*?)</TitleText>`)
	reCAHead     = regexp.MustCompile(`(?s)<thead[^>]*>(.*?)</thead>`)
	reCABody     = regexp.MustCompile(`(?s)<tbody[^>]*>(.*?)</tbody>`)
	reCARow      = regexp.MustCompile(`(?s)<row>(.*?)</row>`)
	reCAEntry    = regexp.MustCompile(`(?s)<entry[^>]*?(?:/>|>(.*?)</entry>)`)
	reCAChapter  = regexp.MustCompile(`^Chapter (\d+)$`)
	reCASpan     = regexp.MustCompile(`\(Chapters? (\d+)(?:-(\d+))?\)`)
	reCANumber   = regexp.MustCompile(`\d+(?:\.\d+)*`)
	reCASpaces   = regexp.MustCompile(`[ \t]+`)
)

// caText is the plain text of an XML fragment: paragraphs (Provision) on
// their own lines, tags dropped, entities read, spaces folded.
func caText(x string) string {
	x = strings.ReplaceAll(x, "</Provision>", "\n")
	x = strings.ReplaceAll(x, "</Label>", " ")
	x = reTag.ReplaceAllString(x, "")
	x = html.UnescapeString(x)
	var lines []string
	for _, l := range strings.Split(x, "\n") {
		if l = strings.TrimSpace(reCASpaces.ReplaceAllString(l, " ")); l != "" {
			lines = append(lines, l)
		}
	}
	return strings.Join(lines, "\n")
}

// CAHeadings rewrites Canada's heading numbers in the 19 CFR form: "84.73"
// becomes "8473". Subheadings ("8473.30") and tariff items ("9808.00.00")
// are left as they are.
func CAHeadings(s string) string {
	return reCANumber.ReplaceAllStringFunc(s, func(t string) string {
		if len(t) == 5 && t[2] == '.' {
			return t[:2] + t[3:]
		}
		return t
	})
}

// caColumn reads the code column: "01.01 – 01.06" → "0101-0106".
func caColumn(s string) string {
	s = strings.NewReplacer("–", "-", "—", "-", " ", "").Replace(s)
	return CAHeadings(s)
}

// caErrata are slips in the published row text that the row's own
// wording settles; the table keeps the corrected text (D720).
var caErrata = strings.NewReplacer(
	// 59.01-59.11: "with" for "within".
	"including another heading with that group", "including another heading within that group",
	// 6815.10-6815.99: "a subheading" for "another subheading".
	"including a subheading within that group", "including another subheading within that group",
)

// caNote is a note of Schedule III and the rules it governs.
type caNote struct {
	text     string
	chapters []string // chapters it covers
	heading  string   // a single heading it covers ("" for the chapters)
	override bool     // it can set the origin by process (s. 4(1)(d))
}

// caNoteKinds are the notes of Schedule III, by their opening words. A
// note not listed is an error, so a new note is read by a person first
// (as applyNotes does for 102.20). override marks a note that sets the
// origin by a process the codes cannot show.
var caNoteKinds = []struct {
	prefix   string
	override bool
	chapters []string // when set, replaces the section's or chapter's span
	heading  string
}{
	{prefix: "Notwithstanding the tariff shift rules of this section, an agricultural or horticultural good grown", override: true},
	{prefix: "Notwithstanding the tariff shift rules of Chapter 28, 29, 31, 32 or 38", override: true, chapters: []string{"28", "29", "31", "32", "38"}},
	{prefix: "Where a good of Chapter 28 or 29 is subject to purification", override: true, chapters: []string{"28", "29"}},
	{prefix: "Where a good of Chapter 29 is subject to isomer separation", override: true, chapters: []string{"29"}},
	{prefix: "A foreign material or component will not be deemed to have satisfied all applicable requirements"},
	{prefix: "For the purposes of headings 0302 through 0303, fry of heading 0301"},
	{prefix: "Notwithstanding the tariff shift rules of this Chapter, where a good of Chapter 27 is the product of a chemical reaction", override: true},
	{prefix: "For the purposes of heading 2710, the country of origin of a good shall be the country in which the good underwent", override: true, heading: "2710"},
	{prefix: "For the purposes of this Chapter, substantial assembly means"},
	{prefix: "For the purposes of this Chapter, major garment parts means"},
	{prefix: "The country of origin of a good of heading 6309 shall be", override: true, heading: "6309"},
	{prefix: "The country of origin of a worn-out article of heading 6310 shall be", override: true, heading: "6310"},
	{prefix: "For purposes of this Chapter, the term formed uppers"},
	{prefix: "In accordance with Schedule I to these Regulations, only some goods are required to be marked"},
}

var reCANoteSplit = regexp.MustCompile(`(?m)^Note(?: \d+)?:\s*`)

// BuildCanadaRules reads the Justice Laws XML of SOR/94-23 and returns the
// Schedule III rows, each split into alternatives, with the notes attached.
func BuildCanadaRules(xml []byte) ([]Rule, error) {
	m := reCASchedule.FindSubmatch(xml)
	if m == nil {
		return nil, fmt.Errorf("tariffshift: no Schedule III in the XML")
	}
	var (
		rules   []Rule
		notes   []caNote
		section string
		secSpan []string
	)
	addNotes := func(text string, span []string) error {
		for _, n := range reCANoteSplit.Split(CAHeadings(text), -1) {
			n = strings.TrimSpace(strings.ReplaceAll(n, "\n", " "))
			if n == "" {
				continue
			}
			found := false
			for _, k := range caNoteKinds {
				if !strings.HasPrefix(n, k.prefix) {
					continue
				}
				found = true
				cn := caNote{text: n, chapters: span, heading: k.heading, override: k.override}
				if k.chapters != nil {
					cn.chapters = k.chapters
				}
				notes = append(notes, cn)
			}
			// A paragraph that continues the note before it ("For the
			// purposes of this Chapter, a chemical reaction is ...").
			if !found && len(notes) > 0 && strings.HasPrefix(n, "For the purposes of this Chapter, a chemical reaction") {
				notes[len(notes)-1].text += " " + n
				found = true
			}
			if !found {
				return fmt.Errorf("tariffshift: Schedule III note not understood: %q", n)
			}
		}
		return nil
	}
	for _, b := range reCABlock.FindAllStringSubmatch(string(m[1]), -1) {
		switch {
		case strings.HasPrefix(b[0], "<Heading"):
			label := ""
			if l := reCALabel.FindStringSubmatch(b[1]); l != nil {
				label = caText(l[1])
			}
			var titles []string
			for _, t := range reCATitle.FindAllStringSubmatch(b[1], -1) {
				titles = append(titles, caText(t[1]))
			}
			section = strings.TrimSpace(label + ": " + strings.Join(titles, " "))
			secSpan = nil
			if s := reCASpan.FindStringSubmatch(section); s != nil {
				lo, _ := strconv.Atoi(s[1])
				hi := lo
				if s[2] != "" {
					hi, _ = strconv.Atoi(s[2])
				}
				secSpan = chapterSpan(lo, hi)
			}
		case strings.HasPrefix(b[0], "<Provision"):
			if err := addNotes(caText(b[0]), secSpan); err != nil {
				return nil, err
			}
		default:
			head := reCAHead.FindStringSubmatch(b[2])
			body := reCABody.FindStringSubmatch(b[2])
			if head == nil || body == nil {
				return nil, fmt.Errorf("tariffshift: Schedule III table without head or body in %s", section)
			}
			hc := cells(head[1])
			if len(hc) != 2 {
				return nil, fmt.Errorf("tariffshift: Schedule III table head %q", hc)
			}
			cm := reCAChapter.FindStringSubmatch(hc[0])
			if cm == nil {
				return nil, fmt.Errorf("tariffshift: Schedule III table head %q", hc[0])
			}
			chapter := fmt.Sprintf("%02s", cm[1])
			for _, row := range reCARow.FindAllStringSubmatch(body[1], -1) {
				c := cells(row[1])
				if len(c) != 2 {
					return nil, fmt.Errorf("tariffshift: Schedule III row with %d cells in chapter %s", len(c), chapter)
				}
				hts := caColumn(c[0])
				if c[0] == "" {
					// A row with no code holds the chapter's notes and, in
					// chapters 61 and 62, a rule for every good of the
					// chapter ("A change to any good of this chapter ...").
					var noteLines, ruleLines []string
					for _, l := range strings.Split(c[1], "\n") {
						if strings.HasPrefix(l, "A change") {
							ruleLines = append(ruleLines, l)
						} else {
							noteLines = append(noteLines, l)
						}
					}
					if err := addNotes(strings.Join(noteLines, "\n"), []string{chapter}); err != nil {
						return nil, err
					}
					if len(ruleLines) == 0 {
						continue
					}
					hts, c[1] = chapter, strings.Join(ruleLines, "\n")
				}
				r, err := ParseRange(hts)
				if err != nil {
					return nil, err
				}
				if r.Lo[:2] != chapter || r.Hi[:2] != chapter {
					return nil, fmt.Errorf("tariffshift: row %s is not in chapter %s", hts, chapter)
				}
				rules = append(rules, Rule{
					ID:        "SOR/94-23 Sch. III, " + hts,
					Paragraph: chapter,
					Section:   section + "; Chapter " + strings.TrimLeft(chapter, "0") + ": " + hc[1],
					HTS:       hts,
					Applies:   r,
					Text:      caErrata.Replace(CAHeadings(c[1])),
				})
			}
		}
	}
	if len(rules) == 0 {
		return nil, fmt.Errorf("tariffshift: no Schedule III rows")
	}
	for i := range rules {
		rules[i].Alternatives = ParseRule(rules[i].Text)
	}
	for _, n := range notes {
		if n.chapters == nil {
			continue // the schedule's opening note on Schedule I
		}
		hit := false
		for i := range rules {
			r := &rules[i]
			in := false
			for _, ch := range n.chapters {
				if r.Paragraph == ch {
					in = true
				}
			}
			if !in || (n.heading != "" && r.Applies.truncate(4).contains(n.heading) != yes) {
				continue
			}
			hit = true
			r.Notes = append(r.Notes, n.text)
			if n.override {
				// s. 4(1)(d): the note is a route to origin of its own. A
				// good that fails the shift is not sent on to s. 5 by
				// code, and a decided answer assumes the note does not
				// apply (D723).
				r.NoteOverrides, r.FallbackNote = true, true
			}
		}
		if !hit && n.heading != "" && n.override {
			// The note is the only rule for its heading (63.09 has no row):
			// a row of its own, with no tariff-shift alternative.
			r, _ := ParseRange(n.heading)
			rules = append(rules, Rule{
				ID: "SOR/94-23 Sch. III, " + n.heading + " (note)", Paragraph: n.heading[:2],
				Section: "Chapter note", HTS: n.heading, Applies: r, Text: n.text,
				Notes: []string{n.text}, NoteOverrides: true, FallbackNote: true,
			})
			hit = true
		}
		if !hit {
			return nil, fmt.Errorf("tariffshift: Schedule III note matches no row: %q", n.text)
		}
	}
	sort.SliceStable(rules, func(i, j int) bool { return padCode(rules[i].Applies.Lo) < padCode(rules[j].Applies.Lo) })
	return rules, nil
}

func padCode(c string) string { return c + strings.Repeat("0", max(0, 8-len(c))) }

// cells returns the text of each entry of a table row.
func cells(row string) []string {
	var out []string
	for _, e := range reCAEntry.FindAllStringSubmatch(row, -1) {
		out = append(out, caText(e[1]))
	}
	return out
}
