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
// schedule. Its applicability still depends on the facts and the treaty.
type Evidence struct {
	Text     string
	URL      string
	Citation string
}

// Table holds the embedded treaty schedules and RCEP Annex 3A.
type Table struct {
	schedules map[string][]schedule
	rcep      *rcep.Table
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
	return t, nil
})

// Default returns the embedded legal text table.
func Default() (*Table, error) { return parseDefault() }

// HasAgreement reports whether an agreement has an embedded rule source.
func (t *Table) HasAgreement(agreement string) bool {
	return agreement == "RCEP" || len(t.schedules[agreement]) > 0
}

// Lookup returns text for a code in the tariff nomenclature named in the
// citation. It does not transpose the code from other nomenclatures. before
// is the case date, not the day this selection happens. No evidence is
// returned for an unsupported agreement, date or insufficiently precise HS
// code, or where the row requires material that was not embedded.
func (t *Table) Lookup(agreement, hs string, before time.Time) ([]Evidence, error) {
	hs = strings.TrimSpace(hs)
	if !hsPattern.MatchString(hs) || before.IsZero() {
		return nil, nil
	}
	digits := strings.NewReplacer(".", "", "-", "").Replace(hs)
	digits = digits[:min(len(digits), 6)]
	if agreement == "RCEP" {
		return t.lookupRCEP(digits, before), nil
	}
	date := before.Format(time.DateOnly)
	for _, s := range t.schedules[agreement] {
		if date < s.From || s.Until != "" && date >= s.Until {
			continue
		}
		rows, blocked := s.match(digits)
		if blocked {
			return nil, nil
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
		return evidence, nil
	}
	return nil, nil
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

func (s schedule) match(hs string) (matching []row, blocked bool) {
	specificity := 0
	for _, r := range s.Rows {
		width := len(r.Start)
		if width > len(hs) {
			if hs >= r.Start[:len(hs)] && hs <= r.End[:len(hs)] {
				return nil, true
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
			return nil, true
		}
	}
	return matching, false
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
