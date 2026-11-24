// Package jprules supplies published Japan-import preferential origin rule
// text to the model. It does not apply the rules or check tariff eligibility.
package jprules

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/rcep"
)

//go:embed data/schedules.json
var schedulesJSON []byte

//go:embed data/ajcep_general.json
var generalJSON []byte

//go:embed data/hs2017_codes.txt
var hs2017CodesTXT []byte

//go:embed data/hs2022_codes.txt
var hs2022CodesTXT []byte

const rcepAnnexURL = "https://rcepsec.org/wp-content/uploads/2024/07/RCEP-PSR-in-HS-2022-adopted-by-RJC-30June2022-corrected20Aug2022.pdf"

var (
	rcepStart = time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	hsPattern = regexp.MustCompile(`^(?:[0-9]{4}(?:[0-9]{2}|[0-9]{5})?|[0-9]{2}\.[0-9]{2}|[0-9]{4}\.[0-9]{2}(?:-?[0-9]{3})?)$`)
)

type row struct {
	Start       string `json:"start"`
	End         string `json:"end"`
	Page        int    `json:"page"`
	Text        string `json:"text"`
	Description string `json:"description"`
	Blocked     bool   `json:"blocked"`
	// Default marks a rule stated for a group of goods (a default sentence, a
	// chapter, heading or range) rather than for one listed code. It applies
	// only to codes that exist in the embedded code list of the schedule's HS
	// edition (HS 2017 unless the schedule says otherwise), never to a code
	// that exists in no edition.
	Default bool `json:"default,omitempty"`
	// Ref locates the row in a source that has no PDF pages, such as an
	// e-Gov law page ("別表 row 37"). Page is then 1 and the citation carries
	// the ref instead of a page number and link fragment.
	Ref string `json:"ref,omitempty"`
}

type note struct {
	Scope string `json:"scope"`
	Key   string `json:"key"`
	Text  string `json:"text"`
	// Page is the PDF page of the note. Zero marks an Unde Labs caveat note,
	// which is not treaty text and has no source page or link.
	Page int `json:"page"`
	// Ref locates the note in a source without PDF pages (see row.Ref).
	Ref string `json:"ref,omitempty"`
	// Applies narrows a scope note to codes inside these [start, end] digit
	// prefix ranges (2 to 6 digits, both ends of equal length). Empty means
	// the whole chapter or section named by Scope and Key.
	Applies [][2]string `json:"applies,omitempty"`
	// SourceURL and Citation, set together or not at all, name a source other
	// than the schedule's own, such as an annex in a separate PDF. The note's
	// evidence is then cited "<Citation>, PDF page <Page>" and links to
	// "<SourceURL>#page=<Page>" instead of the schedule's citation and URL. A
	// note with them has a page of 1 or more and no Ref.
	SourceURL string `json:"source_url,omitempty"`
	Citation  string `json:"citation,omitempty"`
}

// validRef reports whether a locator is a non-empty, trimmed string.
func validRef(ref string) bool { return ref != "" && ref == strings.TrimSpace(ref) }

// validateNoteSource rejects a note that names half a source, a source that is
// not a plain https link (the page fragment is added by the evidence), or a
// source on a note that has no page of the source PDF.
func validateNoteSource(n note) error {
	if n.SourceURL == "" && n.Citation == "" {
		return nil
	}
	if !validRef(n.SourceURL) || !validRef(n.Citation) ||
		!strings.HasPrefix(n.SourceURL, "https://") || strings.Contains(n.SourceURL, "#") {
		return fmt.Errorf("bad note source %q %q", n.SourceURL, n.Citation)
	}
	if n.Page < 1 || n.Ref != "" {
		return fmt.Errorf("note %q %q with a source needs a PDF page and no ref", n.Scope, n.Key)
	}
	return nil
}

// validateApplies rejects a malformed applicability range at load time.
func validateApplies(ranges [][2]string) error {
	for _, r := range ranges {
		if len(r[0]) < 2 || len(r[0]) > 6 || len(r[0]) != len(r[1]) || r[0] > r[1] {
			return fmt.Errorf("bad note applicability %v", r)
		}
		for _, c := range r[0] + r[1] {
			if c < '0' || c > '9' {
				return fmt.Errorf("bad note applicability %v", r)
			}
		}
	}
	return nil
}

const caveatPrefix = "Unde Labs note (not treaty text)"

func validWidth(n int) bool { return n == 2 || n == 4 || n == 6 || n == 9 }

// validateSchedule rejects a malformed schedule at load time. Row codes are
// digit prefixes of 2, 4, 6 or 9 digits (nine digits for a national tariff
// item); both ends of a row have equal length unless the row is withheld.
func validateSchedule(s schedule) error {
	if s.Agreement == "" || s.Citation == "" || s.URL == "" {
		return fmt.Errorf("incomplete schedule %q", s.Agreement)
	}
	from, err := time.Parse(time.DateOnly, s.From)
	if err != nil {
		return fmt.Errorf("%s: bad from date %q", s.Agreement, s.From)
	}
	if s.Until != "" {
		until, err := time.Parse(time.DateOnly, s.Until)
		if err != nil || !until.After(from) {
			return fmt.Errorf("%s: bad until date %q", s.Agreement, s.Until)
		}
	}
	if s.HSEdition != 0 && s.HSEdition != 2017 && s.HSEdition != 2022 {
		return fmt.Errorf("%s: unsupported HS edition %d", s.Agreement, s.HSEdition)
	}
	// A schedule locates its rows and notes either by PDF page or, when the
	// source has no pages, by ref. One schedule never mixes the two.
	if s.CaveatFirst {
		has := false
		for _, n := range s.Notes {
			has = has || n.Page == 0
		}
		if !has {
			return fmt.Errorf("%s: caveat_first needs a caveat note", s.Agreement)
		}
	}
	byRef := false
	for _, r := range s.Rows {
		byRef = byRef || r.Ref != ""
	}
	for _, n := range s.Notes {
		byRef = byRef || n.Ref != ""
	}
	if byRef && strings.Contains(s.URL, "#") {
		return fmt.Errorf("%s: a source located by ref must have a URL without a fragment", s.Agreement)
	}
	for _, r := range s.Rows {
		if byRef && (!validRef(r.Ref) || r.Page != 1) {
			return fmt.Errorf("%s: row %q needs a ref and page 1", s.Agreement, r.Start)
		}
		// A withheld row may span unequal widths (a multi-row cell of the source).
		if n, m := len(r.Start), len(r.End); !validWidth(n) || !validWidth(m) || n != m && !r.Blocked || r.Start > r.End {
			return fmt.Errorf("%s: bad row range %q-%q", s.Agreement, r.Start, r.End)
		}
		for _, c := range r.Start + r.End {
			if c < '0' || c > '9' {
				return fmt.Errorf("%s: bad row range %q-%q", s.Agreement, r.Start, r.End)
			}
		}
		if r.Default && r.Blocked {
			return fmt.Errorf("%s: row %q is both default and withheld", s.Agreement, r.Start)
		}
		if r.Page < 1 || strings.TrimSpace(r.Text) == "" {
			return fmt.Errorf("%s: row %q has no page or text", s.Agreement, r.Start)
		}
	}
	for _, n := range s.Notes {
		if n.Scope == "schedule" {
			if n.Key != "" {
				return fmt.Errorf("%s: a schedule note has no key, got %q", s.Agreement, n.Key)
			}
		} else if n.Scope == "section" {
			if _, ok := sections[n.Key]; !ok {
				return fmt.Errorf("%s: unknown section %q", s.Agreement, n.Key)
			}
		} else if ch, err := strconv.Atoi(n.Key); n.Scope != "chapter" || err != nil || len(n.Key) != 2 || ch < 1 {
			return fmt.Errorf("%s: bad note scope %q %q", s.Agreement, n.Scope, n.Key)
		}
		if strings.TrimSpace(n.Text) == "" || n.Page < 0 || n.Page == 0 && !strings.HasPrefix(n.Text, caveatPrefix) {
			return fmt.Errorf("%s: bad note %q %q", s.Agreement, n.Scope, n.Key)
		}
		if byRef && (n.Page == 0) != (n.Ref == "") || byRef && n.Page > 1 {
			return fmt.Errorf("%s: note %q %q needs a ref and page 1 (a caveat has neither)", s.Agreement, n.Scope, n.Key)
		}
		if n.Ref != "" && !validRef(n.Ref) {
			return fmt.Errorf("%s: bad note ref %q", s.Agreement, n.Ref)
		}
		if err := validateApplies(n.Applies); err != nil {
			return fmt.Errorf("%s: %w", s.Agreement, err)
		}
		if err := validateNoteSource(n); err != nil {
			return fmt.Errorf("%s: %w", s.Agreement, err)
		}
	}
	return nil
}

// appliesTo reports whether a code (digits only) lies in the note's ranges. A
// code shorter than a range is included when it could contain codes in it.
func (n note) appliesTo(hs string) bool {
	if len(n.Applies) == 0 {
		return true
	}
	for _, r := range n.Applies {
		w := min(len(hs), len(r[0]))
		if r[0][:w] <= hs[:w] && hs[:w] <= r[1][:w] {
			return true
		}
	}
	return false
}

type schedule struct {
	Agreement string `json:"agreement"`
	Citation  string `json:"citation"`
	URL       string `json:"source_url"`
	SHA256    string `json:"sha256"`
	From      string `json:"from"`
	Until     string `json:"until"`
	// HSEdition is the HS edition (2017 or 2022) whose code list gates the
	// default rows. Zero means 2017, the list every earlier schedule uses.
	HSEdition int `json:"hs_edition,omitempty"`
	// CaveatFirst returns the Unde Labs caveat note before the rows, so a
	// reader sees what was not checked first. The schedule must have one.
	CaveatFirst bool   `json:"caveat_first,omitempty"`
	Rows        []row  `json:"rows"`
	Notes       []note `json:"notes"`
}

// codes returns the code list of the schedule's HS edition.
func (s schedule) codes() *hsCodes {
	if s.HSEdition == 2022 {
		return hs2022Valid()
	}
	return hs2017Valid()
}

// Evidence quotes a rule branch or accompanying note from an official
// schedule. Its applicability still depends on the facts and the treaty. An
// Unde Labs caveat note has an empty URL because it has no source page.
type Evidence struct {
	Text     string
	URL      string
	Citation string
}

// Table holds the embedded treaty schedules and RCEP Annex 3A.
type Table struct {
	schedules map[string][]schedule
	rcep      *rcep.Table
	general   *generalBundle
	corr      *correlationSet
}

// Default opens the embedded schedules. The table can be shared by callers.
var parseDefault = sync.OnceValues(func() (*Table, error) {
	var sources []schedule
	if err := json.Unmarshal(schedulesJSON, &sources); err != nil {
		return nil, fmt.Errorf("japan PSR schedules: %w", err)
	}
	t := &Table{schedules: make(map[string][]schedule), rcep: rcep.Default()}
	for _, s := range sources {
		if err := validateSchedule(s); err != nil {
			return nil, fmt.Errorf("japan PSR schedules: %w", err)
		}
		t.schedules[s.Agreement] = append(t.schedules[s.Agreement], s)
	}
	g, err := parseGeneral(generalJSON, t.schedules)
	if err != nil {
		return nil, err
	}
	t.general = g
	c, err := parseCorrelations(correlationsJSON, t.schedules)
	if err != nil {
		return nil, err
	}
	t.corr = c
	return t, nil
})

// Default returns the embedded legal text table.
func Default() (*Table, error) { return parseDefault() }

// HasAgreement reports whether an agreement has an embedded rule source.
func (t *Table) HasAgreement(agreement string) bool {
	return agreement == "RCEP" || len(t.schedules[agreement]) > 0
}

// Reason says why a detailed lookup returned the evidence it did. Each value
// names a branch the lookup actually took; none claims to know why a code is
// absent from a schedule.
type Reason string

const (
	// ReasonMalformed: the code is not valid HS syntax or the date is zero.
	ReasonMalformed Reason = "malformed"
	// ReasonUnsupportedAgreement: the agreement has no embedded rule source.
	ReasonUnsupportedAgreement Reason = "unsupported_agreement"
	// ReasonNoDatedSource: the agreement is known but no schedule covers the date.
	ReasonNoDatedSource Reason = "no_dated_source"
	// ReasonTooBroad: the code is less precise than a row that overlaps it.
	ReasonTooBroad Reason = "too_broad"
	// ReasonWithheld: the matching row is withheld from the table.
	ReasonWithheld Reason = "withheld"
	// ReasonInvalidCode: the code sits in a reserved HS chapter (00, 77, 98, 99).
	ReasonInvalidCode Reason = "invalid_code"
	// ReasonNotInEdition: in the AJCEP general branch, or under a schedule that
	// has default rows, a code in a non-reserved chapter is absent from the
	// code list of the schedule's HS edition (HS 2017 unless the schedule says
	// HS 2022). It may be a
	// code of a later edition or a typo; the lookup does not say which.
	ReasonNotInEdition Reason = "not_in_schedule_edition"
	// ReasonNoMatch: a schedule covers the date, no row matches, and no
	// general default applies.
	ReasonNoMatch Reason = "no_match"
	// ReasonExcludedGeneral: the general default exists but excludes the
	// code's chapter (50 through 63).
	ReasonExcludedGeneral Reason = "excluded_general"
	// ReasonPSR: product specific rule rows matched.
	ReasonPSR Reason = "psr"
	// ReasonGeneral: the AJCEP general-rule bundle was returned.
	ReasonGeneral Reason = "general"
	// ReasonMapped: the code is absent from the schedule's HS edition, and a
	// reviewed WCO correlation points to parent codes that share one row. The
	// evidence is that row, behind a note stating the mapping.
	ReasonMapped Reason = "mapped"
	// ReasonEditionHint: the code is absent from the schedule's HS edition and
	// a reviewed WCO correlation names parent codes, but no single rule can be
	// shown. Hint says which parents to check.
	ReasonEditionHint Reason = "edition_hint"
)

// Mapped says how a mapped result was reached. It adds to the result and
// never replaces the original code in Result.Digits.
type Mapped struct {
	From             string   // the requested six digit code
	To               []string // the six digit parent codes looked up instead
	RequestedEdition string   // edition label of the requested code
	ScheduleEdition  string   // edition label of the schedule used
	SourceURL        string   // first WCO correlation source
}

// Result is the outcome of a detailed lookup. Evidence is nil unless Reason is
// ReasonPSR, ReasonGeneral or ReasonMapped. Digits is the normalized code (at
// most six digits) the lookup used, empty when the input was malformed. It is
// always the requested code, also for a mapped result. Mapped is set only for
// ReasonMapped; Hint only for ReasonEditionHint.
type Result struct {
	Reason   Reason
	Evidence []Evidence
	Digits   string
	Mapped   *Mapped
	Hint     string
}

// Lookup returns product specific rule text for a code in the tariff
// nomenclature named in the citation. It does not transpose the code from
// other nomenclatures. before is the case date, not the day this selection
// happens. No evidence is returned for an unsupported agreement, date or
// insufficiently precise HS code, or where the row requires material that was
// not embedded. It never returns the AJCEP general-rule bundle; use
// LookupDetailed for that.
func (t *Table) Lookup(agreement, hs string, before time.Time) ([]Evidence, error) {
	r, err := t.lookup(agreement, hs, before, false)
	if err != nil || r.Reason != ReasonPSR {
		return nil, err
	}
	return r.Evidence, nil
}

// LookupDetailed behaves like Lookup for product specific rules and also
// reports why no rule was returned. When the AJCEP HS 2017 schedule covers
// the date and has no row, no withheld row and no overlapping broader row for
// the code, and the code's chapter is valid and outside 50-63, it returns the
// general originating criteria bundle with ReasonGeneral.
//
// When a schedule covers the date and the code is absent from it, a reviewed
// HS edition correlation for that exact agreement and code may turn the
// no_match into ReasonMapped or ReasonEditionHint. No other outcome changes.
func (t *Table) LookupDetailed(agreement, hs string, before time.Time) (Result, error) {
	res, err := t.lookup(agreement, hs, before, true)
	if err != nil {
		return res, err
	}
	return t.correlate(agreement, res, before)
}

var reservedChapters = map[string]bool{"00": true, "77": true, "98": true, "99": true}

func (t *Table) lookup(agreement, hs string, before time.Time, general bool) (Result, error) {
	hs = strings.TrimSpace(hs)
	if !hsPattern.MatchString(hs) || before.IsZero() {
		return Result{Reason: ReasonMalformed}, nil
	}
	digits := strings.NewReplacer(".", "", "-", "").Replace(hs)
	digits = digits[:min(len(digits), 6)]
	if agreement == "RCEP" {
		if before.Format(time.DateOnly) < rcepStart.Format(time.DateOnly) {
			return Result{Reason: ReasonNoDatedSource, Digits: digits}, nil
		}
		if ev := t.lookupRCEP(digits, before); len(ev) > 0 {
			return Result{Reason: ReasonPSR, Evidence: ev, Digits: digits}, nil
		}
		return Result{Reason: ReasonNoMatch, Digits: digits}, nil
	}
	if !t.HasAgreement(agreement) {
		return Result{Reason: ReasonUnsupportedAgreement, Digits: digits}, nil
	}
	date := before.Format(time.DateOnly)
	covered, notInEdition := false, false
	for _, s := range t.schedules[agreement] {
		if date < s.From || s.Until != "" && date >= s.Until {
			continue
		}
		covered = true
		notInEdition = notInEdition || s.hasDefaultRows() && !s.codes().hasPrefix(digits)
		rows, status := s.match(digits)
		switch status {
		case matchTooBroad:
			return Result{Reason: ReasonTooBroad, Digits: digits}, nil
		case matchWithheld:
			return Result{Reason: ReasonWithheld, Digits: digits}, nil
		}
		if len(rows) == 0 {
			continue
		}
		evidence := make([]Evidence, 0, len(rows))
		for _, r := range rows {
			text := r.Text
			if r.Description != "" {
				text = r.Description + ": " + text
			}
			evidence = append(evidence, s.evidence(text, r.Page, r.Ref))
		}
		evidence = s.withNotes(evidence, digits)
		return Result{Reason: ReasonPSR, Evidence: evidence, Digits: digits}, nil
	}
	switch {
	case !covered:
		return Result{Reason: ReasonNoDatedSource, Digits: digits}, nil
	case reservedChapters[digits[:2]]:
		return Result{Reason: ReasonInvalidCode, Digits: digits}, nil
	case notInEdition:
		// Default rows never answer for a code that exists in no edition. A
		// code with a reviewed correlation stays no_match for the correlation step.
		if _, ok := t.corr.find(agreement, digits); ok && len(digits) == 6 {
			return Result{Reason: ReasonNoMatch, Digits: digits}, nil
		}
		return Result{Reason: ReasonNotInEdition, Digits: digits}, nil
	case general && t.general != nil && t.general.applies(agreement, date):
		// The general fallback is for real HS 2017 codes only (the window is
		// the HS 2017 edition). This check comes before the chapter 50-63
		// exclusion, so a non-existent textile code is not_in_schedule_edition. A code
		// with a reviewed correlation is left as no_match so the correlation
		// step can map it or hint at its parents.
		if !hs2017Valid().hasPrefix(digits) {
			if _, ok := t.corr.find(agreement, digits); ok && len(digits) == 6 {
				return Result{Reason: ReasonNoMatch, Digits: digits}, nil
			}
			return Result{Reason: ReasonNotInEdition, Digits: digits}, nil
		}
		if ch, _ := strconv.Atoi(digits[:2]); ch >= 50 && ch <= 63 {
			return Result{Reason: ReasonExcludedGeneral, Digits: digits}, nil
		}
		return Result{Reason: ReasonGeneral, Evidence: t.general.evidence(), Digits: digits}, nil
	}
	return Result{Reason: ReasonNoMatch, Digits: digits}, nil
}

func (t *Table) lookupRCEP(hs string, before time.Time) []Evidence {
	if before.Format(time.DateOnly) < rcepStart.Format(time.DateOnly) {
		return nil
	}
	r, ok, _ := t.rcep.Lookup(hs)
	if !ok {
		return nil
	}
	citation := "RCEP Annex 3A (HS 2022), " + r.Code
	url := rcepAnnexURL
	if len(r.Pages) > 0 {
		url = fmt.Sprintf("%s#page=%d", url, r.Pages[0])
	}
	text := r.Text
	if r.Description != "" {
		text = r.Description + ": " + text
	}
	return []Evidence{{Text: text, URL: url, Citation: citation}}
}

func (s schedule) evidence(text string, page int, ref string) Evidence {
	if ref != "" {
		return Evidence{Text: text, URL: s.URL, Citation: s.Citation + ", " + ref}
	}
	return Evidence{
		Text:     text,
		URL:      fmt.Sprintf("%s#page=%d", s.URL, page),
		Citation: fmt.Sprintf("%s, PDF page %d", s.Citation, page),
	}
}

type matchStatus int

const (
	matchOK matchStatus = iota
	matchTooBroad
	matchWithheld
)

func (s schedule) hasDefaultRows() bool {
	for _, r := range s.Rows {
		if r.Default {
			return true
		}
	}
	return false
}

// match returns the most specific rows covering hs. It reports matchTooBroad
// when a more specific row overlaps the supplied digits and matchWithheld
// when a matching row is withheld.
func (s schedule) match(hs string) (matching []row, status matchStatus) {
	specificity := 0
	inEdition := s.codes().hasPrefix(hs)
	for _, r := range s.Rows {
		if r.Default && !inEdition {
			continue
		}
		width := len(r.Start)
		if width > len(hs) {
			if hs >= r.Start[:len(hs)] && hs <= r.End[:len(hs)] {
				return nil, matchTooBroad
			}
			continue
		}
		q := hs[:width]
		if width < specificity || q < r.Start || q > r.End {
			continue
		}
		if width > specificity {
			matching = matching[:0]
			specificity = width
		}
		matching = append(matching, r)
	}
	for _, r := range matching {
		if r.Blocked {
			return nil, matchWithheld
		}
	}
	return matching, matchOK
}

// Chapter limits of the Harmonized System sections that may carry notes.
var sections = map[string][2]int{
	"I": {1, 5}, "II": {6, 14}, "III": {15, 15}, "IV": {16, 24},
	"V": {25, 27}, "VI": {28, 38}, "VII": {39, 40}, "VIII": {41, 43},
	"IX": {44, 46}, "X": {47, 49}, "XI": {50, 63}, "XII": {64, 67},
	"XIII": {68, 70}, "XIV": {71, 71}, "XV": {72, 83}, "XVI": {84, 85},
	"XVII": {86, 89}, "XVIII": {90, 92}, "XIX": {93, 93}, "XX": {94, 96},
	"XXI": {97, 97},
}

// withNotes appends the notes that apply to the code, or puts the caveat note
// ahead of the rows when the schedule asks for that.
func (s schedule) withNotes(rows []Evidence, hs string) []Evidence {
	notes := s.notesFor(hs)
	if !s.CaveatFirst {
		return append(rows, notes...)
	}
	var out []Evidence
	for _, n := range notes {
		if n.URL == "" && n.Citation == caveatPrefix {
			out = append(out, n)
		}
	}
	out = append(out, rows...)
	for _, n := range notes {
		if n.URL != "" || n.Citation != caveatPrefix {
			out = append(out, n)
		}
	}
	return out
}

func (s schedule) notesFor(hs string) []Evidence {
	chapter, _ := strconv.Atoi(hs[:2])
	var out []Evidence
	for _, n := range s.Notes {
		section := sections[n.Key]
		if (n.Scope == "schedule" ||
			n.Scope == "chapter" && n.Key == hs[:2] ||
			n.Scope == "section" && chapter >= section[0] && chapter <= section[1]) && n.appliesTo(hs) {
			if n.Page == 0 {
				out = append(out, Evidence{Text: n.Text, Citation: caveatPrefix})
				continue
			}
			src := s
			if n.SourceURL != "" {
				src.URL, src.Citation = n.SourceURL, n.Citation
			}
			out = append(out, src.evidence(n.Text, n.Page, n.Ref))
		}
	}
	return out
}

type generalSource struct {
	Name     string `json:"name"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	SHA256   string `json:"sha256"`
	PDFPages int    `json:"pdf_pages"`
}

type generalProvision struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	Citation string `json:"citation"`
	Page     int    `json:"page"`
	Text     string `json:"text"`
	Kind     string `json:"kind"`
}

// generalBundle is the main-agreement text that applies when no product
// specific rule is listed. It is edited treaty text plus clearly marked
// Unde Labs notes.
type generalBundle struct {
	Agreement  string             `json:"agreement"`
	Citation   string             `json:"citation"`
	From       string             `json:"from"`
	Until      string             `json:"until"`
	HSEdition  int                `json:"hs_edition"`
	Retrieved  string             `json:"retrieved"`
	Sources    []generalSource    `json:"sources"`
	Provisions []generalProvision `json:"provisions"`
}

func parseGeneral(data []byte, schedules map[string][]schedule) (*generalBundle, error) {
	var g generalBundle
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("japan general rules: %w", err)
	}
	if g.Agreement == "" || len(g.Provisions) == 0 || g.Until == "" {
		return nil, fmt.Errorf("japan general rules: incomplete bundle")
	}
	// The window must be exactly that of the matching edition's schedule.
	edition := fmt.Sprintf("(HS %d)", g.HSEdition)
	found := false
	for _, s := range schedules[g.Agreement] {
		if strings.Contains(s.Citation, edition) && s.From == g.From && s.Until == g.Until {
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("japan general rules: window %s to %s matches no %s %s schedule", g.From, g.Until, g.Agreement, edition)
	}
	seen := map[string]bool{}
	for _, p := range g.Provisions {
		if p.ID == "" || seen[p.ID] || p.Text == "" || p.Citation == "" {
			return nil, fmt.Errorf("japan general rules: invalid provision %q", p.ID)
		}
		switch src := g.source(p.Source); p.Kind {
		case "treaty":
			if src == nil || p.Page < 1 || p.Page > src.PDFPages {
				return nil, fmt.Errorf("japan general rules: invalid provision %q", p.ID)
			}
		case "note": // a note is not treaty text and has no source page
			if p.Source != "" || p.Page != 0 {
				return nil, fmt.Errorf("japan general rules: invalid provision %q", p.ID)
			}
		default:
			return nil, fmt.Errorf("japan general rules: invalid provision %q", p.ID)
		}
		seen[p.ID] = true
	}
	return &g, nil
}

func (g *generalBundle) source(name string) *generalSource {
	for i := range g.Sources {
		if g.Sources[i].Name == name {
			return &g.Sources[i]
		}
	}
	return nil
}

func (g *generalBundle) applies(agreement, date string) bool {
	return agreement == g.Agreement && date >= g.From && date < g.Until
}

func (g *generalBundle) evidence() []Evidence {
	out := make([]Evidence, 0, len(g.Provisions))
	for _, p := range g.Provisions {
		if p.Kind == "note" {
			out = append(out, Evidence{Text: p.Text, Citation: p.Citation})
			continue
		}
		out = append(out, Evidence{
			Text:     p.Text,
			URL:      fmt.Sprintf("%s#page=%d", g.source(p.Source).URL, p.Page),
			Citation: fmt.Sprintf("%s, PDF page %d", p.Citation, p.Page),
		})
	}
	return out
}
