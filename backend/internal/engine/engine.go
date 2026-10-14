package engine

import (
	"context"
	"errors"
	"fmt"
	"regexp"

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
	// Usage is the model call's token accounting.
	Usage llm.Usage
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
		for _, h := range e.retr.Search(c.Facts, e.opts.K, retrieval.PrecedentFilter(c)) {
			precedents = append(precedents, h.Ruling)
		}
	}
	resp, err := e.model.Generate(ctx, llm.Request{
		Model:           e.opts.Model,
		System:          System,
		Messages:        []llm.Message{{Role: llm.RoleUser, Content: RenderUser(c.Facts, precedents)}},
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
	ids := make([]string, len(precedents))
	for i, p := range precedents {
		ids[i] = p.ID
	}
	pred := domain.Prediction{
		Origin:       domain.NormalizeCountry(a.Origin),
		DecisiveStep: a.DecisiveStep,
		Reasoning:    a.Reasoning,
		Precedents:   ids,
		CitedRulings: citedIn(a.Reasoning+" "+a.DecisiveStep+" "+a.EssentialCharacter, ids),
		Trace: domain.Trace{
			Provider:      resp.Provider,
			ModelID:       resp.ModelID,
			PromptVersion: PromptVersion,
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
	r := Result{Prediction: pred, Usage: resp.Usage}
	if a.Flip != nil {
		r.Flip = *a.Flip
	}
	return r, nil
}

var rulingRef = regexp.MustCompile(`\b[NH][0-9]{6}\b`)

// citedIn returns the precedents named in text, in retrieval order. Ruling
// numbers that were not shown to the model are ignored, so a hallucinated
// number never reaches the user.
func citedIn(text string, shown []string) []string {
	named := map[string]bool{}
	for _, id := range rulingRef.FindAllString(text, -1) {
		named[id] = true
	}
	var out []string
	for _, id := range shown {
		if named[id] {
			out = append(out, id)
		}
	}
	return out
}

// PrecedentText is what the BM25 index stores for a ruling.
func PrecedentText(r domain.Ruling) string { return corpus.Truncate(r.Facts, retrieval.DocRunes) }
