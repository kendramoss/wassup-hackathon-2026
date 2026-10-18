package httpapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/costest"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

// CostEstimator fills blank group costs (costest.Estimator).
type CostEstimator interface {
	Estimate(ctx context.Context, in costest.Input) (costest.Result, error)
}

// costEstimate answers POST /v1/products/cost-estimate {product, economics}
// (#235 step 4): AI estimates for the group costs the user left blank, with
// a range and a reason each. Nothing is saved; the user's costs never change.
func (a *api) costEstimate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Product   *domain.Product    `json:"product"`
		Economics *planner.Economics `json:"economics"`
	}
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if in.Product == nil || len(in.Product.Plants) == 0 {
		a.fail(w, r, fmt.Errorf("%w: a product with plants is required", domain.ErrInvalid))
		return
	}
	names := map[string]string{}
	if t, ok := a.d.Templates[in.Product.Family]; ok {
		for _, g := range t.Groups {
			names[g.ID] = g.Name
		}
	}
	e := planner.Economics{}
	if in.Economics != nil {
		e = *in.Economics
	}
	res, err := a.d.CostEstimator.Estimate(r.Context(), costest.Input{Product: *in.Product, GroupNames: names, GroupCosts: e.GroupCosts, AnnualImportValueUSD: e.AnnualImportValueUSD})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"estimates": res.Estimates, "note": res.Note, "prompt_version": costest.PromptVersion, "llm_provider": a.d.Provider, "illustrative": true})
}
