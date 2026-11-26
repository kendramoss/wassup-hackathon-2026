package httpapi

import (
	"bytes"
	"compress/flate"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/sso"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/sso/ssotest"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
)

// txt is a settable DNS resolver for the domain checks.
type txt struct {
	mu sync.Mutex
	m  map[string][]string
}

func (r *txt) LookupTXT(_ context.Context, name string) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if v, ok := r.m[name]; ok {
		return v, nil
	}
	return nil, &url.Error{Op: "lookup", URL: name, Err: io.EOF}
}

func (r *txt) set(name, value string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[name] = []string{value}
}

const ssoSite = "https://app.example.com"

func ssoHarness(t *testing.T, client *http.Client) (harness, *txt) {
	t.Helper()
	res := &txt{m: map[string][]string{}}
	hs, _, _ := teamsHarness(t, func(d *Deps) {
		keys, err := sso.NewKeys([]byte(strings.Repeat("s", 32)))
		if err != nil {
			t.Fatal(err)
		}
		svc, err := sso.New(sso.Config{SiteURL: ssoSite, HTTPClient: client, Resolver: res}, keys, sso.NewMemoryStore(), sso.NewMemoryOneTime(nil), d.Workspaces)
		if err != nil {
			t.Fatal(err)
		}
		d.SSO = svc
		d.Users = svc.Verifier(d.Users)
	})
	return hs, res
}

func doRaw(t *testing.T, hs harness, method, path, body string, headers map[string]string) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

// samlRequestID reads the AuthnRequest ID out of a redirect-binding URL.
func samlRequestID(t *testing.T, u string) (relay, id string) {
	t.Helper()
	pu, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(pu.Query().Get("SAMLRequest"))
	if err != nil {
		t.Fatal(err)
	}
	x, err := io.ReadAll(flate.NewReader(bytes.NewReader(raw)))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`ID="([^"]+)"`).FindSubmatch(x)
	if m == nil {
		t.Fatalf("no ID in %s", x)
	}
	return pu.Query().Get("RelayState"), string(m[1])
}

func TestSSOConnectionAdminAndSAMLSignIn(t *testing.T) {
	idp := ssotest.NewSAMLIdP(t)
	hs, res := ssoHarness(t, nil)

	code, out := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Acme"}`, user("alice"))
	if code != http.StatusCreated {
		t.Fatalf("create workspace = %d %v", code, out)
	}
	ws := out["id"].(string)
	hs.addToWorkspace(t, ws, "alice", "carol") // an editor

	certs, _ := json.Marshal([]string{idp.CertPEM})
	body := `{"protocol":"saml","domains":["Example.com"],"enabled":true,"saml_sso_url":"https://idp.example.org/sso","saml_issuer":"` + idp.Issuer + `","saml_certs_pem":` + string(certs) + `}`

	// Only an admin of the workspace manages its connections.
	if code, _ := hs.doAs(t, "POST", "/v1/workspaces/"+ws+"/sso", body, user("carol")); code != http.StatusForbidden {
		t.Fatalf("editor create = %d", code)
	}
	if code, _ := hs.doAs(t, "GET", "/v1/workspaces/"+ws+"/sso", "", user("bob")); code != http.StatusNotFound {
		t.Fatalf("outsider list = %d", code)
	}
	if code, _ := hs.doAs(t, "POST", "/v1/workspaces/"+ws+"/sso", body, nil); code != http.StatusUnauthorized {
		t.Fatalf("anonymous create = %d", code)
	}

	code, out = hs.doAs(t, "POST", "/v1/workspaces/"+ws+"/sso", body, user("alice"))
	if code != http.StatusCreated {
		t.Fatalf("create = %d %v", code, out)
	}
	conn := out["id"].(string)
	if out["enabled"] != false {
		t.Fatalf("a new connection must start off (domains unverified): %v", out)
	}
	dom := out["domains"].([]any)[0].(map[string]any)
	if dom["domain"] != "example.com" || dom["verified"] != false || dom["record_name"] != "_undelabs-verify.example.com" {
		t.Fatalf("domain = %v", dom)
	}
	sp := out["service_provider"].(map[string]any)
	if sp["saml_acs_url"] != ssoSite+"/sso/saml/acs" {
		t.Fatalf("service provider = %v", sp)
	}

	// Not before the domain is verified.
	if code, out := hs.doAs(t, "PUT", "/v1/workspaces/"+ws+"/sso/"+conn, body, user("alice")); code != http.StatusConflict {
		t.Fatalf("enable unverified = %d %v", code, out)
	}
	if code, out := hs.doAs(t, "POST", "/v1/sso/start", `{"email":"dave@example.com"}`, nil); code != http.StatusNotFound {
		t.Fatalf("start before verification = %d %v", code, out)
	}
	code, out = hs.doAs(t, "POST", "/v1/workspaces/"+ws+"/sso/"+conn+"/verify", `{"enable":true}`, user("alice"))
	if code != http.StatusOK || out["enabled"] != false {
		t.Fatalf("verify without record = %d %v", code, out)
	}
	res.set("_undelabs-verify.example.com", dom["record_value"].(string))
	code, out = hs.doAs(t, "POST", "/v1/workspaces/"+ws+"/sso/"+conn+"/verify", `{"enable":true}`, user("alice"))
	if code != http.StatusOK || out["enabled"] != true {
		t.Fatalf("verify = %d %v", code, out)
	}

	// Sign in.
	code, out = hs.doAs(t, "POST", "/v1/sso/start", `{"email":"Dave@Example.com"}`, nil)
	if code != http.StatusOK || out["protocol"] != "saml" {
		t.Fatalf("start = %d %v", code, out)
	}
	relay, reqID := samlRequestID(t, out["url"].(string))
	samlResp := idp.Response(t, ssotest.SAMLOpts{InResponseTo: reqID, Email: "dave@example.com", Audience: ssoSite + "/sso/saml/metadata", Recipient: ssoSite + "/sso/saml/acs", Destination: ssoSite + "/sso/saml/acs"})
	form := url.Values{"SAMLResponse": {samlResp}, "RelayState": {relay}}.Encode()
	formHdr := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	code, out = hs.doAs(t, "POST", "/v1/sso/saml/acs", form, formHdr)
	if code != http.StatusOK || out["provisioned"] != true || out["role"] != "viewer" || out["workspace_id"] != ws {
		t.Fatalf("acs = %d %v", code, out)
	}
	tok := out["token"].(string)
	if !strings.HasPrefix(tok, sso.TokenPrefix) {
		t.Fatalf("token = %q", tok)
	}
	// The session works as X-User-Token.
	code, out = hs.doAs(t, "GET", "/v1/workspaces/mine", "", map[string]string{"X-User-Token": tok})
	if code != http.StatusOK || out["email"] != "dave@example.com" {
		t.Fatalf("mine = %d %v", code, out)
	}
	if w, _ := out["workspace"].(map[string]any); w == nil || w["id"] != ws {
		t.Fatalf("workspace = %v", out["workspace"])
	}
	// A viewer cannot manage SSO.
	if code, _ := hs.doAs(t, "GET", "/v1/workspaces/"+ws+"/sso", "", map[string]string{"X-User-Token": tok}); code != http.StatusForbidden {
		t.Fatalf("viewer list = %d", code)
	}
	// The same response again is refused (relay state used, assertion seen).
	if code, out := hs.doAs(t, "POST", "/v1/sso/saml/acs", form, formHdr); code != http.StatusUnauthorized || out["code"] != "sso_failed" {
		t.Fatalf("replay = %d %v", code, out)
	}
	// Deleting the connection ends its sessions.
	if code, _ := hs.doAs(t, "DELETE", "/v1/workspaces/"+ws+"/sso/"+conn, "", user("alice")); code != http.StatusNoContent {
		t.Fatalf("delete = %d", code)
	}
	if code, _ := hs.doAs(t, "GET", "/v1/workspaces/mine", "", map[string]string{"X-User-Token": tok}); code != http.StatusUnauthorized {
		t.Fatalf("session after delete = %d", code)
	}
}

func TestSSOOIDCSecretIsNeverReturned(t *testing.T) {
	idp := ssotest.NewOIDCIdP(t)
	hs, res := ssoHarness(t, idp.Srv.Client())
	_, out := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Acme"}`, user("alice"))
	ws := out["id"].(string)
	body := `{"protocol":"oidc","domains":["example.com"],"oidc_issuer":"` + idp.Srv.URL + `","oidc_client_id":"` + idp.ClientID + `","oidc_client_secret":"` + idp.Secret + `"}`
	code, raw := doRaw(t, hs, "POST", "/v1/workspaces/"+ws+"/sso", body, user("alice"))
	if code != http.StatusCreated {
		t.Fatalf("create = %d %s", code, raw)
	}
	var created map[string]any
	_ = json.Unmarshal(raw, &created)
	if bytes.Contains(raw, []byte(idp.Secret)) || bytes.Contains(raw, []byte("v1:")) || created["has_client_secret"] != true {
		t.Fatalf("secret leaked or missing: %s", raw)
	}
	conn := created["id"].(string)
	res.set("_undelabs-verify.example.com", created["domains"].([]any)[0].(map[string]any)["record_value"].(string))
	if code, out := hs.doAs(t, "POST", "/v1/workspaces/"+ws+"/sso/"+conn+"/verify", `{"enable":true}`, user("alice")); code != http.StatusOK || out["enabled"] != true {
		t.Fatalf("verify = %d %v", code, out)
	}
	code, raw = doRaw(t, hs, "GET", "/v1/workspaces/"+ws+"/sso", "", user("alice"))
	if code != http.StatusOK || bytes.Contains(raw, []byte(idp.Secret)) {
		t.Fatalf("list = %d %s", code, raw)
	}
	// Full OIDC sign-in through the routes.
	code, out = hs.doAs(t, "POST", "/v1/sso/start", `{"email":"erin@example.com"}`, nil)
	if code != http.StatusOK {
		t.Fatalf("start = %d %v", code, out)
	}
	state, authCode := idp.Authorize(t, out["url"].(string), "erin@example.com")
	code, out = hs.doAs(t, "POST", "/v1/sso/oidc/callback", `{"state":"`+state+`","code":"`+authCode+`"}`, nil)
	if code != http.StatusOK || out["email"] != "erin@example.com" {
		t.Fatalf("callback = %d %v", code, out)
	}
	// A forged state is refused without detail.
	code, out = hs.doAs(t, "POST", "/v1/sso/oidc/callback", `{"state":"forged","code":"x"}`, nil)
	if code != http.StatusUnauthorized || strings.Contains(out["message"].(string), "state") {
		t.Fatalf("forged = %d %v", code, out)
	}
}

func TestSSORoutesAreOffByDefault(t *testing.T) {
	hs, _, _ := teamsHarness(t, nil)
	for _, r := range [][2]string{{"POST", "/v1/sso/start"}, {"POST", "/v1/sso/saml/acs"}, {"GET", "/v1/sso/saml/metadata"}, {"GET", "/v1/workspaces/ws_000000000000/sso"}} {
		if code, _ := hs.doAs(t, r[0], r[1], `{}`, user("alice")); code != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404", r[0], r[1], code)
		}
	}
}

func TestSSOSessionForAnotherWorkspaceIsRefused(t *testing.T) {
	// A connection's session is only good while the user is in its workspace.
	idp := ssotest.NewSAMLIdP(t)
	hs, res := ssoHarness(t, nil)
	_, out := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Acme"}`, user("alice"))
	ws := out["id"].(string)
	// frank is already in another workspace.
	if code, _ := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Frank Co"}`, user("frank")); code != http.StatusCreated {
		t.Fatal("frank's workspace")
	}
	certs, _ := json.Marshal([]string{idp.CertPEM})
	_, out = hs.doAs(t, "POST", "/v1/workspaces/"+ws+"/sso", `{"protocol":"saml","domains":["example.com"],"saml_sso_url":"https://idp.example.org/sso","saml_issuer":"`+idp.Issuer+`","saml_certs_pem":`+string(certs)+`}`, user("alice"))
	conn := out["id"].(string)
	res.set("_undelabs-verify.example.com", out["domains"].([]any)[0].(map[string]any)["record_value"].(string))
	hs.doAs(t, "POST", "/v1/workspaces/"+ws+"/sso/"+conn+"/verify", `{"enable":true}`, user("alice"))
	_, out = hs.doAs(t, "POST", "/v1/sso/start", `{"email":"frank@example.com"}`, nil)
	relay, reqID := samlRequestID(t, out["url"].(string))
	resp := idp.Response(t, ssotest.SAMLOpts{InResponseTo: reqID, Email: "frank@example.com", Audience: ssoSite + "/sso/saml/metadata", Recipient: ssoSite + "/sso/saml/acs", Destination: ssoSite + "/sso/saml/acs"})
	code, out := hs.doAs(t, "POST", "/v1/sso/saml/acs", `{"saml_response":"`+resp+`","relay_state":"`+relay+`"}`, nil)
	if code != http.StatusForbidden || out["code"] != "sso_other_workspace" {
		t.Fatalf("acs = %d %v", code, out)
	}
	_ = teams.RoleAdmin
}
