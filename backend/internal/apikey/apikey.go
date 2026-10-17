// Package apikey issues and checks API keys for brokers who call the API
// directly (D69). A key is "op_" plus 32 random bytes (base64url). It is
// shown once when issued; the store keeps only its SHA-256, with a name, a
// daily request quota and a disabled flag. The key's ID is the first 16 hex
// characters of that hash, so checking a key is one read by ID followed by a
// constant-time comparison of the full hash, with no query or index.
package apikey

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Prefix starts every key, so a leaked one is easy to recognise in logs and
// secret scanners.
const Prefix = "op_"

// MaxName and MaxQuota bound what the admin can set.
const (
	MaxName  = 100
	MaxQuota = 1_000_000
)

var (
	// ErrInvalid means the presented key is malformed, unknown or disabled.
	ErrInvalid = errors.New("apikey: invalid or disabled key")
	// ErrNotFound means no key has that ID.
	ErrNotFound = errors.New("apikey: no such key")
	// ErrBadRequest means an issue request has a bad name or quota.
	ErrBadRequest = errors.New("apikey: bad request")
)

// keyPattern is Prefix plus 32 bytes in unpadded base64url (43 characters).
var keyPattern = regexp.MustCompile(`^op_[A-Za-z0-9_-]{43}$`)

// idPattern is a key ID: 16 lowercase hex characters.
var idPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// Key is a stored key. Hash is never returned to a client.
type Key struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Hash       string    `json:"-"`
	CreatedAt  time.Time `json:"created_at"`
	DailyQuota int       `json:"daily_quota"`
	Disabled   bool      `json:"disabled"`
}

// Store keeps the keys and their per-day request counts.
type Store interface {
	Create(ctx context.Context, k Key) error
	// Get returns ErrNotFound when there is no key with id.
	Get(ctx context.Context, id string) (Key, error)
	List(ctx context.Context) ([]Key, error)
	// Disable returns ErrNotFound when there is no key with id.
	Disable(ctx context.Context, id string) error
	// Use adds one request to the key's count for day (YYYY-MM-DD, UTC)
	// and returns the new count.
	Use(ctx context.Context, id, day string) (int64, error)
}

// Keys issues, checks and meters keys.
type Keys struct {
	store Store
	now   func() time.Time
}

// New returns a key service on store.
func New(store Store) *Keys { return &Keys{store: store, now: time.Now} }

// hashOf is the stored SHA-256 of a key, in hex.
func hashOf(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}

// ValidID reports whether id has the shape of a key ID.
func ValidID(id string) bool { return idPattern.MatchString(id) }

// Issue creates a key and returns its secret, which is not stored and cannot
// be shown again.
func (k *Keys) Issue(ctx context.Context, name string, dailyQuota int) (string, Key, error) {
	name = strings.Join(strings.Fields(name), " ")
	switch {
	case name == "":
		return "", Key{}, fmt.Errorf("%w: name is required", ErrBadRequest)
	case utf8.RuneCountInString(name) > MaxName:
		return "", Key{}, fmt.Errorf("%w: name is longer than %d characters", ErrBadRequest, MaxName)
	case dailyQuota < 1 || dailyQuota > MaxQuota:
		return "", Key{}, fmt.Errorf("%w: daily_quota must be between 1 and %d", ErrBadRequest, MaxQuota)
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", Key{}, fmt.Errorf("apikey: random: %w", err)
	}
	secret := Prefix + base64.RawURLEncoding.EncodeToString(b)
	h := hashOf(secret)
	key := Key{ID: h[:16], Name: name, Hash: h, CreatedAt: k.now().UTC(), DailyQuota: dailyQuota}
	if err := k.store.Create(ctx, key); err != nil {
		return "", Key{}, err
	}
	return secret, key, nil
}

// Authenticate returns the enabled key whose secret this is, or ErrInvalid.
// Other errors mean the store could not be read.
func (k *Keys) Authenticate(ctx context.Context, secret string) (Key, error) {
	if !keyPattern.MatchString(secret) {
		return Key{}, ErrInvalid
	}
	h := hashOf(secret)
	key, err := k.store.Get(ctx, h[:16])
	if errors.Is(err, ErrNotFound) {
		return Key{}, ErrInvalid
	}
	if err != nil {
		return Key{}, err
	}
	if subtle.ConstantTimeCompare([]byte(h), []byte(key.Hash)) != 1 || key.Disabled {
		return Key{}, ErrInvalid
	}
	return key, nil
}

// List returns every key, newest first.
func (k *Keys) List(ctx context.Context) ([]Key, error) {
	ks, err := k.store.List(ctx)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(ks, func(i, j int) bool { return ks[i].CreatedAt.After(ks[j].CreatedAt) })
	return ks, nil
}

// Disable turns a key off for good; requests with it get 401.
func (k *Keys) Disable(ctx context.Context, id string) error {
	if !ValidID(id) {
		return ErrNotFound
	}
	return k.store.Disable(ctx, id)
}

// Use counts one request against key's quota for the current UTC day. It
// reports whether the request is within the quota, the day's count, and the
// time until the next UTC day (when the count starts again).
func (k *Keys) Use(ctx context.Context, key Key) (bool, int64, time.Duration, error) {
	now := k.now().UTC()
	reset := now.Truncate(24 * time.Hour).Add(24 * time.Hour).Sub(now)
	n, err := k.store.Use(context.WithoutCancel(ctx), key.ID, now.Format(time.DateOnly))
	if err != nil {
		return true, 0, reset, err
	}
	return n <= int64(key.DailyQuota), n, reset, nil
}

// Memory is an in-process Store (local runs and tests).
type Memory struct {
	mu    sync.Mutex
	keys  map[string]Key
	usage map[string]int64 // id + "/" + day
}

// NewMemory returns an empty store.
func NewMemory() *Memory { return &Memory{keys: map[string]Key{}, usage: map[string]int64{}} }

// Create implements Store.
func (m *Memory) Create(_ context.Context, k Key) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.keys[k.ID]; ok {
		return fmt.Errorf("apikey: key %s already exists", k.ID)
	}
	m.keys[k.ID] = k
	return nil
}

// Get implements Store.
func (m *Memory) Get(_ context.Context, id string) (Key, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k, ok := m.keys[id]
	if !ok {
		return Key{}, ErrNotFound
	}
	return k, nil
}

// List implements Store.
func (m *Memory) List(_ context.Context) ([]Key, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Key, 0, len(m.keys))
	for _, k := range m.keys {
		out = append(out, k)
	}
	return out, nil
}

// Disable implements Store.
func (m *Memory) Disable(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k, ok := m.keys[id]
	if !ok {
		return ErrNotFound
	}
	k.Disabled = true
	m.keys[id] = k
	return nil
}

// Use implements Store.
func (m *Memory) Use(_ context.Context, id, day string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.usage[id+"/"+day]++
	return m.usage[id+"/"+day], nil
}
