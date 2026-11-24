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
//	eval route  -split dev|final|final2 [-prompt router-v2] [-confirm-final]
//	    Run one router version on the router splits
//	    (eval/splits/splits_router.json) and score the engine each case is
//	    sent to against the engine its legal test belongs to (D230).
//	eval routecompare -a BASE -b VARIANT
//	    Paired comparison of two route files, overall and per gold engine,
//	    with an exact McNemar test.
//	eval routed -route FILE -general F -textile F -pharma F -usmca F
//	    The routed end-to-end path: each ruling scored with the engine its
//	    router label picks, beside each ruling scored by its own engine.
//	eval plan   -fixture FILE [-search local|cross]
//	    Run the planner end to end on a fixture (engine + verifier) and save
//	    the result to eval/results/plan-<case>-<provider>.json.
//	eval predict -facts FILE -date YYYY-MM-DD [-id ID]
//	    One engine prediction on a facts file (debugging and spot checks).
//	eval flips  [-known-a] [-workers 6]
//	    Run the verifier on the development flip pairs (CBP natural
//	    experiments dated before 2026; the 2026 items are spent) and report
//	    precision, recall and specificity. Development-only by construction.
//	eval summary [-out internal/evalsummary/summary.json] [-check]
//	    Build the results-page numbers from the committed result files (plus
//	    the research baselines, with their source) for GET /v1/eval/summary.
//	    With -check, verify the embedded file matches the generated summary,
//	    ignoring generated_at, and fail loudly if it has drifted.
//	eval multiscenario [-out ../eval/data/multi_scenario.json]
//	    Flag split rulings whose holding gives different origins for different
//	    scenarios (a mechanical rule over CBP's analysis text, applied to every
//	    item); they feed eval/data/scoring_adjustments.json (report -adjust).
//	eval compare -a BASE -b VARIANT [-adjust FILE]
//	    Paired comparison on shared rulings: fixed / broken counts and an exact
//	    McNemar test.
//	eval learnability -in RESULTS [-k 10]
//	    DIAGNOSTIC, dev only (uses gold labels): for each wrong prediction, a
//	    judge sees the case, CBP's holding and the k most similar earlier
//	    rulings, and says whether an earlier ruling establishes a rule that
//	    yields CBP's answer.
//	eval clarify -in RESULTS [-below 0.95]
//	    Run the clarify review on below-threshold cases of an engine run (same
//	    precedents the engine saw) and report how its flags relate to errors.
//	eval report -in FILE [-in FILE ...] [-adjust FILE]
//	    Print the Markdown summary of existing result files.
//	eval calibrate [-target 0.95] [-min-n 20]
//	    No model calls. Fit an auto-accept bar per router subfamily on the
//	    dev engine results only, then apply the frozen bars once to the
//	    held-out results and compare with the global bar (D63). Writes
//	    eval/results/calibration-by-family.{json,md} and the bars the
//	    server embeds (internal/triage/family_bars.json).
//	eval slice [-in FILE] [-pick]
//	    Regression slice (D72): run the engine on the 30 committed dev rulings
//	    of eval/splits/slice30.json (needs EVAL_SLICE=on) or take an existing
//	    result file, and fail when it falls below
//	    eval/results/slice30-baseline.json by more than the tolerance.
//	eval flipsfamily [-flips ../eval/data/flips_post.jsonl]
//	    The product's family check on the starting setup of every post-cutoff
//	    flip pair (D170); decides which pairs the plan search can try.
//	eval familycov -split dev|final -set legacy|all [-confirm-final]
//	    Product-scope coverage (D138): run the family checker on a split's
//	    rulings with the legacy three templates or all of them, and report
//	    the share matched to a family with a plan template.
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
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/calibrate"
	"log/slog"
	"math"
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

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/clarify"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/cross"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalsummary"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/nearmiss"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
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
	case "route":
		err = routeCmd(os.Args[2:])
	case "routecompare":
		err = routeCompareCmd(os.Args[2:])
	case "routed":
		err = routedCmd(os.Args[2:])
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
	case "compare":
		err = compareCmd(os.Args[2:])
	case "learnability":
		err = learnabilityCmd(os.Args[2:])
	case "torn":
		err = tornCmd(os.Args[2:])
	case "clarify":
		err = clarifyCmd(os.Args[2:])
	case "tunesplit":
		err = tuneSplitCmd(os.Args[2:])
	case "tunedata":
		err = tuneDataCmd(os.Args[2:])
	case "calibrate":
		err = calibrateCmd(os.Args[2:])
	case "slice":
		err = sliceCmd(os.Args[2:])
	case "flipsfamily":
		err = flipsFamilyCmd(os.Args[2:])
	case "familycov":
		err = familyCovCmd(os.Args[2:])
	case "baselines":
		err = baselinesCmd(os.Args[2:])
	case "p102moves":
		err = p102MovesCmd(os.Args[2:])
	case "adjudicate":
		err = adjudicateCmd(os.Args[2:])
	case "market":
		err = marketCmd(os.Args[2:])
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
	split := fl.String("split", "dev", "dev, final or pilot; final2 with -splits ../eval/splits/splits_final2.json; train or tune_check with -splits ../eval/splits/splits_tune.json")
	limit := fl.Int("limit", 0, "only the first N cases (0 = all)")
	workers := fl.Int("workers", 8, "concurrent model calls")
	hedgeAfter := fl.Duration("hedge-after", 0, "send a call once more if it has not answered after this long and use the first answer (D611; 0 = off)")
	useRouter := fl.Bool("router", false, "also run the router; its regime label drives the scope check")
	cond := fl.String("cond", "prec", "prec (with precedents) or zero (facts only)")
	retrievalMode := fl.String("retrieval", "bm25", "bm25, hybrid (BM25 candidates re-ranked by embeddings) or vector")
	candidates := fl.Int("candidates", 30, "BM25 candidates re-ranked in hybrid mode")
	vectorsPath := fl.String("vectors", "../data/corpus/embeddings-768.bin", "embedding index (go run ./cmd/corpus embed)")
	votes := fl.Int("votes", 1, "calls per low-confidence case (majority over rotated precedent orders); 1 = no voting")
	voteBelow := fl.Float64("vote-below", 0.95, "vote only when the first answer's confidence is below this")
	useRules := fl.Bool("rules", false, "append engine.Rules (rules-v1) to the system prompt")
	rulesVersion := fl.String("rules-version", "v1", "with -rules: v1 (rules-v1) or v2 (rules-v2, D157)")
	foodRules := fl.Bool("food-rules", false, "add engine.FoodRules (food-v1, D856) to the system prompt of cases whose facts describe a food; other cases are sent as without it")
	useReasoning := fl.Bool("reasoning", false, "show each precedent's CBP reasoning (reasoning-v1; needs -raw)")
	rawDir := fl.String("raw", "../data/corpus/raw", "raw ruling JSON, for -reasoning")
	confirmFinal := fl.Bool("confirm-final", false, "required to score a held-out split (final, final2)")
	tag := fl.String("tag", "", "appended to the result file name, to keep a repeat run apart from an earlier file with the same configuration")
	labelsPath := fl.String("labels", "../eval/data/labels.jsonl", "labels file(s), comma-separated; final2 needs ../eval/data/labels.jsonl,../eval/data/labels_final2.jsonl")
	caseLabels := fl.String("case-labels", "", "extra label file(s), comma-separated, for split rulings missing from -labels; they are cases only, never precedents (D230: a regime engine run on another regime's rulings)")
	splitsPath := fl.String("splits", "../eval/splits/splits_final.json", "splits file")
	factsDir := fl.String("facts", "../data/corpus/facts", "facts directory (go run ./cmd/corpus build); comma-separated to add ../eval/data/facts_final2")
	resultsDir := fl.String("results", "../eval/results", "results directory")
	textilePrompt := fl.String("textile-prompt", "v1", "v1 or v2 (textile-v2: filled made-up guidance; use with -reasoning, D53)")
	usmcaPrompt := fl.String("usmca-prompt", "v1", "with -regime usmca: v1 (usmca-v1), v2 (usmca-v2: the 102.20 rows for the codes named in the facts, applied as written; D550), v3 (usmca-v3: the D590 102.11 checklist), v4 (usmca-v4: the D600 compact checklist) or v5 (usmca-v5: the D600 failing-materials checklist)")
	stPrompt := fl.String("st-prompt", "v1", "with -regime st: v1 (engine-v1), ck1 (st-checklist-v1: the D650 component checklist) or ck2 (st-checklist-v2: the checklist and the two closest contrasting precedents)")
	regime := fl.String("regime", "st", "st (substantial transformation), textile (19 CFR 102.21, textile-v1, with -splits ../eval/splits/splits_textile.json; D51), usmca (19 CFR 102.11 / 102.20, usmca-v1, with -labels ../eval/data/labels_usmca.jsonl and -splits ../eval/splits/splits_usmca.json; D61) or pharma (active ingredient, pharma-v1, with -splits ../eval/splits/splits_pharma.json; D62)")
	_ = fl.Parse(args)
	if *regime != "st" && *regime != "textile" && *regime != "usmca" && *regime != "pharma" {
		return fmt.Errorf("-regime must be st, textile, usmca or pharma, got %q", *regime)
	}
	if *splitsPath == "../eval/splits/splits_final.json" {
		switch *regime {
		case "textile":
			*splitsPath = "../eval/splits/splits_textile.json"
		case "pharma":
			*splitsPath = "../eval/splits/splits_pharma.json"
		}
	}
	if *regime == "usmca" {
		// Its own labels: most Part 102 rulings predate labels.jsonl (2023+).
		// Every labelled ruling is a precedent for the cases dated after it.
		if *labelsPath == "../eval/data/labels.jsonl" {
			*labelsPath = "../eval/data/labels_usmca.jsonl"
		}
		if *splitsPath == "../eval/splits/splits_final.json" {
			*splitsPath = "../eval/splits/splits_usmca.json"
		}
	}

	if isHeldOut(*split) && !*confirmFinal {
		return errors.New("the final splits are held out: pass -confirm-final, and do not change prompts, thresholds or retrieval because of its result")
	}
	if *cond != "prec" && *cond != "zero" {
		return fmt.Errorf("-cond must be prec or zero, got %q", *cond)
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
	dirs, err := existingDirs(*factsDir)
	if err != nil {
		return err
	}
	store, err := corpus.LoadDirs(labels, dirs...)
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
	var caseStore *corpus.Store
	if *caseLabels != "" {
		cl, err := loadLabelFiles(*caseLabels)
		if err != nil {
			return err
		}
		if caseStore, err = corpus.LoadDirs(cl, dirs...); err != nil {
			return err
		}
	}
	var cases []domain.Ruling
	for _, id := range ids {
		if r, ok := store.Get(id); ok {
			cases = append(cases, r)
		} else if r, ok := caseStoreGet(caseStore, id); ok {
			cases = append(cases, r)
		} else {
			fmt.Fprintf(os.Stderr, "warning: %s has no facts or label; skipped\n", id)
		}
	}
	if *limit > 0 && *limit < len(cases) {
		cases = cases[:*limit]
	}

	if *useReasoning { // before the retriever copies the rulings
		n, err := store.AttachReasoning(*rawDir)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "reasoning attached to %d of %d precedents\n", n, store.Len())
	}
	model, err := newHedgedModel(ctx, cfg, logger, *hedgeAfter)
	if err != nil {
		return err
	}
	pool := store.Rulings()
	if *regime == "textile" {
		pool = regimePool(store, splits.Textile)
		if len(pool) == 0 {
			return errors.New("textile: the splits file lists no textile rulings")
		}
	}
	if *regime == "pharma" {
		pool = regimePool(store, splits.Pharma)
		if len(pool) == 0 {
			return errors.New("pharma: the splits file lists no pharmaceutical rulings")
		}
	}
	var retr retrieval.Retriever
	if *cond == "prec" {
		bm := retrieval.NewBM25(pool, engine.PrecedentText)
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
	eng := engine.New(model, retr, engine.Options{Model: cfg.EngineModel, K: cfg.PrecedentK, MaxOutputTokens: cfg.MaxOutputTokens,
		Votes: *votes, VoteBelow: *voteBelow})
	if *useRules || *useReasoning {
		opts := engine.Options{Model: cfg.EngineModel, K: cfg.PrecedentK, MaxOutputTokens: cfg.MaxOutputTokens,
			Votes: *votes, VoteBelow: *voteBelow, PrecedentReasoning: *useReasoning}
		if *useRules {
			opts.Rules, opts.RulesVersion = engine.Rules, engine.RulesVersion
			if *rulesVersion == "v2" {
				opts.Rules, opts.RulesVersion = engine.Rules2, engine.RulesVersion2
			} else if *rulesVersion != "v1" {
				return fmt.Errorf("-rules-version must be v1 or v2, got %q", *rulesVersion)
			}
		}
		eng = engine.New(model, retr, opts)
	}
	stVersion := engine.PromptVersion
	if *stPrompt != "v1" {
		sopts, ok := engine.STChecklistOptions(*stPrompt)
		if !ok {
			return fmt.Errorf("-st-prompt must be v1, ck1 or ck2, got %q", *stPrompt)
		}
		if *regime != "st" || *useRules || *useReasoning || *votes > 1 || *retrievalMode != "bm25" {
			return errors.New("-st-prompt ck1|ck2 supports plain bm25 runs of -regime st")
		}
		sopts.Model, sopts.K, sopts.MaxOutputTokens = cfg.EngineModel, cfg.PrecedentK, cfg.MaxOutputTokens
		eng = engine.New(model, retr, sopts)
		stVersion = sopts.Version
	}
	if *foodRules {
		if *regime != "st" || *stPrompt != "v1" || *useRules || *useReasoning || *votes > 1 || *retrievalMode != "bm25" {
			return errors.New("-food-rules supports plain bm25 runs of -regime st")
		}
		eng = engine.New(model, retr, engine.Options{Model: cfg.EngineModel, K: cfg.PrecedentK, MaxOutputTokens: cfg.MaxOutputTokens,
			CaseRules: engine.FoodCaseRules, CaseRulesVersion: engine.FoodVersion})
	}
	if *regime == "textile" {
		if *useRules || *retrievalMode != "bm25" {
			return errors.New("-regime textile supports bm25 runs, optionally with -reasoning")
		}
		sys, ver := engine.TextileSystem, engine.TextileVersion
		if *textilePrompt == "v2" {
			sys, ver = engine.TextileSystem2, engine.TextileVersion2
		}
		eng = engine.New(model, retr, engine.Options{Model: cfg.EngineModel, K: cfg.PrecedentK, MaxOutputTokens: cfg.MaxOutputTokens,
			Votes: *votes, VoteBelow: *voteBelow, System: sys, Version: ver, PrecedentReasoning: *useReasoning})
	}
	if *regime == "usmca" {
		if *useRules || *useReasoning || *retrievalMode != "bm25" {
			return errors.New("-regime usmca supports bm25 runs only")
		}
		uopts, err := usmcaOptions(*usmcaPrompt)
		if err != nil {
			return err
		}
		uopts.Model, uopts.K, uopts.MaxOutputTokens, uopts.Votes, uopts.VoteBelow = cfg.EngineModel, cfg.PrecedentK, cfg.MaxOutputTokens, *votes, *voteBelow
		eng = engine.New(model, retr, uopts)
	}
	if *regime == "pharma" {
		if *useRules || *useReasoning || *retrievalMode != "bm25" {
			return errors.New("-regime pharma supports plain bm25 runs")
		}
		eng = engine.New(model, retr, engine.Options{Model: cfg.EngineModel, K: cfg.PrecedentK, MaxOutputTokens: cfg.MaxOutputTokens,
			Votes: *votes, VoteBelow: *voteBelow, System: engine.PharmaSystem, Version: engine.PharmaVersion})
	}
	var rt *router.Router
	if *useRouter {
		rt = router.New(model, cfg.RouterModel, 2000)
	}

	name := fmt.Sprintf("%s-%s-%s-%s-%s", *split, cfg.EngineModel, *cond, stVersion, cfg.LLMProvider)
	if *regime == "textile" {
		tv := engine.TextileVersion
		if *textilePrompt == "v2" {
			tv = engine.TextileVersion2
		}
		name = fmt.Sprintf("textile-%s-%s-%s-%s-%s", *split, cfg.EngineModel, *cond, tv, cfg.LLMProvider)
		if cfg.PrecedentK != 5 {
			name += fmt.Sprintf("-k%d", cfg.PrecedentK)
		}
	}
	if *regime == "usmca" {
		uopts, _ := usmcaOptions(*usmcaPrompt)
		uv := uopts.Version
		name = fmt.Sprintf("usmca-%s-%s-%s-%s-%s", *split, cfg.EngineModel, *cond, uv, cfg.LLMProvider)
		if cfg.PrecedentK != 5 {
			name += fmt.Sprintf("-k%d", cfg.PrecedentK)
		}
	}
	if *regime == "pharma" {
		name = fmt.Sprintf("pharma-%s-%s-%s-%s-%s", *split, cfg.EngineModel, *cond, engine.PharmaVersion, cfg.LLMProvider)
		if cfg.PrecedentK != 5 {
			name += fmt.Sprintf("-k%d", cfg.PrecedentK)
		}
	}
	if *retrievalMode != "bm25" {
		name = fmt.Sprintf("%s-%s-%s%d-%s-%s", *split, cfg.EngineModel, *retrievalMode, *candidates, engine.PromptVersion, cfg.LLMProvider)
	}
	if *votes > 1 {
		name += fmt.Sprintf("-vote%d", *votes)
	}
	if *useRules {
		if *rulesVersion == "v2" {
			name += "-" + engine.RulesVersion2
		} else {
			name += "-" + engine.RulesVersion
		}
	}
	if *useReasoning {
		name += "-" + engine.ReasoningVersion
	}
	if *foodRules {
		name += "-" + engine.FoodVersion
	}
	if *useRouter {
		name += "-router"
	}
	if *tag != "" {
		if !regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`).MatchString(*tag) {
			return fmt.Errorf("-tag must be lower-case letters, digits, dots and dashes, got %q", *tag)
		}
		name += "-" + *tag
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

	dirtyAtStart := codeDirty() // before this run appends to RUNLOG (D840)
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
			labelStore := store
			if _, ok := store.Label(c.ID); !ok && caseStore != nil {
				labelStore = caseStore // a -case-labels ruling: its label is there
			}
			row := evaluate(ctx, eng, rt, cfg, labelStore, c, *split)
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
	data, err := datasetSHA256(*labelsPath, *splitsPath)
	if err != nil {
		return err
	}
	if *caseLabels != "" {
		extra, err := datasetSHA256(*caseLabels)
		if err != nil {
			return err
		}
		for k, v := range extra {
			data[k] = v
		}
	}
	prov := map[string]any{"dataset_sha256": data, "code_dirty": dirtyAtStart, "llm_no_cache": os.Getenv("LITELLM_NO_CACHE") == "1", "workers": *workers}
	if err := appendRunLog(*resultsDir, name, *split, cfg, s, time.Since(start), len(cases), *limit, ctx.Err() != nil, prov); err != nil {
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
	tally := &llm.CostTally{}
	ctx = llm.ContextWithCostTally(ctx, tally)
	inScope, scopeReason := true, ""
	if rt != nil {
		if l, err := rt.Classify(ctx, c.Facts); err == nil {
			row.RouterRoute, row.RouterRegime, row.RouterFamily = l.Route, l.Regime, l.Subfamily
			inScope, scopeReason = l.Scope()
		} else {
			row.RouterRoute = "error: " + err.Error()
		}
	}
	t0 := time.Now()
	res, err := eng.Predict(ctx, domain.Case{ID: c.ID, Date: c.Date, Facts: c.Facts})
	row.WallMS = time.Since(t0).Milliseconds()
	row.GatewayCostUSD = tally.USD()
	if err != nil {
		row.Error, row.ErrorClass = err.Error(), errorClass(err)
		return row
	}
	p := res.Prediction
	row.Pred, row.Correct = p.Origin, p.Origin == row.Gold
	row.Confidence, row.KeyFactors, row.DecisiveStep = p.Confidence, p.KeyFactors, p.DecisiveStep
	row.Flip, row.Reasoning, row.Precedents, row.Cited = res.Flip, p.Reasoning, p.Precedents, p.CitedRulings
	row.Provider, row.ModelID, row.PromptVersion = p.Trace.Provider, p.Trace.ModelID, p.Trace.PromptVersion
	row.InputTokens, row.OutputTokens, row.LatencyMS = p.Trace.InputTokens, p.Trace.OutputTokens, p.Trace.LatencyMS
	row.Votes = res.Votes
	row.Answer = res.Answer
	row.CostUSD = evalkit.Cost(evalkit.PromoPrices, cfg.EngineModel, row.InputTokens, row.OutputTokens)
	row.CostUSDList = evalkit.Cost(evalkit.ListPrices, cfg.EngineModel, row.InputTokens, row.OutputTokens)
	row.InScope = inScope
	row.TriageRoute = string(triage.Decide(p, cfg.FastPathConfidence, inScope, scopeReason).Route)
	return row
}

// errorClass names what kind of failure a case's error is (evalkit.Row
// ErrorClass, D261): a provider outage the case may get past on a re-run, or
// an unusable answer, which a re-run would only hide.
func errorClass(err error) string {
	switch {
	case llm.Unavailable(err):
		return "unavailable"
	case errors.Is(err, llm.ErrBadResponse):
		return "bad_response"
	}
	return ""
}

func newModel(ctx context.Context, cfg config.Config, logger *slog.Logger) (llm.Model, error) {
	return newHedgedModel(ctx, cfg, logger, 0)
}

// newHedgedModel is newModel with a hedged request after hedgeAfter (D611;
// 0 = none): a call not answered by then is sent once more, inside the
// retry policy, and the first answer is used.
func newHedgedModel(ctx context.Context, cfg config.Config, logger *slog.Logger, hedgeAfter time.Duration) (llm.Model, error) {
	var base llm.Model
	switch cfg.LLMProvider {
	case config.ProviderVertex:
		v, err := llm.NewVertex(ctx, llm.VertexOptions{Project: cfg.GCPProject, Location: cfg.GCPLocation, Timeout: cfg.LLMTimeout})
		if err != nil {
			return nil, err
		}
		base = v
	case providerLiteLLM:
		ll := llm.NewLiteLLM(os.Getenv("LITELLM_BASE_URL"), os.Getenv("LITELLM_API_KEY"), cfg.LLMTimeout)
		// LITELLM_NO_CACHE=1: fresh calls for repeat runs (D190). Without it
		// an identical request can come back from the gateway's cache.
		ll.SkipCache(os.Getenv("LITELLM_NO_CACHE") == "1")
		base = ll
	default:
		return nil, fmt.Errorf("eval needs a real model provider, got %q", cfg.LLMProvider)
	}
	base = llm.WithPace(base, cfg.LLMCallGap, llm.NewGate(cfg.LLMMaxInFlight, cfg.LLMCoolDown))
	base = llm.WithHedge(base, hedgeAfter)
	return llm.WithLogging(llm.WithRetry(base, cfg.LLMMaxRetries, 2*time.Second), logger), nil
}

// isHeldOut reports whether a split is a held-out test split: final (frozen
// 14 Oct) or final2 (rulings dated after 25 Sep 2026, frozen 21 Nov, D150).
// Both need -confirm-final and are logged with held_out true.
func isHeldOut(split string) bool { return split == "final" || split == "final2" }

// providerLiteLLM is accepted by the eval command only (D151): an
// OpenAI-compatible LiteLLM gateway to Gemini, for development runs. The
// server's config still refuses it (D49), so it cannot reach production.
const providerLiteLLM = "litellm"

// loadConfig reads the same environment as the server. LLM_PROVIDER=litellm
// is validated as the offline provider, then recorded as "litellm" in every
// row and in RUNLOG.jsonl; it needs LITELLM_BASE_URL and LITELLM_API_KEY.
func loadConfig() (config.Config, error) {
	if !strings.EqualFold(os.Getenv("LLM_PROVIDER"), providerLiteLLM) {
		return config.Load()
	}
	if os.Getenv("LITELLM_BASE_URL") == "" || os.Getenv("LITELLM_API_KEY") == "" {
		return config.Config{}, errors.New("LLM_PROVIDER=litellm needs LITELLM_BASE_URL and LITELLM_API_KEY")
	}
	_ = os.Setenv("LLM_PROVIDER", config.ProviderFake)
	cfg, err := config.Load()
	_ = os.Setenv("LLM_PROVIDER", providerLiteLLM)
	if err != nil {
		return config.Config{}, err
	}
	cfg.LLMProvider = providerLiteLLM
	return cfg, nil
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
// prov adds provenance keys (dataset_sha256, code_dirty; D840) when given.
func appendRunLog(dir, name, split string, cfg config.Config, s evalkit.Summary, wall time.Duration, n, limit int, interrupted bool, prov map[string]any) error {
	f, err := os.OpenFile(filepath.Join(dir, "RUNLOG.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	prompt := engine.PromptVersion
	if strings.HasPrefix(name, "textile-") {
		prompt = engine.TextileVersion
		if strings.Contains(name, engine.TextileVersion2) {
			prompt = engine.TextileVersion2
		}
	}
	if strings.HasPrefix(name, "usmca-") {
		prompt = engine.USMCAVersion
		for _, v := range []string{engine.USMCAVersion2, engine.USMCAVersion3, engine.USMCAVersion4, engine.USMCAVersion5} {
			if strings.Contains(name, v) {
				prompt = v
			}
		}
	}
	if strings.HasPrefix(name, "pharma-") {
		prompt = engine.PharmaVersion
	}
	if strings.Contains(name, "-"+engine.FoodVersion) {
		prompt += "+" + engine.FoodVersion
	}
	entry := map[string]any{
		"time": time.Now().Format(time.RFC3339), "run": name, "split": split, "held_out": isHeldOut(split),
		"cases": n, "limit": limit, "scored": s.N, "correct": s.Correct, "errors": s.Errors,
		"accuracy": round(s.Acc), "ci95": []float64{round(s.Lo), round(s.Hi)},
		"fast_path_coverage": s.FastN, "fast_path_correct": s.FastK,
		"llm_provider": cfg.LLMProvider, "engine_model": cfg.EngineModel, "router_model": cfg.RouterModel,
		"prompt_version": prompt, "k": cfg.PrecedentK, "fast_path_threshold": cfg.FastPathConfidence,
		"git_revision": revision(), "wall_s": int(wall.Seconds()), "interrupted": interrupted,
		"cost_per_case_usd": s.CostPerCase, "latency_p50_ms": s.LatencyP50, "latency_p95_ms": s.P95,
	}
	for k, v := range prov {
		entry[k] = v
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
	adjustFile := fl.String("adjust", "", "scoring adjustments (eval/data/scoring_adjustments.json)")
	threshold := fl.Float64("threshold", config.DefaultFastPath, "fast-path confidence threshold used with -scope")
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
		if *adjustFile != "" {
			adj, err := evalkit.LoadAdjustments(*adjustFile)
			if err != nil {
				return err
			}
			n := len(rows)
			rows = adj.Apply(rows)
			fmt.Fprintf(os.Stderr, "report: adjustments applied, %d excluded\n", n-len(rows))
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
		if isHeldOut(*splitName) && !*confirmFinal {
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
			"split": *splitName, "held_out": isHeldOut(*splitName), "labelled": len(items), "out_of_scope": outN,
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
	searchMode := fl.String("search", "local", "verifier evidence: local (corpus, reproducible), cross (live CROSS with toDate) or relaxed (the server's searcher: CROSS with shorter retries, filled from the corpus; D89)")
	tag := fl.String("tag", "", "suffix for the output file, e.g. stability-run1 (repeat runs, D321)")
	labelsPath := fl.String("labels", "../eval/data/labels.jsonl", "labels file")
	factsDir := fl.String("facts", "../data/corpus/facts", "facts directory")
	rawDir := fl.String("raw", "../data/corpus/raw", "raw CROSS JSON directory")
	resultsDir := fl.String("results", "../eval/results", "results directory")
	regime := fl.String("regime", "", "the case's router regime: usmca_part102 predicts every move with the Part 102 engine, as the server does on a Part 102 case (D540); empty: the general engine")
	usmcaLabels := fl.String("usmca-labels", "../eval/data/labels_usmca.jsonl", "the Part 102 engine's precedent pool (with -regime usmca_part102)")
	_ = fl.Parse(args)
	if *regime != "" && *regime != router.RegimeUSMCA {
		return fmt.Errorf("-regime must be empty or %s", router.RegimeUSMCA)
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
	req, err := planner.LoadRequest(*fixture)
	if err != nil {
		return err
	}
	req.Regime = *regime
	labels, err := corpus.LoadLabels(*labelsPath)
	if err != nil {
		return err
	}
	store, err := corpus.Load(labels, *factsDir)
	if err != nil {
		return err
	}
	if store.Len() == 0 { // an empty pool leaves the engine and the precedent check with no rulings (D541)
		return fmt.Errorf("plan: no precedent facts in %s (run: go run ./cmd/corpus fetch && go run ./cmd/corpus build)", *factsDir)
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
		searcher = verifier.CROSS{Client: cross.NewHTTPClient(cross.UserAgent)}
	case "relaxed":
		local, err := verifier.NewLocal(labels, *factsDir, *rawDir)
		if err != nil {
			return err
		}
		searcher = verifier.Relaxed{Primary: verifier.CROSS{Client: cross.NewHTTPClient(cross.UserAgent)}, Fill: local}
	default:
		return fmt.Errorf("-search must be local, cross or relaxed")
	}
	// The same verifier as production: VERIFIER_PROMPT, VERIFIER_PANEL and VERIFIER_VOTES.
	ver := verifier.NewMajority(verifier.NewWithPanel(model, searcher, verifier.Options{Model: cfg.VerifierModel, MaxToolCalls: cfg.VerifierMaxToolCalls, MaxOutputTokens: 6000, Prompt: cfg.VerifierPrompt},
		cfg.VerifierPanel == "on", cfg.VerifierPanelLow, cfg.VerifierPanelHigh), cfg.VerifierVotes)
	tmpls, err := planner.LoadTemplates()
	if err != nil {
		return err
	}
	planOpts := planner.Options{PlanConfidence: cfg.PlanConfidence, Concurrency: cfg.PlannerConcurrency, MaxMoves: cfg.PlanMaxMoves}
	if cfg.NearMissAdvice {
		planOpts.NearMiss = nearmiss.New(model, cfg.VerifierModel, searcher, 1500) // D81, uncached here
		if cfg.FollowAdvice {
			planOpts.FollowAdvice, planOpts.Mapper = true, nearmiss.NewMapper(model, cfg.VerifierModel) // D85
		}
	}
	if *regime == router.RegimeUSMCA {
		// The Part 102 engine as the server builds it (cmd/server: USMCA_ENGINE=on).
		ulabels, err := corpus.LoadLabels(*usmcaLabels)
		if err != nil {
			return err
		}
		upool, err := corpus.Load(ulabels, *factsDir)
		if err != nil {
			return err
		}
		if upool.Len() == 0 {
			return fmt.Errorf("plan: no ruling in %s has facts in %s", *usmcaLabels, *factsDir)
		}
		uopts := engine.USMCAProduction()
		uopts.Model, uopts.K, uopts.MaxOutputTokens = cfg.EngineModel, cfg.PrecedentK, cfg.MaxOutputTokens
		usmca := engine.New(model, retrieval.NewBM25(upool.Rulings(), engine.PrecedentText), uopts)
		planOpts.Engines = map[string]planner.Predictor{router.RegimeUSMCA: usmca}
		planOpts.CodesInFacts = map[string]bool{router.RegimeUSMCA: true}
	}
	pl := planner.New(tmpls, eng, ver, planOpts)
	var tally llm.CostTally // the gateway's own per-call cost; zero on Vertex AI
	res, err := pl.Plan(llm.ContextWithCostTally(ctx, &tally), req)
	if err != nil {
		return err
	}
	cost := evalkit.Cost(evalkit.PromoPrices, cfg.EngineModel, res.Stats.EngineInTok, res.Stats.EngineOutTok) +
		evalkit.Cost(evalkit.PromoPrices, cfg.VerifierModel, res.Stats.VerifierInTok+res.Stats.NearMissInTok, res.Stats.VerifierOutTok+res.Stats.NearMissOutTok)
	costList := evalkit.Cost(evalkit.ListPrices, cfg.EngineModel, res.Stats.EngineInTok, res.Stats.EngineOutTok) +
		evalkit.Cost(evalkit.ListPrices, cfg.VerifierModel, res.Stats.VerifierInTok+res.Stats.NearMissInTok, res.Stats.VerifierOutTok+res.Stats.NearMissOutTok)
	nearCost := evalkit.Cost(evalkit.PromoPrices, cfg.VerifierModel, res.Stats.NearMissInTok, res.Stats.NearMissOutTok)
	out := struct {
		planner.Result
		Provider    string  `json:"llm_provider"`
		Search      string  `json:"verifier_search"`
		CostUSD     float64 `json:"cost_usd_promo"`
		CostUSDList float64 `json:"cost_usd_list"`
		GatewayUSD  float64 `json:"gateway_cost_usd,omitempty"`
		Config      string  `json:"verifier_config"`
		NoCache     bool    `json:"llm_no_cache,omitempty"`
		Revision    string  `json:"git_revision"`
		Time        string  `json:"time"`
	}{res, cfg.LLMProvider, *searchMode, cost, costList, tally.USD(),
		fmt.Sprintf("prompt=%s panel=%s votes=%d near_miss=%t follow=%t bar=%.2f", cfg.VerifierPrompt, cfg.VerifierPanel, cfg.VerifierVotes, cfg.NearMissAdvice, cfg.FollowAdvice, cfg.PlanConfidence),
		os.Getenv("LITELLM_NO_CACHE") == "1", revision(), time.Now().Format(time.RFC3339)}
	suffix := ""
	if *tag != "" {
		suffix = "-" + *tag
	}
	path := filepath.Join(*resultsDir, fmt.Sprintf("plan-%s-%s-verifier-%s%s%s-%s.json", req.CaseID, *searchMode, cfg.VerifierPrompt, votesSuffix(cfg.VerifierVotes), suffix, cfg.LLMProvider))
	b, _ := json.MarshalIndent(out, "", "  ")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return err
	}
	fmt.Printf("## Plan %s → %s (%s, verifier search %s)\n\n", req.CaseID, res.Target, cfg.LLMProvider, *searchMode)
	b0 := res.Baseline.Prediction
	fmt.Printf("- Current setup: engine predicts **%s** (confidence %.2f), cited %v\n", b0.Origin, b0.Confidence, b0.CitedRulings)
	fmt.Printf("- Shown (precedent-supported, verifier confidence ≥ %.2f):\n", cfg.PlanConfidence)
	fromAdvice := func(p domain.Plan) string {
		if p.FromAdvice {
			return " [followed from near-miss advice]"
		}
		return ""
	}
	for _, p := range res.Plans {
		fmt.Printf("  - ✓ %s%s — engine %s %.2f; supporting %s, counter %s, verifier conf %.2f%s; duty effect $%.0f/yr, added cost $%.0f/yr (illustrative)\n",
			p.Description, fromAdvice(p), p.Prediction.Origin, p.Prediction.Confidence, p.Verdict.SupportingRuling, p.Verdict.CounterRuling, p.Verdict.Confidence, agreement(p.Verdict), p.DutyEffectYear, p.AddedCostYear)
	}
	fmt.Println("- Rejected:")
	for _, p := range res.Rejected {
		extra := ""
		if p.Verdict != nil {
			extra = fmt.Sprintf(" (verifier: supported=%t conf %.2f%s, supporting %q, counter %q)", p.Verdict.PrecedentSupported, p.Verdict.Confidence, agreement(p.Verdict), p.Verdict.SupportingRuling, p.Verdict.CounterRuling)
		}
		fmt.Printf("  - ✗ %s%s — %s%s\n", p.Description, fromAdvice(p), p.RejectionReason, extra)
		if n := p.NearMiss; n != nil {
			fmt.Printf("    - near miss vs %s: %s Would match: %s\n", n.Ruling, n.Difference, n.WouldMatch)
		}
	}
	if res.Backtest != nil {
		fmt.Printf("- Backtest vs %s: plan moved %v; actual change also moved %v\n", res.Backtest.Source, res.Backtest.MovedByPlan, res.Backtest.NotMovedByPlan)
	}
	s := res.Stats
	fmt.Printf("- Full-plan cost: %d moves, %d engine calls, %d verifier runs (%d model calls, %d tool calls), %d in / %d out tokens, $%.3f promo / $%.3f list, %.0f s wall\n",
		s.MovesSearched, s.EngineCalls, s.VerifierRuns, s.VerifierModel, s.ToolCalls, s.InputTokens, s.OutputTokens, cost, costList, s.WallSeconds)
	if s.NearMissCalls > 0 {
		fmt.Printf("- Near-miss advice: %d calls, %d in / %d out tokens, $%.4f promo\n", s.NearMissCalls, s.NearMissInTok, s.NearMissOutTok, nearCost)
	}
	if s.AdviceFollowed > 0 {
		fmt.Printf("- Followed near-miss advice: %d move\n", s.AdviceFollowed)
	}
	if g := tally.USD(); g > 0 {
		fmt.Printf("- Gateway-reported cost: $%.4f\n", g)
	}
	fmt.Println("- Saved:", path)
	return nil
}

// agreement is ", panel 2/3" when the precedent-check panel ran (D83).
func agreement(v *domain.Verdict) string {
	if v == nil || v.Agreement == "" {
		return ""
	}
	return ", panel " + v.Agreement
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
	cfg, err := loadConfig()
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
	Type          string   `json:"type"`
	Tier          string   `json:"tier"`
	Rulings       []string `json:"rulings"`
	Date          string   `json:"date"`
	Difference    string   `json:"difference"`
	OriginChanged bool     `json:"origin_changed"`
	VariantA      struct {
		Ruling string `json:"ruling"`
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
	prompt := fl.String("verifier", "", "verifier prompt: v1, v2 (D74) or v3 (D75); default VERIFIER_PROMPT, as production")
	tag := fl.String("tag", "", "suffix for the result file, e.g. run2, to score the same prompt twice")
	final := fl.Bool("confirm-final", false, "score the 2026 flip pairs (held out) instead of the earlier ones; run once with the frozen verifier")
	post := fl.Bool("post", false, "score the post-cutoff pairs (eval/data/flips_post.jsonl, D170) once with the production verifier")
	datesPath := fl.String("dates", "../eval/data/labels_final2.jsonl", "with -post: labels of the post-cutoff rulings, for their dates only (the precedent corpus stays -labels)")
	_ = fl.Parse(args)
	if *post {
		if *final {
			return errors.New("flips: -post and -confirm-final are different sets")
		}
		if *flipsPath == "../eval/data/flips.jsonl" {
			*flipsPath = "../eval/data/flips_post.jsonl"
		}
	}
	cfg, err := loadConfig()
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
	if *post {
		later, err := corpus.LoadLabels(*datesPath)
		if err != nil {
			return err
		}
		for _, l := range later {
			dates[l.Ruling] = l.Date
		}
	}
	if *prompt == "" {
		*prompt = cfg.VerifierPrompt
	}
	searcher, err := verifier.NewLocal(labels, *factsDir, *rawDir)
	if err != nil {
		return err
	}
	ver := verifier.NewMajority(verifier.New(model, searcher, verifier.Options{Model: cfg.VerifierModel, MaxToolCalls: cfg.VerifierMaxToolCalls, MaxOutputTokens: 6000, Prompt: *prompt}), cfg.VerifierVotes)

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
		// Dev is the pre-2026 pairs; -confirm-final scores the 2026 pairs.
		// The heat-gun pair (the demo product) is never scored, so no
		// verifier change can be fitted to the demo (D74).
		if it.ID == "BA-N328954-N357771" {
			continue
		}
		if *post || (it.Date < "2026-01-01") != *final {
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
	split := "dev"
	if *final {
		split = "final"
	}
	if *post {
		split = "post"
	}
	name := "verifier"
	if *prompt != "v1" {
		name = "verifier-" + *prompt
	}
	name += votesSuffix(cfg.VerifierVotes)
	if *tag != "" {
		name += "-" + *tag
	}
	outPath := filepath.Join(*resultsDir, fmt.Sprintf("flips-%s-%s-%s-%s.jsonl", split, name, mode, cfg.LLMProvider))
	if _, err := os.Stat(outPath); err == nil && *post {
		return fmt.Errorf("%s exists; the post-cutoff pairs are scored once (D170)", outPath)
	}
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	type row struct {
		ID                 string   `json:"id"`
		Type               string   `json:"type,omitempty"`
		Tier               string   `json:"tier,omitempty"`
		Before             string   `json:"precedents_before,omitempty"`
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
			rw := row{ID: it.ID, Type: it.Type, Tier: it.Tier, OriginChanged: it.OriginChanged, Mode: mode, ModelID: cfg.VerifierModel, Provider: cfg.LLMProvider}
			if *post {
				rw.Before = before
			}
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
	if *post {
		fmt.Printf("## %s on post-cutoff flip pairs (%s, %s) — FROZEN, scored once (D170)\n\n", name, mode, cfg.LLMProvider)
	} else if *final {
		fmt.Printf("## %s on held-out 2026 flip pairs (%s, %s) — HELD OUT, scored once\n\n", name, mode, cfg.LLMProvider)
	} else {
		fmt.Printf("## %s on development flip pairs (%s, %s) — DEVELOPMENT SET ONLY\n\n", name, mode, cfg.LLMProvider)
	}
	n := len(rows) - errs
	set := map[bool]string{false: "pre-2026", true: "2026"}[*final]
	if *post {
		set = "post-cutoff"
	}
	fmt.Printf("- Items: %d (%s, heat-gun pair excluded; %d failed runs)\n", len(rows), set, errs)
	fmt.Printf("- Precision when it says the move changes origin: %s\n", fmtP(tp, tp+fp))
	fmt.Printf("- Recall (real changes caught): %s\n", fmtP(tp, tp+fn))
	fmt.Printf("- Specificity (no-change pairs correctly rejected): %s\n", fmtP(tn, tn+fp))
	fmt.Printf("- Precedent-supported (change + cited supporting ruling it actually read): correct %s\n", fmtP(stp, stp+sfp))
	if n > 0 {
		fmt.Printf("- Cost per verification: $%.3f (promo)\n", cost/float64(n))
	}
	// Precedent-supported verdicts by confidence: how many would clear the
	// plan bar, and whether they were real changes (D74).
	for _, bar := range []float64{config.DefaultPlanBar, 0.85} {
		var k, m int
		for _, r := range rows {
			if r.Error == "" && r.PrecedentSupported && r.Confidence >= bar {
				m++
				if r.OriginChanged {
					k++
				}
			}
		}
		fmt.Printf("- Precedent-supported with confidence ≥ %.2f: %d of %d real changes (%s)\n", bar, m, tp+fn, fmtP(k, m))
	}
	fmt.Println("- Saved:", outPath)
	return nil
}

// summaryStatusProxy is the results-page status while the headline numbers
// still come from the LiteLLM proxy.
const summaryStatusProxy = "proxy, development only: produced through the LiteLLM proxy; official numbers come from the Vertex AI re-run"

// summaryRun is the engine result file and router scope file for one split.
// Official is true when both are the Vertex AI run.
type summaryRun struct {
	File, ScopeFile string
	Official        bool
}

// adjustedDevItem is the development accuracy after the D23 scoring
// adjustments (463 rulings), the figure docs/EVALUATION.md uses.
func adjustedDevItem(resultsDir string, run summaryRun) (evalsummary.Item, error) {
	rows, err := readRows(filepath.Join(resultsDir, run.File), nil)
	if err != nil {
		return evalsummary.Item{}, err
	}
	adj, err := evalkit.LoadAdjustments(filepath.Join(resultsDir, "..", "data", "scoring_adjustments.json"))
	if err != nil {
		return evalsummary.Item{}, err
	}
	s := evalkit.Summarize(adj.Apply(rows))
	return evalsummary.Item{Key: "dev_accuracy_adjusted", Label: fmt.Sprintf("Development (%d rulings after scoring adjustments, D23) accuracy", s.N),
		Value: round(s.Acc), CILow: round(s.Lo), CIHigh: round(s.Hi), N: s.N, Split: "dev", DevelopmentOnly: true,
		Source: "eval/results/" + run.File + " + eval/data/scoring_adjustments.json"}, nil
}

// baselineSummaryItems reads the rule baselines recomputed by
// `eval baselines` on the first held-out split (D155).
func baselineSummaryItems(resultsDir string) ([]evalsummary.Item, error) {
	const file = "baselines-final.json"
	b, err := os.ReadFile(filepath.Join(resultsDir, file))
	if err != nil {
		return nil, err
	}
	var res map[string]json.RawMessage
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, err
	}
	var majority string
	_ = json.Unmarshal(res["majority_country"], &majority)
	var out []evalsummary.Item
	for _, r := range []struct{ rule, key, label string }{
		{"first_mentioned", "baseline_first_mentioned", "Baseline: first country named in the facts (recomputed)"},
		{"most_mentioned", "baseline_most_mentioned", "Baseline: most-mentioned country (recomputed)"},
		{"majority_origin", "baseline_majority", "Baseline: always the development set's most common origin (recomputed)"},
	} {
		var v struct {
			N        int       `json:"n"`
			Accuracy float64   `json:"accuracy"`
			CI       []float64 `json:"ci95"`
		}
		if err := json.Unmarshal(res[r.rule], &v); err != nil || len(v.CI) != 2 {
			return nil, fmt.Errorf("%s: %s: %v", file, r.rule, err)
		}
		label := r.label
		if r.rule == "majority_origin" && majority != "" {
			label = "Baseline: always " + majority + ", the development set's most common origin (recomputed)"
		}
		out = append(out, evalsummary.Item{Key: r.key, Label: label, Value: v.Accuracy, CILow: v.CI[0], CIHigh: v.CI[1], N: v.N, Split: "final",
			Source: "eval/results/" + file + " (go run ./cmd/eval baselines, no model)"})
	}
	return out, nil
}

// final2SummaryItems are the second held-out split's numbers (D154): one run
// of the frozen engine through the gateway, with the router.
func final2SummaryItems(resultsDir string) ([]evalsummary.Item, error) {
	const file = "final2-gemini-4-argon-prec-engine-v1-litellm-router.jsonl"
	rows, err := readRows(filepath.Join(resultsDir, file), nil)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].TriageRoute = "needs_review"
		if rows[i].InScope && rows[i].Error == "" && rows[i].Confidence >= config.DefaultFastPath {
			rows[i].TriageRoute = "auto"
		}
	}
	s := evalkit.Summarize(rows)
	// Six decimals, so a page that rounds to one decimal of a percent shows
	// the same interval as the docs (96.65 must not become 96.7).
	r6 := func(x float64) float64 { return math.Round(x*1e6) / 1e6 }
	src := "eval/results/" + file
	label := fmt.Sprintf("Second held-out (%d rulings, Sep–Nov 2026, gateway)", s.N)
	fp, flo, fhi := evalkit.Wilson(s.FastK, s.FastN)
	return []evalsummary.Item{
		{Key: "final2_accuracy", Label: label + " accuracy", Value: r6(s.Acc), CILow: r6(s.Lo), CIHigh: r6(s.Hi), N: s.N, Split: "final2", Source: src},
		{Key: "final2_fastpath_accuracy", Label: label + fmt.Sprintf(": fast path (calibrated confidence ≥ %.2f, the same cases as stated confidence ≥ %.2f, and in scope) accuracy", config.DefaultCalibratedFastPath, config.DefaultFastPath), Value: r6(fp), CILow: r6(flo), CIHigh: r6(fhi), N: s.FastN, Split: "final2", Source: src},
		{Key: "final2_fastpath_coverage", Label: label + ": fast path coverage", Value: r6(float64(s.FastN) / float64(s.N)), N: s.N, Split: "final2", Source: src},
	}, nil
}

// calibrationSummaryItems are the held-out calibration figures of the map the
// server embeds (D160): mean stated confidence and mean calibrated
// probability against accuracy, ECE with one bin per value on each side, and
// the stated ECE scored on the calibrated bins (D262, #493). The map was fitted
// on dev only; these rows are the held-out check.
func calibrationSummaryItems(resultsDir string, run summaryRun) ([]evalsummary.Item, error) {
	rows, err := readRows(filepath.Join(resultsDir, run.File), nil)
	if err != nil {
		return nil, err
	}
	m, err := calibrate.Load()
	if err != nil {
		return nil, err
	}
	var sc []calibrate.Scored
	for _, r := range rows {
		sc = append(sc, calibrate.Scored{Stated: r.Confidence, OK: r.Correct && r.Error == ""})
	}
	rep := m.Evaluate(sc)
	// Six decimals: a page that rounds to one decimal of a percent must show
	// the docs' 94.7%, not 94.8% from a value already rounded to 0.9475.
	round := func(x float64) float64 { return math.Round(x*1e6) / 1e6 }
	src := "eval/results/" + run.File + " + backend/internal/calibrate/map.json (" + m.Version + ", fitted on " + m.FittedOn + ")"
	label := fmt.Sprintf("Held-out (%d rulings), calibration", rep.N)
	// D262 / #493: the two ECE figures use one bin per value on their own
	// side, and the map pools levels. Say so in the served label and give the
	// same-bin figure beside it; otherwise the drop looks better than it is.
	bins := fmt.Sprintf(" (different bins: %d stated levels, %d calibrated levels on these rows)", rep.StatedLevels, rep.CalibratedLevels)
	return []evalsummary.Item{
		{Key: "final_calibration_mean_stated", Label: label + ": mean stated confidence", Value: round(rep.MeanStated), N: rep.N, Split: "final", Source: src},
		{Key: "final_calibration_mean_calibrated", Label: label + ": mean calibrated probability", Value: round(rep.MeanCalibrated), N: rep.N, Split: "final", Source: src},
		{Key: "final_calibration_ece_stated", Label: label + ": ECE of stated confidence (one bin per stated value)" + bins, Value: round(rep.ECEStated), N: rep.N, Split: "final", Source: src},
		{Key: "final_calibration_ece_calibrated", Label: label + ": ECE of calibrated probability (one bin per calibrated value)" + bins, Value: round(rep.ECECalibrated), N: rep.N, Split: "final", Source: src},
		{Key: "final_calibration_ece_stated_same_bins", Label: label + fmt.Sprintf(": ECE of stated confidence on the same %d calibrated bins", rep.CalibratedLevels), Value: round(rep.ECEStatedOnCalibratedBins), N: rep.N, Split: "final", Source: src},
	}, nil
}

// selectSummaryRun prefers the Vertex AI engine and scope files when both
// exist, and otherwise the LiteLLM proxy files. A Vertex engine file is never
// paired with proxy router labels.
func selectSummaryRun(resultsDir, split string) (summaryRun, error) {
	vertexEngine := split + "-gemini-4-argon-prec-engine-v1-vertex.jsonl"
	proxyEngine := split + "-gemini-4-argon-prec-engine-v1-litellm.jsonl"
	vertexScope := "scope-" + split + "-router-v2-vertex.jsonl"
	proxyScope := "scope-" + split + "-router-v2.jsonl"
	if fileExists(filepath.Join(resultsDir, vertexEngine)) {
		if !fileExists(filepath.Join(resultsDir, vertexScope)) {
			return summaryRun{}, fmt.Errorf("%s: %s exists but %s does not; refusing to pair Vertex predictions with proxy router labels", split, vertexEngine, vertexScope)
		}
		return summaryRun{File: vertexEngine, ScopeFile: vertexScope, Official: true}, nil
	}
	if !fileExists(filepath.Join(resultsDir, proxyEngine)) {
		return summaryRun{}, fmt.Errorf("%s: neither %s nor %s", split, vertexEngine, proxyEngine)
	}
	if !fileExists(filepath.Join(resultsDir, proxyScope)) {
		return summaryRun{}, fmt.Errorf("%s: missing %s", split, proxyScope)
	}
	return summaryRun{File: proxyEngine, ScopeFile: proxyScope}, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// summaryStatus labels the payload. Held-out Vertex numbers are official.
// Development-split rows stay the development set. Verifier and plan figures
// are still proxy until those runs are repeated on Vertex.
func summaryStatus(finalOfficial, devOfficial bool) string {
	if !finalOfficial {
		return summaryStatusProxy
	}
	dev := "Development-split engine numbers are the development set on Vertex AI."
	if !devOfficial {
		dev = "Development-split engine numbers are still proxy, development only."
	}
	return "official: held-out engine and fast path are Gemini 4 Argon on Vertex AI (engine-v1, BM25, K=5). " + dev + " Verifier figures and the heat-gun plan's cost and time are proxy development runs; the heat-gun plan the landing shows (check 0.96) is the recorded 18 Oct production run (docs/DEMO.md, section 6)."
}

func summaryCmd(args []string) error {
	fl := flag.NewFlagSet("summary", flag.ExitOnError)
	resultsDir := fl.String("results", "../eval/results", "results directory")
	outPath := fl.String("out", "internal/evalsummary/summary.json", "output file (embedded in the server)")
	check := fl.Bool("check", false, "verify the committed summary matches the generated data (ignores generated_at)")
	_ = fl.Parse(args)
	type item = evalsummary.Item
	var items []item
	pct := func(k, n int) (float64, float64, float64) { return evalkit.Wilson(k, n) }
	r4 := func(x float64) float64 { return float64(int(x*10000+0.5)) / 10000 }

	finalRun, err := selectSummaryRun(*resultsDir, "final")
	if err != nil {
		return err
	}
	devRun, err := selectSummaryRun(*resultsDir, "dev")
	if err != nil {
		return err
	}

	addRun := func(run summaryRun, split, prefix, label string) error {
		rows, err := readRows(filepath.Join(*resultsDir, run.File), nil)
		if err != nil {
			return err
		}
		sc, err := readScope(filepath.Join(*resultsDir, run.ScopeFile))
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
			if in && rows[i].Error == "" && rows[i].Confidence >= config.DefaultFastPath {
				rows[i].TriageRoute = "auto"
			}
		}
		s := evalkit.Summarize(rows)
		src := "eval/results/" + run.File
		scopeSrc := src + " + eval/results/" + run.ScopeFile
		via := "via proxy"
		if run.Official {
			via = "Vertex AI"
		}
		items = append(items, item{Key: prefix + "_accuracy", Label: label + " accuracy", Value: r4(s.Acc), CILow: r4(s.Lo), CIHigh: r4(s.Hi), N: s.N, Split: split, Source: src})
		p, lo, hi := pct(s.AtLeast95.K, s.AtLeast95.N)
		items = append(items, item{Key: prefix + "_conf95_accuracy", Label: label + ": accuracy when confidence ≥ 0.95", Value: r4(p), CILow: r4(lo), CIHigh: r4(hi), N: s.AtLeast95.N, Split: split, Source: src})
		items = append(items, item{Key: prefix + "_conf95_coverage", Label: label + ": share of cases with confidence ≥ 0.95", Value: r4(float64(s.AtLeast95.N) / float64(s.N)), N: s.N, Split: split, Source: src})
		p, lo, hi = pct(s.FastK, s.FastN)
		items = append(items, item{Key: prefix + "_fastpath_accuracy", Label: label + fmt.Sprintf(": fast path (calibrated confidence ≥ %.2f, the same cases as stated confidence ≥ %.2f, and in scope) accuracy", config.DefaultCalibratedFastPath, config.DefaultFastPath), Value: r4(p), CILow: r4(lo), CIHigh: r4(hi), N: s.FastN, Split: split, Source: scopeSrc})
		items = append(items, item{Key: prefix + "_fastpath_coverage", Label: label + ": fast path coverage", Value: r4(float64(s.FastN) / float64(s.N)), N: s.N, Split: split, Source: scopeSrc})
		items = append(items, item{Key: prefix + "_cost_per_case_usd", Label: label + ": cost per prediction (USD, promo price)", Value: r4(s.CostPerCase), N: s.N, Split: split, Source: src})
		items = append(items, item{Key: prefix + "_latency_p50_s", Label: label + ": latency p50 (s, " + via + ")", Value: float64(s.LatencyP50) / 1000, Split: split, Source: src})
		items = append(items, item{Key: prefix + "_latency_p95_s", Label: label + ": latency p95 (s, " + via + ")", Value: float64(s.P95) / 1000, Split: split, Source: src})
		return nil
	}
	if err := addRun(finalRun, "final", "final", "Held-out (377 rulings, Jan–Sep 2026)"); err != nil {
		return err
	}
	if err := addRun(devRun, "dev", "dev", "Development (469 rulings, Jul–Dec 2025)"); err != nil {
		return err
	}
	for i := range items {
		if items[i].Split == "dev" {
			items[i].DevelopmentOnly = true
		}
	}
	// The research write-up these figures come from is not in this
	// repository, so the source says it is unavailable (docs/NUMBERS.md).
	const v2 = "unavailable: research write-up RESULTS-v2.md, not in this repository (research run, Python, LiteLLM proxy)"
	// Dev after the D23 scoring adjustments (463 rulings), next to the
	// unadjusted dev rows above, so the page can say which is which.
	// Each of these is skipped, with a note, when its input file is absent
	// (the command's tests run on a minimal results directory).
	if adjDev, err := adjustedDevItem(*resultsDir, devRun); errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "summary: skip adjusted dev: %v\n", err)
	} else if err != nil {
		return err
	} else {
		items = append(items, adjDev)
	}
	if baseItems, err := baselineSummaryItems(*resultsDir); errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "summary: skip baselines: %v\n", err)
	} else if err != nil {
		return err
	} else {
		items = append(items, baseItems...)
	}
	if calItems, err := calibrationSummaryItems(*resultsDir, finalRun); errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "summary: skip calibration: %v\n", err)
	} else if err != nil {
		return err
	} else {
		items = append(items, calItems...)
	}
	if f2Items, err := final2SummaryItems(*resultsDir); errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "summary: skip second held-out split: %v\n", err)
	} else if err != nil {
		return err
	} else {
		items = append(items, f2Items...)
	}
	if ppItems, err := plannerPostSummaryItems(*resultsDir); errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "summary: skip post-cutoff flip pairs: %v\n", err)
	} else if err != nil {
		return err
	} else {
		items = append(items, ppItems...)
	}
	if pcItems, err := plannerPostCombinedItems(*resultsDir); errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "summary: skip combined post-cutoff flip pairs: %v\n", err)
	} else if err != nil {
		return err
	} else {
		items = append(items, pcItems...)
	}
	if rpItems, err := repeatabilityItems(*resultsDir); errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "summary: skip repeatability: %v\n", err)
	} else if err != nil {
		return err
	} else {
		items = append(items, rpItems...)
	}
	if hrItems, err := heldoutRepeatItems(*resultsDir); errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "summary: skip held-out repeatability: %v\n", err)
	} else if err != nil {
		return err
	} else {
		items = append(items, hrItems...)
	}
	if psItems, err := planStabilityItems(*resultsDir); errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "summary: skip plan stability: %v\n", err)
	} else if err != nil {
		return err
	} else {
		items = append(items, psItems...)
	}
	if prItems, err := planRankItems(*resultsDir); errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "summary: skip plan ranking: %v\n", err)
	} else if err != nil {
		return err
	} else {
		items = append(items, prItems...)
	}
	items = append(items,
		item{Key: "research_final_accuracy", Label: "Research run: Gemini 4 Argon + dated precedents", Value: 0.891, CILow: 0.856, CIHigh: 0.919, N: 377, Split: "final", Source: v2},
		item{Key: "baseline_final_assembly", Label: "Baseline: final-assembly country", Value: 0.406, CILow: 0.358, CIHigh: 0.456, N: 377, Split: "final", Source: v2},
		item{Key: "baseline_last_mentioned", Label: "Baseline: last country mentioned (research definition)", Value: 0.398, CILow: 0.350, CIHigh: 0.448, N: 377, Split: "final", Source: v2},
		item{Key: "research_argon_facts_only", Label: "Research: Gemini 4 Argon, facts only (no precedents)", Value: 0.859, CILow: 0.821, CIHigh: 0.891, N: 377, Split: "final", Source: v2},
		item{Key: "change_detection_heldout", Label: "Change detection from scratch, held-out 2026 pairs (weak)", Value: 15.0 / 22, CILow: 0.47, CIHigh: 0.84, N: 22, Split: "final", Source: v2},
		item{Key: "change_detection_always_change_baseline", Label: "Baseline: always predict a change (same 22 pairs)", Value: 14.0 / 22, N: 22, Split: "final", Source: v2},
		item{Key: "change_detection_balanced_accuracy", Label: "Change detection balanced accuracy (same 22 pairs)", Value: 0.616, N: 22, Split: "final", Source: v2},
	)
	for _, mode := range []string{"unknown-a", "known-a"} {
		f, err := os.Open(filepath.Join(*resultsDir, "flips-dev-verifier-v2-"+mode+"-litellm.jsonl")) // verifier-v2, in production (D74)
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
		src := "eval/results/flips-dev-verifier-v2-" + mode + "-litellm.jsonl"
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
	textileItems, err := textileSummaryItems(*resultsDir)
	if err != nil {
		return err
	}
	items = append(items, textileItems...)
	regimeItems, err := regimeSummaryItems(*resultsDir)
	if err != nil {
		return err
	}
	items = append(items, regimeItems...)
	scopeItems, err := scopeSummaryItems(*resultsDir, "../eval/splits/splits_textile.json")
	if err != nil {
		return err
	}
	items = append(items, scopeItems...)
	routerItems, err := routerSummaryItems(*resultsDir)
	if err != nil {
		return err
	}
	items = append(items, routerItems...)
	cal, err := verifierCalibration(*resultsDir, config.DefaultPlanBar)
	if err != nil {
		return err
	}
	out := evalsummary.Summary{GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Status: summaryStatus(finalRun.Official, devRun.Official), Items: items, VerifierCalibration: cal}
	b, _ := json.MarshalIndent(out, "", "  ")
	if *check {
		rawExisting, err := os.ReadFile(*outPath)
		if err != nil {
			return fmt.Errorf("summary check: read %s: %w", *outPath, err)
		}
		var existing evalsummary.Summary
		if err := json.Unmarshal(rawExisting, &existing); err != nil {
			return fmt.Errorf("summary check: parse %s: %w", *outPath, err)
		}
		// generated_at is build metadata, not a result; compare everything else.
		out.GeneratedAt = existing.GeneratedAt
		b, _ = json.MarshalIndent(out, "", "  ")
		want := append(b, '\n')
		if string(want) != string(rawExisting) {
			return fmt.Errorf("summary check: %s is stale — regenerate with `go run ./cmd/eval summary` (make eval-summary) and commit it", *outPath)
		}
		fmt.Fprintf(os.Stderr, "summary check: %s matches %d generated items\n", *outPath, len(items))
		return nil
	}
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
	names := fl.String("split-names", "dev,final", "splits to scan (final2 with -splits ../eval/splits/splits_final2.json)")
	_ = fl.Parse(args)
	sp, err := corpus.LoadSplits(*splitsPath)
	if err != nil {
		return err
	}
	out := map[string]map[string]any{}
	for _, split := range splitComma(*names) {
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
	case config.ProviderVertex:
		return llm.NewVertex(ctx, llm.VertexOptions{Project: cfg.GCPProject, Location: cfg.GCPLocation, Timeout: cfg.LLMTimeout})
	}
	return nil, fmt.Errorf("no embedder for provider %q", cfg.LLMProvider)
}

func compareCmd(args []string) error {
	fl := flag.NewFlagSet("compare", flag.ExitOnError)
	a := fl.String("a", "", "baseline result file")
	b := fl.String("b", "", "variant result file")
	adjustFile := fl.String("adjust", "", "scoring adjustments (eval/data/scoring_adjustments.json)")
	_ = fl.Parse(args)
	ra, err := readRows(*a, nil)
	if err != nil {
		return err
	}
	rb, err := readRows(*b, nil)
	if err != nil {
		return err
	}
	if *adjustFile != "" {
		adj, err := evalkit.LoadAdjustments(*adjustFile)
		if err != nil {
			return err
		}
		ra, rb = adj.Apply(ra), adj.Apply(rb)
	}
	ok := func(r evalkit.Row) bool { return r.Correct && r.Error == "" }
	byID := map[string]evalkit.Row{}
	for _, r := range ra {
		byID[r.Ruling] = r
	}
	var n, ka, kb, fixed, broken int
	for _, y := range rb {
		x, found := byID[y.Ruling]
		if !found {
			continue
		}
		n++
		if ok(x) {
			ka++
		}
		if ok(y) {
			kb++
		}
		switch {
		case ok(y) && !ok(x):
			fixed++
		case ok(x) && !ok(y):
			broken++
		}
	}
	pa, la, ha := evalkit.Wilson(ka, n)
	pb, lb, hb := evalkit.Wilson(kb, n)
	fmt.Printf("n=%d | A %.1f%% (%.1f–%.1f) | B %.1f%% (%.1f–%.1f) | B fixed %d, broke %d | exact McNemar p=%.3f\n",
		n, 100*pa, 100*la, 100*ha, 100*pb, 100*lb, 100*hb, fixed, broken, mcnemarExact(fixed, broken))
	return nil
}

// mcnemarExact is the two-sided exact binomial test on discordant pairs.
func mcnemarExact(b, c int) float64 {
	n := b + c
	if n == 0 {
		return 1
	}
	k := min(b, c)
	var tail float64
	for i := 0; i <= k; i++ {
		tail += math.Exp(lchoose(n, i) - float64(n)*math.Ln2)
	}
	return math.Min(1, 2*tail)
}

func lchoose(n, k int) float64 {
	a, _ := math.Lgamma(float64(n + 1))
	b, _ := math.Lgamma(float64(k + 1))
	c, _ := math.Lgamma(float64(n - k + 1))
	return a - b - c
}

const learnSystem = `You are auditing an origin-prediction model on U.S. CBP country-of-origin rulings. You get a case's facts, the origin CBP actually held, the model's wrong answer, and earlier CBP rulings (facts + holding), all dated before the case.

Decide whether the earlier rulings contain a rule or a closely analogous holding that, applied to this case, gives CBP's answer. Be strict: the earlier ruling must involve the same kind of product or the same kind of processing question, and its reasoning must point to CBP's answer here.

Classify the error:
- "learnable_precedent": an earlier ruling here establishes the rule.
- "facts_underdetermined": the facts given do not contain what decided the case (e.g. CBP relied on information not in the facts).
- "cbp_inconsistent": earlier rulings point the other way (toward the model's answer).
- "label_or_scoring": the gold label looks wrong, or the case has several products/scenarios.
- "no_precedent": none of these rulings is on point.

Return ONLY JSON.`

var learnSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"category":     map[string]any{"type": "string", "enum": []string{"learnable_precedent", "facts_underdetermined", "cbp_inconsistent", "label_or_scoring", "no_precedent"}},
		"ruling":       map[string]any{"type": []string{"string", "null"}},
		"rule":         map[string]any{"type": "string"},
		"model_missed": map[string]any{"type": "string"},
	},
	"required":             []string{"category", "ruling", "rule", "model_missed"},
	"additionalProperties": false,
}

func learnabilityCmd(args []string) error {
	fl := flag.NewFlagSet("learnability", flag.ExitOnError)
	in := fl.String("in", "../eval/results/dev-gemini-4-argon-prec-engine-v1-litellm.jsonl", "engine result file (dev)")
	k := fl.Int("k", 10, "earlier rulings shown to the judge")
	vectorsPath := fl.String("vectors", "../data/corpus/embeddings-768.bin", "embedding index")
	labelsPath := fl.String("labels", "../eval/data/labels.jsonl", "labels")
	factsDir := fl.String("facts", "../data/corpus/facts", "facts")
	out := fl.String("out", "../eval/results/learnability-dev.jsonl", "output")
	_ = fl.Parse(args)
	rows, err := readRows(*in, nil)
	if err != nil {
		return err
	}
	if len(rows) > 0 && isHeldOut(rows[0].Split) {
		return errors.New("learnability uses gold labels in the prompt: dev only")
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	ctx := context.Background()
	model, err := newModel(ctx, cfg, slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
	if err != nil {
		return err
	}
	emb, err := newEmbedder(ctx, cfg)
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
	vecs, err := retrieval.LoadVectors(*vectorsPath)
	if err != nil {
		return err
	}
	vector := &retrieval.Hybrid{BM25: retrieval.NewBM25(store.Rulings(), engine.PrecedentText), Vectors: vecs, Embedder: emb}
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	defer f.Close()
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	counts := map[string]int{}
	inShown := 0
	for _, r := range rows {
		if r.Correct || r.Error != "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(r evalkit.Row) {
			defer wg.Done()
			defer func() { <-sem }()
			c, _ := store.Get(r.Ruling)
			hits, err := vector.Search(ctx, c.Facts, *k, retrieval.PrecedentFilter(domain.Case{ID: c.ID, Date: c.Date}))
			if err != nil {
				fmt.Fprintln(os.Stderr, r.Ruling, err)
				return
			}
			var b strings.Builder
			for _, h := range hits {
				fmt.Fprintf(&b, "[%s | %s] FACTS: %s\nCBP HOLDING: %s\n\n", h.Ruling.ID, h.Ruling.Date.Format(time.DateOnly),
					corpus.Truncate(h.Ruling.Facts, 1200), corpus.Truncate(h.Ruling.Holding, 400))
			}
			user := fmt.Sprintf("CASE FACTS:\n%s\n\nCBP HELD: %s\nMODEL ANSWERED: %s\nMODEL REASONING: %s\n\nEARLIER CBP RULINGS:\n%s",
				corpus.Truncate(c.Facts, 8000), r.Gold, r.Pred, r.Reasoning, b.String())
			resp, err := model.Generate(ctx, llm.Request{Model: cfg.EngineModel, System: learnSystem,
				Messages: []llm.Message{{Role: llm.RoleUser, Content: user}}, JSONSchema: learnSchema, SchemaName: "audit", MaxOutputTokens: 4000})
			if err != nil {
				fmt.Fprintln(os.Stderr, r.Ruling, err)
				return
			}
			var a struct {
				Category    string  `json:"category"`
				Ruling      *string `json:"ruling"`
				Rule        string  `json:"rule"`
				ModelMissed string  `json:"model_missed"`
			}
			if err := resp.DecodeJSON(&a); err != nil {
				fmt.Fprintln(os.Stderr, r.Ruling, err)
				return
			}
			shown := false
			if a.Ruling != nil {
				for _, p := range r.Precedents {
					if p == *a.Ruling {
						shown = true
					}
				}
			}
			line, _ := json.Marshal(map[string]any{"ruling": r.Ruling, "gold": r.Gold, "pred": r.Pred, "confidence": r.Confidence,
				"category": a.Category, "anchor": a.Ruling, "anchor_was_shown_to_engine": shown, "rule": a.Rule, "model_missed": a.ModelMissed})
			mu.Lock()
			defer mu.Unlock()
			counts[a.Category]++
			if shown {
				inShown++
			}
			_, _ = f.Write(append(line, '\n'))
		}(r)
	}
	wg.Wait()
	fmt.Printf("learnability (dev errors, judge %s via %s): %v; anchor ruling was among the engine's 5 precedents in %d cases\n", cfg.EngineModel, cfg.LLMProvider, counts, inShown)
	return nil
}

func clarifyCmd(args []string) error {
	fl := flag.NewFlagSet("clarify", flag.ExitOnError)
	in := fl.String("in", "../eval/results/dev-gemini-4-argon-prec-engine-v1-litellm.jsonl", "engine result file")
	below := fl.Float64("below", 0.95, "review cases with confidence below this")
	adjustFile := fl.String("adjust", "../eval/data/scoring_adjustments.json", "scoring adjustments")
	labelsPath := fl.String("labels", "../eval/data/labels.jsonl", "labels")
	factsDir := fl.String("facts", "../data/corpus/facts", "facts")
	confirmFinal := fl.Bool("confirm-final", false, "required for the held-out split")
	_ = fl.Parse(args)
	rows, err := readRows(*in, nil)
	if err != nil {
		return err
	}
	if len(rows) > 0 && isHeldOut(rows[0].Split) && !*confirmFinal {
		return errors.New("held-out split: pass -confirm-final (run once with the frozen clarify prompt)")
	}
	if adj, err := evalkit.LoadAdjustments(*adjustFile); err == nil {
		rows = adj.Apply(rows)
	}
	cfg, err := loadConfig()
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
	cl := clarify.New(model, cfg.EngineModel, 3000)
	split := "dev"
	if len(rows) > 0 {
		split = rows[0].Split
	}
	outPath := filepath.Join(filepath.Dir(*in), fmt.Sprintf("clarify-%s-%s-%s.jsonl", split, clarify.PromptVersion, cfg.LLMProvider))
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	type tally struct{ n, wrong int }
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	t := map[string]*tally{}
	add := func(k string, wrong bool) {
		if t[k] == nil {
			t[k] = &tally{}
		}
		t[k].n++
		if wrong {
			t[k].wrong++
		}
	}
	var cost float64
	for _, r := range rows {
		if r.Error != "" || r.Confidence >= *below {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(r evalkit.Row) {
			defer wg.Done()
			defer func() { <-sem }()
			c, _ := store.Get(r.Ruling)
			var ps []domain.Ruling
			for _, id := range r.Precedents {
				if p, ok := store.Get(id); ok {
					ps = append(ps, p)
				}
			}
			res, err := cl.Review(ctx, c.Facts, domain.Prediction{Origin: r.Pred, Confidence: r.Confidence, KeyFactors: r.KeyFactors, Reasoning: r.Reasoning}, ps)
			if err != nil {
				fmt.Fprintln(os.Stderr, r.Ruling, err)
				return
			}
			b, _ := json.Marshal(map[string]any{"ruling": r.Ruling, "correct": r.Correct, "gold": r.Gold, "pred": r.Pred, "confidence": r.Confidence, "clarify": res})
			mu.Lock()
			defer mu.Unlock()
			_, _ = f.Write(append(b, '\n'))
			wrong := !r.Correct
			add("all reviewed", wrong)
			if res.OnPoint {
				add("precedent on point", wrong)
			} else {
				add("no precedent on point", wrong)
			}
			if res.Conflict != nil {
				add("conflicting precedents", wrong)
			}
			if res.NeedsInfo() {
				add("needs_info route", wrong)
			} else {
				add("needs_review route", wrong)
			}
			cost += evalkit.Cost(evalkit.PromoPrices, cfg.EngineModel, res.Trace.InputTokens, res.Trace.OutputTokens)
		}(r)
	}
	wg.Wait()
	fmt.Printf("## Clarify review (%s, cases with confidence < %.2f, %s)\n\n| Flag | cases | engine wrong | error rate (95%% CI) |\n|---|---:|---:|---|\n", split, *below, cfg.LLMProvider)
	for _, k := range []string{"all reviewed", "precedent on point", "no precedent on point", "conflicting precedents", "needs_info route", "needs_review route"} {
		if x := t[k]; x != nil {
			p, lo, hi := evalkit.Wilson(x.wrong, x.n)
			fmt.Printf("| %s | %d | %d | %.0f%% (%.0f–%.0f) |\n", k, x.n, x.wrong, 100*p, 100*lo, 100*hi)
		}
	}
	if n := t["all reviewed"]; n != nil {
		fmt.Printf("\nCost per reviewed case: $%.4f (promo). Rows: %s\n", cost/float64(n.n), outPath)
	}
	return nil
}

// regimePool is a regime engine's precedents (textile, D51; pharma, D62):
// the listed rulings that have facts and a label.
func regimePool(store *corpus.Store, ids []string) []domain.Ruling {
	var out []domain.Ruling
	for _, id := range ids {
		if r, ok := store.Get(id); ok {
			out = append(out, r)
		}
	}
	return out
}

// textileSummaryItems reports the textile regime (D51, D53): textile-v1 on
// the held-out 2026 textile split (scored once) and textile-v2, the prompt in
// production, on dev. Skipped when the files are missing.
func textileSummaryItems(dir string) ([]evalsummary.Item, error) {
	r4 := func(x float64) float64 { return float64(int(x*10000+0.5)) / 10000 }
	var items []evalsummary.Item
	add := func(file, split, prefix, label string, dev bool) error {
		path := filepath.Join(dir, file)
		if _, err := os.Stat(path); err != nil {
			fmt.Fprintf(os.Stderr, "summary: skip textile %s: %v\n", file, err)
			return nil
		}
		rows, err := readRows(path, nil)
		if err != nil {
			return err
		}
		s := evalkit.Summarize(rows)
		src := "eval/results/" + file
		items = append(items, evalsummary.Item{Key: prefix + "_accuracy", Label: label + " accuracy", Value: r4(s.Acc), CILow: r4(s.Lo), CIHigh: r4(s.Hi), N: s.N, Split: split, DevelopmentOnly: dev, Source: src})
		p, lo, hi := evalkit.Wilson(s.AtLeast95.K, s.AtLeast95.N)
		items = append(items, evalsummary.Item{Key: prefix + "_conf95_accuracy", Label: label + ": accuracy when confidence ≥ 0.95 (the textile auto-accept bar)", Value: r4(p), CILow: r4(lo), CIHigh: r4(hi), N: s.AtLeast95.N, Split: split, DevelopmentOnly: dev, Source: src})
		items = append(items, evalsummary.Item{Key: prefix + "_conf95_coverage", Label: label + ": share auto-accepted (confidence ≥ 0.95)", Value: r4(float64(s.AtLeast95.N) / float64(s.N)), N: s.N, Split: split, DevelopmentOnly: dev, Source: src})
		return nil
	}
	if err := add("textile-final-gemini-4-argon-prec-textile-v1-vertex.jsonl", "final", "textile_final",
		"Textile rules, held-out (71 rulings, 2026, textile-v1)", false); err != nil {
		return nil, err
	}
	if err := add("textile-dev-gemini-4-argon-prec-textile-v2-vertex-reasoning-v1.jsonl", "dev", "textile_v2_dev",
		"Textile rules, development (99 rulings, Jul–Dec 2025, textile-v2 in production)", true); err != nil {
		return nil, err
	}
	return items, nil
}

// regimeSummaryItems reports the pharmaceutical (D62) and USMCA (D61)
// regimes, each scored once on its held-out 2026 split, the same way the
// textile items are. Run through the LiteLLM proxy. Skipped when missing.
func regimeSummaryItems(dir string) ([]evalsummary.Item, error) {
	r4 := func(x float64) float64 { return float64(int(x*10000+0.5)) / 10000 }
	var items []evalsummary.Item
	add := func(file, prefix, label string) error {
		path := filepath.Join(dir, file)
		if _, err := os.Stat(path); err != nil {
			fmt.Fprintf(os.Stderr, "summary: skip %s: %v\n", file, err)
			return nil
		}
		rows, err := readRows(path, nil)
		if err != nil {
			return err
		}
		s := evalkit.Summarize(rows)
		src := "eval/results/" + file
		items = append(items, evalsummary.Item{Key: prefix + "_accuracy", Label: label + " accuracy", Value: r4(s.Acc), CILow: r4(s.Lo), CIHigh: r4(s.Hi), N: s.N, Split: "final", Source: src})
		p, lo, hi := evalkit.Wilson(s.AtLeast95.K, s.AtLeast95.N)
		items = append(items, evalsummary.Item{Key: prefix + "_conf95_accuracy", Label: label + ": accuracy when confidence ≥ 0.95", Value: r4(p), CILow: r4(lo), CIHigh: r4(hi), N: s.AtLeast95.N, Split: "final", Source: src})
		items = append(items, evalsummary.Item{Key: prefix + "_conf95_coverage", Label: label + ": share with confidence ≥ 0.95", Value: r4(float64(s.AtLeast95.N) / float64(s.N)), N: s.N, Split: "final", Source: src})
		return nil
	}
	if err := add("pharma-final-gemini-4-argon-prec-pharma-v1-litellm.jsonl", "pharma_final", "Pharmaceutical rules, held-out (2026, pharma-v1)"); err != nil {
		return nil, err
	}
	if err := add("usmca-final-gemini-4-argon-prec-usmca-v1-litellm.jsonl", "usmca_final", "Part 102 marking-origin rules, held-out (2026, usmca-v1)"); err != nil {
		return nil, err
	}
	// D210: each regime's post-cutoff split (28 Sep-13 Nov 2026), frozen
	// first and scored once through the gateway with the production engine.
	for _, r := range []struct{ file, key, label string }{
		{"textile-final2-gemini-4-argon-prec-textile-v2-litellm-reasoning-v1-router.jsonl", "textile_final2_accuracy", "Textile rules, post-cutoff held-out (Sep–Nov 2026, textile-v2, gateway) accuracy"},
		{"pharma-final2-gemini-4-argon-prec-pharma-v1-litellm-router.jsonl", "pharma_final2_accuracy", "Pharmaceutical rules, post-cutoff held-out (Sep–Nov 2026, pharma-v1, gateway) accuracy; under 10 rulings"},
		{"usmca-final2-gemini-4-argon-prec-usmca-v1-litellm-router.jsonl", "usmca_final2_accuracy", "Part 102 marking-origin rules, post-cutoff held-out (Sep–Nov 2026, usmca-v1, gateway; precedents from the 15 window-2 rulings only, D612) accuracy"},
	} {
		path := filepath.Join(dir, r.file)
		if _, err := os.Stat(path); err != nil {
			fmt.Fprintf(os.Stderr, "summary: skip %s: %v\n", r.file, err)
			continue
		}
		rows, err := readRows(path, nil)
		if err != nil {
			return nil, err
		}
		s := evalkit.Summarize(rows)
		items = append(items, evalsummary.Item{Key: r.key, Label: r.label, Value: r4(s.Acc), CILow: r4(s.Lo), CIHigh: r4(s.Hi), N: s.N, Split: "final2", Source: "eval/results/" + r.file})
	}
	// D602: the production Part 102 engine since 22 Nov (usmca-v5) on the
	// same two windows, scored once each as a candidate (both windows had
	// been scored before with usmca-v1, above).
	for _, r := range []struct{ file, key, label, split string }{
		{"usmca-final-gemini-4-argon-prec-usmca-v5-litellm-d601.jsonl", "usmca_v5_final_accuracy", "Part 102 marking-origin rules, held-out (2026), usmca-v5 (production since D602), scored after usmca-v1", "final"},
		{"usmca-final2-gemini-4-argon-prec-usmca-v5-litellm-router-d601.jsonl", "usmca_v5_final2_accuracy", "Part 102 marking-origin rules, post-cutoff held-out (Sep–Nov 2026), usmca-v5 (production since D602), scored after usmca-v1; precedents from the 15 window-2 rulings only (D612)", "final2"},
	} {
		path := filepath.Join(dir, "..", "tariffshift", "p102ck", "runs", r.file)
		if _, err := os.Stat(path); err != nil {
			fmt.Fprintf(os.Stderr, "summary: skip %s: %v\n", r.file, err)
			continue
		}
		rows, err := readRows(path, nil)
		if err != nil {
			return nil, err
		}
		s := evalkit.Summarize(rows)
		src := "eval/tariffshift/p102ck/runs/" + r.file
		items = append(items, evalsummary.Item{Key: r.key, Label: r.label, Value: r4(s.Acc), CILow: r4(s.Lo), CIHigh: r4(s.Hi), N: s.N, Split: r.split, Source: src})
		// D620: the production engine's own auto-accept share and accuracy at
		// stated confidence 0.95, so /accuracy does not quote usmca-v1's as current.
		prefix := strings.TrimSuffix(r.key, "_accuracy")
		p, lo, hi := evalkit.Wilson(s.AtLeast95.K, s.AtLeast95.N)
		items = append(items, evalsummary.Item{Key: prefix + "_conf95_accuracy", Label: r.label + ": accuracy when confidence ≥ 0.95", Value: r4(p), CILow: r4(lo), CIHigh: r4(hi), N: s.AtLeast95.N, Split: r.split, Source: src})
		items = append(items, evalsummary.Item{Key: prefix + "_conf95_coverage", Label: r.label + ": share with confidence ≥ 0.95", Value: r4(float64(s.AtLeast95.N) / float64(s.N)), N: s.N, Split: r.split, Source: src})
	}
	// The router on the same post-cutoff rulings: pharmaceutical and Part 102
	// cases it kept away from the substantial-transformation engine (D211).
	var kept, seen int
	for _, f := range []string{"pharma-final2-gemini-4-argon-prec-pharma-v1-litellm-router.jsonl", "usmca-final2-gemini-4-argon-prec-usmca-v1-litellm-router.jsonl"} {
		rows, err := readRows(filepath.Join(dir, f), nil)
		if err != nil {
			continue
		}
		for _, r := range rows {
			seen++
			if r.RouterRegime == "pharma_api" || r.RouterRegime == "usmca_part102" {
				kept++
			}
		}
	}
	if seen > 0 {
		p, lo, hi := evalkit.Wilson(kept, seen)
		items = append(items, evalsummary.Item{Key: "router_final2_out_of_scope_recall", Label: "Router-v2 (the router before D233), post-cutoff held-out (Sep–Nov 2026): pharmaceutical and Part 102 rulings kept out of the substantial-transformation engine",
			Value: r4(p), CILow: r4(lo), CIHigh: r4(hi), N: seen, Split: "final2",
			Source: "eval/results/{pharma,usmca}-final2-*-router.jsonl (router_regime)"})
	}
	if it, ok, err := allRegimesItem(dir); err != nil {
		return nil, err
	} else if ok {
		items = append(items, it)
	}
	return items, nil
}

// allRegimesItem pools every regime's held-out run in both windows, each
// ruling once: a ruling in both the textile and the Part 102 split counts
// under Part 102 (D210). The same figure is recomputed by claimscheck
// (regimes.all.dedup.*) and by eval/charts/build_accuracy.py.
func allRegimesItem(dir string) (evalsummary.Item, bool, error) {
	windows := [][4]string{
		{"final-gemini-4-argon-prec-engine-v1-vertex.jsonl", "textile-final-gemini-4-argon-prec-textile-v1-vertex.jsonl",
			"pharma-final-gemini-4-argon-prec-pharma-v1-litellm.jsonl", "usmca-final-gemini-4-argon-prec-usmca-v1-litellm.jsonl"},
		{"final2-gemini-4-argon-prec-engine-v1-litellm-router.jsonl", "textile-final2-gemini-4-argon-prec-textile-v2-litellm-reasoning-v1-router.jsonl",
			"pharma-final2-gemini-4-argon-prec-pharma-v1-litellm-router.jsonl", "usmca-final2-gemini-4-argon-prec-usmca-v1-litellm-router.jsonl"},
	}
	var n, k int
	for _, w := range windows {
		got := make([]map[string]bool, len(w))
		for i, f := range w {
			path := filepath.Join(dir, f)
			if _, err := os.Stat(path); err != nil {
				fmt.Fprintf(os.Stderr, "summary: skip all-regimes item: %v\n", err)
				return evalsummary.Item{}, false, nil
			}
			rows, err := readRows(path, nil)
			if err != nil {
				return evalsummary.Item{}, false, err
			}
			got[i] = map[string]bool{}
			for _, r := range rows {
				got[i][r.Ruling] = r.Correct && r.Error == ""
			}
		}
		for i, m := range got {
			for id, ok := range m {
				if _, both := got[3][id]; i == 1 && both {
					continue // textile and Part 102: counted under Part 102
				}
				n++
				if ok {
					k++
				}
			}
		}
	}
	p, lo, hi := evalkit.Wilson(k, n)
	r6 := func(x float64) float64 { return math.Round(x*1e6) / 1e6 }
	return evalsummary.Item{Key: "all_regimes_heldout_accuracy",
		Label: "All regimes, both held-out windows (Jan–Nov 2026), each ruling once: accuracy (Vertex AI, proxy and gateway runs pooled)",
		Value: r6(p), CILow: r6(lo), CIHigh: r6(hi), N: n, Split: "final+final2",
		Source: "eval/results/{final,final2,textile-final,textile-final2,pharma-final,pharma-final2,usmca-final,usmca-final2}-*.jsonl (eval/results/accuracy-v2-metrics.json, heldout_all_regimes_by_window)"}, true, nil
}

// scopeSummaryItems reports how the router routes cases outside the
// substantial-transformation regime, from the Vertex dev-window scope check
// (D60): out-of-scope recall, textile routing and false out-of-scope.
func scopeSummaryItems(dir, textileSplits string) ([]evalsummary.Item, error) {
	const file = "scope-window-dev-router-v2-vertex.jsonl"
	f, err := os.Open(filepath.Join(dir, file))
	if err != nil {
		fmt.Fprintf(os.Stderr, "summary: skip router scope: %v\n", err)
		return nil, nil
	}
	defer f.Close()
	textile := map[string]bool{}
	if sp, err := corpus.LoadSplits(textileSplits); err == nil {
		for _, id := range sp.Textile {
			textile[id] = true
		}
	}
	var oosK, oosN, texK, texN, falseK, falseN int
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var r struct {
			Ruling string `json:"ruling"`
			Gold   string `json:"gold_regime"`
			Router *struct {
				Regime string `json:"regime"`
			} `json:"router"`
		}
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.Router == nil {
			continue
		}
		reg := r.Router.Regime
		switch {
		case r.Gold == "pharma_api" || (r.Gold == "part102_tariff_shift" && !textile[r.Ruling]):
			oosN++
			if reg != "substantial_transformation" {
				oosK++
			}
		case r.Gold == "part102_tariff_shift":
			texN++
			if reg == router.RegimeTextile {
				texK++
			}
		case r.Gold == "substantial_transformation":
			falseN++
			if reg == "usmca_part102" || reg == "pharma_api" {
				falseK++
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	r4 := func(x float64) float64 { return float64(int(x*10000+0.5)) / 10000 }
	src := "eval/results/" + file
	item := func(key, label string, k, n int) evalsummary.Item {
		p, lo, hi := evalkit.Wilson(k, n)
		return evalsummary.Item{Key: key, Label: label, Value: r4(p), CILow: r4(lo), CIHigh: r4(hi), N: n, Split: "dev", DevelopmentOnly: true, Source: src}
	}
	return []evalsummary.Item{
		item("router_out_of_scope_recall", "Router-v2 (Jul-Dec 2025): pharmaceutical and Part 102 marking rulings kept out of the substantial-transformation engine", oosK, oosN),
		item("router_textile_routing", "Router-v2 (Jul–Dec 2025): textile (19 CFR 102.21) rulings sent to the textile engine", texK, texN),
		item("router_false_out_of_scope", "Router-v2 (Jul–Dec 2025): substantial-transformation rulings wrongly sent out of scope", falseK, falseN),
	}, nil
}

// votesSuffix names a result file scored with a VERIFIER_VOTES majority.
func votesSuffix(votes int) string {
	if votes > 1 {
		return fmt.Sprintf("-vote%d", votes)
	}
	return ""
}

// caseStoreGet looks a ruling up in the optional -case-labels store.
func caseStoreGet(s *corpus.Store, id string) (domain.Ruling, bool) {
	if s == nil {
		return domain.Ruling{}, false
	}
	return s.Get(id)
}

// usmcaOptions is the Part 102 engine's prompt for version v ("v1" or "v2";
// D550): v2 adds the quoted 102.20 rows for the codes named in each case.
func usmcaOptions(v string) (engine.Options, error) {
	switch v {
	case "v1":
		return engine.Options{System: engine.USMCASystem, Version: engine.USMCAVersion}, nil
	case "v2":
		rules, err := tariffshift.Default()
		if err != nil {
			return engine.Options{}, err
		}
		return engine.Options{System: engine.USMCASystem2, Version: engine.USMCAVersion2, RuleText: engine.Part102RuleText(rules)}, nil
	case "v3":
		return engine.Options{System: engine.USMCASystem3, Version: engine.USMCAVersion3, Schema: engine.ChecklistSchema}, nil
	case "v4":
		return engine.Options{System: engine.USMCASystem4, Version: engine.USMCAVersion4, Schema: engine.CompactChecklistSchema}, nil
	case "v5":
		return engine.Options{System: engine.USMCASystem5, Version: engine.USMCAVersion5, Schema: engine.FailingChecklistSchema}, nil
	}
	return engine.Options{}, fmt.Errorf("-usmca-prompt must be v1, v2, v3, v4 or v5, got %q", v)
}
