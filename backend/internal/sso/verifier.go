package sso

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
)

// enforceCacheTTL bounds how long a Google sign-in may slip through after a
// connection is set to enforce single sign-on.
const enforceCacheTTL = time.Minute

// Verifier accepts SSO session tokens and passes every other token to the
// fallback (Google). An SSO session is accepted only while its connection is
// enabled, the email's domain is verified on it, and the user is still a
// member of the connection's workspace. A Google sign-in for a domain whose
// connection enforces single sign-on is refused.
type Verifier struct {
	s        *Service
	fallback teams.Verifier

	mu       sync.Mutex
	enforced map[string]enforceEntry
}

type enforceEntry struct {
	on      bool
	checked time.Time
}

// Verifier returns the composite verifier over fallback (nil: SSO only).
func (s *Service) Verifier(fallback teams.Verifier) *Verifier {
	return &Verifier{s: s, fallback: fallback, enforced: map[string]enforceEntry{}}
}

// Verify implements teams.Verifier.
func (v *Verifier) Verify(ctx context.Context, token string) (teams.User, error) {
	if strings.HasPrefix(token, TokenPrefix) {
		return v.verifySession(ctx, token)
	}
	if v.fallback == nil {
		return teams.User{}, teams.ErrUnauthenticated
	}
	u, err := v.fallback.Verify(ctx, token)
	if err != nil {
		return teams.User{}, err
	}
	on, err := v.enforcedFor(ctx, emailDomain(u.Email))
	if err != nil {
		return teams.User{}, err
	}
	if on {
		return teams.User{}, fmt.Errorf("%w: this organisation signs in with single sign-on", teams.ErrUnauthenticated)
	}
	return u, nil
}

func (v *Verifier) verifySession(ctx context.Context, token string) (teams.User, error) {
	sess, err := v.s.signer.Parse(token)
	if err != nil {
		return teams.User{}, fmt.Errorf("%w: %v", teams.ErrUnauthenticated, err)
	}
	c, err := v.s.conns.Get(ctx, sess.Connection)
	if errors.Is(err, ErrNotFound) {
		return teams.User{}, fmt.Errorf("%w: the connection was removed", teams.ErrUnauthenticated)
	}
	if err != nil {
		return teams.User{}, err
	}
	if c.Workspace != sess.Workspace || !c.Signs(sess.Email) {
		return teams.User{}, fmt.Errorf("%w: the connection no longer signs this user in", teams.ErrUnauthenticated)
	}
	ws, err := v.s.ws.Of(ctx, sess.Email)
	if errors.Is(err, teams.ErrNotFound) || err == nil && ws.ID != sess.Workspace {
		return teams.User{}, fmt.Errorf("%w: no longer a member of the workspace", teams.ErrUnauthenticated)
	}
	if err != nil {
		return teams.User{}, err
	}
	return teams.User{Email: sess.Email}, nil
}

func (v *Verifier) enforcedFor(ctx context.Context, domain string) (bool, error) {
	if domain == "" {
		return false, nil
	}
	now := v.s.now()
	v.mu.Lock()
	e, ok := v.enforced[domain]
	v.mu.Unlock()
	if ok && now.Sub(e.checked) < enforceCacheTTL {
		return e.on, nil
	}
	cs, err := v.s.conns.ByDomain(ctx, domain)
	if err != nil {
		return false, err
	}
	on := false
	for _, c := range cs {
		if c.Enforced && c.Enabled && !c.Verified[domain].IsZero() {
			on = true
		}
	}
	v.mu.Lock()
	if len(v.enforced) > 10000 {
		v.enforced = map[string]enforceEntry{}
	}
	v.enforced[domain] = enforceEntry{on: on, checked: now}
	v.mu.Unlock()
	return on, nil
}

var _ teams.Verifier = (*Verifier)(nil)
