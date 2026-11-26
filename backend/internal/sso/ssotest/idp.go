// Package ssotest is a test OpenID Connect provider and a SAML response
// builder, for tests of single sign-on (#868). Not for production use.
package ssotest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/beevik/etree"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	dsig "github.com/russellhaering/goxmldsig"
)

// --- a test OIDC identity provider -----------------------------------------

// OIDCIdP is an OpenID Connect provider on an httptest TLS server: discovery,
// JWKS and a token endpoint that checks the client secret and PKCE. The
// exported fields change the next ID token it issues.
type OIDCIdP struct {
	Srv      *httptest.Server
	key      *rsa.PrivateKey
	ClientID string
	Secret   string

	mu    sync.Mutex
	codes map[string]issuedCode
	// overrides applied to the next id_token
	Aud           string
	Nonce         *string
	Expired       bool
	Email         string
	EmailVerified *bool
}

type issuedCode struct {
	challenge, nonce, email string
}

// NewOIDCIdP starts a provider for client ID "client-123", secret "s3cret".
func NewOIDCIdP(t *testing.T) *OIDCIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &OIDCIdP{key: key, ClientID: "client-123", Secret: "s3cret", codes: map[string]issuedCode{}}
	mux := http.NewServeMux()
	p.Srv = httptest.NewTLSServer(mux)
	t.Cleanup(p.Srv.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": p.Srv.URL, "authorization_endpoint": p.Srv.URL + "/authorize", "token_endpoint": p.Srv.URL + "/token",
			"jwks_uri": p.Srv.URL + "/jwks", "id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		id, sec, ok := r.BasicAuth()
		if !ok {
			id, sec = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
		}
		if id != p.ClientID || sec != p.Secret {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
			return
		}
		p.mu.Lock()
		c, ok := p.codes[r.PostForm.Get("code")]
		delete(p.codes, r.PostForm.Get("code"))
		p.mu.Unlock()
		sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
		if !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != c.challenge {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "id_token": p.idToken(t, c)})
	})
	return p
}

// Authorize simulates the user signing in at the provider: it returns the
// code the provider would put on the redirect.
func (p *OIDCIdP) Authorize(t *testing.T, authURL, email string) (state, code string) {
	t.Helper()
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" || q.Get("nonce") == "" {
		t.Fatalf("authorize URL lacks PKCE or nonce: %s", authURL)
	}
	code = "code-" + randHex(8)
	p.mu.Lock()
	p.codes[code] = issuedCode{challenge: q.Get("code_challenge"), nonce: q.Get("nonce"), email: email}
	p.mu.Unlock()
	return q.Get("state"), code
}

func (p *OIDCIdP) idToken(t *testing.T, c issuedCode) string {
	sig, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: p.key, KeyID: "k1"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	aud, nonce := p.ClientID, c.nonce
	if p.Aud != "" {
		aud = p.Aud
	}
	if p.Nonce != nil {
		nonce = *p.Nonce
	}
	exp := now.Add(time.Hour)
	if p.Expired {
		exp = now.Add(-time.Hour)
	}
	email := c.email
	if p.Email != "" {
		email = p.Email
	}
	claims := map[string]any{"iss": p.Srv.URL, "sub": "user-1", "aud": aud, "exp": exp.Unix(), "iat": now.Add(-2 * time.Hour).Unix(), "nonce": nonce, "email": email}
	if p.EmailVerified != nil {
		claims["email_verified"] = *p.EmailVerified
	}
	raw, err := jwt.Signed(sig).Claims(claims).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// --- a test SAML identity provider -----------------------------------------

// SAMLIdP signs SAML responses with its own key and self-signed certificate.
type SAMLIdP struct {
	Issuer  string
	cert    tls.Certificate
	CertPEM string
}

// NewSAMLIdP returns an identity provider with a fresh key.
func NewSAMLIdP(t *testing.T) *SAMLIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test idp"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return &SAMLIdP{Issuer: "https://idp.example.org/saml", cert: tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key},
		CertPEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))}
}

// SAMLOpts describes the response to build.
type SAMLOpts struct {
	InResponseTo, Email, Audience, Recipient, Destination, AssertionID string
	NotOnOrAfter                                                       time.Time
	Unsigned                                                           bool
	Signer                                                             *SAMLIdP // signs with another key
	TamperEmail                                                        string   // changed after signing
}

// Response builds a base64 SAMLResponse with a signed assertion.
func (p *SAMLIdP) Response(t *testing.T, o SAMLOpts) string {
	t.Helper()
	now := time.Now().UTC()
	if o.NotOnOrAfter.IsZero() {
		o.NotOnOrAfter = now.Add(5 * time.Minute)
	}
	if o.AssertionID == "" {
		o.AssertionID = "_a" + randHex(8)
	}
	ts := func(t time.Time) string { return t.UTC().Format(time.RFC3339) }
	doc := etree.NewDocument()
	resp := doc.CreateElement("samlp:Response")
	resp.CreateAttr("xmlns:samlp", "urn:oasis:names:tc:SAML:2.0:protocol")
	resp.CreateAttr("xmlns:saml", "urn:oasis:names:tc:SAML:2.0:assertion")
	resp.CreateAttr("ID", "_r"+randHex(8))
	resp.CreateAttr("Version", "2.0")
	resp.CreateAttr("IssueInstant", ts(now))
	resp.CreateAttr("Destination", o.Destination)
	resp.CreateAttr("InResponseTo", o.InResponseTo)
	resp.CreateElement("saml:Issuer").SetText(p.Issuer)
	resp.CreateElement("samlp:Status").CreateElement("samlp:StatusCode").CreateAttr("Value", "urn:oasis:names:tc:SAML:2.0:status:Success")

	a := etree.NewElement("saml:Assertion")
	a.CreateAttr("xmlns:saml", "urn:oasis:names:tc:SAML:2.0:assertion")
	a.CreateAttr("ID", o.AssertionID)
	a.CreateAttr("Version", "2.0")
	a.CreateAttr("IssueInstant", ts(now))
	a.CreateElement("saml:Issuer").SetText(p.Issuer)
	subj := a.CreateElement("saml:Subject")
	nid := subj.CreateElement("saml:NameID")
	nid.CreateAttr("Format", "urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress")
	nid.SetText(o.Email)
	sc := subj.CreateElement("saml:SubjectConfirmation")
	sc.CreateAttr("Method", "urn:oasis:names:tc:SAML:2.0:cm:bearer")
	scd := sc.CreateElement("saml:SubjectConfirmationData")
	scd.CreateAttr("InResponseTo", o.InResponseTo)
	scd.CreateAttr("NotOnOrAfter", ts(o.NotOnOrAfter))
	scd.CreateAttr("Recipient", o.Recipient)
	cond := a.CreateElement("saml:Conditions")
	cond.CreateAttr("NotBefore", ts(now.Add(-time.Minute)))
	cond.CreateAttr("NotOnOrAfter", ts(o.NotOnOrAfter))
	cond.CreateElement("saml:AudienceRestriction").CreateElement("saml:Audience").SetText(o.Audience)
	an := a.CreateElement("saml:AuthnStatement")
	an.CreateAttr("AuthnInstant", ts(now))

	if !o.Unsigned {
		signer := p
		if o.Signer != nil {
			signer = o.Signer
		}
		ctx := dsig.NewDefaultSigningContext(dsig.TLSCertKeyStore(signer.cert))
		ctx.Canonicalizer = dsig.MakeC14N10ExclusiveCanonicalizerWithPrefixList("")
		signed, err := ctx.SignEnveloped(a)
		if err != nil {
			t.Fatal(err)
		}
		a = signed
	}
	if o.TamperEmail != "" {
		a.FindElement(".//NameID").SetText(o.TamperEmail)
	}
	resp.AddChild(a)
	b, err := doc.WriteToBytes()
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// Mutate changes a pending authorization code's PKCE challenge (to test that
// the code exchange checks it).
func (p *OIDCIdP) Mutate(code, challenge string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c := p.codes[code]
	c.challenge = challenge
	p.codes[code] = c
}
