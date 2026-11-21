package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/duty"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/logsafe"
)

// dutyRates answers GET /v1/duty-rates[?hts=8419.89.95]: the sourced rates
// with every component and its source (D52). Without hts it lists every line.
// With Deps.LiveMFN, a subheading the table does not have is answered with
// its live MFN rate from the USITC HTS (D65); the table always wins.
// invalidHTSMessage answers a malformed HTS number on any route; it never
// repeats the input.
const invalidHTSMessage = "hts must be an 8- or 10-digit HTS number such as 8419.89.95 or 8419.89.9500 (one separator style: dot, space or dash)"

// invalidHTSNote explains why a product's HTS number was not used to price.
const invalidHTSNote = "The product's HTS number is not a valid 8- or 10-digit HTS number, so no duty rates were filled; enter the rates yourself or correct the HTS."

func (a *api) dutyRates(w http.ResponseWriter, r *http.Request) {
	if a.d.Duty == nil {
		writeError(w, http.StatusServiceUnavailable, "internal", "the duty table is not loaded", nil)
		return
	}
	// Parse the raw query explicitly: URL.Query() silently drops a pair it
	// cannot parse (an unescaped ';', a bad %-escape), which would read as an
	// absent hts and list the whole table. A query that does not parse is
	// refused whole (even if the bad pair is another key); only an absent
	// hts key lists the table. A present hts, even empty, must be valid. With
	// repeated hts keys the first value is used.
	q, qerr := url.ParseQuery(r.URL.RawQuery)
	vals, present := q["hts"]
	if qerr != nil || (present && len(vals) == 0) {
		writeError(w, http.StatusBadRequest, "invalid_input", invalidHTSMessage, nil)
		return
	}
	if !present {
		writeJSON(w, http.StatusOK, map[string]any{"lines": a.d.Duty.Lines()})
		return
	}
	hts := strings.TrimSpace(vals[0])
	code, err := duty.ParseHTS(hts)
	if err != nil {
		// Checked before any table or live lookup, and before the not-found
		// answer. The text is fixed: user input is never echoed back.
		writeError(w, http.StatusBadRequest, "invalid_input", invalidHTSMessage, nil)
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
	writeError(w, http.StatusNotFound, "not_found", "no sourced duty rates for HTS "+code.Subheading+"; enter the rates yourself", nil)
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
			if allowed, wait := a.allow(r, lookup); !allowed { // the shared counters when configured, as for rulings
				a.tooMany(w, wait)
				return duty.MFN{}, false, true
			}
		}
		m, err = a.d.LiveMFN.MFN(r.Context(), hts)
	}
	if err != nil {
		if !errors.Is(err, duty.ErrNoRate) {
			a.d.Logger.WarnContext(r.Context(), "live MFN lookup failed", "request_id", requestRef(r.Context()), "err", logsafe.Err(err))
		}
		return duty.MFN{}, false, false
	}
	return m, true, false
}
