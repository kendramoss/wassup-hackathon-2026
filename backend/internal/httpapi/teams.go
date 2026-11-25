package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/logsafe"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
)

// Teams (D68). With Deps.Teams on, the frontend proxy forwards a signed-in
// user's Google ID token as X-User-Token. The token is verified once per
// request and the user's email and workspace go in the request context. A
// case the user creates is owned by them (a hash of the email, "u_"-prefixed)
// and belongs to their workspace; members of the workspace see it, nobody
// else does (only the admin token). A signed-in user sees the pinned demos,
// their workspace's cases, their own cases, and the anonymous cases made
// from the same browser (X-Visitor-ID). History events record the user's
// email as the actor. Requests without a token are anonymous and keep D58.
// "Their workspace" is one they created or joined: the admin's invitation
// alone does not make anyone a member, so a user's cases are never shared
// with a workspace they did not accept.

// maxUserToken caps X-User-Token (Google ID tokens are about 1 KB).
const maxUserToken = 4096

// userOwnerPrefix marks the owner hash of a signed-in user's case.
const userOwnerPrefix = "u_"

// identity is the signed-in user behind a request.
type identity struct {
	email     string
	owner     string     // userOwner(email)
	workspace string     // workspace ID, or "" for a user without one
	role      teams.Role // the user's role in it, from the stored workspace on this request
}

type userKey struct{}

// identityOf returns the request's signed-in user; the zero identity for an
// anonymous request or with teams off.
func identityOf(ctx context.Context) identity {
	id, _ := ctx.Value(userKey{}).(identity)
	return id
}

// userOwner is the owner hash stored on a signed-in user's case.
func userOwner(email string) string {
	// legacy name kept: the salt is part of every stored hash; changing it orphans existing records (D-rename).
	h := sha256.Sum256([]byte("origin-proof-user:" + email))
	return userOwnerPrefix + hex.EncodeToString(h[:16])
}

// record appends e to c's history with the signed-in user as the actor.
func record(ctx context.Context, c *store.Case, e store.Event) {
	e.Actor = identityOf(ctx).email
	c.Record(e)
}

// withUser verifies X-User-Token, when present, and puts the user in the
// request context. A token that does not verify is 401: the user believes
// they are signed in, so silently treating them as anonymous would file
// their cases outside their workspace.
func (a *api) withUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// MCP callers are an API key or anonymous (docs/MCP.md): the
		// MCP guard drops X-User-Token before the tools run. Dropping the
		// header alone is not enough when this middleware runs first in the
		// outer chain — the verified identity would still be in the context
		// that every in-process tool request inherits (issue #385). Skip
		// verification on /mcp itself; the token is ignored, not refused.
		if r.URL.Path == "/mcp" {
			next.ServeHTTP(w, r)
			return
		}
		tok := strings.TrimSpace(r.Header.Get("X-User-Token"))
		tok = strings.TrimSpace(strings.TrimPrefix(tok, "Bearer "))
		if tok == "" {
			next.ServeHTTP(w, r)
			return
		}
		if len(tok) > maxUserToken {
			a.auditAuthDenied(r)
			unauthenticated(w)
			return
		}
		u, err := a.d.Users.Verify(r.Context(), tok)
		if err != nil {
			a.d.Logger.InfoContext(r.Context(), "sign-in token refused", "request_id", requestRef(r.Context()), "class", string(logsafe.ClassAuthRefused))
			a.auditAuthDenied(r)
			unauthenticated(w)
			return
		}
		id := identity{email: u.Email, owner: userOwner(u.Email)}
		switch ws, err := a.d.Workspaces.Of(r.Context(), u.Email); {
		case err == nil:
			id.workspace = ws.ID
			id.role, _ = ws.RoleOf(u.Email) // joined: Of found them by membership
		case !errors.Is(err, teams.ErrNotFound):
			a.fail(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, id)))
	})
}

func unauthenticated(w http.ResponseWriter) {
	writeError(w, http.StatusUnauthorized, "unauthenticated", "the sign-in token is invalid or expired; sign in again", nil)
}

// requireUser writes 401 unless the request is signed in.
func requireUser(w http.ResponseWriter, r *http.Request) (identity, bool) {
	id := identityOf(r.Context())
	if id.email == "" {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "sign in to continue", nil)
		return identity{}, false
	}
	return id, true
}

// teamCase reports whether c belongs to a signed-in user or a workspace:
// such a case is private to them whatever CASE_OWNERSHIP says, and stays so
// if teams is turned off again.
func teamCase(c store.Case) bool {
	return c.Workspace != "" || strings.HasPrefix(c.Owner, userOwnerPrefix)
}

// workspaceCases adds the user's workspace cases (one ListWorkspace read,
// de-duplicated) to the candidate listing, so a busy deployment does not
// push them out of the list. The caller filters for visibility, then caps.
func (a *api) workspaceCases(ctx context.Context, cs []store.Case) ([]store.Case, error) {
	ws := identityOf(ctx).workspace
	if ws == "" {
		return cs, nil
	}
	more, err := store.ListWorkspace(ctx, a.d.Cases, ws)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(cs))
	for _, c := range cs {
		seen[c.ID] = true
	}
	for _, c := range more {
		if !seen[c.ID] {
			cs = append(cs, c)
		}
	}
	return cs, nil
}

// workspaceDTO is a workspace as returned to its members.
type workspaceDTO struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Members   []string  `json:"members"`
	Invited   []string  `json:"invited"` // pending invitations
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	Admin     bool      `json:"admin"` // the caller's role is admin
	Role      string    `json:"role"`  // the caller's role: viewer, editor or admin
	// MemberRoles maps each joined member's email to their role; sent to
	// admins only (the member emails are already visible to every member).
	MemberRoles map[string]string `json:"member_roles,omitempty"`
}

func workspaceOut(w teams.Workspace, email string) workspaceDTO {
	members, invited := w.Members, w.Invited
	if members == nil {
		members = []string{}
	}
	if invited == nil {
		invited = []string{}
	}
	role, _ := w.RoleOf(email)
	out := workspaceDTO{ID: w.ID, Name: w.Name, Members: members, Invited: invited, CreatedBy: w.CreatedBy, CreatedAt: w.CreatedAt, Admin: role == teams.RoleAdmin, Role: string(role)}
	if out.Admin {
		out.MemberRoles = make(map[string]string, len(w.Members))
		for _, m := range w.Members {
			r, _ := w.RoleOf(m)
			out.MemberRoles[m] = string(r)
		}
	}
	return out
}

// inviteDTO is a pending invitation as the invitee sees it: who invited
// them to what, not the member list (they are not a member yet).
type inviteDTO struct {
	WorkspaceID string    `json:"workspace_id"`
	Name        string    `json:"name"`
	InvitedBy   string    `json:"invited_by,omitempty"` // the admin who sent it; omitted when unknown (an invitation stored before senders were recorded)
	CreatedAt   time.Time `json:"created_at"`
}

// inviter is who sent email's invitation to ws, or "" when that is unknown.
// It never falls back to the workspace creator, who may not be the sender.
func inviter(ws teams.Workspace, email string) string {
	by, _ := ws.InviterOf(email)
	return by
}

// workspaceError maps a teams error to a response; member is the message
// for ErrMember, which means different things to different endpoints.
func (a *api) workspaceError(w http.ResponseWriter, r *http.Request, err error, member string) {
	switch {
	case errors.Is(err, teams.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "no such workspace", nil)
	case errors.Is(err, teams.ErrMember):
		writeError(w, http.StatusConflict, "conflict", member, nil)
	case errors.Is(err, teams.ErrFull):
		writeError(w, http.StatusConflict, "conflict", err.Error(), nil)
	case errors.Is(err, teams.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "only a workspace admin can do that", nil)
	case errors.Is(err, teams.ErrNotMember):
		writeError(w, http.StatusNotFound, "not_found", "no such member", nil)
	case errors.Is(err, teams.ErrLastAdmin):
		writeError(w, http.StatusConflict, "conflict", "a workspace needs at least one admin: make another member an admin first", nil)
	case errors.Is(err, teams.ErrInvalidRole):
		writeError(w, http.StatusBadRequest, "invalid_input", err.Error(), nil)
	default:
		a.fail(w, r, err)
	}
}

const alreadyInOne = "you are already in a workspace (a user can be in one)"

type createWorkspaceRequest struct {
	Name string `json:"name"`
}

// createWorkspace makes a workspace with the caller as its admin and only
// member. A user can be in one workspace.
func (a *api) createWorkspace(w http.ResponseWriter, r *http.Request) {
	id, ok := requireUser(w, r)
	if !ok {
		return
	}
	var in createWorkspaceRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	name := strings.Join(strings.Fields(in.Name), " ")
	if n := utf8.RuneCountInString(name); n == 0 || n > teams.MaxName {
		a.fail(w, r, fmt.Errorf("%w: name must be 1 to %d characters", domain.ErrInvalid, teams.MaxName))
		return
	}
	if id.workspace != "" { // already in one (the store re-checks atomically)
		a.workspaceError(w, r, teams.ErrMember, alreadyInOne)
		return
	}
	ws := teams.Workspace{ID: "ws_" + newID()[:12], Name: name, Members: []string{id.email}, CreatedBy: id.email, CreatedAt: time.Now().UTC(),
		Roles: map[string]string{id.email: string(teams.RoleAdmin)}}
	if err := a.auditBegin(r, auditStep{Tenant: workspaceTenant(ws.ID), Object: objectRef("workspace", ws.ID)}); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.d.Workspaces.Create(r.Context(), ws); err != nil {
		a.workspaceError(w, r, err, alreadyInOne)
		return
	}
	writeJSON(w, http.StatusCreated, workspaceOut(ws, id.email))
}

// myWorkspace returns the signed-in user, their workspace (null if none)
// and their pending invitations.
func (a *api) myWorkspace(w http.ResponseWriter, r *http.Request) {
	id, ok := requireUser(w, r)
	if !ok {
		return
	}
	out := map[string]any{"email": id.email, "workspace": nil}
	if id.workspace != "" {
		ws, err := a.d.Workspaces.Get(r.Context(), id.workspace)
		if err != nil {
			a.workspaceError(w, r, err, alreadyInOne)
			return
		}
		out["workspace"] = workspaceOut(ws, id.email)
	}
	pending, err := a.d.Workspaces.Invites(r.Context(), id.email)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	invites := make([]inviteDTO, 0, len(pending))
	for _, ws := range pending {
		invites = append(invites, inviteDTO{WorkspaceID: ws.ID, Name: ws.Name, InvitedBy: inviter(ws, id.email), CreatedAt: ws.CreatedAt})
	}
	out["invites"] = invites
	writeJSON(w, http.StatusOK, out)
}

type addMemberRequest struct {
	Email string `json:"email"`
}

// addMember invites a user to the workspace; only an admin may.
// The invitee is not a member, and their cases are not shared, until they
// sign in and accept (joinWorkspace). A workspace the caller is not in is a
// 404, so its existence does not leak.
func (a *api) addMember(w http.ResponseWriter, r *http.Request) {
	id, ok := requireUser(w, r)
	if !ok {
		return
	}
	var in addMemberRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	email, ok := teams.NormalEmail(in.Email)
	if !ok {
		a.fail(w, r, fmt.Errorf("%w: email is not a valid address", domain.ErrInvalid))
		return
	}
	wsID := r.PathValue("id")
	if wsID != id.workspace {
		auditHidden(r)
		a.workspaceError(w, r, teams.ErrNotFound, "")
		return
	}
	auditConfirm(r, "workspace", wsID)
	if id.role != teams.RoleAdmin { // the store re-checks this atomically; refusing here keeps a refusal out of the intent/completion pair
		a.workspaceError(w, r, teams.ErrForbidden, "")
		return
	}
	if err := a.auditBegin(r, auditStep{Tenant: workspaceTenant(wsID), Object: objectRef("workspace", wsID), TargetID: email}); err != nil {
		a.fail(w, r, err)
		return
	}
	// The admin check is made by the store on the stored workspace, in the
	// same atomic step as the write: a role changed a moment ago still counts.
	saved, err := a.d.Workspaces.Invite(r.Context(), wsID, id.email, email)
	if err != nil {
		a.workspaceError(w, r, err, "that user is already a member of this workspace")
		return
	}
	writeJSON(w, http.StatusOK, workspaceOut(saved, id.email))
}

// joinWorkspace accepts the caller's own invitation. Without one it is a
// 404, as for a workspace that does not exist; a user already in a
// workspace gets 409. Only cases created after joining belong to the
// workspace: earlier ones stay personal.
func (a *api) joinWorkspace(w http.ResponseWriter, r *http.Request) {
	id, ok := requireUser(w, r)
	if !ok {
		return
	}
	if id.workspace != "" {
		a.workspaceError(w, r, teams.ErrMember, alreadyInOne)
		return
	}
	wsID := r.PathValue("id")
	// Read the target workspace itself. Invites is capped at MaxInvites for
	// the UI, so a 21st pending invitation would otherwise look absent and
	// could never be accepted (#374). A missing or uninvited workspace is
	// still the same 404, with the denial only in the caller's own trail.
	ws, err := a.d.Workspaces.Get(r.Context(), wsID)
	if err != nil && !errors.Is(err, teams.ErrNotFound) {
		a.fail(w, r, err)
		return
	}
	if err != nil || !ws.HasInvite(id.email) {
		auditHidden(r)
		a.workspaceError(w, r, teams.ErrNotFound, "")
		return
	}
	if err := a.auditBegin(r, auditStep{Tenant: workspaceTenant(wsID), Object: objectRef("workspace", wsID)}); err != nil {
		a.fail(w, r, err)
		return
	}
	ws, err = a.d.Workspaces.Join(r.Context(), wsID, id.email)
	if err != nil {
		a.workspaceError(w, r, err, alreadyInOne)
		return
	}
	writeJSON(w, http.StatusOK, workspaceOut(ws, id.email))
}

type setRoleRequest struct {
	Role string `json:"role"`
}

// memberEmail reads and checks the {email} path value.
func (a *api) memberEmail(w http.ResponseWriter, r *http.Request) (string, bool) {
	email, ok := teams.NormalEmail(r.PathValue("email"))
	if !ok {
		a.fail(w, r, fmt.Errorf("%w: email is not a valid address", domain.ErrInvalid))
	}
	return email, ok
}

// setMemberRole changes a joined member's role (PUT
// /v1/workspaces/{id}/members/{email}/role). The workspace must be the
// caller's own (else 404, as for an unknown one); the store checks that the
// caller is an admin and that an admin remains, in one atomic step.
func (a *api) setMemberRole(w http.ResponseWriter, r *http.Request) {
	id, ok := requireUser(w, r)
	if !ok {
		return
	}
	target, ok := a.memberEmail(w, r)
	if !ok {
		return
	}
	var in setRoleRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	role, valid := teams.ParseRole(in.Role)
	if !valid {
		a.fail(w, r, fmt.Errorf("%w: role must be viewer, editor or admin", domain.ErrInvalid))
		return
	}
	wsID := r.PathValue("id")
	if wsID != id.workspace {
		auditHidden(r)
		a.workspaceError(w, r, teams.ErrNotFound, "")
		return
	}
	auditConfirm(r, "workspace", wsID)
	if id.role != teams.RoleAdmin { // the store re-checks this atomically
		a.workspaceError(w, r, teams.ErrForbidden, "")
		return
	}
	if err := a.auditBegin(r, auditStep{Tenant: workspaceTenant(wsID), Object: objectRef("workspace", wsID), Detail: string(role), TargetID: target}); err != nil {
		a.fail(w, r, err)
		return
	}
	saved, err := a.d.Workspaces.SetRole(r.Context(), wsID, id.email, target, role)
	if err != nil {
		a.workspaceError(w, r, err, "")
		return
	}
	writeJSON(w, http.StatusOK, workspaceOut(saved, id.email))
}

// removeMember removes a member (DELETE /v1/workspaces/{id}/members/{email}):
// an admin may remove anyone, a member may remove themselves (leave). The
// removed user keeps their personal cases but loses the workspace's, including
// ones they made: those belong to the workspace.
func (a *api) removeMember(w http.ResponseWriter, r *http.Request) {
	id, ok := requireUser(w, r)
	if !ok {
		return
	}
	target, ok := a.memberEmail(w, r)
	if !ok {
		return
	}
	wsID := r.PathValue("id")
	if wsID != id.workspace {
		auditHidden(r)
		a.workspaceError(w, r, teams.ErrNotFound, "")
		return
	}
	auditConfirm(r, "workspace", wsID)
	step := auditStep{Tenant: workspaceTenant(wsID), Object: objectRef("workspace", wsID), TargetID: target}
	if target == id.email {
		step.Action = audit.WorkspaceLeave
	} else if id.role != teams.RoleAdmin { // an admin may remove anyone, a member only themselves; the store re-checks atomically
		a.workspaceError(w, r, teams.ErrForbidden, "")
		return
	}
	if err := a.auditBegin(r, step); err != nil {
		a.fail(w, r, err)
		return
	}
	saved, err := a.d.Workspaces.Remove(r.Context(), wsID, id.email, target)
	if err != nil {
		a.workspaceError(w, r, err, "")
		return
	}
	var view any // the caller's updated view; null once they have left
	if saved.HasMember(id.email) {
		view = workspaceOut(saved, id.email)
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": target, "workspace": view})
}
