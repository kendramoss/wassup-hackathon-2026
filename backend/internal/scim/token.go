package scim

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"
)

// TokenPrefix starts every SCIM token, so a leaked one is recognisable.
const TokenPrefix = "scim_"

// ErrNotFound is a missing token.
var ErrNotFound = errors.New("scim: no token")

// Token is a workspace's SCIM credential as stored: the hash, never the
// secret.
type Token struct {
	Workspace   string    `firestore:"workspace" json:"workspace"`
	Hash        string    `firestore:"hash" json:"-"`
	Hint        string    `firestore:"hint" json:"hint"` // the last four characters, for the admin screen
	Domains     []string  `firestore:"domains" json:"domains"`
	DefaultRole string    `firestore:"default_role" json:"default_role"`
	AllowAdmin  bool      `firestore:"allow_admin" json:"allow_admin"`
	CreatedAt   time.Time `firestore:"created_at" json:"created_at"`
}

// Manages reports whether the token's domains cover email.
func (t Token) Manages(email string) bool {
	_, d, ok := strings.Cut(email, "@")
	if !ok {
		return false
	}
	for _, x := range t.Domains {
		if x == d {
			return true
		}
	}
	return false
}

// NewSecret returns a fresh token secret and its hash.
func NewSecret() (secret, hash string) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	secret = TokenPrefix + base64.RawURLEncoding.EncodeToString(b)
	return secret, Hash(secret)
}

// Hash is the stored form of a secret.
func Hash(secret string) string {
	h := sha256.Sum256([]byte("scim-token:" + secret))
	return hex.EncodeToString(h[:])
}

// WellFormed reports whether s could be a token at all; a malformed value is
// refused without a store lookup.
func WellFormed(s string) bool {
	if !strings.HasPrefix(s, TokenPrefix) || len(s) != len(TokenPrefix)+43 {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(s[len(TokenPrefix):])
	return err == nil
}

// Store keeps one token per workspace.
type Store interface {
	// Get returns the workspace's token; ErrNotFound.
	Get(ctx context.Context, workspace string) (Token, error)
	// ByHash returns the token with this hash; ErrNotFound.
	ByHash(ctx context.Context, hash string) (Token, error)
	// Put stores t, replacing the workspace's previous token.
	Put(ctx context.Context, t Token) error
	// Delete removes the workspace's token; no error if there is none.
	Delete(ctx context.Context, workspace string) error
}

// Memory is an in-process Store.
type Memory struct {
	mu   sync.Mutex
	byWS map[string]Token
}

// NewMemory returns an empty Memory.
func NewMemory() *Memory { return &Memory{byWS: map[string]Token{}} }

func (m *Memory) Get(_ context.Context, ws string) (Token, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.byWS[ws]
	if !ok {
		return Token{}, ErrNotFound
	}
	return t, nil
}

func (m *Memory) ByHash(_ context.Context, hash string) (Token, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.byWS {
		if t.Hash == hash {
			return t, nil
		}
	}
	return Token{}, ErrNotFound
}

func (m *Memory) Put(_ context.Context, t Token) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t.Domains = append([]string(nil), t.Domains...)
	m.byWS[t.Workspace] = t
	return nil
}

func (m *Memory) Delete(_ context.Context, ws string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.byWS, ws)
	return nil
}

var _ Store = (*Memory)(nil)
