package httpapi

import (
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planrule"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// The 102.20 rule check on each plan move (D441) is added when a plan is
// read or returned, from the case's product, and is never stored: the
// stored search, its verdicts, what is shown and the ranking are exactly
// what they would be without it. RULE_CHECK=off removes it with the case's
// own check.

func part102Case(c store.Case) bool {
	return c.Router != nil && c.Router.Regime == router.RegimeUSMCA
}

// planRuleOn says whether plans of c carry the per-move check.
func (a *api) planRuleOn(c store.Case) bool {
	return a.d.RuleCheck != nil && planrule.Eligible(c.Product, part102Case(c))
}

// ruleChecked is res with each plan's rule check (a copy).
func (a *api) ruleChecked(c store.Case, res planner.Result) planner.Result {
	if !a.planRuleOn(c) {
		return res
	}
	return planrule.Annotate(a.d.RuleCheck, *c.Product, res, part102Case(c))
}

// ruleCheckedPlan is ruleChecked for one streamed plan (a copy).
func (a *api) ruleCheckedPlan(c store.Case, target string, p *domain.Plan) *domain.Plan {
	if p == nil || !a.planRuleOn(c) {
		return p
	}
	applies := planrule.AppliesInformational
	if part102Case(c) {
		applies = planrule.AppliesPart102
	}
	cp := *p
	cp.RuleCheck = planrule.Check(a.d.RuleCheck, *c.Product, cp, domain.NormalizeCountry(target), applies)
	return &cp
}

// withPlanRuleChecks is c with the check on its latest plan search and on
// each country comparison's searches. c's own slices are not written to.
func (a *api) withPlanRuleChecks(c store.Case) store.Case {
	if !a.planRuleOn(c) {
		return c
	}
	if c.Plan != nil {
		r := a.ruleChecked(c, *c.Plan)
		c.Plan = &r
	}
	if c.Targets != nil && c.Targets.Results != nil {
		t := *c.Targets
		t.Results = make(map[string]planner.Result, len(c.Targets.Results))
		for k, r := range c.Targets.Results {
			t.Results[k] = a.ruleChecked(c, r)
		}
		c.Targets = &t
	}
	return c
}
