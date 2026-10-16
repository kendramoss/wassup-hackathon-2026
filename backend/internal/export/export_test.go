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
		"product of Thailand, based on these key factors: motor-fan assembly made in Thailand.",
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

func TestListPhrase(t *testing.T) {
	for in, want := range map[string]string{
		"Heating element|Electric motor-fan assembly|PCBA": "heating element, electric motor-fan assembly and PCBA",
		"Housing":           "housing",
		"Motor, wound|Fan.": "motor, wound; and fan",
	} {
		if got := listPhrase(strings.Split(in, "|")); got != want {
			t.Errorf("listPhrase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBelowBarNotice(t *testing.T) {
	md := Markdown(Input{ProductName: "x", Facts: "f", Requested: "Thailand", BelowBar: &BelowBar{Confidence: 0.92, Bar: 0.95}})
	if !strings.Contains(md, "**Below the confidence bar:** Origin Proof's precedent check rated this change 0.92, under its 0.95 bar.") {
		t.Errorf("draft = %s", md)
	}
	if strings.Contains(Markdown(Input{ProductName: "x", Facts: "f"}), "Below the confidence bar") {
		t.Error("notice shown without BelowBar")
	}
}

func TestReadRulingsWhenNoneCited(t *testing.T) {
	md := Markdown(Input{ProductName: "x", Facts: "f", Requested: "China", Prediction: domain.Prediction{Precedents: []string{"N340187", "N351782"}}})
	for _, want := range []string{"N340187 — read by Origin Proof, not cited; [CONFIRM RELEVANCE]", "https://rulings.cbp.gov/ruling/N351782"} {
		if !strings.Contains(md, want) {
			t.Errorf("draft missing %q", want)
		}
	}
	if strings.Contains(md, "[ADD RELEVANT EARLIER RULINGS]") {
		t.Error("placeholder shown although rulings were read")
	}
	if !strings.Contains(Markdown(Input{ProductName: "x", Facts: "f"}), "[ADD RELEVANT EARLIER RULINGS]") {
		t.Error("placeholder missing when nothing was read")
	}
}
