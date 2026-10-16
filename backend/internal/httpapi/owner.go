package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"regexp"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// Case ownership (D58). The frontend proxy keeps a random visitor ID in an
// httpOnly cookie and sends it as X-Visitor-ID; the backend stores a hash of
// it on each case it creates. With Deps.CaseOwnership on, a visitor sees the
// pinned demos and their own cases only, and another visitor's case is a 404
// (so its existence does not leak). The admin token sees everything.

var visitorIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)

// ownerOf returns the owner hash for the request's visitor ID, or "".
func ownerOf(r *http.Request) string {
	id := r.Header.Get("X-Visitor-ID")
	if !visitorIDPattern.MatchString(id) {
		return ""
	}
	h := sha256.Sum256([]byte("origin-proof-visitor:" + id))
	return hex.EncodeToString(h[:16])
}

// visible reports whether the request may see case c.
func (a *api) visible(r *http.Request, c store.Case) bool {
	if !a.d.CaseOwnership || c.Pinned || a.isAdmin(r) {
		return true
	}
	o := ownerOf(r)
	return o != "" && o == c.Owner
}

// loadCase gets the case named in the path, as store.ErrNotFound when the
// request may not see it.
func (a *api) loadCase(r *http.Request) (store.Case, error) {
	c, err := a.d.Cases.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		return store.Case{}, err
	}
	if !a.visible(r, c) {
		return store.Case{}, store.ErrNotFound
	}
	return c, nil
}
