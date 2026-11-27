package tariffwatch

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Heading is one Chapter 99 heading as printed in the HTS PDF's text.
type Heading struct {
	Number      string // 9903.91.12
	Description string // the article description, without the dot leader
	General     string // the column 1 general rate text
}

var (
	headingLine = regexp.MustCompile(`^(9903\.\d\d\.\d\d)$`)
	pageNoise   = regexp.MustCompile(`^(99 - [IVX]+ - \d+|Annotated for Statistical Reporting Purposes|[IVX]+|Rates of Duty|Heading/|Stat\.|Unit|Suf-|fix|of|Article Description|Quantity|General|Special|1|2|Subheading|Subheading 2|1/)$`)
	dotLeader   = regexp.MustCompile(`\.{5,}`)
)

// ParseHeadings reads the Chapter 99 headings out of text extracted from the
// HTS Chapter 99 PDF (any extractor that keeps the PDF's line order, such as
// pdftotext or PDFKit). It is a reader for the layout of that PDF, not a
// general parser: a heading is a line holding only its number, the
// description runs to the dot leader, and the column 1 general rate follows
// to the next heading or the end-of-page footnote.
func ParseHeadings(text string) []Heading {
	lines := strings.Split(text, "\n")
	var out []Heading
	for i := 0; i < len(lines); i++ {
		m := headingLine.FindStringSubmatch(strings.TrimSpace(lines[i]))
		if m == nil {
			continue
		}
		h := Heading{Number: m[1]}
		var desc, gen []string
		inGen := false
		j := i + 1
	block:
		for ; j < len(lines); j++ {
			l := strings.TrimSpace(lines[j])
			switch {
			case headingLine.MatchString(l):
				break block
			case strings.HasPrefix(l, "1/ See "):
				break block
			case l == "" || pageNoise.MatchString(l):
				continue
			case inGen:
				gen = append(gen, l)
			case dotLeader.MatchString(l):
				if before := strings.TrimSpace(dotLeader.Split(l, 2)[0]); before != "" {
					desc = append(desc, before)
				}
				inGen = true
			default:
				desc = append(desc, l)
			}
		}
		h.Description = strings.Join(strings.Fields(strings.Join(desc, " ")), " ")
		h.General = strings.Join(strings.Fields(strings.Join(gen, " ")), " ")
		out = append(out, h)
		i = j - 1
	}
	return out
}

var months = map[string]time.Month{"January": 1, "February": 2, "March": 3, "April": 4, "May": 5, "June": 6, "July": 7, "August": 8, "September": 9, "October": 10, "November": 11, "December": 12}

const dateRE = `((?:January|February|March|April|May|June|July|August|September|October|November|December)\s+\d{1,2},\s+\d{4})`

var (
	fromRE     = regexp.MustCompile(`on or after\s+` + dateRE)
	throughRE  = regexp.MustCompile(`through\s+` + dateRE)
	beforeRE   = regexp.MustCompile(`(?:and\s+)?before\s+` + dateRE)
	htsRE      = regexp.MustCompile(`\b(\d{4}\.\d{2}\.\d{2}(?:\d{2})?)\b`)
	plusRateRE = regexp.MustCompile(`\+\s*(\d+(?:\.\d+)?)%`)
)

func parseDate(s string) (time.Time, bool) {
	f := strings.Fields(strings.ReplaceAll(s, ",", ""))
	if len(f) != 3 {
		return time.Time{}, false
	}
	mo, ok := months[f[0]]
	d, e1 := strconv.Atoi(f[1])
	y, e2 := strconv.Atoi(f[2])
	if !ok || e1 != nil || e2 != nil {
		return time.Time{}, false
	}
	return time.Date(y, mo, d, 0, 0, 0, 0, time.UTC), true
}

// StartDate is the "on or after" day of a heading's description, or "".
func (h Heading) StartDate() string {
	if m := fromRE.FindStringSubmatch(h.Description); m != nil {
		if t, ok := parseDate(m[1]); ok {
			return t.Format(time.DateOnly)
		}
	}
	return ""
}

// EndDate is the last entry day of a heading's description: "through D" is
// that day, "before D" the day before it. "" when it has none.
func (h Heading) EndDate() string {
	if m := throughRE.FindStringSubmatch(h.Description); m != nil {
		if t, ok := parseDate(m[1]); ok {
			return t.Format(time.DateOnly)
		}
	}
	if m := beforeRE.FindStringSubmatch(h.Description); m != nil {
		if t, ok := parseDate(m[1]); ok {
			return t.AddDate(0, 0, -1).Format(time.DateOnly)
		}
	}
	return ""
}

// AddRate is the "+ N%" of the general rate as a fraction, or false.
func (h Heading) AddRate() (float64, bool) {
	m := plusRateRE.FindStringSubmatch(h.General)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	return v / 100, true
}

// Subheadings are the HTS numbers the description names (other Chapter 99
// headings are not tariff lines and are left out). A 10-digit statistical
// number is kept as its 8-digit subheading.
func (h Heading) Subheadings() []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range htsRE.FindAllStringSubmatch(h.Description, -1) {
		if strings.HasPrefix(m[1], "99") {
			continue
		}
		s := m[1][:10] // NNNN.NN.NN
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// IsExclusion reports whether the heading is a Section 301 product-exclusion
// heading: its description says each article is covered by an exclusion
// granted by the U.S. Trade Representative.
func (h Heading) IsExclusion() bool {
	d := strings.ToLower(h.Description)
	return strings.Contains(d, "covered by an exclusion granted by the u.s. trade representative")
}

// OfChina reports whether the description is about articles the product of
// China.
func (h Heading) OfChina() bool {
	return strings.Contains(h.Description, "product of China") || strings.Contains(h.Description, "products of China")
}

var revisionRE = regexp.MustCompile(`Revision (\d+) \((\d{4})\)`)

// RevisionName reads "HTS 2026 Revision 23" from the running header of the
// extracted PDF text, or "".
func RevisionName(text string) string {
	if m := revisionRE.FindStringSubmatch(text); m != nil {
		return "HTS " + m[2] + " Revision " + m[1]
	}
	return ""
}
