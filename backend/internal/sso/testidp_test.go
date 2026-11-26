package sso

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/sso/ssotest"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
)

// --- shared fixture ---------------------------------------------------------

type fakeResolver map[string][]string

func (f fakeResolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	if v, ok := f[name]; ok {
		return v, nil
	}
	return nil, &netError{name}
}

type netError struct{ name string }

func (e *netError) Error() string { return "no such host " + e.name }

type fixture struct {
	svc   *Service
	ws    *teams.Memory
	conns *MemoryStore
	state *MemoryOneTime
}

const site = "https://app.example.com"

func newFixture(t *testing.T, client *http.Client) fixture {
	t.Helper()
	keys, err := NewKeys([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	ws := teams.NewMemory()
	ctx := context.Background()
	for _, w := range []teams.Workspace{
		{ID: "ws_acme", Name: "Acme", Members: []string{"boss@acme.com"}, CreatedBy: "boss@acme.com", Roles: map[string]string{"boss@acme.com": "admin"}},
		{ID: "ws_other", Name: "Other", Members: []string{"x@other.com"}, CreatedBy: "x@other.com"},
	} {
		if err := ws.Create(ctx, w); err != nil {
			t.Fatal(err)
		}
	}
	conns, state := NewMemoryStore(), NewMemoryOneTime(nil)
	svc, err := New(Config{SiteURL: site, HTTPClient: client, Resolver: fakeResolver{}}, keys, conns, state, ws)
	if err != nil {
		t.Fatal(err)
	}
	return fixture{svc: svc, ws: ws, conns: conns, state: state}
}

func (f fixture) put(t *testing.T, c Connection) Connection {
	t.Helper()
	if c.ID == "" {
		c.ID = NewConnectionID()
	}
	if c.Workspace == "" {
		c.Workspace = "ws_acme"
	}
	if c.Domains == nil {
		c.Domains = []string{"acme.com"}
	}
	if c.Verified == nil {
		c.Verified = map[string]time.Time{}
		for _, d := range c.Domains {
			c.Verified[d] = time.Now()
		}
	}
	if c.DefaultRole == "" {
		c.DefaultRole = "viewer"
	}
	c.Enabled = true
	if err := f.conns.Put(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	return c
}

func (f fixture) oidcConn(t *testing.T, p *ssotest.OIDCIdP) Connection {
	sealed, err := f.svc.Keys().Seal(p.Secret)
	if err != nil {
		t.Fatal(err)
	}
	return f.put(t, Connection{Protocol: ProtocolOIDC, OIDCIssuer: p.Srv.URL, OIDCClientID: p.ClientID, OIDCSecret: sealed})
}

func (f fixture) samlConn(t *testing.T, p *ssotest.SAMLIdP) Connection {
	return f.put(t, Connection{Protocol: ProtocolSAML, SAMLSSOURL: "https://idp.example.org/sso", SAMLIssuer: p.Issuer, SAMLCertsPEM: []string{p.CertPEM}})
}
