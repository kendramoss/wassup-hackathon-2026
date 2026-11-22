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
	u, _ := relevance(RulingAlert{Ruling: "N1", By: "H2", Kind: "revoked", Status: AlertFinal, HeldBefore: "Taiwan", HeldAfter: "Taiwan", Answer: "China"})
	if u != RelevanceHoldingUnchanged {
		t.Errorf("got %s", u)
	}
}
