package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

// Plan-search dedupe (D402). An MCP client that times out on
// plan_production_moves and retries, or a REST client that resends, must
// not start the same model-heavy search twice. A non-streamed plan request
// whose case and inputs match a search still running joins it and gets the
// same answer. With an Idempotency-Key header (REST) or idempotency_key
// argument (MCP) the answer is also kept for a while after it finishes, so a
// retry that arrives late gets it again instead of a new search.

const (
	// planJoinGrace keeps a search running this long after its last waiting
	// caller left, so a client that timed out can retry and join it.
	planJoinGrace = 2 * time.Minute
	// planReplayTTL is how long a finished search with an idempotency key is
	// replayed to a request with the same key.
	planReplayTTL = 15 * time.Minute
	// planReplayMax bounds the kept answers; the oldest goes first.
	planReplayMax = 256
	// idempotencyKeyMax is the longest key accepted.
	idempotencyKeyMax = 200
)

// planJoins counts callers that joined a running search, for tests.
var planJoins atomic.Int64

// errIdempotencyReuse is a key sent again with different inputs.
var errIdempotencyReuse = errors.New("conflict: this Idempotency-Key was used for a plan search with different inputs; use a new key")

type replayRef struct {
	key string
	fl  *planFlight
}

type planFlight struct {
	inputs   string
	ready    chan struct{}
	body     map[string]any
	err      error
	waiters  int
	cancel   context.CancelFunc
	idle     *time.Timer
	finished time.Time
}

// planFlights holds the plan searches in flight and the finished answers
// kept for idempotency keys.
type planFlights struct {
	mu       sync.Mutex
	inflight map[string]*planFlight
	replay   map[string]*planFlight
	order    []replayRef // replay entries, oldest first
	grace    time.Duration
	ttl      time.Duration
	now      func() time.Time
	// started counts searches actually run (tests).
	started int
}

func newPlanFlights() *planFlights {
	return &planFlights{inflight: map[string]*planFlight{}, replay: map[string]*planFlight{}, grace: planJoinGrace, ttl: planReplayTTL, now: time.Now}
}

// planInputs hashes everything a plan search's answer depends on: the
// planner request as priced, the money basis, the case inputs and whether
// the result is stored.
func planInputs(req planner.Request, basis, fingerprint string, readOnly bool) string {
	b, _ := json.Marshal(struct {
		Req         planner.Request
		Basis, Case string
		ReadOnly    bool
	}{req, basis, fingerprint, readOnly})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// idempotencyKey reads and checks the optional Idempotency-Key header.
func idempotencyKey(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	if len(v) > idempotencyKeyMax {
		return "", fmt.Errorf("%w: Idempotency-Key is longer than %d characters", domain.ErrInvalid, idempotencyKeyMax)
	}
	for _, r := range v {
		if r < 0x21 || r > 0x7e {
			return "", fmt.Errorf("%w: Idempotency-Key must be printable ASCII without spaces", domain.ErrInvalid)
		}
	}
	return v, nil
}

// do runs run once per key: a caller with the same key while it runs waits
// for the same answer. run gets a context that outlives the callers: it is
// cancelled only when every caller has gone and none joined within the
// grace period. With keep, a finished answer is replayed to the same key for
// the TTL; a key whose inputs differ is refused. joined reports whether
// this caller got another caller's answer.
func (f *planFlights) do(ctx context.Context, key, inputs string, keep bool, run func(context.Context) (map[string]any, error)) (body map[string]any, joined bool, err error) {
	f.mu.Lock()
	if keep {
		f.expire()
		if fl, ok := f.replay[key]; ok {
			f.mu.Unlock()
			if fl.inputs != inputs {
				return nil, false, errIdempotencyReuse
			}
			return fl.body, true, nil
		}
	}
	fl, ok := f.inflight[key]
	if ok {
		if fl.inputs != inputs {
			f.mu.Unlock()
			return nil, false, errIdempotencyReuse
		}
		fl.waiters++
		planJoins.Add(1)
		if fl.idle != nil {
			fl.idle.Stop()
			fl.idle = nil
		}
		joined = true
	} else {
		runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		fl = &planFlight{inputs: inputs, ready: make(chan struct{}), waiters: 1, cancel: cancel}
		f.inflight[key] = fl
		f.started++
		go func() {
			b, err := run(runCtx)
			f.mu.Lock()
			fl.body, fl.err, fl.finished = b, err, f.now()
			if f.inflight[key] == fl {
				delete(f.inflight, key)
			}
			if fl.idle != nil {
				fl.idle.Stop()
			}
			if keep && err == nil {
				f.replay[key] = fl
				f.order = append(f.order, replayRef{key, fl})
				for len(f.order) > planReplayMax {
					f.drop()
				}
			}
			f.mu.Unlock()
			cancel()
			close(fl.ready)
		}()
	}
	f.mu.Unlock()

	select {
	case <-fl.ready:
		return fl.body, joined, fl.err
	case <-ctx.Done():
		f.mu.Lock()
		fl.waiters--
		if fl.waiters == 0 && f.inflight[key] == fl {
			fl.idle = time.AfterFunc(f.grace, func() {
				f.mu.Lock()
				defer f.mu.Unlock()
				if fl.waiters == 0 && f.inflight[key] == fl {
					delete(f.inflight, key) // a later request starts afresh
					fl.cancel()
				}
			})
		}
		f.mu.Unlock()
		return nil, joined, ctx.Err()
	}
}

// expire drops replayable answers older than the TTL. f.mu is held.
func (f *planFlights) expire() {
	cut := f.now().Add(-f.ttl)
	for len(f.order) > 0 && !f.order[0].fl.finished.After(cut) {
		f.drop()
	}
}

// drop removes the oldest replay entry. f.mu is held.
func (f *planFlights) drop() {
	r := f.order[0]
	if f.replay[r.key] == r.fl {
		delete(f.replay, r.key)
	}
	f.order = f.order[1:]
}

// running is the number of searches in flight (tests).
func (f *planFlights) running() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.inflight)
}

// waiting is the number of callers waiting on key's search (tests).
func (f *planFlights) waiting(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	if fl, ok := f.inflight[key]; ok {
		return fl.waiters
	}
	return 0
}
