package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
)

// Workspace roles (#283). A real handler over the memory stores, driven over
// HTTP with user tokens that the fake verifier accepts. Expectations are
// written here from the documented permission table, not read from the code.

type roleEnv struct {
	t    *testing.T
	hs   harness
	cs   *store.Memory
	ws   *teams.Memory
	wsID string // carol's workspace: carol admin, dave editor, vera viewer
	zed  string // another workspace: zed admin, zoe editor
	keys map[string]string
	// veraOwn is a case vera made before she joined any workspace.
	veraOwn string
}

func newRoleEnv(t *testing.T, ownership bool, adjust ...func(*Deps)) *roleEnv {
	t.Helper()
	e := &roleEnv{t: t, cs: store.NewMemory(), ws: teams.NewMemory(), keys: map[string]string{}}
	hs, _, _ := teamsHarness(t, func(d *Deps) {
		d.Cases, d.Workspaces, d.CaseOwnership, d.BOMVersions = e.cs, e.ws, ownership, true
		d.BrokerWorkflow, d.SupplierDeclarations = true, true
		d.Planner = &estimatingPlanner{fakePlanner: &fakePlanner{}}
		withKeys(d)
		for _, f := range adjust {
			f(d)
		}
	})
	e.hs = hs
	e.keys["acme"], _ = issueKey(t, hs, "Acme", 1000000)
	// vera has a personal case before she joins any workspace.
	e.veraOwn = hs.newCase(t, user("vera"))
	_, w := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Acme Co"}`, user("carol"))
	e.wsID = w["id"].(string)
	hs.addToWorkspace(t, e.wsID, "carol", "dave")
	hs.addToWorkspace(t, e.wsID, "carol", "vera")
	if code, out := hs.doAs(t, "PUT", e.rolePath(e.wsID, "vera"), `{"role":"viewer"}`, user("carol")); code != http.StatusOK {
		t.Fatalf("make vera a viewer = %d %v", code, out)
	}
	_, z := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Zed Co"}`, user("zed"))
	e.zed = z["id"].(string)
	hs.addToWorkspace(t, e.zed, "zed", "zoe")
	return e
}

func (e *roleEnv) rolePath(ws, name string) string {
	return "/v1/workspaces/" + ws + "/members/" + url.PathEscape(name+"@example.com") + "/role"
}

func (e *roleEnv) memberPath(ws, name string) string {
	return "/v1/workspaces/" + ws + "/members/" + url.PathEscape(name+"@example.com")
}

// wsCase is a fresh case made by dave (editor) in carol's workspace.
func (e *roleEnv) wsCase() string { return e.hs.newCase(e.t, user("dave")) }

func (e *roleEnv) do(rt isoRoute, id string, h map[string]string) isoCall {
	e.t.Helper()
	req := httptest.NewRequest(rt.method, "/v1/cases/"+id+rt.path, strings.NewReader(rt.body))
	for k, v := range with(rt.header, h) {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.hs.h.ServeHTTP(rec, req)
	return isoCall{rec.Code, rec.Body.String(), rec.Header().Get("Content-Type")}
}

func (e *roleEnv) snapshot() string {
	e.t.Helper()
	all, err := e.cs.List(context.Background())
	if err != nil {
		e.t.Fatal(err)
	}
	b, _ := json.Marshal(all)
	return string(b)
}

func errCode(body string) string {
	var out struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal([]byte(body), &out)
	return out.Code
}

// roleRoutes are the case-scoped requests with the export split by its
// preview flag, and whether a viewer may make each (the permission table).
type roleRoute struct {
	isoRoute
	viewer bool
}

func roleRoutes() []roleRoute {
	var out []roleRoute
	for _, rt := range isoRoutes() {
		switch rt.name {
		case "pin":
			continue // admin token only; see TestIsolationBadCredentialsAndAdminRoutes
		case "export":
			out = append(out,
				roleRoute{isoRoute{"export preview", "POST", "/export", `{"preview":true}`, nil, rt.pattern}, true},
				roleRoute{isoRoute{"export record", "POST", "/export", `{}`, nil, rt.pattern}, false},
				roleRoute{isoRoute{"export record preview=false", "POST", "/export", `{"preview":false}`, nil, rt.pattern}, false},
				roleRoute{isoRoute{"export record with a plan index", "POST", "/export", `{"plan_index":0}`, nil, rt.pattern}, false},
			)
			continue
		}
		viewer := rt.name == "get" || rt.name == "plan estimate" || rt.name == "review list" || rt.name == "evidence report" ||
			rt.name == "collab" || rt.name == "comment" || rt.name == "filing get" || rt.name == "supplier request list" || rt.name == "supplier document"
		out = append(out, roleRoute{rt, viewer})
	}
	return out
}

// TestRolePermissionTable: every case-scoped route x every role, on a case
// that belongs to the caller's workspace. Denied cells are 403 forbidden and
// leave every stored case untouched.
func TestRolePermissionTable(t *testing.T) {
	for _, ownership := range []bool{true, false} {
		t.Run(fmt.Sprintf("CASE_OWNERSHIP=%v", ownership), func(t *testing.T) {
			e := newRoleEnv(t, ownership)
			for _, rt := range roleRoutes() {
				for _, who := range []string{"vera", "dave", "carol"} {
					id := e.wsCase()
					before := e.snapshot()
					got := e.do(rt.isoRoute, id, user(who))
					allowed := who != "vera" || rt.viewer
					if allowed {
						if got.code == http.StatusForbidden || (got.code == http.StatusNotFound && !noSuchLink(got.body)) || got.code == http.StatusUnauthorized || got.code >= 500 {
							t.Errorf("%s as %s = %d %s, want it allowed", rt.name, who, got.code, got.body)
						}
						continue
					}
					if got.code != http.StatusForbidden || errCode(got.body) != "forbidden" || !strings.HasPrefix(got.ct, "application/json") {
						t.Errorf("%s as viewer = %d %s, want a JSON 403 forbidden", rt.name, got.code, got.body)
					}
					if e.snapshot() != before {
						t.Errorf("%s as viewer changed a stored case", rt.name)
					}
				}
			}
		})
	}
}

func TestRolePermissionTableCoversEveryCaseRoute(t *testing.T) {
	have := map[string]bool{}
	for _, rt := range roleRoutes() {
		have[rt.pattern] = true
	}
	for p, c := range routeTable {
		if c == caseScoped && !have[p] {
			t.Errorf("no role cell for case-scoped route %q", p)
		}
		if c == caseScoped || c == caseCreate {
			if _, ok := rolePerms[p]; !ok {
				t.Errorf("rolePerms has no entry for %q: decide what a viewer may do", p)
			}
		}
	}
	for p := range rolePerms {
		if _, ok := routeTable[p]; !ok {
			t.Errorf("rolePerms lists unknown route %q", p)
		}
	}
	// The table's read-only rows are exactly the ones the brief allows.
	var reads []string
	for p, perm := range rolePerms {
		if perm == permRead {
			reads = append(reads, p)
		}
	}
	if len(reads) != 12 { // list, get, plan estimate, broker review list, evidence report, with BROKER_WORKFLOW the queue, collab, comment and filing reads, and with SUPPLIER_DECLARATIONS the answers list, a document and the expiry listing; export is permExport
		t.Errorf("read-only routes = %v", reads)
	}
	if rolePerms["POST /v1/cases/{id}/export"] != permExport {
		t.Error("export must be classified as depending on preview")
	}
}

func TestRoleExportPreviewVersusRecord(t *testing.T) {
	e := newRoleEnv(t, true)
	exp := func(id, body string, h map[string]string) (int, map[string]any) {
		return e.hs.doAs(t, "POST", "/v1/cases/"+id+"/export", body, h)
	}
	history := func(id string) int {
		c, err := e.cs.Get(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, ev := range c.History {
			if ev.Kind == "exported" {
				n++
			}
		}
		return n
	}
	id := e.wsCase()
	before := e.snapshot()
	if code, out := exp(id, `{"preview":true}`, user("vera")); code != http.StatusOK || out["content"] == nil {
		t.Fatalf("viewer preview = %d %v", code, out)
	}
	if e.snapshot() != before {
		t.Error("a viewer's preview wrote something")
	}
	for name, body := range map[string]string{"empty body": ``, "empty object": `{}`, "preview false": `{"preview":false}`, "preview null": `{"preview":null}`, "other fields only": `{"format":"markdown"}`} {
		if code, out := exp(id, body, user("vera")); code != http.StatusForbidden {
			t.Errorf("viewer export (%s) = %d %v, want 403", name, code, out)
		}
		if e.snapshot() != before {
			t.Errorf("viewer export (%s) wrote something", name)
		}
	}
	// A preview flag that is not a JSON bool is a typed-parse error, never the preview branch.
	for name, body := range map[string]string{"string": `{"preview":"true"}`, "number": `{"preview":1}`, "array": `{"preview":[true]}`} {
		if code, _ := exp(id, body, user("vera")); code != http.StatusBadRequest && code != http.StatusForbidden {
			t.Errorf("viewer export (preview as %s) = %d, want 400 or 403", name, code)
		}
		if e.snapshot() != before {
			t.Errorf("viewer export (preview as %s) wrote something", name)
		}
	}
	// A preview in a query string or header does not count.
	if code, _ := e.hs.doAs(t, "POST", "/v1/cases/"+id+"/export?preview=true", `{}`, with(user("vera"), map[string]string{"X-Preview": "true"})); code != http.StatusForbidden {
		t.Errorf("viewer with preview in the query/header = %d, want 403", code)
	}
	// Editors record; their preview does not.
	if code, _ := exp(id, `{"preview":true}`, user("dave")); code != http.StatusOK || history(id) != 0 {
		t.Errorf("editor preview recorded or failed: history %d", history(id))
	}
	if code, out := exp(id, `{}`, user("dave")); code != http.StatusOK || history(id) != 1 {
		t.Errorf("editor export = %d %v, history %d", code, out, history(id))
	}
	if c, _ := e.cs.Get(context.Background(), id); c.ExportedAt == nil {
		t.Error("the editor's export did not set ExportedAt")
	}
	// A case the viewer cannot see stays 404, and a missing one is the same.
	zed := e.hs.newCase(t, user("zed"))
	got, _ := exp(zed, `{}`, user("vera"))
	gone, _ := exp("case_000000000000", `{}`, user("vera"))
	if got != http.StatusNotFound || gone != http.StatusNotFound {
		t.Errorf("viewer export of another workspace's case = %d, missing = %d, want 404 both", got, gone)
	}
	// A pinned demo keeps its own rule for everyone: a draft, nothing written.
	demo := e.hs.newCase(t, admin)
	if code, _ := e.hs.doAs(t, "PUT", "/v1/cases/"+demo+"/pin", `{"pinned":true}`, admin); code != http.StatusOK {
		t.Fatal("pin")
	}
	before = e.snapshot()
	if code, _ := exp(demo, `{}`, user("vera")); code != http.StatusOK {
		t.Errorf("viewer export of a pinned demo = %d, want the demo draft", code)
	}
	if e.snapshot() != before {
		t.Error("exporting a pinned demo wrote something")
	}
}

func TestRoleCreateNeedsEditor(t *testing.T) {
	e := newRoleEnv(t, true)
	batch := `{"items":[{"name":"gun","facts":"A heat gun is assembled in Thailand."}]}`
	before := e.snapshot()
	if code, out := e.hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), user("vera")); code != http.StatusForbidden {
		t.Errorf("viewer predict = %d %v, want 403", code, out)
	}
	if code, out := e.hs.doAs(t, "POST", "/v1/cases/batch", batch, user("vera")); code != http.StatusForbidden {
		t.Errorf("viewer batch = %d %v, want 403", code, out)
	}
	if code, _ := e.hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), with(user("vera"), map[string]string{"Accept": "text/event-stream"})); code != http.StatusForbidden {
		t.Errorf("viewer streamed predict = %d, want 403", code)
	}
	if e.snapshot() != before {
		t.Error("a viewer's refused create stored a case")
	}
	for _, who := range []string{"dave", "carol"} {
		if code, out := e.hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), user(who)); code != http.StatusCreated {
			t.Errorf("%s predict = %d %v", who, code, out)
		}
	}
	// A user with no workspace has no role and creates as before.
	if code, _ := e.hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), user("erin")); code != http.StatusCreated {
		t.Errorf("workspace-less user predict = %d", code)
	}
	// Stateless helpers and public pages are no case route: a viewer keeps them.
	for _, p := range []string{"/v1/duty-rates", "/v1/demos", "/v1/eval/summary", "/health", "/v1/workspaces/mine"} {
		if code, _ := e.hs.doAs(t, "GET", p, "", user("vera")); code != http.StatusOK {
			t.Errorf("viewer GET %s = %d", p, code)
		}
	}
}

func TestRoleScopeOwnForeignPinnedAnonymous(t *testing.T) {
	e := newRoleEnv(t, true)
	alice := map[string]string{"X-Visitor-ID": isoAlice}
	aliceCase := e.hs.newCase(t, alice)
	zedCase := e.hs.newCase(t, user("zed"))
	demo := e.hs.newCase(t, admin)
	if code, _ := e.hs.doAs(t, "PUT", "/v1/cases/"+demo+"/pin", `{"pinned":true}`, admin); code != http.StatusOK {
		t.Fatal("pin")
	}
	for _, rt := range roleRoutes() {
		// Another workspace's case and an anonymous case: hidden, identical to a missing one, for every role.
		for _, who := range []string{"vera", "dave", "carol"} {
			gone := e.do(rt.isoRoute, "case_000000000000", user(who))
			for name, id := range map[string]string{"zed's": zedCase, "alice's anonymous": aliceCase} {
				if got := e.do(rt.isoRoute, id, user(who)); got.code != http.StatusNotFound || got != gone {
					t.Errorf("%s as %s on %s case = %+v, want the missing-case 404 %+v", rt.name, who, name, got, gone)
				}
			}
		}
		// The viewer on a pinned demo gets exactly what an anonymous visitor gets (the demo rules), no role effect.
		before := e.snapshot()
		v := e.do(rt.isoRoute, demo, user("vera"))
		a := e.do(rt.isoRoute, demo, alice)
		if v.code != a.code {
			t.Errorf("%s on the pinned demo: viewer %d vs anonymous %d", rt.name, v.code, a.code)
		}
		if e.snapshot() != before {
			t.Errorf("%s as viewer changed the pinned demo", rt.name)
		}
	}
	// vera's personal case (made before she joined) stays hers: owner behaviour, whatever her role.
	for _, rt := range roleRoutes() {
		if rt.name == "delete" {
			continue
		}
		if got := e.do(rt.isoRoute, e.veraOwn, user("vera")); got.code == http.StatusForbidden || (got.code == http.StatusNotFound && !noSuchLink(got.body)) || got.code >= 500 {
			t.Errorf("%s as vera on her own pre-workspace case = %d %s", rt.name, got.code, got.body)
		}
	}
	// ...and workspace members do not see it.
	if got := e.do(isoRoutes()[0], e.veraOwn, user("dave")); got.code != http.StatusNotFound {
		t.Errorf("dave reads vera's personal case = %d", got.code)
	}
}

func TestRoleListShowsOnlyPermittedCasesToViewers(t *testing.T) {
	for _, ownership := range []bool{true, false} {
		t.Run(fmt.Sprintf("CASE_OWNERSHIP=%v", ownership), func(t *testing.T) {
			e := newRoleEnv(t, ownership)
			mine := e.wsCase()
			zed := e.hs.newCase(t, user("zed"))
			alice := e.hs.newCase(t, map[string]string{"X-Visitor-ID": isoAlice})
			ids := e.hs.listIDs(t, user("vera"))
			if !ids[mine] || !ids[e.veraOwn] {
				t.Errorf("viewer list misses workspace/own case: %v", ids)
			}
			if ids[zed] || ids[alice] {
				t.Errorf("viewer list shows another tenant's or an anonymous case: %v", ids)
			}
		})
	}
}

// The list is filtered by visibility before it is capped: a viewer's
// workspace case is not pushed out by newer cases of other tenants.
func TestRoleListBeforeCapForViewers(t *testing.T) {
	e := newRoleEnv(t, true, func(d *Deps) { d.CasesListLimit = 3 })
	mine := e.wsCase()
	time.Sleep(2 * time.Millisecond)
	for i := 0; i < 8; i++ {
		e.hs.newCase(t, user("zed"))
		e.hs.newCase(t, map[string]string{"X-Visitor-ID": isoAlice})
	}
	if ids := e.hs.listIDs(t, user("vera")); !ids[mine] {
		t.Errorf("a viewer's workspace case was capped out by foreign cases: %v", ids)
	}
}

func TestRoleForgedClaimsGrantNothing(t *testing.T) {
	e := newRoleEnv(t, true)
	forged := map[string]string{"X-Role": "admin", "X-User-Role": "admin", "X-Workspace": e.wsID, "X-Tenant": e.wsID, "X-Workspace-Role": "editor", "X-Is-Admin": "true"}
	id := e.wsCase()
	before := e.snapshot()
	// A viewer with forged role headers is still a viewer, on writes and on role changes.
	for _, rt := range roleRoutes() {
		if rt.viewer {
			continue
		}
		if got := e.do(rt.isoRoute, id, with(user("vera"), forged)); got.code != http.StatusForbidden {
			t.Errorf("%s as viewer with forged headers = %d", rt.name, got.code)
		}
	}
	if code, _ := e.hs.doAs(t, "PUT", e.rolePath(e.wsID, "vera"), `{"role":"admin"}`, with(user("vera"), forged)); code != http.StatusForbidden {
		t.Errorf("viewer promotes self with forged headers = %d", code)
	}
	if code, _ := e.hs.doAs(t, "POST", "/v1/workspaces/"+e.wsID+"/members", `{"email":"x@example.com"}`, with(user("vera"), forged)); code != http.StatusForbidden {
		t.Errorf("viewer invites with forged headers = %d", code)
	}
	// Body fields naming a role, workspace or tenant are refused or ignored, never obeyed.
	for _, body := range []string{`{"role":"admin"}`, `{"role":"viewer","workspace":"x"}`, `{"role":"admin","admin":true}`, `{"role":"admin","tenant":"x"}`} {
		code, _ := e.hs.doAs(t, "PUT", e.rolePath(e.wsID, "vera"), body, user("vera"))
		if code != http.StatusForbidden && code != http.StatusBadRequest {
			t.Errorf("viewer sends %s = %d", body, code)
		}
	}
	if code, _ := e.hs.doAs(t, "PATCH", "/v1/cases/"+id, `{"display_name":"x","role":"admin","workspace":"`+e.wsID+`"}`, user("vera")); code != http.StatusForbidden {
		t.Errorf("viewer rename with a role in the body = %d", code)
	}
	if e.snapshot() != before {
		t.Error("forged claims changed a case")
	}
	if _, w := e.hs.doAs(t, "GET", "/v1/workspaces/mine", "", with(user("vera"), forged)); w["workspace"].(map[string]any)["role"] != "viewer" {
		t.Errorf("forged headers changed the reported role: %v", w)
	}
	// A stranger forging a workspace id reaches nothing: 404, like a missing case.
	for _, rt := range roleRoutes() {
		if got := e.do(rt.isoRoute, id, forged); got.code != http.StatusNotFound {
			t.Errorf("%s with only forged headers = %d, want 404", rt.name, got.code)
		}
	}
	// Another workspace's admin cannot manage this one, and it looks like a missing workspace.
	for _, c := range []struct{ method, path, body string }{
		{"PUT", e.rolePath(e.wsID, "vera"), `{"role":"admin"}`},
		{"DELETE", e.memberPath(e.wsID, "vera"), ``},
		{"POST", "/v1/workspaces/" + e.wsID + "/members", `{"email":"x@example.com"}`},
	} {
		got, _ := e.hs.doAs(t, c.method, c.path, c.body, user("zed"))
		gone, _ := e.hs.doAs(t, c.method, strings.Replace(c.path, e.wsID, "ws_doesnotexist", 1), c.body, user("zed"))
		if got != http.StatusNotFound || gone != http.StatusNotFound {
			t.Errorf("cross-tenant %s %s = %d (missing workspace %d), want 404 both", c.method, c.path, got, gone)
		}
	}
	if w, _ := e.ws.Get(context.Background(), e.wsID); !w.IsAdmin("carol@example.com") || w.IsAdmin("vera@example.com") || !w.HasMember("vera@example.com") {
		t.Errorf("workspace changed by forged requests: %+v", w)
	}
}

func TestRoleAPIKeyAndAdminTokenDoNotInheritWorkspaceRoles(t *testing.T) {
	e := newRoleEnv(t, true)
	id := e.wsCase()
	acme := map[string]string{"X-API-Key": e.keys["acme"]}
	forged := map[string]string{"X-Role": "admin", "X-Workspace": e.wsID}
	before := e.snapshot()
	for _, rt := range roleRoutes() {
		// A key never reaches workspace cases, whatever it claims.
		if got := e.do(rt.isoRoute, id, with(acme, forged)); got.code != http.StatusNotFound {
			t.Errorf("%s with an API key = %d, want 404", rt.name, got.code)
		}
	}
	for _, c := range []struct{ method, path, body string }{
		{"PUT", e.rolePath(e.wsID, "vera"), `{"role":"admin"}`},
		{"DELETE", e.memberPath(e.wsID, "vera"), ``},
		{"POST", "/v1/workspaces/" + e.wsID + "/members", `{"email":"x@example.com"}`},
	} {
		if code, _ := e.hs.doAs(t, c.method, c.path, c.body, with(acme, forged)); code != http.StatusUnauthorized {
			t.Errorf("key calls %s %s = %d, want 401 (no signed-in user)", c.method, c.path, code)
		}
		// The admin token is not a workspace admin either; membership routes need a signed-in user.
		if code, _ := e.hs.doAs(t, c.method, c.path, c.body, admin); code != http.StatusUnauthorized {
			t.Errorf("admin token calls %s %s = %d, want 401", c.method, c.path, code)
		}
	}
	// A key beside a viewer's token adds no write right.
	if got := e.do(isoRoute{"rename", "PATCH", "", `{"display_name":"k"}`, nil, ""}, id, with(user("vera"), acme)); got.code != http.StatusForbidden {
		t.Errorf("viewer + API key rename = %d, want 403", got.code)
	}
	// The admin token keeps its existing all-access behaviour, with or without a viewer's token.
	for name, h := range map[string]map[string]string{"admin alone": admin, "admin with viewer token": with(admin, user("vera"))} {
		if got := e.do(isoRoute{"rename", "PATCH", "", `{"display_name":"by admin"}`, nil, ""}, id, h); got.code != http.StatusOK {
			t.Errorf("%s rename = %d %s, want 200", name, got.code, got.body)
		}
	}
	if code, _ := e.hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), with(admin, user("vera"))); code != http.StatusCreated {
		t.Errorf("admin token with a viewer token creates = %d", code)
	}
	// ...but admin + viewer token does not make the viewer a workspace admin on the membership routes.
	if code, _ := e.hs.doAs(t, "PUT", e.rolePath(e.wsID, "vera"), `{"role":"admin"}`, with(admin, user("vera"))); code != http.StatusForbidden {
		t.Errorf("admin token + viewer token changes roles = %d, want 403", code)
	}
	_ = before
}

func TestRoleManagement(t *testing.T) {
	e := newRoleEnv(t, true)
	put := func(by, target, role string) (int, map[string]any) {
		return e.hs.doAs(t, "PUT", e.rolePath(e.wsID, target), `{"role":"`+role+`"}`, user(by))
	}
	roleOf := func(name string) string {
		w, _ := e.ws.Get(context.Background(), e.wsID)
		r, _ := w.RoleOf(name + "@example.com")
		return string(r)
	}
	// Workspace DTO: caller's role; per-member roles for admins only.
	_, mine := e.hs.doAs(t, "GET", "/v1/workspaces/mine", "", user("carol"))
	dto := mine["workspace"].(map[string]any)
	if dto["role"] != "admin" || dto["admin"] != true {
		t.Errorf("admin dto = %v", dto)
	}
	roles, _ := dto["member_roles"].(map[string]any)
	if roles["carol@example.com"] != "admin" || roles["dave@example.com"] != "editor" || roles["vera@example.com"] != "viewer" || len(roles) != 3 {
		t.Errorf("member_roles = %v", roles)
	}
	for who, want := range map[string]string{"dave": "editor", "vera": "viewer"} {
		_, m := e.hs.doAs(t, "GET", "/v1/workspaces/mine", "", user(who))
		d := m["workspace"].(map[string]any)
		if d["role"] != want || d["admin"] != false {
			t.Errorf("%s dto = %v", who, d)
		}
		if _, leaked := d["member_roles"]; leaked {
			t.Errorf("%s sees other members' roles: %v", who, d["member_roles"])
		}
	}
	// Rules.
	if code, _ := put("dave", "vera", "editor"); code != http.StatusForbidden {
		t.Errorf("editor changes a role = %d", code)
	}
	if code, _ := put("vera", "vera", "admin"); code != http.StatusForbidden {
		t.Errorf("viewer promotes self = %d", code)
	}
	for _, bad := range []string{"owner", "Admin", "", "superadmin"} {
		if code, _ := put("carol", "vera", bad); code != http.StatusBadRequest {
			t.Errorf("role %q = %d, want 400", bad, code)
		}
	}
	if code, _ := e.hs.doAs(t, "PUT", e.rolePath(e.wsID, "vera"), `{"role":"editor","x":1}`, user("carol")); code != http.StatusBadRequest {
		t.Errorf("unknown field = %d", code)
	}
	if code, _ := e.hs.doAs(t, "PUT", "/v1/workspaces/"+e.wsID+"/members/not-an-email/role", `{"role":"editor"}`, user("carol")); code != http.StatusBadRequest {
		t.Errorf("bad email = %d", code)
	}
	if code, _ := put("carol", "nobody", "viewer"); code != http.StatusNotFound {
		t.Errorf("role for a non-member = %d, want 404", code)
	}
	e.hs.doAs(t, "POST", "/v1/workspaces/"+e.wsID+"/members", `{"email":"pending@example.com"}`, user("carol"))
	if code, _ := put("carol", "pending", "viewer"); code != http.StatusNotFound {
		t.Errorf("role for an invited-only email = %d, want 404 (no role until joined)", code)
	}
	if code, _ := e.hs.doAs(t, "PUT", e.rolePath(e.wsID, "vera"), `{"role":"viewer"}`, nil); code != http.StatusUnauthorized {
		t.Errorf("no token = %d", code)
	}
	if code, _ := e.hs.doAs(t, "PUT", e.rolePath(e.wsID, "vera"), `{"role":"viewer"}`, map[string]string{"X-User-Token": "bad"}); code != http.StatusUnauthorized {
		t.Errorf("invalid token = %d", code)
	}
	if code, _ := e.hs.doAs(t, "PUT", e.rolePath(e.wsID, "vera"), `{"role":"viewer"}`, user("erin")); code != http.StatusNotFound {
		t.Errorf("user without a workspace = %d, want 404", code)
	}
	if code, out := put("carol", "vera", "viewer"); code != http.StatusOK || out["role"] != "admin" {
		t.Errorf("same role again = %d %v", code, out)
	}
	// Last admin: neither demotion, removal nor leaving; allowed once a second admin exists.
	if code, out := put("carol", "carol", "editor"); code != http.StatusConflict || errCode(jsonString(out)) != "conflict" {
		t.Errorf("last admin demotes self = %d %v", code, out)
	}
	if code, _ := e.hs.doAs(t, "DELETE", e.memberPath(e.wsID, "carol"), "", user("carol")); code != http.StatusConflict {
		t.Errorf("last admin leaves = %d", code)
	}
	if roleOf("carol") != "admin" {
		t.Fatal("the last admin was demoted")
	}
	if code, out := put("carol", "dave", "admin"); code != http.StatusOK || out["member_roles"].(map[string]any)["dave@example.com"] != "admin" {
		t.Fatalf("promote dave = %d %v", code, out)
	}
	if code, out := put("dave", "carol", "viewer"); code != http.StatusOK {
		t.Fatalf("demote the creator with another admin = %d %v", code, out)
	}
	if roleOf("carol") != "viewer" {
		t.Errorf("creator role = %q, want viewer (an explicit role beats the creator rule)", roleOf("carol"))
	}
	// The demoted creator is not silently admin again, and loses every admin power at once.
	if code, _ := put("carol", "carol", "admin"); code != http.StatusForbidden {
		t.Errorf("demoted creator promotes self = %d", code)
	}
	if code, _ := e.hs.doAs(t, "POST", "/v1/workspaces/"+e.wsID+"/members", `{"email":"y@example.com"}`, user("carol")); code != http.StatusForbidden {
		t.Errorf("demoted creator invites = %d", code)
	}
	if got := e.do(isoRoute{"rename", "PATCH", "", `{"display_name":"c"}`, nil, ""}, e.wsCase(), user("carol")); got.code != http.StatusForbidden {
		t.Errorf("demoted creator writes = %d", got.code)
	}
	// Stale membership: a role change applies to the very next request with the same token.
	id := e.wsCase()
	rename := isoRoute{"rename", "PATCH", "", `{"display_name":"r"}`, nil, ""}
	if got := e.do(rename, id, user("vera")); got.code != http.StatusForbidden {
		t.Fatalf("vera before promotion = %d", got.code)
	}
	if code, _ := put("dave", "vera", "editor"); code != http.StatusOK {
		t.Fatal("promote vera")
	}
	if got := e.do(rename, id, user("vera")); got.code != http.StatusOK {
		t.Errorf("vera after promotion = %d", got.code)
	}
	if code, _ := put("dave", "vera", "viewer"); code != http.StatusOK {
		t.Fatal("demote vera")
	}
	if got := e.do(rename, id, user("vera")); got.code != http.StatusForbidden {
		t.Errorf("vera after demotion = %d, a stale editor role was used", got.code)
	}
	// Removal: admin removes; the member loses workspace cases (even ones they made) but keeps personal ones.
	veraWS := e.hs.newCase(t, user("dave")) // dave makes one; vera has none in the workspace (a viewer cannot create)
	if code, _ := put("dave", "vera", "editor"); code != http.StatusOK {
		t.Fatal("promote vera")
	}
	veraMade := e.hs.newCase(t, user("vera"))
	if code, _ := e.hs.doAs(t, "DELETE", e.memberPath(e.wsID, "vera"), "", user("carol")); code != http.StatusForbidden {
		t.Errorf("a viewer removes a member = %d", code)
	}
	code, out := e.hs.doAs(t, "DELETE", e.memberPath(e.wsID, "vera"), "", user("dave"))
	if code != http.StatusOK || out["removed"] != "vera@example.com" || out["workspace"] == nil {
		t.Fatalf("admin removes vera = %d %v", code, out)
	}
	for name, id := range map[string]string{"dave's": veraWS, "her own workspace": veraMade} {
		if got := e.do(isoRoutes()[0], id, user("vera")); got.code != http.StatusNotFound {
			t.Errorf("removed vera reads %s workspace case = %d, want 404", name, got.code)
		}
	}
	if got := e.do(isoRoutes()[0], e.veraOwn, user("vera")); got.code != http.StatusOK {
		t.Errorf("removed vera loses her personal case: %d", got.code)
	}
	if ids := e.hs.listIDs(t, user("vera")); ids[veraWS] || ids[veraMade] || !ids[e.veraOwn] {
		t.Errorf("removed vera's list = %v", ids)
	}
	if code, _ := e.hs.doAs(t, "DELETE", e.memberPath(e.wsID, "vera"), "", user("dave")); code != http.StatusNotFound {
		t.Errorf("removing a non-member = %d, want 404", code)
	}
	// Rejoining is by new invitation, as an editor.
	e.hs.addToWorkspace(t, e.wsID, "dave", "vera")
	if roleOf("vera") != "editor" {
		t.Errorf("rejoined role = %q", roleOf("vera"))
	}
	// Leaving: any member, not the last admin.
	code, out = e.hs.doAs(t, "DELETE", e.memberPath(e.wsID, "vera"), "", user("vera"))
	if code != http.StatusOK || out["workspace"] != nil {
		t.Errorf("vera leaves = %d %v", code, out)
	}
	if code, _ := e.hs.doAs(t, "DELETE", e.memberPath(e.wsID, "carol"), "", user("vera")); code != http.StatusNotFound {
		t.Errorf("an outsider removes a member = %d, want 404", code)
	}
	// A removed creator who is invited back returns as an editor, not by the creator rule.
	e.hs.doAs(t, "DELETE", e.memberPath(e.wsID, "carol"), "", user("carol"))
	e.hs.addToWorkspace(t, e.wsID, "dave", "carol")
	if roleOf("carol") != "editor" {
		t.Errorf("creator rejoined as %q, want editor", roleOf("carol"))
	}
}

func jsonString(v any) string { b, _ := json.Marshal(v); return string(b) }

// A workspace written before roles existed has no role map: its creator is an
// admin and every other member an editor, over HTTP too.
func TestLegacyWorkspaceMembersAreEditors(t *testing.T) {
	e := newRoleEnv(t, true)
	if err := e.ws.Create(context.Background(), teams.Workspace{ID: "ws_legacy000001", Name: "Old", Members: []string{"lea@example.com", "leo@example.com"}, CreatedBy: "lea@example.com", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	_, lea := e.hs.doAs(t, "GET", "/v1/workspaces/mine", "", user("lea"))
	_, leo := e.hs.doAs(t, "GET", "/v1/workspaces/mine", "", user("leo"))
	if d := lea["workspace"].(map[string]any); d["role"] != "admin" || d["admin"] != true {
		t.Errorf("legacy creator = %v", d)
	}
	if d := leo["workspace"].(map[string]any); d["role"] != "editor" || d["admin"] != false {
		t.Errorf("legacy member = %v", d)
	}
	id := e.hs.newCase(t, user("lea"))
	if got := e.do(isoRoute{"rename", "PATCH", "", `{"display_name":"legacy"}`, nil, ""}, id, user("leo")); got.code != http.StatusOK {
		t.Errorf("legacy member renames a workspace case = %d, want 200 (editor)", got.code)
	}
	if code, _ := e.hs.doAs(t, "POST", "/v1/workspaces/ws_legacy000001/members", `{"email":"new@example.com"}`, user("leo")); code != http.StatusForbidden {
		t.Errorf("legacy member invites = %d, want 403", code)
	}
	if code, _ := e.hs.doAs(t, "POST", "/v1/workspaces/ws_legacy000001/members", `{"email":"new@example.com"}`, user("lea")); code != http.StatusOK {
		t.Errorf("legacy creator invites = %d", code)
	}
}

// The token a request carries decides the user; the role is read from the
// workspace store on every request, never cached on the token.
func TestRoleReadFreshFromStoreEachRequest(t *testing.T) {
	e := newRoleEnv(t, true)
	id := e.wsCase()
	rename := isoRoute{"rename", "PATCH", "", `{"display_name":"r"}`, nil, ""}
	// Change the role directly in the store, bypassing the API: the next request sees it.
	if _, err := e.ws.SetRole(context.Background(), e.wsID, "carol@example.com", "dave@example.com", teams.RoleViewer); err != nil {
		t.Fatal(err)
	}
	if got := e.do(rename, id, user("dave")); got.code != http.StatusForbidden {
		t.Errorf("store-level demotion not seen: %d", got.code)
	}
}

// Two admins demote each other at the same moment over HTTP: one wins, the other
// is told it would leave no admin (409) or that it is no longer an admin (403).
func TestRoleConcurrentMutualDemotionOverHTTP(t *testing.T) {
	for i := 0; i < 30; i++ {
		e := newRoleEnv(t, true)
		if code, _ := e.hs.doAs(t, "PUT", e.rolePath(e.wsID, "dave"), `{"role":"admin"}`, user("carol")); code != http.StatusOK {
			t.Fatal("promote dave")
		}
		codes := make(chan int, 2)
		start := make(chan struct{})
		for _, p := range [][2]string{{"carol", "dave"}, {"dave", "carol"}} {
			go func() {
				<-start
				code, _ := e.hs.doAs(t, "PUT", e.rolePath(e.wsID, p[1]), `{"role":"viewer"}`, user(p[0]))
				codes <- code
			}()
		}
		close(start)
		a, b := <-codes, <-codes
		w, _ := e.ws.Get(context.Background(), e.wsID)
		admins := 0
		for _, m := range w.Members {
			if w.IsAdmin(m) {
				admins++
			}
		}
		if admins != 1 || (a != http.StatusOK && b != http.StatusOK) || (a == http.StatusOK && b == http.StatusOK) {
			t.Fatalf("round %d: codes %d and %d, %d admins left", i, a, b, admins)
		}
	}
}
