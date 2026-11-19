package verifier

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// fixedChecker returns a configurable fresh (safe) result and counts calls.
type fixedChecker struct {
	calls int
	res   Result
}

func (f *fixedChecker) Verify(context.Context, Task) (Result, error) {
	f.calls++
	return f.res, nil
}

func safeResult() Result {
	return Result{Verdict: domain.Verdict{PrecedentSupported: true, Confidence: 0.9, SupportingRuling: "N350001",
		Trail: []domain.TrailStep{{Tool: "search", Query: "q", Rulings: []string{"N350001"}}}}}
}

func seed(t *testing.T, c *Cache, tk Task, r Result) {
	t.Helper()
	b, _ := json.Marshal(r)
	if err := c.blobs.Put(context.Background(), c.key(tk), b); err != nil {
		t.Fatal(err)
	}
}

func cacheTask() Task {
	return Task{FactsA: "a", FactsB: "b", Target: "Thailand", Exclude: []string{"N328954"}}
}

// Unsafe legacy verdicts must miss; the contract is shared with #297 via
// cachedViolation.
func TestCacheLookupRejectsUnsafeLegacyVerdicts(t *testing.T) {
	cases := map[string]Result{
		"supports an excluded ruling": {Verdict: domain.Verdict{PrecedentSupported: true, SupportingRuling: "N328954", Confidence: 0.9,
			Trail: []domain.TrailStep{{Tool: "search", Rulings: []string{"N328954"}}}}},
		"support absent from the trail": {Verdict: domain.Verdict{PrecedentSupported: true, SupportingRuling: "N350001", Confidence: 0.9,
			Trail: []domain.TrailStep{{Tool: "search", Rulings: []string{"N340000"}}}}},
		"excluded support with no trail (pre-D80)": {Verdict: domain.Verdict{PrecedentSupported: true, SupportingRuling: "N328954", Confidence: 0.9}},
		"excluded counter-ruling": {Verdict: domain.Verdict{SupportingRuling: "N350001", CounterRuling: "N328954", Confidence: 0.9,
			Trail: []domain.TrailStep{{Tool: "search", Rulings: []string{"N350001"}}}}},
	}
	for name, legacy := range cases {
		t.Run(name, func(t *testing.T) {
			inner := &fixedChecker{res: safeResult()}
			c := NewCacheIn(inner, &memBlobs{m: map[string][]byte{}}, "m")
			tk := cacheTask()
			seed(t, c, tk, legacy)
			if _, ok, err := c.Peek(context.Background(), tk); err != nil || ok {
				t.Fatalf("Peek replayed unsafe verdict: ok=%v err=%v", ok, err)
			}
			r, err := c.Verify(context.Background(), tk)
			if err != nil || r.Cached || inner.calls != 1 || r.Verdict.SupportingRuling != "N350001" {
				t.Fatalf("unsafe verdict replayed: %+v calls=%d err=%v", r, inner.calls, err)
			}
			// The corrected verdict replaces it even though the store never
			// overwrites a key, so the next lookup hits without a rerun.
			r, _ = c.Verify(context.Background(), tk)
			if !r.Cached || inner.calls != 1 {
				t.Errorf("repaired verdict not reused: cached=%v calls=%d", r.Cached, inner.calls)
			}
		})
	}
}

func TestCacheLookupKeepsSafeVerdicts(t *testing.T) {
	inner := &fixedChecker{res: safeResult()}
	c := NewCacheIn(inner, &memBlobs{m: map[string][]byte{}}, "m")
	tk := cacheTask()
	seed(t, c, tk, safeResult())
	tk4 := cacheTask()
	tk4.FactsB = "pre-d80"
	seed(t, c, tk4, Result{Verdict: domain.Verdict{PrecedentSupported: true, SupportingRuling: "N350001", Confidence: 0.9}}) // no trail: kept (D56)
	if r, _ := c.Verify(context.Background(), tk4); !r.Cached || inner.calls != 0 {
		t.Errorf("pre-D80 trail-less verdict dropped: %+v", r)
	}
	seed2 := Result{Verdict: domain.Verdict{Confidence: 0.3}} // unsupported, cites nothing, no trail (pre-D80)
	tk2 := cacheTask()
	tk2.FactsB = "other"
	seed(t, c, tk2, seed2)
	for _, x := range []Task{tk, tk2} {
		if r, err := c.Verify(context.Background(), x); err != nil || !r.Cached || inner.calls != 0 {
			t.Errorf("safe verdict not replayed: %+v calls=%d err=%v", r, inner.calls, err)
		}
	}
	// A panel verdict may carry a counter-ruling from another run: only its
	// exclusion matters, not the trail.
	p := safeResult()
	p.Verdict.CounterRuling = "N340000"
	tk3 := cacheTask()
	tk3.FactsB = "panel"
	seed(t, c, tk3, p)
	if r, _ := c.Verify(context.Background(), tk3); !r.Cached || inner.calls != 0 {
		t.Errorf("panel counter-ruling verdict missed: %+v", r)
	}
}

// Revalidation is against the task's own exclusions, so a verdict citing a
// ruling that is excluded only for another task stays valid.
func TestCacheRevalidatesAgainstTaskExclusions(t *testing.T) {
	inner := &fixedChecker{res: safeResult()}
	c := NewCacheIn(inner, &memBlobs{m: map[string][]byte{}}, "m")
	tk := cacheTask()
	tk.Exclude = nil
	r := safeResult()
	r.Verdict.SupportingRuling = "N328954"
	r.Verdict.Trail = []domain.TrailStep{{Tool: "read", Rulings: []string{"N328954"}}}
	seed(t, c, tk, r)
	if got, _ := c.Verify(context.Background(), tk); !got.Cached {
		t.Error("verdict valid for a task with no exclusions was dropped")
	}
}

func TestCacheValidationEpochIsNotThePromptVersion(t *testing.T) {
	if ValidationEpoch == "" || ValidationEpoch == PromptVersion {
		t.Errorf("epoch %q", ValidationEpoch)
	}
	c := NewCacheIn(New(nil, nil, Options{}), &memBlobs{m: map[string][]byte{}}, "m")
	if checkerVersion(c.inner) != PromptVersion {
		t.Error("validation change must not alter the prompt version")
	}
}
