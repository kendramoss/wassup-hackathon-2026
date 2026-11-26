package sso

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
)

// stateTTL is how long a started sign-in may take.
const stateTTL = 10 * time.Minute

var (
	// ErrNoConnection means no enabled connection signs in the email's domain.
	ErrNoConnection = errors.New("sso: no single sign-on for this email domain")
	// ErrLogin is a sign-in the identity provider or this server refused. The
	// wrapped detail is for logs, never for the client.
	ErrLogin = errors.New("sso: sign-in failed")
	// ErrOtherWorkspace means the user is a member of another workspace.
	ErrOtherWorkspace = errors.New("sso: this user is a member of another workspace")
)

// Config sets up a Service.
type Config struct {
	// SiteURL is the public site origin (https://...). The callback pages are
	// SiteURL/sso/oidc/callback and SiteURL/sso/saml/acs, and the SAML entity
	// ID is SiteURL/sso/saml/metadata.
	SiteURL    string
	SessionTTL time.Duration
	// HTTPClient is used for issuer discovery and the code exchange. Nil means
	// SafeHTTPClient (public addresses only). Tests pass their own.
	HTTPClient *http.Client
	Resolver   Resolver
	Now        func() time.Time
}

// Service runs single sign-on.
type Service struct {
	site      string
	keys      Keys
	signer    *Signer
	conns     Store
	state     OneTime
	ws        teams.Store
	prov      teams.Provisioner
	http      *http.Client
	resolver  Resolver
	now       func() time.Time
	providers providerCache
}

// New returns a Service. The workspace store must also be a
// teams.Provisioner (both built-in stores are).
func New(cfg Config, keys Keys, conns Store, state OneTime, ws teams.Store) (*Service, error) {
	u, err := url.Parse(cfg.SiteURL)
	if err != nil || (u.Scheme != "https" && !(u.Scheme == "http" && isLocalHost(u.Hostname()))) || u.Host == "" || u.Path != "" && u.Path != "/" {
		return nil, fmt.Errorf("sso: the site URL must be an https origin, got %q", cfg.SiteURL)
	}
	prov, ok := ws.(teams.Provisioner)
	if !ok {
		return nil, errors.New("sso: the workspace store cannot provision members")
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = SafeHTTPClient()
	}
	res := cfg.Resolver
	if res == nil {
		res = net.DefaultResolver
	}
	return &Service{
		site: strings.TrimSuffix(u.String(), "/"), keys: keys, signer: NewSigner(keys, cfg.SessionTTL, now),
		conns: conns, state: state, ws: ws, prov: prov, http: hc, resolver: res, now: now,
		providers: providerCache{m: map[string]cachedProvider{}},
	}, nil
}

func isLocalHost(h string) bool { return h == "localhost" || h == "127.0.0.1" || h == "::1" }

// RedirectURL is the OIDC redirect URI to register with the provider.
func (s *Service) RedirectURL() string { return s.site + "/sso/oidc/callback" }

// ACSURL is the SAML assertion consumer service URL.
func (s *Service) ACSURL() string { return s.site + "/sso/saml/acs" }

// EntityID is the SAML service provider entity ID (and audience).
func (s *Service) EntityID() string { return s.site + "/sso/saml/metadata" }

// Keys are the service's keys (for sealing secrets on write).
func (s *Service) Keys() Keys { return s.keys }

// Connections is the connection store.
func (s *Service) Connections() Store { return s.conns }

// signinState is kept server-side between Start and Finish.
type signinState struct {
	Key         string `json:"-"`
	Connection  string `json:"c"`
	Protocol    string `json:"p"`
	Nonce       string `json:"n,omitempty"`
	PKCE        string `json:"v,omitempty"`
	SAMLRequest string `json:"r,omitempty"`
}

func stateKey(k string) string { return "sso-state:" + k }

// StartResult says where to send the browser.
type StartResult struct {
	Protocol     string    `json:"protocol"`
	URL          string    `json:"url"`
	ConnectionID string    `json:"connection_id"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// Start begins a sign-in for email.
func (s *Service) Start(ctx context.Context, email string) (StartResult, error) {
	email, ok := teams.NormalEmail(email)
	if !ok {
		return StartResult{}, ErrNoConnection
	}
	c, err := s.connectionFor(ctx, email)
	if err != nil {
		return StartResult{}, err
	}
	st := signinState{Key: randHex(24), Connection: c.ID, Protocol: c.Protocol}
	var u string
	switch c.Protocol {
	case ProtocolOIDC:
		u, err = s.startOIDC(ctx, c, &st)
	case ProtocolSAML:
		u, err = s.startSAML(c, &st)
	default:
		err = fmt.Errorf("unknown protocol %q", c.Protocol)
	}
	if err != nil {
		return StartResult{}, fmt.Errorf("%w: start: %v", ErrLogin, err)
	}
	b, _ := json.Marshal(st)
	if err := s.state.Put(ctx, stateKey(st.Key), b, stateTTL); err != nil {
		return StartResult{}, err
	}
	return StartResult{Protocol: c.Protocol, URL: u, ConnectionID: c.ID, ExpiresAt: s.now().Add(stateTTL).UTC()}, nil
}

// connectionFor returns the one enabled, verified connection for email.
func (s *Service) connectionFor(ctx context.Context, email string) (Connection, error) {
	cs, err := s.conns.ByDomain(ctx, emailDomain(email))
	if err != nil {
		return Connection{}, err
	}
	for _, c := range cs {
		if c.Signs(email) {
			return c, nil
		}
	}
	return Connection{}, ErrNoConnection
}

func (s *Service) takeState(ctx context.Context, key, protocol string) (signinState, Connection, error) {
	if key == "" || len(key) > 128 {
		return signinState{}, Connection{}, fmt.Errorf("%w: %v", ErrLogin, ErrNoState)
	}
	b, err := s.state.Take(ctx, stateKey(key))
	if err != nil {
		return signinState{}, Connection{}, fmt.Errorf("%w: %v", ErrLogin, err)
	}
	var st signinState
	if err := json.Unmarshal(b, &st); err != nil || st.Protocol != protocol {
		return signinState{}, Connection{}, fmt.Errorf("%w: state is for another protocol", ErrLogin)
	}
	st.Key = key
	c, err := s.conns.Get(ctx, st.Connection)
	if err != nil {
		return signinState{}, Connection{}, fmt.Errorf("%w: connection: %v", ErrLogin, err)
	}
	if c.Protocol != protocol || !c.Enabled {
		return signinState{}, Connection{}, fmt.Errorf("%w: connection is disabled", ErrLogin)
	}
	return st, c, nil
}

// Login is a finished sign-in.
type Login struct {
	Token       string     `json:"token"`
	ExpiresAt   time.Time  `json:"expires_at"`
	Email       string     `json:"email"`
	WorkspaceID string     `json:"workspace_id"`
	Role        teams.Role `json:"role"`
	// Provisioned is true when this sign-in made the user a member.
	Provisioned bool `json:"provisioned"`
}

// FinishOIDC completes an OIDC sign-in with the state and code the provider
// sent to the callback page.
func (s *Service) FinishOIDC(ctx context.Context, state, code string) (Login, error) {
	st, c, err := s.takeState(ctx, state, ProtocolOIDC)
	if err != nil {
		return Login{}, err
	}
	email, err := s.finishOIDC(ctx, c, st, code)
	if err != nil {
		return Login{}, fmt.Errorf("%w: %v", ErrLogin, err)
	}
	return s.complete(ctx, c, email)
}

// FinishSAML completes a SAML sign-in with the SAMLResponse and RelayState
// posted to the ACS page.
func (s *Service) FinishSAML(ctx context.Context, samlResponse, relayState string) (Login, error) {
	st, c, err := s.takeState(ctx, relayState, ProtocolSAML)
	if err != nil {
		return Login{}, err
	}
	l, err := s.finishSAML(c, st, samlResponse)
	if err != nil {
		return Login{}, fmt.Errorf("%w: %v", ErrLogin, err)
	}
	if err := s.markAssertion(ctx, c.ID, l); err != nil {
		return Login{}, fmt.Errorf("%w: %v", ErrLogin, err)
	}
	return s.complete(ctx, c, l.email)
}

// complete checks the email against the connection, provisions the user into
// the connection's workspace if needed, and mints a session.
func (s *Service) complete(ctx context.Context, c Connection, email string) (Login, error) {
	if !c.Signs(email) {
		return Login{}, fmt.Errorf("%w: the identity provider returned an email outside the connection's verified domains", ErrLogin)
	}
	provisioned := false
	ws, err := s.ws.Of(ctx, email)
	switch {
	case err == nil && ws.ID != c.Workspace:
		return Login{}, ErrOtherWorkspace
	case errors.Is(err, teams.ErrNotFound):
		ws, err = s.prov.Provision(ctx, c.Workspace, email, teams.Role(c.DefaultRole))
		if errors.Is(err, teams.ErrMember) {
			return Login{}, ErrOtherWorkspace
		}
		if err != nil {
			return Login{}, err
		}
		provisioned = true
	case err != nil:
		return Login{}, err
	}
	role, _ := ws.RoleOf(email)
	tok, sess, err := s.signer.Mint(email, c.Workspace, c.ID)
	if err != nil {
		return Login{}, err
	}
	return Login{Token: tok, ExpiresAt: sess.Expires, Email: email, WorkspaceID: c.Workspace, Role: role, Provisioned: provisioned}, nil
}

// DomainCheck is one domain's DNS verification result.
type DomainCheck struct {
	Domain   string `json:"domain"`
	Record   string `json:"record"`
	Verified bool   `json:"verified"`
	Problem  string `json:"problem,omitempty"`
}

// VerifyDomains looks up each unverified domain's TXT record and records the
// ones that carry the connection's token. A domain another connection has
// verified is not taken over.
func (s *Service) VerifyDomains(ctx context.Context, c Connection) (Connection, []DomainCheck, error) {
	out := make([]DomainCheck, 0, len(c.Domains))
	if c.Verified == nil {
		c.Verified = map[string]time.Time{}
	}
	for _, d := range c.Domains {
		chk := DomainCheck{Domain: d, Record: VerifyRecordName(d)}
		if _, ok := c.Verified[d]; ok {
			chk.Verified = true
			out = append(out, chk)
			continue
		}
		recs, err := s.resolver.LookupTXT(ctx, chk.Record)
		switch {
		case err != nil:
			chk.Problem = "no TXT record found"
		case !slices.Contains(trimAll(recs), c.VerifyToken):
			chk.Problem = "the TXT record does not hold this connection's token"
		default:
			others, err := s.conns.ByDomain(ctx, d)
			if err != nil {
				return Connection{}, nil, err
			}
			taken := false
			for _, o := range others {
				if _, ok := o.Verified[d]; ok && o.ID != c.ID {
					taken = true
				}
			}
			if taken {
				chk.Problem = "another connection has verified this domain"
			} else {
				c.Verified[d] = s.now().UTC()
				chk.Verified = true
			}
		}
		out = append(out, chk)
	}
	return c, out, nil
}

func trimAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = strings.TrimSpace(s)
	}
	return out
}
