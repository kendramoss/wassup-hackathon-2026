package httpapi

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/aseanfta"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/atiga"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// akftaCheck answers POST /v1/akfta/check: does a good qualify as AKFTA
// originating (ASEAN-Korea, Annex 3 and the Appendix 2 product-specific
// rules)? Deterministic code, no model call, nothing stored (D949). Served
// only with AKFTA_CHECK=on.
func (a *api) akftaCheck(w http.ResponseWriter, r *http.Request) {
	a.aseanFTA(w, r, aseanfta.AKFTA)
}

// acftaCheck answers POST /v1/acfta/check: the same for the ASEAN-China
// agreement as upgraded (Annex 1 and Attachment B). Served only with
// ACFTA_CHECK=on (D949).
func (a *api) acftaCheck(w http.ResponseWriter, r *http.Request) {
	a.aseanFTA(w, r, aseanfta.ACFTA)
}

func (a *api) aseanFTA(w http.ResponseWriter, r *http.Request, ag aseanfta.Agreement) {
	var in atiga.Input
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if len(in.Materials) > maxATIGAMaterials {
		a.fail(w, r, fmt.Errorf("%w: at most %d materials", domain.ErrInvalid, maxATIGAMaterials))
		return
	}
	res, err := aseanfta.Check(ag, in)
	if errors.Is(err, atiga.ErrInput) {
		a.fail(w, r, fmt.Errorf("%w: %v", domain.ErrInvalid, err))
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
