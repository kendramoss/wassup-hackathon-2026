package llm

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPaceRunsAtFullSpeedUntilAnOverload(t *testing.T) {
	var fail atomic.Bool
	var mu sync.Mutex
	var starts []time.Time
	fake := &Fake{Respond: func(Request) (Response, error) {
		mu.Lock()
		starts = append(starts, time.Now())
		mu.Unlock()
		time.Sleep(10 * time.Millisecond)
		if fail.Load() {
			return Response{}, &APIError{Provider: "vertex", StatusCode: 429}
		}
		return Response{Text: "ok"}, nil
	}}
	g := NewGate(0, time.Second)
	m := WithPace(fake, 50*time.Millisecond, g)
	run := func(n int) {
		var wg sync.WaitGroup
		for range n {
			wg.Add(1)
			go func() { defer wg.Done(); _, _ = m.Generate(context.Background(), Request{}) }()
		}
		wg.Wait()
	}
	t0 := time.Now()
	run(3)
	if d := time.Since(t0); d > 40*time.Millisecond || g.Cooling() {
		t.Errorf("healthy calls should run together: %s, cooling %v", d, g.Cooling())
	}
	fail.Store(true)
	run(1)
	if !g.Cooling() {
		t.Fatal("a 429 should start the cool-down")
	}
	fail.Store(false)
	starts = nil
	run(3)
	for i := 1; i < len(starts); i++ {
		if gap := starts[i].Sub(starts[i-1]); gap < 40*time.Millisecond {
			t.Errorf("while cooling, starts %d and %d are %s apart, want about 50ms", i-1, i, gap)
		}
	}
}

func TestPaceIgnoresBadRequestsAndCancels(t *testing.T) {
	g := NewGate(0, time.Second)
	m := WithPace(&Fake{Respond: func(Request) (Response, error) {
		return Response{}, &APIError{Provider: "vertex", StatusCode: 400}
	}}, time.Second, g)
	_, _ = m.Generate(context.Background(), Request{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = WithPace(&Fake{Respond: func(Request) (Response, error) { return Response{}, context.Canceled }}, time.Second, g).Generate(ctx, Request{})
	if g.Cooling() {
		t.Error("a 400 or the caller's own cancel is not an overload")
	}
}

func TestPaceCapsCallsInFlight(t *testing.T) {
	var now, peak atomic.Int32
	fake := &Fake{Respond: func(Request) (Response, error) {
		n := now.Add(1)
		for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
		}
		time.Sleep(20 * time.Millisecond)
		now.Add(-1)
		return Response{Text: "ok"}, nil
	}}
	m := WithPace(fake, 0, NewGate(2, time.Second))
	var wg sync.WaitGroup
	for range 6 {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = m.Generate(context.Background(), Request{}) }()
	}
	wg.Wait()
	if peak.Load() != 2 {
		t.Errorf("peak in flight = %d, want 2", peak.Load())
	}
}

func TestPaceZeroIsUnwrapped(t *testing.T) {
	f := FakeText("ok")
	if got := WithPace(f, 0, NewGate(0, time.Second)); got != f {
		t.Errorf("no cap and gap 0 wrapped the model: %T", got)
	}
}

func TestPaceHonoursCancelWhileWaitingForASlot(t *testing.T) {
	hold := make(chan struct{})
	fake := &Fake{Respond: func(Request) (Response, error) {
		<-hold
		return Response{Text: "ok"}, nil
	}}
	m := WithPace(fake, 0, NewGate(1, time.Second))
	done := make(chan struct{})
	go func() {
		_, _ = m.Generate(context.Background(), Request{})
		close(done)
	}()
	time.Sleep(20 * time.Millisecond) // the first call holds the only slot
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := m.Generate(ctx, Request{}); err == nil {
		t.Fatal("second call returned while the first still held the slot")
	}
	close(hold)
	<-done
}
