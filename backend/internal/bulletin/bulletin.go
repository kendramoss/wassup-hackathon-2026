// Package bulletin reads CBP's Customs Bulletin and Decisions for notices
// that revoke or modify ruling letters (19 U.S.C. § 1625(c)), D400.
//
// Two stages. Extract cuts a bulletin's text (one PDF, converted to text by
// eval/bulletins/pdftext.py) into notices and keeps a short excerpt of each:
// the notice header and, for every attached ruling, its number, subject line
// and the sentences that revoke or modify. Parse reads an excerpt and
// returns the changes. The excerpts are committed
// (eval/data/bulletins/notices.jsonl), so the parse is rebuilt and checked
// without the PDFs.
package bulletin

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
)

// Version names the extraction and parse rules below.
const Version = "bulletin-v1"

// Issue is one fetched bulletin.
type Issue struct {
	Volume int    `json:"volume"`
	Number int    `json:"number"`
	Date   string `json:"date"` // issue date, YYYY-MM-DD
	URL    string `json:"url"`
}

// Cite names where a change was published.
func (i Issue) Cite() string {
	return fmt.Sprintf("Customs Bulletin vol. %d no. %d (%s)", i.Volume, i.Number, i.Date)
}

// Notice is one ruling-letter notice in a bulletin, as committed.
type Notice struct {
	Issue
	// Page is the printed bulletin page the notice starts on (0 when the
	// page footer could not be read).
	Page int `json:"page"`
	// Title is the notice heading in CBP's words.
	Title string `json:"title"`
	// Excerpt is the header (title to the effective date or comment
	// deadline) and, per attached ruling, a "[attachment <id>]" marker, its
	// subject line and the revoking or modifying sentences.
	Excerpt string `json:"excerpt"`
}

// Change is one ruling a notice revokes or modifies, or proposes to.
type Change struct {
	Ruling string `json:"ruling"`
	// By is the attached ruling that makes the change (the proposed ruling
	// for a proposal).
	By   string `json:"by"`
	Kind string `json:"kind"` // "revoked" or "modified"
	// Status is "final" or "proposed".
	Status string `json:"status"`
	// Effective is the date a final change takes effect (YYYY-MM-DD).
	Effective string `json:"effective,omitempty"`
	// CommentsBy is a proposal's comment deadline, when stated.
	CommentsBy string `json:"comments_by,omitempty"`
	Quote      string `json:"quote"`
}

// Result is the parse of one notice.
type Result struct {
	Status string // "final", "proposed" or "withdrawn"
	// Effective and CommentsBy are read from the header.
	Effective, CommentsBy string
	// Expected counts the rulings the ACTION line says are revoked or
	// modified (0 when it gives no count).
	Expected int
	// Subject lists the rulings the changing letters' subject lines name
	// as revoked or modified; ByLaw is set when a subject line says some
	// are changed "by operation of law" (then the ACTION count is a floor).
	Subject []string
	ByLaw   bool
	Changes []Change
}

// Problem says why a parse cannot be trusted ("" when it can): a final
// notice without an effective date, no ruling found, a ruling the changing
// letters' subject lines name that their text does not change, or a count
// other than the notice's ACTION line states (a floor when some rulings are
// changed by operation of law).
func (r Result) Problem() string {
	if r.Status == "withdrawn" {
		return ""
	}
	if r.Status == "final" && r.Effective == "" {
		return "final notice without an effective date"
	}
	got := distinct(r.Changes)
	n := len(got)
	if n == 0 {
		return "no ruling found"
	}
	var missing []string
	for _, id := range r.Subject {
		if !got[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		return "the subject lines name " + strings.Join(missing, " ") + ", which the text does not revoke or modify"
	}
	if r.Expected > 0 && (n < r.Expected || (n > r.Expected && !r.ByLaw)) {
		return fmt.Sprintf("the notice names %d ruling letters, the attachments %d", r.Expected, n)
	}
	return ""
}

func distinct(cs []Change) map[string]bool {
	m := map[string]bool{}
	for _, c := range cs {
		m[c.Ruling] = true
	}
	return m
}

var (
	pageMark   = regexp.MustCompile(`\f\[\[page (\d+)\]\]\n`)
	footerLead = regexp.MustCompile(`^\s*(\d+)\s+CUSTOMS BULLETIN AND DECISIONS\b`)
	footerTail = regexp.MustCompile(`CUSTOMS BULLETIN AND DECISIONS\b.*?(\d+)\s*$`)
	footerOnly = regexp.MustCompile(`^\s*(\d+)\s*$`)
	hyphenEOL  = regexp.MustCompile(`([a-z])-\n([a-z])`)
	actionHead = regexp.MustCompile(`(?m)^ACTION:`)
	actionLine = regexp.MustCompile(`(?s)ACTION:\s*(.*?)\n(?:SUMMARY|DATES|EFFECTIVE DATE|ADDRESSES):`)
	sectionEnd = regexp.MustCompile(`(?m)^(?:U\.S\. Court of International Trade|U\.S\. Court of Appeals for the Federal Circuit|Index)\s*$`)
	headerEnd  = regexp.MustCompile(`(?m)^(?:FOR FURTHER INFORMATION CONTACT|SUPPLEMENTARY INFORMATION):`)
	// attachHead is a line holding only a ruling number: the first line of
	// an attached ruling ("HQ H325364", "N068156").
	attachHead = regexp.MustCompile(`(?m)^(?:(?:PROPOSED\s+)?HQ\s+)?([HN][0-9]{6}|[A-Z][0-9]{5}|[0-9]{6})\s*$`)
	reLine     = regexp.MustCompile(`(?m)^RE:\s`)
	wsRun      = regexp.MustCompile(`\s+`)
)

// page is a printed page number from offset on.
type page struct{ at, n int }

// Normalize turns pdftext.py output into plain text: NFKC (the PDFs use
// ligatures), footers removed, words broken at line ends joined. It returns
// the printed page each part of the text is on.
func Normalize(raw string) (string, []page) {
	raw = norm.NFKC.String(raw)
	idx := pageMark.FindAllStringIndex(raw, -1)
	var b strings.Builder
	var pages []page
	for i, loc := range idx {
		end := len(raw)
		if i+1 < len(idx) {
			end = idx[i+1][0]
		}
		body := strings.TrimRight(raw[loc[1]:end], "\n ")
		printed := 0
		if j := strings.LastIndex(body, "\n"); j >= 0 || body != "" {
			last := body[j+1:]
			if m := footerLead.FindStringSubmatch(last); m != nil {
				printed, _ = strconv.Atoi(m[1])
				body = body[:max(j, 0)]
			} else if m := footerTail.FindStringSubmatch(last); m != nil {
				printed, _ = strconv.Atoi(m[1])
				body = body[:max(j, 0)]
			} else if m := footerOnly.FindStringSubmatch(last); m != nil {
				printed, _ = strconv.Atoi(m[1])
				body = body[:max(j, 0)]
			}
		}
		pages = append(pages, page{at: b.Len(), n: printed})
		b.WriteString(body)
		b.WriteString("\n")
	}
	if len(idx) == 0 {
		b.WriteString(raw)
	}
	text := hyphenEOL.ReplaceAllString(b.String(), "$1$2")
	// Joining shifts offsets by two bytes per join; recompute page starts
	// by counting joins before each start.
	joins := hyphenEOL.FindAllStringIndex(b.String(), -1)
	for i := range pages {
		k := 0
		for _, j := range joins {
			if j[0] < pages[i].at {
				k++
			}
		}
		pages[i].at -= 2 * k
	}
	return text, pages
}

func pageAt(pages []page, off int) int {
	n := 0
	for _, p := range pages {
		if p.at > off {
			break
		}
		n = p.n
	}
	return n
}

// Extract cuts a bulletin's text into its ruling-letter notices.
func Extract(issue Issue, raw string) []Notice {
	text, pages := Normalize(raw)
	lines := strings.SplitAfter(text, "\n")
	lineStart := make([]int, len(lines)+1)
	for i, l := range lines {
		lineStart[i+1] = lineStart[i] + len(l)
	}
	lineOf := func(off int) int {
		return sort.Search(len(lines), func(i int) bool { return lineStart[i+1] > off })
	}
	// Each notice starts at its title: the upper-case lines above its
	// ACTION line, past the AGENCY lines (which the PDF text sometimes
	// lacks the label of).
	type span struct{ start, agency int }
	var spans []span
	for _, loc := range actionHead.FindAllStringIndex(text, -1) {
		s := lineOf(loc[0])
		for k := 0; k < 3 && s > 0 && !isTitleLine(lines[s-1]); k++ {
			s--
		}
		agency := lineStart[s]
		for s > 0 && isTitleLine(lines[s-1]) {
			s--
		}
		spans = append(spans, span{start: lineStart[s], agency: agency})
	}
	var out []Notice
	for i, sp := range spans {
		end := len(text)
		if i+1 < len(spans) {
			end = spans[i+1].start
		}
		if loc := sectionEnd.FindStringIndex(text[sp.agency:end]); loc != nil {
			end = sp.agency + loc[0]
		}
		body := text[sp.start:end]
		action := ""
		if m := actionLine.FindStringSubmatch(body); m != nil {
			action = strings.ToLower(oneLine(m[1]))
		}
		if !strings.Contains(action, "ruling letter") {
			continue
		}
		title := oneLine(text[sp.start:sp.agency])
		out = append(out, Notice{Issue: issue, Page: pageAt(pages, sp.agency), Title: title, Excerpt: excerpt(body)})
	}
	return out
}

func isTitleLine(l string) bool {
	l = strings.TrimSpace(l)
	if l == "" || strings.HasPrefix(l, "19 CFR") || strings.HasPrefix(l, "CBP DEC") || strings.HasPrefix(l, "RIN ") {
		return false
	}
	up, low := 0, 0
	for _, r := range l {
		switch {
		case r >= 'A' && r <= 'Z':
			up++
		case r >= 'a' && r <= 'z':
			low++
		}
	}
	return up >= 3 && low*5 <= up
}

func oneLine(s string) string { return strings.TrimSpace(wsRun.ReplaceAllString(s, " ")) }

// excerpt keeps what Parse reads: the header, and per attached ruling its
// number, subject line and change sentences.
func excerpt(body string) string {
	var b strings.Builder
	head := body
	if loc := headerEnd.FindStringIndex(body); loc != nil {
		head = body[:loc[0]]
	}
	b.WriteString(oneLine(head))
	b.WriteString("\n")
	heads := attachHead.FindAllStringSubmatchIndex(body, -1)
	for i, h := range heads {
		if h[0] < len(head) {
			continue
		}
		end := len(body)
		if i+1 < len(heads) {
			end = heads[i+1][0]
		}
		att := body[h[0]:end]
		id := body[h[2]:h[3]]
		subject := ""
		if loc := reLine.FindStringIndex(att); loc != nil && loc[0] < 900 {
			// The subject runs to the salutation.
			rest := att[loc[0]:]
			if k := strings.Index(rest, "\nDEAR "); k > 0 && k < 600 {
				rest = rest[:k]
			} else if k := strings.Index(rest, "\n\n"); k > 0 {
				rest = rest[:k]
			} else if len(rest) > 400 {
				rest = rest[:400]
			}
			subject = oneLine(rest)
		}
		sents := changeSentences(oneLine(att))
		if subject == "" && len(sents) == 0 {
			continue
		}
		fmt.Fprintf(&b, "[attachment %s]\n", id)
		if subject != "" {
			b.WriteString(subject + "\n")
		}
		for _, s := range sents {
			b.WriteString(s + "\n")
		}
	}
	return b.String()
}

var (
	herebyPassive = regexp.MustCompile(`(?i)\b(?:is|are)\s+(?:hereby\s+)?(revoked|modified)\b`)
	herebyActive  = regexp.MustCompile(`(?i)\bwe\s+hereby\s+(revoke|modify)\b`)
	effectHeader  = regexp.MustCompile(`EFFECT ON OTHER RULINGS:?`)
	// sentenceEnd is a full stop that ends a sentence: not inside a ruling
	// number, a statute ("U.S.C.") or a dated abbreviation ("Mar. 11").
	sentenceEnd = regexp.MustCompile(`[^A-Z.][a-z]{3,}\.\s+[A-Z]|[0-9]\.\s+[A-Z]|\)\.\s+[A-Z]|:\s`)
)

// changeSentences returns the sentences in an attached ruling that revoke or
// modify ("... is hereby REVOKED", "we hereby revoke ...", the EFFECT ON
// OTHER RULINGS section). A passive verb without "hereby" counts only in
// that section.
func changeSentences(text string) []string {
	var out []string
	eff := -1
	if loc := effectHeader.FindAllStringIndex(text, -1); len(loc) > 0 {
		eff = loc[len(loc)-1][0]
	}
	prev := 0
	for _, m := range herebyPassive.FindAllStringIndex(text, -1) {
		hereby := strings.Contains(strings.ToLower(text[m[0]:m[1]]), "hereby")
		if !hereby && (eff < 0 || m[0] < eff) {
			continue
		}
		start := max(prev, m[0]-1500)
		window := text[start:m[0]]
		if eff >= start && eff < m[0] {
			window = text[eff:m[0]]
		} else if ends := sentenceEnd.FindAllStringIndex(window, -1); len(ends) > 0 {
			window = window[ends[len(ends)-1][1]-1:]
		}
		out = append(out, strings.TrimSpace(window+text[m[0]:m[1]]))
		prev = m[1]
	}
	for _, m := range herebyActive.FindAllStringIndex(text, -1) {
		end := min(len(text), m[1]+400)
		w := text[m[0]:end]
		if i := strings.Index(w[len(text[m[0]:m[1]]):], ". "); i >= 0 {
			w = w[:len(text[m[0]:m[1]])+i]
		}
		out = append(out, strings.TrimSpace(w))
	}
	return out
}

var (
	// ruleRun matches ruling numbers as notices write them: after "NY",
	// "HQ" or "PC" (also "New York Ruling Letter (“NY”) N068156"), and the
	// numbers listed after it ("NY N283085, N166443, and D80822"), so style
	// and protest numbers are never read as rulings.
	ruleRun     = regexp.MustCompile(`\b(?:NY|HQ|PC)["”’)]*\s+` + ruleID + `(?:\s*(?:,\s*and|,|and)\s+(?:(?:NY|HQ|PC)\s+)?` + ruleID + `)*`)
	ruleIDs     = regexp.MustCompile(ruleID)
	datedPhrase = regexp.MustCompile(`(?i),?\s*(?:dated|issued)\s+(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Sept|Oct|Nov|Dec)[a-z]*\.?\s+[0-9]{1,2},\s*[0-9]{4}`)
	datePhrase  = regexp.MustCompile(`(?i)\b(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Sept|Oct|Nov|Dec)[a-z]*\.?\s+[0-9]{1,2},\s*[0-9]{4}`)
	effectiveRe = regexp.MustCompile(`(?i)EFFECTIVE DATE:.*?on or after\s+([A-Z][a-z]+\.?\s+[0-9]{1,2},\s*[0-9]{4})`)
	commentsRe  = regexp.MustCompile(`(?i)DATES?:.*?(?:on or before|no later than)\s+([A-Z][a-z]+\.?\s+[0-9]{1,2},\s*[0-9]{4})`)
	actionRe    = regexp.MustCompile(`(?i)ACTION:\s*(.*?)(?:SUMMARY|DATES|EFFECTIVE DATE|ADDRESSES):`)
	countRe     = regexp.MustCompile(`(?i)\b(?:revocation|modification)\s+of\s+([a-z]+|[0-9]+)(?:\s+\([0-9]+\))?\s+(?:pre-classification\s+)?ruling\s+letters?`)
	attachRe    = regexp.MustCompile(`(?m)^\[attachment ([A-Z0-9]+)\]$`)
	activeVerb  = regexp.MustCompile(`(?i)^we\s+hereby\s+(revoke|modify)\b`)
	passiveVerb = regexp.MustCompile(`(?i)\b(?:is|are)\s+(?:hereby\s+)?(revoked|modified)$`)
	subjectVerb = regexp.MustCompile(`(?i)\b(revocation|modification)\s+(?:of|to)\s+([^;]*)`)
)

const ruleID = `\b(?:[A-Z][0-9]{5,6}|[0-9]{6})\b`

var numberWords = map[string]int{"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8,
	"nine": 9, "ten": 10, "eleven": 11, "twelve": 12, "thirteen": 13, "fourteen": 14, "fifteen": 15, "sixteen": 16,
	"seventeen": 17, "eighteen": 18, "nineteen": 19, "twenty": 20}

// ParseDate reads "July 22, 2024" (or "Sept. 3, 2025").
func ParseDate(s string) (string, bool) {
	s = strings.Join(strings.Fields(strings.Replace(s, ".", "", 1)), " ")
	for _, layout := range []string{"January 2, 2006", "Jan 2, 2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format(time.DateOnly), true
		}
	}
	if strings.HasPrefix(s, "Sept ") {
		return ParseDate("Sep " + s[5:])
	}
	return "", false
}

// Parse reads a notice excerpt. A final change takes effect on the notice's
// effective date; a proposal is kept as proposed. A ruling that is the
// attachment itself (the letter being revoked, attached for reference) is
// never an edge.
func Parse(n Notice) Result {
	head, atts := n.Excerpt, ""
	if loc := attachRe.FindStringIndex(n.Excerpt); loc != nil {
		head, atts = n.Excerpt[:loc[0]], n.Excerpt[loc[0]:]
	}
	var r Result
	action := ""
	if m := actionRe.FindStringSubmatch(head); m != nil {
		action = strings.ToLower(m[1])
	}
	if m := effectiveRe.FindStringSubmatch(head); m != nil {
		r.Effective, _ = ParseDate(m[1])
	}
	if m := commentsRe.FindStringSubmatch(head); m != nil {
		r.CommentsBy, _ = ParseDate(m[1])
	}
	switch {
	case strings.HasPrefix(action, "withdrawal"):
		r.Status = "withdrawn"
	case r.Effective != "" || (!strings.Contains(action, "proposed") && strings.Contains(action, "notice of")):
		r.Status = "final"
	default:
		r.Status = "proposed"
	}
	for _, m := range countRe.FindAllStringSubmatch(action, -1) {
		k, ok := numberWords[strings.ToLower(m[1])]
		if !ok {
			k, _ = strconv.Atoi(m[1])
		}
		r.Expected += k
	}
	if r.Status == "withdrawn" {
		return r
	}
	heads := attachRe.FindAllStringSubmatchIndex(atts, -1)
	seen := map[string]bool{}
	for i, h := range heads {
		end := len(atts)
		if i+1 < len(heads) {
			end = heads[i+1][0]
		}
		by := atts[h[2]:h[3]]
		body := strings.Split(strings.TrimSpace(atts[h[1]:end]), "\n")
		add := func(id, kind, quote string) {
			if id == by || seen[id+"|"+by] {
				return
			}
			seen[id+"|"+by] = true
			c := Change{Ruling: id, By: by, Kind: kind, Status: r.Status, Quote: clip(quote)}
			if r.Status == "final" {
				c.Effective = r.Effective
			} else {
				c.CommentsBy = r.CommentsBy
			}
			r.Changes = append(r.Changes, c)
		}
		before := len(r.Changes)
		subject := ""
		for _, line := range body {
			if strings.HasPrefix(line, "RE:") {
				subject = line
				continue
			}
			kind := ""
			ids := line
			if m := activeVerb.FindStringSubmatch(line); m != nil {
				kind = map[string]string{"revoke": "revoked", "modify": "modified"}[strings.ToLower(m[1])]
			} else if m := passiveVerb.FindStringSubmatch(line); m != nil {
				kind = strings.ToLower(m[1])
			}
			if kind == "" {
				continue
			}
			for _, id := range idsIn(ids) {
				add(id, kind, line)
			}
		}
		// The subject line counts only when the letter names nothing
		// itself, and only for a ruling the HQ letter is about.
		if len(r.Changes) == before && subject != "" && by[0] == 'H' {
			for _, m := range subjectVerb.FindAllStringSubmatch(subject, -1) {
				kind := "modified"
				if strings.EqualFold(m[1], "revocation") {
					kind = "revoked"
				}
				for _, id := range idsIn(m[2]) {
					add(id, kind, subject)
				}
			}
		}
	}
	// An attached ruling that another attachment revokes or modifies is
	// the old letter, attached for reference: what it says about other
	// rulings is history, not this notice's change.
	changed := map[string]bool{}
	for _, c := range r.Changes {
		changed[c.Ruling] = true
	}
	kept := r.Changes[:0]
	for _, c := range r.Changes {
		if !changed[c.By] {
			kept = append(kept, c)
		}
	}
	r.Changes = kept
	by := map[string]bool{}
	for _, c := range r.Changes {
		by[c.By] = true
	}
	for i, h := range heads {
		if !by[atts[h[2]:h[3]]] {
			continue
		}
		end := len(atts)
		if i+1 < len(heads) {
			end = heads[i+1][0]
		}
		for _, line := range strings.Split(atts[h[1]:end], "\n") {
			if !strings.HasPrefix(line, "RE:") {
				continue
			}
			if strings.Contains(strings.ToLower(line), "operation of law") {
				r.ByLaw = true
			}
			for _, m := range subjectVerb.FindAllStringSubmatch(line, -1) {
				for _, id := range idsIn(m[2]) {
					if !slices.Contains(r.Subject, id) {
						r.Subject = append(r.Subject, id)
					}
				}
			}
		}
	}
	return r
}

func idsIn(s string) []string {
	var out []string
	s = datedPhrase.ReplaceAllString(s, "")
	for _, run := range ruleRun.FindAllString(s, -1) {
		for _, id := range ruleIDs.FindAllString(run, -1) {
			if !slices.Contains(out, id) {
				out = append(out, id)
			}
		}
	}
	return out
}

func clip(s string) string {
	r := []rune(s)
	if len(r) > 300 {
		return string(r[:299]) + "…"
	}
	return s
}
