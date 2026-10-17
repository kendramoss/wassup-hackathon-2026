package main

// Regression slice (D72).
//
//	eval slice -pick [-from FILE]
//	    Freeze eval/splits/slice30.json (30 dev rulings stratified by the
//	    confidence band they got in FILE, the Vertex dev run) and write
//	    eval/results/slice30-baseline.json from the same rows. No model calls.
//	eval slice -in FILE
//	    Compare an existing result file with the baseline. No model calls.
//	eval slice
//	    Run the engine (and router) on the slice with the current prompts and
//	    compare with the baseline. Needs EVAL_SLICE=on. Exits 1 when the run
//	    has more than EVAL_SLICE_MAX_DROP fewer correct cases than the
//	    baseline, or its accuracy at confidence ≥ 0.95 is under
//	    EVAL_SLICE_BAND_MIN. .github/workflows/eval-slice.yml runs this.

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalslice"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
)

// sliceSize is the number of rulings in the regression slice.
const sliceSize = 30

// errSliceFailed is returned when a run falls outside the tolerance.
var errSliceFailed = errors.New("slice: below the baseline tolerance")

func sliceCmd(args []string) error {
	fl := flag.NewFlagSet("slice", flag.ExitOnError)
	pick := fl.Bool("pick", false, "freeze the slice and its baseline from -from (no model calls)")
	from := fl.String("from", "../eval/results/dev-gemini-4-argon-prec-engine-v1-vertex.jsonl", "dev result file to pick from (-pick)")
	in := fl.String("in", "", "compare this existing result file instead of running the engine")
	slicePath := fl.String("slice", "../eval/splits/slice30.json", "slice file")
	basePath := fl.String("baseline", "../eval/results/slice30-baseline.json", "baseline file")
	adjustPath := fl.String("adjust", "../eval/data/scoring_adjustments.json", "scoring adjustments (-pick skips adjusted rulings)")
	labelsPath := fl.String("labels", "../eval/data/labels.jsonl", "labels file")
	factsDir := fl.String("facts", "data/facts", "facts directory (the bundled corpus, as the server uses)")
	out := fl.String("out", "", "result file of the live run (default ../eval/results/slice30-<prompt>-<provider>.jsonl, overwritten)")
	workers := fl.Int("workers", 6, "concurrent model calls")
	useRouter := fl.Bool("router", true, "also run the router (its scope label is reported, not scored)")
	_ = fl.Parse(args)

	if *pick {
		return slicePick(*from, *adjustPath, *slicePath, *basePath)
	}
	getenv := os.Getenv
	if *in != "" { // comparing a file makes no model call: no provider needed
		getenv = func(k string) string {
			if v := os.Getenv(k); v != "" || k != "LLM_PROVIDER" {
				return v
			}
			return config.ProviderFake
		}
	}
	cfg, err := config.FromEnv(getenv)
	if err != nil {
		return err
	}
	tol := evalslice.Tolerance{MaxDrop: cfg.EvalSliceMaxDrop, BandMin: cfg.EvalSliceBandMin}
	s, err := evalslice.LoadSlice(*slicePath)
	if err != nil {
		return err
	}
	base, err := evalslice.LoadBaseline(*basePath)
	if err != nil {
		return err
	}
	if err := evalslice.SameRulings(s, base); err != nil {
		return err
	}
	resultPath := *in
	if resultPath == "" {
		if !cfg.EvalSlice {
			return errors.New("slice: the live run makes model calls; set EVAL_SLICE=on (or pass -in FILE to compare an existing run)")
		}
		if resultPath, err = sliceRun(cfg, s, *labelsPath, *factsDir, *out, *workers, *useRouter); err != nil {
			return err
		}
	}
	rows, err := readRows(resultPath, s.IDs)
	if err != nil {
		return err
	}
	return sliceReport(os.Stdout, filepath.Base(resultPath), base, rows, tol)
}

// sliceReport prints the comparison and returns errSliceFailed on a regression.
func sliceReport(w io.Writer, title string, base evalslice.Baseline, rows []evalkit.Row, tol evalslice.Tolerance) error {
	rep := evalslice.Compare(base, rows, tol)
	fmt.Fprintln(w, rep.Markdown("Regression slice "+title, base))
	out := 0
	for _, r := range rows {
		if r.RouterRoute != "" && !r.InScope {
			out++
		}
	}
	if out > 0 {
		fmt.Fprintf(w, "- Router: %d of %d slice cases routed out of scope (reported, not scored)\n", out, len(rows))
	}
	if !rep.Passed() {
		return fmt.Errorf("%w: see the report above; if the drop is expected, record it in docs/DECISIONS.md and rebuild the baseline", errSliceFailed)
	}
	return nil
}

func slicePick(from, adjustPath, slicePath, basePath string) error {
	rows, err := readRows(from, nil)
	if err != nil {
		return err
	}
	adj, err := evalkit.LoadAdjustments(adjustPath)
	if err != nil {
		return err
	}
	s, err := evalslice.Pick(rows, adj, sliceSize)
	if err != nil {
		return err
	}
	src := filepath.Base(from)
	s.Doc = "Regression slice (D72): fixed dev rulings for eval slice. Development only; never add held-out rulings. Rebuild only with a recorded decision."
	s.FrozenAt, s.Source = time.Now().Format(time.RFC3339), src
	base, err := evalslice.BaselineFrom(rows, s, src)
	if err != nil {
		return err
	}
	base.Doc = "Baseline for eval slice (D72): the slice's rows of the source run. A run passes with at most EVAL_SLICE_MAX_DROP fewer correct " +
		"cases and accuracy at confidence >= 0.95 of at least EVAL_SLICE_BAND_MIN."
	if err := evalslice.WriteJSON(slicePath, s); err != nil {
		return err
	}
	if err := evalslice.WriteJSON(basePath, base); err != nil {
		return err
	}
	fmt.Printf("slice %d rulings -> %s; baseline %d / %d correct, ≥ 0.95: %d / %d -> %s\n",
		len(s.IDs), slicePath, base.Correct, base.N, base.Band95K, base.Band95N, basePath)
	return nil
}

// sliceRun predicts every slice ruling with the production engine setup
// (engine-v1, BM25 over the bundled corpus, K from the config) into a fresh
// result file and returns its path.
func sliceRun(cfg config.Config, s evalslice.Slice, labelsPath, factsDir, out string, workers int, useRouter bool) (string, error) {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	labels, err := corpus.LoadLabels(labelsPath)
	if err != nil {
		return "", err
	}
	store, err := corpus.Load(labels, factsDir)
	if err != nil {
		return "", err
	}
	var cases []domain.Ruling
	for _, id := range s.IDs {
		r, ok := store.Get(id)
		if !ok {
			return "", fmt.Errorf("slice: %s has no facts or label in %s", id, factsDir)
		}
		cases = append(cases, r)
	}
	model, err := newModel(ctx, cfg, logger)
	if err != nil {
		return "", err
	}
	eng := engine.New(model, retrieval.NewBM25(store.Rulings(), engine.PrecedentText),
		engine.Options{Model: cfg.EngineModel, K: cfg.PrecedentK, MaxOutputTokens: cfg.MaxOutputTokens})
	var rt *router.Router
	if useRouter {
		rt = router.New(model, cfg.RouterModel, 2000)
	}
	name := fmt.Sprintf("slice30-%s-%s-%s", cfg.EngineModel, engine.PromptVersion, cfg.LLMProvider)
	if out == "" {
		out = filepath.Join("../eval/results", name+".jsonl")
	}
	start := time.Now()
	rows := runCases(ctx, cases, workers, func(c domain.Ruling) evalkit.Row {
		row := evaluate(ctx, eng, rt, cfg, store, c, "slice30")
		row.Retrieval = "bm25"
		return row
	})
	if err := writeRows(out, rows); err != nil {
		return "", err
	}
	if err := appendRunLog(filepath.Dir(out), name, "slice30", cfg, evalkit.Summarize(rows), time.Since(start), len(cases), 0, ctx.Err() != nil); err != nil {
		return "", err
	}
	fmt.Fprintf(os.Stderr, "%d slice cases -> %s\n", len(rows), out)
	return out, ctx.Err()
}

// runCases evaluates cases concurrently and returns the rows in case order.
func runCases(ctx context.Context, cases []domain.Ruling, workers int, eval func(domain.Ruling) evalkit.Row) []evalkit.Row {
	rows := make([]evalkit.Row, len(cases))
	sem := make(chan struct{}, max(workers, 1))
	var wg sync.WaitGroup
	for i, c := range cases {
		if ctx.Err() != nil {
			rows[i] = evalkit.Row{Ruling: c.ID, Split: "slice30", Error: ctx.Err().Error()}
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, c domain.Ruling) {
			defer wg.Done()
			defer func() { <-sem }()
			rows[i] = eval(c)
		}(i, c)
	}
	wg.Wait()
	return rows
}

func writeRows(path string, rows []evalkit.Row) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, r := range rows {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	return f.Close()
}
