// Command jpeval runs and scores the Japan Customs origin-ruling evaluation
// (D710, D711). The phase-one baseline runs only on development; the
// phase-two candidate requires a pushed pre-registration for held-out or
// synthetic scoring.
//
//	go run ./cmd/jpeval baseline -split dev_eval -tag d711      (gateway; needs LITELLM_BASE_URL, LITELLM_API_KEY)
//	go run ./cmd/jpeval summary -check                           (no model calls)
//	go run ./cmd/jpeval index -check                             (no model calls)
//	go run ./cmd/jpeval bundle -check                            (no model calls)
//	go run ./cmd/jpeval candidate -split dev_eval -tag d712-dev  (gateway)
//	go run ./cmd/jpeval candidate-summary -split held_out -check  (no model calls)
//	go run ./cmd/jpeval plan -split held_out                     (no model calls, no network)
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpcorpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
)

const dataDir = "../data/jp"

// Output locations are variables so tests can point them at a temporary
// directory; nothing else changes them.
var (
	outDir     = "../eval/jp"
	runlogPath = "../eval/results/RUNLOG.jsonl"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: jpeval baseline|summary|index|bundle|candidate|candidate-summary|plan|smoke|budget-init [flags]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "baseline":
		err = baseline(os.Args[2:])
	case "summary":
		err = summary(os.Args[2:])
	case "index":
		err = index(os.Args[2:])
	case "bundle":
		err = bundle(os.Args[2:])
	case "candidate":
		err = candidate(os.Args[2:])
	case "candidate-summary":
		err = candidateSummary(os.Args[2:])
	case "plan":
		err = planCommand(os.Args[2:])
	case "budget-init":
		err = budgetInit(os.Args[2:])
	case "smoke":
		err = smoke(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "jpeval:", err)
		os.Exit(1)
	}
}

// Row is one scored case.
type Row struct {
	ID            string   `json:"id"`
	Agreement     string   `json:"agreement"`
	HSChapter     int      `json:"hs_chapter"`
	GoldCriterion string   `json:"gold_criterion"`
	GoldCriteria  []string `json:"gold_criteria"`
	StatedOrigin  string   `json:"stated_origin"`
	Qualifies     *bool    `json:"qualifies,omitempty"`
	Criterion     string   `json:"criterion,omitempty"`
	OriginCountry string   `json:"origin_country,omitempty"`
	DecidingRule  string   `json:"deciding_rule,omitempty"`
	Confidence    float64  `json:"confidence,omitempty"`
	Reasoning     string   `json:"reasoning,omitempty"`
	Precedents    []string `json:"precedents"`
	Model         string   `json:"model"`
	Provider      string   `json:"provider"`
	Prompt        string   `json:"prompt_version"`
	CostUSD       float64  `json:"cost_usd"`
	LatencyMS     int64    `json:"latency_ms"`
	TokensIn      int      `json:"tokens_in"`
	TokensOut     int      `json:"tokens_out"`
	Error         string   `json:"error,omitempty"`
}

type answer struct {
	Qualifies     bool    `json:"qualifies"`
	Criterion     string  `json:"criterion"`
	OriginCountry string  `json:"origin_country"`
	DecidingRule  string  `json:"deciding_rule"`
	Confidence    float64 `json:"confidence"`
	Reasoning     string  `json:"reasoning"`
}

func baseline(args []string) error {
	fs := flag.NewFlagSet("baseline", flag.ExitOnError)
	split := fs.String("split", "dev_eval", "split to run: dev_eval (phase 1 allows development splits only)")
	model := fs.String("model", "gemini-4-argon", "gateway model")
	k := fs.Int("k", 5, "precedents per case (the U.S. engine's PRECEDENT_K default)")
	tag := fs.String("tag", "", "run tag")
	workers := fs.Int("workers", 8, "parallel calls")
	limit := fs.Int("limit", 0, "first N cases only (0 = all)")
	_ = fs.Parse(args)
	if *split != "dev_eval" {
		return fmt.Errorf("split %q refused: phase 1 scores dev_eval only; held-out needs phase 2's pre-registration (D710)", *split)
	}
	base, key := os.Getenv("LITELLM_BASE_URL"), os.Getenv("LITELLM_API_KEY")
	if base == "" || key == "" {
		return errors.New("needs LITELLM_BASE_URL and LITELLM_API_KEY")
	}
	c, sp, labels, err := loadAll()
	if err != nil {
		return err
	}
	idx, err := c.Index(sp.Dev) // development pool only
	if err != nil {
		return err
	}
	cases := sp.DevEval
	if *limit > 0 && *limit < len(cases) {
		cases = cases[:*limit]
	}
	name := fmt.Sprintf("jp-%s-%s-%s-litellm", *split, *model, PromptVersion)
	if *tag != "" {
		name += "-" + *tag
	}
	outPath := filepath.Join(outDir, "runs", name+".jsonl")
	done := map[string]bool{}
	if old, err := readRows(outPath); err == nil {
		for _, r := range old {
			if r.Error == "" {
				done[r.ID] = true
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	m := llm.NewLiteLLM(base, key, 120*time.Second)
	start := time.Now()
	var mu sync.Mutex
	sem := make(chan struct{}, *workers)
	var wg sync.WaitGroup
	for _, id := range cases {
		if done[id] {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(id string) {
			defer wg.Done()
			defer func() { <-sem }()
			row := runCase(m, c, idx, labels[id], id, *model, *k)
			b, _ := json.Marshal(row)
			mu.Lock()
			_, _ = f.Write(append(b, '\n'))
			mu.Unlock()
		}(id)
	}
	wg.Wait()
	rows, err := readRows(outPath)
	if err != nil {
		return err
	}
	s := score(rows, c, sp, labels)
	fmt.Printf("%s: criterion %d/%d = %.1f%% (%.1f–%.1f), errors %d, $%.4f, p50 %d ms, p95 %d ms\n", name,
		s.Criterion.K, s.Criterion.N, 100*s.Criterion.P, 100*s.Criterion.Lo, 100*s.Criterion.Hi, s.Errors, s.CostUSD, s.P50MS, s.P95MS)
	return appendRunlog(map[string]any{
		"time": time.Now().Format(time.RFC3339), "run": name, "split": *split, "held_out": false, "decision": "D711",
		"cases": len(cases), "scored": s.Criterion.N, "correct": s.Criterion.K, "errors": s.Errors,
		"accuracy": round4(s.Criterion.P), "ci95": []float64{round4(s.Criterion.Lo), round4(s.Criterion.Hi)},
		"metric": "criterion match", "cost_usd": round4(s.CostUSD), "llm_provider": "litellm", "model": *model,
		"prompt_version": PromptVersion, "git_revision": revision(), "wall_s": int(time.Since(start).Seconds()),
	})
}

func runCase(m llm.Model, c *jpcorpus.Corpus, idx *retrieval.BM25, l jpcorpus.Label, id, model string, k int) Row {
	r, _ := c.Ruling(id)
	row := Row{ID: id, Agreement: r.Agreement, HSChapter: r.HSChapter, GoldCriterion: l.Criterion, GoldCriteria: l.Criteria,
		StatedOrigin: l.StatedOrigin, Model: model, Provider: "litellm", Prompt: PromptVersion, Precedents: []string{}}
	facts, err := c.CaseFacts(id)
	if err != nil {
		row.Error = err.Error()
		return row
	}
	t, _ := r.Time()
	cs := domain.Case{ID: jpcorpus.PrecedentID(id), Date: t}
	hits := idx.Top(facts, k, retrieval.PrecedentFilter(cs))
	precs := make([]domain.Ruling, len(hits))
	for i, h := range hits {
		precs[i] = h.Ruling
		row.Precedents = append(row.Precedents, h.Ruling.ID)
	}
	req := llm.Request{Model: model, System: system(r.Agreement), Messages: []llm.Message{{Role: llm.RoleUser, Content: user(facts, precs)}},
		JSONSchema: schema, SchemaName: "jp_origin", MaxOutputTokens: 8192}
	for attempt := 0; attempt < 3; attempt++ {
		tally := &llm.CostTally{}
		ctx, cancel := context.WithTimeout(llm.ContextWithCostTally(context.Background(), tally), 150*time.Second)
		resp, err := m.Generate(ctx, req)
		cancel()
		row.CostUSD += tally.USD()
		row.LatencyMS = resp.Latency.Milliseconds()
		row.TokensIn += resp.Usage.InputTokens
		row.TokensOut += resp.Usage.OutputTokens
		if err != nil {
			row.Error = err.Error()
			time.Sleep(time.Duration(3*(attempt+1)) * time.Second)
			continue
		}
		var a answer
		if err := json.Unmarshal([]byte(resp.Text), &a); err != nil {
			row.Error = "decode answer: " + err.Error()
			continue
		}
		row.Error = ""
		q := a.Qualifies
		row.Qualifies, row.Criterion, row.OriginCountry = &q, a.Criterion, a.OriginCountry
		row.DecidingRule, row.Confidence, row.Reasoning = a.DecidingRule, a.Confidence, a.Reasoning
		return row
	}
	return row
}

// Rate is k of n with its Wilson 95% interval.
type Rate struct {
	K  int     `json:"k"`
	N  int     `json:"n"`
	P  float64 `json:"p"`
	Lo float64 `json:"lo"`
	Hi float64 `json:"hi"`
}

func rate(k, n int) Rate {
	p, lo, hi := evalkit.Wilson(k, n)
	return Rate{K: k, N: n, P: round4(p), Lo: round4(lo), Hi: round4(hi)}
}

// Summary is the baseline's metrics file.
type Summary struct {
	Run               string          `json:"run"`
	Split             string          `json:"split"`
	Cases             int             `json:"cases"`
	Errors            int             `json:"errors"`
	Criterion         Rate            `json:"criterion_match"`
	CriterionAny      Rate            `json:"criterion_in_cited_set"`
	CriterionErrWrong Rate            `json:"criterion_match_errors_as_wrong"`
	Qualifies         Rate            `json:"says_originating"`
	Origin            Rate            `json:"stated_origin_match"`
	ByAgreement       map[string]Rate `json:"criterion_match_by_agreement"`
	Confusion         map[string]int  `json:"confusion_gold_to_pred"`
	Majority          Rate            `json:"majority_baseline"`
	MajorityLabel     string          `json:"majority_label"`
	CostUSD           float64         `json:"cost_usd"`
	CostPerCase       float64         `json:"cost_per_case_usd"`
	P50MS             int64           `json:"latency_p50_ms"`
	P95MS             int64           `json:"latency_p95_ms"`
	LabelsSHA256      string          `json:"labels_dev_sha256"`
	Note              string          `json:"note"`
}

func score(rows []Row, c *jpcorpus.Corpus, sp jpcorpus.Splits, labels map[string]jpcorpus.Label) Summary {
	s := Summary{Cases: len(rows), ByAgreement: map[string]Rate{}, Confusion: map[string]int{},
		Note: "Development set only (D711). All published answers are positive, so this measures the criterion Japan Customs cited, not whether the engine can refuse a good."}
	var ck, cn, ak, qk, qn, ok, on int
	agK, agN := map[string]int{}, map[string]int{}
	var lat []int64
	// Gold labels come from labels-dev.jsonl as committed now, not from the
	// copy stored in each row at run time, so a label fix re-scores old rows.
	for i, r := range rows {
		l := labels[r.ID]
		rows[i].GoldCriterion, rows[i].GoldCriteria, rows[i].StatedOrigin = l.Criterion, l.Criteria, l.StatedOrigin
	}
	for _, r := range rows {
		s.CostUSD += r.CostUSD
		if r.Error != "" {
			s.Errors++
			continue
		}
		lat = append(lat, r.LatencyMS)
		qn++
		if r.Qualifies != nil && *r.Qualifies {
			qk++
		}
		if r.GoldCriterion != "" {
			cn++
			agN[r.Agreement]++
			s.Confusion[r.GoldCriterion+"->"+r.Criterion]++
			if r.Criterion == r.GoldCriterion {
				ck++
				agK[r.Agreement]++
			}
			for _, g := range r.GoldCriteria {
				if g == r.Criterion {
					ak++
					break
				}
			}
		}
		if r.StatedOrigin != "" && r.StatedOrigin != "EU" {
			on++
			if domain.NormalizeCountry(r.OriginCountry) == domain.NormalizeCountry(r.StatedOrigin) {
				ok++
			}
		}
	}
	withGold := 0
	for _, r := range rows {
		if r.GoldCriterion != "" {
			withGold++
		}
	}
	s.Criterion, s.CriterionAny, s.Qualifies, s.Origin = rate(ck, cn), rate(ak, cn), rate(qk, qn), rate(ok, on)
	s.CriterionErrWrong = rate(ck, withGold)
	for a, n := range agN {
		s.ByAgreement[a] = rate(agK[a], n)
	}
	// Majority baseline: the commonest criterion among development rulings
	// outside dev_eval, predicted for every dev_eval case.
	inEval := map[string]bool{}
	for _, id := range sp.DevEval {
		inEval[id] = true
	}
	count := map[string]int{}
	for _, id := range sp.Dev {
		if !inEval[id] && labels[id].Criterion != "" {
			count[labels[id].Criterion]++
		}
	}
	best := ""
	for _, k := range jpcorpus.Criteria {
		if count[k] > count[best] {
			best = k
		}
	}
	mk, mn := 0, 0
	for _, id := range sp.DevEval {
		if l := labels[id]; l.Criterion != "" {
			mn++
			if l.Criterion == best {
				mk++
			}
		}
	}
	s.Majority, s.MajorityLabel = rate(mk, mn), best
	s.CostUSD = round4(s.CostUSD)
	if len(rows) > 0 {
		s.CostPerCase = round4(s.CostUSD / float64(len(rows)))
	}
	s.P50MS, s.P95MS = evalkit.Percentile(lat, 50), evalkit.Percentile(lat, 95)
	return s
}

func summary(args []string) error {
	fs := flag.NewFlagSet("summary", flag.ExitOnError)
	run := fs.String("run", "jp-dev_eval-gemini-4-argon-jp-baseline-v1-litellm-d711", "run name under eval/jp/runs")
	check := fs.Bool("check", false, "fail if the committed metrics differ")
	_ = fs.Parse(args)
	c, sp, labels, err := loadAll()
	if err != nil {
		return err
	}
	rows, err := readRows(filepath.Join(outDir, "runs", *run+".jsonl"))
	if err != nil {
		return err
	}
	s := score(rows, c, sp, labels)
	s.Run, s.Split = *run, "dev_eval"
	lb, err := os.ReadFile(filepath.Join(dataDir, "labels-dev.jsonl"))
	if err != nil {
		return err
	}
	s.LabelsSHA256 = fmt.Sprintf("%x", sha256.Sum256(lb))
	b, _ := json.MarshalIndent(s, "", " ")
	b = append(b, '\n')
	p := filepath.Join(outDir, "baseline-dev-metrics.json")
	if *check {
		old, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if !bytes.Equal(old, b) {
			return fmt.Errorf("%s differs from the rows; re-run jpeval summary", p)
		}
		fmt.Println("jp baseline metrics ok")
		return nil
	}
	return os.WriteFile(p, b, 0o644)
}

// IndexStats is eval/jp/index-stats.json: what the BM25 index holds.
type IndexStats struct {
	Rulings       int            `json:"rulings"`
	Translated    int            `json:"translated"`
	DevPool       int            `json:"dev_pool"`
	ByAgreement   map[string]int `json:"by_agreement"`
	ByChapter     map[string]int `json:"by_chapter"`
	ByYear        map[string]int `json:"by_year"`
	Splits        map[string]int `json:"splits"`
	SelfRetrieval Rate           `json:"dev_eval_same_agreement_top1"`
}

func index(args []string) error {
	fs := flag.NewFlagSet("index", flag.ExitOnError)
	check := fs.Bool("check", false, "fail if the committed stats differ")
	_ = fs.Parse(args)
	c, sp, _, err := loadAll()
	if err != nil {
		return err
	}
	st := IndexStats{Rulings: c.Len(), ByAgreement: map[string]int{}, ByChapter: map[string]int{}, ByYear: map[string]int{},
		Splits: map[string]int{"dev": len(sp.Dev), "dev_eval": len(sp.DevEval), "held_out": len(sp.HeldOut)}}
	for _, r := range c.Rulings() {
		if _, ok := c.English(r.ID); ok {
			st.Translated++
		}
		st.ByAgreement[r.Agreement]++
		st.ByChapter[fmt.Sprintf("%02d", r.HSChapter)]++
		st.ByYear[r.Date[:4]]++
	}
	idx, err := c.Index(sp.Dev)
	if err != nil {
		return err
	}
	st.DevPool = len(sp.Dev)
	// A label-free retrieval check: how often the top precedent claims the same agreement.
	k, n := 0, 0
	for _, id := range sp.DevEval {
		r, _ := c.Ruling(id)
		facts, _ := c.CaseFacts(id)
		t, _ := r.Time()
		hits := idx.Top(facts, 1, retrieval.PrecedentFilter(domain.Case{ID: jpcorpus.PrecedentID(id), Date: t}))
		if len(hits) == 0 {
			continue
		}
		n++
		h, _ := c.Ruling(strings.TrimPrefix(hits[0].Ruling.ID, "JP"))
		if h.Agreement == r.Agreement {
			k++
		}
	}
	st.SelfRetrieval = rate(k, n)
	b, _ := json.MarshalIndent(st, "", " ")
	b = append(b, '\n')
	p := filepath.Join(outDir, "index-stats.json")
	if *check {
		old, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if !bytes.Equal(old, b) {
			return fmt.Errorf("%s differs; re-run jpeval index", p)
		}
		fmt.Println("jp index stats ok")
		return nil
	}
	return os.WriteFile(p, b, 0o644)
}

func loadAll() (*jpcorpus.Corpus, jpcorpus.Splits, map[string]jpcorpus.Label, error) {
	c, err := jpcorpus.Load(dataDir)
	if err != nil {
		return nil, jpcorpus.Splits{}, nil, err
	}
	sp, err := jpcorpus.LoadSplits(filepath.Join(dataDir, "splits.json"))
	if err != nil {
		return nil, sp, nil, err
	}
	labels, err := jpcorpus.LoadLabels(filepath.Join(dataDir, "labels-dev.jsonl"))
	if err != nil {
		return nil, sp, nil, err
	}
	return c, sp, labels, nil
}

// readRows keeps the last row per case (a retried error is replaced), in first-seen order.
func readRows(path string) ([]Row, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	last := map[string]Row{}
	var order []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var r Row
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, err
		}
		if _, seen := last[r.ID]; !seen {
			order = append(order, r.ID)
		}
		last[r.ID] = r
	}
	sort.Strings(order)
	rows := make([]Row, len(order))
	for i, id := range order {
		rows[i] = last[id]
	}
	return rows, sc.Err()
}

func appendRunlog(entry map[string]any) error {
	f, err := os.OpenFile(runlogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
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
	if out, err := exec.Command("git", "rev-parse", "HEAD").Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}

func round4(v float64) float64 { return math.Round(v*1e4) / 1e4 }
