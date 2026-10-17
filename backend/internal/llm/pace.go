package llm

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Gate is the process-wide clock for WithPace. Engine, fallback and verifier
// clients share one gate so a plan search does not put several Vertex calls
// in flight at once.
type Gate struct {
	mu    sync.Mutex
	ready time.Time
	busy  bool
}

// NewGate returns a gate whose first caller starts immediately.
func NewGate() *Gate { return &Gate{} }

// WithPace serializes calls through gate and waits gap after each one returns
// before the next starts. gap <= 0 leaves m unchanged, so tests and local
// fake runs stay instant. The wait is what keeps Vertex from answering a
// piled-up plan search with "Deadline expired" (D78).
func WithPace(m Model, gap time.Duration, gate *Gate) Model {
	if m == nil || gate == nil || gap <= 0 {
		return m
	}
	return &paced{next: m, gap: gap, gate: gate}
}

type paced struct {
	next Model
	gap  time.Duration
	gate *Gate
}

func (p *paced) Provider() string { return p.next.Provider() }

func (p *paced) Generate(ctx context.Context, req Request) (Response, error) {
	release, err := p.gate.enter(ctx)
	if err != nil {
		return Response{}, fmt.Errorf("llm: pace: %w", err)
	}
	defer release(p.gap)
	return p.next.Generate(ctx, req)
}

// enter marks the gate busy until release. release records that the next call
// may start gap after this one returned. Waiting is cancellable: a caller
// whose request ended does not sit through the rest of the queue.
func (g *Gate) enter(ctx context.Context) (func(time.Duration), error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		g.mu.Lock()
		wait := time.Until(g.ready)
		if !g.busy && wait <= 0 {
			g.busy = true
			g.mu.Unlock()
			return func(gap time.Duration) {
				g.mu.Lock()
				g.busy = false
				g.ready = time.Now().Add(gap)
				g.mu.Unlock()
			}, nil
		}
		g.mu.Unlock()
		d := wait
		if d <= 0 || d > 20*time.Millisecond {
			d = 20 * time.Millisecond
		}
		timer := time.NewTimer(d)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
