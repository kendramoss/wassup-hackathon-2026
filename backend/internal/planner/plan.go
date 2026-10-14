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

	baseFacts := Render(req.Product, tm)
	base, err := p.engine.Predict(ctx, p.caseFor(req, "baseline", baseFacts))
	if err != nil {
		return Result{}, fmt.Errorf("planner: baseline: %w", err)
	}
	addEngine(base.Usage)
	res.Baseline = domain.Plan{Move: domain.Move{}, Description: Describe(req.Product, tm, nil), VariantFacts: baseFacts, Prediction: base.Prediction}
	baseOrigin := base.Prediction.Origin
	var knownA, knownRuling string
	if k := req.CurrentOrigin; k != nil && k.Country != "" {
		baseOrigin, knownA, knownRuling = domain.NormalizeCountry(k.Country), domain.NormalizeCountry(k.Country), k.Ruling
		res.CurrentOrigin = &KnownOrigin{Country: knownA, Ruling: knownRuling}
	}

	moves := Enumerate(req.Product, tm, target)
	res.Stats.MovesSearched = len(moves)
	plans := make([]domain.Plan, len(moves))
	sem := make(chan struct{}, p.opts.Concurrency)
	var wg sync.WaitGroup
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
			pl.DutyEffectYear, pl.AddedCostYear = economics(req, m, baseOrigin, r.Prediction.Origin)
			pl.NetYear = pl.DutyEffectYear - pl.AddedCostYear
			if r.Prediction.Origin != target {
				pl.RejectionReason = fmt.Sprintf("engine predicts origin %s, not %s", r.Prediction.Origin, target)
				plans[i] = pl
				return
			}
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
				plans[i] = pl
				return
			}
			v := vr.Verdict
			pl.Verdict = &v
			switch {
			case !v.PrecedentSupported:
				pl.RejectionReason = "no earlier ruling found where CBP held this kind of change decisive"
			case v.Confidence < p.opts.PlanConfidence:
				pl.RejectionReason = fmt.Sprintf("precedent-supported but verifier confidence %.2f is below %.2f", v.Confidence, p.opts.PlanConfidence)
			default:
				pl.Shown = true
			}
			plans[i] = pl
		}(i, m)
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return Result{}, err
	}
	for _, pl := range plans {
		if pl.Shown {
			res.Plans = append(res.Plans, pl)
		} else {
			res.Rejected = append(res.Rejected, pl)
		}
	}
	sort.SliceStable(res.Plans, func(i, j int) bool { return res.Plans[i].NetYear > res.Plans[j].NetYear })
	// Rejected: moves that reached the target in the engine first (closest
	// calls), then the rest, each in enumeration order.
	sort.SliceStable(res.Rejected, func(i, j int) bool {
		return res.Rejected[i].Verdict != nil && res.Rejected[j].Verdict == nil
	})
	if baseOrigin != target {
		b := res.Baseline
		b.RejectionReason = fmt.Sprintf("current setup: engine predicts origin %s", base.Prediction.Origin)
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
	return res, nil
}

func (p *Planner) caseFor(req Request, key, facts string) domain.Case {
	return domain.Case{ID: req.CaseID + "/" + key, Date: req.Date.Time, Facts: facts}
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
