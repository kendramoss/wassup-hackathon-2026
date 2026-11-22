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
