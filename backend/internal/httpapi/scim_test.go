package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/scim"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/sso"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/sso/ssotest"
)

type scimEnv struct {
	hs  harness
	res *txt
	idp *ssotest.SAMLIdP
}

func newSCIMEnv(t *testing.T, failuresPerMin int) *scimEnv {
	t.Helper()
	e := &scimEnv{res: &txt{m: map[string][]string{}}, idp: ssotest.NewSAMLIdP(t)}
	e.hs, _, _ = teamsHarness(t, func(d *Deps) {
		keys, err := sso.NewKeys([]byte(strings.Repeat("s", 32)))
		if err != nil {
			t.Fatal(err)
		}
		svc, err := sso.New(sso.Config{SiteURL: ssoSite, Resolver: e.res}, keys, sso.NewMemoryStore(), sso.NewMemoryOneTime(nil), d.Workspaces)
		if err != nil {
			t.Fatal(err)
		}
		d.SSO, d.Users, d.SCIM = svc, svc.Verifier(d.Users), scim.NewMemory()
		if failuresPerMin > 0 {
			d.Limits.AuthFailuresPerMin = failuresPerMin
		}
	})
	return e
}

// workspace makes admin's workspace with a verified SSO domain.
func (e *scimEnv) workspace(t *testing.T, admin, domain string) string {
	t.Helper()
	code, out := e.hs.doAs(t, "POST", "/v1/workspaces", `{"name":"W"}`, user(admin))
	if code != http.StatusCreated {
		t.Fatalf("workspace = %d %v", code, out)
	}
	ws := out["id"].(string)
	certs, _ := json.Marshal([]string{e.idp.CertPEM})
	code, out = e.hs.doAs(t, "POST", "/v1/workspaces/"+ws+"/sso", `{"protocol":"saml","domains":["`+domain+`"],"saml_sso_url":"https://idp.example.net/sso","saml_issuer":"`+e.idp.Issuer+`","saml_certs_pem":`+string(certs)+`}`, user(admin))
	if code != http.StatusCreated {
		t.Fatalf("sso = %d %v", code, out)
	}
	conn := out["id"].(string)
	e.res.set("_undelabs-verify."+domain, out["domains"].([]any)[0].(map[string]any)["record_value"].(string))
	if code, out := e.hs.doAs(t, "POST", "/v1/workspaces/"+ws+"/sso/"+conn+"/verify", `{"enable":true}`, user(admin)); code != http.StatusOK || out["enabled"] != true {
		t.Fatalf("verify = %d %v", code, out)
	}
	return ws
}

func (e *scimEnv) token(t *testing.T, admin, ws, body string) string {
	t.Helper()
	code, out := e.hs.doAs(t, "POST", "/v1/workspaces/"+ws+"/scim/token", body, user(admin))
	if code != http.StatusCreated {
		t.Fatalf("token = %d %v", code, out)
	}
	return out["token"].(string)
}

func scimAs(tok string) map[string]string {
	return map[string]string{scimTokenHeader: tok, "Content-Type": scim.ContentType}
}

func TestSCIMTokenIsAdminOnlyAndShownOnce(t *testing.T) {
	e := newSCIMEnv(t, 0)
	ws := e.workspace(t, "alice", "example.com")
	e.hs.addToWorkspace(t, ws, "alice", "carol")
	if code, _ := e.hs.doAs(t, "POST", "/v1/workspaces/"+ws+"/scim/token", `{"domains":["example.com"]}`, user("carol")); code != http.StatusForbidden {
		t.Fatalf("editor issue = %d", code)
	}
	if code, _ := e.hs.doAs(t, "GET", "/v1/workspaces/"+ws+"/scim", "", user("bob")); code != http.StatusNotFound {
		t.Fatalf("outsider status = %d", code)
	}
	for body, want := range map[string]string{
		`{"domains":["example.org"]}`:                        "scim_unverified_domain", // not verified on this workspace
		`{"domains":["gmail.com"]}`:                          "invalid_input",
		`{"domains":[]}`:                                     "invalid_input",
		`{"domains":["example.com"],"default_role":"admin"}`: "invalid_input",
		`{"domains":["example.com"],"default_role":"owner"}`: "invalid_input",
	} {
		if code, out := e.hs.doAs(t, "POST", "/v1/workspaces/"+ws+"/scim/token", body, user("alice")); out["code"] != want {
			t.Errorf("%s = %d %v, want %s", body, code, out, want)
		}
	}
	code, out := e.hs.doAs(t, "GET", "/v1/workspaces/"+ws+"/scim", "", user("alice"))
	if code != http.StatusOK || out["configured"] != false || out["base_url"] != ssoSite+"/api/v1/scim/v2" {
		t.Fatalf("status = %d %v", code, out)
	}
	tok := e.token(t, "alice", ws, `{"domains":["Example.com"]}`)
	if !scim.WellFormed(tok) {
		t.Fatalf("token %q", tok)
	}
	_, raw := doRaw(t, e.hs, "GET", "/v1/workspaces/"+ws+"/scim", "", user("alice"))
	if strings.Contains(string(raw), tok) || strings.Contains(string(raw), scim.Hash(tok)) || !strings.Contains(string(raw), `"hint":"`+tok[len(tok)-4:]+`"`) {
		t.Fatalf("status leaks the token or lacks the hint: %s", raw)
	}
	if code, _ := e.hs.doAs(t, "GET", "/v1/scim/v2/Users", "", scimAs(tok)); code != http.StatusOK {
		t.Fatalf("token works = %d", code)
	}
	// Reissuing replaces it; revoking ends it.
	tok2 := e.token(t, "alice", ws, `{"domains":["example.com"]}`)
	if code, _ := e.hs.doAs(t, "GET", "/v1/scim/v2/Users", "", scimAs(tok)); code != http.StatusUnauthorized {
		t.Fatalf("replaced token = %d", code)
	}
	if code, _ := e.hs.doAs(t, "DELETE", "/v1/workspaces/"+ws+"/scim/token", "", user("alice")); code != http.StatusNoContent {
		t.Fatalf("revoke = %d", code)
	}
	if code, _ := e.hs.doAs(t, "GET", "/v1/scim/v2/Users", "", scimAs(tok2)); code != http.StatusUnauthorized {
		t.Fatalf("revoked token = %d", code)
	}
}

func TestSCIMProvisioningLifecycle(t *testing.T) {
	e := newSCIMEnv(t, 0)
	ws := e.workspace(t, "alice", "example.com")
	tok := e.token(t, "alice", ws, `{"domains":["example.com"]}`)
	h := scimAs(tok)

	code, out := e.hs.doAs(t, "POST", "/v1/scim/v2/Users", `{"schemas":["`+scim.SchemaUser+`"],"userName":"Dave@Example.com","name":{"givenName":"Dave"},"externalId":"00u1","active":true}`, h)
	if code != http.StatusCreated || out["userName"] != "dave@example.com" || out["active"] != true {
		t.Fatalf("create = %d %v", code, out)
	}
	uid := out["id"].(string)
	if out["roles"].([]any)[0].(map[string]any)["value"] != "viewer" || !strings.HasSuffix(out["meta"].(map[string]any)["location"].(string), "/api/v1/scim/v2/Users/"+uid) {
		t.Fatalf("create = %v", out)
	}
	// dave is now a member, through his own sign-in.
	if code, out := e.hs.doAs(t, "GET", "/v1/workspaces/mine", "", user("dave")); code != http.StatusOK || out["workspace"].(map[string]any)["id"] != ws {
		t.Fatalf("dave mine = %d %v", code, out)
	}
	if code, out := e.hs.doAs(t, "POST", "/v1/scim/v2/Users", `{"userName":"dave@example.com"}`, h); code != http.StatusConflict || out["scimType"] != "uniqueness" {
		t.Fatalf("duplicate = %d %v", code, out)
	}
	code, out = e.hs.doAs(t, "GET", `/v1/scim/v2/Users?filter=userName+eq+%22DAVE%40example.com%22`, "", h)
	if code != http.StatusOK || out["totalResults"].(float64) != 1 || out["Resources"].([]any)[0].(map[string]any)["id"] != uid {
		t.Fatalf("filter = %d %v", code, out)
	}
	if code, out := e.hs.doAs(t, "GET", `/v1/scim/v2/Users?filter=userName+eq+%22nobody%40example.com%22`, "", h); code != http.StatusOK || out["totalResults"].(float64) != 0 {
		t.Fatalf("filter miss = %d %v", code, out)
	}
	if code, out := e.hs.doAs(t, "GET", `/v1/scim/v2/Users?filter=displayName+co+%22d%22`, "", h); code != http.StatusBadRequest || out["scimType"] != "invalidFilter" {
		t.Fatalf("bad filter = %d %v", code, out)
	}
	code, out = e.hs.doAs(t, "PATCH", "/v1/scim/v2/Users/"+uid, `{"schemas":["`+scim.SchemaPatch+`"],"Operations":[{"op":"replace","path":"roles","value":[{"value":"editor"}]}]}`, h)
	if code != http.StatusOK || out["roles"].([]any)[0].(map[string]any)["value"] != "editor" {
		t.Fatalf("patch role = %d %v", code, out)
	}
	if code, out := e.hs.doAs(t, "PUT", "/v1/scim/v2/Users/"+uid, `{"userName":"other@example.com","active":true}`, h); code != http.StatusBadRequest || out["scimType"] != "mutability" {
		t.Fatalf("rename = %d %v", code, out)
	}
	// Entra ID deactivates with a string boolean: the member is removed.
	code, out = e.hs.doAs(t, "PATCH", "/v1/scim/v2/Users/"+uid, `{"Operations":[{"op":"Replace","path":"active","value":"False"}]}`, h)
	if code != http.StatusOK || out["active"] != false {
		t.Fatalf("deactivate = %d %v", code, out)
	}
	if code, _ := e.hs.doAs(t, "GET", "/v1/scim/v2/Users/"+uid, "", h); code != http.StatusNotFound {
		t.Fatalf("after deactivate = %d", code)
	}
	if code, _ := e.hs.doAs(t, "GET", "/v1/workspaces/mine", "", user("dave")); code != http.StatusOK {
		t.Fatal("mine")
	} else if _, out := e.hs.doAs(t, "GET", "/v1/workspaces/mine", "", user("dave")); out["workspace"] != nil {
		t.Fatalf("dave still in a workspace: %v", out)
	}
	// Provision again, then DELETE.
	_, out = e.hs.doAs(t, "POST", "/v1/scim/v2/Users", `{"userName":"dave@example.com","roles":["editor"]}`, h)
	if code, _ := e.hs.doAs(t, "DELETE", "/v1/scim/v2/Users/"+out["id"].(string), "", h); code != http.StatusNoContent {
		t.Fatalf("delete = %d", code)
	}
	for _, p := range []string{"ServiceProviderConfig", "ResourceTypes", "Schemas"} {
		if code, _ := e.hs.doAs(t, "GET", "/v1/scim/v2/"+p, "", h); code != http.StatusOK {
			t.Errorf("%s = %d", p, code)
		}
	}
	// A direct call may send the token as the Authorization bearer.
	if code, _ := e.hs.doAs(t, "GET", "/v1/scim/v2/Users", "", map[string]string{"Authorization": "Bearer " + tok}); code != http.StatusOK {
		t.Fatalf("bearer = %d", code)
	}
}

func TestSCIMCannotTouchAdminsUnlessAllowed(t *testing.T) {
	e := newSCIMEnv(t, 0)
	ws := e.workspace(t, "alice", "example.com")
	h := scimAs(e.token(t, "alice", ws, `{"domains":["example.com"]}`))
	_, out := e.hs.doAs(t, "GET", `/v1/scim/v2/Users?filter=userName+eq+%22alice%40example.com%22`, "", h)
	alice := out["Resources"].([]any)[0].(map[string]any)["id"].(string)
	for _, r := range [][3]string{
		{"PATCH", "/v1/scim/v2/Users/" + alice, `{"Operations":[{"op":"replace","path":"roles","value":"viewer"}]}`},
		{"PATCH", "/v1/scim/v2/Users/" + alice, `{"Operations":[{"op":"replace","path":"active","value":false}]}`},
		{"DELETE", "/v1/scim/v2/Users/" + alice, ""},
		{"POST", "/v1/scim/v2/Users", `{"userName":"mallory@example.com","roles":[{"value":"admin"}]}`},
	} {
		if code, out := e.hs.doAs(t, r[0], r[1], r[2], h); code != http.StatusForbidden {
			t.Errorf("%s %s = %d %v", r[0], r[1], code, out)
		}
	}
	if code, out := e.hs.doAs(t, "GET", "/v1/workspaces/mine", "", user("alice")); code != http.StatusOK || out["workspace"].(map[string]any)["role"] != "admin" {
		t.Fatalf("alice = %v", out)
	}
	if code, _ := e.hs.doAs(t, "GET", `/v1/workspaces/mine`, "", user("mallory")); code != http.StatusOK {
		t.Fatal("mallory")
	} else if _, out := e.hs.doAs(t, "GET", "/v1/workspaces/mine", "", user("mallory")); out["workspace"] != nil {
		t.Fatal("mallory was added")
	}
	// With allow_admin: an admin can be made, but the last one cannot go.
	h = scimAs(e.token(t, "alice", ws, `{"domains":["example.com"],"allow_admin":true}`))
	if code, out := e.hs.doAs(t, "DELETE", "/v1/scim/v2/Users/"+alice, "", h); code != http.StatusConflict {
		t.Fatalf("last admin = %d %v", code, out)
	}
	if code, out := e.hs.doAs(t, "POST", "/v1/scim/v2/Users", `{"userName":"mallory@example.com","roles":[{"value":"admin"}]}`, h); code != http.StatusCreated {
		t.Fatalf("admin with allow_admin = %d %v", code, out)
	}
	if code, _ := e.hs.doAs(t, "DELETE", "/v1/scim/v2/Users/"+alice, "", h); code != http.StatusNoContent {
		t.Fatalf("second admin removes first = %d", code)
	}
}

func TestSCIMIsScopedToItsWorkspaceAndDomains(t *testing.T) {
	e := newSCIMEnv(t, 0)
	ws1 := e.workspace(t, "alice", "example.com")
	ws2 := e.workspace(t, "zed", "example.org")
	h1 := scimAs(e.token(t, "alice", ws1, `{"domains":["example.com"]}`))
	h2 := scimAs(e.token(t, "zed", ws2, `{"domains":["example.org"]}`))
	// Another domain is not this token's to provision.
	if code, out := e.hs.doAs(t, "POST", "/v1/scim/v2/Users", `{"userName":"fay@example.org"}`, h1); code != http.StatusForbidden {
		t.Fatalf("other domain = %d %v", code, out)
	}
	_, out := e.hs.doAs(t, "POST", "/v1/scim/v2/Users", `{"userName":"fay@example.org"}`, h2)
	fay := out["id"].(string)
	if code, _ := e.hs.doAs(t, "GET", "/v1/scim/v2/Users/"+fay, "", h1); code != http.StatusNotFound {
		t.Fatalf("other workspace's user = %d", code)
	}
	if code, _ := e.hs.doAs(t, "DELETE", "/v1/scim/v2/Users/"+fay, "", h1); code != http.StatusNotFound {
		t.Fatalf("delete other workspace's user = %d", code)
	}
	_, out = e.hs.doAs(t, "GET", "/v1/scim/v2/Users", "", h1)
	for _, u := range out["Resources"].([]any) {
		if n := u.(map[string]any)["userName"].(string); !strings.HasSuffix(n, "@example.com") {
			t.Errorf("token 1 sees %s", n)
		}
	}
	// zed (example.com, ws2's admin) is invisible to ws2's token, which
	// manages example.org only.
	_, out = e.hs.doAs(t, "GET", "/v1/scim/v2/Users", "", h2)
	if out["totalResults"].(float64) != 1 {
		t.Fatalf("token 2 list = %v", out)
	}
	if code, _ := e.hs.doAs(t, "GET", "/v1/scim/v2/Users/"+scim.UserID(ws2, "zed@example.com"), "", h2); code != http.StatusNotFound {
		t.Fatalf("member outside the token's domains = %d", code)
	}
	// Someone already in another workspace: a 409 that names nothing.
	code, raw := doRaw(t, e.hs, "POST", "/v1/scim/v2/Users", `{"userName":"zed@example.com"}`, h1)
	if code != http.StatusConflict || strings.Contains(string(raw), ws2) {
		t.Fatalf("member elsewhere = %d %s", code, raw)
	}
}

func TestSCIMRefusesBadTokens(t *testing.T) {
	e := newSCIMEnv(t, 4)
	ws := e.workspace(t, "alice", "example.com")
	good := e.token(t, "alice", ws, `{"domains":["example.com"]}`)
	fake, _ := scim.NewSecret()
	for _, h := range []map[string]string{nil, scimAs("nope"), scimAs(fake), {"Authorization": "Bearer op_whatever"}} {
		code, out := e.hs.doAs(t, "GET", "/v1/scim/v2/Users", "", h)
		if code != http.StatusUnauthorized || out["schemas"].([]any)[0] != scim.SchemaError {
			t.Errorf("%v = %d %v", h, code, out)
		}
	}
	// Four failures from this client: even the good token now waits.
	if code, _ := e.hs.doAs(t, "GET", "/v1/scim/v2/Users", "", scimAs(good)); code != http.StatusTooManyRequests {
		t.Fatalf("after failures = %d", code)
	}
	// A signed-in admin is not a SCIM caller.
	if code, _ := e.hs.doAs(t, "GET", "/v1/scim/v2/Users", "", user("alice")); code != http.StatusTooManyRequests && code != http.StatusUnauthorized {
		t.Fatalf("user on scim = %d", code)
	}
}

func TestSCIMRoutesAreOffByDefault(t *testing.T) {
	hs, _ := ssoHarness(t, nil) // SSO on, SCIM off
	for _, r := range [][2]string{{"GET", "/v1/scim/v2/Users"}, {"POST", "/v1/scim/v2/Users"}, {"GET", "/v1/workspaces/ws_000000000000/scim"}} {
		if code, _ := hs.doAs(t, r[0], r[1], `{}`, nil); code != http.StatusNotFound {
			t.Errorf("%s %s = %d", r[0], r[1], code)
		}
	}
}

func TestSCIMEventsAreTheWorkspacesSystemActor(t *testing.T) {
	e := newAuditEnv(t)
	secret, hash := scim.NewSecret()
	if err := e.scim.Put(context.Background(), scim.Token{Workspace: e.wsID, Hash: hash, Domains: []string{"idp.example.org"}, DefaultRole: "viewer"}); err != nil {
		t.Fatal(err)
	}
	if code, out := e.hs.doAs(t, "POST", "/v1/scim/v2/Users", `{"userName":"pat@idp.example.org","roles":"editor"}`, map[string]string{scimTokenHeader: secret}); code != http.StatusCreated {
		t.Fatalf("create = %d %v", code, out)
	}
	evs := e.events(e.tenant())
	last := evs[len(evs)-1]
	if last.Action != audit.SCIMUserCreate || last.ActorKind != audit.ActorSystem || last.Actor != "" || last.Detail != "editor" || last.Target == "" || strings.Contains(last.Target, "pat") {
		t.Fatalf("event = %+v", last)
	}
	// A wrong token is recorded as a denial, with nothing it claimed.
	if code, _ := e.hs.doAs(t, "GET", "/v1/scim/v2/Users", "", map[string]string{scimTokenHeader: "scim_" + strings.Repeat("A", 43)}); code != http.StatusUnauthorized {
		t.Fatal(code)
	}
	evs = e.events("platform")
	if last := evs[len(evs)-1]; last.Action != audit.SCIMUserRead || last.Outcome != audit.Denied || last.ActorKind != audit.ActorAnonymous {
		t.Fatalf("denial = %+v", last)
	}
}
