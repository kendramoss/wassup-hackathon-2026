package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"regexp"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/apikey"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
)

// The request principal (#273).
//
// Who a request acts as is decided in ONE place: principalOf. Everything that
// asks "whose case is this", "may this request see that case" or "which owner
// do new cases get" reads the principal; nothing else looks at X-Visitor-ID,
// the admin token, the API key context or the signed-in user.
//
// What goes into a principal, and how far it can be trusted:
//
//   - Admin: the X-Admin-Token header, compared in constant time with the
//     configured token. Verified.
//   - User: the signed-in identity (email, user owner hash, workspace) that
//     the withUser middleware put in the request context after verifying
//     X-User-Token. Verified. Without teams there is no user.
//   - API key: the key the withAPIKeys middleware authenticated and put in
//     the request context. Verified.
//   - Visitor: a syntactically valid X-Visitor-ID, hashed with the
//     origin-proof-visitor salt. NOT verified: it is a bearer secret the
//     browser proxy keeps in a cookie, so whoever holds the ID is that
//     visitor (see docs/design/case-isolation.md).
//
// Anything else a client sends (X-Tenant, X-Workspace, X-Role, X-Owner,
// X-User-Email, ...) is never read. A verified credential that fails
// verification does not reach here at all: the middleware answers 401.
//
// The owner hash (Owner) keeps the exact meaning ownerOf always had: the API
// key's owner when a key is present, else the visitor hash, else "". It is
// what an anonymous or broker case is stored under, and it is also read for a
// signed-in user (their browser's anonymous cases). Changing a salt or prefix
// orphans stored data, so they stay as they are.
//
// Roles (#283): Role is set from the verified user's stored workspace record
// only (roles.go says what it governs). Kind and Verified are unchanged.

type principalKind string

const (
	kindNone    principalKind = "none"    // no usable credential
	kindVisitor principalKind = "visitor" // a valid X-Visitor-ID only
	kindAPIKey  principalKind = "apikey"
	kindUser    principalKind = "user"
	kindAdmin   principalKind = "admin"
)

type principal struct {
	// Kind is the strongest credential: admin, then user, then API key, then
	// visitor, else none. Visibility does not depend on Kind alone (an admin
	// with a visitor ID still has that Owner); it is for callers that need to
	// ask what the request is.
	Kind  principalKind
	Admin bool
	// Owner is the credential owner hash: the API key's, else the visitor's,
	// else "". Same value as ownerOf(r).
	Owner string
	// HasKey reports an authenticated API key (KeyID is its ID).
	HasKey bool
	KeyID  string
	// KeyWorkspace is the workspace a workspace API key was issued for (#870);
	// its audit events go to that workspace's trail.
	KeyWorkspace string
	// KeyRole is the role a workspace key was made with (#870, D961): viewer
	// or editor, "" for a key without one. It comes from the stored key, never
	// from the request.
	KeyRole teams.Role
	// Email, UserOwner and Workspace are set for a verified signed-in user.
	Email     string
	UserOwner string
	Workspace string
	// Role is the signed-in user's role in Workspace (#283), as stored in the
	// workspace record on this request; "" without a workspace, and for an
	// API key, a visitor or the admin token. It is never read from the
	// request.
	Role teams.Role
}

var visitorIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)

// visitorOwnerHash is the owner hash for a visitor ID: "" when it is not a valid
// ID. The salt is part of every stored hash.
func visitorOwnerHash(id string) string {
	if !visitorIDPattern.MatchString(id) {
		return ""
	}
	// legacy name kept: the salt is part of every stored hash; changing it orphans existing records (D-rename).
	h := sha256.Sum256([]byte("origin-proof-visitor:" + id))
	return hex.EncodeToString(h[:16])
}

// principalOf resolves the request's principal from the credentials that are
// already verified (context) and the admin and visitor headers.
func (a *api) principalOf(r *http.Request) principal {
	p := principal{Admin: a.isAdmin(r), Owner: ownerOf(r)}
	if k, ok := keyFrom(r.Context()); ok {
		p.HasKey, p.KeyID, p.KeyWorkspace = true, k.ID, k.Workspace
		if k.Workspace != "" {
			switch k.Role {
			case apikey.RoleViewer:
				p.KeyRole = teams.RoleViewer
			case apikey.RoleEditor:
				p.KeyRole = teams.RoleEditor
			}
		}
	}
	if id := identityOf(r.Context()); id.email != "" {
		p.Email, p.UserOwner, p.Workspace, p.Role = id.email, id.owner, id.workspace, id.role
	}
	switch {
	case p.Admin:
		p.Kind = kindAdmin
	case p.Email != "":
		p.Kind = kindUser
	case p.HasKey:
		p.Kind = kindAPIKey
	case p.Owner != "":
		p.Kind = kindVisitor
	default:
		p.Kind = kindNone
	}
	return p
}

// Verified reports whether the principal rests on a credential the server
// checked (admin token, signed-in user, API key), not on a bare visitor ID.
func (p principal) Verified() bool { return p.Admin || p.Email != "" || p.HasKey }

// signedIn reports a verified signed-in user.
func (p principal) signedIn() bool { return p.Email != "" }

// wsScope is the workspace whose cases this principal acts on and the role it
// has there: a signed-in user's workspace and role, or, for a request with no
// signed-in user and not the admin token, the workspace of a workspace API key
// that was made with a role (viewer or editor, never admin). Empty when it has
// none. A key without a role, an operator key and a visitor have no workspace.
func (p principal) wsScope() (ws string, role teams.Role) {
	switch {
	case p.signedIn():
		return p.Workspace, p.Role
	case p.HasKey && !p.Admin && p.KeyWorkspace != "" && p.KeyRole != "":
		return p.KeyWorkspace, p.KeyRole
	}
	return "", ""
}

// newCaseScope is the owner and workspace a case made by this principal is
// stored under: a signed-in user's, else a workspace key's own owner in its
// workspace, else the credential owner.
func (p principal) newCaseScope() (owner, workspace string) {
	if p.signedIn() {
		return p.UserOwner, p.Workspace
	}
	if ws, _ := p.wsScope(); ws != "" {
		return p.Owner, ws
	}
	return p.Owner, ""
}

// listOwners are the owner hashes whose cases a listing reads for this
// principal (workspace cases are added separately).
func (p principal) listOwners() []string {
	if p.signedIn() {
		return []string{p.UserOwner, p.Owner}
	}
	return []string{p.Owner}
}

// ownerScoped reports whether the principal's view is its own cases plus the
// pinned demos, rather than the public-demo mode (ownership off, anonymous,
// no key) where anonymous cases are shared.
func (p principal) ownerScoped(ownership bool) bool {
	return ownership || p.signedIn() || p.HasKey
}

// canSee reports whether the principal may see case c, with the deployment's
// CASE_OWNERSHIP setting. Pinned demos and the admin: always. A signed-in
// user: their workspace's cases, their own, and the anonymous cases of their
// browser. Otherwise a team case is hidden; with ownership off and no key an
// anonymous case is shared (but a broker's case never is, D69); else only the
// credential owner's cases.
func (p principal) canSee(c store.Case, ownership bool) bool {
	if c.Pinned || p.Admin {
		return true
	}
	if p.signedIn() {
		return p.teamCanSee(c)
	}
	if ws, _ := p.wsScope(); ws != "" && c.Workspace == ws {
		return true // a workspace key sees its workspace's cases, no other (D961)
	}
	if teamCase(c) {
		return false
	}
	if !p.ownerScoped(ownership) && !strings.HasPrefix(c.Owner, keyOwnerPrefix) {
		return true
	}
	return p.Owner != "" && p.Owner == c.Owner
}

// teamCanSee is canSee for a signed-in user (not pinned, not admin).
func (p principal) teamCanSee(c store.Case) bool {
	switch {
	case c.Workspace != "" && c.Workspace == p.Workspace:
		return true
	case c.Owner == p.UserOwner && (c.Workspace == "" || c.Workspace == p.Workspace):
		// Their own, unless it belongs to a workspace they are no longer in
		// (removed or left): a workspace case is the workspace's (#283).
		return true
	case teamCase(c):
		return false
	}
	return p.Owner != "" && p.Owner == c.Owner
}
