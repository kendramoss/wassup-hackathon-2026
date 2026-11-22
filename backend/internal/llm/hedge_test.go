package llm

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// scripted answers call i after delays[i] with texts[i] or errs[i].
type scripted struct {
	mu     sync.Mutex
	calls  int
	delays []time.Duration
	errs   []error
	done   atomic.Int32 // calls that returned because their context ended
}

func (s *scripted) Provider() string { return "fake" }

func (s *scripted) Generate(ctx context.Context, _ Request) (Response, error) {
	s.mu.Lock()
	i := s.calls
	s.calls++
	s.mu.Unlock()
	select {
	case <-time.After(s.delays[i]):
	case <-ctx.Done():
		s.done.Add(1)
		return Response{}, ctx.Err()
	}
	if i < len(s.errs) && s.errs[i] != nil {
		return Response{}, s.errs[i]
	}
	return Response{Text: string(rune('a' + i))}, nil
}

func TestHedgeOffLeavesModelUnchanged(t *testing.T) {
	m := &scripted{delays: []time.Duration{0}}
	if WithHedge(m, 0) != Model(m) {
		t.Fatal("a zero hedge must return the model itself")
	}
}

func TestHedgeFastCallSendsOnce(t *testing.T) {
	m := &scripted{delays: []time.Duration{time.Millisecond}}
	resp, err := WithHedge(m, 200*time.Millisecond).Generate(context.Background(), Request{})
	if err != nil || resp.Text != "a" || m.calls != 1 {
		t.Fatalf("got %q %v after %d calls, want a from one call", resp.Text, err, m.calls)
	}
}

func TestHedgeSlowCallIsBeatenAndCancelled(t *testing.T) {
	m := &scripted{delays: []time.Duration{5 * time.Second, 5 * time.Millisecond}}
	start := time.Now()
	resp, err := WithHedge(m, 20*time.Millisecond).Generate(context.Background(), Request{})
	if err != nil || resp.Text != "b" {
		t.Fatalf("got %q %v, want the hedge's answer", resp.Text, err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("took %s: the slow first call was waited for", d)
	}
	deadline := time.Now().Add(time.Second)
	for m.done.Load() != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if m.done.Load() != 1 {
		t.Fatal("the losing call was not cancelled")
	}
}

func TestHedgeEarlyErrorIsReturnedWithoutHedging(t *testing.T) {
	boom := errors.New("bad request")
	m := &scripted{delays: []time.Duration{time.Millisecond}, errs: []error{boom}}
	if _, err := WithHedge(m, time.Second).Generate(context.Background(), Request{}); !errors.Is(err, boom) || m.calls != 1 {
		t.Fatalf("err %v after %d calls, want the first error and no hedge", err, m.calls)
	}
}

func TestHedgeOneFailureAfterHedgingUsesTheOther(t *testing.T) {
	boom := errors.New("503")
	// The hedge fails fast; the first call answers later.
	m := &scripted{delays: []time.Duration{60 * time.Millisecond, time.Millisecond}, errs: []error{nil, boom}}
	resp, err := WithHedge(m, 10*time.Millisecond).Generate(context.Background(), Request{})
	if err != nil || resp.Text != "a" {
		t.Fatalf("got %q %v, want the first call's answer", resp.Text, err)
	}
}

func TestHedgeBothFailReturnsTheFirstError(t *testing.T) {
	e1, e2 := errors.New("first"), errors.New("second")
	m := &scripted{delays: []time.Duration{30 * time.Millisecond, 50 * time.Millisecond}, errs: []error{e1, e2}}
	if _, err := WithHedge(m, 10*time.Millisecond).Generate(context.Background(), Request{}); !errors.Is(err, e1) {
		t.Fatalf("err %v, want the first error to arrive", err)
	}
}
