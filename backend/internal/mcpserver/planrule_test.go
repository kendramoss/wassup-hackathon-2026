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
