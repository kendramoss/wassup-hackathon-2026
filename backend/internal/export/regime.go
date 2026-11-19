package export

import "github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"

// regimeText is the wording that depends on the legal regime the prediction
// was actually made under. The general regime's values are the original
// literals, so general drafts do not change byte for byte.
type regimeText struct {
	// Markdown letter.
	mdPurpose  string // ends the opening request; "for purposes of ..."
	mdQuestion string // completes "What is the country of origin of X, produced as described above, ..."
	// 19 CFR 177.2 letter.
	rule         string // "determined under <rule>"
	cfrPurpose   string // follows "for purposes of country of origin marking (19 U.S.C. § 1304; 19 C.F.R. Part 134)"
	cfrQuestion  string // closes the question presented, before the "?"
	positionTail string // follows "is a product of <country>" in the position
	// separate is a paragraph that keeps Section 301 distinct; empty unless
	// the regime is a marking regime.
	separate string
	// note is the reviewer note naming the regime; empty for general.
	note string
}

const section301Separate = "This request concerns the origin of the merchandise for marking under the Part 102 rules only. It does not ask CBP to decide the country of origin for Section 301 duties, which is a separate question, and the requester does not contend that marking origin settles it."

var (
	regimeGeneral = regimeText{
		mdPurpose:   "for purposes of marking and the application of trade remedies, including Section 301 duties",
		mdQuestion:  "under the substantial-transformation test",
		rule:        "substantial transformation (19 C.F.R. § 134.1(b))",
		cfrPurpose:  " and the application of Section 301 duties",
		cfrQuestion: "for purposes of country of origin marking and Section 301 duties",
	}
	regimeTextile = regimeText{
		mdPurpose:   "for purposes of marking and the application of trade remedies, including Section 301 duties",
		mdQuestion:  "under the textile rules of origin, 19 C.F.R. § 102.21",
		rule:        "19 C.F.R. § 102.21",
		cfrPurpose:  " and the application of Section 301 duties",
		cfrQuestion: "for purposes of country of origin marking and Section 301 duties",
		note:        "Asked under 19 C.F.R. § 102.21 (textile and apparel rules) because the prediction behind the requester's position was made under that regime. Confirm that regime applies to this article.",
	}
	regimeUSMCA = regimeText{
		mdPurpose:    "for purposes of marking, determined under the Part 102 rules of origin (19 C.F.R. § 102.11, with the tariff shifts of § 102.20)",
		mdQuestion:   "under the Part 102 rules of origin, 19 C.F.R. § 102.11 with the tariff shifts of § 102.20, for purposes of marking",
		rule:         "the Part 102 rules of origin (19 C.F.R. § 102.11, with the tariff shifts of § 102.20)",
		cfrPurpose:   "",
		cfrQuestion:  "for purposes of country of origin marking",
		positionTail: ", for marking under the Part 102 rules",
		separate:     section301Separate,
		note:         "Asked under the Part 102 rules (19 C.F.R. § 102.11) for marking because the prediction behind the requester's position was made under that regime. Marking origin does not settle the separate Section 301 question; this draft does not ask it. Confirm that regime applies.",
	}
	regimePharma = regimeText{
		mdPurpose:   "for purposes of marking and the application of trade remedies, including Section 301 duties",
		mdQuestion:  "under the substantial-transformation test, as applied to a pharmaceutical product with attention to the country where its active pharmaceutical ingredient (API) is made",
		rule:        "substantial transformation (19 C.F.R. § 134.1(b)), as applied to a pharmaceutical product with attention to the country where its active pharmaceutical ingredient (API) is made",
		cfrPurpose:  " and the application of Section 301 duties",
		cfrQuestion: "for purposes of country of origin marking and Section 301 duties",
		note:        "Pharmaceutical (active ingredient) regime: the test is substantial transformation, with the prediction focused on where the active pharmaceutical ingredient is made. No separate pharmaceutical authority is cited. Confirm with counsel.",
	}
)

// EffectiveRegime is the regime a draft is written under: Regime when set,
// else router.RegimeTextile for the older Textile flag, else "" (general,
// substantial transformation). Regime wins over Textile, so a conflicting
// pair is resolved deterministically: Textile is only a compatibility
// shortcut for a caller that sets no Regime. "substantial_transformation",
// "unclear" and any unrecognised value are the general regime; the draft
// never invents wording for a regime this package does not know.
func (in Input) EffectiveRegime() string {
	r := in.Regime
	if r == "" && in.Textile {
		r = router.RegimeTextile
	}
	switch r {
	case router.RegimeTextile, router.RegimeUSMCA, router.RegimePharma:
		return r
	}
	return ""
}

func (in Input) regimeText() regimeText {
	switch in.EffectiveRegime() {
	case router.RegimeTextile:
		return regimeTextile
	case router.RegimeUSMCA:
		return regimeUSMCA
	case router.RegimePharma:
		return regimePharma
	}
	return regimeGeneral
}
