// Package ratelimit holds the counters behind the shared rate limiter (D70).
// The limiter in httpapi gives every Cloud Run instance its own buckets, so
// with 3 instances the real limits are about 3× the configured ones (D57,
// D59). A Store keeps fixed-window counters that all instances share;
// httpapi decides what to count and what the limits are.
package ratelimit

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Counter is one increment: N added to the counter named ID. N may be
// negative (a refused request taking back what it added).
type Counter struct {
	ID string
	N  int64
}

// Store adds to several counters in one atomic write and returns each
// counter's new total, in order. expires is when the counters' window ends;
// after it they are never read again and may be deleted.
type Store interface {
	Add(ctx context.Context, cs []Counter, expires time.Time) ([]int64, error)
}

// Memory is an in-process Store (tests, and RATE_LIMIT_STORE=memory to try
// the shared limiter locally).
type Memory struct {
	mu     sync.Mutex
	counts map[string]entry
	newest time.Time // latest window end seen; counters of earlier windows are dropped
	fail   error
}

type entry struct {
	n       int64
	expires time.Time
}

// NewMemory returns an empty store.
func NewMemory() *Memory { return &Memory{counts: map[string]entry{}} }

// ErrUnavailable is what a failing Memory returns (see Fail).
var ErrUnavailable = errors.New("ratelimit: store unavailable")

// Add implements Store.
func (s *Memory) Add(_ context.Context, cs []Counter, expires time.Time) ([]int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return nil, s.fail
	}
	if expires.After(s.newest) { // a new window: forget the ended ones
		s.newest = expires
		for id, e := range s.counts {
			if e.expires.Before(expires) {
				delete(s.counts, id)
			}
		}
	}
	out := make([]int64, len(cs))
	for i, c := range cs {
		e := s.counts[c.ID]
		e.n += c.N
		if expires.After(e.expires) {
			e.expires = expires
		}
		s.counts[c.ID] = e
		out[i] = e.n
	}
	return out, nil
}

// Fail makes every later Add return err; nil makes the store work again.
// Tests use it for a store outage.
func (s *Memory) Fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = err
}

// Len is the number of counters held.
func (s *Memory) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.counts)
}
