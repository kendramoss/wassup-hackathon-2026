package engine

import "strings"

// MissingFactVersion names the missing-fact instruction in the prompt version.
const MissingFactVersion = "missing-v1"

// UnresolvedConfidenceCap is the highest confidence an answer that names no
// single country may keep, below every auto-accept bar.
const UnresolvedConfidenceCap = 0.5

// MissingFactRule is appended to a specialist's system prompt (Options.Rules).
// It was written from the D970 development run: on cases whose decisive fact
// was not stated the pharmaceutical, textile and Part 102 engines answered at
// 0.92 to 0.98 confidence, and the pharmaceutical engine answered "Unknown".
const MissingFactRule = `Missing facts. If the facts do not state something this decision turns on (for example the country where the decisive step took place, where the active ingredient is made, a material's tariff code, or its value share when the de minimis rule could change the result, or which of several processes in different countries is the most important), do not settle the gap by assumption. Name the single most likely country, give a confidence below 0.5, and say in the reasoning which fact is missing. Give your normal confidence only when the stated facts decide the origin.`

// WithMissingFactRule returns o with MissingFactRule appended. With
// singleCountry, an answer that is not one country (unknown, a restated
// question, two countries) is also capped at UnresolvedConfidenceCap; leave it
// off for the Part 102 engine, whose rules (c) and (d) can name several.
func WithMissingFactRule(o Options, singleCountry bool) Options {
	if o.Rules != "" {
		o.Rules += "\n\n"
	}
	o.Rules += MissingFactRule
	o.RulesVersion = MissingFactVersion
	o.RequireSingleCountry = singleCountry
	return o
}

var unresolvedWords = []string{"unknown", "unclear", "undetermined", "indeterminate", "not stated", "not specified", "not given", "cannot", "unidentified",
	"n/a", "country where", "to be determined"}

// conjunctionCountries are single countries whose canonical English name
// contains " and ".
var conjunctionCountries = []string{"trinidad and tobago", "bosnia and herzegovina", "antigua and barbuda", "saint kitts and nevis", "st kitts and nevis",
	"st. kitts and nevis", "saint vincent and the grenadines", "st vincent and the grenadines", "sao tome and principe", "turks and caicos islands",
	"turks and caicos", "heard island and mcdonald islands", "saint pierre and miquelon", "wallis and futuna", "jan mayen and svalbard"}

// unresolvedOrigin reports an origin answer that is not one country.
func unresolvedOrigin(raw string) bool {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" || strings.HasPrefix(s, "country ") || strings.HasPrefix(s, "depends") {
		return true
	}
	for _, c := range conjunctionCountries {
		s = strings.ReplaceAll(s, c, "one")
	}
	if strings.ContainsAny(s, "/|") || strings.Contains(s, " or ") || strings.Contains(s, " and ") {
		return true
	}
	for _, w := range unresolvedWords {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// TextileVersion3 is textile-v1 plus the first alternative of the woven-fabric
// rows of Table 1, which textile-v1 left out: on the D970 development cases the
// engine named the weaving country for a greige fabric dyed and printed with
// two finishing operations elsewhere.
const TextileVersion3 = "textile-v3"

// TextileSystem3 is TextileSystem with the woven-fabric finishing clause.
var TextileSystem3 = strings.Replace(TextileSystem, "Minor processes (102.21(b)(6)", `For woven fabric of headings 5208-5212, 5407-5408 and 5512-5516, Table 1 first asks whether greige fabric was changed to finished fabric in one country by both dyeing and printing together with two or more finishing operations (bleaching, shrinking, fulling, napping, decating, permanent stiffening, weighting, permanent embossing, moireing): if so, that country is the origin. Only if not does the country where the fabric was formed (woven) decide.
Minor processes (102.21(b)(6)`, 1)
