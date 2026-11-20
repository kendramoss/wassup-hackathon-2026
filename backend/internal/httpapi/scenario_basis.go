package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// scenarioBasis stamps what a scenario comparison is worked out against: the
// case's product, facts, answers, current origin (the known ruling's, else the
// predicted one), family, date and excluded rulings. It builds on
// inputFingerprint, so there is one definition of "the inputs".
//
// Extension point (#347): the money a comparison is priced with (economics:
// import value, duty rates, group costs) belongs in the struct below, so one
// helper decides whether any saved comparison is still current.
func scenarioBasis(c store.Case) string {
	b, _ := json.Marshal(struct {
		Inputs  string
		Answers []store.Answer
		Origin  string
	}{inputFingerprint(c), c.Answers, baselineOrigin(c)})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
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
// model call, no store write. Without economics there is nothing to price and
// the comparison stays nil (shown as not priced). fetch lets the live MFN
// lookup run (a save); a read passes false and uses cached rates only.
func (a *api) compareScenarios(ctx context.Context, c store.Case, ss []store.Scenario, fetch bool) {
	basis := scenarioBasis(c)
	for i := range ss {
		ss[i].Comparison, ss[i].Basis = nil, basis
	}
	if c.Economics == nil {
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
