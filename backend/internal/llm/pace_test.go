package llm

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestPaceSpacesCalls(t *testing.T) {
	var mu sync.Mutex
	var spans [][2]time.Time
	fake := &Fake{Respond: func(Request) (Response, error) {
		start := time.Now()
		time.Sleep(30 * time.Millisecond)
		mu.Lock()
		spans = append(spans, [2]time.Time{start, time.Now()})
		mu.Unlock()
		return Response{Text: "ok"}, nil
	}}
	m := WithPace(fake, 40*time.Millisecond, NewGate())
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := m.Generate(context.Background(), Request{}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if len(spans) != 3 {
		t.Fatalf("calls = %d, want 3", len(spans))
	}
	for i := 1; i < len(spans); i++ {
		gap := spans[i][0].Sub(spans[i-1][1])
		if gap < 30*time.Millisecond {
			t.Errorf("gap between call %d and %d = %s, want at least 40ms", i-1, i, gap)
		}
		if spans[i][0].Before(spans[i-1][1]) {
			t.Errorf("call %d overlapped call %d", i, i-1)
		}
	}
}

func TestPaceZeroIsUnwrapped(t *testing.T) {
	f := FakeText("ok")
	if got := WithPace(f, 0, NewGate()); got != f {
		t.Errorf("gap 0 wrapped the model: %T", got)
	}
}

func TestPaceHonoursCancel(t *testing.T) {
	hold := make(chan struct{})
	fake := &Fake{Respond: func(Request) (Response, error) {
		<-hold
		return Response{Text: "ok"}, nil
	}}
	m := WithPace(fake, time.Second, NewGate())
	done := make(chan struct{})
	go func() {
		_, _ = m.Generate(context.Background(), Request{})
		close(done)
	}()
	time.Sleep(20 * time.Millisecond) // the first call holds the gate
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := m.Generate(ctx, Request{})
	if err == nil {
		t.Fatal("second call returned while the first still held the gate")
	}
	close(hold)
	<-done
}
