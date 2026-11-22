package export

import (
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
)

func regimeInput(regime string) Input {
	return Input{
		ProductName: "widget", Regime: regime, Facts: "Made in Mexico from Chinese parts.",
		Requested:  "Mexico",
		Prediction: domain.Prediction{KeyFactors: []string{"Final production in Mexico"}, CitedRulings: []string{"N300001"}},
		BelowBar:   &BelowBar{Confidence: 0.5, Bar: 0.9},
	}
}

var bothFormats = map[string]func(Input) string{"markdown": Markdown, "cfr177": CFR177}

// TestRegimeByFormat is the regime x format matrix: what each draft must ask
// and must not say.
func TestRegimeByFormat(t *testing.T) {
	cases := []struct {
		regime string
		want   []string
		absent []string
	}{
		{"", []string{"substantial"}, []string{"102.21", "102.11", "Part 102", "API"}},
		{"substantial_transformation", []string{"substantial"}, []string{"102.21", "102.11", "Part 102"}},
		{"unclear", []string{"substantial"}, []string{"102.21", "102.11", "Part 102"}},
		{"some_future_regime", []string{"substantial"}, []string{"102.21", "102.11", "Part 102"}},
		{router.RegimeTextile, []string{"19 C.F.R. § 102.21"}, []string{"substantial transformation", "substantial-transformation", "134.1(b)", "102.11", "Part 102 rules", "API"}},
		{router.RegimeUSMCA, []string{"19 C.F.R. § 102.11", "§ 102.20", "Part 102", "separate question"}, []string{"substantial transformation", "substantial-transformation", "134.1(b)", "102.21", "API"}},
		{router.RegimePharma, []string{"substantial", "active pharmaceutical ingredient (API)"}, []string{"102.21", "102.11", "Part 102", "tariff shift", "essential character"}},
	}
	for _, c := range cases {
		for format, render := range bothFormats {
			got := render(regimeInput(c.regime))
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("%s regime %q: draft missing %q", format, c.regime, w)
				}
			}
			for _, a := range c.absent {
				if strings.Contains(got, a) {
					t.Errorf("%s regime %q: draft has %q", format, c.regime, a)
				}
			}
			if got != render(regimeInput(c.regime)) {
				t.Errorf("%s regime %q not deterministic", format, c.regime)
			}
		}
	}
}

func TestTextileAsksQuestionUnder10221InBothFormats(t *testing.T) {
	md := Markdown(regimeInput(router.RegimeTextile))
	if !strings.Contains(md, "## Issue\n\nWhat is the country of origin of widget, produced as described above, under the textile rules of origin, 19 C.F.R. § 102.21?") {
		t.Errorf("markdown issue:\n%s", md)
	}
	cfr := CFR177(regimeInput(router.RegimeTextile))
	if !strings.Contains(cfr, "determined under 19 C.F.R. § 102.21") || !strings.Contains(cfr, "produced as described above, under 19 C.F.R. § 102.21, for purposes of country of origin marking and Section 301 duties?") {
		t.Errorf("cfr177 letter:\n%s", cfr)
	}
}

// Marking under Part 102 is kept apart from Section 301.
func TestPart102DoesNotSettleSection301(t *testing.T) {
	for format, render := range bothFormats {
		got := render(regimeInput(router.RegimeUSMCA))
		if strings.Contains(got, "Section 301 duties?") || strings.Contains(got, "application of Section 301 duties") ||
			strings.Contains(got, "including Section 301 duties") || strings.Contains(got, "trade remedies") {
			t.Errorf("%s Part 102 draft asks CBP for a Section 301 origin:\n%s", format, got)
		}
		for _, w := range []string{"It does not ask CBP to decide the country of origin for Section 301 duties", "does not contend that marking origin settles it", "102.11"} {
			if !strings.Contains(got, w) {
				t.Errorf("%s Part 102 draft missing %q", format, w)
			}
		}
	}
	if !strings.Contains(CFR177(regimeInput(router.RegimeUSMCA)), "for purposes of country of origin marking (19 U.S.C. § 1304; 19 C.F.R. Part 134). ") {
		t.Error("cfr177 Part 102 opening should be marking only")
	}
}

// The Part 102 and textile tests are never borrowed by the pharma draft, and
// the pharma draft cites no separate authority.
func TestPharmaWordingBorrowsNoOtherRegime(t *testing.T) {
	for format, render := range bothFormats {
		got := render(regimeInput(router.RegimePharma))
		for _, a := range []string{"102.", "Part 102", "tariff shift", "HQ 563207", "fixed-combination"} {
			if strings.Contains(got, a) {
				t.Errorf("%s pharma draft has %q", format, a)
			}
		}
	}
}

// Reviewer notes name the regime but never enter the printable letter.
func TestRegimeNotesStayOutsideLetter(t *testing.T) {
	for _, regime := range []string{router.RegimeTextile, router.RegimeUSMCA, router.RegimePharma} {
		in := regimeInput(regime)
		notes := strings.Join(Notes(in, true), "\n")
		for _, w := range []string{"Draft for review by a licensed customs broker", domain.Disclaimer, "Below the confidence bar", "prediction behind the requester's position was made under that regime", "Confirm"} {
			if regime == router.RegimePharma && w == "prediction behind the requester's position was made under that regime" {
				w = "Pharmaceutical (active ingredient) regime"
			}
			if !strings.Contains(notes, w) {
				t.Errorf("%s notes missing %q", regime, w)
			}
		}
		for format, render := range bothFormats {
			letter := render(in)
			for _, n := range Notes(in, true) {
				if strings.Contains(letter, n) {
					t.Errorf("%s %s: note is in the letter: %q", regime, format, n)
				}
			}
			for _, w := range []string{"Draft for review", "Below the confidence bar", "Confirm that regime", "Confirm with counsel", domain.Disclaimer} {
				if strings.Contains(letter, w) {
					t.Errorf("%s %s: letter contains reviewer text %q", regime, format, w)
				}
			}
		}
	}
	for _, n := range Notes(regimeInput(""), true) {
		if strings.Contains(n, "regime") {
			t.Errorf("general notes mention a regime: %q", n)
		}
	}
}

// General drafts are unchanged for every empty, unknown or general value.
func TestGeneralRegimeValuesRenderAlike(t *testing.T) {
	base := regimeInput("")
	for _, r := range []string{"substantial_transformation", "unclear", "nonsense"} {
		in := regimeInput(r)
		for format, render := range bothFormats {
			if render(in) != render(base) {
				t.Errorf("%s regime %q differs from the empty regime", format, r)
			}
		}
		if strings.Join(Notes(in, true), "|") != strings.Join(Notes(base, true), "|") {
			t.Errorf("notes for %q differ from general", r)
		}
	}
}

// Textile bool compatibility: it still means textile when Regime is empty;
// an explicit Regime always wins, including a general one.
func TestTextileBoolCompatibility(t *testing.T) {
	legacy := regimeInput("")
	legacy.Textile = true
	explicit := regimeInput(router.RegimeTextile)
	for format, render := range bothFormats {
		if render(legacy) != render(explicit) {
			t.Errorf("%s: legacy Textile=true differs from Regime textile", format)
		}
	}
	for _, r := range []string{router.RegimeUSMCA, router.RegimePharma} {
		conflict := regimeInput(r)
		conflict.Textile = true
		if conflict.EffectiveRegime() != r {
			t.Errorf("Regime %q must win over Textile, got %q", r, conflict.EffectiveRegime())
		}
		for format, render := range bothFormats {
			if render(conflict) != render(regimeInput(r)) {
				t.Errorf("%s: Textile bool altered an explicit %q draft", format, r)
			}
		}
	}
	// An explicit general regime also wins over the legacy flag.
	g := regimeInput("substantial_transformation")
	g.Textile = true
	for format, render := range bothFormats {
		if render(g) != render(regimeInput("")) {
			t.Errorf("%s: Textile=true overrode explicit substantial_transformation", format)
		}
	}
}

// A case routed to Part 102 whose prediction is the general engine's (every
// plan search) asks the Part 102 marking question in both formats, with the
// tariff-shift check as a Part 102 paragraph, and a note that the position
// is not a Part 102 determination (D520).
func TestPart102RoutedGeneralAsksThePart102Question(t *testing.T) {
	in := regimeInput("")
	in.Part102 = true
	in.RuleCheck = &domain.PlanRuleCheck{Applies: "part102", Country: "Mexico", Status: "shifts", Origin: "Mexico", Basis: "19 CFR 102.11(a)(3)"}
	md, cfr := Markdown(in), CFR177(in)
	for format, got := range map[string]string{"markdown": md, "cfr177": cfr} {
		for _, w := range []string{
			"under the Part 102 rules of origin (the general rules of 19 C.F.R. § 102.11, applied in order, with the tariff shifts of § 102.20)",
			"for purposes of country of origin marking (19 C.F.R. § 102.0)?",
			"It does not ask CBP to decide the country of origin for Section 301 duties",
			"product of Mexico, for marking under the Part 102 rules",
			"Tariff-shift check (19 C.F.R. § 102.20): applied to the component codes in the bill of materials, with the good finished in Mexico, the rule gives Mexico (19 C.F.R. § 102.11(a)(3)).",
		} {
			if !strings.Contains(got, w) {
				t.Errorf("%s routed Part 102 draft missing %q:\n%s", format, w, got)
			}
		}
		for _, a := range []string{"substantial transformation", "substantial-transformation", "134.1(b)", "Section 301 duties?", "for reference only"} {
			if strings.Contains(got, a) {
				t.Errorf("%s routed Part 102 draft has %q", format, a)
			}
		}
	}
	// The letter is the Part 102 letter; only the note differs.
	spec := in
	spec.Part102, spec.Regime = false, router.RegimeUSMCA
	if md != Markdown(spec) || cfr != CFR177(spec) {
		t.Error("routed Part 102 letter differs from the Part 102 engine's letter")
	}
	notes := strings.Join(Notes(in, true), "\n")
	for _, w := range []string{"routed to the Part 102 marking rules", "made by Unde's general engine", "is not a Part 102 determination", "not validated legal advice"} {
		if !strings.Contains(notes, w) {
			t.Errorf("notes missing %q: %s", w, notes)
		}
	}
	if strings.Contains(notes, "because the prediction behind the requester's position was made under that regime") {
		t.Error("routed note claims the Part 102 engine made the prediction")
	}
}

// Part102 changes nothing when a specialist made the prediction or the case
// is not routed there: textile and pharma keep their wording, and a
// substantial-transformation case keeps the substantial-transformation
// question.
func TestPart102FlagOnlyForGeneralPredictions(t *testing.T) {
	for _, r := range []string{router.RegimeTextile, router.RegimePharma, router.RegimeUSMCA} {
		in := regimeInput(r)
		with := in
		with.Part102 = true
		if Markdown(in) != Markdown(with) || CFR177(in) != CFR177(with) || strings.Join(Notes(in, true), "|") != strings.Join(Notes(with, true), "|") {
			t.Errorf("Part102 changed the %s draft", r)
		}
	}
	st := regimeInput("")
	for format, got := range map[string]string{"markdown": Markdown(st), "cfr177": CFR177(st)} {
		if !strings.Contains(got, "substantial") || strings.Contains(got, "Part 102") || strings.Contains(got, "102.0") {
			t.Errorf("%s substantial-transformation draft changed:\n%s", format, got)
		}
	}
}
