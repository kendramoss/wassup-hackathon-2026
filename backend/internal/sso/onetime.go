package sso

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrExists means a one-time key is already taken (a replayed assertion).
var ErrExists = errors.New("sso: already used")

// ErrNoState means a one-time key is unknown, used or expired.
var ErrNoState = errors.New("sso: unknown or expired sign-in state")

// OneTime keeps short-lived values that may be taken once: sign-in state and
// the IDs of SAML assertions already accepted.
type OneTime interface {
	// Put stores val under key until ttl passes; ErrExists if the key is
	// held and not expired.
	Put(ctx context.Context, key string, val []byte, ttl time.Duration) error
	// Take returns and deletes the value; ErrNoState if missing or expired.
	Take(ctx context.Context, key string) ([]byte, error)
}

// MemoryOneTime is an in-process OneTime (one instance, tests).
type MemoryOneTime struct {
	mu  sync.Mutex
	m   map[string]oneTimeEntry
	now func() time.Time
}

type oneTimeEntry struct {
	val     []byte
	expires time.Time
}

// NewMemoryOneTime returns an empty store.
func NewMemoryOneTime(now func() time.Time) *MemoryOneTime {
	if now == nil {
		now = time.Now
	}
	return &MemoryOneTime{m: map[string]oneTimeEntry{}, now: now}
}

// Put implements OneTime.
func (s *MemoryOneTime) Put(_ context.Context, key string, val []byte, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if e, ok := s.m[key]; ok && now.Before(e.expires) {
		return ErrExists
	}
	if len(s.m) > 10000 { // sweep expired entries now and then
		for k, e := range s.m {
			if !now.Before(e.expires) {
				delete(s.m, k)
			}
		}
	}
	s.m[key] = oneTimeEntry{val: append([]byte(nil), val...), expires: now.Add(ttl)}
	return nil
}

// Take implements OneTime.
func (s *MemoryOneTime) Take(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[key]
	delete(s.m, key)
	if !ok || !s.now().Before(e.expires) {
		return nil, ErrNoState
	}
	return e.val, nil
}
