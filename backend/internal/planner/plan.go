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
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/nearmiss"
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
	// NearMiss, when set, explains the closest moves whose check is at least
	// NearMissFloor but under PlanConfidence: at most NearMissMax per search
	// (D81). Advice only; nil (the default) adds nothing.
	NearMiss      nearmiss.Adviser
	NearMissFloor float64
	NearMissMax   int
	// MaxMoves caps the moves searched; three-group moves are added only
	// while the total stays within it (D84). 0 searches singles and pairs.
	MaxMoves int
	// FollowAdvice, with NearMiss, searches one more move per search: the
	// advised near miss plus the groups its advice names (D85). Mapper is
	// the fallback when no group name, operation or component in the advice
	// matches; nil means no fallback.
	FollowAdvice bool
	Mapper       GroupMapper
}

// GroupMapper names the template groups a piece of advice asks to move
// (nearmiss.Mapper), for when the text names none of them literally.
type GroupMapper interface {
	MapGroups(ctx context.Context, advice string, groups []domain.OperationGroup) ([]string, llm.Usage, error)
}

// DefaultMaxMoves is PLAN_MAX_MOVES' default (D84).
const DefaultMaxMoves = 12

// Near-miss defaults (D81).
const (
	DefaultNearMissFloor = 0.85
	DefaultNearMissMax   = 2
)

// Event kinds, in order: started, baseline (any time before done),
// move_started and move_done once per move, done.
const (
	EventStarted     = "started"
	EventBaseline    = "baseline"
	EventMoveStarted = "move_started"
	EventMoveDone    = "move_done"
	EventDone        = "done"
)

// Event is one step of a plan search, for progress display.
type Event struct {
	Kind   string       `json:"kind"`
	Target string       `json:"target,omitempty"` // started
	Moves  []MoveRef    `json:"moves,omitempty"`  // started: every move to be searched
	Move   string       `json:"key,omitempty"`    // move_started, move_done
	Plan   *domain.Plan `json:"plan,omitempty"`   // baseline, move_done (economics filled in)
	Result *Result      `json:"result,omitempty"` // done
	Done   int          `json:"done"`             // moves finished
	Total  int          `json:"total"`            // moves searched
}

// MoveRef names a move before it is searched.
type MoveRef struct {
	Key         string      `json:"key"`
	Description string      `json:"description"`
	Move        domain.Move `json:"move"`
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
	if opts.NearMissFloor <= 0 {
		opts.NearMissFloor = DefaultNearMissFloor
	}
	if opts.NearMissMax <= 0 {
		opts.NearMissMax = DefaultNearMissMax
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
	VerifierCached int     `json:"verifier_cached"` // verdicts reused from the verifier cache
	VerifierModel  int     `json:"verifier_model_calls"`
	ToolCalls      int     `json:"verifier_tool_calls"`
	InputTokens    int     `json:"input_tokens"`
	OutputTokens   int     `json:"output_tokens"`
	EngineInTok    int     `json:"engine_input_tokens"`
	EngineOutTok   int     `json:"engine_output_tokens"`
	VerifierInTok  int     `json:"verifier_input_tokens"`
	VerifierOutTok int     `json:"verifier_output_tokens"`
	NearMissCalls  int     `json:"near_miss_calls,omitempty"` // advice model calls made (cached answers not counted)
	NearMissInTok  int     `json:"near_miss_input_tokens,omitempty"`
	NearMissOutTok int     `json:"near_miss_output_tokens,omitempty"`
	AdviceFollowed int     `json:"advice_followed,omitempty"` // moves added from near-miss advice (D85)
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
	addVerifier := func(vr verifier.Result) {
		mu.Lock()
		defer mu.Unlock()
		res.Stats.VerifierRuns++
		if vr.Cached {
			res.Stats.VerifierCached++
		}
		res.Stats.VerifierModel += vr.ModelCalls
		res.Stats.ToolCalls += len(vr.Steps)
		res.Stats.VerifierInTok += vr.Usage.InputTokens
		res.Stats.VerifierOutTok += vr.Usage.OutputTokens
	}

	moves := EnumerateUpTo(req.Product, tm, target, req.Economics.GroupCosts, p.opts.MaxMoves)
	res.Stats.MovesSearched = len(moves)
	done, total := 0, len(moves)
	emit := func(e Event) { // callers hold mu
		if progress != nil {
			e.Done, e.Total = done, total
			progress(e)
		}
	}
	refs := make([]MoveRef, len(moves))
	for i, m := range moves {
		refs[i] = MoveRef{Key: m.Key(), Description: Describe(req.Product, tm, m), Move: m}
	}
	mu.Lock()
	emit(Event{Kind: EventStarted, Target: target, Moves: refs})
	mu.Unlock()

	var knownA, knownRuling string
	if k := req.CurrentOrigin; k != nil && k.Country != "" {
		knownA, knownRuling = domain.NormalizeCountry(k.Country), k.Ruling
		res.CurrentOrigin = &KnownOrigin{Country: knownA, Ruling: knownRuling}
	}
	// The baseline runs alongside the moves: they need its facts now and its
	// origin only for economics, which each move waits for when it finishes.
	baseFacts := Render(req.Product, tm)
	var baseErr error
	baseOrigin := ""
	baseReady := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(baseReady)
		r, err := p.engine.Predict(ctx, p.caseFor(req, "baseline", baseFacts))
		if err != nil {
			baseErr = fmt.Errorf("planner: baseline: %w", err)
			return
		}
		addEngine(r.Usage)
		res.Baseline = domain.Plan{Move: domain.Move{}, Description: Describe(req.Product, tm, nil), VariantFacts: baseFacts, Prediction: r.Prediction}
		baseOrigin = r.Prediction.Origin
		if knownA != "" {
			baseOrigin = knownA
		}
		mu.Lock()
		b := res.Baseline
		emit(Event{Kind: EventBaseline, Plan: &b})
		mu.Unlock()
	}()

	plans := make([]domain.Plan, len(moves))
	sem := make(chan struct{}, p.opts.Concurrency)
	errs := make([]error, len(moves))
	// complete fills a move's economics once the baseline is known, stores
	// it and reports it.
	complete := func(i int, pl domain.Plan) {
		<-baseReady
		if baseErr == nil {
			pl.DutyEffectYear, pl.AddedCostYear = economics(req, moves[i], baseOrigin, pl.Prediction.Origin)
			pl.NetYear = pl.DutyEffectYear - pl.AddedCostYear
		}
		mu.Lock()
		defer mu.Unlock()
		plans[i] = pl
		done++
		emit(Event{Kind: EventMoveDone, Move: pl.Move.Key(), Plan: &pl})
	}
	for i, m := range moves {
		wg.Add(1)
		go func(i int, m domain.Move) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			mu.Lock()
			emit(Event{Kind: EventMoveStarted, Move: m.Key()})
			mu.Unlock()
			pl, err := p.check(ctx, req, tm, m, refs[i].Description, baseFacts, target, knownA, knownRuling, addEngine, addVerifier)
			if err != nil {
				errs[i] = err
				return
			}
			complete(i, pl)
		}(i, m)
	}
	wg.Wait()
	if err := errors.Join(append([]error{baseErr}, errs...)...); err != nil {
		return Result{}, err
	}
	for _, pl := range plans {
		if pl.Shown {
			res.Plans = append(res.Plans, pl)
		} else {
			res.Rejected = append(res.Rejected, pl)
		}
	}
	order(&res)
	if baseOrigin != target {
		b := res.Baseline
		b.RejectionReason = fmt.Sprintf("current setup: engine predicts origin %s", b.Prediction.Origin)
		b.RejectionKind = domain.RejectCurrentSetup
		if knownA != "" {
			b.RejectionReason = fmt.Sprintf("current setup: CBP held %s (%s); engine predicts %s", knownA, knownRuling, b.Prediction.Origin)
		}
		res.Rejected = append([]domain.Plan{b}, res.Rejected...)
	}
	p.adviseNearMisses(ctx, req, &res)
	if p.opts.FollowAdvice && p.opts.NearMiss != nil {
		if m, ok := p.followable(ctx, req, tm, target, moves, &res); ok {
			key := m.Key()
			mu.Lock()
			total++
			res.Stats.MovesSearched++
			emit(Event{Kind: EventMoveStarted, Move: key})
			mu.Unlock()
			pl, err := p.check(ctx, req, tm, m, Describe(req.Product, tm, m), baseFacts, target, knownA, knownRuling, addEngine, addVerifier)
			if err == nil { // a failed followed move leaves the search as it was
				pl.FromAdvice = true
				pl.DutyEffectYear, pl.AddedCostYear = economics(req, m, baseOrigin, pl.Prediction.Origin)
				pl.NetYear = pl.DutyEffectYear - pl.AddedCostYear
				if pl.Shown {
					res.Plans = append(res.Plans, pl)
				} else {
					res.Rejected = append(res.Rejected, pl)
				}
				res.Stats.AdviceFollowed++
				order(&res)
				mu.Lock()
				done++
				emit(Event{Kind: EventMoveDone, Move: key, Plan: &pl})
				mu.Unlock()
			}
		}
	}
	if req.ActualChange != nil {
		res.Backtest = backtest(req, res.Plans)
	}
	res.Stats.InputTokens = res.Stats.EngineInTok + res.Stats.VerifierInTok + res.Stats.NearMissInTok
	res.Stats.OutputTokens = res.Stats.EngineOutTok + res.Stats.VerifierOutTok + res.Stats.NearMissOutTok
	res.Stats.WallSeconds = time.Since(start).Seconds()
	mu.Lock()
	r := res
	emit(Event{Kind: EventDone, Result: &r})
	mu.Unlock()
	return res, nil
}

// check predicts one move and, when the engine says it reaches the target,
// verifies it against precedent. Economics are filled in by the caller.
func (p *Planner) check(ctx context.Context, req Request, tm Template, m domain.Move, desc, baseFacts, target, knownA, knownRuling string,
	addEngine func(llm.Usage), addVerifier func(verifier.Result)) (domain.Plan, error) {
	variant := Apply(req.Product, m)
	facts := Render(variant, tm, groupsOf(m)...)
	r, err := p.engine.Predict(ctx, p.caseFor(req, m.Key(), facts))
	if err != nil {
		return domain.Plan{}, fmt.Errorf("planner: move %s: %w", m.Key(), err)
	}
	addEngine(r.Usage)
	pl := domain.Plan{Move: m, Description: desc, VariantFacts: facts, Prediction: r.Prediction}
	if r.Prediction.Origin != target {
		pl.RejectionReason = fmt.Sprintf("engine predicts origin %s, not %s", r.Prediction.Origin, target)
		pl.RejectionKind = domain.RejectOtherOrigin
		return pl, nil
	}
	vr, err := p.verifier.Verify(ctx, verifier.Task{FactsA: baseFacts, FactsB: facts, Target: target, Before: req.Date.Time,
		Exclude: req.Exclude, KnownOriginA: knownA, KnownRulingA: knownRuling})
	addVerifier(vr)
	if err != nil {
		pl.RejectionReason = "precedent check failed: " + err.Error()
		pl.RejectionKind = domain.RejectCheckFailed
		return pl, nil
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
	return pl, nil
}

// order sorts shown plans by net (best first) and rejected ones closest call
// first: precedent-supported but below the confidence bar (best net first),
// then other verified moves, then moves the engine says miss the target. The
// current setup, when present, stays first.
func order(res *Result) {
	sort.SliceStable(res.Plans, func(i, j int) bool { return res.Plans[i].NetYear > res.Plans[j].NetYear })
	rank := map[domain.RejectionKind]int{domain.RejectCurrentSetup: -1, domain.RejectBelowConfidence: 0, domain.RejectNoPrecedent: 1, domain.RejectCheckFailed: 2, domain.RejectOtherOrigin: 3}
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
}

// adviseNearMisses adds near-miss advice to the closest below-bar moves:
// highest check first (ties keep list order), check at least NearMissFloor,
// at most NearMissMax. A failed advice call leaves that move without advice;
// it never fails the search or changes a verdict.
func (p *Planner) adviseNearMisses(ctx context.Context, req Request, res *Result) {
	if p.opts.NearMiss == nil {
		return
	}
	var idx []int
	for i, pl := range res.Rejected {
		if pl.RejectionKind == domain.RejectBelowConfidence && pl.Verdict != nil &&
			pl.Verdict.Confidence >= p.opts.NearMissFloor && pl.Verdict.Confidence < p.opts.PlanConfidence &&
			domain.ValidRulingID(pl.Verdict.SupportingRuling) {
			idx = append(idx, i)
		}
	}
	sort.SliceStable(idx, func(a, b int) bool {
		return res.Rejected[idx[a]].Verdict.Confidence > res.Rejected[idx[b]].Verdict.Confidence
	})
	if len(idx) > p.opts.NearMissMax {
		idx = idx[:p.opts.NearMissMax]
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, i := range idx {
		wg.Add(1)
		go func(pl *domain.Plan) {
			defer wg.Done()
			n, err := p.opts.NearMiss.Advise(ctx, nearmiss.Task{Facts: pl.VariantFacts, Ruling: pl.Verdict.SupportingRuling,
				Before: req.Date.Time, Exclude: req.Exclude})
			if err != nil {
				return
			}
			pl.NearMiss = &n
			if !n.Trace.Cached {
				mu.Lock()
				res.Stats.NearMissCalls++
				res.Stats.NearMissInTok += n.Trace.InputTokens
				res.Stats.NearMissOutTok += n.Trace.OutputTokens
				mu.Unlock()
			}
		}(&res.Rejected[i])
	}
	wg.Wait()
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
