package httpapi

import (
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/textilerules"
)

// TextileRuleView is the 19 CFR 102.21 check shown next to the model's
// answer (D951). It is computed when the case is read, from the stored
// product, and never stored or fed back: the prediction, triage and
// auto-accept are exactly what they would be without it.
type TextileRuleView struct {
	Rules  string              `json:"rules"`
	Result textilerules.Result `json:"result"`
	// Notes lists the operations that could not be read, so the check was
	// not told the whole chain.
	Notes       []string `json:"notes,omitempty"`
	ModelOrigin string   `json:"model_origin,omitempty"`
	// Agrees is set only when the check decided and the model answered.
	Agrees *bool `json:"agrees,omitempty"`
	// Review is true when the decided check and the model differ: worth a
	// person's look. It does not change the triage route.
	Review bool   `json:"review"`
	Note   string `json:"note"`
}

const textileRuleNote = "A deterministic check of the 19 CFR 102.21 textile rules from the HTS code and the operations given. It does not change the answer above or what is auto-accepted. Not a ruling."

// textileRuleCheck returns the view, or nil when the check is off, the
// product gives no HTS code, or the good is not a textile or apparel
// product under 102.21(b)(5).
func (a *api) textileRuleCheck(c store.Case) *TextileRuleView {
	if a.d.TextileRules == nil || c.Product == nil || strings.TrimSpace(c.Product.HTS) == "" {
		return nil
	}
	f, notes := textilerules.FromProduct(*c.Product)
	res := a.d.TextileRules.Evaluate(f)
	if res.Status == textilerules.OutOfScope {
		return nil
	}
	v := &TextileRuleView{Rules: "19 CFR 102.21", Result: res, Notes: notes, ModelOrigin: c.Prediction.Origin, Note: textileRuleNote}
	if res.Status == textilerules.Decided && c.Prediction.Origin != "" {
		agree := false
		for _, o := range domain.CountryParts(domain.NormalizeCountry(c.Prediction.Origin)) {
			if o == res.Origin {
				agree = true
			}
		}
		v.Agrees, v.Review = &agree, !agree
	}
	return v
}
