package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalsummary"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/familycheck"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

// flipsFamilyCmd runs the product's family check (D138) on the starting
// setup (variant A) of every post-cutoff flip pair (D170), the step that
// decides whether the plan search has a template for the product. One row
// per pair; the file is written once.
func flipsFamilyCmd(args []string) error {
	fl := flag.NewFlagSet("flipsfamily", flag.ExitOnError)
	flipsPath := fl.String("flips", "../eval/data/flips_post.jsonl", "flip pairs")
	labelsPaths := fl.String("labels", "../eval/data/labels.jsonl,../eval/data/labels_final2.jsonl", "label files (for each ruling's subject)")
	resultsDir := fl.String("results", "../eval/results", "results directory")
	model := fl.String("model", "gemini-3.8-flash", "family-check model")
	_ = fl.Parse(args)
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	m, err := newModel(ctx, cfg, slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
	if err != nil {
		return err
	}
	tmpls, err := planner.LoadTemplates()
	if err != nil {
		return err
	}
	checker := familycheck.New(m, *model, tmpls)
	subjects := map[string]string{}
	for _, p := range strings.Split(*labelsPaths, ",") {
		ls, err := corpus.LoadLabels(p)
		if err != nil {
			return err
		}
		for _, l := range ls {
			subjects[l.Ruling] = l.Subject
		}
	}
	items, err := readFlipItems(*flipsPath)
	if err != nil {
		return err
	}
	path := filepath.Join(*resultsDir, "planner-post-familycheck-"+cfg.LLMProvider+".jsonl")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s exists; the post-cutoff pairs are checked once", path)
	}
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	w := bufio.NewWriter(out)
	var cost float64
	for _, it := range items {
		r := domain.Ruling{ID: it.VariantA.Ruling, Subject: subjects[it.VariantA.Ruling], Facts: it.VariantA.Facts}
		row := checkOne(ctx, checker, r, *model)
		cost += row.CostUSD
		b, _ := json.Marshal(struct {
			Pair string `json:"pair"`
			familyCovRow
		}{it.ID, row})
		_, _ = w.Write(append(b, '\n'))
		fmt.Printf("- %s (%s): %s %.2f covered=%t %s\n", it.ID, it.VariantA.Ruling, row.Family, row.Confidence, row.Covered, row.Error)
	}
	fmt.Printf("- Cost: $%.4f (promo)\n- Saved: %s\n", cost, path)
	return w.Flush()
}

func readFlipItems(path string) ([]flipItem, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var items []flipItem
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var it flipItem
		if err := json.Unmarshal(sc.Bytes(), &it); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, sc.Err()
}

// plannerPostSummaryItems are the post-cutoff flip-pair numbers (D170): the
// production precedent check, scored once on 15 frozen pairs, with CBP's
// holding on the starting setup given (known-a, how the plan search runs
// when a ruling is on file). Read from the summary built by
// eval/planner/summarize.py.
func plannerPostSummaryItems(resultsDir string) ([]evalsummary.Item, error) {
	const file = "planner-post-summary.json"
	b, err := os.ReadFile(filepath.Join(resultsDir, file))
	if err != nil {
		return nil, err
	}
	type frac struct {
		K     int       `json:"k"`
		N     int       `json:"n"`
		Value float64   `json:"value"`
		CI    []float64 `json:"ci95"`
	}
	var s struct {
		Modes map[string]struct {
			All struct {
				Precision frac `json:"precision_at_bar"`
				Recall    frac `json:"recall_at_bar"`
				Controls  frac `json:"controls_shown"`
			} `json:"all"`
		} `json:"modes"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	m, ok := s.Modes["known-a"]
	if !ok || len(m.All.Precision.CI) != 2 || len(m.All.Recall.CI) != 2 {
		return nil, fmt.Errorf("%s: no known-a metrics", file)
	}
	src := "eval/results/" + file + " (eval/data/flips_post.jsonl, frozen; scored once)"
	label := fmt.Sprintf("Precedent check on post-cutoff flip pairs (%d pairs from rulings after 25 Sep 2026, gateway)", m.All.Recall.N+m.All.Controls.N)
	return []evalsummary.Item{
		{Key: "planner_post_known-a_precision_at_bar", Label: label + ": moves shown at the 0.92 bar that were real changes", Value: m.All.Precision.Value, CILow: m.All.Precision.CI[0], CIHigh: m.All.Precision.CI[1], N: m.All.Precision.N, Split: "post", Source: src},
		{Key: "planner_post_known-a_recall_at_bar", Label: label + ": real changes shown at the 0.92 bar", Value: m.All.Recall.Value, CILow: m.All.Recall.CI[0], CIHigh: m.All.Recall.CI[1], N: m.All.Recall.N, Split: "post", Source: src},
		{Key: "planner_post_known-a_controls_shown", Label: label + ": no-change pairs shown as plans", Value: m.All.Controls.Value, N: m.All.Controls.N, Split: "post", Source: src},
	}, nil
}

// plannerPostCombinedItems are the D170 pairs plus the D192 addendum (18
// pairs), known-a, as eval/planner/summarize_addendum.py combines them. The
// D170-only items above stay for the record; the results page shows these.
func plannerPostCombinedItems(resultsDir string) ([]evalsummary.Item, error) {
	const file = "planner-post-addendum-summary.json"
	b, err := os.ReadFile(filepath.Join(resultsDir, file))
	if err != nil {
		return nil, err
	}
	type frac struct {
		K     int       `json:"k"`
		N     int       `json:"n"`
		Value float64   `json:"value"`
		CI    []float64 `json:"ci95"`
	}
	var s struct {
		Modes map[string]struct {
			Combined struct {
				Pairs     int  `json:"pairs"`
				Precision frac `json:"precision_at_bar"`
				Recall    frac `json:"recall_at_bar"`
				Controls  frac `json:"controls_shown"`
			} `json:"combined"`
		} `json:"modes"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	m, ok := s.Modes["known-a"]
	c := m.Combined
	if !ok || c.Pairs == 0 || len(c.Precision.CI) != 2 || len(c.Recall.CI) != 2 || len(c.Controls.CI) != 2 {
		return nil, fmt.Errorf("%s: no combined known-a metrics", file)
	}
	src := "eval/results/" + file + " (eval/data/flips_post.jsonl + eval/data/flips_post_addendum.jsonl, each pair frozen and scored once)"
	label := fmt.Sprintf("Precedent check on post-cutoff flip pairs, D170 plus the D192 addendum (%d pairs, gateway, starting origin given)", c.Pairs)
	return []evalsummary.Item{
		{Key: "planner_post_combined_known-a_precision_at_bar", Label: label + ": moves shown at the 0.92 bar that were real changes", Value: c.Precision.Value, CILow: c.Precision.CI[0], CIHigh: c.Precision.CI[1], N: c.Precision.N, Split: "post", Source: src},
		{Key: "planner_post_combined_known-a_recall_at_bar", Label: label + ": real changes shown at the 0.92 bar", Value: c.Recall.Value, CILow: c.Recall.CI[0], CIHigh: c.Recall.CI[1], N: c.Recall.N, Split: "post", Source: src},
		{Key: "planner_post_combined_known-a_controls_shown", Label: label + ": no-change pairs shown as plans", Value: c.Controls.Value, CILow: c.Controls.CI[0], CIHigh: c.Controls.CI[1], N: c.Controls.N, Split: "post", Source: src},
	}, nil
}

// repeatabilityItems are the three development runs of the engine (D190),
// from eval/charts/build_repeatability.py's summary: each run's accuracy, the
// mean, and the rulings whose answer changed between runs.
func repeatabilityItems(resultsDir string) ([]evalsummary.Item, error) {
	const file = "repeatability-dev-litellm.json"
	b, err := os.ReadFile(filepath.Join(resultsDir, file))
	if err != nil {
		return nil, err
	}
	type run struct {
		Correct  int       `json:"correct"`
		Accuracy float64   `json:"accuracy_pct"`
		CI       []float64 `json:"ci95_pct"`
	}
	var s struct {
		N      int            `json:"n"`
		PerRun map[string]run `json:"per_run"`
		Across struct {
			Accuracy struct {
				Mean float64 `json:"mean"`
			} `json:"accuracy_pct"`
			AnswerFlips int `json:"cases_answer_flips"`
		} `json:"across_runs"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	if s.N == 0 || len(s.PerRun) != 3 {
		return nil, fmt.Errorf("%s: want three runs over a non-empty set", file)
	}
	src := "eval/results/" + file + " (eval/charts/build_repeatability.py; development set, D23 adjustments, gateway, no cache)"
	var items []evalsummary.Item
	for i := 1; i <= 3; i++ {
		r, ok := s.PerRun[fmt.Sprintf("run%d", i)]
		if !ok || len(r.CI) != 2 {
			return nil, fmt.Errorf("%s: run%d missing", file, i)
		}
		items = append(items, evalsummary.Item{Key: fmt.Sprintf("repeat_dev_accuracy_run%d", i), Label: fmt.Sprintf("Repeatability: development accuracy, run %d of 3", i),
			Value: float64(r.Correct) / float64(s.N), CILow: r.CI[0] / 100, CIHigh: r.CI[1] / 100, N: s.N, Split: "dev", DevelopmentOnly: true, Source: src})
	}
	return append(items,
		evalsummary.Item{Key: "repeat_dev_accuracy_mean", Label: "Repeatability: mean development accuracy over the three runs", Value: s.Across.Accuracy.Mean / 100, N: s.N, Split: "dev", DevelopmentOnly: true, Source: src},
		evalsummary.Item{Key: "repeat_dev_answer_flips", Label: "Repeatability: rulings whose answer changed between runs (a count)", Value: float64(s.Across.AnswerFlips), N: s.N, Split: "dev", DevelopmentOnly: true, Source: src},
	), nil
}

// heldoutRepeatItems are the held-out repeat runs of the frozen engine (D240,
// D241), from eval/charts/build_repeatability_heldout.py's summary: per split,
// the official run, each repeat, the repeat mean and the rulings whose answer
// changed across the three runs.
func heldoutRepeatItems(resultsDir string) ([]evalsummary.Item, error) {
	const file = "repeatability-heldout-litellm.json"
	b, err := os.ReadFile(filepath.Join(resultsDir, file))
	if err != nil {
		return nil, err
	}
	type run struct {
		Correct int       `json:"correct"`
		CI      []float64 `json:"ci95_pct"`
	}
	var s struct {
		Splits map[string]struct {
			N      int            `json:"n"`
			PerRun map[string]run `json:"per_run"`
			Rep    struct {
				Accuracy struct {
					Mean float64 `json:"mean"`
				} `json:"accuracy_pct"`
			} `json:"repeats"`
			Three struct {
				AnswerFlips int `json:"cases_answer_flips"`
			} `json:"official_and_repeats"`
		} `json:"splits"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	src := "eval/results/" + file + " (eval/charts/build_repeatability_heldout.py; D23 adjustments; repeats through the gateway, no cache)"
	var items []evalsummary.Item
	for _, sp := range []struct{ key, label string }{{"final", "First held-out split"}, {"final2", "Second held-out split"}} {
		x, ok := s.Splits[sp.key]
		if !ok || x.N == 0 {
			return nil, fmt.Errorf("%s: split %s missing", file, sp.key)
		}
		for _, r := range []struct{ key, label string }{{"official", "official run"}, {"repeat1", "repeat 1"}, {"repeat2", "repeat 2"}} {
			v, ok := x.PerRun[r.key]
			if !ok || len(v.CI) != 2 {
				return nil, fmt.Errorf("%s: %s %s missing", file, sp.key, r.key)
			}
			items = append(items, evalsummary.Item{Key: fmt.Sprintf("repeat_%s_accuracy_%s", sp.key, r.key), Label: fmt.Sprintf("Repeatability: %s accuracy, %s", sp.label, r.label),
				Value: float64(v.Correct) / float64(x.N), CILow: v.CI[0] / 100, CIHigh: v.CI[1] / 100, N: x.N, Split: sp.key, Source: src})
		}
		items = append(items,
			evalsummary.Item{Key: fmt.Sprintf("repeat_%s_accuracy_repeat_mean", sp.key), Label: fmt.Sprintf("Repeatability: %s, mean accuracy of the two repeats", sp.label), Value: x.Rep.Accuracy.Mean / 100, N: x.N, Split: sp.key, Source: src},
			evalsummary.Item{Key: fmt.Sprintf("repeat_%s_answer_flips", sp.key), Label: fmt.Sprintf("Repeatability: %s, rulings whose answer changed across the official run and the repeats (a count)", sp.label), Value: float64(x.Three.AnswerFlips), N: x.N, Split: sp.key, Source: src},
		)
	}
	return items, nil
}

// planStabilityItems are the three pre-registered repeat runs of the heat-gun
// plan search (D321), from eval/planner/plan_stability.py's summary: per run,
// how many moves were shown and the best precedent check; across runs,
// whether the shown moves were the same; and the earlier recorded runs, which
// used other configurations.
func planStabilityItems(resultsDir string) ([]evalsummary.Item, error) {
	const file = "plan-stability-heatgun.json"
	b, err := os.ReadFile(filepath.Join(resultsDir, file))
	if err != nil {
		return nil, err
	}
	var s struct {
		Runs []struct {
			Path  string   `json:"path"`
			Shown []any    `json:"shown"`
			Best  *float64 `json:"best"`
		} `json:"runs"`
		Before struct {
			Runs  int `json:"runs"`
			Shown int `json:"with_a_plan_shown"`
		} `json:"recorded_before_d321"`
		D321 struct {
			Runs   int        `json:"runs"`
			Shown  int        `json:"with_a_plan_shown"`
			Same   bool       `json:"same_shown_moves_in_all_runs"`
			Best   []float64  `json:"best_check_per_run"`
			PerRun [][]string `json:"shown_moves_per_run"`
		} `json:"d321"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	if s.D321.Runs != 3 || len(s.D321.Best) != 3 || len(s.D321.PerRun) != 3 {
		return nil, fmt.Errorf("%s: want three D321 runs", file)
	}
	src := "eval/results/" + file + " (eval/planner/plan_stability.py; heat-gun demo, D321: production verifier configuration through the gateway, no cache)"
	same := 0.0
	if s.D321.Same {
		same = 1
	}
	items := []evalsummary.Item{
		{Key: "plan_stability_heatgun_runs_shown", Label: "Plan check, run again: heat-gun runs that showed a plan (D321, a count)", Value: float64(s.D321.Shown), N: s.D321.Runs, Split: "demo", DevelopmentOnly: true, Source: src},
		{Key: "plan_stability_heatgun_same_moves", Label: "Plan check, run again: the same moves were shown in every run (1 yes, 0 no)", Value: same, N: s.D321.Runs, Split: "demo", DevelopmentOnly: true, Source: src},
	}
	for i := 0; i < 3; i++ {
		items = append(items,
			evalsummary.Item{Key: fmt.Sprintf("plan_stability_heatgun_run%d_shown_moves", i+1), Label: fmt.Sprintf("Plan check, run again: moves shown in run %d (a count)", i+1), Value: float64(len(s.D321.PerRun[i])), N: 7, Split: "demo", DevelopmentOnly: true, Source: src},
			evalsummary.Item{Key: fmt.Sprintf("plan_stability_heatgun_run%d_best_check", i+1), Label: fmt.Sprintf("Plan check, run again: best precedent check in run %d", i+1), Value: s.D321.Best[i], Split: "demo", DevelopmentOnly: true, Source: src},
		)
	}
	return append(items, evalsummary.Item{Key: "plan_stability_heatgun_earlier_runs_shown", Label: "Plan check: earlier recorded heat-gun runs, other configurations, that showed a plan (a count)", Value: float64(s.Before.Shown), N: s.Before.Runs, Split: "demo", DevelopmentOnly: true, Source: src}), nil
}
