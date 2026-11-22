package mcpserver

import (
	"strings"
	"testing"
)

// A plan line names the move's 102.20 rule check and its flag (D441).
func TestPlanLineNamesTheRuleCheck(t *testing.T) {
	p := map[string]any{"description": "Make PCBA in Thailand", "verdict": map[string]any{"supporting_ruling": "N350727", "confidence": 0.95},
		"rule_check": map[string]any{"label": "102.20 gives China: finished there", "review": true}}
	got := planLine(p)
	for _, want := range []string{"supported by N350727", "102.20 rule check: 102.20 gives China: finished there", "worth a broker's look"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q lacks %q", got, want)
		}
	}
	delete(p, "rule_check")
	if strings.Contains(planLine(p), "102.20") {
		t.Error("no rule_check, no words")
	}
}

// On a Part 102 case the plan and comparison texts say the rule check
// orders shown plans (D500); otherwise that it is for information.
func TestPlanRuleNote(t *testing.T) {
	if n := planRuleNote(true); !strings.Contains(n, "moves it says shift first") || !strings.Contains(n, "never changes which plans are shown") {
		t.Error(n)
	}
	if n := planRuleNote(false); !strings.Contains(n, "display only") || !strings.Contains(n, "for information") {
		t.Error(n)
	}
}

// #636: the D591 safety flag is an engine-vs-rule disagreement on a row no
// precedent check ran on. The plan line says so, from review_note, and never
// calls it a disagreement with the precedent check.
func TestPlanLineWordsTheEngineFlagFromItsNote(t *testing.T) {
	for name, c := range map[string]struct {
		note string
		want string
	}{
		"current setup": {"The engine predicts China for the current setup, but the 102.20 rule gives Vietnam: worth a broker's look.",
			"(the engine predicts China for the current setup, but the 102.20 rule gives Vietnam: worth a broker's look)"},
		"engine_other_origin move": {"The 102.20 rule says this move reaches Vietnam, but the engine predicts China, so the move was not checked against precedent: worth a broker's look.",
			"(the 102.20 rule says this move reaches Vietnam, but the engine predicts China, so the move was not checked against precedent"},
	} {
		t.Run(name, func(t *testing.T) {
			p := map[string]any{"description": "Move the PCBA to Vietnam", "rejection_kind": "engine_other_origin",
				"rule_check": map[string]any{"label": "102.20 gives Vietnam", "review": true, "review_note": c.note}}
			got := planLine(p)
			if strings.Contains(got, "precedent check:") || strings.Contains(got, "differs from the precedent") {
				t.Errorf("engine flag worded as a precedent disagreement: %q", got)
			}
			if !strings.Contains(got, c.want) {
				t.Errorf("%q lacks %q", got, c.want)
			}
		})
	}
	// A flag with no note and no precedent verdict is not called a precedent disagreement.
	got := planLine(map[string]any{"description": "No change", "rule_check": map[string]any{"label": "102.20 gives Vietnam", "review": true}})
	if strings.Contains(got, "precedent") || !strings.Contains(got, "worth a broker's look") {
		t.Errorf("no note, no verdict: %q", got)
	}
	// The D441 note keeps its precedent wording.
	got = planLine(map[string]any{"description": "Make PCBA in Thailand", "verdict": map[string]any{"supporting_ruling": "N350727", "confidence": 0.95},
		"rule_check": map[string]any{"label": "102.20 gives China", "review": true,
			"review_note": "An earlier ruling supports this move, but the 102.20 rule gives China: worth a broker's look."}})
	if !strings.Contains(got, "(an earlier ruling supports this move, but the 102.20 rule gives China: worth a broker's look)") {
		t.Errorf("D441 note: %q", got)
	}
}
