package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/adcvd"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// adcvdMaxReads caps the model questions of one request.
const adcvdMaxReads = 60

// The case view runs the scope-first check on the first parts only, within a
// time and a call budget; POST /v1/adcvd/check takes the rest (D959).
const (
	adcvdCaseParts  = 10
	adcvdCaseCalls  = 60
	adcvdCaseWithin = 30 * time.Second
)

// adcvdCheck answers POST /v1/adcvd/check: which parts may be subject to an
// active U.S. antidumping or countervailing duty order, from each part's HTS
// code and country of origin. Nothing stored (D939). Served only with
// ADCVD_CHECK=on. Deterministic and with no model call, unless
// ADCVD_SCOPE_READER=on: then a model reads each listed order's written scope
// against the part's name and an order is dropped only when the scope clearly
// points away from the part (D942, D943). With a scope-first check (D959) the
// route runs it instead: the parts need a name and an origin, and the HTS
// code is only a hint.
func (a *api) adcvdCheck(w http.ResponseWriter, r *http.Request) {
	var in adcvd.Input
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if len(in.Parts) == 0 {
		a.fail(w, r, fmt.Errorf("%w: at least one part is required", domain.ErrInvalid))
		return
	}
	if len(in.Parts) > adcvd.MaxParts {
		a.fail(w, r, fmt.Errorf("%w: at most %d parts", domain.ErrInvalid, adcvd.MaxParts))
		return
	}
	if a.d.ADCVDScopeFirst != nil {
		writeJSON(w, http.StatusOK, a.d.ADCVD.CheckScopeFirstCtx(r.Context(), in, a.d.ADCVDScopeFirst, adcvd.ScopeFirstMaxCalls))
		return
	}
	cfg := adcvd.Config{}
	if a.d.ADCVDReader != nil {
		cfg = adcvd.Config{ScopeTerms: true, MinScopeTerms: adcvd.MinScopeTerms, Reader: a.d.ADCVDReader, MaxReads: adcvdMaxReads}
	}
	writeJSON(w, http.StatusOK, a.d.ADCVD.CheckWith(in, cfg))
}

// adcvdCaseCheck is the screen of a case's parts, or nil when the check is
// off or no part gives an HTS code. It is computed when the case is read,
// from the stored product, and is never stored or fed back: the origin
// answer, the triage and every duty figure are exactly what they would be
// without it (D939).
func (a *api) adcvdCaseCheck(c store.Case) *adcvd.Result {
	if a.d.ADCVD == nil || c.Product == nil {
		return nil
	}
	if a.d.ADCVDScopeFirst != nil {
		return a.adcvdCaseScopeFirst(c)
	}
	var in adcvd.Input
	any := false
	for _, comp := range c.Product.Components {
		if strings.TrimSpace(comp.HTS) != "" {
			any = true
		}
		in.Parts = append(in.Parts, adcvd.Part{Name: comp.Name, HTS: comp.HTS, Origin: comp.MadeIn})
	}
	if !any {
		return nil
	}
	if len(in.Parts) > adcvd.MaxParts {
		in.Parts = in.Parts[:adcvd.MaxParts]
	}
	res := a.d.ADCVD.Check(in)
	return &res
}

// adcvdCaseScopeFirst is the case view's scope-first check: the first parts
// that give a name and an origin, within a time and a call budget. Answers
// are cached in memory, so reading the case again asks the model nothing.
func (a *api) adcvdCaseScopeFirst(c store.Case) *adcvd.Result {
	var in adcvd.Input
	for _, comp := range c.Product.Components {
		if strings.TrimSpace(comp.Name) == "" || strings.TrimSpace(comp.MadeIn) == "" {
			continue
		}
		if len(in.Parts) == adcvdCaseParts {
			break
		}
		in.Parts = append(in.Parts, adcvd.Part{Name: comp.Name, HTS: comp.HTS, Origin: comp.MadeIn})
	}
	if len(in.Parts) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), adcvdCaseWithin)
	defer cancel()
	res := a.d.ADCVD.CheckScopeFirstCtx(ctx, in, a.d.ADCVDScopeFirst, adcvdCaseCalls)
	return &res
}
