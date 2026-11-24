package euorigin

import (
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/nporigin"
)

var (
	reTag     = regexp.MustCompile(`(?s)<[^>]+>`)
	reMarker  = regexp.MustCompile(`[►▼]\s*(?:M\d+|B|C\d+|A\d+)?|◄|—{3,}`)
	reSpace   = regexp.MustCompile(`[ \t\r\f\v\x{00a0}]+`)
	reRow     = regexp.MustCompile(`(?s)<tr[^>]*>(.*?)</tr>`)
	reCell    = regexp.MustCompile(`(?s)<td[^>]*>(.*?)</td>`)
	reBlock   = regexp.MustCompile(`(?s)<table[^>]*>.*?</table>|<p(?:\s[^>]*)?>.*?</p>`)
	reModref  = regexp.MustCompile(`^<p class="modref"`)
	reAnnexAt = regexp.MustCompile(`<p class="title-annex-1"[^>]*>\s*ANNEX 22-01\s*</p>`)
	reNextAt  = regexp.MustCompile(`<p class="title-annex-1"[^>]*>\s*ANNEX 22-02\s*</p>`)
	// A list item set as a grid: the label in one column, the paragraph in
	// the next. The label is moved into the paragraph.
	reGridItem = regexp.MustCompile(`(?s)<div class="list grid-list-column-1">\s*<span>([^<]*)</span>\s*</div>\s*<div class="grid-list-column-2">\s*<p([^>]*)>`)
)

// text is the plain text of an XHTML fragment: paragraphs on their own
// lines, tags and amendment markers dropped, entities read, spaces folded.
func text(x string) string {
	x = strings.ReplaceAll(x, "</p>", "\n")
	x = reTag.ReplaceAllString(x, " ")
	x = html.UnescapeString(x)
	x = reMarker.ReplaceAllString(x, " ")
	var lines []string
	for _, l := range strings.Split(x, "\n") {
		if l = strings.TrimSpace(reSpace.ReplaceAllString(l, " ")); l != "" {
			lines = append(lines, l)
		}
	}
	return strings.Join(lines, "\n")
}

// Blocks reads Annex 22-01 out of the consolidated XHTML of Delegated
// Regulation (EU) 2015/2446 (EUR-Lex / Cellar): its paragraphs and tables in
// document order, amendment markers dropped.
func Blocks(xhtml []byte) ([]nporigin.Block, error) {
	s := string(xhtml)
	at := reAnnexAt.FindStringIndex(s)
	if at == nil {
		return nil, fmt.Errorf("euorigin: no ANNEX 22-01 heading in the XHTML")
	}
	end := reNextAt.FindStringIndex(s[at[1]:])
	if end == nil {
		return nil, fmt.Errorf("euorigin: no ANNEX 22-02 heading after Annex 22-01")
	}
	s = s[at[1] : at[1]+end[0]]
	s = reGridItem.ReplaceAllString(s, "<p$2>$1 ")
	var out []nporigin.Block
	for _, m := range reBlock.FindAllString(s, -1) {
		if strings.HasPrefix(m, "<table") {
			var rows [][]string
			for _, r := range reRow.FindAllStringSubmatch(m, -1) {
				var cells []string
				for _, c := range reCell.FindAllStringSubmatch(r[1], -1) {
					cells = append(cells, text(c[1]))
				}
				rows = append(rows, cells)
			}
			out = append(out, nporigin.Block{Rows: rows})
			continue
		}
		if reModref.MatchString(m) {
			continue
		}
		if t := text(m); t != "" {
			out = append(out, nporigin.Block{Text: strings.ReplaceAll(t, "\n", " ")})
		}
	}
	return out, nil
}
