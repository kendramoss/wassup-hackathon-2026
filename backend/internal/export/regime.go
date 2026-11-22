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

// part102Rule names the Part 102 hierarchy as 19 CFR 102.11 sets it out:
// paragraph (a), whose (a)(3) is the tariff shift of § 102.20, then (b) to
// (d), each only where the one before cannot decide (D520).
const part102Rule = "the Part 102 rules of origin (the general rules of 19 C.F.R. § 102.11, applied in order, with the tariff shifts of § 102.20)"

// part102Marking is the purpose: § 102.0 makes §§ 102.1 to 102.18 and
// 102.20 the marking rules for goods imported under the USMCA.
const part102Marking = "for purposes of country of origin marking (19 C.F.R. § 102.0)"

// part102RoutedNote sits beside a draft asked under Part 102 whose position
// came from the general engine: a plan stored before D540, or a Part 102
// engine that was off or did not answer.
const part102RoutedNote = "Legal framework not confirmed: this case is routed to the Part 102 marking rules (19 CFR 102.11), so this draft asks the Part 102 marking question, but the prediction behind the requester's position was made by Unde's general engine, which reads earlier rulings under substantial transformation, not by the Part 102 engine. (A plan search on a Part 102 case predicts with the Part 102 engine when it is on, so this is a plan searched before that change, or the Part 102 engine was off or unavailable.) The requester's position is not a Part 102 determination: treat it as provisional and check it against the tariff-shift paragraph and the rules. Marking origin does not settle the separate Section 301 question; this draft does not ask it. The broker or counsel must select the appropriate framework before filing. This is a draft aid, not validated legal advice."

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
		mdPurpose:    "for purposes of marking, determined under " + part102Rule,
		mdQuestion:   "under " + part102Rule + ", " + part102Marking,
		rule:         part102Rule,
		cfrPurpose:   "",
		cfrQuestion:  part102Marking,
		positionTail: ", for marking under the Part 102 rules",
		separate:     section301Separate,
		note:         "Asked under the Part 102 rules (19 C.F.R. § 102.11) for marking because the prediction behind the requester's position was made under that regime. Marking origin does not settle the separate Section 301 question; this draft does not ask it. Confirm that regime applies.",
	}
	// regimePart102Routed is a case the router sent to the Part 102 marking
	// rules whose prediction came from the general engine (a plan searched
	// before D540, or a Part 102 engine that was off or did not answer). The question and framing
	// are the Part 102 ones, as the case is routed there; the note says the
	// position is the general engine's, not a Part 102 determination (D520).
	regimePart102Routed = func() regimeText {
		r := regimeUSMCA
		r.note = part102RoutedNote
		return r
	}()
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
	switch r := in.EffectiveRegime(); {
	case r == "" && in.Part102:
		return regimePart102Routed
	case r == router.RegimeTextile:
		return regimeTextile
	case r == router.RegimeUSMCA:
		return regimeUSMCA
	case r == router.RegimePharma:
		return regimePharma
	}
	return regimeGeneral
}
