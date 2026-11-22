package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/hts"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/logsafe"
)

// HTSSuggester suggests HTS subheadings (hts.Suggester).
type HTSSuggester interface {
	SuggestNamed(ctx context.Context, facts, name string) (hts.Result, error)
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
	name := ""
	if in.Product != nil {
		name = in.Product.Name
	}
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
	res, err := a.d.HTS.SuggestNamed(r.Context(), text, name)
	if err != nil {
		if !res.ReasonsUnavailable {
			a.fail(w, r, err)
			return
		}
		a.d.Logger.WarnContext(r.Context(), "hts reasons failed; candidates without reasons", "request_id", requestRef(r.Context()), "err", logsafe.Err(err))
	}
	writeJSON(w, http.StatusOK, map[string]any{"candidates": res.Candidates, "note": res.Note,
		"reasons_unavailable": res.ReasonsUnavailable, "trace": res.Trace, "source": res.Source, "llm_provider": a.d.Provider,
		"prompt_version": hts.PromptVersion, "disclaimer": domain.Disclaimer})
}

// PartsHTSSuggester suggests codes for a product's parts (hts.PartsSuggester).
type PartsHTSSuggester interface {
	Suggest(ctx context.Context, prod domain.Product, opt hts.PartsOptions) (hts.PartsResult, error)
}

// maxPartNameRunes caps one part's name.
const maxPartNameRunes = 200

// partsHTSSuggest answers POST /v1/products/parts-hts-suggest {product}: up
// to three codes for each named part (at most hts.MaxParts), each from an
// earlier CBP ruling that classified a similar part ("rulings") or from the
// model ("model"), with a short reason; and, when the product has no HTS
// code, for the good. One model call per request; answers per part are
// cached for a day, only for the same caller and the same product words
// (D380, D410). Nothing is stored or filled in: the user accepts
// codes in the parts table, and the 19 CFR 102.20 rule check reads them when
// the case is read (D363). Rate-limited as light; counts toward the spend cap.
func (a *api) partsHTSSuggest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Product *domain.Product `json:"product"`
	}
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if in.Product == nil || strings.TrimSpace(in.Product.Name) == "" {
		a.fail(w, r, fmt.Errorf("%w: a product with a name is required", domain.ErrInvalid))
		return
	}
	p := *in.Product
	named := 0
	for _, c := range p.Components {
		if n := utf8.RuneCountInString(strings.TrimSpace(c.Name)); n > maxPartNameRunes {
			a.fail(w, r, fmt.Errorf("%w: a part name is at most %d characters", domain.ErrInvalid, maxPartNameRunes))
			return
		} else if n > 0 {
			named++
		}
	}
	if named == 0 || named > hts.MaxParts {
		a.fail(w, r, fmt.Errorf("%w: give 1 to %d named parts (send the parts still without a code)", domain.ErrInvalid, hts.MaxParts))
		return
	}
	if utf8.RuneCountInString(p.Name)+utf8.RuneCountInString(p.Description) > domain.MaxFactsRunes {
		a.fail(w, r, fmt.Errorf("%w: the product name and description are at most %d characters", domain.ErrInvalid, domain.MaxFactsRunes))
		return
	}
	res, err := a.d.PartsHTS.Suggest(r.Context(), p, hts.PartsOptions{Good: strings.TrimSpace(p.HTS) == "", Scope: a.cacheScope(r)})
	if err != nil {
		if !res.ReasonsUnavailable {
			a.fail(w, r, err)
			return
		}
		a.d.Logger.WarnContext(r.Context(), "parts hts reasons failed; ruling candidates without reasons", "request_id", requestRef(r.Context()), "err", logsafe.Err(err))
	}
	flagged := hts.FlagImplausible(&res, p) // D471: flags, never removes
	writeJSON(w, http.StatusOK, map[string]any{"parts": res.Parts, "good": res.Good, "note": res.Note, "flagged": flagged,
		"reasons_unavailable": res.ReasonsUnavailable, "trace": res.Trace, "llm_provider": a.d.Provider,
		"prompt_version": hts.PartsPromptVersion, "max_parts": hts.MaxParts, "disclaimer": domain.Disclaimer})
}

// cacheScope is who a cached model answer belongs to (D410): a signed-in
// user's workspace (else the user), an API key, a visitor ID, else the
// client address bucket. A cached answer is never served outside its scope.
func (a *api) cacheScope(r *http.Request) string {
	p := a.principalOf(r)
	switch {
	case p.signedIn() && p.Workspace != "":
		return "workspace:" + p.Workspace
	case p.signedIn():
		return "user:" + p.UserOwner
	case p.HasKey:
		return "apikey:" + p.KeyID
	case p.Owner != "":
		return "owner:" + p.Owner
	}
	return "client:" + a.clientKey(r)
}
