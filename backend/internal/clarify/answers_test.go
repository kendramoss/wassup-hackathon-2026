package clarify

import (
	"strings"
	"testing"
)

func TestAppendAnswersDropsHedgesAndUnknowns(t *testing.T) {
	out, ok := AppendAnswers("Facts.\n", []Answer{
		{Question: "Is it pre-blended?", Answer: "We blend everything in Australia. Whether any ingredient is pre-blended is not explicitly stated."},
		{Question: "Resin source?", Answer: "Unknown"},
		{Question: "Value share?", Answer: "Not stated."},
	})
	if !ok || !strings.Contains(out, "A: We blend everything in Australia.\n") || strings.Contains(out, "not stated") ||
		strings.Contains(out, "Resin source") || strings.Contains(out, "Value share") || !strings.Contains(out, "not evidence for or against") {
		t.Errorf("AppendAnswers =\n%s", out)
	}
	if out, ok := AppendAnswers("Facts.\n", []Answer{{Question: "q", Answer: "I don't know."}}); ok || out != "Facts.\n" {
		t.Errorf("all-unknown answers changed facts: %q %v", out, ok)
	}
}
