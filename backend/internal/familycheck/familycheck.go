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

// PromptVersion names the prompt below. v2 (D850) shows a template's scope,
// when it has one, beside its name and groups.
const PromptVersion = "family-check-v2"

// None is the answer when no template fits.
const None = "none"

// Result is one check.
type Result struct {
	Family     string  `json:"family"` // a template family, or "none"
	Name       string  `json:"name,omitempty"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
	// ProductName is a short name for the product (e.g. "Hair dryer"),
	// for a BOM that names none.
	ProductName string `json:"product_name,omitempty"`
	// TypedUnclear is true when the line the user typed ("Typed:") names no
	// product or product type (e.g. random letters), whatever the parts
	// suggest, so a match is never credited to it (18 Oct).
	TypedUnclear bool `json:"typed_unclear"`
	// ModelFamily, ModelReason and GuardRule keep the model's answer and the
	// guard rule (GuardVersion) when the guard took the family away (D783).
	// They are for logs and the coverage eval, not the API response.
	ModelFamily string `json:"-"`
	ModelReason string `json:"-"`
	GuardRule   string `json:"-"`
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
	b.WriteString("Do not stretch: a product that shares no operation groups with a family is \"none\". Confidence is 0 to 1. ")
	b.WriteString("Also give product_name: what the finished product is, in 2 to 5 plain words (e.g. \"Hair dryer\"), from its name or parts. ")
	b.WriteString("When the input has a \"Typed:\" line, set typed_unclear to true if that line alone names no product or product type (random letters, a typo you can't read, an unrelated word), even when the parts point to a family; otherwise false.")
	scoped := false
	for id, t := range templates {
		c.ids = append(c.ids, id)
		scoped = scoped || t.Scope != ""
	}
	if scoped {
		// v2 (D850): a family's scope settles a boundary its name and groups
		// leave open.
		b.WriteString(" When a family gives a scope, follow it: it says what the family covers and which family takes what it does not.")
	}
	b.WriteString("\n\nFamilies:\n")
	sort.Strings(c.ids)
	for _, id := range c.ids {
		t := templates[id]
		c.names[id] = t.Name
		groups := make([]string, 0, len(t.Groups))
		for _, g := range t.Groups {
			groups = append(groups, g.Name)
		}
		fmt.Fprintf(&b, "- %s (%s): operation groups %s\n", id, t.Name, strings.Join(groups, ", "))
		if t.Scope != "" {
			fmt.Fprintf(&b, "  Scope: %s\n", t.Scope)
		}
	}
	c.system = b.String()
	return c
}

func (c *Checker) schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"family":        map[string]any{"type": "string", "enum": append(slices.Clone(c.ids), None)},
			"confidence":    map[string]any{"type": "number"},
			"reason":        map[string]any{"type": "string"},
			"product_name":  map[string]any{"type": "string"},
			"typed_unclear": map[string]any{"type": "boolean"},
		},
		"required": []string{"family", "confidence", "reason"},
	}
}

// Check matches text (what the user typed, plus anything known about the
// product) to a family.
func (c *Checker) Check(ctx context.Context, text string) (Result, error) {
	r, _, err := c.CheckUsage(ctx, text)
	return r, err
}

// CheckUsage is Check that also returns the model call's token usage (for
// the coverage eval's cost accounting, D138).
func (c *Checker) CheckUsage(ctx context.Context, text string) (Result, llm.Usage, error) {
	resp, err := c.model.Generate(ctx, llm.Request{
		Model:      c.modelID,
		System:     c.system,
		Messages:   []llm.Message{{Role: llm.RoleUser, Content: "PRODUCT:\n" + text}},
		JSONSchema: c.schema(),
		SchemaName: "family",
		// 2048: with thinking billed as output, 400 cut off 31 of 469
		// development checks before the JSON (D138), and with 14 families
		// 1024 still cut off 35 of 469 (D212). 4096 (D854): every failed
		// check in the recorded runs spent the 2048 cap on both attempts
		// (about 2,034 output tokens each) and returned no JSON.
		MaxOutputTokens: 4096,
	})
	if err != nil {
		return Result{}, llm.Usage{}, fmt.Errorf("familycheck: %w", err)
	}
	var r Result
	if err := resp.DecodeJSON(&r); err != nil {
		return Result{}, resp.Usage, fmt.Errorf("familycheck: %w", err)
	}
	if r.Family != None && !slices.Contains(c.ids, r.Family) {
		return Result{}, resp.Usage, fmt.Errorf("familycheck: %w: family %q", llm.ErrBadResponse, r.Family)
	}
	r.Name = c.names[r.Family]
	r.Confidence = min(max(r.Confidence, 0), 1)
	// Deterministic checks after the model (D783): a match the product line
	// contradicts goes to "none" for review.
	r, _ = Guard(text, r)
	return r, resp.Usage, nil
}
