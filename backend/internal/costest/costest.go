// Package costest estimates the yearly cost of an operation group at a plant
// where the user gave none (#235 step 4, 18 Oct): one model call over the
// product's plants, groups and the costs the user did give. Only blank cells
// that a plant makes or could make are estimated; the user's figures are
// never changed. Every estimate is a range with a reason, and illustrative.
package costest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// PromptVersion names the prompt below.
const PromptVersion = "cost-estimate-v1"

// ErrNoAnchor means there is nothing to scale from: no import value and no
// group cost.
var ErrNoAnchor = fmt.Errorf("%w: add imports a year or at least one group cost, then estimate the rest", domain.ErrInvalid)

// Estimate is one blank cell, filled.
type Estimate struct {
	Group  string  `json:"group"`
	Plant  string  `json:"plant"`
	Low    float64 `json:"low"`
	USD    float64 `json:"usd"`
	High   float64 `json:"high"`
	Reason string  `json:"reason"`
}

// Result is one estimate run.
type Result struct {
	Estimates []Estimate `json:"estimates"`
	Note      string     `json:"note"`
}

// Input is what the estimate reads.
type Input struct {
	Product              domain.Product
	GroupNames           map[string]string // group ID -> display name
	GroupCosts           map[string]map[string]float64
	AnnualImportValueUSD float64
}

// Estimator fills blank group costs.
type Estimator struct {
	model   llm.Model
	modelID string
}

// New returns an estimator on the given model (a fast one: Flash).
func New(model llm.Model, modelID string) *Estimator {
	return &Estimator{model: model, modelID: modelID}
}

const system = `You estimate manufacturing costs for a production-planning tool. For each requested (group, plant) cell, estimate the yearly cost in USD of performing that operation group at that plant, for the same yearly volume as the known costs.

Scale from the known costs: a group's known cost at one plant is the anchor for the same group elsewhere, adjusted by typical differences in labour, overheads and logistics between the countries. A group with no known cost anywhere is estimated as a share of the yearly import value (the total landed value of the finished products), from how much of a product like this that group usually is.

Give usd (your best estimate), low and high (a plausible range; wider when you have less to go on), and a reason of one short sentence naming the anchor and the adjustment. Answer every requested cell once. Never invent cells that were not requested.`

// Blank lists the cells to estimate: each group the product has, at each
// plant that makes it or lists it as a capability, where the user gave no
// cost. Final assembly is left out, as it is everywhere else in the money
// inputs.
func Blank(p domain.Product, costs map[string]map[string]float64) [][2]string {
	groups := map[string]bool{}
	makes := map[string]map[string]bool{}
	add := func(g, plant string) {
		if g == "" || g == "final_assembly" {
			return
		}
		groups[g] = true
		if makes[g] == nil {
			makes[g] = map[string]bool{}
		}
		if plant != "" {
			makes[g][plant] = true
		}
	}
	for _, c := range p.Components {
		add(c.Group, c.Plant)
	}
	for _, o := range p.Operations {
		add(o.Group, o.Plant)
	}
	var out [][2]string
	ids := make([]string, 0, len(groups))
	for g := range groups {
		ids = append(ids, g)
	}
	sort.Strings(ids)
	for _, g := range ids {
		for _, pl := range p.Plants {
			if !makes[g][pl.ID] && !slices.Contains(pl.Capabilities, g) {
				continue
			}
			if _, ok := costs[g][pl.ID]; ok {
				continue
			}
			out = append(out, [2]string{g, pl.ID})
		}
	}
	return out
}

// Estimate fills the blank cells. No blank cells is not an error: the result
// is empty.
func (e *Estimator) Estimate(ctx context.Context, in Input) (Result, error) {
	cells := Blank(in.Product, in.GroupCosts)
	if len(cells) == 0 {
		return Result{Estimates: []Estimate{}, Note: "Every cell already has a cost."}, nil
	}
	known := 0
	for _, row := range in.GroupCosts {
		known += len(row)
	}
	if known == 0 && in.AnnualImportValueUSD <= 0 {
		return Result{}, ErrNoAnchor
	}
	var b strings.Builder
	fmt.Fprintf(&b, "PRODUCT: %s\n", strings.TrimSpace(in.Product.Name))
	if in.AnnualImportValueUSD > 0 {
		fmt.Fprintf(&b, "YEARLY IMPORT VALUE: $%.0f\n", in.AnnualImportValueUSD)
	}
	b.WriteString("PLANTS:\n")
	for _, pl := range in.Product.Plants {
		fmt.Fprintf(&b, "- %s: %s, %s\n", pl.ID, pl.Name, pl.Country)
	}
	b.WriteString("KNOWN YEARLY COSTS (group, plant, USD):\n")
	for g, row := range in.GroupCosts {
		for pl, v := range row {
			fmt.Fprintf(&b, "- %s at %s: $%.0f\n", name(in.GroupNames, g), pl, v)
		}
	}
	if known == 0 {
		b.WriteString("- none\n")
	}
	b.WriteString("CELLS TO ESTIMATE (group id, plant id):\n")
	groupIDs, plantIDs := []string{}, []string{}
	for _, c := range cells {
		fmt.Fprintf(&b, "- %s (%s) at %s\n", c[0], name(in.GroupNames, c[0]), c[1])
		if !slices.Contains(groupIDs, c[0]) {
			groupIDs = append(groupIDs, c[0])
		}
		if !slices.Contains(plantIDs, c[1]) {
			plantIDs = append(plantIDs, c[1])
		}
	}
	resp, err := e.model.Generate(ctx, llm.Request{
		Model:    e.modelID,
		System:   system,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: b.String()}},
		JSONSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"estimates": map[string]any{"type": "array", "items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"group":  map[string]any{"type": "string", "enum": groupIDs},
						"plant":  map[string]any{"type": "string", "enum": plantIDs},
						"low":    map[string]any{"type": "number"},
						"usd":    map[string]any{"type": "number"},
						"high":   map[string]any{"type": "number"},
						"reason": map[string]any{"type": "string"},
					},
					"required": []string{"group", "plant", "low", "usd", "high", "reason"},
				}},
			},
			"required": []string{"estimates"},
		},
		SchemaName:      "cost_estimates",
		MaxOutputTokens: 4000,
	})
	if err != nil {
		return Result{}, fmt.Errorf("costest: %w", err)
	}
	var raw struct {
		Estimates []Estimate `json:"estimates"`
	}
	if err := resp.DecodeJSON(&raw); err != nil {
		return Result{}, fmt.Errorf("costest: %w", err)
	}
	want := map[[2]string]bool{}
	for _, c := range cells {
		want[c] = true
	}
	out := []Estimate{}
	for _, x := range raw.Estimates {
		k := [2]string{x.Group, x.Plant}
		if !want[k] || x.USD <= 0 { // not requested, a repeat, or no answer
			continue
		}
		want[k] = false
		x.Low = min(max(x.Low, 0), x.USD)
		x.High = max(x.High, x.USD)
		out = append(out, x)
	}
	if len(out) == 0 {
		return Result{}, errors.Join(llm.ErrBadResponse, errors.New("costest: no usable estimate"))
	}
	return Result{Estimates: out, Note: "AI estimates, illustrative: scaled from your figures and typical cost differences between countries. Check them before relying on a plan's net per year."}, nil
}

func name(names map[string]string, id string) string {
	if n := names[id]; n != "" {
		return n
	}
	return id
}
