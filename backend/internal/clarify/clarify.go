// Package clarify reviews a low-confidence prediction against the precedents
// the engine saw. It says whether an earlier ruling is on point, surfaces
// close rulings that went different ways (and the difference that decided
// them), and drafts up to three questions about the facts that would settle
// the case. It never changes the engine's origin.
package clarify

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// PromptVersion is recorded with every result.
const PromptVersion = "clarify-v1"

// System is the clarify prompt (new; developed on the dev split).
const System = `You review a U.S. customs origin prediction before it goes to a human expert. You get the product facts, the prediction, and the earlier CBP rulings that were retrieved for it (facts + CBP's holding).

1. ON POINT: is one of these earlier rulings on point — the same kind of product, with the same kind of processing question (which component or operation decides origin)? A ruling on a different product that only shares general doctrine is NOT on point.
2. CONFLICT: do two of these rulings on comparable products go different ways on the question that matters here (e.g. one credits the country that makes the filter media, another the country that pleats and assembles)? If so, name both and the fact that distinguished them in CBP's view.
3. QUESTIONS: list up to 3 short questions to the requester whose answers would most change the origin call — facts missing from the description that CBP relied on in the closest rulings (where a specific component is made, what an operation consists of, value shares, whether parts arrive finished). Ask nothing already answered in the facts. No questions if the facts are sufficient.

Only cite rulings from the list given. Return ONLY JSON.`

// Schema constrains the answer.
var Schema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"on_point":        map[string]any{"type": "boolean"},
		"closest_ruling":  map[string]any{"type": []string{"string", "null"}},
		"on_point_reason": map[string]any{"type": "string"},
		"conflict": map[string]any{"type": []string{"object", "null"}, "properties": map[string]any{
			"ruling_a": map[string]any{"type": "string"}, "holding_a": map[string]any{"type": "string"},
			"ruling_b": map[string]any{"type": "string"}, "holding_b": map[string]any{"type": "string"},
			"deciding_difference": map[string]any{"type": "string"},
		}, "required": []string{"ruling_a", "holding_a", "ruling_b", "holding_b", "deciding_difference"}, "additionalProperties": false},
		"questions": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 3},
	},
	"required":             []string{"on_point", "closest_ruling", "on_point_reason", "conflict", "questions"},
	"additionalProperties": false,
}

// Conflict is a pair of comparable earlier rulings that went different ways.
type Conflict struct {
	RulingA            string `json:"ruling_a"`
	HoldingA           string `json:"holding_a"`
	RulingB            string `json:"ruling_b"`
	HoldingB           string `json:"holding_b"`
	DecidingDifference string `json:"deciding_difference"`
}

// Result is the review.
type Result struct {
	OnPoint       bool         `json:"on_point"`
	ClosestRuling string       `json:"closest_ruling,omitempty"`
	Reason        string       `json:"on_point_reason"`
	Conflict      *Conflict    `json:"conflict,omitempty"`
	Questions     []string     `json:"questions"`
	Trace         domain.Trace `json:"trace"`
}

// NeedsInfo reports whether the case should go to the requester for facts
// first: no precedent on point and at least one question to ask.
func (r Result) NeedsInfo() bool { return !r.OnPoint && len(r.Questions) > 0 }

// Clarifier runs the review.
type Clarifier struct {
	model     llm.Model
	modelID   string
	maxTokens int
}

// New returns a clarifier.
func New(m llm.Model, modelID string, maxTokens int) *Clarifier {
	return &Clarifier{model: m, modelID: modelID, maxTokens: maxTokens}
}

type answer struct {
	OnPoint   bool      `json:"on_point"`
	Closest   *string   `json:"closest_ruling"`
	Reason    string    `json:"on_point_reason"`
	Conflict  *Conflict `json:"conflict"`
	Questions []string  `json:"questions"`
}

// Review checks a prediction against the precedents shown to the engine.
func (c *Clarifier) Review(ctx context.Context, facts string, p domain.Prediction, precedents []domain.Ruling) (Result, error) {
	var b strings.Builder
	shown := make([]string, len(precedents))
	for i, r := range precedents {
		shown[i] = r.ID
		fmt.Fprintf(&b, "[%s | %s] FACTS: %s\nCBP HOLDING: %s\n\n", r.ID, r.Date.Format("2006-01-02"),
			corpus.Truncate(r.Facts, 1500), corpus.Truncate(r.Holding, 600))
	}
	user := fmt.Sprintf("FACTS:\n%s\n\nPREDICTION: origin %s (confidence %.2f). Key factor: %s. Reasoning: %s\n\nEARLIER CBP RULINGS:\n%s",
		corpus.Truncate(facts, 12000), p.Origin, p.Confidence, strings.Join(p.KeyFactors, "; "), p.Reasoning, b.String())
	resp, err := c.model.Generate(ctx, llm.Request{Model: c.modelID, System: System,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: user}}, JSONSchema: Schema, SchemaName: "clarify", MaxOutputTokens: c.maxTokens})
	if err != nil {
		return Result{}, fmt.Errorf("clarify: %w", err)
	}
	var a answer
	if err := resp.DecodeJSON(&a); err != nil {
		return Result{}, fmt.Errorf("clarify: %w", err)
	}
	res := Result{OnPoint: a.OnPoint, Reason: a.Reason, Questions: []string{},
		Trace: domain.Trace{Provider: resp.Provider, ModelID: resp.ModelID, PromptVersion: PromptVersion,
			InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens, LatencyMS: resp.Latency.Milliseconds()}}
	if a.Closest != nil && slices.Contains(shown, strings.TrimSpace(*a.Closest)) {
		res.ClosestRuling = strings.TrimSpace(*a.Closest)
	}
	if res.OnPoint && res.ClosestRuling == "" {
		res.OnPoint = false // "on point" must name a ruling the engine actually saw
	}
	if k := a.Conflict; k != nil && k.RulingA != k.RulingB && slices.Contains(shown, k.RulingA) && slices.Contains(shown, k.RulingB) {
		res.Conflict = k
	}
	for _, q := range a.Questions {
		if q = strings.TrimSpace(q); q != "" && len(res.Questions) < 3 {
			res.Questions = append(res.Questions, q)
		}
	}
	return res, nil
}
