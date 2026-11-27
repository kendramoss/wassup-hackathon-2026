package httpapi

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/apikey"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
)

// Admin console, the changes (#870, D954): rename the workspace, keys that
// belong to the workspace, and a search of its audit trail. All are behind
// ADMIN_CONSOLE and, like every route under /v1/workspaces/{id}, are for the
// admin of that workspace only: another workspace's id is a 404, a viewer or
// editor a 403, and a SCIM token or API key no way in.

const (
	// workspaceKeyQuota is the fixed daily request quota of a workspace key;
	// like a self-service key (#788), the admin cannot ask for more.
	workspaceKeyQuota = 1000
	// maxWorkspaceKeys caps the keys of one workspace that are not revoked.
	maxWorkspaceKeys = 20
)

type renameWorkspaceRequest struct {
	Name string `json:"name"`
}

// renameWorkspace is PUT /v1/workspaces/{id}/name.
func (a *api) renameWorkspace(w http.ResponseWriter, r *http.Request) {
	id, ok := a.ssoAdmin(w, r)
	if !ok {
		return
	}
	var in renameWorkspaceRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	name, valid := teams.CleanName(in.Name)
	if !valid {
		a.fail(w, r, fmt.Errorf("%w: name must be 1 to %d characters", domain.ErrInvalid, teams.MaxName))
		return
	}
	rn, ok := a.d.Workspaces.(teams.Renamer)
	if !ok {
		a.fail(w, r, errors.New("the workspace store cannot rename"))
		return
	}
	if err := a.auditBegin(r, auditStep{Tenant: workspaceTenant(id.workspace), Object: objectRef("workspace", id.workspace)}); err != nil {
		a.fail(w, r, err)
		return
	}
	saved, err := rn.Rename(r.Context(), id.workspace, id.email, name)
	if err != nil {
		a.workspaceError(w, r, err, "")
		return
	}
	writeJSON(w, http.StatusOK, workspaceOut(saved, id.email))
}

// adminAudit is GET /v1/workspaces/{id}/admin/audit: the workspace's own
// trail, filtered by actor kind, action, outcome and time range, a page at a
// time. The tenant is the caller's workspace, never a parameter, and an event
// holds only enumerated values and pseudonyms, so no free text can come back.
func (a *api) adminAudit(w http.ResponseWriter, r *http.Request) {
	id, ok := a.ssoAdmin(w, r)
	if !ok {
		return
	}
	q, err := a.auditListQueryOf(r, "limit", "cursor", "order", "action", "outcome", "actor_kind", "from", "to")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.writeAuditPage(w, r, workspaceTenant(id.workspace), q)
}

type workspaceKeyRequest struct {
	Name string `json:"name"`
}

// issueWorkspaceKey is POST /v1/workspaces/{id}/api-keys. The secret is in
// this answer only; the store keeps its SHA-256.
func (a *api) issueWorkspaceKey(w http.ResponseWriter, r *http.Request) {
	id, ok := a.ssoAdmin(w, r)
	if !ok {
		return
	}
	var in workspaceKeyRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := apikey.ValidateIssue(in.Name, workspaceKeyQuota); err != nil {
		a.fail(w, r, err)
		return
	}
	ks, err := a.d.APIKeys.ListForWorkspace(r.Context(), id.workspace)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	active := 0
	for _, k := range ks {
		if !k.Disabled {
			active++
		}
	}
	if active >= maxWorkspaceKeys {
		writeError(w, http.StatusConflict, "conflict", fmt.Sprintf("a workspace has at most %d active API keys: revoke one first", maxWorkspaceKeys), nil)
		return
	}
	tenant := workspaceTenant(id.workspace)
	if err := a.auditBegin(r, auditStep{Tenant: tenant, Object: "apikey:new"}); err != nil {
		a.fail(w, r, err)
		return
	}
	secret, k, err := a.d.APIKeys.IssueForWorkspace(r.Context(), id.workspace, in.Name, workspaceKeyQuota)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if op := auditOpOf(r.Context()); op != nil && op.intent != nil {
		op.object = objectRef("apikey", k.ID)
	}
	a.d.Logger.InfoContext(r.Context(), "api key issued", "request_id", requestRef(r.Context()), keyAttr(k.ID), "daily_quota", k.DailyQuota)
	writeJSON(w, http.StatusCreated, map[string]any{"key": secret, "api_key": k})
}

// listWorkspaceKeys is GET /v1/workspaces/{id}/api-keys, without secrets.
func (a *api) listWorkspaceKeys(w http.ResponseWriter, r *http.Request) {
	id, ok := a.ssoAdmin(w, r)
	if !ok {
		return
	}
	ks, err := a.d.APIKeys.ListForWorkspace(r.Context(), id.workspace)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if ks == nil {
		ks = []apikey.Key{}
	}
	a.auditSingle(r, auditStep{Tenant: workspaceTenant(id.workspace), Object: objectRef("workspace", id.workspace)})
	writeJSON(w, http.StatusOK, map[string]any{"api_keys": ks})
}

// revokeWorkspaceKey is DELETE /v1/workspaces/{id}/api-keys/{key}. Another
// workspace's key, an admin key and an unknown id are all the same 404.
func (a *api) revokeWorkspaceKey(w http.ResponseWriter, r *http.Request) {
	id, ok := a.ssoAdmin(w, r)
	if !ok {
		return
	}
	keyID := r.PathValue("key")
	if err := a.d.APIKeys.ExistsForWorkspace(r.Context(), id.workspace, keyID); err != nil {
		if errors.Is(err, apikey.ErrNotFound) {
			if a.d.APIKeys.Exists(r.Context(), keyID) == nil {
				auditHidden(r)
			}
		}
		a.fail(w, r, err)
		return
	}
	if err := a.auditBegin(r, auditStep{Tenant: workspaceTenant(id.workspace), Object: objectRef("apikey", keyID)}); err != nil {
		a.fail(w, r, err)
		return
	}
	if op := auditOpOf(r.Context()); op != nil && op.intent != nil {
		op.object = objectRef("apikey", keyID)
	}
	if err := a.d.APIKeys.DisableForWorkspace(r.Context(), id.workspace, keyID); err != nil {
		a.fail(w, r, err)
		return
	}
	a.d.Logger.InfoContext(r.Context(), "api key disabled", "request_id", requestRef(r.Context()), keyAttr(keyID))
	w.WriteHeader(http.StatusNoContent)
}
