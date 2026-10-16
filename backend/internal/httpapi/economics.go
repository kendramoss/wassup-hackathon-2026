package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// validateEconomics checks money inputs are usable: a non-negative yearly
// import value, duty rates between 0 and 1, and non-negative group costs.
func validateEconomics(e planner.Economics) error {
	var bad []string
	if e.AnnualImportValueUSD < 0 {
		bad = append(bad, "annual_import_value_usd must not be negative")
	}
	for c, r := range e.DutyRates {
		if r < 0 || r > 1 {
			bad = append(bad, fmt.Sprintf("duty_rates[%s] = %v; give a fraction between 0 and 1 (0.25 = 25%%)", c, r))
		}
	}
	for g, plants := range e.GroupCosts {
		for p, v := range plants {
			if v < 0 {
				bad = append(bad, fmt.Sprintf("group_costs[%s][%s] must not be negative", g, p))
			}
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("%w: economics: %s", domain.ErrInvalid, strings.Join(bad, "; "))
	}
	return nil
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
	if err := validateEconomics(e); err != nil {
		a.fail(w, r, err)
		return
	}
	now := time.Now().UTC()
	saved, err := a.d.Cases.Update(r.Context(), c.ID, func(latest *store.Case) error {
		latest.Economics, latest.UpdatedAt = &e, now
		latest.Record(store.Event{Kind: "economics", Summary: fmt.Sprintf("money inputs updated: $%.0f a year, %d duty rates, %d groups costed",
			e.AnnualImportValueUSD, len(e.DutyRates), len(e.GroupCosts))})
		return nil
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"case_id": saved.ID, "economics": saved.Economics})
}
