package export

import (
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

func TestMarkdown(t *testing.T) {
	md := Markdown(Input{
		ProductName: "heat gun", Facts: "Motor and PCBA made in Thailand.", Requested: "Thailand",
		PlanSummary: "Make Motor-fan assembly and PCBA (SMT) in Thailand", CurrentRuled: "China (N328954)",
		Prediction: domain.Prediction{KeyFactors: []string{"Motor-fan assembly made in Thailand"}, CitedRulings: []string{"N350001", "N347421"}},
		Supporting: "N347421", Counter: "N345859",
	})
	for _, want := range []string{
		domain.Disclaimer, "Draft for review by a licensed customs broker", "19 C.F.R. Part 177",
		"CBP previously ruled on the current production setup: origin China (N328954)", "make Motor-fan assembly and PCBA (SMT) in Thailand",
		"N347421 — a comparable change", "N345859 — a similar change that did not alter origin", "https://rulings.cbp.gov/ruling/N350001",
		"product of Thailand, because motor-fan assembly made in Thailand",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("draft missing %q", want)
		}
	}
	for _, banned := range []string{"guarantee", "approved", "verified", "cheapest", "proof"} {
		if strings.Contains(strings.ToLower(strings.ReplaceAll(md, "Origin Proof", "")), banned) {
			t.Errorf("draft contains banned word %q", banned)
		}
	}
	if strings.Count(md, "N347421 —") != 1 {
		t.Error("supporting ruling listed twice")
	}
}
