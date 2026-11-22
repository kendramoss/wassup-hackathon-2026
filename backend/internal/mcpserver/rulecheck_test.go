package mcpserver

import (
	"strings"
	"testing"
)

func TestRuleCheckLine(t *testing.T) {
	if ruleCheckLine(map[string]any{}) != "" {
		t.Fatal("no rule_check, no line")
	}
	got := ruleCheckLine(map[string]any{"rule_check": map[string]any{
		"applies": "part102", "review": true,
		"result": map[string]any{"status": "decided", "origin": "China", "basis": "19 CFR 102.11(b)(1)"},
	}})
	for _, want := range []string{"Rule check (19 CFR 102.20): China under 19 CFR 102.11(b)(1).", "differs from the model", "does not change the prediction"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q lacks %q", got, want)
		}
	}
	got = ruleCheckLine(map[string]any{"rule_check": map[string]any{
		"applies":    "part102",
		"result":     map[string]any{"status": "decided", "origin": "Canada", "basis": "19 CFR 102.11(a)(3)"},
		"robustness": map[string]any{"stable": true, "summary": "the same answer for every suggested code (2 combinations of the codes suggested for strip)"},
	}})
	if !strings.Contains(got, "Over the other suggested part codes: the same answer for every suggested code (2 combinations") {
		t.Errorf("%q", got)
	}
	got = ruleCheckLine(map[string]any{"rule_check": map[string]any{
		"applies": "informational",
		"result":  map[string]any{"status": "undetermined", "reason": "no materials given"},
	}})
	if !strings.Contains(got, "for information") || !strings.Contains(got, "undetermined: no materials given.") {
		t.Errorf("%q", got)
	}
}
