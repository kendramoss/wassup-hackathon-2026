package httpapi

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/rcep"
)

// maxRCEPMaterials caps a bill of materials on POST /v1/rcep/check.
const maxRCEPMaterials = 200

// rcepCheck answers POST /v1/rcep/check: does a good qualify as RCEP
// originating (preferential origin under the RCEP Agreement, Chapter 3 and
// Annex 3A)? Deterministic code, no model call, nothing stored (D690). The answer is labelled regime "rcep_preferential" and
// is separate from the marking and non-preferential origin the case engines
// give.
func (a *api) rcepCheck(w http.ResponseWriter, r *http.Request) {
	var in rcep.Input
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if len(in.Materials) > maxRCEPMaterials {
		a.fail(w, r, fmt.Errorf("%w: at most %d materials", domain.ErrInvalid, maxRCEPMaterials))
		return
	}
	res, err := rcep.Check(in)
	if errors.Is(err, rcep.ErrInput) {
		a.fail(w, r, fmt.Errorf("%w: %v", domain.ErrInvalid, err))
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
