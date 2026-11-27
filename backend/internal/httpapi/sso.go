package httpapi

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/logsafe"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/sso"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
)

// Single sign-on (#868, docs/design/sso.md). Off unless Deps.SSO is set
// (SSO=on, which needs TEAMS=on). The sign-in routes are unauthenticated by
// nature; the connection routes need a signed-in admin of the workspace.

type ssoStartRequest struct {
	Email string `json:"email"`
}

// ssoStart returns the identity provider URL for the email's domain.
func (a *api) ssoStart(w http.ResponseWriter, r *http.Request) {
	var in ssoStartRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	res, err := a.d.SSO.Start(r.Context(), in.Email)
	switch {
	case errors.Is(err, sso.ErrNoConnection):
		writeError(w, http.StatusNotFound, "sso_not_configured", "this email domain does not sign in with single sign-on; use another sign-in", nil)
		return
	case err != nil:
		a.ssoFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type ssoOIDCCallback struct {
	State string `json:"state"`
	Code  string `json:"code"`
	// Error is the provider's error parameter, passed on by the callback page.
	Error string `json:"error"`
}

// ssoOIDCCallback finishes an OIDC sign-in with what the provider put on the
// callback URL.
func (a *api) ssoOIDCCallback(w http.ResponseWriter, r *http.Request) {
	var in ssoOIDCCallback
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if in.Error != "" {
		a.d.Logger.InfoContext(r.Context(), "sso provider returned an error", "request_id", requestRef(r.Context()), "class", string(logsafe.ClassAuthRefused))
		writeError(w, http.StatusUnauthorized, "sso_failed", "the identity provider did not sign you in; try again", nil)
		return
	}
	login, err := a.d.SSO.FinishOIDC(r.Context(), in.State, in.Code)
	a.ssoFinish(w, r, login, err)
}

// maxACSBody caps the ACS form: a SAMLResponse plus the relay state.
const maxACSBody = 512 << 10

// ssoSAMLACS finishes a SAML sign-in. It takes the identity provider's form
// post (SAMLResponse, RelayState) as forwarded by the callback page, or the
// same two fields as JSON.
func (a *api) ssoSAMLACS(w http.ResponseWriter, r *http.Request) {
	var resp, relay string
	ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	switch ct {
	case "application/x-www-form-urlencoded":
		b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxACSBody))
		if err != nil {
			a.fail(w, r, fmt.Errorf("%w: the form is too large", domain.ErrInvalid))
			return
		}
		v, err := url.ParseQuery(string(b))
		if err != nil {
			a.fail(w, r, fmt.Errorf("%w: the form does not parse", domain.ErrInvalid))
			return
		}
		resp, relay = v.Get("SAMLResponse"), v.Get("RelayState")
	default:
		var in struct {
			SAMLResponse string `json:"saml_response"`
			RelayState   string `json:"relay_state"`
		}
		if err := decode(w, r, &in); err != nil {
			a.fail(w, r, err)
			return
		}
		resp, relay = in.SAMLResponse, in.RelayState
	}
	login, err := a.d.SSO.FinishSAML(r.Context(), resp, relay)
	a.ssoFinish(w, r, login, err)
}

func (a *api) ssoFinish(w http.ResponseWriter, r *http.Request, login sso.Login, err error) {
	switch {
	case errors.Is(err, sso.ErrOtherWorkspace):
		a.d.Logger.InfoContext(r.Context(), "sso sign-in refused: member of another workspace", "request_id", requestRef(r.Context()), "class", string(logsafe.ClassAuthRefused))
		writeError(w, http.StatusForbidden, "sso_other_workspace", "this account belongs to another workspace; ask an admin to remove it there first", nil)
		return
	case errors.Is(err, sso.ErrLogin):
		// What failed (signature, audience, replay, ...) is never sent to the
		// caller. The audit wrapper records the 401 as a denial of sso.login.
		a.d.Logger.InfoContext(r.Context(), "sso sign-in refused", "request_id", requestRef(r.Context()), "class", string(logsafe.ClassAuthRefused))
		writeError(w, http.StatusUnauthorized, "sso_failed", "single sign-on did not complete; start again from the sign-in page", nil)
		return
	case err != nil:
		a.ssoFail(w, r, err)
		return
	}
	detail := ""
	if login.Provisioned {
		detail = "provisioned"
	}
	a.auditSingle(r, auditStep{Tenant: workspaceTenant(login.WorkspaceID), Object: objectRef("workspace", login.WorkspaceID), Detail: detail, TargetID: login.Email})
	writeJSON(w, http.StatusOK, login)
}

func (a *api) ssoFail(w http.ResponseWriter, r *http.Request, err error) {
	a.d.Logger.ErrorContext(r.Context(), "sso error", "request_id", requestRef(r.Context()), "err", logsafe.Err(err))
	writeError(w, http.StatusBadGateway, "sso_unavailable", "single sign-on is unavailable right now; try again", nil)
}

// ssoMetadata serves the SAML service provider metadata.
func (a *api) ssoMetadata(w http.ResponseWriter, r *http.Request) {
	b, err := a.d.SSO.SPMetadata()
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/samlmetadata+xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

// --- connection management (workspace admins) ---------------------------------

type ssoConnectionDTO struct {
	ID              string            `json:"id"`
	Protocol        string            `json:"protocol"`
	Domains         []ssoDomainDTO    `json:"domains"`
	DefaultRole     string            `json:"default_role"`
	Enabled         bool              `json:"enabled"`
	Enforced        bool              `json:"enforced"`
	OIDCIssuer      string            `json:"oidc_issuer,omitempty"`
	OIDCClientID    string            `json:"oidc_client_id,omitempty"`
	HasClientSecret bool              `json:"has_client_secret"`
	SAMLSSOURL      string            `json:"saml_sso_url,omitempty"`
	SAMLIssuer      string            `json:"saml_issuer,omitempty"`
	SAMLCertsPEM    []string          `json:"saml_certs_pem,omitempty"`
	ServiceProvider ssoSPDTO          `json:"service_provider"`
	Checks          []sso.DomainCheck `json:"checks,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

type ssoDomainDTO struct {
	Domain     string     `json:"domain"`
	Verified   bool       `json:"verified"`
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
	// The TXT record to publish: name and value.
	RecordName  string `json:"record_name"`
	RecordValue string `json:"record_value"`
}

type ssoSPDTO struct {
	RedirectURL string `json:"oidc_redirect_url"`
	ACSURL      string `json:"saml_acs_url"`
	EntityID    string `json:"saml_entity_id"`
}

func (a *api) ssoOut(c sso.Connection, checks []sso.DomainCheck) ssoConnectionDTO {
	ds := make([]ssoDomainDTO, 0, len(c.Domains))
	for _, d := range c.Domains {
		dd := ssoDomainDTO{Domain: d, RecordName: sso.VerifyRecordName(d), RecordValue: c.VerifyToken}
		if t, ok := c.Verified[d]; ok {
			t := t
			dd.Verified, dd.VerifiedAt = true, &t
		}
		ds = append(ds, dd)
	}
	return ssoConnectionDTO{ID: c.ID, Protocol: c.Protocol, Domains: ds, DefaultRole: c.DefaultRole, Enabled: c.Enabled, Enforced: c.Enforced,
		OIDCIssuer: c.OIDCIssuer, OIDCClientID: c.OIDCClientID, HasClientSecret: c.HasSecret(),
		SAMLSSOURL: c.SAMLSSOURL, SAMLIssuer: c.SAMLIssuer, SAMLCertsPEM: c.SAMLCertsPEM,
		ServiceProvider: ssoSPDTO{RedirectURL: a.d.SSO.RedirectURL(), ACSURL: a.d.SSO.ACSURL(), EntityID: a.d.SSO.EntityID()},
		Checks:          checks, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt}
}

// ssoAdmin checks the caller is a signed-in admin of the workspace in the
// path. A workspace the caller is not in is a 404, as for the member routes.
func (a *api) ssoAdmin(w http.ResponseWriter, r *http.Request) (identity, bool) {
	id, ok := requireUser(w, r)
	if !ok {
		return identity{}, false
	}
	wsID := r.PathValue("id")
	if wsID != id.workspace {
		auditHidden(r)
		a.workspaceError(w, r, teams.ErrNotFound, "")
		return identity{}, false
	}
	auditConfirm(r, "workspace", wsID)
	if id.role != teams.RoleAdmin {
		a.workspaceError(w, r, teams.ErrForbidden, "")
		return identity{}, false
	}
	return id, true
}

// ssoConn loads a connection of the caller's workspace; another workspace's
// is a 404.
func (a *api) ssoConn(w http.ResponseWriter, r *http.Request, id identity) (sso.Connection, bool) {
	c, err := a.d.SSO.Connections().Get(r.Context(), r.PathValue("conn"))
	if errors.Is(err, sso.ErrNotFound) || err == nil && c.Workspace != id.workspace {
		writeError(w, http.StatusNotFound, "not_found", "no such SSO connection", nil)
		return sso.Connection{}, false
	}
	if err != nil {
		a.fail(w, r, err)
		return sso.Connection{}, false
	}
	return c, true
}

func (a *api) listSSOConnections(w http.ResponseWriter, r *http.Request) {
	id, ok := a.ssoAdmin(w, r)
	if !ok {
		return
	}
	cs, err := a.d.SSO.Connections().List(r.Context(), id.workspace)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out := make([]ssoConnectionDTO, 0, len(cs))
	for _, c := range cs {
		out = append(out, a.ssoOut(c, nil))
	}
	a.auditSingle(r, auditStep{Tenant: workspaceTenant(id.workspace), Object: objectRef("workspace", id.workspace), Action: audit.SSOConnectionList})
	writeJSON(w, http.StatusOK, map[string]any{"connections": out})
}

type ssoConnectionRequest struct {
	Protocol         string   `json:"protocol"`
	Domains          []string `json:"domains"`
	DefaultRole      string   `json:"default_role"`
	Enabled          *bool    `json:"enabled"`
	Enforced         *bool    `json:"enforced"`
	OIDCIssuer       string   `json:"oidc_issuer"`
	OIDCClientID     string   `json:"oidc_client_id"`
	OIDCClientSecret string   `json:"oidc_client_secret"`
	SAMLSSOURL       string   `json:"saml_sso_url"`
	SAMLIssuer       string   `json:"saml_issuer"`
	SAMLCertsPEM     []string `json:"saml_certs_pem"`
}

// apply copies the request onto c. A blank client secret keeps the stored one.
func (a *api) applySSO(c *sso.Connection, in ssoConnectionRequest) error {
	c.Protocol, c.Domains, c.DefaultRole = strings.ToLower(strings.TrimSpace(in.Protocol)), in.Domains, in.DefaultRole
	c.OIDCIssuer, c.OIDCClientID = strings.TrimSpace(in.OIDCIssuer), strings.TrimSpace(in.OIDCClientID)
	c.SAMLSSOURL, c.SAMLIssuer, c.SAMLCertsPEM = strings.TrimSpace(in.SAMLSSOURL), strings.TrimSpace(in.SAMLIssuer), in.SAMLCertsPEM
	if in.Enabled != nil {
		c.Enabled = *in.Enabled
	}
	if in.Enforced != nil {
		c.Enforced = *in.Enforced
	}
	if in.OIDCClientSecret != "" {
		if len(in.OIDCClientSecret) > 1024 {
			return fmt.Errorf("%w: oidc_client_secret is too long", sso.ErrInvalid)
		}
		sealed, err := a.d.SSO.Keys().Seal(in.OIDCClientSecret)
		if err != nil {
			return err
		}
		c.OIDCSecret = sealed
	}
	return c.Normalize()
}

func (a *api) ssoWriteError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, sso.ErrUnverified):
		writeError(w, http.StatusConflict, "sso_unverified", "verify every domain before turning the connection on", nil)
	case errors.Is(err, sso.ErrDomainTaken):
		writeError(w, http.StatusConflict, "sso_domain_taken", "another connection has verified one of these domains", nil)
	case errors.Is(err, sso.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_input", strings.TrimPrefix(err.Error(), "sso: invalid connection: "), nil)
	default:
		a.fail(w, r, err)
	}
}

func (a *api) createSSOConnection(w http.ResponseWriter, r *http.Request) {
	id, ok := a.ssoAdmin(w, r)
	if !ok {
		return
	}
	var in ssoConnectionRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	existing, err := a.d.SSO.Connections().List(r.Context(), id.workspace)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if len(existing) >= sso.MaxConnections {
		writeError(w, http.StatusConflict, "sso_limit", fmt.Sprintf("a workspace has at most %d SSO connections", sso.MaxConnections), nil)
		return
	}
	now := time.Now().UTC()
	c := sso.Connection{ID: sso.NewConnectionID(), Workspace: id.workspace, VerifyToken: sso.NewVerifyToken(), CreatedBy: id.email, CreatedAt: now, UpdatedAt: now}
	in.Enabled = nil // a new connection's domains are unverified: it starts off
	if err := a.applySSO(&c, in); err != nil {
		a.ssoWriteError(w, r, err)
		return
	}
	if c.Protocol == sso.ProtocolOIDC {
		if err := a.d.SSO.CheckIssuer(r.Context(), c.OIDCIssuer); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_input", "the OIDC issuer could not be discovered at "+c.OIDCIssuer+"/.well-known/openid-configuration", nil)
			return
		}
	}
	if err := a.auditBegin(r, auditStep{Tenant: workspaceTenant(id.workspace), Object: objectRef("ssoconn", c.ID)}); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.d.SSO.Connections().Put(r.Context(), c); err != nil {
		a.ssoWriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, a.ssoOut(c, nil))
}

func (a *api) updateSSOConnection(w http.ResponseWriter, r *http.Request) {
	id, ok := a.ssoAdmin(w, r)
	if !ok {
		return
	}
	c, ok := a.ssoConn(w, r, id)
	if !ok {
		return
	}
	var in ssoConnectionRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	issuer := c.OIDCIssuer
	if err := a.applySSO(&c, in); err != nil {
		a.ssoWriteError(w, r, err)
		return
	}
	if c.Protocol == sso.ProtocolOIDC && c.OIDCIssuer != issuer {
		if err := a.d.SSO.CheckIssuer(r.Context(), c.OIDCIssuer); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_input", "the OIDC issuer could not be discovered at "+c.OIDCIssuer+"/.well-known/openid-configuration", nil)
			return
		}
	}
	c.UpdatedAt = time.Now().UTC()
	if err := a.auditBegin(r, auditStep{Tenant: workspaceTenant(id.workspace), Object: objectRef("ssoconn", c.ID)}); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.d.SSO.Connections().Put(r.Context(), c); err != nil {
		a.ssoWriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, a.ssoOut(c, nil))
}

type ssoVerifyRequest struct {
	// Enable turns the connection on when every domain verifies.
	Enable bool `json:"enable"`
}

func (a *api) verifySSOConnection(w http.ResponseWriter, r *http.Request) {
	id, ok := a.ssoAdmin(w, r)
	if !ok {
		return
	}
	c, ok := a.ssoConn(w, r, id)
	if !ok {
		return
	}
	var in ssoVerifyRequest
	if r.ContentLength != 0 {
		if err := decode(w, r, &in); err != nil {
			a.fail(w, r, err)
			return
		}
	}
	next, checks, err := a.d.SSO.VerifyDomains(r.Context(), c)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if in.Enable && next.AllVerified() {
		next.Enabled = true
	}
	next.UpdatedAt = time.Now().UTC()
	if err := a.auditBegin(r, auditStep{Tenant: workspaceTenant(id.workspace), Object: objectRef("ssoconn", c.ID), Detail: "verify"}); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.d.SSO.Connections().Put(r.Context(), next); err != nil {
		a.ssoWriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, a.ssoOut(next, checks))
}

func (a *api) deleteSSOConnection(w http.ResponseWriter, r *http.Request) {
	id, ok := a.ssoAdmin(w, r)
	if !ok {
		return
	}
	c, ok := a.ssoConn(w, r, id)
	if !ok {
		return
	}
	if err := a.auditBegin(r, auditStep{Tenant: workspaceTenant(id.workspace), Object: objectRef("ssoconn", c.ID)}); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.d.SSO.Connections().Delete(r.Context(), c.ID); err != nil && !errors.Is(err, sso.ErrNotFound) {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
