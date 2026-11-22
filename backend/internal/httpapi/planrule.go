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

// ruleChecked is res with each plan's rule check and plan_id (a copy).
func (a *api) ruleChecked(c store.Case, res planner.Result) planner.Result {
	if a.planRuleOn(c) {
		res = planrule.Annotate(a.d.RuleCheck, *c.Product, res, part102Case(c))
	}
	return withPlanIDs(res)
}

// withPlanIDs is res with every plan's PlanID set (D470), on copies of its
// slices: the stored search is not written to.
func withPlanIDs(res planner.Result) planner.Result {
	ids := func(ps []domain.Plan) []domain.Plan {
		if ps == nil {
			return nil
		}
		out := make([]domain.Plan, len(ps))
		for i, p := range ps {
			out[i] = p.WithID()
		}
		return out
	}
	res.Plans, res.Rejected = ids(res.Plans), ids(res.Rejected)
	return res
}

// withTargetPlanIDs is a country comparison's ranking with each best plan's
// PlanID set, on copies.
func withTargetPlanIDs(ranked []store.TargetSummary) []store.TargetSummary {
	if ranked == nil {
		return nil
	}
	out := make([]store.TargetSummary, len(ranked))
	for i, t := range ranked {
		if t.Best != nil {
			b := t.Best.WithID()
			t.Best = &b
		}
		out[i] = t
	}
	return out
}

// ruleCheckedPlan is ruleChecked for one streamed plan (a copy).
func (a *api) ruleCheckedPlan(c store.Case, target string, p *domain.Plan) *domain.Plan {
	if p == nil {
		return nil
	}
	cp := p.WithID()
	if !a.planRuleOn(c) {
		return &cp
	}
	applies := planrule.AppliesInformational
	if part102Case(c) {
		applies = planrule.AppliesPart102
	}
	cp.RuleCheck = planrule.Check(a.d.RuleCheck, *c.Product, cp, domain.NormalizeCountry(target), applies)
	return &cp
}

// withPlanRuleChecks is c with the check and plan IDs on its latest plan
// search and on each country comparison's searches. c's own slices are not
// written to.
func (a *api) withPlanRuleChecks(c store.Case) store.Case {
	if c.Plan != nil {
		r := a.ruleChecked(c, *c.Plan)
		c.Plan = &r
	}
	if c.Targets != nil {
		t := *c.Targets
		if c.Targets.Results != nil {
			t.Results = make(map[string]planner.Result, len(c.Targets.Results))
			for k, r := range c.Targets.Results {
				t.Results[k] = a.ruleChecked(c, r)
			}
		}
		t.Ranked = withTargetPlanIDs(c.Targets.Ranked)
		c.Targets = &t
	}
	return c
}
