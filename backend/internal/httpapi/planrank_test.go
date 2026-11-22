package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

// A power supply of 8504.40 assembled in China from a Chinese PCBA
// (8504.90) and housing (3926.90); each group can move to Mexico. Codes are
// illustrative.
func d500Case(t *testing.T, regime string) (*api, store.Case) {
	t.Helper()
	rules, err := tariffshift.Default()
	if err != nil {
		t.Fatal(err)
	}
	p := domain.Product{Name: "Power supply", HTS: "8504.40.95",
		Plants: []domain.Plant{{ID: "cn", Name: "China plant", Country: "China"}, {ID: "mx", Name: "Mexico plant", Country: "Mexico"}},
		Components: []domain.Component{
			{Name: "printed circuit assembly", MadeIn: "China", Plant: "cn", Group: "pcba", HTS: "8504.90.75"},
			{Name: "plastic housing", MadeIn: "China", Plant: "cn", Group: "housing", HTS: "3926.90"},
		},
		Operations: []domain.Operation{{Name: "SMT", Plant: "cn", Group: "pcba"}, {Name: "mould", Plant: "cn", Group: "housing"},
			{Name: "assemble and test", Plant: "cn", Group: "final_assembly"}},
	}
	mv := func(g string) domain.Move { return domain.Move{{Group: g, ToPlant: "mx"}} }
	shown := func(desc, g string, net float64) domain.Plan {
		return domain.Plan{Description: desc, Move: mv(g), Shown: true, Tier: domain.TierClearMargin, NetYear: net,
			Prediction: domain.Prediction{Origin: "Mexico"}, Verdict: &domain.Verdict{PrecedentSupported: true, Confidence: 0.95, SupportingRuling: "N340187"}}
	}
	res := planner.Result{Target: "Mexico", Ranking: &planner.Ranking{Rule: planner.RankingRule, Bar: 0.92, ClearFrom: 0.93},
		Stats: planner.Stats{MovesSearched: 2},
		// Ranked by net: the PCBA move first. The rule gives China for it
		// (finished in China) and Mexico for the assembly move.
		Plans: []domain.Plan{shown("Move the PCBA to Mexico", "pcba", 300), shown("Move final assembly to Mexico", "final_assembly", 100)}}
	c := store.Case{ID: "case_d500", Product: &p, Router: &router.Label{Regime: regime}, Plan: &res,
		Targets: &store.TargetComparison{Results: map[string]planner.Result{"Mexico": res, "Vietnam": {Target: "Vietnam", Stats: planner.Stats{MovesSearched: 2}}}}}
	c.Targets.Ranked = []store.TargetSummary{summarise("Mexico", res), summarise("Vietnam", c.Targets.Results["Vietnam"])}
	rankTargets(c.Targets.Ranked)
	return &api{d: Deps{RuleCheck: rules, PlanConfidence: 0.92}}, c
}

// D500: on a Part 102 case a read puts the move the rule supports first;
// plan_index in the draft counts that order; the comparison's best plan
// follows it and carries the check. Nothing stored is written.
func TestPart102ReadRanksByTheRuleAndDraftsItsOrder(t *testing.T) {
	a, c := d500Case(t, router.RegimeUSMCA)
	got := a.withPlanRuleChecks(c)
	ps := got.Plan.Plans
	if ps[0].Description != "Move final assembly to Mexico" || ps[0].RuleCheck.Status != "shifts" || ps[1].RuleCheck.Status != "other_country" || !ps[1].RuleCheck.Review {
		t.Fatalf("order %s, %s; %+v", ps[0].Description, ps[1].Description, ps[1].RuleCheck)
	}
	if got.Plan.Ranking.Rule != "tier_then_rule_then_net" || c.Plan.Plans[0].Description != "Move the PCBA to Mexico" || c.Plan.Plans[0].RuleCheck != nil || c.Plan.Ranking.Rule != planner.RankingRule {
		t.Fatal("the stored search was written to")
	}
	best := got.Targets.Ranked[0]
	if best.Target != "Mexico" || best.Best.Description != "Move final assembly to Mexico" || best.Best.RuleCheck == nil || best.Best.PlanID == "" {
		t.Fatalf("comparison best %+v", best.Best)
	}
	if c.Targets.Ranked[0].Best.Description != "Move the PCBA to Mexico" {
		t.Fatal("the stored ranking was written to")
	}
	zero := 0
	d, err := a.buildDraft(httptest.NewRequest("POST", "/", nil), c, exportRequest{PlanIndex: &zero})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d.content, "move final assembly to Mexico") || !strings.Contains(d.content, "Tariff-shift check (19 C.F.R. § 102.20): applied to the component codes in the bill of materials, with the good finished in Mexico, the rule gives Mexico") {
		t.Fatalf("draft:\n%s", d.content)
	}
	d, err = a.buildDraft(httptest.NewRequest("POST", "/", nil), c, exportRequest{PlanID: "pcba@mx", Target: "Mexico"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d.content, "the rule gives China") || !strings.Contains(strings.Join(d.notes, "\n"), "Tariff-shift check: An earlier ruling supports this move, but the 102.20 rule gives China") {
		t.Fatalf("flagged draft:\n%s\n%v", d.content, d.notes)
	}
}

// On a substantial-transformation case the order stays the planner's: the
// check is for reference, in the read, the comparison and the draft.
func TestInformationalReadKeepsTheOrder(t *testing.T) {
	a, c := d500Case(t, "substantial_transformation")
	got := a.withPlanRuleChecks(c)
	if ps := got.Plan.Plans; ps[0].Description != "Move the PCBA to Mexico" || ps[0].RuleCheck.Applies != "informational" || got.Plan.Ranking.Rule != planner.RankingRule {
		t.Fatalf("order %s %+v", ps[0].Description, ps[0].RuleCheck)
	}
	if b := got.Targets.Ranked[0].Best; b.Description != "Move the PCBA to Mexico" || b.RuleCheck == nil || b.RuleCheck.Applies != "informational" {
		t.Fatalf("comparison best %+v", b)
	}
	zero := 0
	d, err := a.buildDraft(httptest.NewRequest("POST", "/", nil), c, exportRequest{PlanIndex: &zero})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d.content, "for reference only, as this request is made under substantial transformation; applied to the component codes") {
		t.Fatalf("draft:\n%s", d.content)
	}
}

// On a Part 102 comparison a country whose best plan the rule supports
// ranks above one whose best plan the rule gives elsewhere (same tier).
func TestRankTargetsPart102RuleClass(t *testing.T) {
	best := func(status string, net float64) *domain.Plan {
		return &domain.Plan{NetYear: net, Tier: domain.TierClearMargin, RuleCheck: &domain.PlanRuleCheck{Applies: "part102", Status: status}}
	}
	ts := []store.TargetSummary{
		{Target: "Mexico", Status: TargetPlan, Best: best("other_country", 900)},
		{Target: "Canada", Status: TargetPlan, Best: best("shifts", 100)},
	}
	rankTargets(ts)
	if ts[0].Target != "Canada" {
		t.Fatalf("%s first", ts[0].Target)
	}
	ts[0].Best.RuleCheck.Applies, ts[1].Best.RuleCheck.Applies = "informational", "informational"
	rankTargets(ts)
	if ts[0].Target != "Mexico" {
		t.Fatalf("informational: %s first", ts[0].Target)
	}
}
