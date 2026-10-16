package httpapi

import (
	"net/http"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/duty"
)

// dutyRates answers GET /v1/duty-rates[?hts=8419.89.95]: the sourced rates
// with every component and its source (D52). Without hts it lists every line.
func (a *api) dutyRates(w http.ResponseWriter, r *http.Request) {
	if a.d.Duty == nil {
		writeError(w, http.StatusServiceUnavailable, "internal", "the duty table is not loaded", nil)
		return
	}
	hts := strings.TrimSpace(r.URL.Query().Get("hts"))
	if hts == "" {
		writeJSON(w, http.StatusOK, map[string]any{"lines": a.d.Duty.Lines()})
		return
	}
	l, ok := a.d.Duty.Line(hts)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "no sourced duty rates for HTS "+duty.Normalize(hts)+"; enter the rates yourself", nil)
		return
	}
	writeJSON(w, http.StatusOK, l)
}
