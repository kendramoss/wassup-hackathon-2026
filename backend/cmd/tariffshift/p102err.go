package main

// Development error analysis of the Part 102 engine against the 19 CFR
// 102.20 rule check (D590), single setups, offline (no model calls):
//
//	go run ./cmd/tariffshift p102err-single [-check]   (in make reproduce)
//
// For every development Part 102 ruling (splits_usmca.json dev, 72) the
// rule check runs on three committed inputs, in this order: CBP's codes as
// the full ruling text states them (D362), CBP's code for the good with the
// top D381 development suggestion for every part (D382), and the top
// suggestion for the good and every part (D382). The rule's reference
// answer is the first input that decides; when none decides, the first
// whose only open question is the de minimis value gives a "lean" (the
// origin if de minimis does not apply). The engine's answers are the
// committed development rows of the Part 102 engine. No held-out file is
// read.

import (
	"encoding/json"
	"flag"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/hts"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

const p102errDir = evalDir + "/p102err"

// p102errRuns are the committed development runs of the Part 102 engine.
var p102errRuns = []struct{ Key, File string }{
	{"usmca-v1 (October)", "../eval/results/usmca-dev-gemini-4-argon-prec-usmca-v1-litellm.jsonl"},
	{"usmca-v1 (D550 B)", "../eval/tariffshift/ground102/runs/usmca-dev-gemini-4-argon-prec-usmca-v1-litellm-d550.jsonl"},
	{"usmca-v2 (D550 V)", "../eval/tariffshift/ground102/runs/usmca-dev-gemini-4-argon-prec-usmca-v2-litellm-d550.jsonl"},
}

// RuleOn is the rule check on one input.
type RuleOn struct {
	Input  string `json:"input"`
	Status string `json:"status"`
	Origin string `json:"origin,omitempty"`
	// IfNotDeMinimis: the origin when only the de minimis value is open.
	IfNotDeMinimis string `json:"if_not_de_minimis,omitempty"`
	Rule           string `json:"rule,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

// SingleRow is one development ruling.
type SingleRow struct {
	Ruling string   `json:"ruling"`
	CBP    string   `json:"cbp_origin"`
	Rules  []RuleOn `json:"rule_checks"`
	// RefKind: "decided", "lean" or "" (the rule gives nothing); RefOrigin
	// its origin; RefInput the input that gave it.
	RefKind   string            `json:"ref_kind,omitempty"`
	RefOrigin string            `json:"ref_origin,omitempty"`
	RefInput  string            `json:"ref_input,omitempty"`
	Engine    map[string]string `json:"engine"`
}

// SingleCell counts one engine run against the rule.
type SingleCell struct {
	Run          string   `json:"run"`
	Decided      int      `json:"rule_decided"`
	AgreeDecided int      `json:"engine_agrees_decided"`
	Lean         int      `json:"rule_lean"`
	AgreeLean    int      `json:"engine_agrees_lean"`
	CBPCorrect   int      `json:"engine_correct_of_72"`
	Disagree     []string `json:"disagreements"`
}

// SingleSummary is summary-single.json.
type SingleSummary struct {
	Note      string         `json:"note"`
	N         int            `json:"n"`
	ByRefKind map[string]int `json:"rulings_by_reference"`
	RefAgrees int            `json:"reference_agrees_with_cbp"`
	Cells     []SingleCell   `json:"runs"`
}

func cmdP102ErrSingle(args []string) error {
	fl := flag.NewFlagSet("p102err-single", flag.ExitOnError)
	check := fl.Bool("check", false, "recompute and fail if the committed files differ")
	_ = fl.Parse(args)
	rules, err := tariffshift.Default()
	if err != nil {
		return err
	}
	ids, err := splitIDs("dev")
	if err != nil {
		return err
	}
	gold, err := loadGold()
	if err != nil {
		return err
	}
	inputs, err := readInputs(filepath.Join(evalDir, "inputs-dev-full_text.jsonl"))
	if err != nil {
		return err
	}
	inBy := map[string]InputRow{}
	for _, r := range inputs {
		inBy[r.Ruling] = r
	}
	sugg, err := readJSONL[SuggestRow](plansSuggest)
	if err != nil {
		return err
	}
	sugBy := map[string]SuggestRow{}
	for _, s := range sugg {
		if s.Set != "usmca" {
			continue
		}
		if s.Split != "dev" {
			return fmt.Errorf("%s: %s is not a development row", plansSuggest, s.Ruling)
		}
		sugBy[s.Ruling] = s
	}
	runs := make([]map[string]string, len(p102errRuns))
	for i, r := range p102errRuns {
		if runs[i], err = loadModel(r.File); err != nil {
			return err
		}
	}
	sort.Strings(ids)
	sum := SingleSummary{N: len(ids), ByRefKind: map[string]int{},
		Note: "D590 development error analysis, single setups: the 19 CFR 102.20 rule check on the 72 development Part 102 rulings with three committed inputs (CBP's codes as the full text states them, D362; CBP's good code with the top D381 development suggestion for every part, D382; the top suggestion for the good and every part, D382), first deciding input as the reference, else a de minimis lean; against the committed development rows of the Part 102 engine. The rule inputs come from CBP's full ruling text and suggested codes, which the engine does not see, so this measures how far the engine reasons to the answer the rule gives, not the rule's accuracy. No model calls, no held-out file. Recompute: cd backend && go run ./cmd/tariffshift p102err-single -check"}
	cells := make([]SingleCell, len(p102errRuns))
	for i, r := range p102errRuns {
		cells[i].Run = r.Key
	}
	var rows []SingleRow
	for _, id := range ids {
		row := SingleRow{Ruling: id, CBP: domain.NormalizeCountry(gold[id]), Engine: map[string]string{}}
		var ins []struct {
			name string
			in   tariffshift.Input
		}
		if in, ok := inBy[id]; ok && in.Error == "" {
			ins = append(ins, struct {
				name string
				in   tariffshift.Input
			}{"CBP's codes (full text)", in.Input})
			if s, ok := sugBy[id]; ok && s.Error == "" && len(s.Parts) == len(in.Input.Materials) {
				top := map[int][]hts.PartCandidate{}
				for _, p := range s.Parts {
					top[p.Index] = p.Candidates
				}
				ins = append(ins, struct {
					name string
					in   tariffshift.Input
				}{"CBP's good code, top part suggestions", withParts(in.Input, in.Input.GoodHTS, top)})
				if s.Good != nil && len(s.Good.Candidates) > 0 {
					ins = append(ins, struct {
						name string
						in   tariffshift.Input
					}{"top suggestions for the good and parts", withParts(in.Input, s.Good.Candidates[0].HTS, top)})
				}
			}
		}
		for _, x := range ins {
			res := rules.Evaluate(x.in)
			ro := RuleOn{Input: x.name, Status: res.Status, Origin: domain.NormalizeCountry(res.Origin), Reason: res.Reason}
			if res.Rule != nil {
				ro.Rule = res.Rule.ID
			}
			if res.IfNotDeMinimis != nil {
				ro.IfNotDeMinimis = domain.NormalizeCountry(res.IfNotDeMinimis.Origin)
			}
			row.Rules = append(row.Rules, ro)
		}
		for _, ro := range row.Rules {
			if ro.Status == tariffshift.Decided {
				row.RefKind, row.RefOrigin, row.RefInput = "decided", ro.Origin, ro.Input
				break
			}
		}
		if row.RefKind == "" {
			for _, ro := range row.Rules {
				if ro.IfNotDeMinimis != "" {
					row.RefKind, row.RefOrigin, row.RefInput = "lean", ro.IfNotDeMinimis, ro.Input
					break
				}
			}
		}
		sum.ByRefKind[orNone(row.RefKind)]++
		if row.RefKind != "" && row.RefOrigin == row.CBP {
			sum.RefAgrees++
		}
		for i, r := range p102errRuns {
			pred := domain.NormalizeCountry(runs[i][id])
			row.Engine[r.Key] = pred
			if pred != "" && pred == row.CBP {
				cells[i].CBPCorrect++
			}
			switch row.RefKind {
			case "decided":
				cells[i].Decided++
				if pred == row.RefOrigin {
					cells[i].AgreeDecided++
				} else {
					cells[i].Disagree = append(cells[i].Disagree, fmt.Sprintf("%s: engine %s, rule %s (CBP %s)", id, orNone(pred), row.RefOrigin, row.CBP))
				}
			case "lean":
				cells[i].Lean++
				if pred == row.RefOrigin {
					cells[i].AgreeLean++
				} else {
					cells[i].Disagree = append(cells[i].Disagree, fmt.Sprintf("%s: engine %s, rule leans %s (CBP %s)", id, orNone(pred), row.RefOrigin, row.CBP))
				}
			}
		}
		rows = append(rows, row)
	}
	sum.Cells = cells
	sb, _ := json.MarshalIndent(sum, "", "  ")
	if err := writeOrCheckPlans(p102errDir+"/single-dev.jsonl", jsonlOf(rows), *check); err != nil {
		return err
	}
	if err := writeOrCheckPlans(p102errDir+"/summary-single.json", append(sb, '\n'), *check); err != nil {
		return err
	}
	for _, c := range cells {
		fmt.Printf("p102err-single %s: decided %d/%d, lean %d/%d, CBP %d/72\n", c.Run, c.AgreeDecided, c.Decided, c.AgreeLean, c.Lean, c.CBPCorrect)
	}
	return nil
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
