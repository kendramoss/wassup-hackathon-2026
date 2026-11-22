package httpapi

import (
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

// RuleCheckView is the 19 CFR 102.20 rule check shown next to the model's
// answer (D363). It is computed when the case is read, from the stored
// product, and never stored or fed back: the prediction, triage and
// auto-accept are exactly what they would be without it.
type RuleCheckView struct {
	Rules string `json:"rules"`
	// Applies is "part102" when the router sent the case to the Part 102
	// rules, else "informational": the case is decided under substantial
	// transformation, and Part 102 is shown for comparison only.
	Applies string `json:"applies"`
	// Country is the country of production the check used, and
	// CountryFrom says how it was read from the product.
	Country     string             `json:"country"`
	CountryFrom string             `json:"country_from"`
	Result      tariffshift.Result `json:"result"`
	ModelOrigin string             `json:"model_origin,omitempty"`
	// Agrees is set only when the rule check decided and the model answered.
	Agrees *bool `json:"agrees,omitempty"`
	// Review is true when the decided rule check and the model differ: worth
	// a person's look. It does not change the triage route.
	Review bool   `json:"review"`
	Note   string `json:"note"`
	// Robustness, when a part carries other suggested codes
	// (components[].hts_alternatives), says whether the answer is the same
	// under each combination of them (D430). Display only, like the rest.
	Robustness *tariffshift.Robustness `json:"robustness,omitempty"`
}

const ruleCheckNote = "A deterministic check of the 19 CFR 102.20 tariff-shift rule from the HTS codes given. It does not change the answer above or what is auto-accepted. Not a ruling."

// ruleCheck returns the view, or nil when the check is off or the case does
// not give an HTS code for the good and at least one part.
func (a *api) ruleCheck(c store.Case) *RuleCheckView {
	if a.d.RuleCheck == nil || c.Product == nil || strings.TrimSpace(c.Product.HTS) == "" {
		return nil
	}
	anyCode := false
	for _, comp := range c.Product.Components {
		if strings.TrimSpace(comp.HTS) != "" {
			anyCode = true
			break
		}
	}
	if !anyCode {
		return nil
	}
	country, from := productionCountry(*c.Product)
	in := tariffshift.Input{GoodHTS: c.Product.HTS, Country: country}
	var alts [][]string
	for _, comp := range c.Product.Components {
		in.Materials = append(in.Materials, tariffshift.Material{Name: comp.Name, HTS: comp.HTS, Origin: comp.MadeIn})
		alts = append(alts, comp.HTSAlternatives)
	}
	v := &RuleCheckView{
		Rules: "19 CFR 102.20", Applies: "informational", Country: country, CountryFrom: from,
		Result: a.d.RuleCheck.Evaluate(in), ModelOrigin: c.Prediction.Origin, Note: ruleCheckNote,
		Robustness: a.d.RuleCheck.Robust(in, alts),
	}
	if c.Router != nil && c.Router.Regime == router.RegimeUSMCA {
		v.Applies = "part102"
	}
	if v.Result.Status == tariffshift.Decided && c.Prediction.Origin != "" {
		agree := false
		for _, o := range domain.CountryParts(domain.NormalizeCountry(c.Prediction.Origin)) {
			if o == v.Result.Origin {
				agree = true
			}
		}
		v.Agrees, v.Review = &agree, !agree
	}
	return v
}

// productionCountry is where the good is produced: the plant of the
// final-assembly group, else the plant of the last operation, else the only
// plant. "" when none of those is known.
func productionCountry(p domain.Product) (string, string) {
	for i := len(p.Operations) - 1; i >= 0; i-- {
		o := p.Operations[i]
		if o.Group == "final_assembly" {
			if pl, ok := p.PlantByID(o.Plant); ok && pl.Country != "" {
				return pl.Country, "final assembly plant"
			}
		}
	}
	if n := len(p.Operations); n > 0 {
		if pl, ok := p.PlantByID(p.Operations[n-1].Plant); ok && pl.Country != "" {
			return pl.Country, "plant of the last operation"
		}
	}
	if len(p.Plants) == 1 && p.Plants[0].Country != "" {
		return p.Plants[0].Country, "the only plant"
	}
	return "", "not known: add a plant and its operations"
}
