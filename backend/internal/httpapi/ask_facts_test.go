package httpapi

import (
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// get_case text never stuffs the whole facts, never truncates silently, and
// counts runes.
func TestAgentCaseTextFlagsFactsBeyondHead(t *testing.T) {
	facts := strings.Repeat("é", 6000) + "TAIL-FACT"
	txt := agentCaseText(store.Case{ID: "c1", Date: "2026-01-14", Facts: facts})
	if strings.Contains(txt, "TAIL-FACT") || !strings.Contains(txt, "INCOMPLETE") || !strings.Contains(txt, "of 6009") || !strings.Contains(txt, "get_case_facts") {
		t.Errorf("long facts not flagged:\n%.300s", txt)
	}
	short := agentCaseText(store.Case{ID: "c1", Date: "2026-01-14", Facts: strings.Repeat("é", 6000)})
	if strings.Contains(short, "INCOMPLETE") || !strings.Contains(short, "complete") {
		t.Errorf("6000-rune facts flagged as truncated")
	}
}
