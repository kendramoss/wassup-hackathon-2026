package sso

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// TokenPrefix marks an SSO session token, so the verifier can tell it from a
// Google ID token without trying both.
const TokenPrefix = "ops1."

// MaxSessionTTL caps a session's lifetime whatever the configuration says.
const MaxSessionTTL = 24 * time.Hour

// clockSkew is the tolerance for an issue time slightly in the future.
const clockSkew = time.Minute

// ErrInvalidSession is any session token that does not verify.
var ErrInvalidSession = errors.New("sso: invalid session token")

// Session is what a session token asserts.
type Session struct {
	Email      string    `json:"e"`
	Workspace  string    `json:"w"`
	Connection string    `json:"c"`
	IssuedAt   time.Time `json:"-"`
	Expires    time.Time `json:"-"`
	ID         string    `json:"j"`
}

type sessionWire struct {
	Session
	IAT int64 `json:"iat"`
	EXP int64 `json:"exp"`
}

// Signer mints and parses session tokens.
type Signer struct {
	key []byte
	ttl time.Duration
	now func() time.Time
}

// NewSigner returns a signer with the derived signing key and a lifetime
// (capped at MaxSessionTTL).
func NewSigner(k Keys, ttl time.Duration, now func() time.Time) *Signer {
	if ttl <= 0 || ttl > MaxSessionTTL {
		ttl = 8 * time.Hour
	}
	if now == nil {
		now = time.Now
	}
	return &Signer{key: k.sign, ttl: ttl, now: now}
}

// Mint returns a token for the session.
func (s *Signer) Mint(email, workspace, connection string) (string, Session, error) {
	var jti [12]byte
	if _, err := rand.Read(jti[:]); err != nil {
		return "", Session{}, err
	}
	now := s.now().UTC().Truncate(time.Second)
	sess := Session{Email: email, Workspace: workspace, Connection: connection, IssuedAt: now, Expires: now.Add(s.ttl), ID: hex.EncodeToString(jti[:])}
	body, err := json.Marshal(sessionWire{Session: sess, IAT: now.Unix(), EXP: sess.Expires.Unix()})
	if err != nil {
		return "", Session{}, err
	}
	signed := TokenPrefix + base64.RawURLEncoding.EncodeToString(body)
	return signed + "." + base64.RawURLEncoding.EncodeToString(s.mac(signed)), sess, nil
}

func (s *Signer) mac(signed string) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(signed))
	return m.Sum(nil)
}

// Parse verifies a token's signature and time window.
func (s *Signer) Parse(tok string) (Session, error) {
	if !strings.HasPrefix(tok, TokenPrefix) || len(tok) > 2048 {
		return Session{}, ErrInvalidSession
	}
	i := strings.LastIndexByte(tok, '.')
	if i <= len(TokenPrefix) {
		return Session{}, ErrInvalidSession
	}
	signed, sig := tok[:i], tok[i+1:]
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(got, s.mac(signed)) {
		return Session{}, ErrInvalidSession
	}
	body, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(signed, TokenPrefix))
	if err != nil {
		return Session{}, ErrInvalidSession
	}
	var w sessionWire
	if err := json.Unmarshal(body, &w); err != nil || w.Email == "" || w.Workspace == "" || w.Connection == "" {
		return Session{}, ErrInvalidSession
	}
	iat, exp := time.Unix(w.IAT, 0).UTC(), time.Unix(w.EXP, 0).UTC()
	now := s.now()
	if now.Add(clockSkew).Before(iat) || !now.Before(exp) || exp.Sub(iat) > MaxSessionTTL {
		return Session{}, ErrInvalidSession
	}
	sess := w.Session
	sess.IssuedAt, sess.Expires = iat, exp
	return sess, nil
}
