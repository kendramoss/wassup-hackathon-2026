package httpapi

import (
	"fmt"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner/opcost"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// Operations-mode money state (#281). A plan searched with operations inputs
// carries economics_basis, the hash of the RAW money it was priced from. The
// stamp is taken from the request's economics before a.priced fills any rate,
// so a normal request is not stale the moment it is stored, and the case holds
// the same raw economics, so a later hash of the stored case matches until the
// user edits a cost, a rate, a constraint or the annual value. Results without
// operations carry no stamp and none of this applies to them.

// economicsHash is the plan-stamp hash of an economics value (nil: no money).
func economicsHash(e *planner.Economics) string {
	if e == nil {
		return opcost.BasisHash(opcost.MoneyBasis{})
	}
	return opcost.BasisHash(moneyOf(e))
}

// rawBasis is the stamp for a request: the hash of its raw economics when it
// carries operations, else "" (legacy results are not stamped).
func rawBasis(e planner.Economics) string {
	if !e.Operations.Active() {
		return ""
	}
	return economicsHash(&e)
}

// stampBasis writes the stamp on an operations-mode result.
func stampBasis(res *planner.Result, basis string) {
	if res.Objective != nil && basis != "" {
		res.EconomicsBasis = basis
	}
}

// stillPriced is stillCurrent for money, run inside the store update: a result
// priced from the case's stored economics (the request carried no annual value
// of its own) is discarded when those economics changed while it ran. A request
// that carries its own money saves that money with the result, so they agree by
// construction. Legacy results (no stamp) are not checked: unchanged behaviour.
func stillPriced(latest *store.Case, supplied *planner.Economics, basis string) error {
	if basis == "" || (supplied != nil && supplied.AnnualImportValueUSD > 0) {
		return nil
	}
	if economicsHash(latest.Economics) != basis {
		return errSuperseded
	}
	return nil
}

// resultMoneyStale reports that an operations-mode result was priced from other
// money than the case now holds. A result without a stamp is never reported.
func resultMoneyStale(c store.Case, res *planner.Result) bool {
	return res != nil && res.EconomicsBasis != "" && res.EconomicsBasis != economicsHash(c.Economics)
}

// planMoneyStale and targetsMoneyStale tell readers (the case view, Ask) not
// to present a stored plan's modelled amounts as the current money.
func planMoneyStale(c store.Case) bool { return resultMoneyStale(c, c.Plan) }

func targetsMoneyStale(c store.Case) bool {
	if c.Targets == nil {
		return false
	}
	for _, res := range c.Targets.Results {
		res := res
		if resultMoneyStale(c, &res) {
			return true
		}
	}
	return false
}

// moneyStaleFields marks, on a case view or a money save, the stored results
// whose modelled amounts were worked out from other money than the case now
// holds (operations mode only). The keys are absent otherwise, so a case
// without operations serialises as before.
func moneyStaleFields(f map[string]any, c store.Case) {
	if planMoneyStale(c) {
		f["plan_economics_stale"] = true
	}
	if targetsMoneyStale(c) {
		f["targets_economics_stale"] = true
	}
}

// opsNetText states an operations-mode plan's net with its scope: the figure
// covers modelled terms only, and the terms it leaves out are named, also when
// the plan is fully priced.
func opsNetText(p domain.Plan, stale bool) string {
	o := p.Operations
	if stale {
		return "net not current: the money inputs changed after this search, run the plan again"
	}
	var s string
	switch {
	case p.NetPriced != nil && *p.NetPriced:
		s = fmt.Sprintf("modelled net $%.0f/yr", p.NetYear)
	default:
		s = "modelled net not fully priced"
	}
	s += " (modelled terms only; not modelled:"
	if len(o.NotModelled) == 0 {
		s += " none listed"
	}
	for i, n := range o.NotModelled {
		if i > 0 {
			s += ","
		}
		s += " " + n
	}
	s += ")"
	if o.Assessed {
		s += "; feasibility " + o.Feasibility
	} else {
		s += "; feasibility not assessed"
	}
	return s
}
