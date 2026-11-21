package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/adjudicate"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// adjRow is a stage-1 row with the adjudicator's second stage applied (D163).
// The embedded row holds the final answer; Base* keep stage 1's.
type adjRow struct {
	evalkit.Row
	BasePred        string   `json:"base_pred"`
	BaseCorrect     bool     `json:"base_correct"`
	BaseConfidence  float64  `json:"base_confidence"`
	AdjVersion      string   `json:"adj_version"`
	Triggered       bool     `json:"adj_triggered"`
	Changed         bool     `json:"adj_changed"`
	AdjOther        string   `json:"adj_other,omitempty"`
	AdjOrigin       string   `json:"adj_origin,omitempty"`
	AdjTransforms   *bool    `json:"adj_transforms,omitempty"`
	AdjConfidence   float64  `json:"adj_confidence,omitempty"`
	AdjClosest      string   `json:"adj_closest,omitempty"`
	AdjReasoning    string   `json:"adj_reasoning,omitempty"`
	AdjHeld         []string `json:"adj_held_china,omitempty"`
	AdjConferred    []string `json:"adj_conferred_elsewhere,omitempty"`
	AdjModelID      string   `json:"adj_model_id,omitempty"`
	AdjInputTokens  int      `json:"adj_input_tokens,omitempty"`
	AdjOutputTokens int      `json:"adj_output_tokens,omitempty"`
	AdjLatencyMS    int64    `json:"adj_latency_ms,omitempty"`
	AdjCostGateway  float64  `json:"adj_cost_usd_gateway,omitempty"`
	AdjCostPromo    float64  `json:"adj_cost_usd,omitempty"`
	AdjError        string   `json:"adj_error,omitempty"`
}

// adjudicateCmd applies the second stage to a committed stage-1 result file
// (D163). Untriggered cases are copied unchanged; nothing else is re-run.
func adjudicateCmd(args []string) error {
	fl := flag.NewFlagSet("adjudicate", flag.ExitOnError)
	basePath := fl.String("base", "", "stage-1 result file (engine-v1 rows), e.g. ../eval/results/dev-gemini-4-argon-prec-engine-v1-vertex.jsonl")
	split := fl.String("split", "dev", "split the base file scores")
	mode := fl.String("mode", adjudicate.Narrow, "narrow or symmetric (D163)")
	below := fl.Float64("below", 0.96, "trigger only when stage-1 confidence is below this")
	perSide := fl.Int("per-side", 3, "precedents per contrastive group")
	limit := fl.Int("limit", 0, "only the first N cases (0 = all)")
	workers := fl.Int("workers", 6, "concurrent model calls")
	confirmFinal := fl.Bool("confirm-final", false, "required for a held-out split (final, final2)")
	tag := fl.String("tag", "", "appended to the result file name")
	labelsPath := fl.String("labels", "../eval/data/labels.jsonl", "labels file(s), comma-separated")
	splitsPath := fl.String("splits", "../eval/splits/splits_final.json", "splits file")
	factsDir := fl.String("facts", "../data/corpus/facts", "facts directory(ies), comma-separated")
	resultsDir := fl.String("results", "../eval/results", "results directory")
	_ = fl.Parse(args)

	if *basePath == "" {
		return errors.New("-base is required")
	}
	if *mode != adjudicate.Narrow && *mode != adjudicate.Symmetric {
		return fmt.Errorf("-mode must be narrow or symmetric, got %q", *mode)
	}
	if isHeldOut(*split) && !*confirmFinal {
		return errors.New("held-out split: pass -confirm-final, and score each candidate at most once (D163)")
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	labels, err := loadLabelFiles(*labelsPath)
	if err != nil {
		return err
	}
	store, err := corpus.LoadDirs(labels, splitComma(*factsDir)...)
	if err != nil {
		return err
	}
	splits, err := corpus.LoadSplits(*splitsPath)
	if err != nil {
		return err
	}
	ids, err := splits.IDs(*split)
	if err != nil {
		return err
	}
	base, err := readRows(*basePath, ids)
	if err != nil {
		return err
	}
	if *limit > 0 && *limit < len(base) {
		base = base[:*limit]
	}
	model, err := newModel(ctx, cfg, logger)
	if err != nil {
		return err
	}
	opts := adjudicate.Options{Model: cfg.EngineModel, MaxOutputTokens: cfg.MaxOutputTokens, Mode: *mode, Below: *below, PerSide: *perSide}
	adj := adjudicate.New(model, store.Rulings(), opts)

	stem := strings.TrimSuffix(filepath.Base(*basePath), ".jsonl")
	name := fmt.Sprintf("%s+%s-%s-b%03.0f-%s", stem, adjudicate.Version, *mode, *below*100, cfg.LLMProvider)
	if *tag != "" {
		name += "-" + *tag
	}
	outPath := filepath.Join(*resultsDir, name+".jsonl")
	done := map[string]bool{}
	if prev, err := readAdjRows(outPath); err == nil {
		for _, r := range prev {
			if r.AdjError == "" {
				done[r.Ruling] = true
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()

	start := time.Now()
	var mu sync.Mutex
	sem := make(chan struct{}, *workers)
	var wg sync.WaitGroup
	triggered := 0
	for _, b := range base {
		if done[b.Ruling] || ctx.Err() != nil {
			continue
		}
		c, ok := store.Get(b.Ruling)
		if !ok {
			fmt.Fprintf(os.Stderr, "warning: %s has no facts; skipped\n", b.Ruling)
			continue
		}
		row := adjRow{Row: b, BasePred: b.Pred, BaseCorrect: b.Correct, BaseConfidence: b.Confidence, AdjVersion: adjudicate.Version}
		other, trig := "", false
		if b.Error == "" {
			other, trig = adjudicate.Candidates(c.Facts, b.Pred, b.Confidence, opts)
		}
		if !trig {
			writeAdjRow(&mu, out, row)
			continue
		}
		triggered++
		wg.Add(1)
		sem <- struct{}{}
		go func(row adjRow, c domain.Ruling, other string) {
			defer wg.Done()
			defer func() { <-sem }()
			tally := &llm.CostTally{}
			cctx := llm.ContextWithCostTally(ctx, tally)
			t0 := time.Now()
			d, err := adj.Decide(cctx, domain.Case{ID: c.ID, Date: c.Date, Facts: c.Facts}, other)
			row.Triggered, row.AdjOther = true, other
			row.AdjLatencyMS = time.Since(t0).Milliseconds()
			row.AdjCostGateway = tally.USD()
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				// Never let the gateway host reach a committed result file.
				row.AdjError = llm.ScrubGatewayHost(err.Error(), os.Getenv("LITELLM_BASE_URL"))
				writeAdjRow(&mu, out, row)
				return
			}
			tr := d.Transforms
			row.AdjOrigin, row.AdjTransforms, row.AdjConfidence = d.Origin, &tr, d.Confidence
			row.AdjClosest, row.AdjReasoning, row.AdjHeld, row.AdjConferred = d.Closest, d.Reasoning, d.Held, d.Conferred
			row.AdjModelID, row.AdjInputTokens, row.AdjOutputTokens = d.ModelID, d.Usage.InputTokens, d.Usage.OutputTokens
			row.AdjCostPromo = evalkit.Cost(evalkit.PromoPrices, cfg.EngineModel, d.Usage.InputTokens, d.Usage.OutputTokens)
			pred, conf, changed := adjudicate.Apply(row.Pred, row.Confidence, other, d)
			row.Changed = changed
			if changed {
				row.Pred, row.Confidence = pred, conf
				row.Correct = domain.NormalizeCountry(pred) == row.Gold
			}
			writeAdjRow(&mu, out, row)
		}(row, c, other)
	}
	wg.Wait()

	rows, err := readAdjRows(outPath)
	if err != nil {
		return err
	}
	var plain []evalkit.Row
	nTrig, nChanged, nErr, fixed, broke := 0, 0, 0, 0, 0
	adjCost := 0.0
	for _, r := range rows {
		plain = append(plain, r.Row)
		if r.Triggered {
			nTrig++
		}
		if r.AdjError != "" {
			nErr++
		}
		if r.Changed {
			nChanged++
		}
		if !r.BaseCorrect && r.Correct {
			fixed++
		}
		if r.BaseCorrect && !r.Correct {
			broke++
		}
		adjCost += r.AdjCostGateway
	}
	s := evalkit.Summarize(plain)
	f, err := os.OpenFile(filepath.Join(*resultsDir, "RUNLOG.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	entry := map[string]any{
		"time": time.Now().Format(time.RFC3339), "run": name, "split": *split, "held_out": isHeldOut(*split),
		"stage1": filepath.Base(*basePath), "adjudicator": adjudicate.Version, "mode": *mode, "below": *below, "per_side": *perSide,
		"cases": len(rows), "limit": *limit, "triggered": nTrig, "changed": nChanged, "adj_errors": nErr,
		"fixed_unadjusted": fixed, "broke_unadjusted": broke,
		"scored": s.N, "correct": s.Correct, "accuracy": round(s.Acc), "ci95": []float64{round(s.Lo), round(s.Hi)},
		"llm_provider": cfg.LLMProvider, "engine_model": cfg.EngineModel, "adj_cost_gateway_usd": round(adjCost),
		"git_revision": revision(), "wall_s": int(time.Since(start).Seconds()), "interrupted": ctx.Err() != nil,
	}
	b, _ := json.Marshal(entry)
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	fmt.Printf("%s: %d cases, %d triggered (%d this run), %d changed, %d errors; fixed %d, broke %d (unadjusted); accuracy %.1f%%; adjudicator gateway cost $%.4f\n",
		name, len(rows), nTrig, triggered, nChanged, nErr, fixed, broke, 100*s.Acc, adjCost)
	return ctx.Err()
}

func writeAdjRow(mu *sync.Mutex, out *os.File, r adjRow) {
	b, _ := json.Marshal(r)
	mu.Lock()
	defer mu.Unlock()
	_, _ = out.Write(append(b, '\n'))
}

// readAdjRows reads an adjudicated result file, keeping the last row per
// ruling (a retried adjudicator error is replaced by its success).
func readAdjRows(path string) ([]adjRow, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	last := map[string]adjRow{}
	var order []string
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line == "" {
			continue
		}
		var r adjRow
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if _, ok := last[r.Ruling]; !ok {
			order = append(order, r.Ruling)
		}
		last[r.Ruling] = r
	}
	out := make([]adjRow, len(order))
	for i, id := range order {
		out[i] = last[id]
	}
	return out, nil
}
