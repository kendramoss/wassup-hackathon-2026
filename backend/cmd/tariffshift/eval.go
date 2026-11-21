package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

// evalDir holds the committed extractions and results (run from backend/).
const evalDir = "../eval/tariffshift"

var modelRows = map[string]string{
	"dev":    "../eval/results/usmca-dev-gemini-4-argon-prec-usmca-v1-litellm.jsonl",
	"final":  "../eval/results/usmca-final-gemini-4-argon-prec-usmca-v1-litellm.jsonl",
	"final2": "../eval/results/usmca-final2-gemini-4-argon-prec-usmca-v1-litellm-router.jsonl",
}

func splitIDs(split string) ([]string, error) {
	file, key := "../eval/splits/splits_usmca.json", split
	if split == "final2" {
		file = "../eval/splits/splits_usmca_final2.json"
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	var ids []string
	if err := json.Unmarshal(m[key], &ids); err != nil || len(ids) == 0 {
		return nil, fmt.Errorf("split %q not found in %s", split, file)
	}
	return ids, nil
}

// CaseResult is one scored row.
type CaseResult struct {
	Ruling string             `json:"ruling"`
	Gold   string             `json:"gold"`
	Model  string             `json:"model"`
	Rule   tariffshift.Result `json:"rule"`
	// IfNotDeMinimis is the rule's origin when only the de minimis value is
	// open, else its decided origin.
	Agree          *bool `json:"agree_cbp,omitempty"`
	AgreeModel     *bool `json:"agree_model,omitempty"`
	CondAgree      *bool `json:"cond_agree_cbp,omitempty"`
	CondAgreeModel *bool `json:"cond_agree_model,omitempty"`
}

// Cell is one split × source summary.
type Cell struct {
	Split          string         `json:"split"`
	Source         string         `json:"source"`
	N              int            `json:"n"`
	Extracted      int            `json:"extracted"` // rows without an extraction error
	OutOfScope     int            `json:"out_of_scope"`
	Decided        int            `json:"decided"`
	AgreeCBP       int            `json:"agree_cbp"`
	AgreeCBPCI     *[2]float64    `json:"agree_cbp_ci,omitempty"`
	DisagreeModel  int            `json:"disagree_model"`
	CondDecided    int            `json:"decided_if_not_de_minimis"`
	CondAgreeCBP   int            `json:"agree_cbp_if_not_de_minimis"`
	CondDisagreeMd int            `json:"disagree_model_if_not_de_minimis"`
	Reasons        map[string]int `json:"undetermined_reasons"`
	ModelAccuracy  string         `json:"model_accuracy_on_decided,omitempty"`
}

func cmdEval(args []string) error {
	fl := flag.NewFlagSet("eval", flag.ExitOnError)
	check := fl.Bool("check", false, "recompute and compare with the committed summary; write nothing")
	_ = fl.Parse(args)
	rules, err := tariffshift.Default()
	if err != nil {
		return err
	}
	gold, err := loadGold()
	if err != nil {
		return err
	}
	var cells []Cell
	files, _ := filepath.Glob(filepath.Join(evalDir, "inputs-*.jsonl"))
	sort.Strings(files)
	order := map[string]int{"dev": 0, "final": 1, "final2": 2}
	sort.SliceStable(files, func(i, j int) bool { return order[splitOf(files[i])] < order[splitOf(files[j])] })
	for _, f := range files {
		rows, err := readInputs(f)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			continue
		}
		split, source := rows[0].Split, rows[0].Source
		preds, err := loadModel(modelRows[split])
		if err != nil {
			return err
		}
		cell := Cell{Split: split, Source: source, N: len(rows), Reasons: map[string]int{}}
		var out []CaseResult
		modelRight := 0
		for _, r := range rows {
			cr := CaseResult{Ruling: r.Ruling, Gold: gold[r.Ruling], Model: preds[r.Ruling]}
			if r.Error != "" {
				cr.Rule = tariffshift.Result{Status: tariffshift.Undetermined, Reason: "extraction failed"}
				cell.Reasons["extraction failed"]++
				out = append(out, cr)
				continue
			}
			cell.Extracted++
			cr.Rule = rules.Evaluate(r.Input)
			origin := ""
			switch {
			case cr.Rule.Status == tariffshift.Decided:
				origin = cr.Rule.Origin
				cell.Decided++
				cr.Agree, cr.AgreeModel = same(origin, cr.Gold), same(origin, cr.Model)
				if *cr.Agree {
					cell.AgreeCBP++
				}
				if !*cr.AgreeModel {
					cell.DisagreeModel++
				}
				if *same(cr.Model, cr.Gold) {
					modelRight++
				}
			case cr.Rule.Status == tariffshift.OutOfScope:
				cell.OutOfScope++
			default:
				cell.Reasons[reasonKey(cr.Rule)]++
			}
			cond := origin
			if cond == "" && cr.Rule.IfNotDeMinimis != nil {
				cond = cr.Rule.IfNotDeMinimis.Origin
			}
			if cond != "" {
				cell.CondDecided++
				cr.CondAgree, cr.CondAgreeModel = same(cond, cr.Gold), same(cond, cr.Model)
				if *cr.CondAgree {
					cell.CondAgreeCBP++
				}
				if !*cr.CondAgreeModel {
					cell.CondDisagreeMd++
				}
			}
			out = append(out, cr)
		}
		if cell.Decided > 0 {
			lo, hi := wilson(cell.AgreeCBP, cell.Decided)
			cell.AgreeCBPCI = &[2]float64{lo, hi}
			cell.ModelAccuracy = fmt.Sprintf("%d/%d", modelRight, cell.Decided)
		}
		cells = append(cells, cell)
		resFile := filepath.Join(evalDir, fmt.Sprintf("results-%s-%s.jsonl", split, source))
		if err := writeOrCheck(resFile, jsonl(out), *check); err != nil {
			return err
		}
	}
	b, _ := json.MarshalIndent(map[string]any{
		"note":  "19 CFR 102.20 rule check on the committed Part 102 rulings (D360, D361). source facts = the facts the engine reads; source full_text = the whole CROSS text with CBP's own classifications (an encoding check, not product evidence). Recompute: cd backend && go run ./cmd/tariffshift eval -check",
		"cells": cells,
	}, "", " ")
	if err := writeOrCheck(filepath.Join(evalDir, "summary.json"), append(b, '\n'), *check); err != nil {
		return err
	}
	for _, c := range cells {
		fmt.Printf("%-6s %-9s n=%d extracted=%d out_of_scope=%d decided=%d agree_cbp=%d disagree_model=%d | if-not-de-minimis decided=%d agree=%d\n",
			c.Split, c.Source, c.N, c.Extracted, c.OutOfScope, c.Decided, c.AgreeCBP, c.DisagreeModel, c.CondDecided, c.CondAgreeCBP)
	}
	return nil
}

func splitOf(f string) string {
	b := strings.TrimPrefix(filepath.Base(f), "inputs-")
	s, _, _ := strings.Cut(b, "-")
	return s
}

// reasonKey buckets an undetermined result for the summary.
func reasonKey(r tariffshift.Result) string {
	switch {
	case strings.Contains(r.Reason, "HTS code is missing"):
		return "no HTS code for the good"
	case strings.Contains(r.Reason, "too short"):
		return "good's code too short"
	case strings.Contains(r.Reason, "no materials"):
		return "no materials"
	case strings.Contains(r.Reason, "origin of"), strings.Contains(r.Reason, "country where the good is produced"):
		return "an origin or the country of production not given"
	case r.IfNotDeMinimis != nil || strings.Contains(r.Reason, "7%"):
		return "de minimis value not given"
	case strings.Contains(r.Reason, "cannot be decided for"):
		for _, m := range r.Materials {
			if m.Status == tariffshift.MatMissingHTS && m.Foreign {
				return "a foreign material has no HTS code"
			}
		}
		return "a material needs a fact (condition or goods description)"
	case strings.Contains(r.Reason, "essential character"), strings.Contains(r.Reason, "must be judged"):
		return "essential character needs a judgment"
	}
	return "other: " + r.Reason
}

func same(a, b string) *bool {
	v := a != "" && b != "" && domain.NormalizeCountry(a) == domain.NormalizeCountry(b)
	return &v
}

func readInputs(f string) ([]InputRow, error) {
	b, err := os.ReadFile(f)
	if err != nil {
		return nil, err
	}
	var rows []InputRow
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var r InputRow
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		rows = append(rows, r)
	}
	return rows, sc.Err()
}

func loadGold() (map[string]string, error) {
	out := map[string]string{}
	for _, f := range []string{"../eval/data/labels_usmca.jsonl", "../eval/data/labels_usmca_final2.jsonl"} {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		for _, line := range bytes.Split(b, []byte("\n")) {
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			var r struct {
				Ruling string  `json:"ruling"`
				Origin *string `json:"primary_origin"`
			}
			if err := json.Unmarshal(line, &r); err != nil {
				return nil, err
			}
			if r.Origin != nil {
				out[r.Ruling] = *r.Origin
			}
		}
	}
	return out, nil
}

func loadModel(f string) (map[string]string, error) {
	b, err := os.ReadFile(f)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, line := range bytes.Split(b, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var r struct {
			Ruling string `json:"ruling"`
			Pred   string `json:"pred"`
		}
		if err := json.Unmarshal(line, &r); err != nil {
			return nil, err
		}
		out[r.Ruling] = r.Pred
	}
	return out, nil
}

func jsonl(rows []CaseResult) []byte {
	var buf bytes.Buffer
	for _, r := range rows {
		b, _ := json.Marshal(r)
		buf.Write(append(b, '\n'))
	}
	return buf.Bytes()
}

func writeOrCheck(path string, b []byte, check bool) error {
	if !check {
		return os.WriteFile(path, b, 0o644)
	}
	old, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(old, b) {
		return fmt.Errorf("%s is stale: run go run ./cmd/tariffshift eval", path)
	}
	return nil
}

// wilson is the 95% Wilson score interval, rounded to 0.1 percent.
func wilson(k, n int) (float64, float64) {
	if n == 0 {
		return 0, 0
	}
	z := 1.959963984540054
	p := float64(k) / float64(n)
	d := 1 + z*z/float64(n)
	c := (p + z*z/(2*float64(n))) / d
	h := z * math.Sqrt(p*(1-p)/float64(n)+z*z/(4*float64(n)*float64(n))) / d
	r := func(x float64) float64 { return math.Round(x*1000) / 10 }
	return r(c - h), r(c + h)
}

func round4(x float64) float64 { return math.Round(x*10000) / 10000 }

func appendRunlog(entry map[string]any) error {
	f, err := os.OpenFile("../eval/results/RUNLOG.jsonl", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}

func revision() string {
	out, err := exec.Command("git", "rev-parse", "--short=8", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	rev := strings.TrimSpace(string(out))
	if st, err := exec.Command("git", "status", "--porcelain", "--untracked-files=no", "--", ".").Output(); err == nil && len(bytes.TrimSpace(st)) > 0 {
		rev += "-dirty"
	}
	return rev
}
