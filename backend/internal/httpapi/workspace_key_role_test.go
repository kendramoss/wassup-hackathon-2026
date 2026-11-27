package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
)

// Workspace API keys acting on workspace cases (#870, D961). A key is made by
// a workspace admin with a role, viewer or editor, and acts on that
// workspace's cases as a member of that role would. It is never an admin.

func keyRoleEnv(t *testing.T) (*roleEnv, *audit.Service) {
	t.Helper()
	ps, err := audit.NewPseudonymizer("1", []byte(auditTestKey))
	if err != nil {
		t.Fatal(err)
	}
	svc := audit.NewService(audit.NewMemory(), ps, audit.Options{})
	e := newRoleEnv(t, true, func(d *Deps) { d.AdminConsole, d.Audit = true, svc })
	return e, svc
}

// wsKey issues a key in workspace ws as its admin and returns its secret and id.
func (e *roleEnv) wsKey(ws, admin, role string) (string, string) {
	e.t.Helper()
	body := `{"name":"ERP"`
	if role != "" {
		body += `,"role":"` + role + `"`
	}
	code, out := e.hs.doAs(e.t, "POST", "/v1/workspaces/"+ws+"/api-keys", body+`}`, user(admin))
	if code != http.StatusCreated {
		e.t.Fatalf("issue %q key = %d %v", role, code, out)
	}
	return out["key"].(string), out["api_key"].(map[string]any)["id"].(string)
}

func TestWorkspaceKeyIssueTakesARole(t *testing.T) {
	e, _ := keyRoleEnv(t)
	path := "/v1/workspaces/" + e.wsID + "/api-keys"
	for _, bad := range []string{`"admin"`, `"owner"`, `"Editor"`, `"root"`, `"viewer,editor"`, `1`, `true`, `["editor"]`} {
		if code, out := e.hs.doAs(t, "POST", path, `{"name":"x","role":`+bad+`}`, user("carol")); code != http.StatusBadRequest {
			t.Errorf("role %s = %d %v, want 400", bad, code, out)
		}
	}
	if code, _ := e.hs.doAs(t, "POST", path, `{"name":"x","role":"editor"}`, user("dave")); code != http.StatusForbidden {
		t.Errorf("an editor issues a key = %d", code)
	}
	if code, _ := e.hs.doAs(t, "POST", path, `{"name":"x","role":"editor"}`, user("zed")); code != http.StatusNotFound {
		t.Errorf("another workspace's admin issues a key in this one = %d", code)
	}
	for _, role := range []string{"viewer", "editor"} {
		code, out := e.hs.doAs(t, "POST", path, `{"name":"`+role+` key","role":"`+role+`"}`, user("carol"))
		if code != http.StatusCreated || out["api_key"].(map[string]any)["role"] != role {
			t.Errorf("issue %s = %d %v", role, code, out)
		}
	}
	_, out := e.hs.doAs(t, "GET", path, "", user("carol"))
	roles := map[any]bool{}
	for _, k := range out["api_keys"].([]any) {
		roles[k.(map[string]any)["role"]] = true
	}
	if !roles["viewer"] || !roles["editor"] || len(roles) != 2 {
		t.Errorf("the list shows roles %v", roles)
	}
}

// A key made without a role keeps what every workspace key had before: its own
// cases only, no workspace case.
func TestWorkspaceKeyWithoutRoleReachesNoWorkspaceCase(t *testing.T) {
	e, _ := keyRoleEnv(t)
	secret, _ := e.wsKey(e.wsID, "carol", "")
	id := e.wsCase()
	h := map[string]string{"X-API-Key": secret}
	for _, rt := range roleRoutes() {
		if got := e.do(rt.isoRoute, id, h); got.code != http.StatusNotFound {
			t.Errorf("%s with a role-less workspace key = %d, want 404", rt.name, got.code)
		}
	}
	if ids := e.hs.listIDs(t, h); ids[id] {
		t.Error("a role-less key lists a workspace case")
	}
}

// The permission table of roles.go, for a key of each role, on a case of the
// key's own workspace: the same cells as a member of that role.
func TestWorkspaceKeyRolePermissionTable(t *testing.T) {
	e, _ := keyRoleEnv(t)
	for _, role := range []string{"viewer", "editor"} {
		secret, _ := e.wsKey(e.wsID, "carol", role)
		h := map[string]string{"X-API-Key": secret}
		for _, rt := range roleRoutes() {
			id := e.wsCase()
			before := e.snapshot()
			got := e.do(rt.isoRoute, id, h)
			if role == "editor" || rt.viewer {
				if got.code == http.StatusForbidden || (got.code == http.StatusNotFound && !noSuchLink(got.body)) || got.code == http.StatusUnauthorized || got.code >= 500 {
					t.Errorf("%s key: %s = %d %s, want it allowed", role, rt.name, got.code, got.body)
				}
				continue
			}
			if got.code != http.StatusForbidden || errCode(got.body) != "forbidden" {
				t.Errorf("viewer key: %s = %d %s, want 403 forbidden", rt.name, got.code, got.body)
			}
			if e.snapshot() != before {
				t.Errorf("viewer key: %s changed a stored case", rt.name)
			}
		}
	}
}

func TestWorkspaceKeyOtherWorkspaceAndStrangersAre404(t *testing.T) {
	e, _ := keyRoleEnv(t)
	zedCase := e.hs.newCase(t, user("zed"))
	aliceCase := e.hs.newCase(t, map[string]string{"X-Visitor-ID": isoAlice})
	veraOwn := e.veraOwn
	for _, role := range []string{"viewer", "editor"} {
		secret, _ := e.wsKey(e.wsID, "carol", role)
		h := map[string]string{"X-API-Key": secret}
		zedSecret, _ := e.wsKey(e.zed, "zed", role)
		for _, rt := range roleRoutes() {
			gone := e.do(rt.isoRoute, "case_000000000000", h)
			for name, id := range map[string]string{"another workspace's": zedCase, "an anonymous visitor's": aliceCase, "a member's personal": veraOwn} {
				before := e.snapshot()
				if got := e.do(rt.isoRoute, id, h); got != gone {
					t.Errorf("%s key: %s on %s case = %+v, want the missing-case answer %+v", role, rt.name, name, got, gone)
				}
				if e.snapshot() != before {
					t.Errorf("%s key: %s on %s case changed something", role, rt.name, name)
				}
			}
		}
		// The other workspace's key reaches this workspace's case no better.
		mine := e.wsCase()
		for _, rt := range roleRoutes() {
			gone := e.do(rt.isoRoute, "case_000000000000", map[string]string{"X-API-Key": zedSecret})
			if got := e.do(rt.isoRoute, mine, map[string]string{"X-API-Key": zedSecret}); got != gone {
				t.Errorf("%s key of workspace B: %s on workspace A's case = %+v, want %+v", role, rt.name, got, gone)
			}
		}
	}
}

func TestWorkspaceKeyListsOnlyItsWorkspace(t *testing.T) {
	e, _ := keyRoleEnv(t)
	secret, _ := e.wsKey(e.wsID, "carol", "viewer")
	mine := e.wsCase()
	zed := e.hs.newCase(t, user("zed"))
	alice := e.hs.newCase(t, map[string]string{"X-Visitor-ID": isoAlice})
	ids := e.hs.listIDs(t, map[string]string{"X-API-Key": secret})
	if !ids[mine] || ids[zed] || ids[alice] || ids[e.veraOwn] {
		t.Errorf("list = %v", ids)
	}
	_, out := e.hs.doAs(t, "GET", "/v1/cases", "", map[string]string{"X-API-Key": secret})
	for _, c := range out["cases"].([]any) {
		if row := c.(map[string]any); row["id"] == mine && row["can_write"] != false {
			t.Errorf("a viewer key's can_write = %v", row["can_write"])
		}
	}
}

func TestWorkspaceKeyCreate(t *testing.T) {
	e, _ := keyRoleEnv(t)
	viewer, _ := e.wsKey(e.wsID, "carol", "viewer")
	editor, _ := e.wsKey(e.wsID, "carol", "editor")
	batch := `{"items":[{"name":"gun","facts":"A heat gun is assembled in Thailand."}]}`
	before := e.snapshot()
	vh := map[string]string{"X-API-Key": viewer}
	if code, out := e.hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), vh); code != http.StatusForbidden {
		t.Errorf("viewer key predict = %d %v", code, out)
	}
	if code, out := e.hs.doAs(t, "POST", "/v1/cases/batch", batch, vh); code != http.StatusForbidden {
		t.Errorf("viewer key batch = %d %v", code, out)
	}
	if e.snapshot() != before {
		t.Error("a viewer key's refused create stored a case")
	}
	eh := map[string]string{"X-API-Key": editor}
	id := e.hs.newCase(t, eh)
	c, err := e.cs.Get(context.Background(), id)
	if err != nil || c.Workspace != e.wsID || !strings.HasPrefix(c.Owner, keyOwnerPrefix) {
		t.Fatalf("a key's case = workspace %q owner %q err %v", c.Workspace, c.Owner, err)
	}
	// Members see it; the other workspace and a viewer of nothing do not.
	if code, _ := e.hs.doAs(t, "GET", "/v1/cases/"+id, "", user("vera")); code != http.StatusOK {
		t.Errorf("a member reads the key's case = %d", code)
	}
	if code, _ := e.hs.doAs(t, "GET", "/v1/cases/"+id, "", user("zoe")); code != http.StatusNotFound {
		t.Errorf("another workspace reads the key's case = %d", code)
	}
	if code, _ := e.hs.doAs(t, "GET", "/v1/cases/"+id, "", map[string]string{"X-API-Key": e.keys["acme"]}); code != http.StatusNotFound {
		t.Errorf("an operator-issued key reads the key's case = %d", code)
	}
	// The viewer key reads it but cannot change it.
	if code, _ := e.hs.doAs(t, "GET", "/v1/cases/"+id, "", vh); code != http.StatusOK {
		t.Errorf("viewer key reads = %d", code)
	}
	if code, _ := e.hs.doAs(t, "DELETE", "/v1/cases/"+id, "", vh); code != http.StatusForbidden {
		t.Errorf("viewer key deletes = %d", code)
	}
	if code, _ := e.hs.doAs(t, "PATCH", "/v1/cases/"+id, `{"display_name":"renamed"}`, eh); code != http.StatusOK {
		t.Errorf("editor key renames = %d", code)
	}
}

// A key is never an admin: it cannot reach any workspace-admin route, change a
// role, mint a key, read the trail, or act as the operator.
func TestWorkspaceKeyIsNeverAnAdmin(t *testing.T) {
	e, _ := keyRoleEnv(t)
	secret, id := e.wsKey(e.wsID, "carol", "editor")
	h := map[string]string{"X-API-Key": secret}
	ws := "/v1/workspaces/" + e.wsID
	for _, c := range []struct{ method, path, body string }{
		{"GET", ws + "/admin", ""},
		{"GET", ws + "/admin/audit", ""},
		{"PUT", ws + "/name", `{"name":"mine"}`},
		{"GET", ws + "/api-keys", ""},
		{"POST", ws + "/api-keys", `{"name":"more","role":"editor"}`},
		{"DELETE", ws + "/api-keys/" + id, ""},
		{"PUT", e.rolePath(e.wsID, "vera"), `{"role":"admin"}`},
		{"DELETE", e.memberPath(e.wsID, "vera"), ``},
		{"POST", ws + "/members", `{"email":"x@example.com"}`},
		{"PUT", ws + "/retention", `{"case_days":7,"audit_days":7}`},
		{"GET", "/v1/audit?tenant=ws:" + e.wsID, ""},
		{"GET", "/v1/admin/api-keys", ""},
		{"POST", "/v1/admin/api-keys", `{"name":"x","daily_quota":5}`},
		{"POST", "/v1/api-keys", `{"name":"mine"}`},
	} {
		code, _ := e.hs.doAs(t, c.method, c.path, c.body, h)
		if code != http.StatusUnauthorized && code != http.StatusForbidden && code != http.StatusNotFound {
			t.Errorf("a workspace key: %s %s = %d, want it refused", c.method, c.path, code)
		}
	}
	// Forged claims add nothing to a viewer key.
	vsecret, _ := e.wsKey(e.wsID, "carol", "viewer")
	forged := with(map[string]string{"X-API-Key": vsecret}, map[string]string{"X-Role": "admin", "X-Workspace": e.zed, "X-Tenant": e.zed, "X-Workspace-Role": "editor"})
	cid := e.wsCase()
	zedCase := e.hs.newCase(t, user("zed"))
	if code, _ := e.hs.doAs(t, "DELETE", "/v1/cases/"+cid, "", forged); code != http.StatusForbidden {
		t.Errorf("a viewer key with forged role headers deletes = %d", code)
	}
	if code, _ := e.hs.doAs(t, "GET", "/v1/cases/"+zedCase, "", forged); code != http.StatusNotFound {
		t.Errorf("a viewer key with a forged workspace reads another workspace's case = %d", code)
	}
	// The pin route is the operator's alone.
	if code, _ := e.hs.doAs(t, "PUT", "/v1/cases/"+cid+"/pin", `{"pinned":true}`, h); code != http.StatusForbidden {
		t.Errorf("a key pins a case = %d", code)
	}
	// A signed-in user's own role wins over a key sent beside it.
	if code, _ := e.hs.doAs(t, "DELETE", "/v1/cases/"+cid, "", with(user("vera"), map[string]string{"X-API-Key": secret})); code != http.StatusForbidden {
		t.Errorf("a viewer member sending an editor key deletes = %d", code)
	}
}

func TestWorkspaceKeyRevokedStops(t *testing.T) {
	e, _ := keyRoleEnv(t)
	secret, id := e.wsKey(e.wsID, "carol", "editor")
	cid := e.wsCase()
	h := map[string]string{"X-API-Key": secret}
	if code, _ := e.hs.doAs(t, "GET", "/v1/cases/"+cid, "", h); code != http.StatusOK {
		t.Fatalf("key = %d", code)
	}
	if code, _ := e.hs.doAs(t, "DELETE", "/v1/workspaces/"+e.wsID+"/api-keys/"+id, "", user("carol")); code != http.StatusNoContent {
		t.Fatal("revoke")
	}
	if code, _ := e.hs.doAs(t, "GET", "/v1/cases/"+cid, "", h); code != http.StatusUnauthorized {
		t.Errorf("revoked key = %d, want 401", code)
	}
}

// What a key does is in its workspace's trail, as the key.
func TestWorkspaceKeyActionsAreAuditedAsTheKey(t *testing.T) {
	e, svc := keyRoleEnv(t)
	secret, _ := e.wsKey(e.wsID, "carol", "editor")
	h := map[string]string{"X-API-Key": secret}
	id := e.hs.newCase(t, h)
	if code, _ := e.hs.doAs(t, "PATCH", "/v1/cases/"+id, `{"display_name":"renamed"}`, h); code != http.StatusOK {
		t.Fatal("rename")
	}
	for _, a := range []audit.Action{audit.CaseCreate, audit.CaseUpdate} {
		page, err := svc.List(t.Context(), "ws:"+e.wsID, audit.ListQuery{Filter: audit.Filter{Action: a, ActorKind: audit.ActorAPIKey}})
		if err != nil || len(page.Events) == 0 {
			t.Errorf("%s by the key in the workspace's trail = %v %v", a, page.Events, err)
			continue
		}
		for _, ev := range page.Events {
			if ev.Actor == "" || ev.ActorKind != audit.ActorAPIKey {
				t.Errorf("event %+v is not attributed to the key", ev)
			}
		}
	}
	// A refused write by a viewer key is a denial in the same trail.
	vsecret, _ := e.wsKey(e.wsID, "carol", "viewer")
	if code, _ := e.hs.doAs(t, "DELETE", "/v1/cases/"+id, "", map[string]string{"X-API-Key": vsecret}); code != http.StatusForbidden {
		t.Fatal("viewer delete")
	}
	page, err := svc.List(t.Context(), "ws:"+e.wsID, audit.ListQuery{Filter: audit.Filter{ActorKind: audit.ActorAPIKey, Outcome: audit.Denied}})
	if err != nil || len(page.Events) == 0 {
		t.Errorf("a viewer key's refused delete is not in the trail: %v %v", page.Events, err)
	}
}

// The same key over MCP acts as the same principal: a viewer key reads its
// workspace's cases and cannot make a review link; another workspace's case is
// not found.
func TestWorkspaceKeyOverMCP(t *testing.T) {
	e, _ := keyRoleEnv(t)
	secret, _ := e.wsKey(e.wsID, "carol", "viewer")
	mine := e.wsCase()
	zed := e.hs.newCase(t, user("zed"))
	cs := mcpClient(t, e.hs, map[string]string{"X-API-Key": secret, "X-Client-IP": "198.51.100.77"})
	if res, _ := callTool(t, cs, "get_case", map[string]any{"case_id": mine}); res.IsError {
		t.Errorf("get_case on the workspace's case: %s", text(res))
	}
	if res, _ := callTool(t, cs, "get_case", map[string]any{"case_id": zed}); !res.IsError {
		t.Error("get_case on another workspace's case succeeded")
	}
	if _, listed := callTool(t, cs, "list_cases", nil); !strings.Contains(mustJSON(listed), mine) || strings.Contains(mustJSON(listed), zed) {
		t.Errorf("list_cases = %s", mustJSON(listed))
	}
	before := e.snapshot()
	if res, _ := callTool(t, cs, "create_broker_review_link", map[string]any{"case_id": mine}); !res.IsError {
		t.Error("a viewer key made a review link over MCP")
	}
	if e.snapshot() != before {
		t.Error("a viewer key changed a case over MCP")
	}
}
