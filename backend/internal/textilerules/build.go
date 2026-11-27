package textilerules

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

var (
	reTag   = regexp.MustCompile(`<[^>]+>`)
	reSpace = regexp.MustCompile(`\s+`)
	reTR    = regexp.MustCompile(`(?s)<TR>(.*?)</TR>`)
	reTD    = regexp.MustCompile(`(?s)<TD[^>]*?(?:/>|>(.*?)</TD>)`)
	reFP    = regexp.MustCompile(`(?s)<FP-1>(.*?)</FP-1>`)
)

func clean(s string) string {
	s = reTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.NewReplacer(" ", " ", "“", `"`, "”", `"`, "’", "'").Replace(s)
	return strings.TrimSpace(reSpace.ReplaceAllString(s, " "))
}

// Build encodes 19 CFR 102.21 from the eCFR XML of the section and the
// Federal Register excerpt for the entry the eCFR lacks.
func Build(ecfrXML, frExcerpt []byte, src Source) (Table, error) {
	x := string(ecfrXML)
	t := Table{Source: src}
	// (b)(5) scope list.
	i := strings.Index(x, "following HTSUS headings or subheadings:")
	j := strings.Index(x[i:], "(6)")
	if i < 0 || j < 0 {
		return t, fmt.Errorf("textilerules: (b)(5) list not found")
	}
	for _, m := range reFP.FindAllStringSubmatch(x[i:i+j], -1) {
		if c := clean(m[1]); c != "" {
			t.Scope = append(t.Scope, c)
		}
	}
	// (c)(3)(ii) exclusions.
	k := strings.Index(x, "(ii) Except for fabrics of chapter 59")
	if k < 0 {
		return t, fmt.Errorf("textilerules: (c)(3)(ii) not found")
	}
	ex := clean(x[k : k+strings.Index(x[k:], "if the good was not knit")])
	t.Excl3ii = parseExclusions(ex)
	// Table 1 rows.
	a := strings.Index(x, "<TBODY>")
	b := strings.Index(x, "</TBODY>")
	if a < 0 || b < a {
		return t, fmt.Errorf("textilerules: table 1 not found")
	}
	var cur *Row
	var cp *cellParser
	for _, m := range reTR.FindAllStringSubmatch(x[a:b], -1) {
		tds := reTD.FindAllStringSubmatch(m[1], -1)
		if len(tds) != 2 {
			return t, fmt.Errorf("textilerules: row with %d cells: %.200q", len(tds), m[1])
		}
		code, text := clean(tds[0][1]), clean(tds[1][1])
		if code != "" {
			lo, hi, ok := rangeOf(code)
			if !ok {
				return t, fmt.Errorf("textilerules: unreadable code %q", code)
			}
			t.Rows = append(t.Rows, Row{Code: code, Lo: lo, Hi: hi, Origin: "eCFR"})
			cur = &t.Rows[len(t.Rows)-1]
			cp = &cellParser{}
		}
		if cur == nil {
			return t, fmt.Errorf("textilerules: text before the first code")
		}
		cur.Alts = append(cur.Alts, cp.parse(text)...)
	}
	// The entry the eCFR lacks (editorial note: 87 FR 68356, 6201-6208).
	fr, err := frRow(string(frExcerpt))
	if err != nil {
		return t, err
	}
	pos := len(t.Rows)
	for n, r := range t.Rows {
		if r.Lo > fr.Lo {
			pos = n
			break
		}
	}
	t.Rows = append(t.Rows[:pos], append([]Row{fr}, t.Rows[pos:]...)...)
	if t.E2, err = buildE2(x); err != nil {
		return t, err
	}
	return t, nil
}

var reLeader = regexp.MustCompile(`^(\d{4}-\d{4})\.+\s*`)

func frRow(ex string) (Row, error) {
	s := reSpace.ReplaceAllString(strings.TrimSpace(ex), " ")
	m := reLeader.FindStringSubmatch(s)
	if m == nil {
		return Row{}, fmt.Errorf("textilerules: Federal Register excerpt: no entry code")
	}
	code := m[1]
	lo, hi, _ := rangeOf(code)
	body := strings.TrimPrefix(s, m[0])
	row := Row{Code: code, Lo: lo, Hi: hi, Origin: "87 FR 68356"}
	// Split "(1) ... (2) ..." at the enumerators.
	idx := regexp.MustCompile(`\((\d)\) If `).FindAllStringIndex(body, -1)
	cp := &cellParser{}
	for n, p := range idx {
		end := len(body)
		if n+1 < len(idx) {
			end = idx[n+1][0]
		}
		row.Alts = append(row.Alts, cp.parse(strings.TrimSpace(body[p[0]:end]))...)
	}
	return row, nil
}

var (
	reCodeTok = regexp.MustCompile(`\d{4}(?:\.\d{2}(?:\.\d{2,4})?)?(?:-\d{2,4})?`)
)

func parseExclusions(s string) []string {
	var out []string
	if strings.Contains(strings.ToLower(s), "fabrics of chapter 59") {
		out = append(out, "5900-5999")
	}
	rest := s
	if i := strings.Index(rest, "goods of headings"); i >= 0 {
		rest = rest[i:]
	}
	rest = strings.NewReplacer(" through ", "-", " and ", " ").Replace(rest)
	out = append(out, reCodeTok.FindAllString(rest, -1)...)
	return out
}

// reRange reads one "5208 through 5212", "5007", "subheading 6307.90" or
// "Chapter 54" from an except list.
var reRange = regexp.MustCompile(`(?i)(?:chapter (\d+))|(\d{4}(?:\.\d{2}(?:\.\d{2,4})?)?)(?: through (\d{4}(?:\.\d{2}(?:\.\d{2,4})?)?))?`)

func exceptList(s string) []string {
	var out []string
	for _, m := range reRange.FindAllStringSubmatch(s, -1) {
		switch {
		case m[1] != "":
			out = append(out, "Chapter "+m[1])
		case m[3] != "":
			out = append(out, m[2]+"-"+m[3])
		default:
			out = append(out, m[2])
		}
	}
	return out
}
