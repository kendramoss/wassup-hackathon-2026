package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/logsafe"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/ratelimit"
)

// rateWindow is the shared limiter's fixed window: the limits are per minute.
const rateWindow = time.Minute

// sharedStoreTimeout bounds the counter write; past it the request falls
// back to this instance's buckets rather than wait on the store.
const sharedStoreTimeout = time.Second

// fullFor is how long a counter seen full refuses without a store write.
const fullFor = 10 * time.Second

// sharedLimiter enforces the same classes and limits as limiter, but with
// fixed-window counters in a store every instance shares (D70), so a limit
// holds for the whole service rather than per instance. One request is one
// store write: its client counter and the global counter are incremented
// together and the new totals come back with the write. A refused request
// takes its increments back (a second write), so it uses up nothing.
//
// A fixed window allows up to twice a limit across a window boundary (the
// end of one minute and the start of the next); the token buckets did not.
type sharedLimiter struct {
	lim   Limits
	store ratelimit.Store
	now   func() time.Time

	mu      sync.Mutex
	lastLog time.Time // store errors are logged at most once a window
	// full holds counters seen at their limit, until when to refuse
	// without a store write (a client retrying in a loop then costs
	// nothing, and the global counter, which every request writes, is
	// spared). Within a window a counter only grows by allowed requests,
	// but a total read while another refused request has not yet taken its
	// increment back can be one too high, so the mark lasts fullFor, not
	// the whole window.
	full map[string]time.Time
}

func newSharedLimiter(l Limits, s ratelimit.Store) *sharedLimiter {
	return &sharedLimiter{lim: l, store: s, now: time.Now, full: map[string]time.Time{}}
}

// counters returns what one request of class c for key adds, and the limit
// each counter must stay within. Disabled limits (0) are left out.
func (s *sharedLimiter) counters(key string, c class, start time.Time) ([]ratelimit.Counter, []int64) {
	var cs []ratelimit.Counter
	var limits []int64
	bucket, perClient, n, weight := "light", s.lim.LightPerMin, 1, 1
	switch c {
	case heavy:
		bucket, perClient, weight = "heavy", s.lim.HeavyPerMin, HeavyWeight
	case bulk: // a whole heavy burst: one batch a minute per client (D57)
		bucket, perClient, n, weight = "heavy", s.lim.HeavyPerMin, max(1, s.lim.HeavyPerMin), MaxBatch
	case lookup:
		bucket, perClient, weight = "lookup", s.lim.LookupPerMin, 0
	case basic: // never sent here (counted per instance); kept for completeness
		bucket, perClient, weight = "basic", s.lim.BasicPerMin, 0
	}
	win := start.Unix()
	if perClient > 0 {
		// The key is hashed: the store never holds a visitor's address, and
		// every key makes a valid document ID.
		h := sha256.Sum256([]byte(key))
		cs = append(cs, ratelimit.Counter{ID: fmt.Sprintf("%s-%s-%d", hex.EncodeToString(h[:12]), bucket, win), N: int64(n)})
		limits = append(limits, int64(perClient))
	}
	if weight > 0 && s.lim.GlobalPerMin > 0 {
		cs = append(cs, ratelimit.Counter{ID: fmt.Sprintf("global-%d", win), N: int64(weight)})
		limits = append(limits, int64(s.lim.GlobalPerMin))
	}
	return cs, limits
}

// perClient is the per-client limit of class c (0: none).
func (s *sharedLimiter) perClient(c class) int {
	switch c {
	case heavy, bulk:
		return s.lim.HeavyPerMin
	case lookup:
		return s.lim.LookupPerMin
	case basic:
		return s.lim.BasicPerMin
	}
	return s.lim.LightPerMin
}

// allow takes one request of class c for key. When refused it returns how
// long until the window ends. A store error is returned for the caller to
// fall back on its local limiter.
func (s *sharedLimiter) allow(ctx context.Context, key string, c class) (bool, time.Duration, error) {
	d, err := s.decide(ctx, key, c)
	return d.ok, d.wait, err
}

// decide is allow with the client counter's state for the RateLimit-*
// headers: its limit, what is left of it and when the window ends.
func (s *sharedLimiter) decide(ctx context.Context, key string, c class) (rateDecision, error) {
	now := s.now()
	start := now.Truncate(rateWindow)
	end := start.Add(rateWindow)
	cs, limits := s.counters(key, c, start)
	if len(cs) == 0 {
		return rateDecision{ok: true}, nil
	}
	// The client counter, when the class has a per-client limit, is first
	// (see counters).
	clientLimit := s.perClient(c)
	if s.anyFull(cs, now) {
		return rateDecision{wait: end.Sub(now), limit: clientLimit, reset: end.Sub(now)}, nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sharedStoreTimeout)
	defer cancel()
	totals, err := s.store.Add(ctx, cs, end)
	if err != nil {
		return rateDecision{}, err
	}
	refused := false
	for i, t := range totals {
		if t > limits[i] {
			refused = true
			if t-cs[i].N >= limits[i] { // full even without this request
				until := now.Add(fullFor)
				if end.Before(until) {
					until = end
				}
				s.markFull(cs[i].ID, until, now)
			}
		}
	}
	d := rateDecision{limit: clientLimit, reset: end.Sub(now)}
	if !refused {
		d.ok = true
		if clientLimit > 0 {
			d.remaining = clientLimit - int(totals[0])
		}
		return d, nil
	}
	undo := make([]ratelimit.Counter, len(cs))
	for i, x := range cs {
		undo[i] = ratelimit.Counter{ID: x.ID, N: -x.N}
	}
	// Best effort: if this fails the request still counts until the window ends.
	_, _ = s.store.Add(ctx, undo, end)
	d.wait = end.Sub(now)
	return d, nil
}

func (s *sharedLimiter) anyFull(cs []ratelimit.Counter, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range cs {
		if end, ok := s.full[c.ID]; ok && now.Before(end) {
			return true
		}
	}
	return false
}

func (s *sharedLimiter) markFull(id string, end, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, e := range s.full { // drop ended windows so the map stays small
		if !now.Before(e) {
			delete(s.full, k)
		}
	}
	s.full[id] = end
}

// shouldLog reports whether a store error now should be logged: once a
// window, so an outage does not flood the logs.
func (s *sharedLimiter) shouldLog(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.Sub(s.lastLog) < rateWindow {
		return false
	}
	s.lastLog = now
	return true
}

// rateCheck applies the rate limits to r for the client key: the shared
// counters when configured, this instance's buckets otherwise. If the shared
// store fails, the request falls back to this instance's buckets (fail open
// to the local limits, D70) rather than being refused or let through
// unlimited. The basic class is always counted here (D137): a store write
// per read would cost more than the limit protects.
func (a *api) rateCheck(r *http.Request, key string, c class) rateDecision {
	if a.shared != nil && c != basic {
		d, err := a.shared.decide(r.Context(), key, c)
		if err == nil {
			return d
		}
		if a.shared.shouldLog(a.shared.now()) {
			a.d.Logger.LogAttrs(r.Context(), slog.LevelWarn, "shared rate limiter unavailable; using this instance's limits",
				slog.Any("request_id", requestRef(r.Context())), slog.Any("err", logsafe.Err(err)))
		}
	}
	return a.limiter.decide(key, c)
}

// allow is rateCheck for r's own client key, without the header state.
func (a *api) allow(r *http.Request, c class) (bool, time.Duration) {
	d := a.rateCheck(r, clientKey(r), c)
	return d.ok, d.wait
}
