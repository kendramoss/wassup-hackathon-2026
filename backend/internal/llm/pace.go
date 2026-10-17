package llm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Gate is the process-wide limit for WithPace. Engine, fallback and verifier
// clients share one gate. Normally it only caps how many calls are in flight
// at once. After a call fails because the provider is overloaded (429, 5xx,
// deadline), it spaces call starts by the gap for a cool-down, then goes back
// to full speed (D78, D79).
type Gate struct {
	slots chan struct{} // nil = no in-flight cap
	cool  time.Duration

	mu        sync.Mutex
	coolUntil time.Time
	next      time.Time // earliest start for the next call while cooling
}

// NewGate returns a gate with at most maxInFlight calls at once (0 = no cap)
// and a cool-down of cool after an overload.
func NewGate(maxInFlight int, cool time.Duration) *Gate {
	g := &Gate{cool: cool}
	if maxInFlight > 0 {
		g.slots = make(chan struct{}, maxInFlight)
	}
	return g
}

// WithPace sends calls through gate. gap is the spacing between call starts
// while the gate is cooling down after an overload. A gate with no in-flight
// cap and gap <= 0 leaves m unchanged, so tests and local fake runs stay
// instant.
func WithPace(m Model, gap time.Duration, gate *Gate) Model {
	if m == nil || gate == nil || (gate.slots == nil && gap <= 0) {
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
	release, err := p.gate.enter(ctx, p.gap)
	if err != nil {
		return Response{}, fmt.Errorf("llm: pace: %w", err)
	}
	resp, err := p.next.Generate(ctx, req)
	release()
	if err != nil && overloaded(ctx, err) {
		p.gate.overload()
	}
	return resp, err
}

// overloaded reports whether err says the provider is busy, not that the
// request was bad or the caller gave up.
func overloaded(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	return retryable(err) || errors.Is(err, context.DeadlineExceeded)
}

func (g *Gate) overload() {
	g.mu.Lock()
	g.coolUntil = time.Now().Add(g.cool)
	g.mu.Unlock()
}

// Cooling reports whether the gate is spacing calls after an overload.
func (g *Gate) Cooling() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return time.Now().Before(g.coolUntil)
}

// enter takes an in-flight slot and, while cooling, waits for this call's
// start time (call starts gap apart). Waiting is cancellable: a caller whose
// request ended does not sit through the rest of the queue.
func (g *Gate) enter(ctx context.Context, gap time.Duration) (func(), error) {
	if g.slots != nil {
		select {
		case g.slots <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	release := func() {
		if g.slots != nil {
			<-g.slots
		}
	}
	if gap > 0 {
		g.mu.Lock()
		now := time.Now()
		var wait time.Duration
		if now.Before(g.coolUntil) {
			start := now
			if g.next.After(start) {
				start = g.next
			}
			g.next = start.Add(gap)
			wait = start.Sub(now)
		}
		g.mu.Unlock()
		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				release()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return release, nil
}
