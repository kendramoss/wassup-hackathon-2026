package corpus

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// SupersessionVersion names the detection rules below (D320; v2 adds the
// Customs Bulletin notices, D400).
const SupersessionVersion = "supersession-v2"

// SupersessionRule is the rule text written into the committed map.
const SupersessionRule = "A ruling is superseded when a later CBP ruling in the corpus states that it is revoked or modified: " +
	"the EFFECT ON OTHER RULINGS section, any \"is/are hereby revoked/modified\" or \"we hereby revoke/modify\" sentence, " +
	"a subject line that starts \"Revocation of\" or \"Modification of/to\", or a later ruling stating that one ruling " +
	"\"was revoked/modified by\" another. The date is the superseding ruling's issue date. Notices of proposed " +
	"revocation and affirmations are not supersessions. A ruling revoked after a case date stays usable for that case. " +
	"Final revocations and modifications published in fetched Customs Bulletin notices are added, dated by the notice's effective date, " +
	"with the bulletin's volume, number, page and URL (D400); proposed ones are listed under \"proposed\" and never bar a precedent."

// ruleIDs matches ruling numbers as CBP writes them in revocation letters:
// current NY and HQ numbers (N331974, H303816), older NY numbers (E87234,
// R02589) and older numeric HQ numbers (963233).
var ruleIDs = regexp.MustCompile(`\b(?:[A-Z][0-9]{5,6}|[0-9]{6})\b`)

var (
	wsRun        = regexp.MustCompile(`\s+`)
	herebyPassiv = regexp.MustCompile(`(?i)\b(?:is|are)\s+hereby\s+(revoked|modified)\b`)
	herebyActive = regexp.MustCompile(`(?i)\bwe\s+hereby\s+(revoke|modify)\b`)
	effectHeader = regexp.MustCompile(`EFFECT ON OTHER RULINGS:?`)
	subjectEdge  = regexp.MustCompile(`(?i)\b(revocation|modification)\s+(?:of|to)\s+([^;]*)`)
	byReference  = regexp.MustCompile(`(?i)\b((?:NY|HQ)\s+[A-Z]?[0-9]{5,6})\s+(?:was|were|has been)\s+(revoked|modified)\s+by\s+(?:NY|HQ)\s+([NH][0-9]{6})(?:,\s+dated\s+([A-Z][a-z]+\.?\s+[0-9]{1,2},\s*[0-9]{4}))?`)
	// sentenceEnd is a full stop that ends a sentence: not inside a ruling
	// number, a statute ("U.S.C.") or a dated abbreviation ("Mar. 11").
	sentenceEnd = regexp.MustCompile(`[^A-Z.][a-z]{3,}\.\s+[A-Z]|[0-9]\.\s+[A-Z]|\)\.\s+[A-Z]`)
)

// SupersessionInput is one ruling as the detector reads it.
type SupersessionInput struct {
	ID      string
	Date    time.Time
	Subject string
	// Text is the full ruling text, or "" when only the subject is known.
	Text string
}

// DetectSupersession returns the rulings that in revokes or modifies, plus
// any "X was modified by Y" statements it makes about other rulings. Edges
// from the subject line are kept only when the text names no edge (the text
// is authoritative); Pools is left empty.
func DetectSupersession(in SupersessionInput, dateOf func(id string) (time.Time, bool)) []domain.Supersession {
	text := wsRun.ReplaceAllString(in.Text, " ")
	date := in.Date.Format(time.DateOnly)
	var out []domain.Supersession
	seen := map[string]bool{}
	add := func(e domain.Supersession) {
		key := e.Ruling + "|" + e.By
		if e.Ruling == "" || e.Ruling == e.By || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, e)
	}

	// 1. "<ids> is/are hereby revoked|modified": the ids since the previous
	// sentence end (or section header) belong to this verb.
	prev := 0
	for _, m := range herebyPassiv.FindAllStringSubmatchIndex(text, -1) {
		start := max(prev, m[0]-300)
		window := text[start:m[0]]
		if loc := effectHeader.FindAllStringIndex(window, -1); len(loc) > 0 {
			window = window[loc[len(loc)-1][1]:]
		} else if ends := sentenceEnd.FindAllStringIndex(window, -1); len(ends) > 0 {
			window = window[ends[len(ends)-1][1]-1:]
		}
		source := "hereby"
		if hdr := effectHeader.FindAllStringIndex(text[:m[0]], -1); len(hdr) > 0 && !strings.Contains(text[hdr[len(hdr)-1][1]:m[0]], "Sincerely") {
			source = "effect_on_other_rulings"
		}
		kind := strings.ToLower(text[m[2]:m[3]])
		for _, id := range ruleIDs.FindAllString(stripDates(window), -1) {
			add(domain.Supersession{Ruling: id, By: in.ID, Date: date, Kind: kind, Source: source, SourceRuling: in.ID,
				Quote: clip(strings.TrimLeft(window, ". ;,") + text[m[0]:m[1]])})
		}
		prev = m[1]
	}
	// 2. "we hereby revoke|modify <ids>": the ids up to the sentence end.
	for _, m := range herebyActive.FindAllStringSubmatchIndex(text, -1) {
		end := min(len(text), m[1]+200)
		window := text[m[1]:end]
		if i := strings.Index(window, ". "); i >= 0 {
			window = window[:i]
		}
		kind := map[string]string{"revoke": "revoked", "modify": "modified"}[strings.ToLower(text[m[2]:m[3]])]
		for _, id := range ruleIDs.FindAllString(stripDates(window), -1) {
			add(domain.Supersession{Ruling: id, By: in.ID, Date: date, Kind: kind, Source: "hereby", SourceRuling: in.ID,
				Quote: clip(text[m[0]:m[1]] + window)})
		}
	}
	textEdges := len(out)
	// 3. The subject line, only when the text named nothing (subject-only
	// input, or a letter without a hereby sentence).
	if textEdges == 0 {
		for _, m := range subjectEdge.FindAllStringSubmatch(in.Subject, -1) {
			kind := "modified"
			if strings.EqualFold(m[1], "revocation") {
				kind = "revoked"
			}
			for _, id := range ruleIDs.FindAllString(stripDates(m[2]), -1) {
				add(domain.Supersession{Ruling: id, By: in.ID, Date: date, Kind: kind, Source: "subject", SourceRuling: in.ID,
					Quote: clip(strings.TrimSpace(m[0]))})
			}
		}
	}
	// 4. "HQ X was modified by HQ Y, dated <date>": an edge between two
	// other rulings, dated by Y.
	for _, m := range byReference.FindAllStringSubmatch(text, -1) {
		old := ruleIDs.FindString(m[1])
		by := m[3]
		var d time.Time
		if t, ok := dateOf(by); ok {
			d = t
		} else if t, err := time.Parse("January 2, 2006", strings.Replace(m[4], ".", "", 1)); err == nil {
			d = t
		}
		if d.IsZero() || by == in.ID {
			continue
		}
		add(domain.Supersession{Ruling: old, By: by, Date: d.Format(time.DateOnly), Kind: strings.ToLower(m[2]),
			Source: "reference", SourceRuling: in.ID, Quote: clip(m[0])})
	}
	return out
}

// datePhrase removes "dated March 20, 2019" and similar, so day and year
// numbers are never read as ruling numbers.
var datePhrase = regexp.MustCompile(`(?i)\b(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Sept|Oct|Nov|Dec)[a-z]*\.?\s+[0-9]{1,2},\s*[0-9]{4}`)

func stripDates(s string) string { return datePhrase.ReplaceAllString(s, " ") }

func clip(s string) string {
	r := []rune(s)
	if len(r) > 300 {
		return string(r[:299]) + "…"
	}
	return s
}

// SortSupersessions orders edges by superseded ruling, then date, then the
// superseding ruling, so the committed map is stable.
func SortSupersessions(es []domain.Supersession) {
	sort.Slice(es, func(i, j int) bool {
		a, b := es[i], es[j]
		if a.Ruling != b.Ruling {
			return a.Ruling < b.Ruling
		}
		if a.Date != b.Date {
			return a.Date < b.Date
		}
		return a.By < b.By
	})
}
