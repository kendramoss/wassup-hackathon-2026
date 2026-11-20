package httpapi

import (
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// #368 (was #360): get_case is what the Ask agent grounds its claims in. A plan
// with no economics must read as not priced, and a plan whose precedent check
// did not run (no verdict) as not run, never as $0/yr or 0.00. Real zeros stay.
func caseTextLine(t *testing.T, c store.Case, marker string) string {
	t.Helper()
	for _, l := range strings.Split(agentCaseText(c), "\n") {
		if strings.Contains(l, marker) {
			return l
		}
	}
	t.Fatalf("no %q line in:\n%s", marker, agentCaseText(c))
	return ""
}

func planCase(econ *planner.Economics, shown, rejected domain.Plan) store.Case {
	return store.Case{ID: "c1", Date: "2025-01-01", Facts: "x\n", Economics: econ,
		Plan: &planner.Result{Target: "Thailand", Plans: []domain.Plan{shown}, Rejected: []domain.Plan{rejected}}}
}

func TestAgentCaseTextUnpricedPlanAndMissingVerdict(t *testing.T) {
	shown := domain.Plan{Description: "Make PCBA in Thailand", Prediction: domain.Prediction{Origin: "Thailand"}}
	rejected := domain.Plan{Description: "Move the motor", Prediction: domain.Prediction{Origin: "Thailand"}, RejectionKind: "no_precedent"}
	// Even a case that now holds money does not price a plan that was searched without it.
	for name, econ := range map[string]*planner.Economics{"no economics": nil, "economics added later": {AnnualImportValueUSD: 1000000, DutyRates: map[string]float64{"China": 0.3, "Thailand": 0.1}}} {
		c := planCase(econ, shown, rejected)
		s := caseTextLine(t, c, "SHOWN")
		if strings.Contains(s, "$0/yr") || strings.Contains(s, "0.00") || !strings.Contains(s, "net not priced") || !strings.Contains(s, "precedent check not run") {
			t.Errorf("%s: unpriced shown plan: %s", name, s)
		}
		r := caseTextLine(t, c, "not shown")
		if strings.Contains(r, "0.00") || !strings.Contains(r, "precedent check not run") {
			t.Errorf("%s: rejected plan with no verdict: %s", name, r)
		}
	}
}

func TestAgentCaseTextKeepsTruthfulZeroesAndNumbers(t *testing.T) {
	v := &domain.Verdict{Confidence: 0, SupportingRuling: ""}
	shown := domain.Plan{Description: "Make PCBA in Thailand", Prediction: domain.Prediction{Origin: "Thailand"}, Verdict: v, NetPriced: true}
	rejected := domain.Plan{Description: "Move the motor", Prediction: domain.Prediction{Origin: "Thailand"}, Verdict: v, RejectionKind: "below_bar", NetPriced: true}
	// Priced by the search: a computed net of exactly 0 is real, whatever the case holds now.
	c := planCase(nil, shown, rejected)
	s := caseTextLine(t, c, "SHOWN")
	if !strings.Contains(s, "net $0/yr") || !strings.Contains(s, "precedent check 0.00, supporting none") {
		t.Errorf("priced zero / zero-confidence verdict lost: %s", s)
	}
	if r := caseTextLine(t, c, "not shown"); !strings.Contains(r, "precedent check 0.00") {
		t.Errorf("rejected plan with a zero-confidence verdict: %s", r)
	}
	shown.Verdict = &domain.Verdict{Confidence: 0.93, SupportingRuling: "N350001"}
	shown.NetYear = 380000
	c = planCase(nil, shown, rejected)
	if s := caseTextLine(t, c, "SHOWN"); !strings.Contains(s, "net $380000/yr") || !strings.Contains(s, "precedent check 0.93, supporting N350001") {
		t.Errorf("priced plan: %s", s)
	}
	// A legacy plan (stored before the flag) with money on it but no flag is not claimed as priced.
	legacy := shown
	legacy.NetPriced = false
	if s := caseTextLine(t, planCase(nil, legacy, rejected), "SHOWN"); strings.Contains(s, "$380000") || !strings.Contains(s, "net not priced") {
		t.Errorf("legacy plan: %s", s)
	}
}
