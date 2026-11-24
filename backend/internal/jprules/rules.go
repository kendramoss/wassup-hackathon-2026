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
}

type note struct {
	Scope string `json:"scope"`
	Key   string `json:"key"`
	Text  string `json:"text"`
	Page  int    `json:"page"`
}

type schedule struct {
	Agreement string `json:"agreement"`
	Citation  string `json:"citation"`
	URL       string `json:"source_url"`
	From      string `json:"from"`
	Until     string `json:"until"`
	Rows      []row  `json:"rows"`
	Notes     []note `json:"notes"`
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
}

// Default opens the embedded schedules. The table can be shared by callers.
var parseDefault = sync.OnceValues(func() (*Table, error) {
	var sources []schedule
	if err := json.Unmarshal(schedulesJSON, &sources); err != nil {
		return nil, fmt.Errorf("japan PSR schedules: %w", err)
	}
	t := &Table{schedules: make(map[string][]schedule), rcep: rcep.Default()}
	for _, s := range sources {
		t.schedules[s.Agreement] = append(t.schedules[s.Agreement], s)
	}
	g, err := parseGeneral(generalJSON, t.schedules)
	if err != nil {
		return nil, err
	}
	t.general = g
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
	// ReasonInvalidCode: the code sits in a reserved or invalid HS chapter.
	ReasonInvalidCode Reason = "invalid_code"
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
)

// Result is the outcome of a detailed lookup. Evidence is nil unless Reason is
// ReasonPSR or ReasonGeneral. Digits is the normalized code (at most six
// digits) the lookup used, empty when the input was malformed.
type Result struct {
	Reason   Reason
	Evidence []Evidence
	Digits   string
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
func (t *Table) LookupDetailed(agreement, hs string, before time.Time) (Result, error) {
	return t.lookup(agreement, hs, before, true)
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
	covered := false
	for _, s := range t.schedules[agreement] {
		if date < s.From || s.Until != "" && date >= s.Until {
			continue
		}
		covered = true
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
			evidence = append(evidence, s.evidence(text, r.Page))
		}
		evidence = append(evidence, s.notesFor(digits)...)
		return Result{Reason: ReasonPSR, Evidence: evidence, Digits: digits}, nil
	}
	switch {
	case !covered:
		return Result{Reason: ReasonNoDatedSource, Digits: digits}, nil
	case reservedChapters[digits[:2]]:
		return Result{Reason: ReasonInvalidCode, Digits: digits}, nil
	case general && t.general != nil && t.general.applies(agreement, date):
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

func (s schedule) evidence(text string, page int) Evidence {
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

// match returns the most specific rows covering hs. It reports matchTooBroad
// when a more specific row overlaps the supplied digits and matchWithheld
// when a matching row is withheld.
func (s schedule) match(hs string) (matching []row, status matchStatus) {
	specificity := 0
	for _, r := range s.Rows {
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

func (s schedule) notesFor(hs string) []Evidence {
	chapter, _ := strconv.Atoi(hs[:2])
	var out []Evidence
	for _, n := range s.Notes {
		section := sections[n.Key]
		if n.Scope == "chapter" && n.Key == hs[:2] ||
			n.Scope == "section" && chapter >= section[0] && chapter <= section[1] {
			out = append(out, s.evidence(n.Text, n.Page))
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
