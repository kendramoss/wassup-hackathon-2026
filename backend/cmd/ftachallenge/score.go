package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/aseanfta"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/atiga"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/rcep"
)

// Actual is what an engine returned, reduced to the fields cases can check.
type Actual struct {
	Outcome       string            `json:"outcome,omitempty"`
	Applied       string            `json:"applied,omitempty"`
	Alternative   map[string]string `json:"alternative,omitempty"`
	Accumulation  bool              `json:"accumulation"`
	DeMinimisUsed bool              `json:"de_minimis_used"`
	RVCDown       *float64          `json:"rvc_down,omitempty"`
	RVCUp         *float64          `json:"rvc_up,omitempty"`
	Refused       bool              `json:"refused,omitempty"`
	Error         string            `json:"error,omitempty"`
}

// Result is the retained record of one scored case.
type Result struct {
	ID                 string   `json:"id"`
	Split              string   `json:"split"`
	Feature            string   `json:"feature"`
	ScenarioFamily     string   `json:"scenario_family"`
	SourceFamilyID     string   `json:"source_family_id"`
	ParentID           string   `json:"parent_id,omitempty"`
	Status             string   `json:"status"`
	ExpectedAbstention bool     `json:"expected_abstention"`
	Expected           Expect   `json:"expected"`
	Actual             Actual   `json:"actual"`
	Failures           []string `json:"failures"`
}

const (
	Matched    = "matched"
	Mismatched = "mismatched"
	Errored    = "error"
)

func strictDecode(raw json.RawMessage, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func alternatives(rows []atiga.AltResult) map[string]string {
	m := map[string]string{}
	for _, a := range rows {
		m[a.Rule] = a.Outcome
	}
	return m
}

// callEngine runs the production check for a feature.
func callEngine(feature string, raw json.RawMessage) (Actual, error) {
	switch feature {
	case "atiga", "akfta", "acfta":
		var in atiga.Input
		if err := strictDecode(raw, &in); err != nil {
			return Actual{}, fmt.Errorf("decode input: %w", err)
		}
		var res atiga.Result
		var err error
		switch feature {
		case "atiga":
			res, err = atiga.Check(in)
		case "akfta":
			res, err = aseanfta.Check(aseanfta.AKFTA, in)
		default:
			res, err = aseanfta.Check(aseanfta.ACFTA, in)
		}
		if err != nil {
			a := Actual{Error: err.Error()}
			a.Refused = errors.Is(err, atiga.ErrInput)
			return a, err
		}
		a := Actual{Outcome: res.Outcome, Applied: res.Applied, Alternative: alternatives(res.Alternatives), Accumulation: res.Accumulation}
		if res.RVC != nil {
			a.RVCDown, a.RVCUp = res.RVC.Indirect, res.RVC.Direct
		}
		a.DeMinimisUsed = res.DeMinimis != nil && res.DeMinimis.Used
		return a, nil
	case "rcep":
		var in rcep.Input
		if err := strictDecode(raw, &in); err != nil {
			return Actual{}, fmt.Errorf("decode input: %w", err)
		}
		res, err := rcep.Check(in)
		if err != nil {
			a := Actual{Error: err.Error()}
			a.Refused = errors.Is(err, rcep.ErrInput)
			return a, err
		}
		alts := map[string]string{}
		for _, r := range res.Alternatives {
			alts[r.Rule] = r.Outcome
		}
		a := Actual{Outcome: res.Outcome, Applied: res.Applied, Alternative: alts, Accumulation: res.Accumulation}
		if res.RVC != nil {
			a.RVCDown, a.RVCUp = res.RVC.BuildDown, res.RVC.BuildUp
		}
		a.DeMinimisUsed = res.DeMinimis != nil && res.DeMinimis.Used
		return a, nil
	}
	return Actual{}, fmt.Errorf("unknown feature %q", feature)
}

func within(v *float64, lo, hi *float64) bool {
	if lo == nil && hi == nil {
		return true
	}
	if v == nil || math.IsNaN(*v) {
		return false
	}
	return (lo == nil || *v >= *lo) && (hi == nil || *v <= *hi)
}

// Compare lists every way the actual answer departs from the expected one.
func Compare(e Expect, a Actual) []string {
	var bad []string
	add := func(format string, args ...any) { bad = append(bad, fmt.Sprintf(format, args...)) }
	if a.Outcome != e.Outcome {
		add("outcome: got %s, want %s", a.Outcome, e.Outcome)
	}
	if e.Applied != "" && a.Applied != e.Applied {
		add("applied: got %q, want %q", a.Applied, e.Applied)
	}
	rules := make([]string, 0, len(e.Alternative))
	for r := range e.Alternative {
		rules = append(rules, r)
	}
	sort.Strings(rules)
	for _, r := range rules {
		if got := a.Alternative[r]; got != e.Alternative[r] {
			add("alternative %s: got %q, want %q", r, got, e.Alternative[r])
		}
	}
	if e.Accumulation != nil && a.Accumulation != *e.Accumulation {
		add("accumulation: got %v, want %v", a.Accumulation, *e.Accumulation)
	}
	if e.DeMinimisUsed != nil && a.DeMinimisUsed != *e.DeMinimisUsed {
		add("de_minimis_used: got %v, want %v", a.DeMinimisUsed, *e.DeMinimisUsed)
	}
	if !within(a.RVCDown, e.RVCDownMin, e.RVCDownMax) {
		add("rvc build-down outside the expected range")
	}
	if !within(a.RVCUp, e.RVCUpMin, e.RVCUpMax) {
		add("rvc build-up outside the expected range")
	}
	return bad
}

// ScoreCase runs one case. Engine failures other than an expected refusal
// are errors, never silent passes.
func ScoreCase(c Case) Result {
	r := Result{ID: c.ID, Split: c.Split, Feature: c.Feature, ScenarioFamily: c.ScenarioFamily, SourceFamilyID: c.SourceFamilyID,
		ParentID: c.ParentID, ExpectedAbstention: c.ExpectedAbstention, Expected: c.Expect, Failures: []string{}}
	act, err := callEngine(c.Feature, c.Input)
	r.Actual = act
	switch {
	case c.Expect.Outcome == "refused":
		if err != nil && act.Refused {
			r.Status = Matched
		} else if err != nil {
			r.Status = Errored
			r.Failures = append(r.Failures, "expected a refusal of invalid input, got a different error: "+err.Error())
		} else {
			r.Status = Mismatched
			r.Failures = append(r.Failures, "expected a refusal, got outcome "+act.Outcome)
		}
	case err != nil:
		r.Status = Errored
		r.Failures = append(r.Failures, "engine error: "+err.Error())
	default:
		r.Failures = append(r.Failures, Compare(c.Expect, act)...)
		if len(r.Failures) == 0 {
			r.Status = Matched
		} else {
			r.Status = Mismatched
		}
	}
	return r
}

// Count is a numerator over a denominator that keeps errors in it.
type Count struct {
	Num int `json:"num"`
	Den int `json:"den"`
}

type Totals struct {
	Cases      int `json:"cases"`
	Matched    int `json:"matched"`
	Mismatched int `json:"mismatched"`
	Errors     int `json:"errors"`
}

type FamilyTotals struct {
	Totals
	Parent bool `json:"has_parent_cases"`
}

type Abstention struct {
	Expected          int `json:"expected"`
	CorrectlyAbstains int `json:"correct"`
	Decided           int `json:"decided_instead"`
	Missed            int `json:"abstained_when_a_decision_was_expected"`
}

type Metrics struct {
	Expected           map[string]int            `json:"expected_outcomes"`
	Confusion          map[string]map[string]int `json:"confusion"`
	Abstention         Abstention                `json:"abstention"`
	YesPrecision       Count                     `json:"yes_precision"`
	YesRecall          Count                     `json:"yes_recall"`
	DecidedAgreement   Count                     `json:"decided_only_agreement"`
	RefusalsExpected   int                       `json:"refusals_expected"`
	FormattingOnly     int                       `json:"formatting_only_cases"`
	ByFamily           map[string]Totals         `json:"by_scenario_family"`
	ScenarioFamilies   int                       `json:"scenario_families"`
	SourceFamilies     int                       `json:"source_families"`
	ParentGroups       int                       `json:"parent_groups"`
	CasesWithParent    int                       `json:"cases_with_parent"`
	SharedFamilies     []string                  `json:"families_shared_with_development,omitempty"`
	TestOnlyFamilies   []string                  `json:"families_only_in_test,omitempty"`
	SharedFamilyCases  int                       `json:"cases_in_families_shared_with_development,omitempty"`
	TestOnlyFamilyCase int                       `json:"cases_in_families_only_in_test,omitempty"`
}

func Tally(rs []Result) Totals {
	var t Totals
	for _, r := range rs {
		t.Cases++
		switch r.Status {
		case Matched:
			t.Matched++
		case Mismatched:
			t.Mismatched++
		default:
			t.Errors++
		}
	}
	return t
}

func ComputeMetrics(ds *Dataset, rs []Result, other *Dataset) Metrics {
	m := Metrics{Expected: map[string]int{}, Confusion: map[string]map[string]int{}, ByFamily: map[string]Totals{}}
	byFam := map[string][]Result{}
	for i, r := range rs {
		c := ds.Cases[i]
		byFam[r.ScenarioFamily] = append(byFam[r.ScenarioFamily], r)
		e := c.Expect.Outcome
		m.Expected[e]++
		got := r.Actual.Outcome
		if r.Status == Errored {
			got = "error"
		}
		if r.Actual.Refused && r.Status == Matched {
			got = "refused"
		}
		if m.Confusion[e] == nil {
			m.Confusion[e] = map[string]int{}
		}
		m.Confusion[e][got]++
		if c.FormattingOnly {
			m.FormattingOnly++
		}
		if c.ParentID != "" {
			m.CasesWithParent++
		}
		switch {
		case e == "refused":
			m.RefusalsExpected++
		case e == "undetermined":
			m.Abstention.Expected++
			if got == "undetermined" {
				m.Abstention.CorrectlyAbstains++
			} else {
				m.Abstention.Decided++
			}
		case got == "undetermined":
			m.Abstention.Missed++
		}
		if e != "refused" {
			if got == "yes" {
				m.YesPrecision.Den++
				if e == "yes" {
					m.YesPrecision.Num++
				}
			}
			if e == "yes" {
				m.YesRecall.Den++
				if got == "yes" {
					m.YesRecall.Num++
				}
			}
			if (e == "yes" || e == "no") && (got == "yes" || got == "no") {
				m.DecidedAgreement.Den++
				if e == got {
					m.DecidedAgreement.Num++
				}
			}
		}
	}
	for f, rows := range byFam {
		m.ByFamily[f] = Tally(rows)
	}
	m.ScenarioFamilies = len(byFam)
	m.SourceFamilies = distinct(ds.Cases, func(c Case) string { return c.SourceFamilyID })
	m.ParentGroups = distinct(ds.Cases, func(c Case) string { return c.ParentID })
	if other != nil {
		shared, only := FamilyOverlap(other, ds)
		m.SharedFamilies, m.TestOnlyFamilies = shared, only
		for _, c := range ds.Cases {
			for _, f := range only {
				if c.ScenarioFamily == f {
					m.TestOnlyFamilyCase++
				}
			}
		}
		m.SharedFamilyCases = len(ds.Cases) - m.TestOnlyFamilyCase
	}
	return m
}

// ScoreDataset scores every case of a dataset in file order.
func ScoreDataset(ds *Dataset) []Result {
	out := make([]Result, 0, len(ds.Cases))
	for _, c := range ds.Cases {
		out = append(out, ScoreCase(c))
	}
	return out
}
