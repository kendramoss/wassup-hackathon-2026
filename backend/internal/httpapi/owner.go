package httpapi

import (
	"errors"
	"net/http"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// Case ownership (D58). The frontend proxy keeps a random visitor ID in an
// httpOnly cookie and sends it as X-Visitor-ID; the backend stores a hash of
// it on each case it creates. With Deps.CaseOwnership on, a visitor sees the
// pinned demos and their own cases only, and another visitor's case is a 404
// (so its existence does not leak). The admin token sees everything.

// ownerOf returns the owner hash for the request's API key (D69) or visitor
// ID, or "". It is the credential owner of the request's principal.
func ownerOf(r *http.Request) string {
	if k, ok := keyFrom(r.Context()); ok {
		return keyOwner(k)
	}
	return visitorOwnerHash(r.Header.Get("X-Visitor-ID"))
}

// visible reports whether the request may see case c: the rule is
// principal.canSee. A signed-in user's visibility is its team branch (D68); a
// case made by a signed-in user is never visible to anonymous visitors. A
// request with an API key, and a case created with one, follow ownership even
// with CaseOwnership off (D69).
func (a *api) visible(r *http.Request, c store.Case) bool {
	return a.principalOf(r).canSee(c, a.d.CaseOwnership)
}

// loadCase gets the case named in the path, as store.ErrNotFound when the
// request may not see it. It is also where the two #283 rules for a case-scoped
// route are applied, because every such route reads through here:
//
//   - STRICT_AUTH: a request without a verified principal gets errNeedsVerified
//     for any case that is not a pinned demo, the same answer whether the case
//     is the caller's, someone else's or does not exist.
//   - roles: a case the principal may see but their workspace role does not
//     allow this route to touch is errForbidden (403). A hidden case is still
//     ErrNotFound (404). Export decides its recording branch itself (permExport).
func (a *api) loadCase(r *http.Request) (store.Case, error) {
	p := a.principalOf(r)
	strict := a.d.StrictAuth && !p.Verified()
	c, err := a.d.Cases.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		if strict && errors.Is(err, store.ErrNotFound) {
			return store.Case{}, errNeedsVerified
		}
		return store.Case{}, err
	}
	if strict && !c.Pinned {
		return store.Case{}, errNeedsVerified
	}
	if !p.canSee(c, a.d.CaseOwnership) {
		auditHidden(r) // the 404 hides a case that exists: a denial in the audit trail
		return store.Case{}, store.ErrNotFound
	}
	auditConfirm(r, "case", c.ID)
	if need := permOf(r); need == permEdit && !p.allows(c, need) {
		return store.Case{}, errForbidden
	}
	return c, nil
}
