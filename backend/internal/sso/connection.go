package sso

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
)

// Protocols.
const (
	ProtocolOIDC = "oidc"
	ProtocolSAML = "saml"
)

// MaxDomains caps the email domains on one connection.
const MaxDomains = 20

// MaxConnections caps the connections of one workspace.
const MaxConnections = 5

var (
	// ErrNotFound is a connection that does not exist (or is not this workspace's).
	ErrNotFound = errors.New("sso: connection not found")
	// ErrDomainTaken means another connection has already verified a domain.
	ErrDomainTaken = errors.New("sso: another connection has verified that domain")
	// ErrInvalid is a connection that fails validation.
	ErrInvalid = errors.New("sso: invalid connection")
	// ErrUnverified is enabling a connection with an unverified domain.
	ErrUnverified = errors.New("sso: every domain must be verified first")
)

// Connection is one workspace's identity provider.
type Connection struct {
	ID        string `json:"id" firestore:"id"`
	Workspace string `json:"workspace_id" firestore:"workspace_id"`
	Protocol  string `json:"protocol" firestore:"protocol"`
	// Domains are the lower-case email domains the connection signs in.
	Domains []string `json:"domains" firestore:"domains"`
	// Verified maps a domain to the time its DNS record was checked.
	Verified map[string]time.Time `json:"verified,omitempty" firestore:"verified,omitempty"`
	// VerifyToken is the value of the _undelabs-verify TXT record.
	VerifyToken string `json:"verify_token" firestore:"verify_token"`
	// DefaultRole is the role a user signed in for the first time gets.
	DefaultRole string `json:"default_role" firestore:"default_role"`
	// Enabled connections sign users in; only with every domain verified.
	Enabled bool `json:"enabled" firestore:"enabled"`
	// Enforced refuses Google sign-in for the connection's domains.
	Enforced bool `json:"enforced" firestore:"enforced"`

	OIDCIssuer   string `json:"oidc_issuer,omitempty" firestore:"oidc_issuer,omitempty"`
	OIDCClientID string `json:"oidc_client_id,omitempty" firestore:"oidc_client_id,omitempty"`
	// OIDCSecret is the sealed client secret; never sent to a client.
	OIDCSecret string `json:"-" firestore:"oidc_secret,omitempty"`

	SAMLSSOURL   string   `json:"saml_sso_url,omitempty" firestore:"saml_sso_url,omitempty"`
	SAMLIssuer   string   `json:"saml_issuer,omitempty" firestore:"saml_issuer,omitempty"`
	SAMLCertsPEM []string `json:"saml_certs_pem,omitempty" firestore:"saml_certs_pem,omitempty"`

	CreatedBy string    `json:"created_by" firestore:"created_by"`
	CreatedAt time.Time `json:"created_at" firestore:"created_at"`
	UpdatedAt time.Time `json:"updated_at" firestore:"updated_at"`
}

// HasSecret reports whether a client secret is stored.
func (c Connection) HasSecret() bool { return c.OIDCSecret != "" }

// AllVerified reports whether every domain has been verified.
func (c Connection) AllVerified() bool {
	if len(c.Domains) == 0 {
		return false
	}
	for _, d := range c.Domains {
		if _, ok := c.Verified[d]; !ok {
			return false
		}
	}
	return true
}

// Signs reports whether the connection may sign in email now.
func (c Connection) Signs(email string) bool {
	d := emailDomain(email)
	return c.Enabled && d != "" && slices.Contains(c.Domains, d) && !c.Verified[d].IsZero()
}

// NewConnectionID returns a fresh connection ID.
func NewConnectionID() string { return "sso_" + randHex(8) }

// NewVerifyToken returns a fresh domain verification token.
func NewVerifyToken() string { return "undelabs-verify-" + randHex(16) }

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

var domainPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*\.[a-z]{2,63}$`)

// publicMailDomains cannot be claimed: their users are not one organisation.
var publicMailDomains = map[string]bool{
	"gmail.com": true, "googlemail.com": true, "outlook.com": true, "hotmail.com": true, "live.com": true, "msn.com": true,
	"yahoo.com": true, "ymail.com": true, "icloud.com": true, "me.com": true, "mac.com": true, "aol.com": true,
	"proton.me": true, "protonmail.com": true, "gmx.com": true, "gmx.de": true, "mail.com": true, "zoho.com": true,
	"yandex.com": true, "yandex.ru": true, "qq.com": true, "163.com": true, "126.com": true, "naver.com": true,
	"hey.com": true, "fastmail.com": true, "tutanota.com": true, "web.de": true, "comcast.net": true,
}

// NormalDomain lower-cases and checks an email domain.
func NormalDomain(s string) (string, error) {
	d := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), ".")
	if !domainPattern.MatchString(d) || len(d) > 253 {
		return "", fmt.Errorf("%w: %q is not a domain", ErrInvalid, s)
	}
	if publicMailDomains[d] {
		return "", fmt.Errorf("%w: %s is a public mail domain and cannot be claimed", ErrInvalid, d)
	}
	return d, nil
}

func emailDomain(email string) string {
	i := strings.LastIndexByte(email, '@')
	if i < 0 {
		return ""
	}
	return strings.ToLower(email[i+1:])
}

// Normalize checks a connection and puts its fields in canonical form. It does
// not check the issuer is reachable.
func (c *Connection) Normalize() error {
	switch c.Protocol {
	case ProtocolOIDC, ProtocolSAML:
	default:
		return fmt.Errorf("%w: protocol must be oidc or saml", ErrInvalid)
	}
	if len(c.Domains) == 0 || len(c.Domains) > MaxDomains {
		return fmt.Errorf("%w: give 1 to %d email domains", ErrInvalid, MaxDomains)
	}
	seen := map[string]bool{}
	ds := make([]string, 0, len(c.Domains))
	for _, d := range c.Domains {
		n, err := NormalDomain(d)
		if err != nil {
			return err
		}
		if !seen[n] {
			seen[n] = true
			ds = append(ds, n)
		}
	}
	sort.Strings(ds)
	c.Domains = ds
	for d := range c.Verified { // a removed domain loses its verification
		if !seen[d] {
			delete(c.Verified, d)
		}
	}
	if c.DefaultRole == "" {
		c.DefaultRole = string(teams.RoleViewer)
	}
	if _, ok := teams.ParseRole(c.DefaultRole); !ok {
		return fmt.Errorf("%w: default_role must be viewer, editor or admin", ErrInvalid)
	}
	switch c.Protocol {
	case ProtocolOIDC:
		u, err := url.Parse(c.OIDCIssuer)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
			return fmt.Errorf("%w: oidc_issuer must be an https URL", ErrInvalid)
		}
		if c.OIDCClientID == "" || len(c.OIDCClientID) > 512 {
			return fmt.Errorf("%w: oidc_client_id is required", ErrInvalid)
		}
		c.SAMLSSOURL, c.SAMLIssuer, c.SAMLCertsPEM = "", "", nil
	case ProtocolSAML:
		u, err := url.Parse(c.SAMLSSOURL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return fmt.Errorf("%w: saml_sso_url must be an https URL", ErrInvalid)
		}
		if c.SAMLIssuer == "" || len(c.SAMLIssuer) > 1024 {
			return fmt.Errorf("%w: saml_issuer (the identity provider's entity ID) is required", ErrInvalid)
		}
		if len(c.SAMLCertsPEM) == 0 || len(c.SAMLCertsPEM) > 3 {
			return fmt.Errorf("%w: give 1 to 3 signing certificates", ErrInvalid)
		}
		if _, err := ParseCerts(c.SAMLCertsPEM); err != nil {
			return err
		}
		c.OIDCIssuer, c.OIDCClientID, c.OIDCSecret = "", "", ""
	}
	if c.Enabled && !c.AllVerified() {
		return ErrUnverified
	}
	return nil
}

// ParseCerts parses PEM (or bare base64 DER) certificates.
func ParseCerts(pems []string) ([]*x509.Certificate, error) {
	out := make([]*x509.Certificate, 0, len(pems))
	for _, p := range pems {
		p = strings.TrimSpace(p)
		var der []byte
		if b, _ := pem.Decode([]byte(p)); b != nil {
			der = b.Bytes
		} else {
			wrapped := "-----BEGIN CERTIFICATE-----\n" + p + "\n-----END CERTIFICATE-----"
			b, _ := pem.Decode([]byte(wrapped))
			if b == nil {
				return nil, fmt.Errorf("%w: a signing certificate is not PEM", ErrInvalid)
			}
			der = b.Bytes
		}
		c, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, fmt.Errorf("%w: a signing certificate does not parse", ErrInvalid)
		}
		out = append(out, c)
	}
	return out, nil
}

// Store keeps connections.
type Store interface {
	Get(ctx context.Context, id string) (Connection, error)
	List(ctx context.Context, workspace string) ([]Connection, error)
	// ByDomain returns the connections holding domain (normally one).
	ByDomain(ctx context.Context, domain string) ([]Connection, error)
	// Put creates or replaces a connection. ErrDomainTaken if another
	// connection has verified one of the domains this one has verified.
	// An unverified claim blocks nobody: anyone can type a domain, only its
	// owner can verify it.
	Put(ctx context.Context, c Connection) error
	Delete(ctx context.Context, id string) error
}

// MemoryStore is an in-process Store.
type MemoryStore struct {
	mu sync.Mutex
	m  map[string]Connection
}

// NewMemoryStore returns an empty store.
func NewMemoryStore() *MemoryStore { return &MemoryStore{m: map[string]Connection{}} }

// Get implements Store.
func (s *MemoryStore) Get(_ context.Context, id string) (Connection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.m[id]
	if !ok {
		return Connection{}, ErrNotFound
	}
	return clone(c), nil
}

// List implements Store.
func (s *MemoryStore) List(_ context.Context, workspace string) ([]Connection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Connection
	for _, c := range s.m {
		if c.Workspace == workspace {
			out = append(out, clone(c))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// ByDomain implements Store.
func (s *MemoryStore) ByDomain(_ context.Context, domain string) ([]Connection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Connection
	for _, c := range s.m {
		if slices.Contains(c.Domains, domain) {
			out = append(out, clone(c))
		}
	}
	return out, nil
}

// Put implements Store.
func (s *MemoryStore) Put(_ context.Context, c Connection) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, o := range s.m {
		if o.ID != c.ID && VerifiedClash(o, c) {
			return ErrDomainTaken
		}
	}
	s.m[c.ID] = clone(c)
	return nil
}

// Delete implements Store.
func (s *MemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[id]; !ok {
		return ErrNotFound
	}
	delete(s.m, id)
	return nil
}

func clone(c Connection) Connection {
	c.Domains = slices.Clone(c.Domains)
	c.SAMLCertsPEM = slices.Clone(c.SAMLCertsPEM)
	if c.Verified != nil {
		v := make(map[string]time.Time, len(c.Verified))
		for k, t := range c.Verified {
			v[k] = t
		}
		c.Verified = v
	}
	return c
}

// VerifiedClash reports whether a and b have both verified some domain.
func VerifiedClash(a, b Connection) bool {
	for d := range b.Verified {
		if _, ok := a.Verified[d]; ok {
			return true
		}
	}
	return false
}
