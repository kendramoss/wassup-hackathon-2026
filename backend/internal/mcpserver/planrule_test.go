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
