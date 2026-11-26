package tariffshift

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Mexico's Apéndice de Reglas Específicas (D914) is read from a plain-text
// export of the SNICE consolidation: one row per line, "code<TAB>rule",
// with continuation lines after it. Each row's Spanish text is rendered in
// the English phrasing ParseRule reads by a fixed list of replacements; a
// row with any Spanish left after them keeps its Spanish text, so it is
// never encoded and never decided by code.

var (
	reMXChapter = regexp.MustCompile(`^Cap[íi]tulo (\d{1,2})(?: \(sic\))?\s*\t(.*)$`)
	reMXSection = regexp.MustCompile(`^Secci[óo]n [IVXL]+\b`)
	// Amendment marks the consolidation adds under a row ("Regla adicionada
	// DOF 16-10-08", "(Se deroga)") are not rule text.
	reMXAmend   = regexp.MustCompile(`^(?:(?:Regla|Artículo|Apéndice|Nota|Párrafo|Fracción|Inciso)s? (?:adicionad|reformad|derogad)[oa]s? DOF \d{2}-\d{2}-\d{2,4}\.?|\(Se deroga\)\.?)$`)
	reMXRow     = regexp.MustCompile(`^\s*(\d{2,4}\.\s?\d{2}(?:\s*-\s*\d{2,4}\.\s?\d{2})?)\s*\t(.*)$`)
	reMXCode    = `\d{4}(?:\.\d{2})?`
	reMXThrough = regexp.MustCompile(`(` + reMXCode + `) a (` + reMXCode + `)`)
	reMXOr      = regexp.MustCompile(`(` + reMXCode + `),? o (` + reMXCode + `)`)
	reMXChThru  = regexp.MustCompile(`cap[íi]tulos? (\d{1,2}) a (\d{1,2})`)
	// Words that only Spanish text has; any left means the rendering failed.
	reMXSpanish = regexp.MustCompile(`(?i)\b(?:de|la|el|los|las|del|que|siempre|cualquier|otra|otro|cambio|partida|subpartida|cap[íi]tulo|bienes?|excepto|incluyendo|dentro|fuera|grupo|y|o|en|por|con|sea|haya)\b`)
)

// mxPhrases are applied in order after the codes are rewritten.
var mxPhrases = []struct{ es, en string }{
	{"Un cambio a la partida ", "A change to heading "},
	{"Un cambio a la subpartida ", "A change to subheading "},
	{"un cambio a la partida ", "a change to heading "},
	{"un cambio a la subpartida ", "a change to subheading "},
	{"de cualquier otra partida, incluyendo otra partida dentro del grupo", "from any other heading, including another heading within that group"},
	{"de cualquier otra subpartida, incluyendo otra subpartida dentro del grupo", "from any other subheading, including another subheading within that group"},
	{"de cualquier otro capítulo, incluyendo otro capítulo dentro del grupo", "from any other chapter, including another chapter within that group"},
	{"de cualquier partida, incluyendo otra partida dentro del grupo", "from any heading, including another heading within that group"},
	{"de cualquier subpartida, incluyendo otra subpartida dentro del grupo", "from any subheading, including another subheading within that group"},
	{"de cualquier otra partida, incluyendo una partida dentro del grupo", "from any other heading, including another heading within that group"},
	{"de cualquier otra subpartida, incluyendo una subpartida dentro del grupo", "from any other subheading, including another subheading within that group"},
	{" cuando este cambio se efectúe por aplicación de la Regla General 2(a) del Sistema Armonizado", " when that change is pursuant to General Rule of Interpretation 2(a)"},
	{" cuando este cambio se efectúe de acuerdo a la Regla General 2(a) del Sistema Armonizado", " when that change is pursuant to General Rule of Interpretation 2(a)"},
	{", excepto cuando este cambio sea el resultado de un ensamble simple", ", except when resulting from a simple assembly"},
	{" cuando este cambio sea el resultado de un ensamble simple", " when resulting from a simple assembly"},
	{"de cualquier otra subpartida fuera del grupo", "from any other subheading outside that group"},
	{"de cualquier otra partida fuera del grupo", "from any other heading outside that group"},
	{"de cualquier partida fuera del grupo", "from any heading outside that group"},
	{"de cualquier subpartida fuera del grupo", "from any subheading outside that group"},
	{"de cualquier otra partida", "from any other heading"},
	{"de cualquier otra subpartida", "from any other subheading"},
	{"de cualquier otro capítulo", "from any other chapter"},
	{"de cualquier partida", "from any heading"},
	{"de cualquier subpartida", "from any subheading"},
	{", excepto de la partida ", ", except from heading "},
	{", excepto de la subpartida ", ", except from subheading "},
	{" excepto de la partida ", " except from heading "},
	{" excepto de la subpartida ", " except from subheading "},
	{", excepto del capítulo ", ", except from Chapter "},
	{", excepto de los capítulos ", ", except from Chapter "},
	{" o de la partida ", " or from heading "},
	{" o de la subpartida ", " or from subheading "},
	{", o de la partida ", ", or from heading "},
	{", o de la subpartida ", ", or from subheading "},
	{" o del capítulo ", " or from Chapter "},
	{"; o un cambio", "; or a change"},
	{"; o a change", "; or a change"},
}

// MXRender renders one Spanish row text in the parser's English phrasing.
// ok is false when Spanish is left.
func MXRender(es string) (string, bool) {
	s := strings.Join(strings.Fields(es), " ")
	s = CAHeadings(s)
	s = reMXChThru.ReplaceAllString(s, "capítulo $1 through $2")
	for range 3 { // lists of ranges and alternatives: "2714, 2715 o 2716 a 2718"
		s = reMXThrough.ReplaceAllString(s, "$1 through $2")
		s = reMXOr.ReplaceAllString(s, "$1 or $2")
	}
	for _, p := range mxPhrases {
		s = strings.ReplaceAll(s, p.es, p.en)
	}
	s = strings.ReplaceAll(s, "capítulo ", "Chapter ")
	return s, !reMXSpanish.MatchString(s)
}

// BuildMexicoRules reads the Apéndice rows from the text export.
func BuildMexicoRules(txt []byte) ([]Rule, error) {
	i := bytes.Index(txt, []byte("APENDICE DE REGLAS ESPEC"))
	if i < 0 {
		return nil, fmt.Errorf("tariffshift: no Apéndice de Reglas Específicas in the text")
	}
	var (
		rules   []Rule
		section string
		chapter string
		chTitle string
		// titleNext: the next line is a section title.
		titleNext bool
	)
	sc := bufio.NewScanner(bytes.NewReader(txt[i:]))
	sc.Buffer(make([]byte, 1<<20), 1<<22)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), " \r")
		if strings.TrimSpace(line) == "" || reMXAmend.MatchString(strings.TrimSpace(line)) {
			continue
		}
		if strings.HasPrefix(line, "ANEXO ") {
			break // the Apéndice ends where Anexo II starts
		}
		if m := reMXChapter.FindStringSubmatch(line); m != nil {
			chapter, chTitle = fmt.Sprintf("%02s", m[1]), strings.TrimSpace(m[2])
			continue
		}
		if reMXSection.MatchString(line) {
			section = strings.TrimSpace(line)
			titleNext = true
			continue
		}
		if titleNext {
			// The line after "Sección XI" is the section's title.
			section += ": " + strings.TrimSpace(line)
			titleNext = false
			continue
		}
		if m := reMXRow.FindStringSubmatch(line); m != nil && chapter != "" {
			col := mxColumn(m[1])
			hts := CAHeadings(col)
			r, err := ParseRange(hts)
			if err != nil {
				return nil, fmt.Errorf("tariffshift: Apéndice row %q: %w", m[1], err)
			}
			if r.Lo[:2] != chapter || r.Hi[:2] != chapter {
				return nil, fmt.Errorf("tariffshift: Apéndice row %s is not in chapter %s", hts, chapter)
			}
			rules = append(rules, Rule{
				ID:        "Acuerdo DOF 30-08-1994, Anexo I, Apéndice, " + col,
				Paragraph: chapter,
				Section:   strings.TrimSpace(section + "; Capítulo " + strings.TrimLeft(chapter, "0") + ": " + chTitle),
				HTS:       hts,
				Applies:   r,
				TextES:    strings.TrimSpace(m[2]),
			})
			continue
		}
		if len(rules) > 0 && rules[len(rules)-1].Paragraph == chapter {
			// A continuation line (a process list or the rest of a row).
			rules[len(rules)-1].TextES += "\n" + strings.TrimSpace(line)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	var out []Rule
	for _, r := range rules {
		r.TextES = strings.TrimSpace(r.TextES)
		if strings.HasPrefix(r.TextES, "Suprimida") {
			continue // a deleted row: the code has no specific rule (art. 3 II)
		}
		if en, ok := MXRender(r.TextES); ok {
			r.Text = en
		} else {
			r.Text = r.TextES
		}
		r.Alternatives = ParseRule(r.Text)
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("tariffshift: no Apéndice rows")
	}
	sort.SliceStable(out, func(i, j int) bool { return padCode(out[i].Applies.Lo) < padCode(out[j].Applies.Lo) })
	return out, nil
}

// mxColumn reads the code column. One row drops a digit from the end of
// its range ("2905.22-905.29"); the start of the range settles it.
func mxColumn(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, " ", ""), "–", "-")
	lo, hi, ok := strings.Cut(s, "-")
	if !ok {
		return s
	}
	li, hj := strings.Index(lo, "."), strings.Index(hi, ".")
	if li > hj && hj > 0 {
		hi = lo[:li-hj] + hi
	}
	return lo + "-" + hi
}
