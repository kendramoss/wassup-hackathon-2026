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
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/familycheck"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

// legacyFamilies are the three plan templates before the families work
// (D138). Coverage "before" runs the family checker with only these.
var legacyFamilies = []string{"led_lights", "lidar_modules", "motorised_tools_small_appliances"}

// familyCovBar is the confidence at which a family-check answer counts as a
// match, the same bar the BOM import uses to fill in a family
// (httpapi/products.go).
const familyCovBar = 0.6

type familyCovRow struct {
	Ruling       string  `json:"ruling"`
	Family       string  `json:"family,omitempty"`
	Confidence   float64 `json:"confidence"`
	Covered      bool    `json:"covered"`
	Reason       string  `json:"reason,omitempty"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	Error        string  `json:"error,omitempty"`
	// LatencyMS is the wall time of the check, retry included (D785).
	LatencyMS int64 `json:"latency_ms,omitempty"`
	// ModelFamily and GuardRule: when the guard (familycheck.GuardVersion)
	// took the match away, the model's own family and reason and the rule
	// (D786, D789).
	ModelFamily string `json:"model_family,omitempty"`
	ModelReason string `json:"model_reason,omitempty"`
	GuardRule   string `json:"guard_rule,omitempty"`
}

// familyCovCmd measures product-scope coverage (D138): the share of a
// split's substantial-transformation rulings that the family checker maps
// to a family with a plan template, with the legacy three templates
// ("before") or every template ("after").
func familyCovCmd(args []string) error {
	fl := flag.NewFlagSet("familycov", flag.ExitOnError)
	split := fl.String("split", "dev", "dev or final")
	set := fl.String("set", "all", "legacy (the three templates before D138) or all")
	workers := fl.Int("workers", 6, "concurrent model calls")
	limit := fl.Int("limit", 0, "only the first N cases (0 = all)")
	confirmFinal := fl.Bool("confirm-final", false, "required for the held-out final split (scored once per set)")
	labelsPath := fl.String("labels", "../eval/data/labels.jsonl", "labels file")
	splitsPath := fl.String("splits", "../eval/splits/splits_final.json", "splits file")
	factsDir := fl.String("facts", "data/facts", "facts directory")
	resultsDir := fl.String("results", "../eval/results", "results directory")
	model := fl.String("model", "gemini-3.8-flash", "family-check model")
	tag := fl.String("tag", "", "appended to the result name, to keep a later template set apart (D212: f14)")
	_ = fl.Parse(args)
	if *split == "final" && !*confirmFinal {
		return errors.New("familycov: the final split is held out: pass -confirm-final once the templates are frozen")
	}
	if *set != "legacy" && *set != "all" {
		return fmt.Errorf("familycov: -set must be legacy or all, got %q", *set)
	}
	cfg, err := loadConfig() // accepts the gateway provider (D151), as the committed coverage runs used
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	m, err := newModel(ctx, cfg, logger)
	if err != nil {
		return err
	}
	tmpls, err := planner.LoadTemplates()
	if err != nil {
		return err
	}
	if *set == "legacy" {
		keep := map[string]planner.Template{}
		for _, f := range legacyFamilies {
			keep[f] = tmpls[f]
		}
		tmpls = keep
	}
	checker := familycheck.New(m, *model, tmpls)

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
		}
	}
	if *limit > 0 && *limit < len(cases) {
		cases = cases[:*limit]
	}

	name := fmt.Sprintf("families-coverage-%s-%s-%s", *split, *set, cfg.LLMProvider)
	if *tag != "" {
		name += "-" + *tag
	}
	path := filepath.Join(*resultsDir, name+".jsonl")
	if _, err := os.Stat(path); err == nil && *split == "final" {
		return fmt.Errorf("familycov: %s exists; the held-out split is scored once per set", path)
	}
	start := time.Now()
	rows := make([]familyCovRow, len(cases))
	var wg sync.WaitGroup
	sem := make(chan struct{}, max(*workers, 1))
	for i, c := range cases {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			rows[i] = checkOne(ctx, checker, c, *model)
		}()
	}
	wg.Wait()

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	for _, r := range rows {
		b, _ := json.Marshal(r)
		w.Write(append(b, '\n'))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	var covered, errs int
	var cost float64
	byFamily := map[string]int{}
	for _, r := range rows {
		cost += r.CostUSD
		if r.Error != "" {
			errs++
			continue
		}
		if r.Covered {
			covered++
			byFamily[r.Family]++
		}
	}
	n := len(rows) - errs
	p, lo, hi := evalkit.Wilson(covered, n)
	fams := make([]string, 0, len(byFamily))
	for k := range byFamily {
		fams = append(fams, k)
	}
	sort.Slice(fams, func(i, j int) bool {
		return byFamily[fams[i]] > byFamily[fams[j]] || byFamily[fams[i]] == byFamily[fams[j]] && fams[i] < fams[j]
	})
	fmt.Printf("## Family coverage, %s split, %s templates (%d), %s, %s\n\n", *split, *set, len(tmpls), *model, cfg.LLMProvider)
	fmt.Printf("- Covered (family match at confidence >= %.1f): %d / %d = %.1f%% (95%% CI %.1f-%.1f); %d errors\n", familyCovBar, covered, n, 100*p, 100*lo, 100*hi, errs)
	for _, k := range fams {
		fmt.Printf("  - %s: %d\n", k, byFamily[k])
	}
	fmt.Printf("- Cost: $%.4f (promo prices), %.0f s\n", cost, time.Since(start).Seconds())

	lf, err := os.OpenFile(filepath.Join(*resultsDir, "RUNLOG.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer lf.Close()
	entry := map[string]any{
		"time": time.Now().Format(time.RFC3339), "run": name, "split": *split, "held_out": *split == "final",
		"cases": len(rows), "scored": n, "errors": errs, "covered": covered, "coverage": round(p),
		"ci95": []float64{round(lo), round(hi)}, "templates": len(tmpls), "family_model": *model,
		"llm_provider": cfg.LLMProvider, "prompt_version": familycheck.PromptVersion, "guard_version": familycheck.GuardVersion, "match_bar": familyCovBar,
		"cost_usd": round(cost), "git_revision": revision(), "wall_s": int(time.Since(start).Seconds()),
	}
	b, _ := json.Marshal(entry)
	_, err = lf.Write(append(b, '\n'))
	return err
}

func checkOne(ctx context.Context, c *familycheck.Checker, r domain.Ruling, model string) familyCovRow {
	text := "Product: " + r.Subject + "\n\n" + clip(r.Facts, 1500)
	// One retry: a reply cut off by the output cap is an error, not "none".
	var row familyCovRow
	var res familycheck.Result
	var err error
	began := time.Now()
	for attempt := 0; attempt < 2; attempt++ {
		var usage llm.Usage
		res, usage, err = c.CheckUsage(ctx, text)
		row.InputTokens += usage.InputTokens
		row.OutputTokens += usage.OutputTokens
		row.CostUSD += evalkit.Cost(evalkit.PromoPrices, model, usage.InputTokens, usage.OutputTokens)
		if err == nil || ctx.Err() != nil {
			break
		}
	}
	row.Ruling = r.ID
	row.LatencyMS = time.Since(began).Milliseconds()
	if err != nil {
		// Never write the gateway's address into a result file.
		row.Error = llm.ScrubGatewayHost(err.Error(), os.Getenv("LITELLM_BASE_URL"))
		return row
	}
	row.Family, row.Confidence, row.Reason = res.Family, res.Confidence, res.Reason
	row.ModelFamily, row.ModelReason, row.GuardRule = res.ModelFamily, res.ModelReason, res.GuardRule
	row.Covered = res.Family != familycheck.None && res.Confidence >= familyCovBar
	return row
}

func clip(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n])
}
