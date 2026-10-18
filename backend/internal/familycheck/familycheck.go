// Package familycheck matches what a user types as a product family to one
// of the planner's templates (18 Oct). The model may only answer with a
// template that exists or "none", so it can't invent a family; "none" means
// origin prediction works but the plan search has no template for it.
package familycheck

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

// PromptVersion names the prompt below.
const PromptVersion = "family-check-v1"

// None is the answer when no template fits.
const None = "none"

// Result is one check.
type Result struct {
	Family     string  `json:"family"` // a template family, or "none"
	Name       string  `json:"name,omitempty"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

// Checker matches text to a template family.
type Checker struct {
	model   llm.Model
	modelID string
	ids     []string
	names   map[string]string
	system  string
}

// New lists the templates in the prompt and the answer schema.
func New(model llm.Model, modelID string, templates map[string]planner.Template) *Checker {
	c := &Checker{model: model, modelID: modelID, names: map[string]string{}}
	var b strings.Builder
	b.WriteString("You match a product a manufacturer describes to one product family that a production-planning tool supports. ")
	b.WriteString("Answer with the family whose operation groups fit how the product is made, or \"none\" when no family fits. ")
	b.WriteString("Do not stretch: a product that shares no operation groups with a family is \"none\". Confidence is 0 to 1.\n\nFamilies:\n")
	for id := range templates {
		c.ids = append(c.ids, id)
	}
	sort.Strings(c.ids)
	for _, id := range c.ids {
		t := templates[id]
		c.names[id] = t.Name
		groups := make([]string, 0, len(t.Groups))
		for _, g := range t.Groups {
			groups = append(groups, g.Name)
		}
		fmt.Fprintf(&b, "- %s (%s): operation groups %s\n", id, t.Name, strings.Join(groups, ", "))
	}
	c.system = b.String()
	return c
}

func (c *Checker) schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"family":     map[string]any{"type": "string", "enum": append(slices.Clone(c.ids), None)},
			"confidence": map[string]any{"type": "number"},
			"reason":     map[string]any{"type": "string"},
		},
		"required": []string{"family", "confidence", "reason"},
	}
}

// Check matches text (what the user typed, plus anything known about the
// product) to a family.
func (c *Checker) Check(ctx context.Context, text string) (Result, error) {
	resp, err := c.model.Generate(ctx, llm.Request{
		Model:           c.modelID,
		System:          c.system,
		Messages:        []llm.Message{{Role: llm.RoleUser, Content: "PRODUCT:\n" + text}},
		JSONSchema:      c.schema(),
		SchemaName:      "family",
		MaxOutputTokens: 400,
	})
	if err != nil {
		return Result{}, fmt.Errorf("familycheck: %w", err)
	}
	var r Result
	if err := resp.DecodeJSON(&r); err != nil {
		return Result{}, fmt.Errorf("familycheck: %w", err)
	}
	if r.Family != None && !slices.Contains(c.ids, r.Family) {
		return Result{}, fmt.Errorf("familycheck: %w: family %q", llm.ErrBadResponse, r.Family)
	}
	r.Name = c.names[r.Family]
	r.Confidence = min(max(r.Confidence, 0), 1)
	return r, nil
}
