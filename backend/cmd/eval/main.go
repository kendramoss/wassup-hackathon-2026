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
//	eval plan   -fixture FILE [-search local|cross]
//	    Run the planner end to end on a fixture (engine + verifier) and save
//	    the result to eval/results/plan-<case>-<provider>.json.
//	eval predict -facts FILE -date YYYY-MM-DD [-id ID]
//	    One engine prediction on a facts file (debugging and spot checks).
//	eval flips  [-known-a] [-workers 6]
//	    Run the verifier on the development flip pairs (CBP natural
//	    experiments dated before 2026; the 2026 items are spent) and report
//	    precision, recall and specificity. Development-only by construction.
//	eval summary [-out internal/evalsummary/summary.json]
//	    Build the results-page numbers from the committed result files (plus
//	    the research baselines, with their source) for GET /v1/eval/summary.
//	eval multiscenario [-out ../eval/data/multi_scenario.json]
//	    Flag split rulings whose holding gives different origins for different
//	    scenarios (a mechanical rule over CBP's analysis text, applied to every
//	    item); report -exclude uses the list to score with and without them.
//	eval report -in FILE [-in FILE ...] [-exclude FILE]
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
	"regexp"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/cross"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalsummary"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/triage"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/verifier"
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
	case "plan":
		err = planCmd(os.Args[2:])
	case "predict":
		err = predictCmd(os.Args[2:])
	case "flips":
		err = flipsCmd(os.Args[2:])
	case "summary":
		err = summaryCmd(os.Args[2:])
	case "multiscenario":
		err = multiScenarioCmd(os.Args[2:])
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
	retrievalMode := fl.String("retrieval", "bm25", "bm25, hybrid (BM25 candidates re-ranked by embeddings) or vector")
	candidates := fl.Int("candidates", 30, "BM25 candidates re-ranked in hybrid mode")
	vectorsPath := fl.String("vectors", "../data/corpus/embeddings-768.bin", "embedding index (go run ./cmd/corpus embed)")
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
		bm := retrieval.NewBM25(store.Rulings(), engine.PrecedentText)
		retr = bm
		switch *retrievalMode {
		case "bm25":
		case "hybrid", "vector":
			vecs, err := retrieval.LoadVectors(*vectorsPath)
			if err != nil {
				return err
			}
			emb, ok := model.(llm.Embedder)
			if !ok {
				emb, err = newEmbedder(ctx, cfg)
				if err != nil {
					return err
				}
			}
			n := *candidates
			if *retrievalMode == "vector" {
				n = 0
			}
			retr = &retrieval.Hybrid{BM25: bm, Vectors: vecs, Embedder: emb, Candidates: n}
		default:
			return fmt.Errorf("-retrieval must be bm25, hybrid or vector")
		}
	}
	eng := engine.New(model, retr, engine.Options{Model: cfg.EngineModel, K: cfg.PrecedentK, MaxOutputTokens: cfg.MaxOutputTokens})
	var rt *router.Router
	if *useRouter {
		rt = router.New(model, cfg.RouterModel, 2000)
	}

	name := fmt.Sprintf("%s-%s-%s-%s-%s", *split, cfg.EngineModel, *cond, engine.PromptVersion, cfg.LLMProvider)
	if *retrievalMode != "bm25" {
		name = fmt.Sprintf("%s-%s-%s%d-%s-%s", *split, cfg.EngineModel, *retrievalMode, *candidates, engine.PromptVersion, cfg.LLMProvider)
	}
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
			row.Retrieval = *retrievalMode
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
	excludeFile := fl.String("exclude", "", "JSON object keyed by ruling (eval multiscenario output): score without these rulings")
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
		if *excludeFile != "" {
			b, err := os.ReadFile(*excludeFile)
			if err != nil {
				return err
			}
			var ex map[string]any
			if err := json.Unmarshal(b, &ex); err != nil {
				return err
			}
			kept := rows[:0]
			for _, r := range rows {
				if _, skip := ex[r.Ruling]; !skip {
					kept = append(kept, r)
				}
			}
			fmt.Fprintf(os.Stderr, "report: excluded %d multi-scenario rulings\n", len(rows)-len(kept))
			rows = kept
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
	confirmFinal := fl.Bool("confirm-final", false, "label the held-out final split once with the frozen router")
	workers := fl.Int("workers", 8, "concurrent model calls")
	labelsPath := fl.String("labels", "../eval/data/labels.jsonl", "labels file")
	factsDir := fl.String("facts", "../data/corpus/facts", "facts directory")
	_ = fl.Parse(args)
	if *to >= "2026-01-01" {
		return errors.New("scope: the window must end before 2026 (the final split)")
	}
	var only map[string]bool
	if *splitName != "" {
		if *splitName == "final" && !*confirmFinal {
			return errors.New("scope: tune the router on dev only; label final once with the frozen router (-confirm-final)")
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
	if *splitName != "" {
		f, err := os.OpenFile("../eval/results/RUNLOG.jsonl", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		outN := 0
		for _, t := range res {
			outN += t.out
		}
		b, _ := json.Marshal(map[string]any{"time": time.Now().Format(time.RFC3339), "run": "scope-" + *splitName + "-" + router.PromptVersion,
			"split": *splitName, "held_out": *splitName == "final", "labelled": len(items), "out_of_scope": outN,
			"router_model": cfg.RouterModel, "llm_provider": cfg.LLMProvider, "prompt_version": router.PromptVersion,
			"git_revision": revision(), "interrupted": ctx.Err() != nil})
		_, _ = f.Write(append(b, '\n'))
		f.Close()
	}
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

func planCmd(args []string) error {
	fl := flag.NewFlagSet("plan", flag.ExitOnError)
	fixture := fl.String("fixture", "testdata/heatgun_2022.json", "plan request fixture")
	searchMode := fl.String("search", "local", "verifier evidence: local (corpus, reproducible) or cross (live CROSS with toDate)")
	labelsPath := fl.String("labels", "../eval/data/labels.jsonl", "labels file")
	factsDir := fl.String("facts", "../data/corpus/facts", "facts directory")
	rawDir := fl.String("raw", "../data/corpus/raw", "raw CROSS JSON directory")
	resultsDir := fl.String("results", "../eval/results", "results directory")
	_ = fl.Parse(args)

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
	req, err := planner.LoadRequest(*fixture)
	if err != nil {
		return err
	}
	labels, err := corpus.LoadLabels(*labelsPath)
	if err != nil {
		return err
	}
	store, err := corpus.Load(labels, *factsDir)
	if err != nil {
		return err
	}
	eng := engine.New(model, retrieval.NewBM25(store.Rulings(), engine.PrecedentText),
		engine.Options{Model: cfg.EngineModel, K: cfg.PrecedentK, MaxOutputTokens: cfg.MaxOutputTokens})
	var searcher verifier.Searcher
	switch *searchMode {
	case "local":
		if searcher, err = verifier.NewLocal(labels, *factsDir, *rawDir); err != nil {
			return err
		}
	case "cross":
		searcher = verifier.CROSS{Client: cross.NewHTTPClient("origin-proof (Google Cloud AI Builder Cup 2026 prototype)")}
	default:
		return fmt.Errorf("-search must be local or cross")
	}
	ver := verifier.New(model, searcher, verifier.Options{Model: cfg.VerifierModel, MaxToolCalls: cfg.VerifierMaxToolCalls, MaxOutputTokens: 6000})
	tmpls, err := planner.LoadTemplates()
	if err != nil {
		return err
	}
	pl := planner.New(tmpls, eng, ver, planner.Options{PlanConfidence: cfg.PlanConfidence, Concurrency: cfg.PlannerConcurrency})
	res, err := pl.Plan(ctx, req)
	if err != nil {
		return err
	}
	cost := evalkit.Cost(evalkit.PromoPrices, cfg.EngineModel, res.Stats.EngineInTok, res.Stats.EngineOutTok) +
		evalkit.Cost(evalkit.PromoPrices, cfg.VerifierModel, res.Stats.VerifierInTok, res.Stats.VerifierOutTok)
	costList := evalkit.Cost(evalkit.ListPrices, cfg.EngineModel, res.Stats.EngineInTok, res.Stats.EngineOutTok) +
		evalkit.Cost(evalkit.ListPrices, cfg.VerifierModel, res.Stats.VerifierInTok, res.Stats.VerifierOutTok)
	out := struct {
		planner.Result
		Provider    string  `json:"llm_provider"`
		Search      string  `json:"verifier_search"`
		CostUSD     float64 `json:"cost_usd_promo"`
		CostUSDList float64 `json:"cost_usd_list"`
		Revision    string  `json:"git_revision"`
		Time        string  `json:"time"`
	}{res, cfg.LLMProvider, *searchMode, cost, costList, revision(), time.Now().Format(time.RFC3339)}
	path := filepath.Join(*resultsDir, fmt.Sprintf("plan-%s-%s-%s.json", req.CaseID, *searchMode, cfg.LLMProvider))
	b, _ := json.MarshalIndent(out, "", "  ")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return err
	}
	fmt.Printf("## Plan %s → %s (%s, verifier search %s)\n\n", req.CaseID, res.Target, cfg.LLMProvider, *searchMode)
	if cfg.LLMProvider == config.ProviderLiteLLM {
		fmt.Println("**Proxy, development only.**")
	}
	b0 := res.Baseline.Prediction
	fmt.Printf("- Current setup: engine predicts **%s** (confidence %.2f), cited %v\n", b0.Origin, b0.Confidence, b0.CitedRulings)
	fmt.Printf("- Shown (precedent-supported, verifier confidence ≥ %.2f):\n", cfg.PlanConfidence)
	for _, p := range res.Plans {
		fmt.Printf("  - ✓ %s — engine %s %.2f; supporting %s, counter %s, verifier conf %.2f; duty effect $%.0f/yr, added cost $%.0f/yr (illustrative)\n",
			p.Description, p.Prediction.Origin, p.Prediction.Confidence, p.Verdict.SupportingRuling, p.Verdict.CounterRuling, p.Verdict.Confidence, p.DutyEffectYear, p.AddedCostYear)
	}
	fmt.Println("- Rejected:")
	for _, p := range res.Rejected {
		extra := ""
		if p.Verdict != nil {
			extra = fmt.Sprintf(" (verifier: supported=%t conf %.2f, supporting %q, counter %q)", p.Verdict.PrecedentSupported, p.Verdict.Confidence, p.Verdict.SupportingRuling, p.Verdict.CounterRuling)
		}
		fmt.Printf("  - ✗ %s — %s%s\n", p.Description, p.RejectionReason, extra)
	}
	if res.Backtest != nil {
		fmt.Printf("- Backtest vs %s: plan moved %v; actual change also moved %v\n", res.Backtest.Source, res.Backtest.MovedByPlan, res.Backtest.NotMovedByPlan)
	}
	s := res.Stats
	fmt.Printf("- Full-plan cost: %d moves, %d engine calls, %d verifier runs (%d model calls, %d tool calls), %d in / %d out tokens, $%.3f promo / $%.3f list, %.0f s wall\n",
		s.MovesSearched, s.EngineCalls, s.VerifierRuns, s.VerifierModel, s.ToolCalls, s.InputTokens, s.OutputTokens, cost, costList, s.WallSeconds)
	fmt.Println("- Saved:", path)
	return nil
}

func predictCmd(args []string) error {
	fl := flag.NewFlagSet("predict", flag.ExitOnError)
	factsPath := fl.String("facts", "", "facts text file")
	dateStr := fl.String("date", "", "case date (YYYY-MM-DD); precedents are strictly earlier")
	id := fl.String("id", "adhoc", "case ID (excluded from its own precedents)")
	labelsPath := fl.String("labels", "../eval/data/labels.jsonl", "labels file")
	factsDir := fl.String("corpus", "../data/corpus/facts", "facts directory of the precedent corpus")
	_ = fl.Parse(args)
	date, err := time.Parse(time.DateOnly, *dateStr)
	if err != nil {
		return fmt.Errorf("-date: %w", err)
	}
	facts, err := os.ReadFile(*factsPath)
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	model, err := newModel(ctx, cfg, slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
	if err != nil {
		return err
	}
	labels, err := corpus.LoadLabels(*labelsPath)
	if err != nil {
		return err
	}
	store, err := corpus.Load(labels, *factsDir)
	if err != nil {
		return err
	}
	eng := engine.New(model, retrieval.NewBM25(store.Rulings(), engine.PrecedentText),
		engine.Options{Model: cfg.EngineModel, K: cfg.PrecedentK, MaxOutputTokens: cfg.MaxOutputTokens})
	res, err := eng.Predict(ctx, domain.Case{ID: *id, Date: date, Facts: string(facts)})
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(map[string]any{"prediction": res.Prediction, "flip": res.Flip}, "", "  ")
	fmt.Println(string(b))
	return nil
}

type flipItem struct {
	ID            string   `json:"id"`
	Rulings       []string `json:"rulings"`
	Date          string   `json:"date"`
	Difference    string   `json:"difference"`
	OriginChanged bool     `json:"origin_changed"`
	VariantA      struct {
		Facts  string `json:"facts"`
		Origin string `json:"origin"`
	} `json:"variant_a"`
	VariantB struct {
		Facts  string `json:"facts"`
		Origin string `json:"origin"`
	} `json:"variant_b"`
}

func flipsCmd(args []string) error {
	fl := flag.NewFlagSet("flips", flag.ExitOnError)
	knownA := fl.Bool("known-a", false, "state CBP's origin for variant A in the task (verifier-v1+known-a)")
	workers := fl.Int("workers", 6, "concurrent verifier runs")
	limit := fl.Int("limit", 0, "first N items only")
	flipsPath := fl.String("flips", "../eval/data/flips.jsonl", "flip pairs")
	labelsPath := fl.String("labels", "../eval/data/labels.jsonl", "labels file")
	factsDir := fl.String("facts", "../data/corpus/facts", "facts directory")
	rawDir := fl.String("raw", "../data/corpus/raw", "raw CROSS JSON directory")
	resultsDir := fl.String("results", "../eval/results", "results directory")
	_ = fl.Parse(args)
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	model, err := newModel(ctx, cfg, slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
	if err != nil {
		return err
	}
	labels, err := corpus.LoadLabels(*labelsPath)
	if err != nil {
		return err
	}
	dates := map[string]string{}
	for _, l := range labels {
		dates[l.Ruling] = l.Date
	}
	searcher, err := verifier.NewLocal(labels, *factsDir, *rawDir)
	if err != nil {
		return err
	}
	ver := verifier.New(model, searcher, verifier.Options{Model: cfg.VerifierModel, MaxToolCalls: cfg.VerifierMaxToolCalls, MaxOutputTokens: 6000})

	f, err := os.Open(*flipsPath)
	if err != nil {
		return err
	}
	var items []flipItem
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var it flipItem
		if err := json.Unmarshal(sc.Bytes(), &it); err != nil {
			f.Close()
			return err
		}
		if it.Date < "2026-01-01" { // DEV ONLY: the 2026 flip items are spent
			items = append(items, it)
		}
	}
	f.Close()
	if *limit > 0 && *limit < len(items) {
		items = items[:*limit]
	}
	mode := "unknown-a"
	if *knownA {
		mode = "known-a"
	}
	outPath := filepath.Join(*resultsDir, fmt.Sprintf("flips-dev-verifier-%s-%s.jsonl", mode, cfg.LLMProvider))
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	type row struct {
		ID                 string   `json:"id"`
		OriginChanged      bool     `json:"origin_changed"`
		ChangesOrigin      bool     `json:"pred_changes_origin"`
		PrecedentSupported bool     `json:"precedent_supported"`
		Supporting         string   `json:"supporting_ruling"`
		Counter            string   `json:"counter_ruling"`
		Dropped            []string `json:"dropped_unseen_rulings,omitempty"`
		OriginBOK          bool     `json:"origin_b_ok"`
		Confidence         float64  `json:"confidence"`
		ToolCalls          int      `json:"tool_calls"`
		ModelCalls         int      `json:"model_calls"`
		CostUSD            float64  `json:"cost_usd"`
		Error              string   `json:"error,omitempty"`
		Mode               string   `json:"mode"`
		ModelID            string   `json:"model_id"`
		Provider           string   `json:"llm_provider"`
	}
	var mu sync.Mutex
	var rows []row
	sem := make(chan struct{}, *workers)
	var wg sync.WaitGroup
	for _, it := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func(it flipItem) {
			defer wg.Done()
			defer func() { <-sem }()
			before := it.Date
			for _, r := range it.Rulings {
				if d, ok := dates[r]; ok && d < before {
					before = d
				}
			}
			bt, _ := time.Parse(time.DateOnly, before)
			task := verifier.Task{FactsA: it.VariantA.Facts, FactsB: it.VariantB.Facts, Before: bt, Exclude: it.Rulings}
			if *knownA {
				task.KnownOriginA = it.VariantA.Origin
			}
			res, err := ver.Verify(ctx, task)
			rw := row{ID: it.ID, OriginChanged: it.OriginChanged, Mode: mode, ModelID: cfg.VerifierModel, Provider: cfg.LLMProvider}
			if err != nil {
				rw.Error = err.Error()
			} else {
				v := res.Verdict
				rw.ChangesOrigin, rw.PrecedentSupported = res.ChangesOrigin, v.PrecedentSupported
				rw.Supporting, rw.Counter, rw.Dropped, rw.Confidence = v.SupportingRuling, v.CounterRuling, res.Dropped, v.Confidence
				for _, o := range strings.Split(it.VariantB.Origin, "|") {
					if domain.NormalizeCountry(strings.TrimSpace(o)) == res.OriginB {
						rw.OriginBOK = true
					}
				}
				rw.ToolCalls, rw.ModelCalls = len(res.Steps), res.ModelCalls
				rw.CostUSD = evalkit.Cost(evalkit.PromoPrices, cfg.VerifierModel, res.Usage.InputTokens, res.Usage.OutputTokens)
			}
			b, _ := json.Marshal(rw)
			mu.Lock()
			defer mu.Unlock()
			rows = append(rows, rw)
			_, _ = out.Write(append(b, '\n'))
		}(it)
	}
	wg.Wait()

	var tp, fp, fn, tn, stp, sfp, errs int
	var cost float64
	for _, r := range rows {
		if r.Error != "" {
			errs++
			continue
		}
		cost += r.CostUSD
		switch {
		case r.ChangesOrigin && r.OriginChanged:
			tp++
		case r.ChangesOrigin && !r.OriginChanged:
			fp++
		case !r.ChangesOrigin && r.OriginChanged:
			fn++
		default:
			tn++
		}
		if r.PrecedentSupported {
			if r.OriginChanged {
				stp++
			} else {
				sfp++
			}
		}
	}
	fmtP := func(k, n int) string {
		p, lo, hi := evalkit.Wilson(k, n)
		return fmt.Sprintf("%d/%d = %.0f%% (%.0f–%.0f)", k, n, 100*p, 100*lo, 100*hi)
	}
	fmt.Printf("## Verifier on development flip pairs (%s, %s) — DEVELOPMENT SET ONLY\n\n", mode, cfg.LLMProvider)
	if cfg.LLMProvider == config.ProviderLiteLLM {
		fmt.Println("**Proxy, development only.**")
	}
	n := len(rows) - errs
	fmt.Printf("- Items: %d (pre-2026; %d failed runs)\n", len(rows), errs)
	fmt.Printf("- Precision when it says the move changes origin: %s\n", fmtP(tp, tp+fp))
	fmt.Printf("- Recall (real changes caught): %s\n", fmtP(tp, tp+fn))
	fmt.Printf("- Specificity (no-change pairs correctly rejected): %s\n", fmtP(tn, tn+fp))
	fmt.Printf("- Precedent-supported (change + cited supporting ruling it actually read): correct %s\n", fmtP(stp, stp+sfp))
	if n > 0 {
		fmt.Printf("- Cost per verification: $%.3f (promo)\n", cost/float64(n))
	}
	fmt.Println("- Saved:", outPath)
	return nil
}

func summaryCmd(args []string) error {
	fl := flag.NewFlagSet("summary", flag.ExitOnError)
	resultsDir := fl.String("results", "../eval/results", "results directory")
	outPath := fl.String("out", "internal/evalsummary/summary.json", "output file (embedded in the server)")
	_ = fl.Parse(args)
	type item = evalsummary.Item
	var items []item
	pct := func(k, n int) (float64, float64, float64) { return evalkit.Wilson(k, n) }
	r4 := func(x float64) float64 { return float64(int(x*10000+0.5)) / 10000 }

	addRun := func(file, split, prefix, label string, scopeFile string) error {
		rows, err := readRows(filepath.Join(*resultsDir, file), nil)
		if err != nil {
			return err
		}
		if scopeFile != "" {
			sc, err := readScope(filepath.Join(*resultsDir, scopeFile))
			if err != nil {
				return err
			}
			for i := range rows {
				in, ok := sc[rows[i].Ruling]
				if !ok {
					in = true
				}
				rows[i].InScope = in
				rows[i].TriageRoute = "needs_review"
				if in && rows[i].Error == "" && rows[i].Confidence >= 0.95 {
					rows[i].TriageRoute = "auto"
				}
			}
		}
		s := evalkit.Summarize(rows)
		src := "eval/results/" + file
		items = append(items, item{Key: prefix + "_accuracy", Label: label + " accuracy", Value: r4(s.Acc), CILow: r4(s.Lo), CIHigh: r4(s.Hi), N: s.N, Split: split, Source: src})
		p, lo, hi := pct(s.AtLeast95.K, s.AtLeast95.N)
		items = append(items, item{Key: prefix + "_conf95_accuracy", Label: label + ": accuracy when confidence ≥ 0.95", Value: r4(p), CILow: r4(lo), CIHigh: r4(hi), N: s.AtLeast95.N, Split: split, Source: src})
		items = append(items, item{Key: prefix + "_conf95_coverage", Label: label + ": share of cases with confidence ≥ 0.95", Value: r4(float64(s.AtLeast95.N) / float64(s.N)), N: s.N, Split: split, Source: src})
		if scopeFile != "" {
			p, lo, hi := pct(s.FastK, s.FastN)
			items = append(items, item{Key: prefix + "_fastpath_accuracy", Label: label + ": fast path (confidence ≥ 0.95 and in scope) accuracy", Value: r4(p), CILow: r4(lo), CIHigh: r4(hi), N: s.FastN, Split: split, Source: src + " + eval/results/" + scopeFile})
			items = append(items, item{Key: prefix + "_fastpath_coverage", Label: label + ": fast path coverage", Value: r4(float64(s.FastN) / float64(s.N)), N: s.N, Split: split, Source: src + " + eval/results/" + scopeFile})
		}
		items = append(items, item{Key: prefix + "_cost_per_case_usd", Label: label + ": cost per prediction (USD, promo price)", Value: r4(s.CostPerCase), N: s.N, Split: split, Source: src})
		items = append(items, item{Key: prefix + "_latency_p50_s", Label: label + ": latency p50 (s, via proxy)", Value: float64(s.LatencyP50) / 1000, Split: split, Source: src})
		items = append(items, item{Key: prefix + "_latency_p95_s", Label: label + ": latency p95 (s, via proxy)", Value: float64(s.P95) / 1000, Split: split, Source: src})
		return nil
	}
	if err := addRun("final-gemini-4-argon-prec-engine-v1-litellm.jsonl", "final", "final", "Held-out (377 rulings, Jan–Sep 2026)", "scope-final-router-v2.jsonl"); err != nil {
		return err
	}
	if err := addRun("dev-gemini-4-argon-prec-engine-v1-litellm.jsonl", "dev", "dev", "Development (469 rulings, Jul–Dec 2025)", "scope-dev-router-v2.jsonl"); err != nil {
		return err
	}
	for i := range items {
		if items[i].Split == "dev" {
			items[i].DevelopmentOnly = true
		}
	}
	const v2 = "hackathon-research/experiments/origin-proof/RESULTS-v2.md (research run, Python, LiteLLM proxy)"
	items = append(items,
		item{Key: "research_final_accuracy", Label: "Research run: Gemini 4 Argon + dated precedents", Value: 0.891, CILow: 0.856, CIHigh: 0.919, N: 377, Split: "final", Source: v2},
		item{Key: "baseline_most_mentioned", Label: "Baseline: most-mentioned country", Value: 0.605, CILow: 0.555, CIHigh: 0.653, N: 377, Split: "final", Source: v2},
		item{Key: "baseline_final_assembly", Label: "Baseline: final-assembly country", Value: 0.406, CILow: 0.358, CIHigh: 0.456, N: 377, Split: "final", Source: v2},
		item{Key: "baseline_last_mentioned", Label: "Baseline: last country mentioned", Value: 0.398, CILow: 0.350, CIHigh: 0.448, N: 377, Split: "final", Source: v2},
		item{Key: "baseline_majority", Label: "Baseline: majority country (Vietnam)", Value: 0.122, CILow: 0.093, CIHigh: 0.159, N: 377, Split: "final", Source: v2},
		item{Key: "research_argon_facts_only", Label: "Research: Gemini 4 Argon, facts only (no precedents)", Value: 0.859, CILow: 0.821, CIHigh: 0.891, N: 377, Split: "final", Source: v2},
		item{Key: "change_detection_heldout", Label: "Change detection from scratch, held-out 2026 pairs (weak)", Value: 15.0 / 22, CILow: 0.47, CIHigh: 0.84, N: 22, Split: "final", Source: v2},
		item{Key: "change_detection_always_change_baseline", Label: "Baseline: always predict a change (same 22 pairs)", Value: 14.0 / 22, N: 22, Split: "final", Source: v2},
		item{Key: "change_detection_balanced_accuracy", Label: "Change detection balanced accuracy (same 22 pairs)", Value: 0.616, N: 22, Split: "final", Source: v2},
	)
	for _, mode := range []string{"unknown-a", "known-a"} {
		f, err := os.Open(filepath.Join(*resultsDir, "flips-dev-verifier-"+mode+"-litellm.jsonl"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "summary: skip verifier %s: %v\n", mode, err)
			continue
		}
		var tp, fp, fn, tn int
		var cost float64
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			var r struct {
				OriginChanged bool    `json:"origin_changed"`
				Changes       bool    `json:"pred_changes_origin"`
				Error         string  `json:"error"`
				Cost          float64 `json:"cost_usd"`
			}
			if json.Unmarshal(sc.Bytes(), &r) != nil || r.Error != "" {
				continue
			}
			cost += r.Cost
			switch {
			case r.Changes && r.OriginChanged:
				tp++
			case r.Changes:
				fp++
			case r.OriginChanged:
				fn++
			default:
				tn++
			}
		}
		f.Close()
		src := "eval/results/flips-dev-verifier-" + mode + "-litellm.jsonl"
		lbl := "Move verifier (development set"
		if mode == "known-a" {
			lbl += ", CBP origin of current setup given"
		}
		lbl += ")"
		p, lo, hi := pct(tp, tp+fp)
		items = append(items, item{Key: "verifier_" + mode + "_precision", Label: lbl + ": correct when it says a move changes origin", Value: r4(p), CILow: r4(lo), CIHigh: r4(hi), N: tp + fp, Split: "dev", DevelopmentOnly: true, Source: src})
		p, lo, hi = pct(tp, tp+fn)
		items = append(items, item{Key: "verifier_" + mode + "_recall", Label: lbl + ": real changes caught", Value: r4(p), CILow: r4(lo), CIHigh: r4(hi), N: tp + fn, Split: "dev", DevelopmentOnly: true, Source: src})
		p, lo, hi = pct(tn, tn+fp)
		items = append(items, item{Key: "verifier_" + mode + "_specificity", Label: lbl + ": no-change moves rejected", Value: r4(p), CILow: r4(lo), CIHigh: r4(hi), N: tn + fp, Split: "dev", DevelopmentOnly: true, Source: src})
		if n := tp + fp + fn + tn; n > 0 {
			items = append(items, item{Key: "verifier_" + mode + "_cost_usd", Label: lbl + ": cost per check (USD, promo)", Value: r4(cost / float64(n)), N: n, Split: "dev", DevelopmentOnly: true, Source: src})
		}
	}
	if b, err := os.ReadFile(filepath.Join(*resultsDir, "plan-heatgun-2022-local-litellm.json")); err == nil {
		var p struct {
			Cost  float64       `json:"cost_usd_promo"`
			Stats planner.Stats `json:"stats"`
		}
		if json.Unmarshal(b, &p) == nil {
			src := "eval/results/plan-heatgun-2022-local-litellm.json"
			items = append(items,
				item{Key: "plan_heatgun_cost_usd", Label: "Full plan, heat gun demo: cost (USD, promo)", Value: r4(p.Cost), Split: "demo", DevelopmentOnly: true, Source: src},
				item{Key: "plan_heatgun_wall_s", Label: "Full plan, heat gun demo: wall time (s, via proxy)", Value: float64(int(p.Stats.WallSeconds + 0.5)), Split: "demo", DevelopmentOnly: true, Source: src})
		}
	}
	out := evalsummary.Summary{GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Status: "proxy, development only: produced through the LiteLLM proxy; official numbers come from the Vertex AI re-run", Items: items}
	b, _ := json.MarshalIndent(out, "", "  ")
	if err := os.WriteFile(*outPath, append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "summary: %d items -> %s\n", len(items), *outPath)
	return nil
}

var (
	scenarioSentence = regexp.MustCompile(`(?i)scenario|configuration|option [12AB]|process [12AB]`)
	originWord       = regexp.MustCompile(`(?i)origin`)
	heldCountry      = regexp.MustCompile(`\b(?:is|be|are|remains)\s+(?:the\s+)?(China|Thailand|Vietnam|Malaysia|Taiwan|South Korea|Korea|Indonesia|Cambodia|India|Mexico|Japan|Germany|United States|Philippines|Canada|Italy|Turkey|Bangladesh|Sri Lanka|Hong Kong|Singapore|Israel|Spain|France|Poland|Brazil|Laos|Pakistan|United Kingdom|Ireland|Peru|Chile|Costa Rica|Netherlands|Switzerland|Sweden|Austria|Belgium|Czech Republic|Hungary|Romania|Egypt|Morocco|Tunisia|Jordan|United Arab Emirates|Australia|Colombia|Ecuador|Guatemala|Honduras|Dominican Republic|Myanmar|Nepal)\b`)
	sentenceEnd      = regexp.MustCompile(`\.\s`)
)

func multiScenarioCmd(args []string) error {
	fl := flag.NewFlagSet("multiscenario", flag.ExitOnError)
	splitsPath := fl.String("splits", "../eval/splits/splits_final.json", "splits file")
	rawDir := fl.String("raw", "../data/corpus/raw", "raw CROSS JSON directory")
	outPath := fl.String("out", "../eval/data/multi_scenario.json", "output")
	_ = fl.Parse(args)
	sp, err := corpus.LoadSplits(*splitsPath)
	if err != nil {
		return err
	}
	out := map[string]map[string]any{}
	for _, split := range []string{"dev", "final"} {
		ids, _ := sp.IDs(split)
		for _, id := range ids {
			b, err := os.ReadFile(filepath.Join(*rawDir, id+".json"))
			if err != nil {
				return err
			}
			var r cross.Ruling
			if err := json.Unmarshal(b, &r); err != nil {
				return err
			}
			_, meta := corpus.SplitFacts(r.Text)
			t := []rune(strings.Join(strings.Fields(r.Text), " "))
			rest := ""
			if meta.End < len(t) {
				rest = string(t[meta.End:])
			}
			held := map[string]bool{}
			for _, s := range sentenceEnd.Split(rest, -1) {
				if !scenarioSentence.MatchString(s) || !originWord.MatchString(s) {
					continue
				}
				for _, m := range heldCountry.FindAllStringSubmatch(s, -1) {
					c := m[1]
					if c == "Korea" {
						c = "South Korea"
					}
					held[c] = true
				}
			}
			if len(held) >= 2 {
				var cs []string
				for c := range held {
					cs = append(cs, c)
				}
				slices.Sort(cs)
				out[id] = map[string]any{"split": split, "held": cs}
			}
		}
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	if err := os.WriteFile(*outPath, append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "multiscenario: %d rulings flagged -> %s\n", len(out), *outPath)
	return nil
}

func newEmbedder(ctx context.Context, cfg config.Config) (llm.Embedder, error) {
	switch cfg.LLMProvider {
	case config.ProviderLiteLLM:
		return llm.NewLiteLLM(cfg.LiteLLMBaseURL, cfg.LiteLLMAPIKey, cfg.LLMTimeout), nil
	case config.ProviderVertex:
		return llm.NewVertex(ctx, llm.VertexOptions{Project: cfg.GCPProject, Location: cfg.GCPLocation, Timeout: cfg.LLMTimeout})
	}
	return nil, fmt.Errorf("no embedder for provider %q", cfg.LLMProvider)
}
