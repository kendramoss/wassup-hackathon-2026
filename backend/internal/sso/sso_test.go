package sso

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/sso/ssotest"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
)

var ctx = context.Background()

func TestOIDCSignInProvisionsAndVerifies(t *testing.T) {
	p := ssotest.NewOIDCIdP(t)
	f := newFixture(t, p.Srv.Client())
	c := f.oidcConn(t, p)

	st, err := f.svc.Start(ctx, "New.User@ACME.com")
	if err != nil {
		t.Fatal(err)
	}
	if st.Protocol != ProtocolOIDC || st.ConnectionID != c.ID || !strings.HasPrefix(st.URL, p.Srv.URL+"/authorize?") {
		t.Fatalf("start = %+v", st)
	}
	u, _ := url.Parse(st.URL)
	if got := u.Query().Get("redirect_uri"); got != site+"/sso/oidc/callback" {
		t.Fatalf("redirect_uri = %q", got)
	}
	state, code := p.Authorize(t, st.URL, "new.user@acme.com")
	login, err := f.svc.FinishOIDC(ctx, state, code)
	if err != nil {
		t.Fatal(err)
	}
	if login.Email != "new.user@acme.com" || login.WorkspaceID != "ws_acme" || login.Role != teams.RoleViewer || !login.Provisioned {
		t.Fatalf("login = %+v", login)
	}
	user, err := f.svc.Verifier(nil).Verify(ctx, login.Token)
	if err != nil || user.Email != "new.user@acme.com" {
		t.Fatalf("verify = %+v %v", user, err)
	}
	// The state was single use.
	if _, err := f.svc.FinishOIDC(ctx, state, code); !errors.Is(err, ErrLogin) {
		t.Fatalf("reused state: %v", err)
	}
}

func TestOIDCRefusals(t *testing.T) {
	f2 := false
	bad := "wrong-nonce"
	cases := []struct {
		name  string
		setup func(p *ssotest.OIDCIdP)
		email string // the email the user signs in as at the provider
	}{
		{"wrong audience", func(p *ssotest.OIDCIdP) { p.Aud = "someone-else" }, "a@acme.com"},
		{"wrong nonce", func(p *ssotest.OIDCIdP) { p.Nonce = &bad }, "a@acme.com"},
		{"expired token", func(p *ssotest.OIDCIdP) { p.Expired = true }, "a@acme.com"},
		{"unverified email", func(p *ssotest.OIDCIdP) { p.EmailVerified = &f2 }, "a@acme.com"},
		{"email outside the connection's domains", func(p *ssotest.OIDCIdP) { p.Email = "a@evil.com" }, "a@acme.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := ssotest.NewOIDCIdP(t)
			f := newFixture(t, p.Srv.Client())
			f.oidcConn(t, p)
			tc.setup(p)
			st, err := f.svc.Start(ctx, tc.email)
			if err != nil {
				t.Fatal(err)
			}
			state, code := p.Authorize(t, st.URL, tc.email)
			_, err = f.svc.FinishOIDC(ctx, state, code)
			if !errors.Is(err, ErrLogin) {
				t.Fatalf("err = %v, want ErrLogin", err)
			}
			t.Log(err)
			if _, err := f.ws.Of(ctx, tc.email); !errors.Is(err, teams.ErrNotFound) {
				t.Fatalf("a refused sign-in provisioned the user: %v", err)
			}
		})
	}
}

func TestOIDCPKCEIsChecked(t *testing.T) {
	p := ssotest.NewOIDCIdP(t)
	f := newFixture(t, p.Srv.Client())
	f.oidcConn(t, p)
	st, err := f.svc.Start(ctx, "a@acme.com")
	if err != nil {
		t.Fatal(err)
	}
	state, code := p.Authorize(t, st.URL, "a@acme.com")
	p.Mutate(code, "not-the-challenge")
	if _, err := f.svc.FinishOIDC(ctx, state, code); !errors.Is(err, ErrLogin) {
		t.Fatalf("err = %v", err)
	}
}

func TestStartNeedsAVerifiedEnabledConnection(t *testing.T) {
	p := ssotest.NewOIDCIdP(t)
	f := newFixture(t, p.Srv.Client())
	c := f.oidcConn(t, p)
	if _, err := f.svc.Start(ctx, "a@unknown.com"); !errors.Is(err, ErrNoConnection) {
		t.Fatalf("unknown domain: %v", err)
	}
	c.Verified = map[string]time.Time{}
	if err := f.conns.Put(ctx, c); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Start(ctx, "a@acme.com"); !errors.Is(err, ErrNoConnection) {
		t.Fatalf("unverified domain: %v", err)
	}
}

func TestMemberOfAnotherWorkspaceIsRefused(t *testing.T) {
	p := ssotest.NewOIDCIdP(t)
	f := newFixture(t, p.Srv.Client())
	// other.com belongs to ws_other's user; a connection for ws_acme cannot
	// sign them in even when it holds the domain.
	f.put(t, Connection{Protocol: ProtocolOIDC, OIDCIssuer: p.Srv.URL, OIDCClientID: p.ClientID, Domains: []string{"other.com"}, OIDCSecret: mustSeal(t, f, p.Secret)})
	st, err := f.svc.Start(ctx, "x@other.com")
	if err != nil {
		t.Fatal(err)
	}
	state, code := p.Authorize(t, st.URL, "x@other.com")
	if _, err := f.svc.FinishOIDC(ctx, state, code); !errors.Is(err, ErrOtherWorkspace) {
		t.Fatalf("err = %v, want ErrOtherWorkspace", err)
	}
}

func mustSeal(t *testing.T, f fixture, s string) string {
	t.Helper()
	v, err := f.svc.Keys().Seal(s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func samlStart(t *testing.T, f fixture, email string) (relay, reqID string) {
	t.Helper()
	st, err := f.svc.Start(ctx, email)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(st.URL)
	if err != nil {
		t.Fatal(err)
	}
	relay = u.Query().Get("RelayState")
	b, err := f.state.Take(ctx, stateKey(relay))
	if err != nil {
		t.Fatal(err)
	}
	// put it back: we only peeked for the request ID
	if err := f.state.Put(ctx, stateKey(relay), b, time.Minute); err != nil {
		t.Fatal(err)
	}
	i := strings.Index(string(b), `"r":"`)
	reqID = string(b)[i+5:]
	reqID = reqID[:strings.IndexByte(reqID, '"')]
	return relay, reqID
}

func (f fixture) samlOK(reqID, email string) ssotest.SAMLOpts {
	return ssotest.SAMLOpts{InResponseTo: reqID, Email: email, Audience: f.svc.EntityID(), Recipient: f.svc.ACSURL(), Destination: f.svc.ACSURL()}
}

func TestSAMLSignIn(t *testing.T) {
	idp := ssotest.NewSAMLIdP(t)
	f := newFixture(t, nil)
	f.samlConn(t, idp)
	relay, reqID := samlStart(t, f, "m@acme.com")
	login, err := f.svc.FinishSAML(ctx, idp.Response(t, f.samlOK(reqID, "m@acme.com")), relay)
	if err != nil {
		t.Fatal(err)
	}
	if login.Email != "m@acme.com" || login.WorkspaceID != "ws_acme" || !login.Provisioned {
		t.Fatalf("login = %+v", login)
	}
	if _, err := f.svc.Verifier(nil).Verify(ctx, login.Token); err != nil {
		t.Fatal(err)
	}
}

func TestSAMLRefusals(t *testing.T) {
	other := ssotest.NewSAMLIdP(t)
	cases := []struct {
		name string
		mod  func(o *ssotest.SAMLOpts)
	}{
		{"unsigned", func(o *ssotest.SAMLOpts) { o.Unsigned = true }},
		{"signed by another key", func(o *ssotest.SAMLOpts) { o.Signer = other }},
		{"email changed after signing", func(o *ssotest.SAMLOpts) { o.TamperEmail = "boss@acme.com" }},
		{"wrong InResponseTo", func(o *ssotest.SAMLOpts) { o.InResponseTo = "_someone-elses-request" }},
		{"expired", func(o *ssotest.SAMLOpts) { o.NotOnOrAfter = time.Now().Add(-time.Minute) }},
		{"wrong audience", func(o *ssotest.SAMLOpts) { o.Audience = "https://other-sp.example.com" }},
		{"wrong recipient", func(o *ssotest.SAMLOpts) { o.Recipient = "https://evil.example.com/acs" }},
		{"wrong destination", func(o *ssotest.SAMLOpts) { o.Destination = "https://evil.example.com/acs" }},
		{"email outside the domains", func(o *ssotest.SAMLOpts) { o.Email = "m@evil.com" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idp := ssotest.NewSAMLIdP(t)
			f := newFixture(t, nil)
			f.samlConn(t, idp)
			relay, reqID := samlStart(t, f, "m@acme.com")
			o := f.samlOK(reqID, "m@acme.com")
			tc.mod(&o)
			_, err := f.svc.FinishSAML(ctx, idp.Response(t, o), relay)
			if !errors.Is(err, ErrLogin) {
				t.Fatalf("err = %v, want ErrLogin", err)
			}
			t.Log(err)
		})
	}
}

func TestSAMLReplayAndRelayReuse(t *testing.T) {
	idp := ssotest.NewSAMLIdP(t)
	f := newFixture(t, nil)
	c := f.samlConn(t, idp)
	relay, reqID := samlStart(t, f, "m@acme.com")
	o := f.samlOK(reqID, "m@acme.com")
	o.AssertionID = "_seen-before"
	// The same assertion ID was accepted earlier (from a stolen response).
	if err := f.state.Put(ctx, replayKey(c.ID, "_seen-before"), []byte{1}, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.FinishSAML(ctx, idp.Response(t, o), relay); !errors.Is(err, ErrLogin) {
		t.Fatalf("replayed assertion accepted: %v", err)
	}
	// The relay state was used up by that attempt.
	o.AssertionID = ""
	if _, err := f.svc.FinishSAML(ctx, idp.Response(t, o), relay); !errors.Is(err, ErrLogin) {
		t.Fatalf("relay state reused: %v", err)
	}
}

func TestSessionVerifierFollowsMembershipAndConnection(t *testing.T) {
	idp := ssotest.NewSAMLIdP(t)
	f := newFixture(t, nil)
	c := f.samlConn(t, idp)
	relay, reqID := samlStart(t, f, "m@acme.com")
	login, err := f.svc.FinishSAML(ctx, idp.Response(t, f.samlOK(reqID, "m@acme.com")), relay)
	if err != nil {
		t.Fatal(err)
	}
	v := f.svc.Verifier(nil)
	// Tampered token.
	if _, err := v.Verify(ctx, login.Token[:len(login.Token)-2]+"xx"); !errors.Is(err, teams.ErrUnauthenticated) {
		t.Fatalf("tampered: %v", err)
	}
	// Connection disabled.
	c.Enabled = false
	_ = f.conns.Put(ctx, c)
	if _, err := v.Verify(ctx, login.Token); !errors.Is(err, teams.ErrUnauthenticated) {
		t.Fatalf("disabled connection: %v", err)
	}
	c.Enabled = true
	_ = f.conns.Put(ctx, c)
	if _, err := v.Verify(ctx, login.Token); err != nil {
		t.Fatalf("re-enabled: %v", err)
	}
	// Removed from the workspace.
	if _, err := f.ws.Deprovision(ctx, "ws_acme", "m@acme.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(ctx, login.Token); !errors.Is(err, teams.ErrUnauthenticated) {
		t.Fatalf("removed member: %v", err)
	}
}

func TestSessionExpiry(t *testing.T) {
	keys, _ := NewKeys([]byte(strings.Repeat("z", 40)))
	now := time.Now()
	s := NewSigner(keys, time.Hour, func() time.Time { return now })
	tok, _, err := s.Mint("a@acme.com", "ws_acme", "sso_1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Parse(tok); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour)
	if _, err := s.Parse(tok); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("expired token accepted: %v", err)
	}
	// Another key's token.
	k2, _ := NewKeys([]byte(strings.Repeat("y", 40)))
	tok2, _, _ := NewSigner(k2, time.Hour, nil).Mint("a@acme.com", "ws_acme", "sso_1")
	if _, err := NewSigner(keys, time.Hour, nil).Parse(tok2); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("foreign token accepted: %v", err)
	}
}

type fakeGoogle struct{ email string }

func (g fakeGoogle) Verify(context.Context, string) (teams.User, error) {
	return teams.User{Email: g.email}, nil
}

func TestEnforcedConnectionRefusesGoogle(t *testing.T) {
	idp := ssotest.NewSAMLIdP(t)
	f := newFixture(t, nil)
	c := f.samlConn(t, idp)
	v := f.svc.Verifier(fakeGoogle{email: "m@acme.com"})
	if _, err := v.Verify(ctx, "google-token"); err != nil {
		t.Fatalf("not enforced: %v", err)
	}
	c.Enforced = true
	_ = f.conns.Put(ctx, c)
	v = f.svc.Verifier(fakeGoogle{email: "m@acme.com"}) // fresh cache
	if _, err := v.Verify(ctx, "google-token"); !errors.Is(err, teams.ErrUnauthenticated) {
		t.Fatalf("enforced: %v", err)
	}
	if _, err := f.svc.Verifier(fakeGoogle{email: "x@other.com"}).Verify(ctx, "google-token"); err != nil {
		t.Fatalf("other domain: %v", err)
	}
}

func TestDomainVerification(t *testing.T) {
	f := newFixture(t, nil)
	c := Connection{ID: "sso_a", Workspace: "ws_acme", Protocol: ProtocolSAML, Domains: []string{"acme.com", "acme.io"}, VerifyToken: "undelabs-verify-abc"}
	f.svc.resolver = fakeResolver{"_undelabs-verify.acme.com": {"undelabs-verify-abc"}, "_undelabs-verify.acme.io": {"wrong"}}
	c, checks, err := f.svc.VerifyDomains(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if !checks[0].Verified || checks[1].Verified || c.AllVerified() {
		t.Fatalf("checks = %+v", checks)
	}
	if err := f.conns.Put(ctx, c); err != nil {
		t.Fatal(err)
	}
	// Another workspace cannot verify a domain already verified here.
	o := Connection{ID: "sso_b", Workspace: "ws_other", Protocol: ProtocolSAML, Domains: []string{"acme.com"}, VerifyToken: "undelabs-verify-abc"}
	o, checks, err = f.svc.VerifyDomains(ctx, o)
	if err != nil {
		t.Fatal(err)
	}
	if checks[0].Verified || len(o.Verified) != 0 {
		t.Fatalf("takeover verified: %+v", checks)
	}
}

func TestNormalizeRules(t *testing.T) {
	for _, d := range []string{"gmail.com", "not a domain", "localhost"} {
		c := Connection{Protocol: ProtocolSAML, Domains: []string{d}}
		if err := c.Normalize(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: %v", d, err)
		}
	}
	c := Connection{Protocol: ProtocolOIDC, Domains: []string{"Acme.COM"}, OIDCIssuer: "http://idp.acme.com", OIDCClientID: "x"}
	if err := c.Normalize(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("http issuer accepted: %v", err)
	}
	c.OIDCIssuer = "https://idp.acme.com"
	c.Enabled = true
	if err := c.Normalize(); !errors.Is(err, ErrUnverified) {
		t.Fatalf("enabled unverified: %v", err)
	}
	c.Enabled = false
	if err := c.Normalize(); err != nil || c.Domains[0] != "acme.com" || c.DefaultRole != "viewer" {
		t.Fatalf("normalize = %+v %v", c, err)
	}
}

func TestSealOpen(t *testing.T) {
	k, _ := NewKeys([]byte(strings.Repeat("q", 32)))
	s, err := k.Seal("client-secret")
	if err != nil || strings.Contains(s, "client-secret") {
		t.Fatalf("seal = %q %v", s, err)
	}
	if got, err := k.Open(s); err != nil || got != "client-secret" {
		t.Fatalf("open = %q %v", got, err)
	}
	k2, _ := NewKeys([]byte(strings.Repeat("w", 32)))
	if _, err := k2.Open(s); err == nil {
		t.Fatal("opened with another key")
	}
	if _, err := NewKeys([]byte("short")); err == nil {
		t.Fatal("short key accepted")
	}
}

func TestPublicOnlyDialer(t *testing.T) {
	for _, a := range []string{"127.0.0.1:443", "169.254.169.254:80", "10.0.0.1:443", "192.168.1.1:443", "[::1]:443", "100.64.0.1:443", "0.0.0.0:443", "[fd00::1]:443"} {
		if err := publicOnly("tcp", a, nil); err == nil {
			t.Errorf("%s allowed", a)
		}
	}
	if err := publicOnly("tcp", "8.8.8.8:443", nil); err != nil {
		t.Errorf("public address refused: %v", err)
	}
}
