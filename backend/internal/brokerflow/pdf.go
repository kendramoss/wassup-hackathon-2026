package brokerflow

import (
	"bytes"
	"fmt"
	"strings"
)

// TextPDF renders Markdown-ish text as a plain, paged A4-width Letter PDF in
// Helvetica: headings in bold, Markdown emphasis and list markers kept as
// plain text. It needs no font files (the base-14 fonts) and no library. It
// is for printing and attaching, not layout fidelity.
func TextPDF(title, text string) []byte {
	const (
		pageW, pageH = 612.0, 792.0 // US Letter
		margin       = 54.0
		size, lead   = 10.0, 13.0
		maxChars     = 95
	)
	type line struct {
		s    string
		bold bool
	}
	var lines []line
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		bold := false
		s := strings.TrimRight(raw, " ")
		if strings.HasPrefix(s, "#") {
			s, bold = strings.TrimSpace(strings.TrimLeft(s, "#")), true
		}
		s = strings.ReplaceAll(s, "**", "")
		if s == "" {
			lines = append(lines, line{})
			continue
		}
		for _, w := range wrap(s, maxChars) {
			lines = append(lines, line{w, bold})
		}
	}
	perPage := 52 // (pageH - 2*margin) / lead, rounded down
	var pages [][]line
	for len(lines) > 0 {
		n := min(perPage, len(lines))
		pages = append(pages, lines[:n])
		lines = lines[n:]
	}
	if len(pages) == 0 {
		pages = [][]line{{}}
	}

	var objs []string // object bodies, numbered from 1
	add := func(s string) int { objs = append(objs, s); return len(objs) }
	catalog := add("") // filled below
	pagesObj := add("")
	font := add("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	bold := add("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")
	var kids []string
	for _, pg := range pages {
		var c bytes.Buffer
		fmt.Fprintf(&c, "BT\n%.0f TL\n%.0f %.0f Td\n", lead, margin, pageH-margin)
		cur := ""
		for _, l := range pg {
			f := "/F1"
			if l.bold {
				f = "/F2"
			}
			if f != cur {
				fmt.Fprintf(&c, "%s %.0f Tf\n", f, size)
				cur = f
			}
			fmt.Fprintf(&c, "(%s) Tj T*\n", pdfString(l.s))
		}
		c.WriteString("ET\n")
		content := add(fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", c.Len(), c.String()))
		page := add(fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 %.0f %.0f] /Resources << /Font << /F1 %d 0 R /F2 %d 0 R >> >> /Contents %d 0 R >>", pagesObj, pageW, pageH, font, bold, content))
		kids = append(kids, fmt.Sprintf("%d 0 R", page))
	}
	info := add(fmt.Sprintf("<< /Title (%s) /Producer (Unde Labs) >>", pdfString(title)))
	objs[catalog-1] = fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pagesObj)
	objs[pagesObj-1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(kids))

	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root %d 0 R /Info %d 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, catalog, info, xref)
	return out.Bytes()
}

// wrap breaks s into lines of at most n characters at spaces, keeping a
// list item's indent on its continuation lines.
func wrap(s string, n int) []string {
	indent := ""
	if strings.HasPrefix(s, "- ") {
		indent = "  "
	}
	var out []string
	words := strings.Fields(s)
	cur := ""
	for _, w := range words {
		for len([]rune(w)) > n { // a word longer than a line: hard break
			if cur != "" {
				out = append(out, cur)
				cur = indent
			}
			r := []rune(w)
			out = append(out, string(r[:n]))
			w = string(r[n:])
		}
		switch {
		case cur == "" || cur == indent:
			cur += w
		case len([]rune(cur))+1+len([]rune(w)) > n:
			out = append(out, cur)
			cur = indent + w
		default:
			cur += " " + w
		}
	}
	if strings.TrimSpace(cur) != "" {
		out = append(out, cur)
	}
	return out
}

// winAnsi maps the punctuation outside Latin-1 that the letters use.
var winAnsi = map[rune]byte{'—': 0x97, '–': 0x96, '‘': 0x91, '’': 0x92, '“': 0x93, '”': 0x94, '•': 0x95, '…': 0x85, '€': 0x80}

// pdfString escapes s for a PDF literal string in WinAnsiEncoding; a
// character it cannot encode becomes "?".
func pdfString(s string) string {
	var b strings.Builder
	for _, r := range s {
		var c byte
		switch {
		case r == '(' || r == ')' || r == '\\':
			b.WriteByte('\\')
			c = byte(r)
		case r >= 0x20 && r < 0x7f:
			c = byte(r)
		case r >= 0xa0 && r <= 0xff:
			c = byte(r)
		default:
			if v, ok := winAnsi[r]; ok {
				c = v
			} else if r == '\t' {
				c = ' '
			} else {
				c = '?'
			}
		}
		if c >= 0x80 {
			fmt.Fprintf(&b, "\\%03o", c)
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}
