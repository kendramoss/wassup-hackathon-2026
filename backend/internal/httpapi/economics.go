package httpapi

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner/opcost"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// maxOperationsBytes bounds the operations block (contract section 3), inside
// the request body limit.
const maxOperationsBytes = 64 << 10

// validateEconomics checks money inputs are usable: a non-negative yearly
// import value, duty rates between 0 and 1, and non-negative group costs.
// Operations, when active, are checked with no product (see validateEconomicsFor).
func validateEconomics(e planner.Economics) error { return validateEconomicsFor(e, nil, false) }

// validateEconomicsFor is validateEconomics plus the operations block (#281):
// each problem opcost.Validate finds is joined into the same invalid_input
// message, as "operations.<slug>: <path>: <why>". product, when known, tells
// which one-time costs sit at a group's current plant (sunk). strict, used
// where the money is written (PUT economics, create case), also checks group
// and plant IDs and plant refs against the product; at plan time a ref that no
// longer matches is not an error, it is read as unknown, so a later product
// edit cannot brick plan runs.
func validateEconomicsFor(e planner.Economics, product *domain.Product, strict bool) error {
	var bad []string
	if math.IsNaN(e.AnnualImportValueUSD) || math.IsInf(e.AnnualImportValueUSD, 0) {
		bad = append(bad, "annual_import_value_usd must be a finite number")
	} else if e.AnnualImportValueUSD < 0 {
		bad = append(bad, "annual_import_value_usd must not be negative")
	}
	for c, r := range e.DutyRates {
		if math.IsNaN(r) || math.IsInf(r, 0) || r < 0 || r > 1 {
			bad = append(bad, fmt.Sprintf("duty_rates[%s] = %v; give a fraction between 0 and 1 (0.25 = 25%%)", c, r))
		}
	}
	for g, plants := range e.GroupCosts {
		for p, v := range plants {
			if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
				bad = append(bad, fmt.Sprintf("group_costs[%s][%s] must be a finite number, not negative", g, p))
			}
		}
	}
	if e.Operations.Active() {
		if b, _ := json.Marshal(e.Operations); len(b) > maxOperationsBytes {
			bad = append(bad, fmt.Sprintf("operations.too_large: operations: at most %d KiB", maxOperationsBytes>>10))
		}
		for _, p := range opcost.Validate(planner.CanonicalOps(e.Operations), planner.ValidateEnv(product, strict, e)) {
			bad = append(bad, p.Error())
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("%w: economics: %s", domain.ErrInvalid, strings.Join(bad, "; "))
	}
	return nil
}

// operationsDecodeError explains a request body whose operations block cannot
// be decoded, with the slug opcost.Decode gives (unknown_field, invalid_json,
// trailing_data) under the economics: prefix. It returns nil when the block is
// absent or decodes, so the caller's own error stands. The block sits at the
// top level (PUT economics) or under "economics" (every other route).
func operationsDecodeError(body []byte) error {
	var top map[string]json.RawMessage
	if json.Unmarshal(body, &top) != nil {
		return nil
	}
	hold := top
	if raw, ok := top["economics"]; ok {
		var inner map[string]json.RawMessage
		if json.Unmarshal(raw, &inner) != nil {
			return nil
		}
		hold = inner
	}
	raw, ok := hold["operations"]
	if !ok {
		return nil
	}
	if _, err := opcost.Decode(raw); err != nil {
		return fmt.Errorf("%w: economics: %v", domain.ErrInvalid, err)
	}
	return nil
}

// cleanEconomics drops an inactive operations block (null, {}, {"groups":{}})
// and canonicalises negative zero in an active one, on a copy. A case without
// operations is returned as it was.
func cleanEconomics(e planner.Economics) planner.Economics {
	e.Operations = opcost.Normalize(e.Operations)
	return e
}

// effectiveEconomics is the economics a plan, estimate, scenarios or
// compare-targets call will use: the supplied body when it carries an annual
// value, else the case's stored economics (the one reuse rule, see
// economicsFrom). The supplied body and the effective economics are both
// validated, so invalid input is refused whether it is used or ignored, and
// legacy invalid stored economics are refused when reused. Callers run it
// before any pricing, engine call or write.
func effectiveEconomics(supplied planner.Economics, stored *planner.Economics, product *domain.Product) (planner.Economics, error) {
	if err := validateEconomicsFor(supplied, product, false); err != nil {
		return planner.Economics{}, err
	}
	econ := supplied
	if econ.AnnualImportValueUSD == 0 && stored != nil {
		econ = *stored
	}
	if err := validateEconomicsFor(econ, product, false); err != nil {
		return planner.Economics{}, err
	}
	return cleanEconomics(econ), nil
}

// putEconomics replaces the case's money inputs without running anything;
// plan, scenarios and compare-targets use them when a call omits its own.
func (a *api) putEconomics(w http.ResponseWriter, r *http.Request) {
	c, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if a.refuseDemoEdit(w, r, c) {
		return
	}
	var e planner.Economics
	if err := decode(w, r, &e); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := validateEconomicsFor(e, c.Product, true); err != nil {
		a.fail(w, r, err)
		return
	}
	e = cleanEconomics(e) // an inactive operations block is not stored
	if err := a.auditCase(r, c, "economics"); err != nil {
		a.fail(w, r, err)
		return
	}
	now := time.Now().UTC()
	saved, err := a.d.Cases.Update(r.Context(), c.ID, func(latest *store.Case) error {
		latest.Economics, latest.UpdatedAt = &e, now
		a.repriceScenarios(r.Context(), latest) // never leave old numbers under the new money
		sum := fmt.Sprintf("money inputs updated: $%.0f a year, %d duty rates, %d groups costed",
			e.AnnualImportValueUSD, len(e.DutyRates), len(e.GroupCosts))
		if e.Operations != nil {
			sum += fmt.Sprintf(", operations: %d groups", len(e.Operations.Groups))
		}
		record(r.Context(), latest, store.Event{Kind: "economics", Summary: sum})
		return nil
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out := map[string]any{"case_id": saved.ID, "economics": saved.Economics}
	moneyStaleFields(out, saved)
	if len(saved.Scenarios) > 0 {
		out["scenarios"] = a.scenarioFields(saved.Scenarios) // priced with the money just saved
	}
	writeJSON(w, http.StatusOK, out)
}
