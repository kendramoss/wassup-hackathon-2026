package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/rcep"
)

type share struct {
	Count   int      `json:"count"`
	Total   int      `json:"total"`
	Rate    *float64 `json:"rate,omitempty"`
	Lower95 *float64 `json:"lower95,omitempty"`
	Upper95 *float64 `json:"upper95,omitempty"`
}

func proportion(k, n int) share {
	s := share{Count: k, Total: n}
	if n > 0 {
		p, low, high := evalkit.Wilson(k, n)
		s.Rate, s.Lower95, s.Upper95 = &p, &low, &high
	}
	return s
}

type scopeResult struct {
	Outcomes                   map[string]share `json:"outcomes"`
	DecidingCriterionAgreement share            `json:"deciding_criterion_agreement"`
}

type breakdown struct {
	Published                    scopeResult      `json:"published_positives"`
	TariffShiftWithMaterialCodes scopeResult      `json:"published_tariff_shift_with_material_codes"`
	RVC                          scopeResult      `json:"published_rvc"`
	ExplicitAnnexRowAgreement    share            `json:"explicit_annex_row_agreement"`
	CitedTariffScopeCompatible   share            `json:"cited_tariff_scope_compatible"`
	Synthetic                    map[string]share `json:"synthetic_negatives"`
}

func outcomes(counts map[string]int, total int, synthetic bool) map[string]share {
	out := map[string]share{}
	for _, status := range []string{rcep.Yes, rcep.No, rcep.Undetermined, "error"} {
		out[status] = proportion(counts[status], total)
	}
	if synthetic {
		out["no_or_undetermined"] = proportion(counts[rcep.No]+counts[rcep.Undetermined], total)
	}
	return out
}

func collect(rows []row) scopeResult {
	counts := map[string]int{}
	decided, agree := 0, 0
	for _, r := range rows {
		counts[r.Result.Status]++
		if r.Criterion != "" && r.Result.Criterion != "" {
			decided++
			if r.Criterion == r.Result.Criterion {
				agree++
			}
		}
	}
	return scopeResult{Outcomes: outcomes(counts, len(rows), false), DecidingCriterionAgreement: proportion(agree, decided)}
}

func hasCodesForShift(r row) bool {
	want := map[string]int{"CC": 2, "CTH": 4, "CTSH": 6}[r.Criterion]
	if want == 0 || len(r.Input.Materials) == 0 {
		return false
	}
	return !slices.ContainsFunc(r.Input.Materials, func(m rcep.Material) bool {
		return m.Status != rcep.StatusOriginating && len(m.HS) < want
	})
}

func breakdownOf(rows []row) breakdown {
	var tariffs, rvc []row
	var hasExact, exact, hasScope, compatible, synth int
	synthetic := map[string]int{}
	for _, r := range rows {
		if hasCodesForShift(r) {
			tariffs = append(tariffs, r)
		}
		if r.Criterion == "RVC" {
			rvc = append(rvc, r)
		}
		if r.PublishedRow != "" && r.Result.Rule != "" {
			hasExact++
			if r.PublishedRow == r.Result.Rule {
				exact++
			}
		}
		if r.PublishedScope != "" && r.Result.Rule != "" {
			hasScope++
			if strings.HasPrefix(r.PublishedScope, r.Result.Rule) || strings.HasPrefix(r.Result.Rule, r.PublishedScope) {
				compatible++
			}
		}
		if r.Synthetic != nil {
			synth++
			synthetic[r.Synthetic.Result.Status]++
		}
	}
	return breakdown{
		Published: collect(rows), TariffShiftWithMaterialCodes: collect(tariffs), RVC: collect(rvc),
		ExplicitAnnexRowAgreement:  proportion(exact, hasExact),
		CitedTariffScopeCompatible: proportion(compatible, hasScope),
		Synthetic:                  outcomes(synthetic, synth, true),
	}
}

func summarize(check bool) error {
	summary := struct {
		Table   string    `json:"table_version"`
		Dev     breakdown `json:"development"`
		Heldout breakdown `json:"held_out"`
		Overall breakdown `json:"overall"`
	}{Table: rcep.Default().Version}
	dev, err := readLines[row](filepath.Join(reportDir, "japan-dev.jsonl"))
	if err != nil {
		return err
	}
	held, err := readLines[row](filepath.Join(reportDir, "japan-heldout.jsonl"))
	if err != nil {
		return err
	}
	summary.Dev, summary.Heldout, summary.Overall = breakdownOf(dev), breakdownOf(held), breakdownOf(append(dev, held...))
	b, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	file := filepath.Join(reportDir, "japan-summary.json")
	if !check {
		return os.WriteFile(file, b, 0644)
	}
	before, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	if !bytes.Equal(before, b) {
		return fmt.Errorf("%s differs from the recorded rows", file)
	}
	fmt.Println("RCEP Japan evaluation summary matches the recorded rows")
	return nil
}
