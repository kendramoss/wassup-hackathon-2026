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
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalsummary"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
)

// Engines a router label sends a case to (D230). "unclear" and
// substantial_transformation both go to the general engine.
const engineGeneral = "general"

// goldNone marks a ruling that is routed but not scored for routing (no
// single engine is right; build_router_split.py lists why).
const goldNone = "none"

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
		if r.GoldEngine == goldNone {
			continue
		}
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
	fmt.Printf("\nAll (gold engine known): %d/%d sent to the right engine (%.1f%%, %.1f–%.1f); specialist cases %d/%d (%.1f%%, %.1f–%.1f); errors %d; cost $%.4f ($%.6f per case); mean latency %.0f ms\n",
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
		if !ok || y.GoldEngine == goldNone {
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

// routedCmd scores the routed end-to-end path (D230): every ruling of a
// route file goes to the engine its router label picks, and the prediction
// of that engine on that ruling is looked up in the engine result files.
// A (ruling, engine) pair with no prediction is listed; it is run once with
// `eval run -tag routed-...` on a split file holding just those rulings.
func routedCmd(args []string) error {
	fl := flag.NewFlagSet("routed", flag.ExitOnError)
	routeFile := fl.String("route", "", "route result file (eval route)")
	files := map[string]*string{
		engineGeneral:        fl.String("general", "", "general engine result files, comma-separated"),
		router.RegimeTextile: fl.String("textile", "", "textile engine result files, comma-separated"),
		router.RegimePharma:  fl.String("pharma", "", "pharmaceutical engine result files, comma-separated"),
		router.RegimeUSMCA:   fl.String("usmca", "", "Part 102 engine result files, comma-separated"),
	}
	_ = fl.Parse(args)
	routes, err := readRouteRows(*routeFile)
	if err != nil {
		return err
	}
	pred := map[string]map[string]evalkit.Row{} // engine -> ruling -> row
	for eng, list := range files {
		pred[eng] = map[string]evalkit.Row{}
		for _, f := range splitComma(*list) {
			rows, err := readRows(f, nil)
			if err != nil {
				return err
			}
			for _, r := range rows {
				pred[eng][r.Ruling] = r
			}
		}
	}
	// ownEngine is the engine D211 counts a ruling under (each ruling once):
	// its gold engine, and the textile engine for the post-cutoff
	// textile-rule rulings that are in no Part 102 split (gold "none" in
	// final2; this command is for the final2 route files).
	ownEngine := func(r routeRow) string {
		if r.GoldEngine == goldNone {
			return router.RegimeTextile
		}
		return r.GoldEngine
	}
	type cell struct{ n, routedOK, ownOK, misrouted, misroutedOK int }
	cells := map[string]*cell{}
	var missing []string
	for _, r := range routes {
		if r.Error != "" || r.Router == nil {
			missing = append(missing, "router-error "+r.Ruling)
			continue
		}
		own := ownEngine(r)
		pr, okR := pred[r.Engine][r.Ruling]
		po, okO := pred[own][r.Ruling]
		if !okR {
			missing = append(missing, r.Engine+" "+r.Ruling)
		}
		if !okO {
			missing = append(missing, own+" "+r.Ruling+" (own engine)")
		}
		if !okR || !okO {
			continue
		}
		for _, key := range []string{"all", r.GoldEngine} {
			c := cells[key]
			if c == nil {
				c = &cell{}
				cells[key] = c
			}
			c.n++
			right := func(x evalkit.Row) bool { return x.Correct && x.Error == "" }
			if right(pr) {
				c.routedOK++
			}
			if right(po) {
				c.ownOK++
			}
			if r.Engine != own {
				c.misrouted++
				if right(pr) {
					c.misroutedOK++
				}
			}
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		fmt.Println("missing predictions (engine ruling):")
		for _, m := range missing {
			fmt.Println("  " + m)
		}
		return fmt.Errorf("routed: %d predictions missing", len(missing))
	}
	fmt.Println("| Gold engine | n | routed path right | each ruling to its own engine (best case) | sent elsewhere | of those, right anyway |")
	fmt.Println("|---|---:|---|---|---:|---:|")
	for _, key := range append([]string{"all"}, append(slices.Clone(goldEngines), goldNone)...) {
		c := cells[key]
		if c == nil {
			continue
		}
		pr, lr, hr := evalkit.Wilson(c.routedOK, c.n)
		po, lo, ho := evalkit.Wilson(c.ownOK, c.n)
		fmt.Printf("| %s | %d | %d (%.1f%%, %.1f–%.1f) | %d (%.1f%%, %.1f–%.1f) | %d | %d |\n", key, c.n,
			c.routedOK, 100*pr, 100*lr, 100*hr, c.ownOK, 100*po, 100*lo, 100*ho, c.misrouted, c.misroutedOK)
	}
	return nil
}

// routedFiles are the engine result files the routed path reads (D231,
// D232): each engine's D211 run plus the routed pairs run since.
func routedFiles(dir string) map[string][]string {
	j := func(names ...string) []string {
		var out []string
		for _, n := range names {
			out = append(out, filepath.Join(dir, n))
		}
		return out
	}
	return map[string][]string{
		engineGeneral:        j("final2-gemini-4-argon-prec-engine-v1-litellm-router.jsonl", "final2-gemini-4-argon-prec-engine-v1-litellm-routed-v2.jsonl"),
		router.RegimeTextile: j("textile-final2-gemini-4-argon-prec-textile-v2-litellm-reasoning-v1-router.jsonl"),
		router.RegimePharma:  j("pharma-final2-gemini-4-argon-prec-pharma-v1-litellm-router.jsonl"),
		router.RegimeUSMCA: j("usmca-final2-gemini-4-argon-prec-usmca-v1-litellm-router.jsonl",
			"usmca-final2-gemini-4-argon-prec-usmca-v1-litellm-routed-v2.jsonl", "usmca-final2-gemini-4-argon-prec-usmca-v1-litellm-routed-v4.jsonl"),
	}
}

// routedScore scores a route file's routed path: each ruling with the
// prediction of the engine its label picks. Missing predictions are errors.
func routedScore(routeFile string, files map[string][]string) (k, n int, err error) {
	routes, err := readRouteRows(routeFile)
	if err != nil {
		return 0, 0, err
	}
	pred := map[string]map[string]bool{}
	for eng, list := range files {
		pred[eng] = map[string]bool{}
		for _, f := range list {
			rows, err := readRows(f, nil)
			if err != nil {
				return 0, 0, err
			}
			for _, r := range rows {
				pred[eng][r.Ruling] = r.Correct && r.Error == ""
			}
		}
	}
	for _, r := range routes {
		ok, found := pred[r.Engine][r.Ruling]
		if r.Error != "" || !found {
			return 0, 0, fmt.Errorf("routed: no %s prediction for %s", r.Engine, r.Ruling)
		}
		n++
		if ok {
			k++
		}
	}
	return k, n, nil
}

// routerSummaryItems reports the production router on the router splits
// (D230-D232) and the routed end-to-end path on the post-cutoff rulings.
func routerSummaryItems(dir string) ([]evalsummary.Item, error) {
	r4 := func(x float64) float64 { return float64(int(x*10000+0.5)) / 10000 }
	var items []evalsummary.Item
	add := func(key, label, split, src string, k, n int) {
		p, lo, hi := evalkit.Wilson(k, n)
		items = append(items, evalsummary.Item{Key: key, Label: label, Value: r4(p), CILow: r4(lo), CIHigh: r4(hi), N: n,
			Split: split, DevelopmentOnly: split == "dev", Source: src})
	}
	v := router.PromptVersion
	for _, s := range []struct{ split, file, what string }{
		{"dev", "route-dev-" + v + "-gemini-3.8-flash-litellm.jsonl", "development (before 2026)"},
		{"final", "route-final-" + v + "-gemini-3.8-flash-litellm.jsonl", "held-out (Jan–Sep 2026)"},
		{"final2", "route-final2-" + v + "-gemini-3.8-flash-litellm.jsonl", "post-cutoff (28 Sep–13 Nov 2026; NOT clean held-out for router-v3/v4: the \"trade programs\" cue came from reading this window's misses, #464)"},
	} {
		path := filepath.Join(dir, s.file)
		rows, err := readRouteRows(path)
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "summary: skip router %s: %v\n", s.split, err)
			continue
		} else if err != nil {
			return nil, err
		}
		var k, n, sk, sn int
		for _, r := range rows {
			if r.GoldEngine == goldNone {
				continue
			}
			ok := r.Error == "" && r.Correct
			n++
			if ok {
				k++
			}
			if r.GoldEngine != engineGeneral {
				sn++
				if ok {
					sk++
				}
			}
		}
		src := "eval/results/" + s.file
		add("router_"+s.split+"_routing", "Router ("+v+"), "+s.what+": rulings sent to the engine of their legal test", s.split, src, k, n)
		add("router_"+s.split+"_specialist_routing", "Router ("+v+"), "+s.what+": pharmaceutical, Part 102 and textile rulings sent to their own engine", s.split, src, sk, sn)
	}
	routeFile := filepath.Join(dir, "route-final2-"+v+"-gemini-3.8-flash-litellm.jsonl")
	if v == router.PromptV2 {
		routeFile = filepath.Join(dir, "route-final2-router-v2-d211.jsonl")
	}
	if _, err := os.Stat(routeFile); err == nil {
		k, n, err := routedScore(routeFile, routedFiles(dir))
		if err != nil {
			return nil, err
		}
		add("all_regimes_final2_routed_accuracy", "All regimes, post-cutoff (28 Sep–13 Nov 2026), routed path: each ruling answered by the engine the router ("+v+") picks, each ruling once. NOT clean held-out for router-v3/v4 (#464): without the 6 \"trade programs\" rulings it is 90/101, below router-v2's 91/101",
			"final2", "eval/results/"+filepath.Base(routeFile)+" + the D211 and routed engine rows", k, n)
	}
	if k, n, err := routedScore(filepath.Join(dir, "route-final2-router-v2-d211.jsonl"), routedFiles(dir)); err == nil && v != router.PromptV2 {
		add("all_regimes_final2_routed_accuracy_router_v2", "All regimes, post-cutoff, routed path with the previous router (router-v2, as D211 ran it). This is the baseline for router-v4, whose post-cutoff figure is NOT clean held-out (#464): without the 6 \"trade programs\" rulings v2 is 91/101, above v4's 90/101",
			"final2", "eval/results/route-final2-router-v2-d211.jsonl + the D211 and routed-v2 engine rows", k, n)
	}
	return items, nil
}
