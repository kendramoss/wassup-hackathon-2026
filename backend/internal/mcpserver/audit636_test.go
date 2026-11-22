package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planrule"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

// #636, end to end: a Part 102 search with no plan prints the current setup
// as the closest rejected row. planrule flags it with safety flag S (D591),
// which no precedent check ran on; the tool's text names the engine-vs-rule
// disagreement, not the precedent check.
func TestPlanToolWordsSafetyFlagSAsEngineVsRule(t *testing.T) {
	rules, err := tariffshift.Default()
	if err != nil {
		t.Fatal(err)
	}
	// Power supply 8504.40 assembled in Vietnam from a Chinese PCBA (8504.90)
	// and housing (3926.90): the 102.20 rule gives Vietnam.
	p := domain.Product{Name: "Power supply", HTS: "8504.40.95",
		Plants: []domain.Plant{{ID: "cn", Country: "China"}, {ID: "vn", Country: "Vietnam"}},
		Components: []domain.Component{
			{Name: "printed circuit assembly", MadeIn: "China", Plant: "cn", Group: "pcba", HTS: "8504.90.75"},
			{Name: "plastic housing", MadeIn: "China", Plant: "cn", Group: "housing", HTS: "3926.90"}},
		Operations: []domain.Operation{{Name: "SMT", Plant: "cn", Group: "pcba"}, {Name: "mould", Plant: "cn", Group: "housing"},
			{Name: "assemble and test", Plant: "vn", Group: "final_assembly"}}}
	base := domain.Plan{Description: "No change: keep the current setup", Prediction: domain.Prediction{Origin: "China"}}
	cur := base
	cur.RejectionKind, cur.RejectionReason = domain.RejectCurrentSetup, "current setup: engine predicts origin China"
	ann := planrule.Annotate(rules, p, planner.Result{Target: "Vietnam", Baseline: base, Rejected: []domain.Plan{cur}}, true)
	if rc := ann.Rejected[0].RuleCheck; rc == nil || !rc.Review || ann.Rejected[0].Verdict != nil {
		t.Fatalf("fixture: want an S flag with no precedent verdict, got %+v", ann.Rejected[0])
	}
	body, _ := json.Marshal(map[string]any{"case_id": "case_x", "plan": ann})
	cs := connect(t, &stubAPI{body: map[string]string{"POST /v1/cases/case_x/plan": string(body)}}, false, nil)
	_, txt, _ := call(t, cs, "plan_production_moves", map[string]any{"case_id": "case_x", "target_country": "Vietnam"})
	if strings.Contains(txt, "it differs from the precedent check") {
		t.Errorf("S flag (no precedent check ran) described as a precedent disagreement:\n%s", txt)
	}
	if !strings.Contains(txt, "the engine predicts China for the current setup, but the 102.20 rule gives Vietnam") {
		t.Errorf("the engine-vs-rule disagreement is not named:\n%s", txt)
	}
}
