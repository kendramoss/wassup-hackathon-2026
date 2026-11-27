package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/scim"
)

func TestAdminConsoleIsForTheWorkspaceAdmin(t *testing.T) {
	e := newSCIMEnv(t, 0)
	ws := e.workspace(t, "alice", "example.com")
	e.hs.addToWorkspace(t, ws, "alice", "carol")
	e.hs.doAs(t, "POST", "/v1/workspaces/"+ws+"/members", `{"email":"ivy@example.com"}`, user("alice"))
	tok := e.token(t, "alice", ws, `{"domains":["example.com"]}`)
	path := "/v1/workspaces/" + ws + "/admin"
	if code, _ := e.hs.doAs(t, "GET", path, "", nil); code != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d", code)
	}
	if code, _ := e.hs.doAs(t, "GET", path, "", user("carol")); code != http.StatusForbidden {
		t.Fatalf("editor = %d", code)
	}
	if code, _ := e.hs.doAs(t, "GET", path, "", user("bob")); code != http.StatusNotFound {
		t.Fatalf("outsider = %d", code)
	}
	if code, _ := e.hs.doAs(t, "GET", path, "", map[string]string{scimTokenHeader: tok}); code != http.StatusUnauthorized {
		t.Fatalf("SCIM token = %d", code)
	}
	code, raw := doRaw(t, e.hs, "GET", path, "", user("alice"))
	if code != http.StatusOK {
		t.Fatalf("admin = %d %s", code, raw)
	}
	body := string(raw)
	for _, secret := range []string{tok, scim.Hash(tok), tok[len(scim.TokenPrefix):], "invited_by", "verify_token", "record_value"} {
		if strings.Contains(body, secret) {
			t.Errorf("the overview carries %q", secret)
		}
	}
	_, out := e.hs.doAs(t, "GET", path, "", user("alice"))
	if out["workspace"].(map[string]any)["member_count"].(float64) != 2 || len(out["invited"].([]any)) != 1 {
		t.Fatalf("members = %v", out)
	}
	f := out["features"].(map[string]any)
	if f["sso"] != true || f["scim"] != true || f["retention"] != false {
		t.Fatalf("features = %v", f)
	}
	conns := out["sso"].(map[string]any)["connections"].([]any)
	if d := conns[0].(map[string]any)["domains"].([]any)[0].(map[string]any); d["domain"] != "example.com" || d["verified"] != true {
		t.Fatalf("sso = %v", conns)
	}
	if s := out["scim"].(map[string]any); s["configured"] != true || s["hint"] != tok[len(tok)-4:] {
		t.Fatalf("scim = %v", s)
	}
	if _, ok := out["retention"]; ok {
		t.Error("retention shown while off")
	}
}

func TestAdminConsoleIsOffByDefault(t *testing.T) {
	hs, _, _ := teamsHarness(t, nil)
	_, out := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"A"}`, user("alice"))
	if code, _ := hs.doAs(t, "GET", "/v1/workspaces/"+out["id"].(string)+"/admin", "", user("alice")); code != http.StatusNotFound {
		t.Fatalf("flag off = %d", code)
	}
}
