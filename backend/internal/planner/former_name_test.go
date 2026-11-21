package planner

import (
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// D342: the pinned heat-gun case was made before the power cord, knobs and
// covers were split from the switch group (D330). Its product still files the
// cord, knob and back cover under switch_cord_minor_parts, so the group keeps
// its old name for that product; a product made after the split reads the new
// one. The name never contradicts the parts listed under it.
func TestGroupKeepsItsFormerNameForAProductMadeBeforeTheSplit(t *testing.T) {
	r, tm := fixture(t, "heatgun_2022.json")
	pre := r.Product
	pre.Components = append([]domain.Component(nil), r.Product.Components...)
	for i := range pre.Components {
		if pre.Components[i].Group == "cord_knobs_covers" {
			pre.Components[i].Group = "switch_cord_minor_parts"
		}
	}
	if got := GroupNameFor(pre, tm, "switch_cord_minor_parts"); got != "Switch, cord and minor parts" {
		t.Errorf("pre-split product: %q", got)
	}
	if got := GroupNameFor(r.Product, tm, "switch_cord_minor_parts"); got != "Switch, wiring and minor parts" {
		t.Errorf("split product: %q", got)
	}
	m := domain.Move{{Group: "motor_fan_assembly", ToPlant: "th"}, {Group: "pcba", ToPlant: "th"}, {Group: "switch_cord_minor_parts", ToPlant: "th"}}
	if d := Describe(pre, tm, m); !strings.Contains(d, "switch, cord and minor parts") {
		t.Errorf("pre-split Describe = %q", d)
	}
	if d := Describe(r.Product, tm, m); !strings.Contains(d, "switch, wiring and minor parts") {
		t.Errorf("split Describe = %q", d)
	}
	// Groups without a former name are unaffected.
	if got := GroupNameFor(pre, tm, "pcba"); got != GroupName(tm, "pcba") {
		t.Errorf("pcba = %q", got)
	}
}

func TestTemplateRejectsAFormerNameWithoutItsSplitGroup(t *testing.T) {
	tm := Template{Family: "x", Groups: []TemplateGroup{
		{OperationGroup: domain.OperationGroup{ID: "a", Name: "A", Operations: []string{"do a"}}, Movable: true, Former: &FormerName{Name: "A and B", Until: "b"}},
	}}
	if err := tm.Validate(); err == nil {
		t.Error("a former name pointing at a missing group validated")
	}
}
