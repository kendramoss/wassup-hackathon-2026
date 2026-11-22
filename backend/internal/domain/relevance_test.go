package domain

import (
	"strings"
	"testing"
	"time"
)

// D531: N344694 (China) was modified by H344999 (Taiwan). The label is the
// same whether the case's answer is the old holding, the new one, or neither.
func TestWithRelevanceComparesHeldOriginsWithTheAnswer(t *testing.T) {
	if HeldOrigin("N344694") != "China" || HeldOrigin("H344999") != "Taiwan" {
		t.Fatalf("holdings: %q %q", HeldOrigin("N344694"), HeldOrigin("H344999"))
	}
	day := func(s string) time.Time { d, _ := time.Parse(time.DateOnly, s); return d }
	alert := func(answer string) RulingAlert {
		as := RulingAlerts(day("2025-12-22"), day("2026-11-22"), []RulingRef{{ID: "N344694", Role: RoleCited}})
		if len(as) != 1 {
			t.Fatalf("want one alert, got %+v", as)
		}
		return WithRelevance(as, answer)[0]
	}
	for _, tc := range []struct{ answer, want, words string }{
		{"Vietnam", RelevanceNotAnswer, "from China to Taiwan. This case's answer was Vietnam, so the change does not touch the answer directly."},
		{"china", RelevanceTouchesAnswer, "This case's answer was China, so the change bears on the answer."},
		{"Taiwan", RelevanceTouchesAnswer, "This case's answer was Taiwan, the new holding, so the change bears on the answer."},
		{"", RelevanceUnknown, "This case has no answer to compare."},
	} {
		a := alert(tc.answer)
		if a.Relevance != tc.want || !strings.Contains(a.RelevanceNote, tc.words) {
			t.Errorf("answer %q: got %s %q", tc.answer, a.Relevance, a.RelevanceNote)
		}
		if a.HeldBefore != "China" || a.HeldAfter != "Taiwan" {
			t.Errorf("answer %q: held %q -> %q", tc.answer, a.HeldBefore, a.HeldAfter)
		}
	}
	if n := CountAlerts([]RulingAlert{alert("China")}); n.Final != 1 || n.TouchesAnswer != 1 {
		t.Errorf("counts %+v", n)
	}
	if n := CountAlerts([]RulingAlert{alert("Vietnam")}); n.TouchesAnswer != 0 {
		t.Errorf("counts %+v", n)
	}
}

func TestRelevanceUnknownAndUnchanged(t *testing.T) {
	// N322364 has no labelled origin; H323218 holds Vietnam.
	a := WithRelevance([]RulingAlert{{Ruling: "N322364", By: "H323218", Kind: "modified", Status: AlertFinal}}, "Vietnam")[0]
	if a.Relevance != RelevanceUnknown || !strings.Contains(a.RelevanceNote, "H323218 holds Vietnam, but the origin N322364 held is not in the labels.") {
		t.Errorf("%s %q", a.Relevance, a.RelevanceNote)
	}
	// H325607's label was read from its account of N331974 ("In NY
	// N331974, CBP found ..."), so its own holding is not known.
	if HeldOrigin("H325607") != "" {
		t.Errorf("H325607: a recital must not count as its holding, got %q", HeldOrigin("H325607"))
	}
	u, _ := relevance(RulingAlert{Ruling: "N1", By: "H2", Kind: "revoked", Status: AlertFinal, HeldBefore: "Taiwan", HeldAfter: "Taiwan", Answer: "China"}, judgement{RoleCited, []string{"China"}})
	if u != RelevanceHoldingUnchanged {
		t.Errorf("got %s", u)
	}
}

// #626 (D561): each role is judged against what it relies on the ruling
// for; a ruling with several roles keeps the most relevant label and names
// the role.
func TestWithRelevanceByJudgesEachRole(t *testing.T) {
	day := func(s string) time.Time { d, _ := time.Parse(time.DateOnly, s); return d }
	alert := func(b RelevanceBasis, roles ...string) RulingAlert {
		var refs []RulingRef
		for _, r := range roles {
			refs = append(refs, RulingRef{ID: "N344694", Role: r})
		}
		as := RulingAlerts(day("2025-12-22"), day("2026-11-22"), refs)
		if len(as) != 1 {
			t.Fatalf("want one alert, got %+v", as)
		}
		return WithRelevanceBy(as, b)[0]
	}
	china := map[string][]string{"N344694": {"China"}}
	for _, tc := range []struct {
		name        string
		basis       RelevanceBasis
		roles       []string
		want, role  string
		words       string
		touchesSeen int
	}{
		{"plan to the old holding", RelevanceBasis{Predicted: "Vietnam", PlanTargets: china}, []string{RolePlanSupport}, RelevanceTouchesAnswer, RolePlanSupport, "The plan to China rests on it, so the change bears on the plan: the ruling no longer holds China.", 1},
		{"plan to the new holding", RelevanceBasis{Predicted: "Vietnam", PlanTargets: map[string][]string{"N344694": {"Taiwan"}}}, []string{RolePlanSupport}, RelevanceTouchesAnswer, RolePlanSupport, "The plan to Taiwan rests on it, and Taiwan is the new holding", 1},
		{"plan elsewhere", RelevanceBasis{Predicted: "China", PlanTargets: map[string][]string{"N344694": {"Mexico", "Canada"}}}, []string{RolePlanSupport}, RelevanceNotAnswer, RolePlanSupport, "The plans to Mexico and Canada rest on it, so the change does not touch the plan directly.", 0},
		{"plan target not known", RelevanceBasis{Predicted: "China"}, []string{RolePlanSupport}, RelevanceUnknown, RolePlanSupport, "its target is not known", 0},
		{"current setup held China", RelevanceBasis{Predicted: "Vietnam", CurrentSetup: "china"}, []string{RoleCurrentSetup}, RelevanceTouchesAnswer, RoleCurrentSetup, "CBP's holding on the current setup (China), so the change bears on the current setup.", 1},
		{"current setup elsewhere", RelevanceBasis{Predicted: "China", CurrentSetup: "Mexico"}, []string{RoleCurrentSetup}, RelevanceNotAnswer, RoleCurrentSetup, "given as Mexico, so the change does not touch the current setup directly.", 0},
		{"cited and plan: the plan bears", RelevanceBasis{Predicted: "Vietnam", PlanTargets: china}, []string{RoleCited, RolePlanSupport}, RelevanceTouchesAnswer, RolePlanSupport, "The plan to China rests on it", 1},
		{"cited bears, plan does not", RelevanceBasis{Predicted: "China", PlanTargets: map[string][]string{"N344694": {"Mexico"}}}, []string{RoleCited, RolePlanSupport}, RelevanceTouchesAnswer, RoleCited, "This case's answer was China", 1},
		{"not known beats not touching", RelevanceBasis{Predicted: "Vietnam"}, []string{RoleCited, RolePlanSupport}, RelevanceUnknown, RolePlanSupport, "its target is not known", 0},
		{"read only: the answer", RelevanceBasis{Predicted: "Vietnam", PlanTargets: china}, []string{RoleRead}, RelevanceNotAnswer, RoleRead, "This case's answer was Vietnam", 0},
	} {
		a := alert(tc.basis, tc.roles...)
		if a.Relevance != tc.want || a.RelevanceRole != tc.role || !strings.Contains(a.RelevanceNote, tc.words) {
			t.Errorf("%s: got %s for %s, %q", tc.name, a.Relevance, a.RelevanceRole, a.RelevanceNote)
		}
		if a.Answer != NormalizeCountry(tc.basis.Predicted) {
			t.Errorf("%s: answer %q", tc.name, a.Answer)
		}
		if n := CountAlerts([]RulingAlert{a}); a.Relied() && n.TouchesAnswer != tc.touchesSeen {
			t.Errorf("%s: counts %+v", tc.name, n)
		}
	}
	// WithRelevance is the cited-only basis: unchanged from D531.
	plain := WithRelevance(RulingAlerts(day("2025-12-22"), day("2026-11-22"), []RulingRef{{ID: "N344694", Role: RolePlanSupport}}), "China")[0]
	if plain.Relevance != RelevanceUnknown {
		t.Errorf("WithRelevance on a plan's ruling with no target: %s %q", plain.Relevance, plain.RelevanceNote)
	}
}
