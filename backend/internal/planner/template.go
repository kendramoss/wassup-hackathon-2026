package planner

import (
	"embed"
	"encoding/json"
	"fmt"
	"path"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// Templates are product-family definitions: the operation groups a planner
// may move. A new family is a new JSON file here; the planner core does not
// change.
//
//go:embed templates/*.json
var templateFS embed.FS

// Template is one product family.
type Template struct {
	Family  string          `json:"family"`
	Version string          `json:"version"`
	Name    string          `json:"name"`
	Note    string          `json:"note"`
	Groups  []TemplateGroup `json:"groups"`
	// Sources are the CBP rulings the family was written from (D138). Every
	// ID must be a substantial-transformation ruling in the bundled corpus;
	// a test checks it.
	Sources []string `json:"sources,omitempty"`
}

// TemplateGroup is an operation group with whether the planner may move it.
type TemplateGroup struct {
	domain.OperationGroup
	Movable bool `json:"movable"`
	// Precedent cites the rulings behind the group (D138). It documents the
	// template; the planner does not read it, and a plan is still checked by
	// the verifier against the rulings dated before the case.
	Precedent *GroupPrecedent `json:"precedent,omitempty"`
}

// GroupPrecedent is what CBP held about a group's work.
type GroupPrecedent struct {
	// ConfersOrigin: rulings where doing this work in a country was part of
	// what made that country the origin.
	ConfersOrigin []string `json:"confers_origin,omitempty"`
	// NotEnough: rulings where this work (or assembly with this group's
	// output imported) did not change the origin.
	NotEnough []string `json:"not_enough,omitempty"`
	Note      string   `json:"note,omitempty"`
}

// Group returns the group with the given ID.
func (t Template) Group(id string) (TemplateGroup, bool) {
	for _, g := range t.Groups {
		if g.ID == id {
			return g, true
		}
	}
	return TemplateGroup{}, false
}

// Validate checks group IDs are unique and each group is valid.
func (t Template) Validate() error {
	if t.Family == "" || len(t.Groups) == 0 {
		return fmt.Errorf("planner: template %q needs a family and groups", t.Family)
	}
	seen := map[string]bool{}
	for _, g := range t.Groups {
		if err := g.OperationGroup.Validate(); err != nil {
			return fmt.Errorf("planner: template %s: %w", t.Family, err)
		}
		if seen[g.ID] {
			return fmt.Errorf("planner: template %s: duplicate group %q", t.Family, g.ID)
		}
		seen[g.ID] = true
	}
	return nil
}

// LoadTemplates reads every embedded template, keyed by family.
func LoadTemplates() (map[string]Template, error) {
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		return nil, fmt.Errorf("planner: read templates: %w", err)
	}
	out := map[string]Template{}
	for _, e := range entries {
		b, err := templateFS.ReadFile(path.Join("templates", e.Name()))
		if err != nil {
			return nil, fmt.Errorf("planner: read %s: %w", e.Name(), err)
		}
		var t Template
		if err := json.Unmarshal(b, &t); err != nil {
			return nil, fmt.Errorf("planner: decode %s: %w", e.Name(), err)
		}
		if err := t.Validate(); err != nil {
			return nil, err
		}
		out[t.Family] = t
	}
	return out, nil
}
