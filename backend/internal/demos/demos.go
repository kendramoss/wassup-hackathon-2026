// Package demos holds the demo cases the frontend's "load demo" button
// offers, embedded so the UI keeps no copy of them. Each demo is a ready
// predict request and, when the product has a family template, a plan
// request. Facts come from CBP rulings (public domain).
package demos

import (
	"embed"
	"encoding/json"
	"fmt"
	"path"
	"sort"
)

//go:embed fixtures/*.json
var fixtures embed.FS

// Demo is one demo case. Predict and Plan are request bodies for
// POST /v1/cases/predict and POST /v1/cases/{id}/plan.
type Demo struct {
	ID          string          `json:"id"`
	Order       int             `json:"order"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Source      string          `json:"source"`
	CBPHeld     string          `json:"cbp_held"`
	Notes       string          `json:"notes"`
	Predict     json.RawMessage `json:"predict"`
	Plan        json.RawMessage `json:"plan"`
}

// Summary is a list entry.
type Summary struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	CBPHeld     string `json:"cbp_held"`
	HasPlan     bool   `json:"has_plan"`
}

// Load returns all demos in display order.
func Load() ([]Demo, error) {
	entries, err := fixtures.ReadDir("fixtures")
	if err != nil {
		return nil, fmt.Errorf("demos: %w", err)
	}
	var out []Demo
	for _, e := range entries {
		b, err := fixtures.ReadFile(path.Join("fixtures", e.Name()))
		if err != nil {
			return nil, fmt.Errorf("demos: %w", err)
		}
		var d Demo
		if err := json.Unmarshal(b, &d); err != nil {
			return nil, fmt.Errorf("demos: %s: %w", e.Name(), err)
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out, nil
}

// Summaries lists the demos without their request bodies.
func Summaries(ds []Demo) []Summary {
	out := make([]Summary, len(ds))
	for i, d := range ds {
		out[i] = Summary{ID: d.ID, Title: d.Title, Description: d.Description, CBPHeld: d.CBPHeld,
			HasPlan: len(d.Plan) > 0 && string(d.Plan) != "null"}
	}
	return out
}
