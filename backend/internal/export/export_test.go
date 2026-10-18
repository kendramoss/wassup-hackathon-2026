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
		if strings.Contains(strings.ToLower(strings.ReplaceAll(md, "Unde", "")), banned) {
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
	if !strings.Contains(md, "**Below the confidence bar:** Unde's precedent check rated this change 0.92, under its 0.95 bar.") {
		t.Errorf("draft = %s", md)
	}
	if strings.Contains(Markdown(Input{ProductName: "x", Facts: "f"}), "Below the confidence bar") {
		t.Error("notice shown without BelowBar")
	}
}

func TestReadRulingsWhenNoneCited(t *testing.T) {
	md := Markdown(Input{ProductName: "x", Facts: "f", Requested: "China", Prediction: domain.Prediction{Precedents: []string{"N340187", "N351782"}}})
	for _, want := range []string{"N340187 — read by Unde, not cited; [CONFIRM RELEVANCE]", "https://rulings.cbp.gov/ruling/N351782"} {
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

// TestCFR177Contents checks the letter covers each part of 19 CFR 177.2 and
// leaves a [FILL IN] for what the case does not know.
func TestCFR177Contents(t *testing.T) {
	md := CFR177(goldenInputs()["plan"])
	for _, want := range []string{
		domain.Disclaimer, "201 Varick Street, Suite 501", // 177.2(a): NCSD address
		"## 1. Interested parties", "Manufacturer, Rayong plant (Thailand): [FILL IN", // 177.2(b)(1)
		"## 2. Port of entry\n\n[FILL IN",
		"## 3. The transaction", "Prospective importation", "proposed change to that setup: make Motor-fan assembly and PCBA (SMT) in Thailand",
		"- switch: Malaysia (bought in).", "- wind the motor stator and assemble the motor-fan unit: Rayong plant, Thailand.", // eRulings origin checklist
		"HTSUS subheading, for reference (classification is not requested): 8419.89.95.",
		"Section 301 duties", "19 C.F.R. § 134.1(b)",
		"19 C.F.R. § 177.2(b)(6)", "N347421 — a comparable change", "https://rulings.cbp.gov/ruling/N350001",
		"not pending before the U.S. Court of International Trade", // 177.2(b)(5)
		"one of a series of similar and related transactions",
		"19 C.F.R. § 177.2(b)(7)", "10 working days", // confidentiality, 177.8(a)(3)
		"I am authorized to make this request", "[FILL IN: signature]", // 177.2(c)
	} {
		if !strings.Contains(md, want) {
			t.Errorf("letter missing %q", want)
		}
	}
	for _, banned := range []string{"guarantee", "approved", "verified", "cheapest", "proof"} {
		if strings.Contains(strings.ToLower(strings.ReplaceAll(md, "Unde", "")), banned) {
			t.Errorf("letter contains banned word %q", banned)
		}
	}
	// The Markdown draft's placeholders are filled by the Draft page's form;
	// the 177.2 letter uses [FILL IN: ...] only.
	for _, old := range []string{"[REQUESTER NAME]", "[NAME]", "[TITLE]", "[COMPANY]", "[ADDRESS]", "[CONTACT]"} {
		if strings.Contains(md, old) {
			t.Errorf("letter uses the markdown placeholder %s", old)
		}
	}
	if strings.Count(md, "N347421 —") != 1 {
		t.Error("supporting ruling listed twice")
	}
}

func TestCFR177FillsInWhatTheCaseLacks(t *testing.T) {
	md := CFR177(Input{Facts: "Brackets stamped in Vietnam."})
	for _, want := range []string{
		"country of origin of the subject merchandise", "Manufacturer(s): [FILL IN", "Name and model: [FILL IN", "Description: [FILL IN",
		"a product of [FILL IN: country]", "[ADD RELEVANT EARLIER RULINGS]", "Brackets stamped in Vietnam.",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("letter missing %q", want)
		}
	}
	for _, absent := range []string{"Components and where each is made", "CBP has ruled", "Below the confidence bar", "Suppliers of bought-in"} {
		if strings.Contains(md, absent) {
			t.Errorf("letter has %q without the facts for it", absent)
		}
	}
	if tx := CFR177(Input{Facts: "f", Textile: true}); !strings.Contains(tx, "determined under 19 C.F.R. § 102.21") || strings.Contains(tx, "134.1(b)") {
		t.Error("textile letter does not ask under 102.21")
	}
	if bb := CFR177(goldenInputs()["below_bar"]); !strings.Contains(bb, "rated this change 0.88, under its 0.92 bar") {
		t.Error("below-bar notice missing")
	}
}
