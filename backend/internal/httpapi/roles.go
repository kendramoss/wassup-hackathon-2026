package httpapi

import (
	"errors"
	"net/http"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
)

// Workspace roles and strict auth (#283). See docs/design/roles-and-strict-auth.md.
//
// A signed-in user in a workspace has one role there (viewer, editor, admin),
// read from the stored workspace on every request by withUser: never from a
// header, a body field or a token claim. The role governs the cases that
// carry the user's own workspace ID. It does not touch: a pinned demo (demo
// rules apply to everyone), a user's own pre-workspace or anonymous-browser
// cases (owner behaviour), a case of another workspace or another owner
// (hidden: 404), or any request that is an admin token or has no signed-in
// user. An API key never has a role. The admin token wins over a user token:
// its holder keeps the existing admin behaviour whatever the user's role.

// perm is what a case-scoped request needs.
type perm int

const (
	// permRead changes nothing stored: any member of the workspace may.
	permRead perm = iota
	// permEdit creates or changes a case, or records history on it (even a
	// question to Ask appends an event): editor or admin.
	permEdit
	// permExport is export: preview:true is a read, anything else records an
	// "exported" event and sets ExportedAt, so it is permEdit. The handler
	// decides after it has parsed the typed body (see export).
	permExport
)

// rolePerms is the permission table: one row per case-scoped or case-creating
// route, keyed by the mux pattern. A route missing from it is treated as
// permEdit, so a new route is closed to viewers until someone decides; the
// route-table test fails until it has a row.
var rolePerms = map[string]perm{
	"GET /v1/cases":                            permRead,
	"GET /v1/cases/{id}":                       permRead,
	"POST /v1/cases/{id}/plan/estimate":        permRead, // reads caches only: no model call, no write
	"POST /v1/cases/{id}/export":               permExport,
	"POST /v1/cases/predict":                   permEdit,
	"POST /v1/cases/batch":                     permEdit,
	"POST /v1/cases/{id}/bom-version":          permEdit,
	"POST /v1/cases/{id}/scenarios":            permEdit,
	"POST /v1/cases/{id}/compare-targets":      permEdit,
	"POST /v1/cases/{id}/ask":                  permEdit,
	"PUT /v1/cases/{id}/economics":             permEdit,
	"POST /v1/cases/{id}/watch":                permEdit,
	"DELETE /v1/cases/{id}":                    permEdit,
	"PATCH /v1/cases/{id}":                     permEdit,
	"POST /v1/cases/{id}/plan":                 permEdit,
	"POST /v1/cases/{id}/answers":              permEdit,
	"PUT /v1/cases/{id}/origin-facts/{market}": permEdit, // saves supplied facts and records an event (D801)
	"PUT /v1/cases/{id}/family-facts":          permEdit, // saves answers to the family questions and records an event (D810)
	"GET /v1/cases/{id}/evidence-report":       permRead, // runs rules checks and reads the stored answer; stores nothing (D803)
	"GET /v1/cases/{id}/reviews":               permRead, // the owner's view of the links: status and comments
	"POST /v1/cases/{id}/reviews":              permEdit, // shares case content outside the workspace
	"DELETE /v1/cases/{id}/reviews/{review}":   permEdit,
	// Broker workflow (BROKER_WORKFLOW): reading the queue, discussion and
	// package is a read; any member may take part in the discussion;
	// queueing, deciding, sign-offs, roles and filing are edits.
	"GET /v1/queue":                        permRead,
	"PUT /v1/cases/{id}/queue":             permEdit,
	"POST /v1/cases/{id}/queue/transition": permEdit,
	"GET /v1/cases/{id}/collab":            permRead,
	"POST /v1/cases/{id}/comments":         permRead, // a viewer may comment: it changes only the discussion
	"POST /v1/cases/{id}/approvals":        permEdit,
	"PUT /v1/cases/{id}/collaborators":     permEdit,
	"POST /v1/cases/{id}/filing":           permEdit,
	"GET /v1/cases/{id}/filing":            permRead,
	"POST /v1/cases/{id}/filing/status":    permEdit,
}

// permOf is what the matched route needs; unknown means permEdit.
func permOf(r *http.Request) perm {
	if p, ok := rolePerms[r.Pattern]; ok {
		return p
	}
	return permEdit
}

var (
	errForbidden       = errors.New("forbidden")
	errNeedsVerified   = errors.New("needs a verified principal")
	forbiddenMessage   = "your role in this workspace does not allow this; ask a workspace admin for editor access"
	needsVerifiedError = "sign in or use an API key; a visitor id alone does not authenticate (STRICT_AUTH is on)"
)

// roleGoverns reports whether the principal's workspace role decides access to
// c: a signed-in, non-admin-token user, a workspace case of their own
// workspace, not a pinned demo.
func (p principal) roleGoverns(c store.Case) bool {
	return !p.Admin && p.signedIn() && p.Workspace != "" && c.Workspace == p.Workspace && !c.Pinned
}

// allows reports whether the principal may do need to c. Where the role does
// not govern, the answer is yes: visibility (canSee) and the demo rules decide.
// A user in a workspace with no role on the principal is a viewer (fail closed).
func (p principal) allows(c store.Case, need perm) bool {
	if !p.roleGoverns(c) {
		return true
	}
	return roleAllows(p.Role, need)
}

func roleAllows(role teams.Role, need perm) bool {
	switch role {
	case teams.RoleAdmin, teams.RoleEditor:
		return true
	}
	return need == permRead // viewer, and anything unrecognised
}

// mayCreate refuses, with the response written, a request that may not create
// cases: STRICT_AUTH without a verified principal (401), or a viewer (403).
// Creating always files the case under the user's workspace, so a viewer would
// otherwise write into it.
func (a *api) mayCreate(w http.ResponseWriter, r *http.Request) bool {
	p := a.principalOf(r)
	switch {
	case a.d.StrictAuth && !p.Verified():
		a.fail(w, r, errNeedsVerified)
	case p.signedIn() && p.Workspace != "" && !p.Admin && !roleAllows(p.Role, permEdit):
		a.fail(w, r, errForbidden)
	default:
		return true
	}
	return false
}
