package verifier

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

type countingChecker struct{ calls int }

func (c *countingChecker) Verify(context.Context, Task) (Result, error) {
	c.calls++
	conf := 0.95
	if c.calls > 1 {
		conf = 0.92 // a fresh run would drift
	}
	return Result{Verdict: domain.Verdict{PrecedentSupported: true, Confidence: conf, SupportingRuling: "N350001"}, ModelCalls: 4, Steps: make([]Step, 3)}, nil
}

func TestCacheReturnsTheSameVerdict(t *testing.T) {
	dir := t.TempDir()
	inner := &countingChecker{}
	task := Task{FactsA: "a", FactsB: "b", Target: "Thailand", Before: time.Date(2026, 1, 14, 0, 0, 0, 0, time.UTC), Exclude: []string{"N2", "N1"}}
	c, err := NewCache(inner, dir, "m")
	if err != nil {
		t.Fatal(err)
	}
	first, _ := c.Verify(context.Background(), task)
	task.Exclude = []string{"N1", "N2"} // order does not matter
	again, _ := c.Verify(context.Background(), task)
	if inner.calls != 1 || again.Verdict.Confidence != 0.95 || !again.Cached || again.ModelCalls != 0 || len(again.Steps) != 0 || first.Cached {
		t.Errorf("calls=%d first=%+v again=%+v", inner.calls, first, again)
	}
	reopened, _ := NewCache(inner, dir, "m") // survives a restart
	if r, _ := reopened.Verify(context.Background(), task); inner.calls != 1 || r.Verdict.Confidence != 0.95 {
		t.Errorf("after reopen: calls=%d verdict=%+v", inner.calls, r.Verdict)
	}
	other, _ := NewCache(inner, dir, "other-model")
	if _, _ = other.Verify(context.Background(), task); inner.calls != 2 {
		t.Errorf("a different model must not reuse verdicts: calls=%d", inner.calls)
	}
}

type memBlobs struct {
	mu   sync.Mutex
	m    map[string][]byte
	fail error
}

func (b *memBlobs) Get(_ context.Context, k string) ([]byte, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.fail != nil {
		return nil, false, b.fail
	}
	v, ok := b.m[k]
	return v, ok, nil
}

func (b *memBlobs) Put(_ context.Context, k string, v []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.m[k]; !ok {
		b.m[k] = v
	}
	return nil
}

func TestCacheInSharedBlobs(t *testing.T) {
	blobs := &memBlobs{m: map[string][]byte{}}
	inner := &countingChecker{}
	a, b := NewCacheIn(inner, blobs, "m"), NewCacheIn(inner, blobs, "m") // two instances
	task := Task{FactsA: "a", FactsB: "b", Target: "Thailand"}
	r1, err := a.Verify(context.Background(), task)
	if err != nil || r1.Cached {
		t.Fatalf("first = %+v %v", r1, err)
	}
	r2, err := b.Verify(context.Background(), task)
	if err != nil || !r2.Cached || r2.Verdict != r1.Verdict || inner.calls != 1 {
		t.Errorf("other instance = %+v %v, inner calls %d", r2, err, inner.calls)
	}
	blobs.fail = errors.New("permission denied")
	if _, err := a.Verify(context.Background(), task); err == nil {
		t.Error("a failed cache read must not silently re-verify")
	}
}

type flakyChecker struct {
	mu                sync.Mutex
	toolErrors, calls int
}

func (f *flakyChecker) Verify(context.Context, Task) (Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return Result{Verdict: domain.Verdict{PrecedentSupported: false, Confidence: 0.3}, ToolErrors: f.toolErrors}, nil
}

func TestCacheSkipsVerdictsMadeWithoutEvidence(t *testing.T) {
	blobs := &memBlobs{m: map[string][]byte{}}
	inner := &flakyChecker{toolErrors: 1} // CROSS failed mid-check
	c := NewCacheIn(inner, blobs, "m")
	task := Task{FactsA: "a", FactsB: "b", Target: "Thailand"}
	c.Verify(context.Background(), task)
	if len(blobs.m) != 0 {
		t.Fatal("a verdict reached during a tool failure was cached")
	}
	inner.toolErrors = 0 // CROSS back
	c.Verify(context.Background(), task)
	if r, _ := c.Verify(context.Background(), task); !r.Cached || inner.calls != 2 {
		t.Errorf("clean verdict not cached: cached %v, calls %d", r.Cached, inner.calls)
	}
}

func TestMajorityCarriesToolErrors(t *testing.T) {
	m := NewMajority(&flakyChecker{toolErrors: 1}, 3).(*Majority)
	r, err := m.Verify(context.Background(), Task{})
	if err != nil || r.ToolErrors != 3 {
		t.Errorf("majority = %d tool errors, %v", r.ToolErrors, err)
	}
}
