package httpapi

import (
	"net/http"
	"slices"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/scim"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
)

// Admin console (#870, backend only): one read for a workspace admin with
// everything the admin screen shows. Off unless Deps.AdminConsole is set
// (ADMIN_CONSOLE=on, which needs TEAMS=on). It reads only; each setting is
// changed through its own route, which the answer lists. No secret is ever
// in it: no SCIM token or hash, no OIDC client secret, no invitation
// metadata beyond the invited emails.

type adminMember struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type adminSSODomain struct {
	Domain   string `json:"domain"`
	Verified bool   `json:"verified"`
}

type adminSSOConnection struct {
	ID       string           `json:"id"`
	Protocol string           `json:"protocol"`
	Enabled  bool             `json:"enabled"`
	Enforced bool             `json:"enforced"`
	Domains  []adminSSODomain `json:"domains"`
}

func (a *api) adminConsole(w http.ResponseWriter, r *http.Request) {
	id, ok := a.ssoAdmin(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	ws, err := a.d.Workspaces.Get(ctx, id.workspace)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	members := make([]adminMember, 0, len(ws.Members))
	for _, e := range ws.Members {
		role, _ := ws.RoleOf(e)
		members = append(members, adminMember{Email: e, Role: string(role)})
	}
	invited := append([]string{}, ws.Invited...)
	features := map[string]bool{"sso": a.d.SSO != nil, "scim": a.d.SCIM != nil, "retention": a.d.Retention != nil,
		"metering": a.d.Metering != nil, "audit": a.d.Audit != nil, "api_keys": a.d.APIKeys != nil}
	// The routes that change each part, so the screen needs no guessing.
	routes := map[string]string{"members": "/v1/workspaces/{id}/members"}
	if _, ok := a.d.Workspaces.(teams.Renamer); ok {
		routes["name"] = "/v1/workspaces/{id}/name"
	}
	out := map[string]any{
		"workspace": map[string]any{"id": ws.ID, "name": ws.Name, "created_at": ws.CreatedAt, "member_count": len(ws.Members)},
		"members":   members, "invited": invited, "features": features,
	}
	if a.d.SSO != nil {
		cs, err := a.d.SSO.Connections().List(ctx, id.workspace)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		conns := make([]adminSSOConnection, 0, len(cs))
		for _, c := range cs {
			ds := make([]adminSSODomain, 0, len(c.Domains))
			for _, d := range c.Domains {
				ds = append(ds, adminSSODomain{Domain: d, Verified: !c.Verified[d].IsZero()})
			}
			conns = append(conns, adminSSOConnection{ID: c.ID, Protocol: c.Protocol, Enabled: c.Enabled, Enforced: c.Enforced, Domains: ds})
		}
		out["sso"] = map[string]any{"connections": conns}
		routes["sso"] = "/v1/workspaces/{id}/sso"
	}
	if a.d.SCIM != nil {
		t, err := a.d.SCIM.Get(ctx, id.workspace)
		if err != nil && err != scim.ErrNotFound {
			a.fail(w, r, err)
			return
		}
		s := map[string]any{"configured": err == nil, "domains": []string{}}
		if err == nil {
			s["hint"], s["domains"], s["default_role"], s["allow_admin"], s["created_at"] = t.Hint, slices.Clone(t.Domains), t.DefaultRole, t.AllowAdmin, t.CreatedAt
		}
		out["scim"] = s
		routes["scim"] = "/v1/workspaces/{id}/scim/token"
	}
	if a.d.Retention != nil {
		p, _, err := a.d.Retention.Policy(ctx, id.workspace)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		out["retention"] = map[string]any{"case_days": p.CaseDays, "audit_days": p.AuditDays}
		routes["retention"] = "/v1/workspaces/{id}/retention"
	}
	if a.d.Metering != nil {
		st, err := a.d.Metering.Status(ctx, id.workspace, a.d.Metering.Month())
		if err != nil {
			a.fail(w, r, err)
			return
		}
		out["usage"] = st
		routes["budget"] = "/v1/workspaces/{id}/budget"
	}
	if a.d.Audit != nil {
		routes["audit"] = "/v1/audit"
		routes["audit_search"] = "/v1/workspaces/{id}/admin/audit"
	}
	if a.d.APIKeys != nil {
		routes["api_keys"] = "/v1/workspaces/{id}/api-keys"
	}
	out["routes"] = routes
	a.auditSingle(r, auditStep{Tenant: workspaceTenant(id.workspace), Object: objectRef("workspace", id.workspace), Action: audit.WorkspaceAdminView})
	writeJSON(w, http.StatusOK, out)
}
