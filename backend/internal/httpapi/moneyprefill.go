package httpapi

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/duty"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/hts"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

// moneyPrefill answers POST /v1/products/money-prefill {product, economics?}
// (#235): it fills the duty rates the user has not given, so the money
// inputs start filled instead of blank.
//
//  1. HTS: the product's own, else (with HTS_SUGGEST) the top suggestion
//     from CBP rulings, labelled as a suggestion.
//  2. Duty: for every country in the product, the sourced table rate (D52)
//     or the live MFN rate plus Section 301 (D65, D90), as a plan search
//     would price it.
//
// The user's own values always win; nothing is saved. Sources say where
// each filled value came from.
func (a *api) moneyPrefill(w http.ResponseWriter, r *http.Request) {
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
	p := *in.Product
	e := planner.Economics{Illustrative: true}
	if in.Economics != nil {
		e = *in.Economics
	}
	out := map[string]any{"disclaimer": domain.Disclaimer}
	htsSource := ""
	if code, ok := duty.LiveHTS(p.HTS); ok {
		p.HTS, htsSource = code, "product"
	} else if a.d.HTS != nil && strings.TrimSpace(p.Name) != "" {
		res, err := a.d.HTS.SuggestNamed(r.Context(), hts.ProductText(p), p.Name)
		if err != nil && !res.ReasonsUnavailable {
			a.d.Logger.WarnContext(r.Context(), "money prefill: hts suggestion failed; no duty filled", "request_id", RequestID(r.Context()), "err", err)
		}
		if len(res.Candidates) > 0 {
			p.HTS, htsSource = res.Candidates[0].HTS, "suggested"
			out["hts_suggestion"] = res.Candidates[0]
			out["hts_note"] = res.Note
		}
	}
	if htsSource != "" {
		out["hts"], out["hts_source"] = p.HTS, htsSource
		given := e.AnnualImportValueUSD
		if given <= 0 {
			e.AnnualImportValueUSD = 1 // priced only fills rates for economics with an import value
		}
		e = a.priced(r.Context(), e, &p, productCountries(&p), true)
		e.AnnualImportValueUSD = given
	}
	if e.DutyRates == nil {
		e.DutyRates = map[string]float64{}
	}
	src := map[string]string{}
	for _, c := range e.DutyRatesFromTable {
		src[c] = "table"
	}
	for _, c := range e.DutyRatesLiveMFN {
		src[c] = "live"
	}
	for c := range e.DutyRates {
		if _, ok := src[c]; !ok {
			src[c] = "given"
		}
	}
	out["economics"], out["duty_sources"] = e, src
	writeJSON(w, http.StatusOK, out)
}
