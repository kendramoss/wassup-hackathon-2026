package nporigin

import (
	"fmt"
	"regexp"
	"strings"
)

// Block is one piece of the published list in document order: a paragraph
// (Text) or a table (Rows, each a list of cell texts whose paragraphs are
// joined by "\n").
type Block struct {
	Text string
	Rows [][]string
}

var (
	reChapterHead = regexp.MustCompile(`^CHAPTER (\d{1,2})$`)
	reSectionHead = regexp.MustCompile(`^SECTION [IVXL]+$`)
	reResidual    = regexp.MustCompile(`(?i)^Chapter residual rule:?$`)
	reMixture     = regexp.MustCompile(`(?i)^Chapter residual rule applicable to mixtures:?$`)
	reNoteHead    = regexp.MustCompile(`(?i)^(?:Chapter notes?|Note \d+:.*|Definitions?)\s*:?$`)
	rePrimaryHead = regexp.MustCompile(`(?i)^Primary Rule:`)
	reDefHead     = regexp.MustCompile(`(?i)^Definition of `)
	reFallback    = regexp.MustCompile(`(?i)^Where the primary rule for headings .* is not met`)
	reHeaderCode  = regexp.MustCompile(`(?i)code$`)
	reDeleted     = regexp.MustCompile(`^\[.*\]$`)
	reContainer   = regexp.MustCompile(`(?i)^As specified for (?:split )?(?:sub)?headings$`)
)

// BuildChapters reads the list's blocks, from the first "CHAPTER" heading
// on, into chapters: the paragraphs before each chapter's table are its
// preamble; each table row with a code, a description and a rule is a row.
// Group labels (no code), container rows ("As specified for split
// headings") and deleted rows ("[5304]") are skipped. An unreadable code is
// an error, so a new row shape is read by a person first.
func BuildChapters(blocks []Block) ([]Chapter, error) {
	var (
		out []Chapter
		cur *Chapter
		// titled: the line after "CHAPTER n" is the chapter's title.
		titled bool
		inSect bool
		seen   = map[string]bool{}
	)
	var flat []Block
	for _, b := range blocks {
		// A layout table (numbered paragraphs set as a table of one or two
		// columns) is read as paragraphs.
		if b.Rows != nil && maxCells(b.Rows) < 3 {
			for _, r := range b.Rows {
				flat = append(flat, Block{Text: oneLine(strings.Join(r, " "))})
			}
			continue
		}
		flat = append(flat, b)
	}
	for _, b := range flat {
		if b.Rows == nil {
			t := strings.TrimSpace(b.Text)
			if t == "" {
				continue
			}
			if m := reChapterHead.FindStringSubmatch(t); m != nil {
				out = append(out, Chapter{Chapter: fmt.Sprintf("%02s", m[1])})
				cur, titled, inSect = &out[len(out)-1], false, false
				continue
			}
			if cur == nil {
				continue
			}
			if reSectionHead.MatchString(t) {
				// A section heading and its title end the chapter's text.
				inSect = true
				continue
			}
			if inSect {
				continue
			}
			if !titled {
				cur.Title, titled = t, true
				continue
			}
			if len(cur.Rows) == 0 {
				cur.Preamble = append(cur.Preamble, t)
			}
			continue
		}
		if cur == nil {
			continue
		}
		for _, cells := range b.Rows {
			if len(cells) < 3 {
				continue
			}
			code, desc, rule := strings.TrimSpace(cells[0]), strings.TrimSpace(cells[1]), strings.TrimSpace(cells[2])
			switch {
			case code == "" || reHeaderCode.MatchString(oneLine(code)) || strings.Contains(code, "◄"):
				continue
			case reDeleted.MatchString(oneLine(code)) && rule == "":
				continue
			case reContainer.MatchString(oneLine(rule)):
				continue
			case rule == "":
				return nil, fmt.Errorf("nporigin: chapter %s: row %q has no rule", cur.Chapter, code)
			}
			applies, ex, split, key, err := ParseCode(code)
			if err != nil {
				return nil, fmt.Errorf("nporigin: chapter %s: %w", cur.Chapter, err)
			}
			for _, r := range applies {
				if r.Lo[:2] != cur.Chapter || r.Hi[:2] != cur.Chapter {
					return nil, fmt.Errorf("nporigin: chapter %s: row %q is outside the chapter", cur.Chapter, code)
				}
			}
			if seen[key] {
				return nil, fmt.Errorf("nporigin: chapter %s: row key %q twice", cur.Chapter, key)
			}
			seen[key] = true
			cur.Rows = append(cur.Rows, Row{
				Key: key, Code: oneLine(code), Applies: applies, Ex: ex, Split: split,
				Description: strings.TrimSpace(desc), Text: rule, Alts: ParseRule(key, rule),
			})
		}
	}
	for i := range out {
		readPreamble(&out[i])
		if len(out[i].Rows) == 0 {
			return nil, fmt.Errorf("nporigin: chapter %s has no rows", out[i].Chapter)
		}
	}
	return out, nil
}

// readPreamble sorts a chapter's preamble paragraphs into the residual
// rule, the mixture rule, chapter primary rules, a fallback note and other
// notes. Each paragraph stays verbatim in Preamble.
func readPreamble(c *Chapter) {
	const (
		other = iota
		residual
		mixture
		primary
	)
	part := other
	var mix []string
	for _, p := range c.Preamble {
		switch {
		case reMixture.MatchString(p):
			part = mixture
			continue
		case reResidual.MatchString(p):
			part = residual
			continue
		case rePrimaryHead.MatchString(p):
			part = primary
			c.ChapterRules = append(c.ChapterRules, p)
			continue
		case reNoteHead.MatchString(p), reDefHead.MatchString(p):
			part = other
			c.Notes = append(c.Notes, p)
			continue
		}
		switch part {
		case residual:
			c.ResidualText = strings.TrimSpace(c.ResidualText + " " + p)
		case mixture:
			mix = append(mix, p)
		case primary:
			c.ChapterRules[len(c.ChapterRules)-1] += "\n" + p
		default:
			if reFallback.MatchString(p) {
				c.Fallback = p
			} else {
				c.Notes = append(c.Notes, p)
			}
		}
	}
	// A note heading with no note after it (the note became the fallback)
	// is dropped.
	for len(c.Notes) > 0 && reNoteHead.MatchString(c.Notes[len(c.Notes)-1]) {
		c.Notes = c.Notes[:len(c.Notes)-1]
	}
	c.Mixture = strings.Join(mix, "\n")
	switch {
	case strings.Contains(c.ResidualText, "on the basis of the value"):
		c.Residual = "value"
	case strings.Contains(c.ResidualText, "on the basis of the weight"):
		c.Residual = "weight"
	}
}

func maxCells(rows [][]string) int {
	n := 0
	for _, r := range rows {
		n = max(n, len(r))
	}
	return n
}
