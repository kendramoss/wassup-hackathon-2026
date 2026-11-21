package corpus

import "regexp"

// Regime names used in labels.jsonl.
const (
	RegimeST      = "substantial_transformation"
	RegimeSingle  = "single_country"
	RegimePart102 = "part102_tariff_shift"
	RegimePharma  = "pharma_api"
	Regime9802    = "9802_us_components"
	RegimeOther   = "other"
)

var (
	regimePart102Cite = regexp.MustCompile(`102\.(?:20|11)\b|102\.21\s*\(`)
	regimeTariffShift = regexp.MustCompile(`(?i)tariff shift|change in tariff classification`)
	regimeNCU         = regexp.MustCompile(`(?i)name, character,? (?:and|or) use`)
	regime9802        = regexp.MustCompile(`9802`)
	regimePharma      = regexp.MustCompile(`(?i)active pharmaceutical ingredient|\bAPI\b|pharmaceutical|medicament|drug substance`)
	regimeSubstantial = regexp.MustCompile(`(?i)substantial(?:ly)? transform`)
)

// ClassifyRegime assigns a ruling's origin regime from its full text and
// whether its facts name two or more countries (Label.MultiCountry). The
// labeller that produced the regime column of labels.jsonl is not in this
// repository; this rule is a reconstruction, written for the second held-out
// split (D150), and checked against labels.jsonl by `corpus label-check`.
// Rules, first match wins:
//
//   - Part 102: cites 19 CFR 102.11, 102.20 or a 102.21 paragraph, or speaks
//     of a tariff shift without the name/character/use test;
//   - 9802: mentions heading 9802 (U.S. components assembled abroad);
//   - pharmaceutical: an active ingredient or a pharmaceutical product;
//   - other: no substantial-transformation analysis;
//   - substantial transformation when the facts name two or more countries,
//     single country otherwise.
func ClassifyRegime(text string, multiCountry bool) string {
	switch {
	case regimePart102Cite.MatchString(text) || (regimeTariffShift.MatchString(text) && !regimeNCU.MatchString(text)):
		return RegimePart102
	case regime9802.MatchString(text):
		return Regime9802
	case regimePharma.MatchString(text):
		return RegimePharma
	case !regimeSubstantial.MatchString(text):
		return RegimeOther
	case multiCountry:
		return RegimeST
	}
	return RegimeSingle
}

// LabelRuling labels a ruling of any regime: the holding as LabelHolding
// finds it, and the regime from ClassifyRegime.
func LabelRuling(id, rulingDate, collection, subject, text string, meta SplitMeta) Label {
	l := LabelHolding(id, rulingDate, collection, subject, text, meta)
	l.Regime = ClassifyRegime(text, l.MultiCountry)
	return l
}
