package store

import "github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"

// RulingRefs lists the rulings a saved case relies on, for revocation alerts
// (D480): the ones the prediction cited, each shown plan's supporting
// ruling, CBP's holding on the current setup, then the rest of the rulings
// the prediction read.
func (c Case) RulingRefs() []domain.RulingRef {
	var refs []domain.RulingRef
	for _, id := range c.Prediction.CitedRulings {
		refs = append(refs, domain.RulingRef{ID: id, Role: domain.RoleCited})
	}
	if c.Plan != nil {
		for _, p := range c.Plan.Plans {
			if p.Verdict != nil && p.Verdict.SupportingRuling != "" {
				refs = append(refs, domain.RulingRef{ID: p.Verdict.SupportingRuling, Role: domain.RolePlanSupport})
			}
		}
	}
	if co := c.CurrentOrigin; co != nil && co.Ruling != "" {
		refs = append(refs, domain.RulingRef{ID: co.Ruling, Role: domain.RoleCurrentSetup})
	} else if c.Plan != nil && c.Plan.CurrentOrigin != nil && c.Plan.CurrentOrigin.Ruling != "" {
		refs = append(refs, domain.RulingRef{ID: c.Plan.CurrentOrigin.Ruling, Role: domain.RoleCurrentSetup})
	}
	for _, id := range c.Prediction.Precedents {
		refs = append(refs, domain.RulingRef{ID: id, Role: domain.RoleRead})
	}
	return refs
}

// RelevanceBasis is what each relied-on ruling's change is judged against
// (#626, D561): the predicted origin for cited rulings, each supporting
// ruling's plan target (the search's target, or the plan's own predicted
// origin when the search has none), and CBP's holding on the current setup
// for the current setup's ruling, from the same fields RulingRefs reads.
func (c Case) RelevanceBasis() domain.RelevanceBasis {
	b := domain.RelevanceBasis{Predicted: c.Prediction.Origin}
	if c.Plan != nil {
		for _, p := range c.Plan.Plans {
			if p.Verdict == nil || p.Verdict.SupportingRuling == "" {
				continue
			}
			t := c.Plan.Target
			if t == "" {
				t = p.Prediction.Origin
			}
			if b.PlanTargets == nil {
				b.PlanTargets = map[string][]string{}
			}
			b.PlanTargets[p.Verdict.SupportingRuling] = append(b.PlanTargets[p.Verdict.SupportingRuling], t)
		}
	}
	if co := c.CurrentOrigin; co != nil && co.Ruling != "" {
		b.CurrentSetup = co.Country
	} else if c.Plan != nil && c.Plan.CurrentOrigin != nil && c.Plan.CurrentOrigin.Ruling != "" {
		b.CurrentSetup = c.Plan.CurrentOrigin.Country
	}
	return b
}
