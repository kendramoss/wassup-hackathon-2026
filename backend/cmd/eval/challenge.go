package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/verifier"
)

var errMoveChallengeBudget = errors.New("challenge model budget reached")

// Reserve a conservative allowance before each raw provider call. The gateway
// cost is reconciled after it; no retry or panel perspective bypasses this gate.
type moveChallengeBudget struct {
	inner      llm.Model
	mu         sync.Mutex
	spent      float64
	pending    int
	limit      float64
	unreported int
}

func (m *moveChallengeBudget) Provider() string { return m.inner.Provider() }
func (m *moveChallengeBudget) total() float64   { m.mu.Lock(); defer m.mu.Unlock(); return m.spent }
func (m *moveChallengeBudget) Generate(ctx context.Context, req llm.Request) (llm.Response, error) {
	const reserve = 0.5
	m.mu.Lock()
	if m.spent+float64(m.pending+1)*reserve > m.limit+1e-12 {
		m.mu.Unlock()
		return llm.Response{}, errMoveChallengeBudget
	}
	m.pending++
	m.mu.Unlock()
	var local llm.CostTally
	r, err := m.inner.Generate(llm.ContextWithCostTally(ctx, &local), req)
	usd := local.USD()
	if local.Reported() == 0 {
		usd = evalkit.Cost(evalkit.PromoPrices, req.Model, r.Usage.InputTokens, r.Usage.OutputTokens)
	}
	m.mu.Lock()
	m.pending--
	m.spent += usd
	if local.Reported() == 0 {
		m.unreported++
	}
	m.mu.Unlock()
	if parent := llm.CostTallyFrom(ctx); parent != nil {
		parent.Record(usd)
	}
	return r, err
}

func readMoveChallengeRows(path string) ([]moveChallengeRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var rows []moveChallengeRow
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 65536), 4<<20)
	for s.Scan() {
		var r moveChallengeRow
		if err := json.Unmarshal(s.Bytes(), &r); err != nil {
			return nil, err
		}
		rows = append(rows, r)
	}
	return rows, s.Err()
}

func moveChallengeCmd(args []string) error {
	fl := flag.NewFlagSet("challenge", flag.ContinueOnError)
	dataset := fl.String("dataset", "../eval/challenges/verifier/cases.json", "frozen source-derived cases")
	split := fl.String("split", "dev", "dev or test")
	out := fl.String("out", "", "result JSONL; never overwritten")
	summary := fl.String("summary", "", "summary JSON path")
	facts := fl.String("facts", "../data/corpus/facts", "read-only precedent facts")
	raw := fl.String("raw", "../data/corpus/raw", "read-only raw rulings")
	labels := fl.String("labels", "../eval/data/labels.jsonl", "frozen precedent labels")
	workers := fl.Int("workers", 3, "parallel case checks")
	prompt := fl.String("prompt", "", "development candidate v2 or v4; empty uses configuration")
	maxUSD := fl.Float64("max-usd", 30, "model-call budget remaining for this run")
	resume := fl.Bool("resume", false, "continue only unattempted cases under same candidate")
	summarizeOnly := fl.Bool("summarize-only", false, "offline scoring of complete stored rows")
	if err := fl.Parse(args); err != nil {
		return err
	}
	if (*split != "dev" && *split != "test") || *out == "" || *workers < 1 || *workers > 6 || *maxUSD <= 0 || *maxUSD > 30 {
		return errors.New("invalid challenge options")
	}
	d, hash, err := loadMoveChallenge(*dataset)
	if err != nil {
		return err
	}
	var selected []moveChallengeCase
	for _, c := range d.Cases {
		if c.Split == *split {
			selected = append(selected, c)
		}
	}
	if len(selected) == 0 || (*split == "test" && len(selected) < 100) {
		return errors.New("missing or undersized challenge split")
	}
	if *summarizeOnly {
		rows, err := readMoveChallengeRows(*out)
		if err != nil {
			return err
		}
		return writeMoveChallengeSummary(d, hash, selected, rows, *summary)
	}
	if os.Getenv("LLM_PROVIDER") != providerLiteLLM {
		return errors.New("challenge requires the approved offline gateway provider")
	}
	// Uncommitted candidate code cannot be tied to a reproducible test result.
	cmd := exec.Command("git", "status", "--porcelain", "--", "cmd/eval", "internal/verifier", "internal/llm")
	status, err := cmd.Output()
	if err != nil {
		return err
	}
	if len(status) != 0 {
		return errors.New("commit challenge runner and candidate before paid scoring")
	}
	candidateBytes, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return err
	}
	candidate := string(bytes.TrimSpace(candidateBytes))
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if *prompt != "" {
		if *prompt != "v2" && *prompt != "v4" {
			return errors.New("unsupported challenge candidate prompt")
		}
		cfg.VerifierPrompt = *prompt
	}
	labs, err := corpus.LoadLabels(*labels)
	if err != nil {
		return err
	}
	search, err := verifier.NewLocal(labs, *facts, *raw)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0755); err != nil {
		return err
	}
	var rows []moveChallengeRow
	done := map[string]bool{}
	flags := os.O_CREATE | os.O_WRONLY | os.O_EXCL
	spent := 0.0
	if *resume {
		rows, err = readMoveChallengeRows(*out)
		if err != nil {
			return err
		}
		for _, r := range rows {
			if r.DatasetSHA256 != hash || r.Candidate != candidate || r.Split != *split || done[r.ID] || r.ModelID != cfg.VerifierModel || r.PromptVersion != cfg.VerifierPrompt || r.Panel != (cfg.VerifierPanel == "on") || r.PlanBar != cfg.PlanConfidence {
				return errors.New("resume candidate/dataset/split mismatch or duplicate row")
			}
			done[r.ID] = true
			spent += r.CostUSD
		}
		flags = os.O_WRONLY | os.O_APPEND
	}
	f, err := os.OpenFile(*out, flags, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	provider := llm.NewLiteLLM(os.Getenv("LITELLM_BASE_URL"), os.Getenv("LITELLM_API_KEY"), 2*time.Minute)
	budget := &moveChallengeBudget{inner: provider, spent: spent, limit: *maxUSD}
	model := llm.WithRetry(llm.WithPace(budget, cfg.LLMCallGap, llm.NewGate(4, cfg.LLMCoolDown)), cfg.LLMMaxRetries, 2*time.Second)
	checker := verifier.NewMajority(verifier.NewWithPanel(model, search, verifier.Options{Model: cfg.VerifierModel, MaxToolCalls: cfg.VerifierMaxToolCalls, MaxOutputTokens: 6000, Prompt: cfg.VerifierPrompt}, cfg.VerifierPanel == "on", cfg.VerifierPanelLow, cfg.VerifierPanelHigh), cfg.VerifierVotes)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sem := make(chan struct{}, *workers)
	var mu sync.Mutex
	var wg sync.WaitGroup
	var writeErr error
	for _, c := range selected {
		if done[c.ID] {
			continue
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(c moveChallengeCase) {
			defer wg.Done()
			defer func() { <-sem }()
			start := time.Now()
			before, _ := time.Parse(time.DateOnly, c.Input.Before)
			var tally llm.CostTally
			callCtx, stop := context.WithTimeout(llm.ContextWithCostTally(ctx, &tally), 8*time.Minute)
			defer stop()
			res, callErr := checker.Verify(callCtx, verifier.Task{FactsA: c.Input.FactsA, FactsB: c.Input.FactsB, Target: c.Input.Target, Before: before, Exclude: d.ExcludedRulings, KnownOriginA: c.Input.KnownOriginA})
			r := moveChallengeRow{ID: c.ID, Split: c.Split, DatasetSHA256: hash, Candidate: candidate, ModelID: cfg.VerifierModel, PromptVersion: cfg.VerifierPrompt, Panel: cfg.VerifierPanel == "on", PlanBar: cfg.PlanConfidence, OriginA: res.OriginA, OriginB: res.OriginB, ChangesOrigin: res.ChangesOrigin, Confidence: res.Verdict.Confidence, Supporting: res.Verdict.SupportingRuling, Counter: res.Verdict.CounterRuling, ModelCalls: res.ModelCalls, CostUSD: tally.USD(), LatencyMS: time.Since(start).Milliseconds()}
			r.Supported = callErr == nil && res.Verdict.PrecedentSupported && res.Verdict.Confidence >= cfg.PlanConfidence
			if callErr != nil {
				r.Error = "model_error"
				if errors.Is(callErr, errMoveChallengeBudget) {
					r.Error = "budget_reached"
				}
			}
			b, _ := json.Marshal(r)
			mu.Lock()
			defer mu.Unlock()
			if _, err := f.Write(append(b, '\n')); err != nil {
				writeErr = err
				cancel()
				return
			}
			if err := f.Sync(); err != nil {
				writeErr = err
				cancel()
				return
			}
			rows = append(rows, r)
			fmt.Printf("case %d/%d %s cost_usd=%.6f error=%s\n", len(rows), len(selected), r.ID, r.CostUSD, r.Error)
		}(c)
	}
	wg.Wait()
	if writeErr != nil {
		return writeErr
	}
	cost := map[string]any{"cost_usd": budget.total(), "unreported_cost_calls": budget.unreported, "dataset_sha256": hash, "candidate_revision": candidate}
	b, _ := json.MarshalIndent(cost, "", "  ")
	if err := os.WriteFile(*out+".cost.json", append(b, '\n'), 0644); err != nil {
		return err
	}
	return writeMoveChallengeSummary(d, hash, selected, rows, *summary)
}

func writeMoveChallengeSummary(d moveChallengeDataset, hash string, cases []moveChallengeCase, rows []moveChallengeRow, path string) error {
	for _, r := range rows {
		if r.DatasetSHA256 != hash || r.Candidate == "" || r.ModelID == "" || r.PromptVersion == "" {
			return errors.New("result dataset hash mismatch or missing candidate configuration")
		}
		if len(rows) > 0 && (r.ModelID != rows[0].ModelID || r.PromptVersion != rows[0].PromptVersion || r.Panel != rows[0].Panel || r.PlanBar != rows[0].PlanBar) {
			return errors.New("mixed model configurations")
		}
	}
	s, err := scoreMoveChallenge(cases, rows)
	if err != nil {
		return err
	}
	var cost float64
	var latency []int64
	candidate := ""
	for _, r := range rows {
		cost += r.CostUSD
		latency = append(latency, r.LatencyMS)
		if candidate == "" {
			candidate = r.Candidate
		}
		if r.Candidate != candidate {
			return errors.New("mixed candidate revisions")
		}
	}
	slices.Sort(latency)
	quantile := func(p float64) int64 {
		if len(latency) == 0 {
			return 0
		}
		return latency[int(math.Ceil(p*float64(len(latency))))-1]
	}
	result := map[string]any{"schema_version": 1, "evidence_kind": "source_grounded_synthetic", "feature": "move_verifier", "candidate_revision": candidate, "dataset_sha256": hash, "metrics": s, "cost_usd": cost, "latency_p50_ms": quantile(.5), "latency_p95_ms": quantile(.95), "limitations": d.Limits}
	if len(rows) > 0 {
		result["model_id"], result["prompt_version"], result["panel"], result["plan_bar"] = rows[0].ModelID, rows[0].PromptVersion, rows[0].Panel, rows[0].PlanBar
	}
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if path == "" {
		fmt.Println(string(b))
		return nil
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}
