package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/textilerules"
)

// The textile engine's rows (D51), which the rule check sits beside. The
// final split's committed run is the Vertex textile-v1 run scored in D51; the
// final2 window was run once, with textile-v2 and the router.
var modelRows = map[string]string{
	"dev":    "../eval/results/textile-dev-gemini-4-argon-prec-textile-v1-litellm.jsonl",
	"final":  "../eval/results/textile-final-gemini-4-argon-prec-textile-v1-vertex.jsonl",
	"final2": "../eval/results/textile-final2-gemini-4-argon-prec-textile-v2-litellm-reasoning-v1-router.jsonl",
}

// freshSplits are the two sets of D955, scored apart from D952's: older (2019-2022
// rulings fetched by number) and pool (2023-01 to 2025-06 corpus rulings). The model's
// textile engine never ran on them, so there is no model column.
var freshSplits = map[string]bool{"older": true, "pool": true}

// thirdSplit is the third held-out set of D957: 2014-2018 New York rulings, labelled by
// the textile labeller (labels-textile.jsonl). It is reported apart from D955's two.
const thirdSplit = "third"

func splitIDs(split string) ([]string, error) {
	file := "../eval/splits/splits_textile.json"
	switch {
	case split == "final2":
		file = "../eval/splits/splits_textile_final2.json"
	case freshSplits[split]:
		file = "../eval/splits/splits_textile_fresh.json"
	case split == thirdSplit:
		file = "../eval/splits/splits_textile_third.json"
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
	if err := json.Unmarshal(m[split], &ids); err != nil || len(ids) == 0 {
		return nil, fmt.Errorf("split %q not found in %s", split, file)
	}
	return ids, nil
}

// Gold is CBP's answer for a ruling, with the scoring adjustments applied
// identically to every run in the repository (eval/data/scoring_adjustments.json).
type Gold struct {
	Origin       string
	Alternatives []string
	Excluded     string
}

// loadGold reads the labels the sets were first built on. With textile set, the
// labels of the textile labeller (D957) replace them for every ruling they cover;
// without it they are read for the third set only, which has no other label.
func loadGold(textile bool) (map[string]Gold, error) {
	out := map[string]Gold{}
	for _, f := range []string{"../eval/data/labels.jsonl", "../eval/data/labels_final2.jsonl", "data/labels_older.jsonl"} {
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
			if _, seen := out[r.Ruling]; seen || r.Origin == nil {
				continue
			}
			out[r.Ruling] = Gold{Origin: *r.Origin}
		}
	}
	tl, err := os.ReadFile(textileLabelsFile)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, line := range bytes.Split(tl, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var r struct {
			Ruling string  `json:"ruling"`
			Set    string  `json:"set"`
			Origin *string `json:"primary_origin"`
		}
		if err := json.Unmarshal(line, &r); err != nil {
			return nil, err
		}
		if !textile && r.Set != thirdSplit {
			continue
		}
		if r.Origin == nil { // no single holding under the textile labeller: out of the set, not a miss
			out[r.Ruling] = Gold{Excluded: "no single holding (textile labeller, D957)"}
			continue
		}
		out[r.Ruling] = Gold{Origin: *r.Origin}
	}
	b, err := os.ReadFile("../eval/data/scoring_adjustments.json")
	if err != nil {
		return nil, err
	}
	var adj struct {
		Exclude      map[string]string   `json:"exclude"`
		Alternatives map[string][]string `json:"alternatives"`
	}
	if err := json.Unmarshal(b, &adj); err != nil {
		return nil, err
	}
	for id, why := range adj.Exclude {
		g := out[id]
		g.Excluded = why
		out[id] = g
	}
	for id, alts := range adj.Alternatives {
		g := out[id]
		g.Alternatives = alts
		out[id] = g
	}
	return out, nil
}

func (g Gold) matches(origin string) bool {
	if origin == "" {
		return false
	}
	o := domain.NormalizeCountry(origin)
	if g.Origin != "" && domain.NormalizeCountry(g.Origin) == o {
		return true
	}
	for _, a := range g.Alternatives {
		if domain.NormalizeCountry(a) == o {
			return true
		}
	}
	return false
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

// CaseResult is one scored row.
type CaseResult struct {
	Ruling     string              `json:"ruling"`
	Gold       string              `json:"gold"`
	Model      string              `json:"model"`
	Rule       textilerules.Result `json:"rule"`
	Scored     bool                `json:"scored"`
	AgreeCBP   *bool               `json:"agree_cbp,omitempty"`
	AgreeModel *bool               `json:"agree_model,omitempty"`
	// CandidateAgree is the (c)(5) candidate against CBP where (c)(4) is a
	// judgment: exploratory, never counted as decided.
	CandidateAgree *bool `json:"candidate_agree_cbp,omitempty"`
}

// Cell is one split x source summary.
type Cell struct {
	Split          string         `json:"split"`
	Source         string         `json:"source"`
	N              int            `json:"n"`
	Extracted      int            `json:"extracted"`
	Excluded       int            `json:"excluded_no_single_holding"`
	OutOfScope     int            `json:"out_of_scope"`
	Decided        int            `json:"decided"`
	AgreeCBP       int            `json:"agree_cbp"`
	AgreeCBPCI     *[2]float64    `json:"agree_cbp_ci,omitempty"`
	DisagreeModel  int            `json:"disagree_model"`
	ModelOnDecided int            `json:"model_right_on_decided"`
	ByPara         map[string]int `json:"decided_by_paragraph"`
	Reasons        map[string]int `json:"undetermined_reasons"`
	Candidates     int            `json:"candidate_where_judgment"`
	CandidateRight int            `json:"candidate_agree_cbp"`
}

func cmdEval(args []string) error {
	fl := flag.NewFlagSet("eval", flag.ExitOnError)
	check := fl.Bool("check", false, "recompute and compare with the committed summary; write nothing")
	outDir := fl.String("out", evalDir, "directory for results-*.jsonl and summary.json")
	only := fl.String("split", "", "one split only (dev, final, final2); a partial run never writes summary.json over the committed one")
	goldFlag := fl.String("gold", "legacy", "legacy: the labels the sets were built on (reproduces D952 and D956); textile: the labels of the textile labeller, a post-hoc restatement that needs -out elsewhere (D957)")
	_ = fl.Parse(args)
	if *goldFlag != "legacy" && *goldFlag != "textile" {
		return fmt.Errorf("-gold must be legacy or textile")
	}
	if *goldFlag == "textile" && *outDir == evalDir {
		return fmt.Errorf("-gold textile restates the committed sets: give -out a directory of its own")
	}
	rules, err := textilerules.Default()
	if err != nil {
		return err
	}
	gold, err := loadGold(*goldFlag == "textile")
	if err != nil {
		return err
	}
	files, _ := filepath.Glob(filepath.Join(evalDir, "inputs-*.jsonl"))
	sort.Strings(files)
	order := map[string]int{"dev": 0, "final": 1, "final2": 2, "older": 3, "pool": 4, "third": 5}
	sort.SliceStable(files, func(i, j int) bool { return order[splitOf(files[i])] < order[splitOf(files[j])] })
	var cells, freshCells, thirdCells []Cell
	thirdRows := map[string][]InputRow{}
	for _, f := range files {
		if *only != "" && splitOf(f) != *only {
			continue
		}
		rows, err := readInputs(f)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			continue
		}
		split, source := rows[0].Split, rows[0].Source
		preds := map[string]string{}
		if !freshSplits[split] && split != thirdSplit {
			if preds, err = loadModel(modelRows[split]); err != nil {
				return err
			}
		}
		for _, plus := range []bool{false, true} {
			src := source
			if plus {
				src += "+suggested"
			}
			cell, out := scoreRows(rules, gold, preds, rows, split, src, plus)
			if cell.Decided > 0 {
				lo, hi := wilson(cell.AgreeCBP, cell.Decided)
				cell.AgreeCBPCI = &[2]float64{lo, hi}
			}
			if freshSplits[split] {
				freshCells = append(freshCells, cell)
			} else if split == thirdSplit {
				thirdCells = append(thirdCells, cell)
				thirdRows[src] = rows
			} else {
				cells = append(cells, cell)
			}
			if err := writeOrCheck(filepath.Join(*outDir, fmt.Sprintf("results-%s-%s.jsonl", split, src)), jsonl(out), *check); err != nil {
				return err
			}
		}
	}
	if *only == "" {
		b, _ := json.MarshalIndent(map[string]any{
			"note":  "19 CFR 102.21 rule check on the committed textile rulings (D951). source facts = the facts the engine reads; source full_text = the whole CROSS text with CBP's own classification (an encoding check, not product evidence). Recompute: cd backend && go run ./cmd/textilerules eval -check",
			"cells": cells,
		}, "", " ")
		if err := writeOrCheck(filepath.Join(*outDir, "summary.json"), append(b, '\n'), *check); err != nil {
			return err
		}
	}
	if *only == "" || freshSplits[*only] {
		if len(freshCells) > 0 {
			fc := freshReport(freshCells)
			b, _ := json.MarshalIndent(map[string]any{
				"note":  "D955: the second held-out test of the 19 CFR 102.21 rule check. sets older (2019-2022, fetched by number) and pool (2023-01 to 2025-06); pooled = both. Headline input: facts+suggested. Recompute: cd backend && go run ./cmd/textilerules eval -check",
				"cells": fc,
			}, "", " ")
			if *only == "" {
				if err := writeOrCheck(filepath.Join(*outDir, "summary-fresh.json"), append(b, '\n'), *check); err != nil {
					return err
				}
			}
			for _, c := range fc {
				fmt.Printf("%-6s %-17s n=%d out_of_scope=%d in_scope=%d decided=%d (%.1f%%) agree_cbp=%d ci=%v undetermined=%v\n",
					c.Split, c.Source, c.N, c.OutOfScope, c.InScope, c.Decided, c.DecidedShare, c.AgreeCBP, c.AgreeCBPCI, c.Reasons)
			}
		}
	}
	if len(thirdCells) > 0 {
		tc, err := thirdReport(rules, gold, thirdCells, thirdRows)
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(map[string]any{
			"note":  "D957: the third held-out test of the 19 CFR 102.21 rule check (2014-2018 New York rulings, labels of the textile labeller). Headline input: facts+suggested. Recompute: cd backend && go run ./cmd/textilerules eval -check",
			"cells": tc,
		}, "", " ")
		if *only == "" || *only == thirdSplit {
			if err := writeOrCheck(filepath.Join(*outDir, "summary-third.json"), append(b, '\n'), *check); err != nil {
				return err
			}
		}
		for _, c := range tc {
			fmt.Printf("%-8s %-17s n=%d out_of_scope=%d in_scope=%d decided=%d (%.1f%%) agree_cbp=%d ci=%v undetermined=%v\n",
				c.Split, c.Source, c.N, c.OutOfScope, c.InScope, c.Decided, c.DecidedShare, c.AgreeCBP, c.AgreeCBPCI, c.Reasons)
		}
	}
	for _, c := range cells {
		fmt.Printf("%-6s %-9s n=%d extracted=%d excluded=%d out_of_scope=%d decided=%d agree_cbp=%d disagree_model=%d paras=%v reasons=%v candidate=%d/%d\n",
			c.Split, c.Source, c.N, c.Extracted, c.Excluded, c.OutOfScope, c.Decided, c.AgreeCBP, c.DisagreeModel, c.ByPara, c.Reasons, c.CandidateRight, c.Candidates)
	}
	return nil
}

func sameCountry(a, b string) bool {
	return a != "" && b != "" && domain.NormalizeCountry(a) == domain.NormalizeCountry(b)
}

func splitOf(f string) string {
	b := strings.TrimPrefix(filepath.Base(f), "inputs-")
	s, _, _ := strings.Cut(b, "-")
	return s
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
		return fmt.Errorf("%s is stale: run go run ./cmd/textilerules eval", path)
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

// FreshCell is a Cell of the D955 sets with the in-scope count and the decided share.
type FreshCell struct {
	Cell
	InScope      int     `json:"in_scope"`
	DecidedShare float64 `json:"decided_share_of_in_scope_pct"`
}

// freshReport adds the pooled cell (older + pool, per input) to the per-set cells
// and the decided share of the rulings in scope. The model columns are not
// defined for these sets and are dropped.
func freshReport(cells []Cell) []FreshCell {
	pooled := map[string]*Cell{}
	var srcs []string
	for _, c := range cells {
		p := pooled[c.Source]
		if p == nil {
			p = &Cell{Split: "pooled", Source: c.Source, Reasons: map[string]int{}, ByPara: map[string]int{}}
			pooled[c.Source] = p
			srcs = append(srcs, c.Source)
		}
		p.N += c.N
		p.Extracted += c.Extracted
		p.Excluded += c.Excluded
		p.OutOfScope += c.OutOfScope
		p.Decided += c.Decided
		p.AgreeCBP += c.AgreeCBP
		p.Candidates += c.Candidates
		p.CandidateRight += c.CandidateRight
		for k, v := range c.Reasons {
			p.Reasons[k] += v
		}
		for k, v := range c.ByPara {
			p.ByPara[k] += v
		}
	}
	all := append([]Cell{}, cells...)
	for _, s := range srcs {
		p := pooled[s]
		if p.Decided > 0 {
			lo, hi := wilson(p.AgreeCBP, p.Decided)
			p.AgreeCBPCI = &[2]float64{lo, hi}
		}
		all = append(all, *p)
	}
	out := make([]FreshCell, 0, len(all))
	for _, c := range all {
		c.DisagreeModel, c.ModelOnDecided = 0, 0
		in := c.Extracted - c.OutOfScope
		// A ruling whose extraction failed is in scope and undetermined (D952).
		in += c.Reasons["extraction_failed"]
		fc := FreshCell{Cell: c, InScope: in}
		if in > 0 {
			fc.DecidedShare = math.Round(1000*float64(c.Decided)/float64(in)) / 10
		}
		out = append(out, fc)
	}
	return out
}

// scoreRows scores one set of extracted rows: the rule layer against CBP's holding.
func scoreRows(rules *textilerules.Rules, gold map[string]Gold, preds map[string]string, rows []InputRow, split, src string, plus bool) (Cell, []CaseResult) {
	cell := Cell{Split: split, Source: src, N: len(rows), Reasons: map[string]int{}, ByPara: map[string]int{}}
	var out []CaseResult
	for _, r := range rows {
		g := gold[r.Ruling]
		facts := r.Facts
		if plus && facts.GoodHTS == "" {
			facts.GoodHTS = r.Suggested
		}
		cr := CaseResult{Ruling: r.Ruling, Gold: g.Origin, Model: preds[r.Ruling]}
		if g.Excluded != "" {
			cell.Excluded++
			out = append(out, cr)
			continue
		}
		if r.Error != "" {
			cr.Rule = textilerules.Result{Status: textilerules.Undetermined, Reason: "extraction failed", Class: "extraction_failed"}
			cell.Reasons["extraction_failed"]++
			cr.Scored = true
			out = append(out, cr)
			continue
		}
		cell.Extracted++
		cr.Scored = true
		cr.Rule = rules.Evaluate(facts)
		switch cr.Rule.Status {
		case textilerules.Decided:
			cell.Decided++
			cell.ByPara[cr.Rule.Para]++
			a, m := g.matches(cr.Rule.Origin), sameCountry(cr.Rule.Origin, cr.Model)
			cr.AgreeCBP, cr.AgreeModel = &a, &m
			if a {
				cell.AgreeCBP++
			}
			if !m {
				cell.DisagreeModel++
			}
			if g.matches(cr.Model) {
				cell.ModelOnDecided++
			}
		case textilerules.OutOfScope:
			cell.OutOfScope++
		default:
			cell.Reasons[cr.Rule.Class]++
			if cr.Rule.Candidate != "" {
				cell.Candidates++
				c := g.matches(cr.Rule.Candidate)
				cr.CandidateAgree = &c
				if c {
					cell.CandidateRight++
				}
			}
		}
		out = append(out, cr)
	}
	return cell, out
}

// thirdReport is the third set's cells (one per input) followed by each
// period's cells, scored from the same extractions. The periods are fixed in
// the split file before scoring (D957); the bar is read on the whole set.
func thirdReport(rules *textilerules.Rules, gold map[string]Gold, cells []Cell, rows map[string][]InputRow) ([]FreshCell, error) {
	b, err := os.ReadFile("../eval/splits/splits_textile_third.json")
	if err != nil {
		return nil, err
	}
	var sp struct {
		Periods map[string][]string `json:"periods"`
	}
	if err := json.Unmarshal(b, &sp); err != nil {
		return nil, err
	}
	all := append([]Cell{}, cells...)
	names := make([]string, 0, len(sp.Periods))
	for n := range sp.Periods {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, c := range cells {
		for _, n := range names {
			in := map[string]bool{}
			for _, id := range sp.Periods[n] {
				in[id] = true
			}
			var sub []InputRow
			for _, r := range rows[c.Source] {
				if in[r.Ruling] {
					sub = append(sub, r)
				}
			}
			plus := strings.HasSuffix(c.Source, "+suggested")
			cell, _ := scoreRows(rules, gold, map[string]string{}, sub, "third "+n, c.Source, plus)
			if cell.Decided > 0 {
				lo, hi := wilson(cell.AgreeCBP, cell.Decided)
				cell.AgreeCBPCI = &[2]float64{lo, hi}
			}
			all = append(all, cell)
		}
	}
	out := make([]FreshCell, 0, len(all))
	for _, c := range all {
		c.DisagreeModel, c.ModelOnDecided = 0, 0
		in := c.Extracted - c.OutOfScope + c.Reasons["extraction_failed"]
		fc := FreshCell{Cell: c, InScope: in}
		if in > 0 {
			fc.DecidedShare = math.Round(1000*float64(c.Decided)/float64(in)) / 10
		}
		out = append(out, fc)
	}
	return out, nil
}
