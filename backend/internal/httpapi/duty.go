package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/duty"
)

// dutyRates answers GET /v1/duty-rates[?hts=8419.89.95]: the sourced rates
// with every component and its source (D52). Without hts it lists every line.
// With Deps.LiveMFN, a subheading the table does not have is answered with
// its live MFN rate from the USITC HTS (D65); the table always wins.
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
	if l, ok := a.d.Duty.Line(hts); ok {
		writeJSON(w, http.StatusOK, l)
		return
	}
	if a.d.LiveMFN != nil {
		m, ok, refused := a.liveMFN(w, r, hts)
		if refused {
			return
		}
		if ok {
			writeJSON(w, http.StatusOK, map[string]any{"hts": m.HTS, "description": m.Description, "live_mfn": m})
			return
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "no sourced duty rates for HTS "+duty.Normalize(hts)+"; enter the rates yourself", nil)
}

// liveMFN looks up hts on the HTS. A fetch (not a cached answer) takes from
// the client's lookup bucket, as ruling fetches do (D58), so enumerating
// subheadings cannot get our user agent blocked; when that is refused it
// writes 429 and reports refused. A missing line or a failed fetch is ok
// false, so the caller answers 404 rather than an error.
func (a *api) liveMFN(w http.ResponseWriter, r *http.Request, hts string) (m duty.MFN, ok, refused bool) {
	m, cached, err := a.d.LiveMFN.Cached(hts)
	if !cached {
		if a.limiter != nil && !a.isAdmin(r) {
			if allowed, wait := a.limiter.allow(clientKey(r), lookup); !allowed {
				a.tooMany(w, wait)
				return duty.MFN{}, false, true
			}
		}
		m, err = a.d.LiveMFN.MFN(r.Context(), hts)
	}
	if err != nil {
		if !errors.Is(err, duty.ErrNoRate) {
			a.d.Logger.WarnContext(r.Context(), "live MFN lookup failed", "request_id", RequestID(r.Context()), "hts", hts, "err", err)
		}
		return duty.MFN{}, false, false
	}
	return m, true, false
}
