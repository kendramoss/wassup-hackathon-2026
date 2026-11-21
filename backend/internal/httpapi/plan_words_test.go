package httpapi

import (
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// A scenario that makes the same move as a plan row carries that row's
// precedent check (D220), so Scenarios and Plan can't disagree on one move.
func TestScenarioCarriesThePlanCheckOfTheSameMove(t *testing.T) {
	tms, err := planner.LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	a := &api{d: Deps{Templates: tms, PlanConfidence: 0.92}}
	cur := domain.Product{Name: "heat gun",
		Plants: []domain.Plant{{ID: "cn", Name: "CN", Country: "China"}, {ID: "th", Name: "TH", Country: "Thailand"}},
		Components: []domain.Component{
			{Name: "motor", MadeIn: "China", Plant: "cn", Group: "motor_fan_assembly"},
			{Name: "board", MadeIn: "China", Plant: "cn", Group: "pcba"},
			{Name: "housing", MadeIn: "Thailand", Plant: "th", Group: "final_assembly"},
		}}
	motor := domain.Move{{Group: "motor_fan_assembly", ToPlant: "th"}}
	both := domain.Move{{Group: "motor_fan_assembly", ToPlant: "th"}, {Group: "pcba", ToPlant: "th"}}
	c := store.Case{Product: &cur, Family: "motorised_tools_small_appliances", Plan: &planner.Result{Family: "motorised_tools_small_appliances", Target: "Thailand",
		Plans: []domain.Plan{{Move: both, Shown: true, NetYear: 285000, Verdict: &domain.Verdict{Confidence: 0.96, SupportingRuling: "N345938", PrecedentSupported: true}}},
		Rejected: []domain.Plan{{Move: motor, Description: "Make Motor-fan assembly in Thailand", NetYear: 380000, RejectionKind: domain.RejectBelowConfidence,
			Verdict: &domain.Verdict{Confidence: 0.90, SupportingRuling: "N325810", PrecedentSupported: true}}},
	}}
	moved := planner.Apply(cur, motor)
	out := a.scenarioFields(c, []store.Scenario{{Label: "As today", Product: &cur}, {Label: "motor", Product: &moved}, {Label: "text only"}})
	if _, ok := out[0]["plan_check"]; ok {
		t.Error("a scenario that moves nothing has no plan check")
	}
	if _, ok := out[2]["plan_check"]; ok {
		t.Error("a description-only scenario has no plan check")
	}
	pc, _ := out[1]["plan_check"].(*scenarioPlanCheck)
	if pc == nil || pc.Shown || pc.Confidence == nil || *pc.Confidence != 0.90 || pc.Bar != 0.92 || pc.RejectionKind != domain.RejectBelowConfidence ||
		pc.Description != "Move the motor-fan assembly to Thailand" || pc.NetPerYear != 380000 || pc.Target != "Thailand" || pc.SupportingRuling != "N325810" {
		t.Errorf("plan_check = %+v", pc)
	}
	// The read view rewrites stored words and leaves the stored case alone.
	view := a.redescribed(c)
	if view.Plan.Rejected[0].Description != "Move the motor-fan assembly to Thailand" || c.Plan.Rejected[0].Description != "Make Motor-fan assembly in Thailand" {
		t.Errorf("redescribed = %q, stored = %q", view.Plan.Rejected[0].Description, c.Plan.Rejected[0].Description)
	}
}

// The case says which text its own prediction read (D220).
func TestFactsSource(t *testing.T) {
	tms, err := planner.LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	a := &api{d: Deps{Templates: tms}}
	p := domain.Product{Name: "heat gun", Plants: []domain.Plant{{ID: "th", Name: "TH", Country: "Thailand"}},
		Components: []domain.Component{{Name: "motor", MadeIn: "Thailand", Plant: "th", Group: "motor_fan_assembly"}}}
	fam := "motorised_tools_small_appliances"
	rendered := planner.Render(p, tms[fam])
	for _, c := range []struct {
		c    store.Case
		want string
	}{
		{store.Case{Product: &p, Family: fam, Facts: rendered + "\n"}, "parts_list"},
		{store.Case{Product: &p, Family: fam, Facts: rendered + "\n\nAnswer: yes.", OriginalFacts: rendered}, "parts_list"},
		{store.Case{Product: &p, Family: fam, Facts: "In your letter dated October 25, 2022, you requested a ruling."}, "pasted"},
		{store.Case{Facts: "text only"}, ""},
	} {
		if got := a.factsSource(c.c); got != c.want {
			t.Errorf("factsSource(%q) = %q, want %q", strings.SplitN(c.c.Facts, "\n", 2)[0], got, c.want)
		}
	}
}
