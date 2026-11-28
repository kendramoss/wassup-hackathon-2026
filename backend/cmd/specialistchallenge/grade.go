package main

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

func confident(r Row) bool {
	if r.Route != "" {
		return r.Route == "auto"
	}
	return r.Confidence >= ConfidentBar
}

// Grade scores one row against the case's label. The label is written before
// any model runs; the grader shares no code with the candidates.
func Grade(c Case, r Row) Row {
	r.ID, r.Feature, r.Split = c.ID, c.Feature, c.Split
	if r.Error != "" {
		r.Verdict, r.Class = ErrorV, "error"
		return r
	}
	if c.Feature == "japan_engine" {
		return gradeJapan(c, r)
	}
	if c.Expected.Kind == "uncertain" {
		if confident(r) {
			r.Verdict, r.Class = Mismatch, "overconfident"
		} else {
			r.Verdict, r.Class = Match, "abstain_correct"
		}
		return r
	}
	agree := r.Origin != "" && domain.NormalizeCountry(r.Origin) == domain.NormalizeCountry(c.Expected.Origin)
	switch {
	case agree && confident(r):
		r.Verdict, r.Class = Match, "agree_confident"
	case agree:
		r.Verdict, r.Class = Match, "agree_review"
	case confident(r):
		r.Verdict, r.Class = Mismatch, "wrong_confident"
	default:
		r.Verdict, r.Class = Mismatch, "wrong_review"
	}
	return r
}

func gradeJapan(c Case, r Row) Row {
	e := c.Expected
	if e.Kind == "uncertain" {
		if r.Outcome == "undetermined" {
			r.Verdict, r.Class = Match, "abstain_correct"
		} else {
			r.Verdict, r.Class = Mismatch, "decided_on_uncertain"
		}
		return r
	}
	switch {
	case r.Outcome == e.Outcome:
		r.Verdict, r.Class = Match, "agree"
		if e.Outcome == "qualifies" {
			ok := slices.Contains(e.Criteria, r.Criterion)
			r.CriterionOK = &ok
		}
	case r.Outcome == "undetermined":
		r.Verdict, r.Class = Mismatch, "abstained_on_decided"
	default:
		r.Verdict, r.Class = Mismatch, "wrong_decision"
	}
	return r
}

// Complete requires exactly one row per case, none extra, matching feature
// and split.
func Complete(cases []Case, rows []Row) error {
	want := map[string]Case{}
	for _, c := range cases {
		want[c.ID] = c
	}
	got := map[string]bool{}
	var problems []string
	for _, r := range rows {
		c, ok := want[r.ID]
		switch {
		case !ok:
			problems = append(problems, "row for unknown case "+r.ID)
		case got[r.ID]:
			problems = append(problems, "duplicate row for "+r.ID)
		case r.Split != c.Split || r.Feature != c.Feature:
			problems = append(problems, "row "+r.ID+" has the wrong feature or split")
		}
		got[r.ID] = true
	}
	for id := range want {
		if !got[id] {
			problems = append(problems, "no row for "+id)
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("incomplete scoring: %s", strings.Join(problems, "; "))
	}
	return nil
}

// SummarizeSplit recomputes every count from the rows. Errors stay in the
// denominator; decided and uncertain cases keep their own denominators.
func SummarizeSplit(cases []Case, rows []Row) (SplitSummary, error) {
	var s SplitSummary
	if err := Complete(cases, rows); err != nil {
		return s, err
	}
	byID := map[string]Row{}
	for _, r := range rows {
		byID[r.ID] = r
	}
	for _, c := range cases {
		r := byID[c.ID]
		s.Cases++
		if r.CalledModel {
			s.ModelCalls++
		} else if r.Error == "" {
			s.Preflight++
		}
		switch r.Verdict {
		case Match:
			s.Matched++
		case Mismatch:
			s.Mismatched++
		default:
			s.Errors++
		}
		if r.CriterionOK != nil {
			if *r.CriterionOK {
				s.CriterionValid++
			} else {
				s.CriterionInvalid++
			}
		}
		if c.Expected.Kind == "uncertain" {
			s.Uncertain.Cases++
			switch r.Class {
			case "abstain_correct":
				s.Uncertain.AbstainedCorrectly++
			case "error":
				s.Uncertain.Errors++
			default:
				s.Uncertain.Overconfident++
			}
			continue
		}
		d := &s.Decided
		d.Cases++
		switch r.Class {
		case "agree_confident":
			d.Agreed++
			d.Confident++
			d.ConfidentCorrect++
		case "agree_review", "agree":
			d.Agreed++
		case "wrong_confident":
			d.Wrong++
			d.Confident++
			d.ConfidentWrong++
		case "abstained_on_decided":
			d.Abstained++
		case "error":
			d.Errors++
		default:
			d.Wrong++
		}
	}
	return s, nil
}

// Gate refuses rows that could not have come from the approved gateway: a
// fake or Vertex provider, a negative cost, or an answer with no model call.
func Gate(rows []Row) error {
	for _, r := range rows {
		low := strings.ToLower(r.Provider + " " + r.ModelID)
		switch {
		case strings.Contains(low, "vertex") || strings.Contains(low, "fake"):
			return fmt.Errorf("row %s: provider %q model %q is not the approved gateway", r.ID, r.Provider, r.ModelID)
		case r.CostUSD < 0:
			return fmt.Errorf("row %s: negative cost", r.ID)
		case r.Error != "":
			continue
		case r.CalledModel && r.Provider != "litellm":
			return fmt.Errorf("row %s: provider %q is not litellm", r.ID, r.Provider)
		case !r.CalledModel && (r.Origin != "" || r.Provider != "" || r.Confidence != 0):
			return fmt.Errorf("row %s carries a model answer but records no model call", r.ID)
		}
	}
	return nil
}

// Percentile is the nearest-rank percentile (p in 0..1); 0 for no values.
func Percentile(v []int64, p float64) int64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]int64(nil), v...)
	slices.Sort(s)
	i := int(math.Ceil(p*float64(len(s)))) - 1
	if i < 0 {
		i = 0
	}
	return s[i]
}
