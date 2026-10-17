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

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
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
	owner     string // userOwner(email)
	workspace string // workspace ID, or "" for a user without one
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
		tok := strings.TrimSpace(r.Header.Get("X-User-Token"))
		tok = strings.TrimSpace(strings.TrimPrefix(tok, "Bearer "))
		if tok == "" {
			next.ServeHTTP(w, r)
			return
		}
		if len(tok) > maxUserToken {
			unauthenticated(w)
			return
		}
		u, err := a.d.Users.Verify(r.Context(), tok)
		if err != nil {
			a.d.Logger.InfoContext(r.Context(), "sign-in token refused", "request_id", RequestID(r.Context()), "err", err)
			unauthenticated(w)
			return
		}
		id := identity{email: u.Email, owner: userOwner(u.Email)}
		switch ws, err := a.d.Workspaces.Of(r.Context(), u.Email); {
		case err == nil:
			id.workspace = ws.ID
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
		writeError(w, http.StatusUnauthorized, "unauthenticated", "sign in to use workspaces", nil)
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

// teamVisible reports whether a signed-in user may see c (not pinned, no
// admin token): their workspace's cases, their own, and the anonymous cases
// from the same browser.
func teamVisible(r *http.Request, id identity, c store.Case) bool {
	switch {
	case c.Workspace != "" && c.Workspace == id.workspace:
		return true
	case c.Owner == id.owner:
		return true
	case teamCase(c):
		return false
	}
	o := ownerOf(r)
	return o != "" && o == c.Owner
}

// workspaceCases adds the user's workspace cases to a listing already capped
// at the newest others, so a busy deployment does not push them out of the
// list. The caller caps again after filtering for visibility.
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
	Admin     bool      `json:"admin"` // the caller administers it
}

func workspaceOut(w teams.Workspace, email string) workspaceDTO {
	members, invited := w.Members, w.Invited
	if members == nil {
		members = []string{}
	}
	if invited == nil {
		invited = []string{}
	}
	return workspaceDTO{ID: w.ID, Name: w.Name, Members: members, Invited: invited, CreatedBy: w.CreatedBy, CreatedAt: w.CreatedAt, Admin: w.IsAdmin(email)}
}

// inviteDTO is a pending invitation as the invitee sees it: who invited
// them to what, not the member list (they are not a member yet).
type inviteDTO struct {
	WorkspaceID string    `json:"workspace_id"`
	Name        string    `json:"name"`
	InvitedBy   string    `json:"invited_by"`
	CreatedAt   time.Time `json:"created_at"`
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
	ws := teams.Workspace{ID: "ws_" + newID()[:12], Name: name, Members: []string{id.email}, CreatedBy: id.email, CreatedAt: time.Now().UTC()}
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
		invites = append(invites, inviteDTO{WorkspaceID: ws.ID, Name: ws.Name, InvitedBy: ws.CreatedBy, CreatedAt: ws.CreatedAt})
	}
	out["invites"] = invites
	writeJSON(w, http.StatusOK, out)
}

type addMemberRequest struct {
	Email string `json:"email"`
}

// addMember invites a user to the workspace; only its admin (creator) may.
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
		a.workspaceError(w, r, teams.ErrNotFound, "")
		return
	}
	ws, err := a.d.Workspaces.Get(r.Context(), wsID)
	if err != nil {
		a.workspaceError(w, r, err, "")
		return
	}
	if !ws.IsAdmin(id.email) {
		writeError(w, http.StatusForbidden, "forbidden", "only the workspace's admin can invite members", nil)
		return
	}
	saved, err := a.d.Workspaces.Invite(r.Context(), wsID, email)
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
	ws, err := a.d.Workspaces.Join(r.Context(), r.PathValue("id"), id.email)
	if err != nil {
		a.workspaceError(w, r, err, alreadyInOne)
		return
	}
	writeJSON(w, http.StatusOK, workspaceOut(ws, id.email))
}
