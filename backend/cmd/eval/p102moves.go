package main

// p102moves runs the Part 102 engine alone on the current setup and on each
// plan move of development plan fixtures that the 19 CFR 102.20 check
// decides or leans on, with no precedent check (D590 error analysis):
//
//	go run ./cmd/eval p102moves -tag d590-b [-usmca-prompt v1|v3] [-codes-in-facts]
//
// Moves are enumerated as the planner enumerates them (EnumerateUpTo, the
// production move cap) and the engine reads each move's facts as a plan
// search renders them, so a row is the call a plan search makes for that
// move. The rule check runs first, offline; a move it leaves undetermined
// for any reason other than de minimis is not sent to the engine, since
// there is nothing to compare. Only development fixtures are read:
// testdata/planner_part102 (D501), testdata/planner_part102_dev (D590) and
// testdata/planner_families (D171, with the top D381 development
// suggestion for the good and every part, as D442).

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

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/hts"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planrule"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

// p102MoveSets are the development fixture sets, in order.
var p102MoveSets = []struct{ Set, Dir string }{
	{"part102-d501", "testdata/planner_part102"},
	{"part102-dev", "testdata/planner_part102_dev"},
	{"family", "testdata/planner_families"},
}

// P102MoveRow is one engine call on a current setup or a move.
type P102MoveRow struct {
	Set     string `json:"set"`
	Fixture string `json:"fixture"`
	Target  string `json:"target"`
	CBP     string `json:"cbp_origin,omitempty"`
	Kind    string `json:"kind"` // baseline or move
	Move    string `json:"move,omitempty"`
	GoodHTS string `json:"good_hts"`
	Country string `json:"country"`
	// Rule check: status as plans102-eval classifies it, the decided
	// origin, the de minimis lean and the parts that fail.
	Status  string   `json:"status"`
	Why     string   `json:"why,omitempty"`
	Origin  string   `json:"origin,omitempty"`
	Lean    string   `json:"if_not_de_minimis,omitempty"`
	Failing []string `json:"failing,omitempty"`
	Rule    string   `json:"rule,omitempty"`
	// Engine answer.
	Engine     string          `json:"engine_origin,omitempty"`
	Confidence float64         `json:"confidence,omitempty"`
	Step       string          `json:"essential_character,omitempty"`
	Decisive   string          `json:"decisive_step,omitempty"`
	Reasoning  string          `json:"reasoning,omitempty"`
	Checklist  json.RawMessage `json:"checklist,omitempty"`
	Prompt     string          `json:"prompt_version,omitempty"`
	InTok      int             `json:"input_tokens,omitempty"`
	OutTok     int             `json:"output_tokens,omitempty"`
	GatewayUSD float64         `json:"gateway_cost_usd,omitempty"`
	Error      string          `json:"error,omitempty"`
}

func p102MovesCmd(args []string) error {
	fl := flag.NewFlagSet("p102moves", flag.ExitOnError)
	tag := fl.String("tag", "", "run tag (required), e.g. d590-b")
	prompt := fl.String("usmca-prompt", "v1", "Part 102 engine prompt: v1 (production), v3 (D590 checklist), v4 or v5 (D600 compact checklists)")
	codesInFacts := fl.Bool("codes-in-facts", false, "render the good's and each part's code into the facts (D590 variant)")
	sets := fl.String("sets", "part102-d501,part102-dev,family", "fixture sets")
	dryRun := fl.Bool("dry-run", false, "count the calls and write nothing")
	labelsPath := fl.String("usmca-labels", "../eval/data/labels_usmca.jsonl", "the Part 102 engine's precedent pool")
	factsDir := fl.String("facts", "../data/corpus/facts", "facts directory")
	suggestPath := fl.String("suggest", "../eval/tariffshift/parts/suggest-dev-v1.jsonl", "D381 development suggestions (family fixtures' codes)")
	outDir := fl.String("out", "../eval/tariffshift/p102err/runs", "output directory")
	workers := fl.Int("workers", 4, "concurrent engine calls")
	_ = fl.Parse(args)
	if *tag == "" {
		return errors.New("p102moves: -tag is required")
	}
	want := map[string]bool{}
	for _, s := range strings.Split(*sets, ",") {
		want[strings.TrimSpace(s)] = true
	}
	rules, err := tariffshift.Default()
	if err != nil {
		return err
	}
	tmpls, err := planner.LoadTemplates()
	if err != nil {
		return err
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	famCodes, err := familySuggestions(*suggestPath)
	if err != nil {
		return err
	}

	type job struct {
		row  P102MoveRow
		req  planner.Request
		tm   planner.Template
		move domain.Move
	}
	var jobs []job
	for _, s := range p102MoveSets {
		if !want[s.Set] {
			continue
		}
		files, _ := filepath.Glob(s.Dir + "/*.json")
		sort.Strings(files)
		for _, f := range files {
			id := strings.TrimSuffix(filepath.Base(f), ".json")
			req, err := planner.LoadRequest(f)
			if err != nil {
				return err
			}
			if s.Set == "family" {
				sr, ok := famCodes[id]
				if !ok {
					return fmt.Errorf("no family suggestion row for %s", id)
				}
				if req.Product, err = topCodes(req.Product, sr); err != nil {
					return fmt.Errorf("%s: %w", id, err)
				}
			}
			tm, ok := tmpls[req.Family]
			if !ok {
				return fmt.Errorf("%s: no template %q", id, req.Family)
			}
			target := domain.NormalizeCountry(req.Target)
			cbp := ""
			if req.CurrentOrigin != nil {
				cbp = domain.NormalizeCountry(req.CurrentOrigin.Country)
			}
			moves := planner.EnumerateUpTo(req.Product, tm, target, req.Economics.GroupCosts, cfg.PlanMaxMoves)
			for _, m := range append([]domain.Move{nil}, moves...) {
				r := ruleRow(rules, req.Product, m, target)
				r.Set, r.Fixture, r.Target, r.CBP = s.Set, id, target, cbp
				if m == nil {
					r.Kind = "baseline"
				} else {
					r.Kind, r.Move = "move", m.Key()
					if r.Status == planrule.Undetermined && r.Lean == "" {
						continue // nothing to compare the engine with
					}
				}
				jobs = append(jobs, job{row: r, req: req, tm: tm, move: m})
			}
		}
	}
	fmt.Printf("p102moves %s: %d engine calls (%d moves, rest current setups)\n", *tag, len(jobs), countMoves(jobs, func(j job) bool { return j.row.Kind == "move" }))
	if *dryRun {
		return nil
	}
	path := filepath.Join(*outDir, fmt.Sprintf("moves-%s.jsonl", *tag))
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("p102moves: %s exists; each run is kept as it was", path)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	model, err := newModel(ctx, cfg, logger)
	if err != nil {
		return err
	}
	labels, err := corpus.LoadLabels(*labelsPath)
	if err != nil {
		return err
	}
	pool, err := corpus.Load(labels, *factsDir)
	if err != nil {
		return err
	}
	if pool.Len() == 0 {
		return fmt.Errorf("p102moves: no precedent facts in %s (link the local corpus)", *factsDir)
	}
	opts, err := p102Options(*prompt, cfg)
	if err != nil {
		return err
	}
	eng := engine.New(model, retrieval.NewBM25(pool.Rulings(), engine.PrecedentText), opts)

	start := time.Now()
	rows := make([]P102MoveRow, len(jobs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, max(*workers, 1))
	for i, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			rows[i] = j.row
			facts := moveFacts(j.req.Product, j.tm, j.move, *codesInFacts)
			c := domain.Case{ID: j.req.CaseID + "/" + j.move.Key(), Date: j.req.Date.Time, Facts: facts, Exclude: j.req.Exclude}
			if j.move == nil {
				c.ID = j.req.CaseID + "/baseline"
			}
			var tally llm.CostTally
			var res engine.Result
			var err error
			for try := 0; try < 2; try++ { // one re-run on an error
				if res, err = eng.Predict(llm.ContextWithCostTally(ctx, &tally), c); err == nil {
					break
				}
			}
			rows[i].GatewayUSD = round6(tally.USD())
			if err != nil {
				rows[i].Error = err.Error()
				return
			}
			p := res.Prediction
			rows[i].Engine, rows[i].Confidence = domain.NormalizeCountry(p.Origin), p.Confidence
			if len(p.KeyFactors) > 0 {
				rows[i].Step = p.KeyFactors[0]
			}
			rows[i].Decisive, rows[i].Reasoning, rows[i].Checklist = p.DecisiveStep, p.Reasoning, res.Answer
			rows[i].Prompt, rows[i].InTok, rows[i].OutTok = p.Trace.PromptVersion, p.Trace.InputTokens, p.Trace.OutputTokens
		}()
	}
	wg.Wait()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	var usd float64
	errs := 0
	for _, r := range rows {
		usd += r.GatewayUSD
		if r.Error != "" {
			errs++
		}
		b, _ := json.Marshal(r)
		w.Write(append(b, '\n'))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Printf("p102moves %s: %d calls, %d errors, gateway $%.4f, %.0f s -> %s\n", *tag, len(rows), errs, usd, time.Since(start).Seconds(), path)
	lf, err := os.OpenFile("../eval/results/RUNLOG.jsonl", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer lf.Close()
	entry := map[string]any{"time": time.Now().Format(time.RFC3339), "run": "p102moves-" + *tag, "split": "dev", "held_out": false,
		"calls": len(rows), "errors": errs, "engine_prompt": opts.Version, "codes_in_facts": *codesInFacts, "model": cfg.EngineModel,
		"k": cfg.PrecedentK, "llm_provider": cfg.LLMProvider, "llm_no_cache": os.Getenv("LITELLM_NO_CACHE") == "1",
		"gateway_cost_usd": round6(usd), "git_revision": revision(), "decision": "D590", "output": strings.TrimPrefix(path, "../")}
	b, _ := json.Marshal(entry)
	_, err = lf.Write(append(b, '\n'))
	return err
}

func countMoves[T any](xs []T, ok func(T) bool) int {
	n := 0
	for _, x := range xs {
		if ok(x) {
			n++
		}
	}
	return n
}

// ruleRow is the rule check on the product after the move (nil: as is).
func ruleRow(rules *tariffshift.Rules, p domain.Product, m domain.Move, target string) P102MoveRow {
	in := planrule.Input(p, m)
	res := rules.Evaluate(in)
	status, why, failing := planrule.Classify(res, target)
	r := P102MoveRow{GoodHTS: p.HTS, Country: domain.NormalizeCountry(in.Country), Status: status, Why: why, Failing: failing,
		Origin: domain.NormalizeCountry(res.Origin)}
	if res.Rule != nil {
		r.Rule = res.Rule.ID
	}
	if status == planrule.Undetermined && why == planrule.WhyDeMinimis && res.IfNotDeMinimis != nil {
		r.Lean = domain.NormalizeCountry(res.IfNotDeMinimis.Origin)
	}
	return r
}

// moveFacts are the facts a plan search gives the engine for the move (nil:
// the current setup), with the codes added when asked (D590 variant).
func moveFacts(p domain.Product, tm planner.Template, m domain.Move, codes bool) string {
	v := planner.Apply(p, m)
	groups := make([]string, len(m))
	for i, gm := range m {
		groups[i] = gm.Group
	}
	if m == nil {
		groups = nil
	}
	facts := planner.Render(v, tm, groups...)
	if codes {
		facts += planner.ClassificationNote(v)
	}
	return facts
}

// usmcaOptions is the Part 102 engine as the server builds it, with the
// prompt asked for.
func p102Options(prompt string, cfg config.Config) (engine.Options, error) {
	o := engine.Options{Model: cfg.EngineModel, K: cfg.PrecedentK, MaxOutputTokens: cfg.MaxOutputTokens,
		System: engine.USMCASystem, Version: engine.USMCAVersion}
	switch prompt {
	case "v1":
	case "v3":
		o.System, o.Version, o.Schema = engine.USMCASystem3, engine.USMCAVersion3, engine.ChecklistSchema
	case "v4":
		o.System, o.Version, o.Schema = engine.USMCASystem4, engine.USMCAVersion4, engine.CompactChecklistSchema
	case "v5":
		o.System, o.Version, o.Schema = engine.USMCASystem5, engine.USMCAVersion5, engine.FailingChecklistSchema
	default:
		return o, fmt.Errorf("-usmca-prompt must be v1, v3, v4 or v5")
	}
	return o, nil
}

// familySuggestions are the D381 development rows for the family fixtures,
// by fixture id.
func familySuggestions(path string) (map[string]famSuggestion, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]famSuggestion{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var r famSuggestion
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, err
		}
		if r.Set != "family" {
			continue
		}
		if r.Split != "dev" {
			return nil, fmt.Errorf("%s: %s is not a development row", path, r.Ruling)
		}
		out[strings.ToLower(r.Ruling)] = r
	}
	return out, sc.Err()
}

type famSuggestion struct {
	Ruling string               `json:"ruling"`
	Set    string               `json:"set"`
	Split  string               `json:"split"`
	Parts  []hts.PartSuggestion `json:"parts"`
	Good   *hts.GoodSuggestion  `json:"good,omitempty"`
}

// topCodes is p with the top suggestion on the good and every part (the
// D442 input; a part with no suggestion keeps no code).
func topCodes(p domain.Product, s famSuggestion) (domain.Product, error) {
	if len(s.Parts) != len(p.Components) {
		return p, fmt.Errorf("%d suggested parts for %d components", len(s.Parts), len(p.Components))
	}
	p.Components = append([]domain.Component(nil), p.Components...)
	for i, sp := range s.Parts {
		if sp.Index != i || sp.Name != p.Components[i].Name {
			return p, fmt.Errorf("part %d: suggestion %q does not match %q", i, sp.Name, p.Components[i].Name)
		}
		p.Components[i].HTS = ""
		if len(sp.Candidates) > 0 {
			p.Components[i].HTS = sp.Candidates[0].HTS
		}
	}
	if s.Good != nil && len(s.Good.Candidates) > 0 {
		p.HTS = s.Good.Candidates[0].HTS
	}
	return p, nil
}

func round6(x float64) float64 { return float64(int64(x*1e6+0.5)) / 1e6 }
