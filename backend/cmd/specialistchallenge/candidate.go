package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpengine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/logsafe"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/triage"
)

// Pools are the precedent rulings each specialist retrieves from.
type Pools struct {
	Textile, Pharma, Part102 []domain.Ruling
	// TextileV2 is the textile pool with each ruling's CBP reasoning, as the
	// deployed textile-v2 engine retrieves it (empty: Textile).
	TextileV2 []domain.Ruling
}

// Candidate is the set of engines under test, built the way the server builds
// them (production prompts, K=5, the engine model).
type Candidate struct {
	model   llm.Model
	modelID string
	pools   Pools
	wrap    func(retrieval.Retriever) retrieval.Retriever
	variant string // "", "cand-1" (every specialist) or "frozen" (the per-specialist choice in frozenVariants)
	eng     map[string]*engine.Engine
	jp      *jpengine.Engine
}

type runState struct {
	mu        sync.Mutex
	exclude   map[string]bool
	retrieved []string
	calls     int
}

type ctxKey struct{}

func stateFrom(ctx context.Context) *runState {
	s, _ := ctx.Value(ctxKey{}).(*runState)
	return s
}

// countingModel counts the requests that reach the gateway for one case.
type countingModel struct{ inner llm.Model }

func (m countingModel) Provider() string { return m.inner.Provider() }
func (m countingModel) Generate(ctx context.Context, req llm.Request) (llm.Response, error) {
	if s := stateFrom(ctx); s != nil {
		s.mu.Lock()
		s.calls++
		s.mu.Unlock()
	}
	return m.inner.Generate(ctx, req)
}

// ErrExcludedRetrieved stops a case whose source-answer ruling was retrieved.
var ErrExcludedRetrieved = errors.New("an excluded source ruling was retrieved")

// guard records every retrieved ruling id and refuses a retrieval that
// returns a ruling the case excludes, whatever the inner filter did.
type guard struct{ inner retrieval.Retriever }

func (g guard) Search(ctx context.Context, q string, k int, allow func(domain.Ruling) bool) ([]retrieval.Hit, error) {
	hits, err := g.inner.Search(ctx, q, k, allow)
	if err != nil {
		return nil, err
	}
	if s := stateFrom(ctx); s != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, h := range hits {
			if s.exclude[h.Ruling.ID] {
				return nil, fmt.Errorf("%w: %s", ErrExcludedRetrieved, h.Ruling.ID)
			}
			s.retrieved = append(s.retrieved, h.Ruling.ID)
		}
	}
	return hits, nil
}

// NewCandidate builds the engines. variant names a dev-selected prompt change
// ("" is the production configuration).
func NewCandidate(m llm.Model, modelID string, p Pools) (*Candidate, error) {
	c := &Candidate{model: countingModel{m}, modelID: modelID, pools: p}
	return c, c.build()
}

// WrapRetrievers puts f between each engine and its BM25 index (tests use it
// to simulate a retriever that ignores the case filter); the guard stays outside.
func (c *Candidate) WrapRetrievers(f func(retrieval.Retriever) retrieval.Retriever) {
	c.wrap = f
	_ = c.build()
}

func (c *Candidate) retr(rs []domain.Ruling) retrieval.Retriever {
	var r retrieval.Retriever = retrieval.NewBM25(rs, engine.PrecedentText)
	if c.wrap != nil {
		r = c.wrap(r)
	}
	return guard{r}
}

func (c *Candidate) build() error {
	base := engine.Options{Model: c.modelID, K: 5, MaxOutputTokens: 8000}
	t, ph, u := base, base, engine.USMCAProduction()
	t.System, t.Version = engine.TextileSystem, engine.TextileVersion
	ph.System, ph.Version = engine.PharmaSystem, engine.PharmaVersion
	u.Model, u.K, u.MaxOutputTokens = c.modelID, 5, 8000
	t, ph, u = c.variantOptions(t, ph, u)
	textilePool := c.pools.Textile
	if c.isLive() {
		// The deployed textile engine (deploy workflow: TEXTILE_PROMPT=v2): textile-v2
		// with each precedent's CBP reasoning, built as cmd/server builds it.
		t = base
		t.System, t.Version, t.PrecedentReasoning = engine.TextileSystem2, engine.TextileVersion2, true
		if c.variant == LiveCandidate {
			t = engine.WithMissingFactRule(t, true)
		}
		if len(c.pools.TextileV2) > 0 {
			textilePool = c.pools.TextileV2
		}
	}
	c.eng = map[string]*engine.Engine{
		"textile_specialist": engine.New(c.model, c.retr(textilePool), t),
		"pharma_specialist":  engine.New(c.model, c.retr(c.pools.Pharma), ph),
		"part102_specialist": engine.New(c.model, c.retr(c.pools.Part102), u),
	}
	jp, err := jpengine.NewDefault(c.model, c.modelID, jpengine.DefaultK)
	if err != nil {
		return err
	}
	if c.variantFor("japan_engine") == "cand-1" {
		jp = jp.WithUnknownStatusRule()
	}
	c.jp = jp
	return nil
}

// frozenVariants is the development-selected candidate, per specialist: the
// variant each specialist runs on the reserved cases. Empty string is the
// production configuration. It is filled in from the development runs and
// committed before the first reserved score.
// frozenLive is the comparator's development-selected textile variant (LiveBase
// or LiveCandidate), committed before the one comparator score on the reserved
// cases. Empty until selected.
var frozenLive = LiveBase

var frozenVariants = map[string]string{
	"textile_specialist": "cand-1", "pharma_specialist": "cand-1", "part102_specialist": "cand-1", "japan_engine": "cand-1",
}

func (c *Candidate) variantFor(feature string) string {
	if c.variant == "frozen" {
		return frozenVariants[feature]
	}
	return c.variant
}

func (c *Candidate) variantOptions(t, p, u engine.Options) (engine.Options, engine.Options, engine.Options) {
	if c.variantFor("textile_specialist") == "cand-1" {
		t.System, t.Version = engine.TextileSystem3, engine.TextileVersion3
		t = engine.WithMissingFactRule(t, true)
	}
	if c.variantFor("pharma_specialist") == "cand-1" {
		p = engine.WithMissingFactRule(p, true)
	}
	if c.variantFor("part102_specialist") == "cand-1" {
		u = engine.WithMissingFactRule(u, false)
	}
	return t, p, u
}

func routeOf(feature string, p domain.Prediction, bar float64) string {
	switch feature {
	case "textile_specialist":
		return string(triage.Textile(p, bar).Route)
	case "pharma_specialist":
		return string(triage.Pharma(p, bar).Route)
	default:
		return string(triage.USMCA(p, bar).Route)
	}
}

// Live comparator (D971 amendment 2): the deployed textile engine against the
// same engine plus the missing-fact rule and single-country guard.
const (
	LiveBase          = "live-v2"
	LiveCandidate     = "live-v2-missing"
	ComparatorFeature = "textile_live_comparator"
	// DeployedTextileBar is the deployed textile auto-accept bar (deploy workflow).
	DeployedTextileBar = 0.95
)

const comparatorTestFile = "textile-live-test.jsonl"

// The Japan comparator (D971 amendment 4): the unmodified engine, once.
const (
	JapanComparatorFeature  = "japan_live_comparator"
	japanComparatorTestFile = "japan-live-test.jsonl"
)

func (c *Candidate) isLive() bool { return c.variant == LiveBase || c.variant == LiveCandidate }

func (c *Candidate) barFor() float64 {
	if c.isLive() {
		return DeployedTextileBar
	}
	return ConfidentBar
}

// SelectLive applies the pre-registered rule: the candidate replaces the
// deployed prompt only if, on development, matched cases do not fall, errors
// and confident-wrong decisions do not rise, confident-correct decisions fall
// by at most one, and something improves (matched rises or fewer uncertain
// cases are answered confidently). Ties keep the deployed prompt.
func SelectLive(base, cand SplitSummary) string {
	ok := cand.Matched >= base.Matched && cand.Errors <= base.Errors && cand.Decided.ConfidentWrong <= base.Decided.ConfidentWrong &&
		cand.Decided.ConfidentCorrect >= base.Decided.ConfidentCorrect-1
	better := cand.Matched > base.Matched || cand.Uncertain.Overconfident < base.Uncertain.Overconfident
	if ok && better {
		return LiveCandidate
	}
	return LiveBase
}

// RunCase runs one case through its engine and grades it. Every outcome,
// including an error, is a row.
func RunCase(ctx context.Context, cand *Candidate, c Case) Row {
	st := &runState{exclude: map[string]bool{}}
	for _, id := range c.ExcludeRulings {
		st.exclude[id] = true
	}
	tally := &llm.CostTally{}
	ctx = context.WithValue(llm.ContextWithCostTally(ctx, tally), ctxKey{}, st)
	row := Row{ID: c.ID, Feature: c.Feature, Split: c.Split}
	date, err := time.Parse("2006-01-02", c.Input.Date)
	if err != nil {
		row.Error = "bad_date"
		return Grade(c, row)
	}
	dc := domain.Case{ID: c.ID, Date: date, Facts: c.Input.Facts, Exclude: slices.Clone(c.ExcludeRulings)}
	start := time.Now()
	var res engine.Result
	if c.Feature == "japan_engine" {
		if c.Input.Japan == nil {
			row.Error = "no_japan_input"
			return Grade(c, row)
		}
		dc.Japan = &domain.JapanInput{Agreement: c.Input.Japan.Agreement, HS: c.Input.Japan.HS}
		res, err = cand.jp.Predict(ctx, dc)
	} else {
		e := cand.eng[c.Feature]
		if e == nil {
			row.Error = "unknown_feature"
			return Grade(c, row)
		}
		res, err = e.Predict(ctx, dc)
	}
	row.LatencyMS = time.Since(start).Milliseconds()
	row.CostUSD = tally.USD()
	st.mu.Lock()
	row.Retrieved, row.CalledModel = slices.Clone(st.retrieved), st.calls > 0
	st.mu.Unlock()
	p := res.Prediction
	row.Provider, row.ModelID, row.PromptVersion = p.Trace.Provider, p.Trace.ModelID, p.Trace.PromptVersion
	row.Reasoning, row.DecisiveStep = p.Reasoning, p.DecisiveStep
	row.InputTokens, row.OutputTokens = p.Trace.InputTokens, p.Trace.OutputTokens
	if err != nil {
		code, status := logsafe.Classify(err)
		row.Error = string(code)
		if errors.Is(err, ErrExcludedRetrieved) {
			row.Error = "excluded_ruling_retrieved"
		} else if status > 0 {
			row.Error = fmt.Sprintf("%s (HTTP %d)", code, status)
		}
		if row.Error == "" {
			row.Error = "error"
		}
		return Grade(c, row)
	}
	if c.Feature == "japan_engine" {
		if p.Japan == nil {
			row.Error = "no_japan_result"
			return Grade(c, row)
		}
		row.Outcome, row.Criterion, row.Confidence = p.Japan.Outcome, p.Japan.Criterion, p.Confidence
		return Grade(c, row)
	}
	row.Origin, row.Confidence = p.Origin, p.Confidence
	row.Route = routeOf(c.Feature, p, cand.barFor())
	return Grade(c, row)
}

// SetVariant selects a dev-chosen candidate variant ("" is production).
func (c *Candidate) SetVariant(v string) error {
	switch v {
	case "", "cand-1", "frozen", LiveBase, LiveCandidate:
	default:
		return fmt.Errorf("unknown variant %q", v)
	}
	c.variant = v
	return c.build()
}
