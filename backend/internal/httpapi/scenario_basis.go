package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner/opcost"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// scenarioBasis stamps what a scenario comparison is worked out against: the
// case's product, facts, answers, current origin (the known ruling's, else the
// predicted one), family, date and excluded rulings, plus the money it is
// priced with (moneyBasis). It builds on inputFingerprint, so there is one
// definition of "the inputs", and it is the one helper that decides whether
// any saved comparison is still current.
func scenarioBasis(c store.Case) string {
	answers := c.Answers
	if len(answers) == 0 { // stores differ on nil and empty
		answers = nil
	}
	b, _ := json.Marshal(struct {
		Inputs  string
		Answers []store.Answer
		Origin  string
		Money   *opcost.Canon
	}{inputFingerprint(c), answers, baselineOrigin(c), moneyBasis(c.Economics)})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// moneyOf is the raw money of an economics value: what the user entered,
// before the tariff table or the live lookup fills any rate.
func moneyOf(e *planner.Economics) opcost.MoneyBasis {
	return opcost.MoneyBasis{Illustrative: e.Illustrative, AnnualImportValueUSD: e.AnnualImportValueUSD,
		DutyRates: e.DutyRates, GroupCosts: e.GroupCosts, Operations: e.Operations}
}

// moneyBasis is the part of the case's economics planner.Compare reads: the
// annual import value, the duty rate per origin, the group costs, the
// operations inputs that change a result (#281; not their note or sources),
// and the illustrative flag the comparison carries. Derived notes and
// provenance lists (what filled a rate) are not inputs, so they are left out.
// Rates the tariff table or the live lookup fill in at pricing time are a
// function of the product's HTS, which the inputs already cover. It is
// opcost.CanonicalMoney, the one normalisation the plan stamp also hashes;
// without operations it marshals to the bytes this struct always did.
func moneyBasis(e *planner.Economics) *opcost.Canon {
	if e == nil {
		return nil
	}
	c := opcost.CanonicalMoney(moneyOf(e))
	return &c
}

// baselineOrigin is the origin a scenario is compared with: CBP's known one,
// else the case's prediction.
func baselineOrigin(c store.Case) string {
	if c.CurrentOrigin != nil && c.CurrentOrigin.Country != "" {
		return c.CurrentOrigin.Country
	}
	return c.Prediction.Origin
}

// compareScenarios prices each scenario against c's baseline and stamps the
// result with scenarioBasis(c). Pure arithmetic on stored predictions: no
// model call, no store write. Without economics, or without an annual import value, there is nothing to price and
// the comparison stays nil (shown as not priced). fetch lets the live MFN
// lookup run (a save); a read passes false and uses cached rates only.
func (a *api) compareScenarios(ctx context.Context, c store.Case, ss []store.Scenario, fetch bool) {
	basis := scenarioBasis(c)
	for i := range ss {
		ss[i].Comparison, ss[i].Basis = nil, basis
	}
	if c.Economics == nil || c.Economics.AnnualImportValueUSD <= 0 { // nothing to price: null, not $0
		return
	}
	origins := caseOrigins(c)
	for _, s := range ss {
		origins = append(append(origins, domain.CountryParts(s.Prediction.Origin)...), productCountries(s.Product)...)
	}
	e := a.priced(ctx, *c.Economics, c.Product, origins, fetch) // table and live rates are not stored on the case
	current := baselineOrigin(c)
	for i := range ss {
		cmp := planner.Compare(e, c.Product, ss[i].Product, current, ss[i].Prediction.Origin)
		ss[i].Comparison = &cmp
	}
}

// currentScenarios is the case's scenarios as a reader may see them: a
// comparison stamped with another basis is recomputed from the current
// baseline, never shown stale. The stored case is not changed.
func (a *api) currentScenarios(c store.Case) []store.Scenario {
	basis := scenarioBasis(c)
	stale := false
	for _, s := range c.Scenarios {
		if s.Basis != basis {
			stale = true
		}
	}
	if !stale {
		return c.Scenarios
	}
	out := append([]store.Scenario(nil), c.Scenarios...)
	a.compareScenarios(context.Background(), c, out, false)
	return out
}

// dropScenarioComparisons removes the comparisons worked out against a
// baseline that just changed (clearDerived); the definitions stay.
func dropScenarioComparisons(c *store.Case) {
	for i := range c.Scenarios {
		c.Scenarios[i].Comparison, c.Scenarios[i].Basis = nil, ""
	}
}

// repriceScenarios brings the stored comparisons up to date with the case as
// it now stands (after its economics changed): only scenarios stamped with
// another basis are recomputed, from cached rates (no network), so an
// unchanged save leaves them as they were.
func (a *api) repriceScenarios(ctx context.Context, c *store.Case) {
	basis := scenarioBasis(*c)
	for _, s := range c.Scenarios {
		if s.Basis != basis {
			a.compareScenarios(ctx, *c, c.Scenarios, false)
			return
		}
	}
}
