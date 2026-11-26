package httpapi

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/atiga"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// maxATIGAMaterials caps a bill of materials on POST /v1/atiga/check.
const maxATIGAMaterials = 200

// atigaCheck answers POST /v1/atiga/check: does a good qualify as ATIGA
// originating (preferential origin under the ASEAN Trade in Goods
// Agreement, Chapter 3 and Annex 3)? Deterministic code, no model call,
// nothing stored (D925). Served only with ATIGA_CHECK=on.
func (a *api) atigaCheck(w http.ResponseWriter, r *http.Request) {
	var in atiga.Input
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if len(in.Materials) > maxATIGAMaterials {
		a.fail(w, r, fmt.Errorf("%w: at most %d materials", domain.ErrInvalid, maxATIGAMaterials))
		return
	}
	res, err := atiga.Check(in)
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
