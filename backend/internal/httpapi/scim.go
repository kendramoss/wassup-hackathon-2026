package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/logsafe"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/scim"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/sso"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
)

// SCIM provisioning (#869, docs/design/scim.md). Off unless Deps.SCIM is set
// (SCIM=on, which needs SSO=on). The token routes need a signed-in admin of
// the workspace; the /v1/scim/v2 routes need the workspace's SCIM token.

const (
	scimTokenHeader = "X-SCIM-Token"
	scimMaxBody     = 64 << 10
)

// scimError writes a SCIM error (RFC 7644 3.12).
func scimError(w http.ResponseWriter, status int, scimType, detail string) {
	body := map[string]any{"schemas": []string{scim.SchemaError}, "status": strconv.Itoa(status), "detail": detail}
	if scimType != "" {
		body["scimType"] = scimType
	}
	scimJSON(w, status, body)
}

func scimJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", scim.ContentType)
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

// scimSecret is the token the request carries: X-SCIM-Token (what the
// frontend proxy forwards), else an Authorization bearer that is a SCIM token
// (a direct call outside the gateway).
func scimSecret(r *http.Request) string {
	if s := strings.TrimSpace(r.Header.Get(scimTokenHeader)); s != "" {
		return s
	}
	scheme, tok, ok := strings.Cut(strings.TrimSpace(r.Header.Get("Authorization")), " ")
	if tok = strings.TrimSpace(tok); ok && strings.EqualFold(scheme, "Bearer") && strings.HasPrefix(tok, scim.TokenPrefix) {
		return tok
	}
	return ""
}

// scimAuth verifies the SCIM token. A client that has failed too often waits
// before any lookup; a malformed token is refused without one.
func (a *api) scimAuth(w http.ResponseWriter, r *http.Request) (scim.Token, bool) {
	refuse := func() {}
	if a.scimFails != nil {
		client := a.clientKey(r)
		release, ok, wait := a.scimFails.begin(client)
		if !ok {
			secs := max(1, int(math.Ceil(wait.Seconds())))
			w.Header().Set("Retry-After", fmt.Sprint(secs))
			scimError(w, http.StatusTooManyRequests, "", fmt.Sprintf("too many invalid SCIM tokens from this client; try again in %d s", secs))
			return scim.Token{}, false
		}
		defer release()
		refuse = func() { a.scimFails.fail(client) }
	}
	secret := scimSecret(r)
	if !scim.WellFormed(secret) {
		refuse()
		scimError(w, http.StatusUnauthorized, "", "a valid SCIM token is required")
		return scim.Token{}, false
	}
	return a.scimLookup(w, r, secret, refuse)
}

func (a *api) scimLookup(w http.ResponseWriter, r *http.Request, secret string, refuse func()) (scim.Token, bool) {
	t, err := a.d.SCIM.ByHash(r.Context(), scim.Hash(secret))
	if errors.Is(err, scim.ErrNotFound) {
		refuse()
		scimError(w, http.StatusUnauthorized, "", "a valid SCIM token is required")
		return scim.Token{}, false
	}
	if err != nil {
		a.scimFail(w, r, err)
		return scim.Token{}, false
	}
	auditAsSystem(r, t.Workspace)
	auditConfirm(r, "workspace", t.Workspace)
	return t, true
}

func (a *api) scimFail(w http.ResponseWriter, r *http.Request, err error) {
	a.d.Logger.ErrorContext(r.Context(), "scim error", "request_id", requestRef(r.Context()), "err", logsafe.Err(err))
	scimError(w, http.StatusInternalServerError, "", "the request could not be completed; try again")
}

func (a *api) scimBase() string { return a.d.SSO.SiteURL() + "/api/v1/scim/v2" }

// scimMember returns the member with this SCIM id that the token manages.
func scimMember(t scim.Token, ws teams.Workspace, uid string) (string, teams.Role, bool) {
	for _, e := range ws.Members {
		if t.Manages(e) && scim.UserID(ws.ID, e) == uid {
			r, _ := ws.RoleOf(e)
			return e, r, true
		}
	}
	return "", "", false
}

func (a *api) scimWorkspace(w http.ResponseWriter, r *http.Request, t scim.Token) (teams.Workspace, bool) {
	ws, err := a.d.Workspaces.Get(r.Context(), t.Workspace)
	if err != nil {
		a.scimFail(w, r, err)
		return teams.Workspace{}, false
	}
	return ws, true
}

func (a *api) scimUser(ws teams.Workspace, email string, role teams.Role) scim.User {
	return scim.NewUser(ws.ID, email, string(role), a.scimBase()+"/Users")
}

func scimDecode(w http.ResponseWriter, r *http.Request, v any) error {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, scimMaxBody))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

// listSCIMUsers answers GET /Users, with the one filter identity providers
// send (userName eq "...").
func (a *api) listSCIMUsers(w http.ResponseWriter, r *http.Request) {
	t, ok := a.scimAuth(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	email, all, err := scim.ParseFilter(q.Get("filter"))
	if err != nil {
		scimError(w, http.StatusBadRequest, "invalidFilter", err.Error())
		return
	}
	start, count, err := scim.Page(q.Get("startIndex"), q.Get("count"))
	if err != nil {
		scimError(w, http.StatusBadRequest, "invalidValue", err.Error())
		return
	}
	ws, ok := a.scimWorkspace(w, r, t)
	if !ok {
		return
	}
	var users []scim.User
	for _, e := range ws.Members {
		if !t.Manages(e) || !all && e != email {
			continue
		}
		role, _ := ws.RoleOf(e)
		users = append(users, a.scimUser(ws, e, role))
	}
	total := len(users)
	page := users[min(start-1, total):min(start-1+count, total)]
	if page == nil {
		page = []scim.User{}
	}
	scimJSON(w, http.StatusOK, map[string]any{"schemas": []string{scim.SchemaList}, "totalResults": total, "startIndex": start,
		"itemsPerPage": len(page), "Resources": page})
}

func (a *api) getSCIMUser(w http.ResponseWriter, r *http.Request) {
	t, ok := a.scimAuth(w, r)
	if !ok {
		return
	}
	ws, ok := a.scimWorkspace(w, r, t)
	if !ok {
		return
	}
	email, role, ok := scimMember(t, ws, r.PathValue("uid"))
	if !ok {
		scimError(w, http.StatusNotFound, "", "no such user")
		return
	}
	scimJSON(w, http.StatusOK, a.scimUser(ws, email, role))
}

// scimRole is the role a request asks for, checked against what the token
// may grant: admin only with allow_admin.
func scimRole(t scim.Token, asked string) (teams.Role, error) {
	if asked == "" {
		asked = t.DefaultRole
	}
	role, ok := teams.ParseRole(asked)
	if !ok {
		return "", scim.ErrRole
	}
	if role == teams.RoleAdmin && !t.AllowAdmin {
		return "", errAdminNotAllowed
	}
	return role, nil
}

var errAdminNotAllowed = errors.New("this SCIM token cannot grant, change or remove the admin role; a workspace admin can reissue it with allow_admin")

// createSCIMUser adds a member. Someone in another workspace is a 409 that
// does not say which.
func (a *api) createSCIMUser(w http.ResponseWriter, r *http.Request) {
	t, ok := a.scimAuth(w, r)
	if !ok {
		return
	}
	var in scim.UserInput
	if err := scimDecode(w, r, &in); err != nil {
		scimError(w, http.StatusBadRequest, "invalidSyntax", "the body is not a SCIM User")
		return
	}
	email := in.Email()
	if email == "" {
		scimError(w, http.StatusBadRequest, "invalidValue", "userName must be an email address")
		return
	}
	if !t.Manages(email) {
		scimError(w, http.StatusForbidden, "", "this SCIM token does not manage that email domain")
		return
	}
	if in.Active != nil && !*in.Active {
		scimError(w, http.StatusBadRequest, "invalidValue", "a user is created active; deactivate removes the member")
		return
	}
	asked, err := scim.ParseRoles(in.Roles)
	if err != nil {
		scimError(w, http.StatusBadRequest, "invalidValue", err.Error())
		return
	}
	role, err := scimRole(t, asked)
	if errors.Is(err, errAdminNotAllowed) {
		scimError(w, http.StatusForbidden, "", err.Error())
		return
	}
	if err != nil {
		scimError(w, http.StatusBadRequest, "invalidValue", err.Error())
		return
	}
	ws, ok := a.scimWorkspace(w, r, t)
	if !ok {
		return
	}
	if ws.HasMember(email) {
		scimError(w, http.StatusConflict, "uniqueness", "the user is already a member")
		return
	}
	if err := a.auditBegin(r, auditStep{Tenant: workspaceTenant(ws.ID), Object: objectRef("workspace", ws.ID), Detail: string(role), TargetID: email}); err != nil {
		a.fail(w, r, err)
		return
	}
	ws, err = a.d.Workspaces.(teams.Provisioner).Provision(r.Context(), ws.ID, email, role)
	switch {
	case errors.Is(err, teams.ErrMember):
		scimError(w, http.StatusConflict, "uniqueness", "the user cannot be added to this workspace")
		return
	case errors.Is(err, teams.ErrFull):
		scimError(w, http.StatusConflict, "", "the workspace is full")
		return
	case err != nil:
		a.scimFail(w, r, err)
		return
	}
	got, _ := ws.RoleOf(email)
	scimJSON(w, http.StatusCreated, a.scimUser(ws, email, got))
}

// replaceSCIMUser (PUT) sets active and roles; userName cannot change.
func (a *api) replaceSCIMUser(w http.ResponseWriter, r *http.Request) {
	var in scim.UserInput
	if err := scimDecode(w, r, &in); err != nil {
		scimError(w, http.StatusBadRequest, "invalidSyntax", "the body is not a SCIM User")
		return
	}
	role, err := scim.ParseRoles(in.Roles)
	if err != nil {
		scimError(w, http.StatusBadRequest, "invalidValue", err.Error())
		return
	}
	a.changeSCIMUser(w, r, scim.Change{Active: in.Active, Role: role}, in.Email())
}

// patchSCIMUser (PATCH) changes active or roles.
func (a *api) patchSCIMUser(w http.ResponseWriter, r *http.Request) {
	var in scim.PatchRequest
	if err := scimDecode(w, r, &in); err != nil {
		scimError(w, http.StatusBadRequest, "invalidSyntax", "the body is not a SCIM PatchOp")
		return
	}
	c, err := scim.ParsePatch(in)
	if err != nil {
		scimError(w, http.StatusBadRequest, "invalidValue", err.Error())
		return
	}
	a.changeSCIMUser(w, r, c, "")
}

// changeSCIMUser applies a change: active false removes the member, a role
// sets it. sameEmail, when given (PUT), must be the member's.
func (a *api) changeSCIMUser(w http.ResponseWriter, r *http.Request, c scim.Change, sameEmail string) {
	t, ok := a.scimAuth(w, r)
	if !ok {
		return
	}
	ws, ok := a.scimWorkspace(w, r, t)
	if !ok {
		return
	}
	email, cur, ok := scimMember(t, ws, r.PathValue("uid"))
	if !ok {
		scimError(w, http.StatusNotFound, "", "no such user")
		return
	}
	if sameEmail != "" && sameEmail != email {
		scimError(w, http.StatusBadRequest, "mutability", "userName cannot be changed")
		return
	}
	if c.Active != nil && !*c.Active {
		a.removeSCIMMember(w, r, t, ws, email, cur, "deactivate")
		return
	}
	if c.Role == "" || teams.Role(c.Role) == cur {
		scimJSON(w, http.StatusOK, a.scimUser(ws, email, cur))
		return
	}
	role, err := scimRole(t, c.Role)
	if err == nil && cur == teams.RoleAdmin && !t.AllowAdmin {
		err = errAdminNotAllowed
	}
	if errors.Is(err, errAdminNotAllowed) {
		scimError(w, http.StatusForbidden, "", err.Error())
		return
	}
	if err != nil {
		scimError(w, http.StatusBadRequest, "invalidValue", err.Error())
		return
	}
	if err := a.auditBegin(r, auditStep{Tenant: workspaceTenant(ws.ID), Object: objectRef("workspace", ws.ID), Action: audit.SCIMUserUpdate, Detail: string(role), TargetID: email}); err != nil {
		a.fail(w, r, err)
		return
	}
	ws, err = a.d.Workspaces.(teams.Provisioner).SetProvisionedRole(r.Context(), ws.ID, email, role)
	if a.scimTeamsError(w, r, err) {
		return
	}
	scimJSON(w, http.StatusOK, a.scimUser(ws, email, role))
}

func (a *api) deleteSCIMUser(w http.ResponseWriter, r *http.Request) {
	t, ok := a.scimAuth(w, r)
	if !ok {
		return
	}
	ws, ok := a.scimWorkspace(w, r, t)
	if !ok {
		return
	}
	email, cur, ok := scimMember(t, ws, r.PathValue("uid"))
	if !ok {
		scimError(w, http.StatusNotFound, "", "no such user")
		return
	}
	a.removeSCIMMember(w, r, t, ws, email, cur, "")
}

func (a *api) removeSCIMMember(w http.ResponseWriter, r *http.Request, t scim.Token, ws teams.Workspace, email string, cur teams.Role, detail string) {
	if cur == teams.RoleAdmin && !t.AllowAdmin {
		scimError(w, http.StatusForbidden, "", errAdminNotAllowed.Error())
		return
	}
	if err := a.auditBegin(r, auditStep{Tenant: workspaceTenant(ws.ID), Object: objectRef("workspace", ws.ID), Action: audit.SCIMUserDelete, Detail: detail, TargetID: email}); err != nil {
		a.fail(w, r, err)
		return
	}
	_, err := a.d.Workspaces.(teams.Provisioner).Deprovision(r.Context(), ws.ID, email)
	if a.scimTeamsError(w, r, err) {
		return
	}
	if detail == "deactivate" {
		u := a.scimUser(ws, email, cur)
		u.Active, u.Roles = false, []scim.Value{}
		scimJSON(w, http.StatusOK, u)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) scimTeamsError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, teams.ErrNotMember), errors.Is(err, teams.ErrNotFound):
		scimError(w, http.StatusNotFound, "", "no such user")
	case errors.Is(err, teams.ErrLastAdmin):
		scimError(w, http.StatusConflict, "", "a workspace needs at least one admin")
	case errors.Is(err, teams.ErrInvalidRole):
		scimError(w, http.StatusBadRequest, "invalidValue", err.Error())
	default:
		a.scimFail(w, r, err)
	}
	return true
}

// The discovery documents (RFC 7643 5, 6, 7), for identity providers that
// read them.
func (a *api) scimServiceProviderConfig(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.scimAuth(w, r); !ok {
		return
	}
	no := map[string]bool{"supported": false}
	scimJSON(w, http.StatusOK, map[string]any{
		"schemas": []string{scim.SchemaSPConfig}, "patch": map[string]bool{"supported": true}, "bulk": map[string]any{"supported": false, "maxOperations": 0, "maxPayloadSize": 0},
		"filter": map[string]any{"supported": true, "maxResults": scim.MaxPage}, "changePassword": no, "sort": no, "etag": no,
		"authenticationSchemes": []map[string]any{{"type": "oauthbearertoken", "name": "SCIM token", "description": "The workspace's SCIM token, issued by a workspace admin.", "primary": true}},
	})
}

func (a *api) scimResourceTypes(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.scimAuth(w, r); !ok {
		return
	}
	scimJSON(w, http.StatusOK, map[string]any{"schemas": []string{scim.SchemaList}, "totalResults": 1, "startIndex": 1, "itemsPerPage": 1,
		"Resources": []map[string]any{{"schemas": []string{scim.SchemaResType}, "id": "User", "name": "User", "endpoint": "/Users", "schema": scim.SchemaUser}}})
}

func (a *api) scimSchemas(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.scimAuth(w, r); !ok {
		return
	}
	attr := func(name, typ string, multi, required bool, uniq string) map[string]any {
		return map[string]any{"name": name, "type": typ, "multiValued": multi, "required": required, "caseExact": false, "mutability": "readWrite", "returned": "default", "uniqueness": uniq}
	}
	user := map[string]any{"schemas": []string{scim.SchemaSchema}, "id": scim.SchemaUser, "name": "User", "attributes": []map[string]any{
		attr("userName", "string", false, true, "server"), attr("active", "boolean", false, false, "none"),
		attr("emails", "complex", true, false, "none"), attr("roles", "complex", true, false, "none"),
	}}
	scimJSON(w, http.StatusOK, map[string]any{"schemas": []string{scim.SchemaList}, "totalResults": 1, "startIndex": 1, "itemsPerPage": 1, "Resources": []any{user}})
}

// The token routes, for a signed-in workspace admin.

type scimTokenRequest struct {
	Domains     []string `json:"domains"`
	DefaultRole string   `json:"default_role"`
	AllowAdmin  bool     `json:"allow_admin"`
}

type scimStatus struct {
	Configured  bool       `json:"configured"`
	Hint        string     `json:"hint,omitempty"`
	Domains     []string   `json:"domains"`
	DefaultRole string     `json:"default_role,omitempty"`
	AllowAdmin  bool       `json:"allow_admin"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	BaseURL     string     `json:"base_url"`
	Header      string     `json:"token_header"`
	Token       string     `json:"token,omitempty"` // only in the answer that issues it
}

func (a *api) scimStatusOf(t scim.Token, ok bool) scimStatus {
	s := scimStatus{Configured: ok, Domains: []string{}, BaseURL: a.scimBase(), Header: scimTokenHeader}
	if ok {
		created := t.CreatedAt
		s.Hint, s.Domains, s.DefaultRole, s.AllowAdmin, s.CreatedAt = t.Hint, t.Domains, t.DefaultRole, t.AllowAdmin, &created
	}
	return s
}

func (a *api) getSCIMToken(w http.ResponseWriter, r *http.Request) {
	id, ok := a.ssoAdmin(w, r)
	if !ok {
		return
	}
	t, err := a.d.SCIM.Get(r.Context(), id.workspace)
	if err != nil && !errors.Is(err, scim.ErrNotFound) {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, a.scimStatusOf(t, err == nil))
}

// issueSCIMToken makes a new token for the workspace, replacing the old one.
// Each domain must be verified on one of the workspace's SSO connections.
func (a *api) issueSCIMToken(w http.ResponseWriter, r *http.Request) {
	id, ok := a.ssoAdmin(w, r)
	if !ok {
		return
	}
	var in scimTokenRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	role := in.DefaultRole
	if role == "" {
		role = string(teams.RoleViewer)
	}
	if r, ok := teams.ParseRole(role); !ok || r == teams.RoleAdmin {
		writeError(w, http.StatusBadRequest, "invalid_input", "default_role must be viewer or editor", nil)
		return
	}
	if len(in.Domains) == 0 || len(in.Domains) > 20 {
		writeError(w, http.StatusBadRequest, "invalid_input", "give 1 to 20 domains", nil)
		return
	}
	conns, err := a.d.SSO.Connections().List(r.Context(), id.workspace)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var domains []string
	for _, d := range in.Domains {
		nd, err := sso.NormalDomain(d)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_input", err.Error(), nil)
			return
		}
		verified := slices.ContainsFunc(conns, func(c sso.Connection) bool { return !c.Verified[nd].IsZero() })
		if !verified {
			writeError(w, http.StatusConflict, "scim_unverified_domain", "verify "+nd+" on one of this workspace's SSO connections first", nil)
			return
		}
		if !slices.Contains(domains, nd) {
			domains = append(domains, nd)
		}
	}
	slices.Sort(domains)
	secret, hash := scim.NewSecret()
	t := scim.Token{Workspace: id.workspace, Hash: hash, Hint: secret[len(secret)-4:], Domains: domains, DefaultRole: role, AllowAdmin: in.AllowAdmin, CreatedAt: time.Now().UTC()}
	if err := a.auditBegin(r, auditStep{Tenant: workspaceTenant(id.workspace), Object: objectRef("workspace", id.workspace)}); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.d.SCIM.Put(r.Context(), t); err != nil {
		a.fail(w, r, err)
		return
	}
	out := a.scimStatusOf(t, true)
	out.Token = secret
	writeJSON(w, http.StatusCreated, out)
}

func (a *api) revokeSCIMToken(w http.ResponseWriter, r *http.Request) {
	id, ok := a.ssoAdmin(w, r)
	if !ok {
		return
	}
	if err := a.auditBegin(r, auditStep{Tenant: workspaceTenant(id.workspace), Object: objectRef("workspace", id.workspace)}); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.d.SCIM.Delete(r.Context(), id.workspace); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
