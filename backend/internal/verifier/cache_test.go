package verifier

import (
	"context"
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
