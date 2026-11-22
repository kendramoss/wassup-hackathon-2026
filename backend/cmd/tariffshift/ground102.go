package main

// The D550 development comparison, read offline (no model calls):
//
//	go run ./cmd/tariffshift ground102-eval [-check]   (in make reproduce)
//
// Part 102 engine usmca-v1 (re-run, B) against usmca-v2 (V, the 102.20 rows
// for the codes named in the facts quoted and applied as written), each once
// on the 72 development Part 102 rulings, scored with the rule pre-registered
// in D550: V's correct count not lower than B's, fewer rule contradictions
// on the rulings the committed 102.20 check decides, and V's cost at most
// 1.5 x B's. The committed October usmca-v1 run is reported beside them.

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

const (
	ground102Out   = evalDir + "/ground102/summary-d551.json"
	ground102Facts = "data/facts"
)

// ground102Runs: the D550 rows are kept out of eval/results (as the plan
// searches are) so the committed-row counts of the supersession and alert
// checks stay as published; RUNLOG.jsonl still records both runs.
var ground102Runs = []struct{ Key, File string }{
	{"committed_v1", "../eval/results/usmca-dev-gemini-4-argon-prec-usmca-v1-litellm.jsonl"},
	{"B_v1_d550", evalDir + "/ground102/runs/usmca-dev-gemini-4-argon-prec-usmca-v1-litellm-d550.jsonl"},
	{"V_v2_d550", evalDir + "/ground102/runs/usmca-dev-gemini-4-argon-prec-usmca-v2-litellm-d550.jsonl"},
}

type g102Row struct {
	Ruling  string  `json:"ruling"`
	Gold    string  `json:"gold"`
	Pred    string  `json:"pred"`
	Correct bool    `json:"correct"`
	Error   string  `json:"error"`
	Gateway float64 `json:"cost_usd_gateway"`
	Promo   float64 `json:"cost_usd"`
	// Latency and output tokens (read by p102ck-eval, D601).
	LatencyMS int64 `json:"latency_ms"`
	OutTok    int   `json:"output_tokens"`
	// Stated confidence, input tokens and the whole call's wall time (read
	// by p102bar, D610 and D611; wall_ms is on rows from D611 on).
	Confidence float64 `json:"confidence"`
	InTok      int     `json:"input_tokens"`
	WallMS     int64   `json:"wall_ms"`
}

// G102Run is one run's score on the development set.
type G102Run struct {
	File           string   `json:"file"`
	N              int      `json:"n"`
	Correct        int      `json:"correct"`
	Errors         int      `json:"errors_left"`
	QuotedCorrect  int      `json:"correct_with_quoted_rule"`
	OtherCorrect   int      `json:"correct_without_quoted_rule"`
	Contradictions []string `json:"rule_contradictions"`
	GatewayUSD     float64  `json:"gateway_cost_usd,omitempty"`
	PromoUSD       float64  `json:"promo_cost_usd"`
}

// G102Change is a ruling where B and V differ.
type G102Change struct {
	Ruling string `json:"ruling"`
	Gold   string `json:"cbp"`
	B      string `json:"b_v1"`
	V      string `json:"v_v2"`
	Quoted bool   `json:"rule_quoted"`
}

// G102Summary is the committed comparison.
type G102Summary struct {
	Note        string             `json:"note"`
	N           int                `json:"n"`
	Quoted      int                `json:"rulings_with_quoted_rule"`
	RuleDecided []string           `json:"rule_decided_rulings"`
	Runs        map[string]G102Run `json:"runs"`
	BOnly       int                `json:"b_only_correct"`
	VOnly       int                `json:"v_only_correct"`
	McNemarP    float64            `json:"mcnemar_exact_p"`
	CostRatio   float64            `json:"cost_ratio_v_over_b"`
	Pass        map[string]bool    `json:"pass"`
	Passed      bool               `json:"passed"`
	Changes     []G102Change       `json:"changes"`
}

func cmdGround102Eval(args []string) error {
	fl := flag.NewFlagSet("ground102-eval", flag.ExitOnError)
	check := fl.Bool("check", false, "recompute and fail if the committed file differs")
	_ = fl.Parse(args)
	splits, err := corpus.LoadSplits("../eval/splits/splits_usmca.json")
	if err != nil {
		return err
	}
	ids, err := splits.IDs("dev")
	if err != nil {
		return err
	}
	rules, err := tariffshift.Default()
	if err != nil {
		return err
	}
	ruleText := engine.Part102RuleText(rules)
	quoted := map[string]bool{}
	for _, id := range ids {
		b, err := os.ReadFile(filepath.Join(ground102Facts, id+".txt"))
		if err != nil {
			return err
		}
		if ruleText(domain.Case{Facts: string(b)}) != "" {
			quoted[id] = true
		}
	}
	ruleOrigin, err := ruleOrigins(filepath.Join(evalDir, "results-dev-full_text.jsonl"))
	if err != nil {
		return err
	}
	s := G102Summary{
		Note: "D550/D551: Part 102 engine usmca-v1 (B, re-run) and usmca-v2 (V, the 19 CFR 102.20 rows for the codes named in the facts quoted and applied as written), once each on the 72 development Part 102 rulings through an OpenAI-compatible LiteLLM gateway to Gemini; committed_v1 is the October usmca-v1 run. Rule contradictions: the engine's origin differs from the committed 102.20 check's on the rulings it decides (results-dev-full_text.jsonl, decided or decided if not de minimis). Recompute: cd backend && go run ./cmd/tariffshift ground102-eval -check",
		N:    len(ids), Quoted: len(quoted), Runs: map[string]G102Run{}, Pass: map[string]bool{},
	}
	for _, id := range ids {
		if _, ok := ruleOrigin[id]; ok {
			s.RuleDecided = append(s.RuleDecided, id)
		}
	}
	final := map[string]map[string]g102Row{}
	for _, run := range ground102Runs {
		rows, err := readG102(run.File)
		if err != nil {
			return err
		}
		last := map[string]g102Row{}
		r := G102Run{File: strings.TrimPrefix(run.File, "../"), N: len(ids), Contradictions: []string{}}
		for _, row := range rows {
			last[row.Ruling] = row
			r.GatewayUSD += row.Gateway
			r.PromoUSD += row.Promo
		}
		for _, id := range ids {
			row, ok := last[id]
			if !ok {
				return fmt.Errorf("%s: no row for %s", run.File, id)
			}
			if row.Error != "" {
				r.Errors++
			}
			ok = row.Error == "" && row.Correct
			if ok {
				r.Correct++
				if quoted[id] {
					r.QuotedCorrect++
				} else {
					r.OtherCorrect++
				}
			}
			if o, dec := ruleOrigin[id]; dec && domain.NormalizeCountry(row.Pred) != o {
				r.Contradictions = append(r.Contradictions, id)
			}
		}
		r.GatewayUSD, r.PromoUSD = round4(r.GatewayUSD), round4(r.PromoUSD)
		s.Runs[run.Key] = r
		final[run.Key] = last
	}
	b, v := final["B_v1_d550"], final["V_v2_d550"]
	for _, id := range ids {
		bc := b[id].Error == "" && b[id].Correct
		vc := v[id].Error == "" && v[id].Correct
		switch {
		case bc && !vc:
			s.BOnly++
		case vc && !bc:
			s.VOnly++
		}
		if bc != vc || b[id].Pred != v[id].Pred {
			s.Changes = append(s.Changes, G102Change{Ruling: id, Gold: b[id].Gold, B: b[id].Pred, V: v[id].Pred, Quoted: quoted[id]})
		}
	}
	s.McNemarP = round4(mcnemarExact(s.BOnly, s.VOnly))
	rb, rv := s.Runs["B_v1_d550"], s.Runs["V_v2_d550"]
	s.CostRatio = round4(rv.GatewayUSD / rb.GatewayUSD)
	s.Pass["accuracy_not_lower"] = rv.Correct >= rb.Correct
	s.Pass["fewer_contradictions"] = len(rv.Contradictions) < len(rb.Contradictions)
	s.Pass["cost_at_most_1.5x"] = s.CostRatio <= 1.5
	s.Passed = s.Pass["accuracy_not_lower"] && s.Pass["fewer_contradictions"] && s.Pass["cost_at_most_1.5x"]
	out, _ := json.MarshalIndent(s, "", " ")
	out = append(out, '\n')
	if err := os.MkdirAll(filepath.Dir(ground102Out), 0o755); err != nil {
		return err
	}
	if err := writeOrCheck(ground102Out, out, *check); err != nil {
		return fmt.Errorf("%w (ground102-eval)", err)
	}
	fmt.Printf("dev n=%d quoted=%d | B %d/%d contradictions %d $%.4f | V %d/%d contradictions %d $%.4f | B-only %d V-only %d p=%.4f | passed=%v\n",
		s.N, s.Quoted, rb.Correct, s.N, len(rb.Contradictions), rb.GatewayUSD, rv.Correct, s.N, len(rv.Contradictions), rv.GatewayUSD, s.BOnly, s.VOnly, s.McNemarP, s.Passed)
	return nil
}

// ruleOrigins is the 102.20 check's origin per ruling where it decides, or
// decides if not de minimis.
func ruleOrigins(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var r struct {
			Ruling string             `json:"ruling"`
			Rule   tariffshift.Result `json:"rule"`
		}
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, err
		}
		switch {
		case r.Rule.Status == tariffshift.Decided:
			out[r.Ruling] = domain.NormalizeCountry(r.Rule.Origin)
		case r.Rule.IfNotDeMinimis != nil:
			out[r.Ruling] = domain.NormalizeCountry(r.Rule.IfNotDeMinimis.Origin)
		}
	}
	return out, sc.Err()
}

func readG102(path string) ([]g102Row, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rows []g102Row
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var r g102Row
		if err := json.Unmarshal([]byte(l), &r); err != nil {
			return nil, err
		}
		rows = append(rows, r)
	}
	return rows, nil
}

// mcnemarExact is the exact two-sided McNemar p for b and c discordant pairs.
func mcnemarExact(b, c int) float64 {
	n := b + c
	if n == 0 {
		return 1
	}
	k := min(b, c)
	tail := new(big.Int)
	for i := 0; i <= k; i++ {
		tail.Add(tail, new(big.Int).Binomial(int64(n), int64(i)))
	}
	p, _ := new(big.Rat).SetFrac(new(big.Int).Mul(tail, big.NewInt(2)), new(big.Int).Lsh(big.NewInt(1), uint(n))).Float64()
	return min(p, 1)
}
