package httpapi

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/adcvd"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// adcvdCheck answers POST /v1/adcvd/check: which parts may be subject to an
// active U.S. antidumping or countervailing duty order, from each part's HTS
// code and country of origin. Deterministic, no model call, nothing stored
// (D939). Served only with ADCVD_CHECK=on.
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
	writeJSON(w, http.StatusOK, a.d.ADCVD.Check(in))
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
