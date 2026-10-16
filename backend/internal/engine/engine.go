package engine

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"sync"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
)

// Options configures the engine.
type Options struct {
	Model           string // e.g. gemini-4-argon
	K               int    // precedents per case
	MaxOutputTokens int
	// Votes > 1 re-asks low-confidence cases (first answer below VoteBelow)
	// Votes-1 more times with the precedents in rotated order, still at
	// temperature 0, and returns the majority origin (D22).
	Votes     int
	VoteBelow float64
	// Rules, when set, are appended to the system prompt (eval experiments).
	Rules        string
	RulesVersion string
	// PrecedentReasoning adds each precedent's CBP reasoning (Ruling.Reasoning)
	// to the user message (eval experiments).
	PrecedentReasoning bool
}

// Engine predicts origin from facts plus dated precedents.
type Engine struct {
	model llm.Model
	retr  retrieval.Retriever
	opts  Options
}

// New returns an engine. retr may be nil for the facts-only condition.
func New(model llm.Model, retr retrieval.Retriever, opts Options) *Engine {
	return &Engine{model: model, retr: retr, opts: opts}
}

// Result is a prediction plus fields only the eval keeps.
type Result struct {
	Prediction domain.Prediction
	// Flip is the model's "smallest change that would flip origin" (eval only;
	// the product does not show it).
	Flip string
	// Usage is the token accounting over every call made for this case.
	Usage llm.Usage
	// Votes is how many calls the answer is based on (1 without voting).
	Votes int
	// VoteCounts is how many calls gave each origin (nil without voting).
	VoteCounts map[string]int
}

// ErrNoFacts means the case has no facts text to analyse.
var ErrNoFacts = errors.New("engine: case has no facts")

type answer struct {
	Origin             string   `json:"origin_country"`
	EssentialCharacter string   `json:"essential_character"`
	DecisiveStep       string   `json:"decisive_step"`
	Flip               *string  `json:"flip"`
	Confidence         *float64 `json:"confidence"`
	Reasoning          string   `json:"reasoning"`
}

// Predict retrieves precedents dated strictly before c.Date (never c itself)
// and asks the model for the origin.
func (e *Engine) Predict(ctx context.Context, c domain.Case) (Result, error) {
	if c.Facts == "" {
		return Result{}, ErrNoFacts
	}
	if c.Date.IsZero() {
		return Result{}, fmt.Errorf("engine: case %s has no date to bound precedents", c.ID)
	}
	var precedents []domain.Ruling
	if e.retr != nil && e.opts.K > 0 {
		hits, err := e.retr.Search(ctx, c.Facts, e.opts.K, retrieval.PrecedentFilter(c))
		if err != nil {
			return Result{}, fmt.Errorf("engine: predict %s: retrieve: %w", c.ID, err)
		}
		for _, h := range hits {
			precedents = append(precedents, h.Ruling)
		}
	}
	first, err := e.ask(ctx, c, precedents)
	if err != nil {
		return Result{}, err
	}
	if e.opts.Votes <= 1 || first.Prediction.Confidence >= e.opts.VoteBelow || len(precedents) < 2 {
		return first, nil
	}
	return e.vote(ctx, c, precedents, first)
}

// vote asks again with rotated precedent orders and picks the majority
// origin; ties go to the higher summed confidence, then to the first answer.
// The returned prediction is the most confident call with the winning origin,
// its confidence unchanged, so voting cannot lift a case into the fast path.
func (e *Engine) vote(ctx context.Context, c domain.Case, precedents []domain.Ruling, first Result) (Result, error) {
	n := e.opts.Votes
	results := make([]Result, n)
	errs := make([]error, n)
	results[0] = first
	var wg sync.WaitGroup
	for i := 1; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			shift := i % len(precedents)
			rotated := append(append([]domain.Ruling{}, precedents[shift:]...), precedents[:shift]...)
			if i >= len(precedents) {
				for l, r := 0, len(rotated)-1; l < r; l, r = l+1, r-1 { // reversed rotations after a full cycle
					rotated[l], rotated[r] = rotated[r], rotated[l]
				}
			}
			results[i], errs[i] = e.ask(ctx, c, rotated)
		}(i)
	}
	wg.Wait()
	count, confSum := map[string]int{}, map[string]float64{}
	var usage llm.Usage
	ok := 0
	for i, r := range results {
		if errs[i] != nil {
			continue
		}
		ok++
		o := r.Prediction.Origin
		count[o]++
		confSum[o] += r.Prediction.Confidence
		usage.InputTokens += r.Usage.InputTokens
		usage.OutputTokens += r.Usage.OutputTokens
		usage.TotalTokens += r.Usage.TotalTokens
	}
	win := first.Prediction.Origin
	for o := range count {
		if count[o] > count[win] || (count[o] == count[win] && confSum[o] > confSum[win]) {
			win = o
		}
	}
	best := first
	if best.Prediction.Origin != win {
		best = Result{}
	}
	for i, r := range results {
		if errs[i] == nil && r.Prediction.Origin == win && r.Prediction.Confidence > best.Prediction.Confidence {
			best = r
		}
	}
	best.Usage = usage
	best.Prediction.Trace.InputTokens, best.Prediction.Trace.OutputTokens = usage.InputTokens, usage.OutputTokens
	best.Prediction.VoteAgreement = fmt.Sprintf("%d/%d", count[win], ok)
	best.Votes, best.VoteCounts = ok, count
	return best, nil
}

func (e *Engine) system() string {
	if e.opts.Rules == "" {
		return System
	}
	return System + "\n\n" + e.opts.Rules
}

func (e *Engine) promptVersion() string {
	v := PromptVersion
	if e.opts.Rules != "" {
		v += "+" + e.opts.RulesVersion
	}
	if e.opts.PrecedentReasoning {
		v += "+" + ReasoningVersion
	}
	return v
}

// ask makes one model call with the given precedents in the given order.
func (e *Engine) ask(ctx context.Context, c domain.Case, precedents []domain.Ruling) (Result, error) {
	resp, err := e.model.Generate(ctx, llm.Request{
		Model:           e.opts.Model,
		System:          e.system(),
		Messages:        []llm.Message{{Role: llm.RoleUser, Content: renderUser(c.Facts, precedents, e.opts.PrecedentReasoning)}},
		JSONSchema:      Schema,
		SchemaName:      "origin_prediction",
		MaxOutputTokens: e.opts.MaxOutputTokens,
	})
	if err != nil {
		return Result{}, fmt.Errorf("engine: predict %s: %w", c.ID, err)
	}
	var a answer
	if err := resp.DecodeJSON(&a); err != nil {
		return Result{}, fmt.Errorf("engine: predict %s: %w", c.ID, err)
	}
	ids := make([]string, len(precedents)) // never nil: APIs return [] not null
	for i, p := range precedents {
		ids[i] = p.ID
	}
	sort.Strings(ids) // order-independent, so votes over rotations report the same set
	pred := domain.Prediction{
		KeyFactors:   []string{},
		Origin:       domain.NormalizeCountry(a.Origin),
		DecisiveStep: a.DecisiveStep,
		Reasoning:    a.Reasoning,
		Precedents:   ids,
		CitedRulings: citedIn(a.Reasoning+" "+a.DecisiveStep+" "+a.EssentialCharacter, precedentIDs(precedents)),
		Trace: domain.Trace{
			Provider:      resp.Provider,
			ModelID:       resp.ModelID,
			PromptVersion: e.promptVersion(),
			InputTokens:   resp.Usage.InputTokens,
			OutputTokens:  resp.Usage.OutputTokens,
			LatencyMS:     resp.Latency.Milliseconds(),
		},
	}
	if a.EssentialCharacter != "" {
		// engine-v1 returns one driver; v2 may return several key factors.
		pred.KeyFactors = []string{a.EssentialCharacter}
	}
	if a.Confidence != nil {
		pred.Confidence = *a.Confidence
	}
	if err := pred.Validate(); err != nil {
		return Result{}, fmt.Errorf("engine: predict %s: %w: %v", c.ID, llm.ErrBadResponse, err)
	}
	r := Result{Prediction: pred, Usage: resp.Usage, Votes: 1}
	if a.Flip != nil {
		r.Flip = *a.Flip
	}
	return r, nil
}

func precedentIDs(ps []domain.Ruling) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.ID
	}
	return out
}

var rulingRef = regexp.MustCompile(`\b[NH][0-9]{6}\b`)

// citedIn returns the precedents named in text, in retrieval order. Ruling
// numbers that were not shown to the model are ignored, so a hallucinated
// number never reaches the user.
func citedIn(text string, shown []string) []string {
	out := []string{}
	named := map[string]bool{}
	for _, id := range rulingRef.FindAllString(text, -1) {
		named[id] = true
	}
	for _, id := range shown {
		if named[id] {
			out = append(out, id)
		}
	}
	return out
}

// PrecedentText is what the BM25 index stores for a ruling.
func PrecedentText(r domain.Ruling) string { return corpus.Truncate(r.Facts, retrieval.DocRunes) }
