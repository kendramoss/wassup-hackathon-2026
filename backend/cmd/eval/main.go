// Command eval runs the origin engine on a frozen split and reports accuracy.
//
//	eval run    -split dev|final [-limit N] [-workers 8] [-router] [-cond prec|zero] [-confirm-final]
//	    Predict every ruling in the split (resume-safe), write one JSONL row per
//	    case to eval/results/, append the run to eval/results/RUNLOG.jsonl and
//	    print a Markdown summary. The held-out split needs -confirm-final:
//	    never tune prompts, thresholds or retrieval on it.
//	eval scope  [-from 2025-07-01] [-to 2025-12-31] [-per-regime 40]
//	    Run the router on labelled rulings of every regime in a date window
//	    (dev window by default) and report how often each regime is routed
//	    out of scope. Checks the scope guard; never touches the final split.
//	eval report -in FILE [-in FILE ...]
//	    Print the Markdown summary of existing result files.
//
// Model and provider come from the same environment as the server
// (LLM_PROVIDER, ENGINE_MODEL, ROUTER_MODEL, PRECEDENT_K, ...). Run from backend/.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/triage"
)

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: eval run|report [flags]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "run":
		err = run(os.Args[2:])
	case "report":
		err = report(os.Args[2:])
	case "scope":
		err = scope(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "eval:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fl := flag.NewFlagSet("run", flag.ExitOnError)
	split := fl.String("split", "dev", "dev, final or pilot")
	limit := fl.Int("limit", 0, "only the first N cases (0 = all)")
	workers := fl.Int("workers", 8, "concurrent model calls")
	useRouter := fl.Bool("router", false, "also run the router; its regime label drives the scope check")
	cond := fl.String("cond", "prec", "prec (with precedents) or zero (facts only)")
	confirmFinal := fl.Bool("confirm-final", false, "required to score the held-out final split")
	labelsPath := fl.String("labels", "../eval/data/labels.jsonl", "labels file")
	splitsPath := fl.String("splits", "../eval/splits/splits_final.json", "splits file")
	factsDir := fl.String("facts", "../data/corpus/facts", "facts directory (go run ./cmd/corpus build)")
	resultsDir := fl.String("results", "../eval/results", "results directory")
	_ = fl.Parse(args)

	if *split == "final" && !*confirmFinal {
		return errors.New("the final split is held out: pass -confirm-final, and do not change prompts, thresholds or retrieval because of its result")
	}
	if *cond != "prec" && *cond != "zero" {
		return fmt.Errorf("-cond must be prec or zero, got %q", *cond)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	labels, err := corpus.LoadLabels(*labelsPath)
	if err != nil {
		return err
	}
	store, err := corpus.Load(labels, *factsDir)
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
	var cases []domain.Ruling
	for _, id := range ids {
		if r, ok := store.Get(id); ok {
			cases = append(cases, r)
		} else {
			fmt.Fprintf(os.Stderr, "warning: %s has no facts or label; skipped\n", id)
		}
	}
	if *limit > 0 && *limit < len(cases) {
		cases = cases[:*limit]
	}

	model, err := newModel(ctx, cfg, logger)
	if err != nil {
		return err
	}
	var retr retrieval.Retriever
	if *cond == "prec" {
		retr = retrieval.NewBM25(store.Rulings(), engine.PrecedentText)
	}
	eng := engine.New(model, retr, engine.Options{Model: cfg.EngineModel, K: cfg.PrecedentK, MaxOutputTokens: cfg.MaxOutputTokens})
	var rt *router.Router
	if *useRouter {
		rt = router.New(model, cfg.RouterModel, 2000)
	}

	name := fmt.Sprintf("%s-%s-%s-%s-%s", *split, cfg.EngineModel, *cond, engine.PromptVersion, cfg.LLMProvider)
	if *useRouter {
		name += "-router"
	}
	outPath := filepath.Join(*resultsDir, name+".jsonl")
	done, err := completed(outPath)
	if err != nil {
		return err
	}
	var todo []domain.Ruling
	for _, c := range cases {
		if !done[c.ID] {
			todo = append(todo, c)
		}
	}
	fmt.Fprintf(os.Stderr, "%d cases, %d to run -> %s\n", len(cases), len(todo), outPath)

	if err := os.MkdirAll(*resultsDir, 0o755); err != nil {
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
	finished := 0
	for _, c := range todo {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(c domain.Ruling) {
			defer wg.Done()
			defer func() { <-sem }()
			row := evaluate(ctx, eng, rt, cfg, store, c, *split)
			if ctx.Err() != nil && row.Error != "" {
				return // interrupted: leave the case for the next run
			}
			b, _ := json.Marshal(row)
			mu.Lock()
			defer mu.Unlock()
			_, _ = out.Write(append(b, '\n'))
			finished++
			if finished%25 == 0 {
				fmt.Fprintf(os.Stderr, "  %d/%d\n", finished, len(todo))
			}
		}(c)
	}
	wg.Wait()

	rows, err := readRows(outPath, ids)
	if err != nil {
		return err
	}
	s := evalkit.Summarize(rows)
	if err := appendRunLog(*resultsDir, name, *split, cfg, s, time.Since(start), len(cases), *limit, ctx.Err() != nil); err != nil {
		return err
	}
	fmt.Println(evalkit.Markdown(name, *split, s))
	return ctx.Err()
}

func evaluate(ctx context.Context, eng *engine.Engine, rt *router.Router, cfg config.Config, store *corpus.Store, c domain.Ruling, split string) evalkit.Row {
	lab, _ := store.Label(c.ID)
	row := evalkit.Row{
		Ruling: c.ID, Date: lab.Date, Split: split, LabelMethod: lab.Method,
		Gold: domain.NormalizeCountry(lab.Origin()), Provider: cfg.LLMProvider, ModelID: cfg.EngineModel,
		PromptVersion: engine.PromptVersion, InScope: true,
	}
	inScope, scopeReason := true, ""
	if rt != nil {
		if l, err := rt.Classify(ctx, c.Facts); err == nil {
			row.RouterRoute, row.RouterRegime, row.RouterFamily = l.Route, l.Regime, l.Subfamily
			inScope, scopeReason = l.Scope()
		} else {
			row.RouterRoute = "error: " + err.Error()
		}
	}
	res, err := eng.Predict(ctx, domain.Case{ID: c.ID, Date: c.Date, Facts: c.Facts})
	if err != nil {
		row.Error = err.Error()
		return row
	}
	p := res.Prediction
	row.Pred, row.Correct = p.Origin, p.Origin == row.Gold
	row.Confidence, row.KeyFactors, row.DecisiveStep = p.Confidence, p.KeyFactors, p.DecisiveStep
	row.Flip, row.Reasoning, row.Precedents, row.Cited = res.Flip, p.Reasoning, p.Precedents, p.CitedRulings
	row.Provider, row.ModelID = p.Trace.Provider, p.Trace.ModelID
	row.InputTokens, row.OutputTokens, row.LatencyMS = p.Trace.InputTokens, p.Trace.OutputTokens, p.Trace.LatencyMS
	row.CostUSD = evalkit.Cost(evalkit.PromoPrices, cfg.EngineModel, row.InputTokens, row.OutputTokens)
	row.CostUSDList = evalkit.Cost(evalkit.ListPrices, cfg.EngineModel, row.InputTokens, row.OutputTokens)
	row.InScope = inScope
	row.TriageRoute = string(triage.Decide(p, cfg.FastPathConfidence, inScope, scopeReason).Route)
	return row
}

func newModel(ctx context.Context, cfg config.Config, logger *slog.Logger) (llm.Model, error) {
	var base llm.Model
	switch cfg.LLMProvider {
	case config.ProviderLiteLLM:
		base = llm.NewLiteLLM(cfg.LiteLLMBaseURL, cfg.LiteLLMAPIKey, cfg.LLMTimeout)
	case config.ProviderVertex:
		v, err := llm.NewVertex(ctx, llm.VertexOptions{Project: cfg.GCPProject, Location: cfg.GCPLocation, Timeout: cfg.LLMTimeout})
		if err != nil {
			return nil, err
		}
		base = v
	default:
		return nil, fmt.Errorf("eval needs a real model provider, got %q", cfg.LLMProvider)
	}
	return llm.WithLogging(llm.WithRetry(base, cfg.LLMMaxRetries, 2*time.Second), logger), nil
}

// completed returns rulings already scored without error in a result file.
func completed(path string) (map[string]bool, error) {
	done := map[string]bool{}
	rows, err := readRows(path, nil)
	if errors.Is(err, os.ErrNotExist) {
		return done, nil
	}
	for _, r := range rows {
		if r.Error == "" {
			done[r.Ruling] = true
		}
	}
	return done, err
}

// readRows reads a result file, keeping the last row per ruling (a retried
// error is replaced by its success) and, if ids is set, only those rulings.
func readRows(path string, ids []string) ([]evalkit.Row, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	last := map[string]evalkit.Row{}
	var order []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var r evalkit.Row
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if len(want) > 0 && !want[r.Ruling] {
			continue
		}
		if _, seen := last[r.Ruling]; !seen {
			order = append(order, r.Ruling)
		}
		last[r.Ruling] = r
	}
	rows := make([]evalkit.Row, len(order))
	for i, id := range order {
		rows[i] = last[id]
	}
	return rows, sc.Err()
}

// appendRunLog records every run (dev and held-out) in RUNLOG.jsonl.
func appendRunLog(dir, name, split string, cfg config.Config, s evalkit.Summary, wall time.Duration, n, limit int, interrupted bool) error {
	f, err := os.OpenFile(filepath.Join(dir, "RUNLOG.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	entry := map[string]any{
		"time": time.Now().Format(time.RFC3339), "run": name, "split": split, "held_out": split == "final",
		"cases": n, "limit": limit, "scored": s.N, "correct": s.Correct, "errors": s.Errors,
		"accuracy": round(s.Acc), "ci95": []float64{round(s.Lo), round(s.Hi)},
		"fast_path_coverage": s.FastN, "fast_path_correct": s.FastK,
		"llm_provider": cfg.LLMProvider, "engine_model": cfg.EngineModel, "router_model": cfg.RouterModel,
		"prompt_version": engine.PromptVersion, "k": cfg.PrecedentK, "fast_path_threshold": cfg.FastPathConfidence,
		"git_revision": revision(), "wall_s": int(wall.Seconds()), "interrupted": interrupted,
		"cost_per_case_usd": s.CostPerCase, "latency_p50_ms": s.LatencyP50, "latency_p95_ms": s.P95,
	}
	b, _ := json.Marshal(entry)
	_, err = f.Write(append(b, '\n'))
	return err
}

func round(x float64) float64 { return float64(int(x*10000+0.5)) / 10000 }

func revision() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		rev, dirty := "", false
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				rev = s.Value
			case "vcs.modified":
				dirty = s.Value == "true"
			}
		}
		if rev != "" {
			if dirty {
				rev += "-dirty"
			}
			return rev
		}
	}
	// go run does not embed VCS info; ask git directly.
	if out, err := exec.Command("git", "rev-parse", "HEAD").Output(); err == nil {
		rev := strings.TrimSpace(string(out))
		if st, err := exec.Command("git", "status", "--porcelain", "--untracked-files=no", "--", ".").Output(); err == nil && len(strings.TrimSpace(string(st))) > 0 {
			rev += "-dirty"
		}
		return rev
	}
	return "unknown"
}

func report(args []string) error {
	fl := flag.NewFlagSet("report", flag.ExitOnError)
	var in stringList
	fl.Var(&in, "in", "result file (repeatable)")
	scopeFile := fl.String("scope", "", "router label file from eval scope -out; recomputes in-scope and the fast path")
	threshold := fl.Float64("threshold", 0.95, "fast-path confidence threshold used with -scope")
	_ = fl.Parse(args)
	var scopeOf map[string]bool
	if *scopeFile != "" {
		var err error
		if scopeOf, err = readScope(*scopeFile); err != nil {
			return err
		}
	}
	if len(in) == 0 {
		return errors.New("report: -in is required")
	}
	for _, p := range in {
		rows, err := readRows(p, nil)
		if err != nil {
			return err
		}
		split := ""
		if len(rows) > 0 {
			split = rows[0].Split
		}
		if scopeOf != nil {
			for i := range rows {
				inScope, ok := scopeOf[rows[i].Ruling]
				if !ok {
					inScope = true // no label: the router is not consulted
				}
				rows[i].InScope = inScope
				rows[i].TriageRoute = "needs_review"
				if inScope && rows[i].Error == "" && rows[i].Confidence >= *threshold {
					rows[i].TriageRoute = "auto"
				}
			}
		}
		fmt.Println(evalkit.Markdown(strings.TrimSuffix(filepath.Base(p), ".jsonl"), split, evalkit.Summarize(rows)))
	}
	return nil
}

func scope(args []string) error {
	fl := flag.NewFlagSet("scope", flag.ExitOnError)
	from := fl.String("from", "2025-07-01", "first ruling date (inclusive)")
	to := fl.String("to", "2025-12-31", "last ruling date (inclusive)")
	perRegime := fl.Int("per-regime", 40, "max rulings per gold regime (first in file order)")
	splitName := fl.String("split", "", "instead of a date window, label every ruling of this split (dev only)")
	splitsPath := fl.String("splits", "../eval/splits/splits_final.json", "splits file")
	outPath := fl.String("out", "", "write one JSONL line per ruling: ruling, gold regime, router label")
	workers := fl.Int("workers", 8, "concurrent model calls")
	labelsPath := fl.String("labels", "../eval/data/labels.jsonl", "labels file")
	factsDir := fl.String("facts", "../data/corpus/facts", "facts directory")
	_ = fl.Parse(args)
	if *to >= "2026-01-01" {
		return errors.New("scope: the window must end before 2026 (the final split)")
	}
	var only map[string]bool
	if *splitName != "" {
		if *splitName == "final" {
			return errors.New("scope: tune the router on dev only; label final once with the frozen router (eval run -router)")
		}
		sp, err := corpus.LoadSplits(*splitsPath)
		if err != nil {
			return err
		}
		ids, err := sp.IDs(*splitName)
		if err != nil {
			return err
		}
		only = map[string]bool{}
		for _, id := range ids {
			only[id] = true
		}
		*perRegime = 1 << 30
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	model, err := newModel(ctx, cfg, logger)
	if err != nil {
		return err
	}
	rt := router.New(model, cfg.RouterModel, 2000)
	labels, err := corpus.LoadLabels(*labelsPath)
	if err != nil {
		return err
	}
	type item struct{ id, regime, facts string }
	count := map[string]int{}
	var items []item
	for _, l := range labels {
		if only != nil && !only[l.Ruling] {
			continue
		}
		if only == nil && (l.Date < *from || l.Date > *to || count[l.Regime] >= *perRegime) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(*factsDir, l.Ruling+".txt"))
		if err != nil {
			continue
		}
		count[l.Regime]++
		items = append(items, item{l.Ruling, l.Regime, string(b)})
	}
	type tally struct {
		n, out, errs int
		labels       map[string]int
	}
	res := map[string]*tally{}
	var outF *os.File
	if *outPath != "" {
		if outF, err = os.Create(*outPath); err != nil {
			return err
		}
		defer outF.Close()
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, *workers)
	for _, it := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func(it item) {
			defer wg.Done()
			defer func() { <-sem }()
			l, err := rt.Classify(ctx, it.facts)
			mu.Lock()
			defer mu.Unlock()
			t := res[it.regime]
			if t == nil {
				t = &tally{labels: map[string]int{}}
				res[it.regime] = t
			}
			t.n++
			if err != nil {
				t.errs++
				return
			}
			t.labels[l.Regime]++
			in, _ := l.Scope()
			if !in {
				t.out++
			}
			if outF != nil {
				b, _ := json.Marshal(map[string]any{"ruling": it.id, "gold_regime": it.regime, "in_scope": in, "router": l})
				_, _ = outF.Write(append(b, '\n'))
			}
		}(it)
	}
	wg.Wait()
	fmt.Printf("## Router scope check (%s to %s, up to %d per gold regime; router %s via %s)\n\n", *from, *to, *perRegime, cfg.RouterModel, cfg.LLMProvider)
	fmt.Println("| Gold regime (labels) | n | routed out of scope | router regime labels | errors |\n|---|---:|---|---|---:|")
	var names []string
	for k := range res {
		names = append(names, k)
	}
	slices.Sort(names)
	for _, k := range names {
		t := res[k]
		p, lo, hi := evalkit.Wilson(t.out, t.n-t.errs)
		fmt.Printf("| %s | %d | %d (%.0f%%, %.0f–%.0f) | %v | %d |\n", k, t.n, t.out, 100*p, 100*lo, 100*hi, t.labels, t.errs)
	}
	return ctx.Err()
}

func readScope(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var r struct {
			Ruling  string `json:"ruling"`
			InScope bool   `json:"in_scope"`
		}
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		out[r.Ruling] = r.InScope
	}
	return out, sc.Err()
}
