package usmca

import "testing"

// Defects found on the development cases of the legal-rules challenge study
// (eval/challenges/legal-rules). Each test failed before its fix.

// A de minimis rescue counts every non-originating material that fails the
// shift. An unconfirmed materials list may hide more of them, so a listed
// total inside the 10 percent limit does not make the rule met.
func TestDeMinimisNeedsAConfirmedMaterialsList(t *testing.T) {
	rs := ext(t)
	in := deMinimisInput("5")
	in.Profile.MaterialsComplete = false
	out := run(t, rs, in)
	if out.Status != RuleNeedsInformation || !hasFact(out, "materials_incomplete") {
		t.Fatalf("unconfirmed list, 5 of 100: %s %v, want needs_information asking to confirm the list", out.Status, out.MissingFacts)
	}
	in = deMinimisInput("10.01")
	in.Profile.MaterialsComplete = false
	if out := run(t, rs, in); out.Status != RuleNotMet {
		t.Errorf("unconfirmed list but already over the limit: %s, want not_met", out.Status)
	}
	in = deMinimisInput("5")
	if out := run(t, rs, in); out.Status != RuleMet {
		t.Errorf("confirmed list, 5 of 100: %s, want met", out.Status)
	}
}

// The parts route needs a non-originating part that shares the good's
// classification. With an unconfirmed list, none being listed does not show
// that none exists.
func TestPartsRouteNoSharedPartNeedsAConfirmedList(t *testing.T) {
	rs := ext(t)
	in := partsInput(PartsBasisSameClassification, true, "40")
	in.Materials = nonOrig("7318")
	in.Profile.MaterialsComplete = false
	out := run(t, rs, in)
	var route *AlternativeResult
	for i := range out.Alternatives {
		if out.Alternatives[i].ID == "parts_route" {
			route = &out.Alternatives[i]
		}
	}
	if route == nil || route.Result != OutcomeUnknown {
		t.Fatalf("parts route with no listed shared part and an unconfirmed list = %+v, want unknown", route)
	}
	// 7318 shifts away from 8414, so alternative A is met; make A fail to see the status.
	in.Materials = nonOrig("841490")
	out = run(t, rs, in)
	if out.Status != RuleNeedsInformation || !hasFact(out, "materials_incomplete") {
		t.Errorf("status %s %v, want needs_information asking to confirm the list", out.Status, out.MissingFacts)
	}
	in.Profile.MaterialsComplete = true
	in.Materials = nonOrig("7318")
	for _, a := range run(t, rs, in).Alternatives {
		if a.ID == "parts_route" && a.Result != OutcomeNotSatisfied {
			t.Errorf("confirmed list, no shared part: %s, want not_satisfied", a.Result)
		}
	}
}
