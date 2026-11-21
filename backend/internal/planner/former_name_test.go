package planner

import (
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// preSplit is the heat-gun fixture as the pinned case files it: made before
// the power cord, knobs and covers were split from the switch group (D330),
// so the cord, knob and back cover are still under switch_cord_minor_parts.
func preSplit(t *testing.T) (domain.Product, domain.Product, Template) {
	t.Helper()
	r, tm := fixture(t, "heatgun_2022.json")
	pre := r.Product
	pre.Components = append([]domain.Component(nil), r.Product.Components...)
	for i := range pre.Components {
		if pre.Components[i].Group == "cord_knobs_covers" {
			pre.Components[i].Group = "switch_cord_minor_parts"
		}
	}
	return pre, r.Product, tm
}

// D342, D372: the former name comes from the version of the template the
// stored plan was made with, never from the parts alone. The pinned
// heat-gun plan was made with version 1, before the split, so it keeps
// "Switch, cord and minor parts"; a plan made now reads the new name.
func TestGroupKeepsItsFormerNameForAPlanMadeBeforeTheSplit(t *testing.T) {
	pre, split, tm := preSplit(t)
	if tm.Version != "2" {
		t.Fatalf("motorised template version %q: the split is version 2", tm.Version)
	}
	const id = "switch_cord_minor_parts"
	if got := GroupNameAt(pre, tm, id, "1"); got != "Switch, cord and minor parts" {
		t.Errorf("pre-split plan: %q", got)
	}
	for _, v := range []string{"2", ""} {
		if got := GroupNameAt(pre, tm, id, v); got != "Switch, wiring and minor parts" {
			t.Errorf("plan made with version %q: %q", v, got)
		}
	}
	if got := GroupNameFor(pre, tm, id); got != "Switch, wiring and minor parts" {
		t.Errorf("a plan made now: %q", got)
	}
	// A product that files its cord under the split-out group was filed
	// after the split, whatever the stored version says.
	if got := GroupNameAt(split, tm, id, "1"); got != "Switch, wiring and minor parts" {
		t.Errorf("split product: %q", got)
	}
	m := domain.Move{{Group: "motor_fan_assembly", ToPlant: "th"}, {Group: "pcba", ToPlant: "th"}, {Group: id, ToPlant: "th"}}
	if d := DescribeAt(pre, tm, m, "1"); !strings.Contains(d, "switch, cord and minor parts") {
		t.Errorf("pre-split DescribeAt = %q", d)
	}
	if d := Describe(split, tm, m); !strings.Contains(d, "switch, wiring and minor parts") {
		t.Errorf("split Describe = %q", d)
	}
	// Groups without a former name are unaffected.
	if got := GroupNameAt(pre, tm, "pcba", "1"); got != GroupName(tm, "pcba") {
		t.Errorf("pcba = %q", got)
	}
}

// #533: motorised products with no cord, knobs or covers, cordless tools
// included, read the template's name when planned today.
func TestProductsWithoutACordReadTheNewGroupName(t *testing.T) {
	_, tm := fixture(t, "heatgun_2022.json")
	for _, name := range []string{"planner_post/stove_heater_2026.json", "planner_families/n350310.json", "planner_post/purifier_waffle_2026.json", "heatgun_2022.json"} {
		r, rtm := fixture(t, name)
		if rtm.Family != tm.Family {
			t.Fatalf("%s: family %q", name, rtm.Family)
		}
		if got := GroupNameFor(r.Product, rtm, "switch_cord_minor_parts"); got != "Switch, wiring and minor parts" {
			t.Errorf("%s: %q", name, got)
		}
	}
	cordless := domain.Product{Family: tm.Family, Components: []domain.Component{
		{Name: "trigger switch", Group: "switch_cord_minor_parts", MadeIn: "China"},
		{Name: "battery pack", Group: "pcba", MadeIn: "China"},
	}}
	if got := GroupNameFor(cordless, tm, "switch_cord_minor_parts"); got != "Switch, wiring and minor parts" {
		t.Errorf("cordless tool: %q", got)
	}
}

func TestTemplateRejectsAFormerNameWithoutItsSplitGroup(t *testing.T) {
	g := func(f *FormerName) Template {
		return Template{Family: "x", Version: "2", Groups: []TemplateGroup{
			{OperationGroup: domain.OperationGroup{ID: "a", Name: "A", Operations: []string{"do a"}}, Movable: true, Former: f},
			{OperationGroup: domain.OperationGroup{ID: "b", Name: "B", Operations: []string{"do b"}}, Movable: true},
		}}
	}
	if err := g(&FormerName{Name: "A and B", Until: "c", Since: "2"}).Validate(); err == nil {
		t.Error("a former name pointing at a missing group validated")
	}
	if err := g(&FormerName{Name: "A and B", Until: "b"}).Validate(); err == nil {
		t.Error("a former name without the version that split it validated")
	}
	if err := g(&FormerName{Name: "A and B", Until: "b", Since: "3"}).Validate(); err == nil {
		t.Error("a split newer than the template validated")
	}
	if err := g(&FormerName{Name: "A and B", Until: "b", Since: "2"}).Validate(); err != nil {
		t.Error(err)
	}
}

// A stored plan search is redescribed with the names of the template version
// it was made with (D372).
func TestRedescribeUsesTheStoredTemplateVersion(t *testing.T) {
	pre, _, tm := preSplit(t)
	m := domain.Move{{Group: "motor_fan_assembly", ToPlant: "th"}, {Group: "switch_cord_minor_parts", ToPlant: "th"}}
	for v, want := range map[string]string{"1": "switch, cord and minor parts", "2": "switch, wiring and minor parts"} {
		r := Result{Family: tm.Family, Templates: map[string]string{tm.Family: v}, Plans: []domain.Plan{{Move: m}}}
		Redescribe(&r, &pre, map[string]Template{tm.Family: tm})
		if !strings.Contains(r.Plans[0].Description, want) {
			t.Errorf("version %s: %q, want %q", v, r.Plans[0].Description, want)
		}
	}
}
