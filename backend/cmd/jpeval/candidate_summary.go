package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpengine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llmbudget"
)

type candidateMetrics struct {
	Run               string `json:"run"`
	Split             string `json:"split"`
	Expected          int    `json:"expected_cases"`
	Cases             int    `json:"cases"`
	Errors            int    `json:"errors"`
	ModelCalls        int    `json:"model_calls"`
	Qualifies         int    `json:"qualifies"`
	DoesNotQualify    int    `json:"does_not_qualify"`
	Undetermined      int    `json:"undetermined"`
	Criterion         Rate   `json:"criterion_match"`
	CriterionAny      Rate   `json:"criterion_in_cited_set"`
	NamedCountry      Rate   `json:"stated_country_match"`
	NamedOrigin       Rate   `json:"stated_origin_match_including_eu"`
	PublishedPositive Rate   `json:"positive_recall"`
	SyntheticRefusals Rate   `json:"synthetic_refusals"`
	// NegativesNotApproved (dev_neg, D913): negatives answered anything but
	// "qualifies". A wrong approval is the failure this split measures.
	NegativesNotApproved *Rate           `json:"negatives_not_approved,omitempty"`
	ByAgreement          map[string]Rate `json:"criterion_match_by_agreement"`
	CostUSD              float64         `json:"cost_usd"`
	// A call with no reported cost is counted here and charged to the budget,
	// never presented as free. Both are absent from runs before the guard.
	UnreportedCostCalls int     `json:"calls_without_reported_cost,omitempty"`
	BudgetChargedUSD    float64 `json:"budget_charged_usd,omitempty"`
	BudgetOverruns      int     `json:"budget_overruns,omitempty"`
	CostPerCase         float64 `json:"cost_per_case_usd"`
	CostPerModelCall    float64 `json:"cost_per_model_call_usd"`
	P50MS               int64   `json:"model_call_cases_latency_p50_ms"`
	P95MS               int64   `json:"model_call_cases_latency_p95_ms"`
	LabelsSHA256        string  `json:"labels_sha256,omitempty"`
}

func scoreCandidate(name, split string, rows []candidateRow, expected int, labelHash string) candidateMetrics {
	s := candidateMetrics{Run: name, Split: split, Cases: len(rows), Expected: expected, LabelsSHA256: labelHash, ByAgreement: map[string]Rate{}}
	var calls []int64
	var criterion, cited, country, origin, positive, synthetic int
	var nc, ncountry, no, np, ns, nn, notApproved int
	agN, agK := map[string]int{}, map[string]int{}
	for _, row := range rows {
		s.CostUSD += row.CostUSD
		s.BudgetChargedUSD += row.BudgetUSD
		if row.BudgetOverrun {
			s.BudgetOverruns++
		}
		if row.CalledModel && row.CostProvenance != "" && row.CostProvenance != llmbudget.Reported {
			s.UnreportedCostCalls++
		}
		if row.CalledModel {
			s.ModelCalls++
			calls = append(calls, row.LatencyMS)
		}
		outcome, decidedCriterion := "", ""
		switch {
		case row.Error != "" || row.Prediction.Japan == nil:
			s.Errors++
		default:
			p := row.Prediction.Japan
			outcome, decidedCriterion = p.Outcome, p.Criterion
			switch outcome {
			case jpengine.Qualifies:
				s.Qualifies++
			case jpengine.DoesNotQualify:
				s.DoesNotQualify++
			case jpengine.Undetermined:
				s.Undetermined++
			}
		}
		l := row.Label
		if l.Criterion != "" {
			nc++
			agN[l.Agreement]++
			if decidedCriterion == l.Criterion {
				criterion++
				agK[l.Agreement]++
			}
			for _, c := range l.Criteria {
				if c == decidedCriterion {
					cited++
					break
				}
			}
		}
		if l.StatedOrigin != "" {
			no++
			isCountry := !strings.EqualFold(l.StatedOrigin, "EU")
			if isCountry {
				ncountry++
			}
			if outcome == jpengine.Qualifies && domain.NormalizeCountry(l.StatedOrigin) == domain.NormalizeCountry(row.Prediction.Origin) {
				origin++
				if isCountry {
					country++
				}
			}
		}
		if l.Outcome == "positive" {
			np++
			if outcome == jpengine.Qualifies {
				positive++
			}
		}
		if split == "dev_neg" && l.Outcome == "negative" {
			nn++
			if outcome != jpengine.Qualifies {
				notApproved++
			}
		}
		if split == "synthetic" && l.Outcome == "negative" {
			ns++
			if outcome == jpengine.DoesNotQualify {
				synthetic++
			}
		}
	}
	s.Criterion, s.CriterionAny = rate(criterion, nc), rate(cited, nc)
	s.NamedCountry, s.NamedOrigin = rate(country, ncountry), rate(origin, no)
	s.PublishedPositive, s.SyntheticRefusals = rate(positive, np), rate(synthetic, ns)
	if split == "dev_neg" {
		r := rate(notApproved, nn)
		s.NegativesNotApproved = &r
	}
	for a, n := range agN {
		s.ByAgreement[a] = rate(agK[a], n)
	}
	if len(rows) > 0 {
		s.CostPerCase = round4(s.CostUSD / float64(len(rows)))
	}
	if s.ModelCalls > 0 {
		s.CostPerModelCall = round4(s.CostUSD / float64(s.ModelCalls))
	}
	s.CostUSD = round4(s.CostUSD)
	s.BudgetChargedUSD = round4(s.BudgetChargedUSD)
	s.P50MS, s.P95MS = evalkit.Percentile(calls, 50), evalkit.Percentile(calls, 95)
	return s
}

func candidateSummary(args []string) error {
	fs := flag.NewFlagSet("candidate-summary", flag.ExitOnError)
	split := fs.String("split", "dev_eval", "dev_eval, dev_neg, held_out, or synthetic")
	pv := fs.String("prompt-version", jpengine.PromptVersion, "prompt version in the run's file name, for runs of an earlier prompt")
	tag := fs.String("tag", "", "tag for a development run")
	id := fs.String("run-id", "", "run id of a held_out or synthetic run")
	check := fs.Bool("check", false, "fail if the committed metrics differ")
	_ = fs.Parse(args)
	runPromptVersion = *pv
	name, err := candidateName(*split, *tag, *id)
	if err != nil {
		return err
	}
	rows, err := readCandidateRows(candidateRowsPath(name))
	if err != nil {
		return err
	}
	items, _, labelHash, err := loadCandidate(*split)
	if err != nil {
		return err
	}
	rowsByID := map[string]int{}
	for i, r := range rows {
		rowsByID[r.ID] = i + 1
	}
	for _, item := range items {
		if i := rowsByID[item.label.ID]; i > 0 {
			rows[i-1].Label = item.label
			delete(rowsByID, item.label.ID)
		}
	}
	if len(rowsByID) > 0 {
		return fmt.Errorf("run contains cases outside the %s split", *split)
	}
	return writeCandidateSummary(scoreCandidate(name, *split, rows, len(items), labelHash), *check, false)
}

// writeCandidateSummary checks or writes the metrics file. A fresh run
// creates it exclusively; regenerating from saved rows rewrites it.
func writeCandidateSummary(s candidateMetrics, check, exclusive bool) error {
	b, err := json.MarshalIndent(s, "", " ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	path := candidateMetricsPath(s.Run)
	if check {
		got, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(got, b) {
			return fmt.Errorf("metrics differ from candidate rows: %s", path)
		}
		fmt.Println("jp candidate metrics ok", s.Run)
		return nil
	}
	if exclusive {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("metrics file %s cannot be created exclusively", filepath.Base(path))
		}
		_, werr := f.Write(b)
		return errors.Join(werr, f.Close())
	}
	return os.WriteFile(path, b, 0o644)
}
