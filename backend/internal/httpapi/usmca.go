package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/usmca"
)

// dottedHS matches an HS code written with dots (8202.31, 8202.31.00,
// 82.02). Only such strings are rewritten to plain digits; anything else is
// left for the engine to refuse.
var dottedHS = regexp.MustCompile(`^[0-9]{2}(?:\.?[0-9]{2}){0,4}$`)

// dottedGoodHS is the stricter shape of good_hs, matching the OpenAPI pattern:
// four leading digits, then up to three pairs ("82.02" is refused).
var dottedGoodHS = regexp.MustCompile(`^[0-9]{4}(?:\.?[0-9]{2}){0,3}$`)

// normaliseUSMCAHS rewrites dotted HS codes (good_hs and materials[].hs) to
// the plain digits the engine requires. Anything it cannot parse is returned
// unchanged so usmca.DecodeInput reports the real error.
func normaliseUSMCAHS(raw []byte) []byte {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil || top == nil {
		return raw
	}
	fix := func(m json.RawMessage, shape *regexp.Regexp) (json.RawMessage, bool) {
		var s string
		if json.Unmarshal(m, &s) != nil || !strings.Contains(s, ".") || !shape.MatchString(s) {
			return m, false
		}
		b, _ := json.Marshal(strings.ReplaceAll(s, ".", ""))
		return b, true
	}
	changed := false
	if v, ok := top["good_hs"]; ok {
		if nv, c := fix(v, dottedGoodHS); c {
			top["good_hs"], changed = nv, true
		}
	}
	if v, ok := top["materials"]; ok {
		var mats []map[string]json.RawMessage
		if json.Unmarshal(v, &mats) == nil {
			for _, m := range mats {
				if h, ok := m["hs"]; ok {
					if nv, c := fix(h, dottedHS); c {
						m["hs"], changed = nv, true
					}
				}
			}
			if b, err := json.Marshal(mats); err == nil {
				top["materials"] = b
			}
		}
	}
	if !changed {
		return raw
	}
	out, err := json.Marshal(top)
	if err != nil {
		return raw
	}
	return out
}

// usmcaCheck answers POST /v1/usmca/check: is one selected USMCA
// product-specific origin rule met by the facts given? Deterministic code, no
// model call, nothing stored. It is not a certification of origin, a tariff
// or duty determination, and it is distinct from marking and RCEP origin.
func (a *api) usmcaCheck(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		a.fail(w, r, fmt.Errorf("%w: request body: %v", domain.ErrInvalid, err))
		return
	}
	in, err := usmca.DecodeInput(normaliseUSMCAHS(raw))
	if err != nil {
		a.fail(w, r, fmt.Errorf("%w: %v", domain.ErrInvalid, err))
		return
	}
	res, err := usmca.Check(in)
	if errors.Is(err, usmca.ErrInput) {
		a.fail(w, r, fmt.Errorf("%w: %v", domain.ErrInvalid, err))
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
