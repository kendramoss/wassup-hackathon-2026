package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/apikey"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
)

// adminEnv is a TEAMS harness with the admin console, API keys and the
// audit trail on.
func adminEnv(t *testing.T, adjust ...func(*Deps)) (harness, *audit.Service) {
	t.Helper()
	ps, err := audit.NewPseudonymizer("1", []byte(auditTestKey))
	if err != nil {
		t.Fatal(err)
	}
	svc := audit.NewService(audit.NewMemory(), ps, audit.Options{})
	hs, _, _ := teamsHarness(t, func(d *Deps) {
		d.AdminConsole, d.APIKeys, d.Audit = true, apikey.New(apikey.NewMemory()), svc
		for _, f := range adjust {
			f(d)
		}
	})
	return hs, svc
}

func mkWorkspace(t *testing.T, hs harness, admin, name string) string {
	t.Helper()
	code, out := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"`+name+`"}`, user(admin))
	if code != http.StatusCreated {
		t.Fatalf("workspace = %d %v", code, out)
	}
	return out["id"].(string)
}

func TestWorkspaceRename(t *testing.T) {
	hs, svc := adminEnv(t)
	ws := mkWorkspace(t, hs, "alice", "Acme Secret Imports")
	mkWorkspace(t, hs, "zed", "Zed")
	hs.addToWorkspace(t, ws, "alice", "carol")
	path := "/v1/workspaces/" + ws + "/name"
	body := `{"name":"  Acme   Customs  "}`
	if code, _ := hs.doAs(t, "PUT", path, body, nil); code != http.StatusUnauthorized {
		t.Errorf("anonymous = %d", code)
	}
	if code, _ := hs.doAs(t, "PUT", path, body, user("carol")); code != http.StatusForbidden {
		t.Errorf("editor = %d", code)
	}
	if code, _ := hs.doAs(t, "PUT", path, body, user("zed")); code != http.StatusNotFound {
		t.Errorf("another workspace's admin = %d", code)
	}
	if code, _ := hs.doAs(t, "PUT", "/v1/workspaces/ws_000000000000/name", body, user("alice")); code != http.StatusNotFound {
		t.Errorf("unknown workspace = %d", code)
	}
	for _, bad := range []string{`{"name":""}`, `{"name":"   "}`, `{"name":"` + strings.Repeat("n", 101) + `"}`, `{"name":"x","owner":"y"}`} {
		if code, _ := hs.doAs(t, "PUT", path, bad, user("alice")); code != http.StatusBadRequest {
			t.Errorf("%.30s = %d, want 400", bad, code)
		}
	}
	if _, out := hs.doAs(t, "GET", "/v1/workspaces/mine", "", user("alice")); out["workspace"].(map[string]any)["name"] != "Acme Secret Imports" {
		t.Fatalf("a refused rename changed the name: %v", out)
	}
	code, out := hs.doAs(t, "PUT", path, body, user("alice"))
	if code != http.StatusOK || out["name"] != "Acme Customs" {
		t.Fatalf("rename = %d %v", code, out)
	}
	if _, out := hs.doAs(t, "GET", "/v1/workspaces/mine", "", user("carol")); out["workspace"].(map[string]any)["name"] != "Acme Customs" {
		t.Errorf("members see %v", out)
	}
	if _, out := hs.doAs(t, "GET", "/v1/workspaces/mine", "", user("zed")); out["workspace"].(map[string]any)["name"] != "Zed" {
		t.Errorf("the other workspace changed: %v", out)
	}
	// Audited as an intent and a completion; the name is nowhere in the trail.
	page, err := svc.List(t.Context(), "ws:"+ws, audit.ListQuery{Filter: audit.Filter{Action: audit.WorkspaceRename, Outcome: audit.Allowed}})
	if err != nil || len(page.Events) != 2 || page.Events[0].Phase != audit.PhaseIntent || page.Events[1].Outcome != audit.Allowed {
		t.Fatalf("rename events = %v %v", page.Events, err)
	}
}

func TestAdminConsoleChangesAreOffWithTheFlag(t *testing.T) {
	hs, _ := adminEnv(t, func(d *Deps) { d.AdminConsole = false })
	ws := mkWorkspace(t, hs, "alice", "A")
	for _, rt := range [][2]string{{"PUT", "/name"}, {"GET", "/api-keys"}, {"POST", "/api-keys"}, {"GET", "/admin/audit"}} {
		if code, _ := hs.doAs(t, rt[0], "/v1/workspaces/"+ws+rt[1], `{"name":"x"}`, user("alice")); code != http.StatusNotFound {
			t.Errorf("%s %s with the flag off = %d", rt[0], rt[1], code)
		}
	}
}

func TestWorkspaceAPIKeys(t *testing.T) {
	hs, svc := adminEnv(t)
	ws := mkWorkspace(t, hs, "alice", "A")
	other := mkWorkspace(t, hs, "zed", "Z")
	hs.addToWorkspace(t, ws, "alice", "carol")
	path := "/v1/workspaces/" + ws + "/api-keys"
	if code, _ := hs.doAs(t, "POST", path, `{"name":"ERP"}`, nil); code != http.StatusUnauthorized {
		t.Errorf("anonymous issue = %d", code)
	}
	if code, _ := hs.doAs(t, "POST", path, `{"name":"ERP"}`, user("carol")); code != http.StatusForbidden {
		t.Errorf("editor issue = %d", code)
	}
	if code, _ := hs.doAs(t, "GET", path, "", user("carol")); code != http.StatusForbidden {
		t.Errorf("editor list = %d", code)
	}
	if code, _ := hs.doAs(t, "POST", path, `{"name":"ERP"}`, user("zed")); code != http.StatusNotFound {
		t.Errorf("another workspace's admin issues = %d", code)
	}
	for _, bad := range []string{`{"name":""}`, `{"name":"x","daily_quota":999999}`, `{"name":"x","workspace":"` + other + `"}`} {
		if code, _ := hs.doAs(t, "POST", path, bad, user("alice")); code != http.StatusBadRequest {
			t.Errorf("issue %.40s = %d, want 400", bad, code)
		}
	}
	code, out := hs.doAs(t, "POST", path, `{"name":"  ERP   sync "}`, user("alice"))
	if code != http.StatusCreated {
		t.Fatalf("issue = %d %v", code, out)
	}
	secret := out["key"].(string)
	rec := out["api_key"].(map[string]any)
	id := rec["id"].(string)
	if !strings.HasPrefix(secret, "op_") || rec["name"] != "ERP sync" || rec["daily_quota"] != float64(workspaceKeyQuota) {
		t.Fatalf("issued = %v", out)
	}
	for f := range rec {
		if f != "id" && f != "name" && f != "daily_quota" && f != "disabled" && f != "created_at" {
			t.Errorf("a key record carries %q", f)
		}
	}
	// The secret works, and is shown once only.
	if code, _ := hs.doAs(t, "GET", "/v1/cases", "", map[string]string{"X-API-Key": secret}); code != http.StatusOK {
		t.Errorf("the key = %d", code)
	}
	if code, _ := hs.doAs(t, "GET", "/v1/workspaces/"+ws+"/admin", "", map[string]string{"X-API-Key": secret}); code != http.StatusUnauthorized {
		t.Errorf("a key reaches the admin console = %d", code)
	}
	code, b := doRaw(t, hs, "GET", path, "", user("alice"))
	body := string(b)
	if code != http.StatusOK || strings.Contains(body, secret) || strings.Contains(body, secret[len(apikey.Prefix):]) || strings.Contains(body, "sha256") || !strings.Contains(body, id) {
		t.Fatalf("list = %d %s", code, body)
	}
	// Other workspaces, the operator list and the self-service list.
	if _, out := hs.doAs(t, "GET", "/v1/workspaces/"+other+"/api-keys", "", user("zed")); len(out["api_keys"].([]any)) != 0 {
		t.Errorf("another workspace lists %v", out)
	}
	if code, _ := hs.doAs(t, "GET", path, "", user("zed")); code != http.StatusNotFound {
		t.Errorf("another workspace's admin lists mine = %d", code)
	}
	if _, out := hs.doAs(t, "GET", "/v1/api-keys", "", user("alice")); len(out["api_keys"].([]any)) != 0 {
		t.Errorf("self-service list shows the workspace key: %v", out)
	}
	if _, out := hs.doAs(t, "GET", "/v1/admin/api-keys", "", admin); len(out["api_keys"].([]any)) != 1 {
		t.Errorf("the operator no longer sees it: %v", out)
	}
	// Revoking: another workspace and the self-service route cannot; the
	// workspace admin can; an unknown id is the same 404.
	if code, _ := hs.doAs(t, "DELETE", "/v1/workspaces/"+other+"/api-keys/"+id, "", user("zed")); code != http.StatusNotFound {
		t.Errorf("revoke from another workspace = %d", code)
	}
	if code, _ := hs.doAs(t, "DELETE", "/v1/workspaces/"+ws+"/api-keys/"+id, "", user("zed")); code != http.StatusNotFound {
		t.Errorf("revoke by another workspace's admin = %d", code)
	}
	if code, _ := hs.doAs(t, "DELETE", path+"/"+id, "", user("carol")); code != http.StatusForbidden {
		t.Errorf("editor revoke = %d", code)
	}
	if code, _ := hs.doAs(t, "DELETE", "/v1/api-keys/"+id, "", user("alice")); code != http.StatusNotFound {
		t.Errorf("self-service revoke of a workspace key = %d", code)
	}
	if code, _ := hs.doAs(t, "DELETE", path+"/0123456789abcdef", "", user("alice")); code != http.StatusNotFound {
		t.Errorf("revoke unknown = %d", code)
	}
	if code, _ := hs.doAs(t, "GET", "/v1/cases", "", map[string]string{"X-API-Key": secret}); code != http.StatusOK {
		t.Fatalf("a refused revoke disabled the key = %d", code)
	}
	if code, _ := hs.doAs(t, "DELETE", path+"/"+id, "", user("alice")); code != http.StatusNoContent {
		t.Fatalf("revoke = %d", code)
	}
	if code, _ := hs.doAs(t, "GET", "/v1/cases", "", map[string]string{"X-API-Key": secret}); code != http.StatusUnauthorized {
		t.Errorf("revoked key = %d", code)
	}
	// The workspace's trail holds the issue and the revoke, not the name.
	for _, a := range []audit.Action{audit.APIKeyIssue, audit.APIKeyDisable} {
		page, err := svc.List(t.Context(), "ws:"+ws, audit.ListQuery{Filter: audit.Filter{Action: a, Phase: audit.PhaseCompletion}})
		if err != nil || len(page.Events) != 1 || page.Events[0].Object != "apikey:"+id {
			t.Errorf("%s events = %v %v", a, page.Events, err)
		}
	}
}

func TestWorkspaceAPIKeyLimitAndRateClass(t *testing.T) {
	hs, _ := adminEnv(t)
	ws := mkWorkspace(t, hs, "alice", "A")
	path := "/v1/workspaces/" + ws + "/api-keys"
	for i := 0; i < maxWorkspaceKeys; i++ {
		if code, out := hs.doAs(t, "POST", path, `{"name":"k"}`, user("alice")); code != http.StatusCreated {
			t.Fatalf("key %d = %d %v", i, code, out)
		}
	}
	if code, _ := hs.doAs(t, "POST", path, `{"name":"one more"}`, user("alice")); code != http.StatusConflict {
		t.Errorf("past the cap = %d", code)
	}
	_, out := hs.doAs(t, "GET", path, "", user("alice"))
	id := out["api_keys"].([]any)[0].(map[string]any)["id"].(string)
	if code, _ := hs.doAs(t, "DELETE", path+"/"+id, "", user("alice")); code != http.StatusNoContent {
		t.Fatal(code)
	}
	if code, _ := hs.doAs(t, "POST", path, `{"name":"again"}`, user("alice")); code != http.StatusCreated {
		t.Errorf("after a revoke = %d", code)
	}

	// The routes are in the cheap rate class, per client.
	limited, _ := adminEnv(t, func(d *Deps) {
		d.Limits = Limits{BasicPerMin: 2, LightPerMin: 100, HeavyPerMin: 100, GlobalPerMin: 1000}
	})
	for n, rt := range []struct{ method, path, body string }{
		{"PUT", "/name", `{"name":"x"}`}, {"GET", "/api-keys", ""}, {"POST", "/api-keys", `{"name":"x"}`},
	} {
		w := mkWorkspaceFrom(t, limited, "bob"+rt.method, "198.51.100."+strconv.Itoa(10+n))
		client := with(user("bob"+rt.method), map[string]string{"X-Client-IP": "203.0.113." + strconv.Itoa(10+n)})
		for i := 0; i < 2; i++ { // mkWorkspaceFrom used none of this client's bucket
			if rec := serve(limited, rt.method, "/v1/workspaces/"+w+rt.path, rt.body, client); rec.Code == http.StatusTooManyRequests {
				t.Fatalf("%s %s request %d limited", rt.method, rt.path, i+1)
			}
		}
		if rec := serve(limited, rt.method, "/v1/workspaces/"+w+rt.path, rt.body, client); rec.Code != http.StatusTooManyRequests {
			t.Errorf("%s %s past the limit = %d", rt.method, rt.path, rec.Code)
		}
	}
}

// mkWorkspaceFrom makes a workspace from another client address, so the
// setup does not use the bucket under test.
func mkWorkspaceFrom(t *testing.T, hs harness, admin, ip string) string {
	t.Helper()
	code, out := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"W"}`, with(user(admin), map[string]string{"X-Client-IP": ip}))
	if code != http.StatusCreated {
		t.Fatalf("workspace = %d %v", code, out)
	}
	return out["id"].(string)
}

func TestAdminAuditSearch(t *testing.T) {
	hs, _ := adminEnv(t)
	ws := mkWorkspace(t, hs, "alice", "Hidden Name Co")
	other := mkWorkspace(t, hs, "zed", "Z")
	hs.addToWorkspace(t, ws, "alice", "carol")
	// Some events: two key issues, a revoke, a rename, a key's refused call.
	keys := workspaceKeys(t, hs, ws, "alice", 2)
	hs.doAs(t, "DELETE", "/v1/workspaces/"+ws+"/api-keys/"+keys[1].id, "", user("alice"))
	hs.doAs(t, "PUT", "/v1/workspaces/"+ws+"/name", `{"name":"Renamed Co"}`, user("alice"))
	hs.doAs(t, "GET", "/v1/workspaces/"+ws+"/admin", "", map[string]string{"X-API-Key": keys[0].secret})
	hs.doAs(t, "POST", "/v1/workspaces/"+other+"/api-keys", `{"name":"z"}`, user("zed"))

	path := "/v1/workspaces/" + ws + "/admin/audit"
	for name, who := range map[string]map[string]string{"anonymous": nil, "editor": user("carol")} {
		want := http.StatusUnauthorized
		if who != nil {
			want = http.StatusForbidden
		}
		if code, _ := hs.doAs(t, "GET", path, "", who); code != want {
			t.Errorf("%s = %d, want %d", name, code, want)
		}
	}
	if code, _ := hs.doAs(t, "GET", path, "", user("zed")); code != http.StatusNotFound {
		t.Errorf("another workspace's admin = %d", code)
	}
	if code, _ := hs.doAs(t, "GET", path, "", map[string]string{"X-API-Key": keys[0].secret}); code != http.StatusUnauthorized {
		t.Errorf("a key = %d", code)
	}
	code, out := hs.doAs(t, "GET", path, "", user("alice"))
	events := out["events"].([]any)
	if code != http.StatusOK || out["tenant"] != "ws:"+ws || len(events) == 0 {
		t.Fatalf("search = %d %v", code, out)
	}
	for _, e := range events {
		if tn := e.(map[string]any)["tenant"]; tn != "ws:"+ws {
			t.Errorf("an event of tenant %v", tn)
		}
	}
	// Action, actor kind, outcome and time range.
	if code, out = hs.doAs(t, "GET", path+"?action=apikey.issue", "", user("alice")); code != http.StatusOK || len(out["events"].([]any)) != 4 {
		t.Errorf("by action = %d %v", code, out)
	}
	if _, out = hs.doAs(t, "GET", path+"?actor_kind=apikey", "", user("alice")); len(out["events"].([]any)) == 0 {
		t.Errorf("no event by an API key: %v", out)
	} else {
		for _, e := range out["events"].([]any) {
			if e.(map[string]any)["actor_kind"] != "apikey" {
				t.Errorf("by actor kind: %v", e)
			}
		}
	}
	if _, out = hs.doAs(t, "GET", path+"?actor_kind=user&action=workspace.rename", "", user("alice")); len(out["events"].([]any)) != 2 {
		t.Errorf("rename by users = %v", out)
	}
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	if _, out = hs.doAs(t, "GET", path+"?from="+future, "", user("alice")); len(out["events"].([]any)) != 0 {
		t.Errorf("from the future = %v", out)
	}
	if _, out = hs.doAs(t, "GET", path+"?to="+future, "", user("alice")); len(out["events"].([]any)) == 0 {
		t.Errorf("to the future = %v", out)
	}
	// Paging: pages join up with no overlap and no gap.
	var all []any
	cursor := ""
	for i := 0; i < 50; i++ {
		_, out = hs.doAs(t, "GET", path+"?limit=3&order=asc&cursor="+cursor, "", user("alice"))
		if len(out["events"].([]any)) > 3 {
			t.Fatalf("page of %d", len(out["events"].([]any)))
		}
		all = append(all, out["events"].([]any)...)
		if cursor, _ = out["next_cursor"].(string); cursor == "" {
			break
		}
	}
	seen := map[float64]bool{}
	for _, e := range all {
		seq := e.(map[string]any)["seq"].(float64)
		if seen[seq] {
			t.Errorf("seq %v twice", seq)
		}
		seen[seq] = true
	}
	if len(all) < 8 {
		t.Errorf("paged %d events", len(all))
	}
	// Bad input is refused, and the tenant is not a parameter.
	for _, q := range []string{"?tenant=platform", "?actor_kind=owner", "?action=nope", "?limit=0", "?limit=500", "?from=yesterday", "?actor=a1_00000000000000000000000000000000", "?order=sideways", "?cursor=garbage", "?q=Renamed"} {
		if code, _ := hs.doAs(t, "GET", path+q, "", user("alice")); code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", q, code)
		}
	}
	// Only what the trail stores: no email, workspace name or key secret.
	code, b := doRaw(t, hs, "GET", path+"?limit=200", "", user("alice"))
	body := string(b)
	for _, s := range []string{"@example.com", "alice", "carol", "Hidden Name", "Renamed Co", keys[0].secret, keys[0].secret[len(apikey.Prefix):], "tok-"} {
		if code != http.StatusOK || strings.Contains(body, s) {
			t.Errorf("the search carries %q (%d)", s, code)
		}
	}
	// The overview points at it.
	if _, out := hs.doAs(t, "GET", "/v1/workspaces/"+ws+"/admin", "", user("alice")); out["routes"].(map[string]any)["audit_search"] == nil ||
		out["routes"].(map[string]any)["api_keys"] == nil || out["routes"].(map[string]any)["name"] == nil || out["features"].(map[string]any)["api_keys"] != true {
		t.Errorf("overview routes = %v", out)
	}
}

type issuedKey struct{ id, secret string }

func workspaceKeys(t *testing.T, hs harness, ws, admin string, n int) []issuedKey {
	t.Helper()
	var out []issuedKey
	for i := 0; i < n; i++ {
		code, o := hs.doAs(t, "POST", "/v1/workspaces/"+ws+"/api-keys", `{"name":"k"}`, user(admin))
		if code != http.StatusCreated {
			t.Fatalf("key = %d %v", code, o)
		}
		out = append(out, issuedKey{o["api_key"].(map[string]any)["id"].(string), o["key"].(string)})
	}
	return out
}
