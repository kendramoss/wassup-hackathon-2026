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
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
)

// Engines a router label sends a case to (D230). "unclear" and
// substantial_transformation both go to the general engine.
const engineGeneral = "general"

// routedEngine is the engine the product runs for a router regime label
// (httpapi dispatch with every specialist engine on, as in production).
func routedEngine(regime string) string {
	switch regime {
	case router.RegimeTextile, router.RegimeUSMCA, router.RegimePharma:
		return regime
	}
	return engineGeneral
}

// routerSplits is eval/splits/splits_router.json (build_router_split.py).
type routerSplits struct {
	FrozenAt string            `json:"frozen_at"`
	Dev      []string          `json:"dev"`
	Final    []string          `json:"final"`
	Final2   []string          `json:"final2"`
	Gold     map[string]string `json:"gold"`
}

func (s routerSplits) ids(name string) ([]string, error) {
	switch name {
	case "dev":
		return s.Dev, nil
	case "final":
		return s.Final, nil
	case "final2":
		return s.Final2, nil
	}
	return nil, fmt.Errorf("route: unknown split %q (want dev, final or final2)", name)
}

// routeRow is one line of a route result file. Correct and Error let
// `eval compare` read it like an engine result.
type routeRow struct {
	Ruling     string        `json:"ruling"`
	Split      string        `json:"split"`
	GoldEngine string        `json:"gold_engine"`
	Engine     string        `json:"engine,omitempty"`
	Correct    bool          `json:"correct"`
	Router     *router.Label `json:"router,omitempty"`
	CostUSD    float64       `json:"cost_usd,omitempty"`
	Error      string        `json:"error,omitempty"`
}

// routeCmd runs one router version on a router split and scores the engine
// each case is sent to against the gold engine (D230).
func routeCmd(args []string) error {
	fl := flag.NewFlagSet("route", flag.ExitOnError)
	split := fl.String("split", "dev", "dev, final or final2 (final and final2 are held out: -confirm-final, once per router candidate)")
	splitsPath := fl.String("splits", "../eval/splits/splits_router.json", "router splits file (eval/splits/build_router_split.py)")
	factsDirs := fl.String("facts", "../data/corpus/facts,../eval/data/facts_final2", "facts directories, searched in order")
	version := fl.String("prompt", router.PromptVersion, "router prompt version")
	confirmFinal := fl.Bool("confirm-final", false, "required to score a held-out split")
	workers := fl.Int("workers", 8, "concurrent model calls")
	resultsDir := fl.String("results", "../eval/results", "results directory")
	_ = fl.Parse(args)
	if (*split == "final" || *split == "final2") && !*confirmFinal {
		return errors.New("route: tune the router on dev only; score a held-out split once with -confirm-final")
	}
	b, err := os.ReadFile(*splitsPath)
	if err != nil {
		return err
	}
	var sp routerSplits
	if err := json.Unmarshal(b, &sp); err != nil {
		return fmt.Errorf("route: %s: %w", *splitsPath, err)
	}
	ids, err := sp.ids(*split)
	if err != nil {
		return err
	}
	cfg, err := loadConfig()
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
	rt, err := router.NewVersion(model, cfg.RouterModel, 2000, *version)
	if err != nil {
		return err
	}
	name := fmt.Sprintf("route-%s-%s-%s-%s", *split, *version, cfg.RouterModel, cfg.LLMProvider)
	outPath := filepath.Join(*resultsDir, name+".jsonl")
	done := map[string]bool{}
	if prev, err := readRouteRows(outPath); err == nil {
		if *split != "dev" && len(prev) > 0 {
			complete := true
			for _, r := range prev {
				if r.Error != "" {
					complete = false
				}
			}
			if complete && len(prev) >= len(ids) {
				return fmt.Errorf("route: %s is already scored on %s (%s); a held-out split is scored once", *version, *split, outPath)
			}
		}
		for _, r := range prev {
			if r.Error == "" {
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
	dirs := strings.Split(*factsDirs, ",")
	readFacts := func(id string) (string, error) {
		for _, d := range dirs {
			if b, err := os.ReadFile(filepath.Join(d, id+".txt")); err == nil {
				return string(b), nil
			}
		}
		return "", fmt.Errorf("no facts file for %s in %v", id, dirs)
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, *workers)
	for _, id := range ids {
		if done[id] {
			continue
		}
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(id string) {
			defer wg.Done()
			defer func() { <-sem }()
			row := routeRow{Ruling: id, Split: *split, GoldEngine: sp.Gold[id]}
			facts, err := readFacts(id)
			if err == nil {
				tally := &llm.CostTally{}
				var l router.Label
				l, err = rt.Classify(llm.ContextWithCostTally(ctx, tally), facts)
				if err == nil {
					row.Router, row.Engine, row.CostUSD = &l, routedEngine(l.Regime), tally.USD()
					row.Correct = row.Engine == row.GoldEngine
				}
			}
			if err != nil {
				row.Error = err.Error()
			}
			b, _ := json.Marshal(row)
			mu.Lock()
			_, _ = out.Write(append(b, '\n'))
			mu.Unlock()
		}(id)
	}
	wg.Wait()
	rows, err := readRouteRows(outPath)
	if err != nil {
		return err
	}
	s := summarizeRoutes(rows)
	f, err := os.OpenFile(filepath.Join(*resultsDir, "RUNLOG.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	lb, _ := json.Marshal(map[string]any{"time": time.Now().Format(time.RFC3339), "run": name, "split": *split, "held_out": *split != "dev",
		"n": s.n, "correct": s.k, "errors": s.errs, "cost_usd": s.cost, "router_model": cfg.RouterModel, "llm_provider": cfg.LLMProvider,
		"prompt_version": *version, "splits_frozen_at": sp.FrozenAt, "git_revision": revision(), "interrupted": ctx.Err() != nil})
	_, _ = f.Write(append(lb, '\n'))
	f.Close()
	fmt.Printf("## Router %s on %s (%s via %s): %s\n\n", *version, *split, cfg.RouterModel, cfg.LLMProvider, outPath)
	printRouteSummary(s)
	return ctx.Err()
}

func readRouteRows(path string) ([]routeRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	last := map[string]routeRow{}
	var order []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var r routeRow
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if _, seen := last[r.Ruling]; !seen {
			order = append(order, r.Ruling)
		}
		last[r.Ruling] = r
	}
	rows := make([]routeRow, 0, len(order))
	for _, id := range order {
		rows = append(rows, last[id])
	}
	return rows, sc.Err()
}

var goldEngines = []string{engineGeneral, router.RegimePharma, router.RegimeUSMCA, router.RegimeTextile}

type routeSummary struct {
	n, k, errs int
	cost       float64
	latencyMS  int64
	byGold     map[string]map[string]int // gold engine -> routed engine -> count
}

func summarizeRoutes(rows []routeRow) routeSummary {
	s := routeSummary{byGold: map[string]map[string]int{}}
	for _, r := range rows {
		if r.Error != "" {
			s.errs++
			continue
		}
		s.n++
		if r.Correct {
			s.k++
		}
		s.cost += r.CostUSD
		if r.Router != nil {
			s.latencyMS += r.Router.LatencyMS
		}
		if s.byGold[r.GoldEngine] == nil {
			s.byGold[r.GoldEngine] = map[string]int{}
		}
		s.byGold[r.GoldEngine][r.Engine]++
	}
	return s
}

func printRouteSummary(s routeSummary) {
	fmt.Println("| Gold engine | n | sent to the right engine | general | pharma_api | usmca_part102 | textile_102_21 |")
	fmt.Println("|---|---:|---|---:|---:|---:|---:|")
	golds := slices.Clone(goldEngines)
	for g := range s.byGold {
		if !slices.Contains(golds, g) {
			golds = append(golds, g)
		}
	}
	var spec, specN int
	for _, g := range golds {
		m := s.byGold[g]
		n := 0
		for _, c := range m {
			n += c
		}
		if n == 0 {
			continue
		}
		if g != engineGeneral {
			spec += m[g]
			specN += n
		}
		p, lo, hi := evalkit.Wilson(m[g], n)
		fmt.Printf("| %s | %d | %d (%.1f%%, %.1f–%.1f) | %d | %d | %d | %d |\n", g, n, m[g], 100*p, 100*lo, 100*hi,
			m[engineGeneral], m[router.RegimePharma], m[router.RegimeUSMCA], m[router.RegimeTextile])
	}
	p, lo, hi := evalkit.Wilson(s.k, s.n)
	ps, los, his := evalkit.Wilson(spec, specN)
	mean := func(x float64) float64 {
		if s.n == 0 {
			return 0
		}
		return x / float64(s.n)
	}
	fmt.Printf("\nAll: %d/%d sent to the right engine (%.1f%%, %.1f–%.1f); specialist cases %d/%d (%.1f%%, %.1f–%.1f); errors %d; cost $%.4f ($%.6f per case); mean latency %.0f ms\n",
		s.k, s.n, 100*p, 100*lo, 100*hi, spec, specN, 100*ps, 100*los, 100*his, s.errs, s.cost, mean(s.cost), mean(float64(s.latencyMS)))
}

// routeCompareCmd pairs two route result files on shared rulings: overall
// and per gold engine, fixed and broken counts with an exact McNemar test.
func routeCompareCmd(args []string) error {
	fl := flag.NewFlagSet("routecompare", flag.ExitOnError)
	a := fl.String("a", "", "baseline route file")
	b := fl.String("b", "", "variant route file")
	_ = fl.Parse(args)
	ra, err := readRouteRows(*a)
	if err != nil {
		return err
	}
	rb, err := readRouteRows(*b)
	if err != nil {
		return err
	}
	byID := map[string]routeRow{}
	for _, r := range ra {
		byID[r.Ruling] = r
	}
	type cell struct{ n, ka, kb, fixed, broken int }
	cells := map[string]*cell{}
	add := func(key string, x, y routeRow) {
		c := cells[key]
		if c == nil {
			c = &cell{}
			cells[key] = c
		}
		okx, oky := x.Correct && x.Error == "", y.Correct && y.Error == ""
		c.n++
		if okx {
			c.ka++
		}
		if oky {
			c.kb++
		}
		switch {
		case oky && !okx:
			c.fixed++
		case okx && !oky:
			c.broken++
		}
	}
	var costA, costB float64
	for _, y := range rb {
		x, ok := byID[y.Ruling]
		if !ok {
			continue
		}
		costA += x.CostUSD
		costB += y.CostUSD
		add("all", x, y)
		add(y.GoldEngine, x, y)
		if y.GoldEngine != engineGeneral {
			add("specialist (pharma, Part 102, textile)", x, y)
		}
	}
	fmt.Println("| Gold engine | n | A | B | B fixed | B broke | exact McNemar p |")
	fmt.Println("|---|---:|---|---|---:|---:|---:|")
	for _, key := range append([]string{"all", "specialist (pharma, Part 102, textile)"}, goldEngines...) {
		c := cells[key]
		if c == nil {
			continue
		}
		pa, la, ha := evalkit.Wilson(c.ka, c.n)
		pb, lb, hb := evalkit.Wilson(c.kb, c.n)
		fmt.Printf("| %s | %d | %d (%.1f%%, %.1f–%.1f) | %d (%.1f%%, %.1f–%.1f) | %d | %d | %.4f |\n", key, c.n,
			c.ka, 100*pa, 100*la, 100*ha, c.kb, 100*pb, 100*lb, 100*hb, c.fixed, c.broken, mcnemarExact(c.fixed, c.broken))
	}
	if n := cells["all"]; n != nil && costA > 0 {
		fmt.Printf("\nCost: A $%.4f, B $%.4f (B/A = %.2f) on %d shared rulings\n", costA, costB, costB/costA, n.n)
	}
	return nil
}
