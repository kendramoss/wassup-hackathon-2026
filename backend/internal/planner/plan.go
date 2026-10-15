package planner

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/verifier"
)

// Predictor predicts origin for a case (engine.Engine).
type Predictor interface {
	Predict(ctx context.Context, c domain.Case) (engine.Result, error)
}

// Checker verifies a move against precedent (verifier.Verifier).
type Checker interface {
	Verify(ctx context.Context, t verifier.Task) (verifier.Result, error)
}

// Options configures the planner.
type Options struct {
	// PlanConfidence is the minimum verifier confidence for a plan to be shown.
	PlanConfidence float64
	// Concurrency limits parallel model calls.
	Concurrency int
}

// Event is one step of a plan search, for progress display.
type Event struct {
	Kind        string `json:"kind"` // started | baseline | predicted | verifying | verified | done
	Move        string `json:"move,omitempty"`
	Description string `json:"description,omitempty"`
	Origin      string `json:"origin,omitempty"`  // predicted origin (baseline, predicted)
	Outcome     string `json:"outcome,omitempty"` // verified: shown | below_confidence_bar | no_supporting_precedent | check_failed; predicted: engine_other_origin when it misses the target
	Done        int    `json:"done"`              // moves finished
	Total       int    `json:"total"`             // moves searched
}

// Planner searches operation-group moves.
type Planner struct {
	templates map[string]Template
	engine    Predictor
	verifier  Checker
	opts      Options
}

// New returns a planner.
func New(templates map[string]Template, e Predictor, v Checker, opts Options) *Planner {
	if opts.Concurrency < 1 {
		opts.Concurrency = 1
	}
	return &Planner{templates: templates, engine: e, verifier: v, opts: opts}
}

// Result is a full plan search.
type Result struct {
	CaseID   string      `json:"case_id"`
	Target   string      `json:"target"`
	Family   string      `json:"family"`
	Baseline domain.Plan `json:"baseline"`
	// CurrentOrigin is CBP's holding for the current setup when the user gave
	// it; Baseline.Prediction is the engine's own view, shown alongside.
	CurrentOrigin *KnownOrigin      `json:"current_origin,omitempty"`
	Plans         []domain.Plan     `json:"plans"`    // shown: precedent-supported, best-ranked first
	Rejected      []domain.Plan     `json:"rejected"` // greyed out, with the reason
	Backtest      *Backtest         `json:"backtest,omitempty"`
	Stats         Stats             `json:"stats"`
	Illustrative  bool              `json:"economics_illustrative"`
	DutyNote      string            `json:"duty_rates_note,omitempty"`
	Templates     map[string]string `json:"template_versions"`
}

// Backtest compares the best plan with a real change.
type Backtest struct {
	Source         string   `json:"source"`
	PlanMoves      string   `json:"plan"`
	ActualMoved    []string `json:"actual_moved"`
	MovedByPlan    []string `json:"moved_by_plan"`
	NotMovedByPlan []string `json:"in_actual_change_not_in_plan"`
}

// Stats is the full-plan cost: every model call, tokens and wall time.
type Stats struct {
	MovesSearched  int     `json:"moves_searched"`
	EngineCalls    int     `json:"engine_calls"`
	VerifierRuns   int     `json:"verifier_runs"`
	VerifierModel  int     `json:"verifier_model_calls"`
	ToolCalls      int     `json:"verifier_tool_calls"`
	InputTokens    int     `json:"input_tokens"`
	OutputTokens   int     `json:"output_tokens"`
	EngineInTok    int     `json:"engine_input_tokens"`
	EngineOutTok   int     `json:"engine_output_tokens"`
	VerifierInTok  int     `json:"verifier_input_tokens"`
	VerifierOutTok int     `json:"verifier_output_tokens"`
	WallSeconds    float64 `json:"wall_seconds"`
}

// ErrUnknownFamily means no template exists for the request's family.
var ErrUnknownFamily = errors.New("planner: unknown product family")

// Plan runs the search. The baseline (current setup) is predicted first;
// every enumerated move is rendered and predicted; moves predicted to reach
// the target are verified; shown plans are precedent-supported with verifier
// confidence ≥ PlanConfidence, ranked by duty effect minus added cost.
func (p *Planner) Plan(ctx context.Context, req Request) (Result, error) {
	return p.PlanWithProgress(ctx, req, nil)
}

// PlanWithProgress is Plan, calling progress (if not nil) after each step.
// Calls are serialised; progress must not block for long.
func (p *Planner) PlanWithProgress(ctx context.Context, req Request, progress func(Event)) (Result, error) {
	start := time.Now()
	if err := req.Validate(); err != nil {
		return Result{}, err
	}
	tm, ok := p.templates[req.Family]
	if !ok {
		return Result{}, fmt.Errorf("%w: %q", ErrUnknownFamily, req.Family)
	}
	target := domain.NormalizeCountry(req.Target)
	res := Result{CaseID: req.CaseID, Target: target, Family: req.Family,
		Plans: []domain.Plan{}, Rejected: []domain.Plan{},
		Illustrative: req.Economics.Illustrative, DutyNote: req.Economics.DutyRatesNote,
		Templates: map[string]string{tm.Family: tm.Version}}
	var mu sync.Mutex
	addEngine := func(u llm.Usage) {
		mu.Lock()
		defer mu.Unlock()
		res.Stats.EngineCalls++
		res.Stats.EngineInTok += u.InputTokens
		res.Stats.EngineOutTok += u.OutputTokens
	}

	moves := Enumerate(req.Product, tm, target)
	res.Stats.MovesSearched = len(moves)
	done := 0
	emit := func(e Event) { // callers hold mu
		if progress != nil {
			e.Done, e.Total = done, len(moves)
			progress(e)
		}
	}
	finish := func(e Event) {
		mu.Lock()
		defer mu.Unlock()
		done++
		emit(e)
	}
	mu.Lock()
	emit(Event{Kind: "started"})
	mu.Unlock()

	var knownA, knownRuling string
	if k := req.CurrentOrigin; k != nil && k.Country != "" {
		knownA, knownRuling = domain.NormalizeCountry(k.Country), k.Ruling
		res.CurrentOrigin = &KnownOrigin{Country: knownA, Ruling: knownRuling}
	}
	// The baseline runs alongside the moves: they need its facts (known now),
	// and its origin only for economics, which are filled in after the search.
	baseFacts := Render(req.Product, tm)
	var base engine.Result
	var baseErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		r, err := p.engine.Predict(ctx, p.caseFor(req, "baseline", baseFacts))
		if err != nil {
			baseErr = fmt.Errorf("planner: baseline: %w", err)
			return
		}
		addEngine(r.Usage)
		base = r
		mu.Lock()
		emit(Event{Kind: "baseline", Origin: r.Prediction.Origin, Description: Describe(req.Product, tm, nil)})
		mu.Unlock()
	}()

	plans := make([]domain.Plan, len(moves))
	sem := make(chan struct{}, p.opts.Concurrency)
	errs := make([]error, len(moves))
	for i, m := range moves {
		wg.Add(1)
		go func(i int, m domain.Move) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			variant := Apply(req.Product, m)
			facts := Render(variant, tm, groupsOf(m)...)
			r, err := p.engine.Predict(ctx, p.caseFor(req, m.Key(), facts))
			if err != nil {
				errs[i] = fmt.Errorf("planner: move %s: %w", m.Key(), err)
				return
			}
			addEngine(r.Usage)
			pl := domain.Plan{Move: m, Description: Describe(req.Product, tm, m), VariantFacts: facts, Prediction: r.Prediction}
			if r.Prediction.Origin != target {
				pl.RejectionReason = fmt.Sprintf("engine predicts origin %s, not %s", r.Prediction.Origin, target)
				pl.RejectionKind = domain.RejectOtherOrigin
				plans[i] = pl
				finish(Event{Kind: "predicted", Move: m.Key(), Description: pl.Description, Origin: r.Prediction.Origin, Outcome: string(domain.RejectOtherOrigin)})
				return
			}
			mu.Lock()
			emit(Event{Kind: "predicted", Move: m.Key(), Description: pl.Description, Origin: r.Prediction.Origin})
			emit(Event{Kind: "verifying", Move: m.Key(), Description: pl.Description})
			mu.Unlock()
			vr, err := p.verifier.Verify(ctx, verifier.Task{FactsA: baseFacts, FactsB: facts, Target: target, Before: req.Date.Time,
				Exclude: req.Exclude, KnownOriginA: knownA, KnownRulingA: knownRuling})
			mu.Lock()
			res.Stats.VerifierRuns++
			res.Stats.VerifierModel += vr.ModelCalls
			res.Stats.ToolCalls += len(vr.Steps)
			res.Stats.VerifierInTok += vr.Usage.InputTokens
			res.Stats.VerifierOutTok += vr.Usage.OutputTokens
			mu.Unlock()
			if err != nil {
				pl.RejectionReason = "precedent check failed: " + err.Error()
				pl.RejectionKind = domain.RejectCheckFailed
				plans[i] = pl
				finish(Event{Kind: "verified", Move: m.Key(), Description: pl.Description, Outcome: string(domain.RejectCheckFailed)})
				return
			}
			v := vr.Verdict
			pl.Verdict = &v
			switch {
			case !v.PrecedentSupported:
				pl.RejectionReason = "no earlier ruling found where CBP held this kind of change decisive"
				pl.RejectionKind = domain.RejectNoPrecedent
			case v.Confidence < p.opts.PlanConfidence:
				pl.RejectionReason = fmt.Sprintf("precedent-supported but verifier confidence %.2f is below %.2f", v.Confidence, p.opts.PlanConfidence)
				pl.RejectionKind = domain.RejectBelowConfidence
			default:
				pl.Shown = true
			}
			plans[i] = pl
			outcome := "shown"
			if !pl.Shown {
				outcome = string(pl.RejectionKind)
			}
			finish(Event{Kind: "verified", Move: m.Key(), Description: pl.Description, Outcome: outcome})
		}(i, m)
	}
	wg.Wait()
	if err := errors.Join(append([]error{baseErr}, errs...)...); err != nil {
		return Result{}, err
	}
	res.Baseline = domain.Plan{Move: domain.Move{}, Description: Describe(req.Product, tm, nil), VariantFacts: baseFacts, Prediction: base.Prediction}
	baseOrigin := base.Prediction.Origin
	if knownA != "" {
		baseOrigin = knownA
	}
	for i := range plans {
		plans[i].DutyEffectYear, plans[i].AddedCostYear = economics(req, moves[i], baseOrigin, plans[i].Prediction.Origin)
		plans[i].NetYear = plans[i].DutyEffectYear - plans[i].AddedCostYear
	}
	for _, pl := range plans {
		if pl.Shown {
			res.Plans = append(res.Plans, pl)
		} else {
			res.Rejected = append(res.Rejected, pl)
		}
	}
	sort.SliceStable(res.Plans, func(i, j int) bool { return res.Plans[i].NetYear > res.Plans[j].NetYear })
	// Rejected, closest calls first: precedent-supported but below the
	// confidence bar (best net first), then other verified moves, then moves
	// the engine says miss the target; the current setup is prepended below.
	rank := map[domain.RejectionKind]int{domain.RejectBelowConfidence: 0, domain.RejectNoPrecedent: 1, domain.RejectCheckFailed: 2, domain.RejectOtherOrigin: 3}
	sort.SliceStable(res.Rejected, func(i, j int) bool {
		a, b := res.Rejected[i], res.Rejected[j]
		if rank[a.RejectionKind] != rank[b.RejectionKind] {
			return rank[a.RejectionKind] < rank[b.RejectionKind]
		}
		if a.RejectionKind == domain.RejectBelowConfidence {
			return a.NetYear > b.NetYear
		}
		return false
	})
	if baseOrigin != target {
		b := res.Baseline
		b.RejectionReason = fmt.Sprintf("current setup: engine predicts origin %s", base.Prediction.Origin)
		b.RejectionKind = domain.RejectCurrentSetup
		if knownA != "" {
			b.RejectionReason = fmt.Sprintf("current setup: CBP held %s (%s); engine predicts %s", knownA, knownRuling, base.Prediction.Origin)
		}
		res.Rejected = append([]domain.Plan{b}, res.Rejected...)
	}
	if req.ActualChange != nil {
		res.Backtest = backtest(req, res.Plans)
	}
	res.Stats.InputTokens = res.Stats.EngineInTok + res.Stats.VerifierInTok
	res.Stats.OutputTokens = res.Stats.EngineOutTok + res.Stats.VerifierOutTok
	res.Stats.WallSeconds = time.Since(start).Seconds()
	mu.Lock()
	emit(Event{Kind: "done"})
	mu.Unlock()
	return res, nil
}

func (p *Planner) caseFor(req Request, key, facts string) domain.Case {
	return domain.Case{ID: req.CaseID + "/" + key, Date: req.Date.Time, Facts: facts, Exclude: req.Exclude}
}

// economics returns the annual duty effect of the origin change and the
// annual added cost of the move, from the user's inputs.
func economics(req Request, m domain.Move, fromOrigin, toOrigin string) (dutyEffect, addedCost float64) {
	e := req.Economics
	if r1, ok1 := e.DutyRate(fromOrigin); ok1 {
		if r2, ok2 := e.DutyRate(toOrigin); ok2 {
			dutyEffect = e.AnnualImportValueUSD * (r1 - r2)
		}
	}
	for _, gm := range m {
		costs := e.GroupCosts[gm.Group]
		from, ok := groupPlant(req.Product, gm.Group)
		if !ok {
			continue
		}
		addedCost += costs[gm.ToPlant] - costs[from.ID]
	}
	return math.Round(dutyEffect*100) / 100, math.Round(addedCost*100) / 100
}

func backtest(req Request, plans []domain.Plan) *Backtest {
	a := req.ActualChange
	bt := &Backtest{Source: a.Source, ActualMoved: a.MovedComponents}
	if len(plans) == 0 {
		bt.PlanMoves = "no precedent-supported plan"
		bt.NotMovedByPlan = a.MovedComponents
		return bt
	}
	best := plans[0]
	bt.PlanMoves = best.Description
	moved := map[string]bool{}
	for _, c := range Apply(req.Product, best.Move).Components {
		for _, orig := range req.Product.Components {
			if orig.Name == c.Name && orig.MadeIn != c.MadeIn {
				moved[c.Name] = true
			}
		}
	}
	for _, name := range a.MovedComponents {
		if moved[name] {
			bt.MovedByPlan = append(bt.MovedByPlan, name)
		} else {
			bt.NotMovedByPlan = append(bt.NotMovedByPlan, name)
		}
	}
	return bt
}

func groupsOf(m domain.Move) []string {
	out := make([]string, len(m))
	for i, gm := range m {
		out[i] = gm.Group
	}
	return out
}
