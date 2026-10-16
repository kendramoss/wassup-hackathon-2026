package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/hts"
)

// HTSSuggester suggests HTS subheadings (hts.Suggester).
type HTSSuggester interface {
	Suggest(ctx context.Context, facts string) (hts.Result, error)
}

// htsSuggest answers POST /v1/products/hts-suggest {facts} or {product}: up
// to three candidate 8-digit subheadings from rulings that state one, each
// with its rulings, a short reason and whether the duty table has rates for
// it (D64). Only routed when HTS_SUGGEST=on. If the reason call fails the
// candidates are still returned, with reasons_unavailable.
func (a *api) htsSuggest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Facts   string          `json:"facts"`
		Product *domain.Product `json:"product"`
	}
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	text := strings.TrimSpace(in.Facts)
	if text == "" && in.Product != nil {
		if strings.TrimSpace(in.Product.Name) == "" {
			a.fail(w, r, fmt.Errorf("%w: product name is required", domain.ErrInvalid))
			return
		}
		text = hts.ProductText(*in.Product)
	}
	if text == "" || utf8.RuneCountInString(text) > domain.MaxFactsRunes {
		a.fail(w, r, fmt.Errorf("%w: facts or a product are required, up to %d characters", domain.ErrInvalid, domain.MaxFactsRunes))
		return
	}
	res, err := a.d.HTS.Suggest(r.Context(), text)
	if err != nil {
		if !res.ReasonsUnavailable {
			a.fail(w, r, err)
			return
		}
		a.d.Logger.WarnContext(r.Context(), "hts reasons failed; candidates without reasons", "request_id", RequestID(r.Context()), "err", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"candidates": res.Candidates, "note": res.Note,
		"reasons_unavailable": res.ReasonsUnavailable, "trace": res.Trace, "llm_provider": a.d.Provider,
		"prompt_version": hts.PromptVersion, "disclaimer": domain.Disclaimer})
}
