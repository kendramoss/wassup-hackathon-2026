package verifier

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// fixedChecker returns a configurable fresh result (given the task, so proof
// can match it) and counts calls.
type fixedChecker struct {
	calls int
	make  func(Task) Result
}

func (f *fixedChecker) Verify(_ context.Context, t Task) (Result, error) {
	f.calls++
	return f.make(t), nil
}

var bound = time.Date(2026, 1, 14, 0, 0, 0, 0, time.UTC)

func cacheTask() Task {
	return Task{FactsA: "a", FactsB: "b", Target: "Thailand", Before: bound, Exclude: []string{"N328954"}}
}

func goodResult(t Task) Result {
	return Result{OriginA: "China", OriginB: "Thailand", ChangesOrigin: true, Verdict: domain.Verdict{PrecedentSupported: true, Confidence: 0.9, SupportingRuling: "N350001", CounterRuling: "N340000",
		Trail: []domain.TrailStep{{Tool: "search", Query: "q", Rulings: []string{"N350001", "N340000"}}}},
		Proof: ProofFor(t, "N350001", "N340000")}
}

// withDistinctOrigins gives a supported fixture the origins of a real change
// (China -> Thailand) so tests of other rules are not decided by the origin rule.
func withDistinctOrigins(r Result) Result {
	if r.Verdict.PrecedentSupported {
		r.OriginA, r.OriginB, r.ChangesOrigin = "China", "Thailand", true
	}
	return r
}

func seed(t *testing.T, c *Cache, tk Task, r Result) {
	t.Helper()
	b, _ := json.Marshal(r)
	if err := c.blobs.Put(context.Background(), c.key(tk), b); err != nil {
		t.Fatal(err)
	}
}

func newFixedCache() (*Cache, *fixedChecker, *memBlobs) {
	inner := &fixedChecker{make: goodResult}
	blobs := &memBlobs{m: map[string][]byte{}}
	return NewCacheIn(inner, blobs, "m"), inner, blobs
}

// Entries without current validation proof, or whose proof does not cover the
// citation or the task, are a miss: even a plain, non-excluded id in a
// trail-less legacy or seeded verdict (N999999). The display Trail is not
// proof.
func TestCacheMissesWithoutCurrentProof(t *testing.T) {
	tk := cacheTask()
	other := tk
	other.Before = bound.AddDate(0, 1, 0)
	otherEx := tk
	otherEx.Exclude = []string{"N1"}
	withTrail := func(ids ...string) []domain.TrailStep {
		return []domain.TrailStep{{Tool: "read", Query: "x", Rulings: ids}}
	}
	stale := ProofFor(tk, "N350001")
	stale.Epoch = "val0"
	cases := map[string]Result{
		"legacy trail-less non-excluded N999999": {Verdict: domain.Verdict{PrecedentSupported: true, SupportingRuling: "N999999", Confidence: 0.9}},
		"legacy with trail but no proof":         {Verdict: domain.Verdict{PrecedentSupported: true, SupportingRuling: "N350001", Confidence: 0.9, Trail: withTrail("N350001")}},
		"future-dated read in the trail":         {Verdict: domain.Verdict{PrecedentSupported: true, SupportingRuling: "N360000", Confidence: 0.9, Trail: withTrail("N360000")}, Proof: ProofFor(tk, "N350001")},
		"search hit in trail, not validated":     {Verdict: domain.Verdict{PrecedentSupported: true, SupportingRuling: "N360000", Confidence: 0.9, Trail: withTrail("N360000")}, Proof: ProofFor(tk)},
		"mismatched read in trail":               {Verdict: domain.Verdict{PrecedentSupported: true, SupportingRuling: "N370000", Confidence: 0.9, Trail: withTrail("N370000")}, Proof: ProofFor(tk, "N350001")},
		"excluded support":                       {Verdict: domain.Verdict{PrecedentSupported: true, SupportingRuling: "N328954", Confidence: 0.9}, Proof: ProofFor(tk, "N328954")},
		"excluded counter":                       {Verdict: domain.Verdict{SupportingRuling: "N350001", CounterRuling: "N328954", Confidence: 0.9}, Proof: ProofFor(tk, "N350001", "N328954")},
		"counter not validated":                  {Verdict: domain.Verdict{SupportingRuling: "N350001", CounterRuling: "N340000", Confidence: 0.9}, Proof: ProofFor(tk, "N350001")},
		"proof from another epoch":               {Verdict: domain.Verdict{PrecedentSupported: true, SupportingRuling: "N350001", Confidence: 0.9}, Proof: stale},
		"proof for another date bound":           {Verdict: domain.Verdict{PrecedentSupported: true, SupportingRuling: "N350001", Confidence: 0.9}, Proof: ProofFor(other, "N350001")},
		"proof for other exclusions":             {Verdict: domain.Verdict{PrecedentSupported: true, SupportingRuling: "N350001", Confidence: 0.9}, Proof: ProofFor(otherEx, "N350001")},
	}
	for name, legacy := range cases {
		legacy = withDistinctOrigins(legacy) // isolate the proof rules from the origin rule
		t.Run(name, func(t *testing.T) {
			c, inner, blobs := newFixedCache()
			seed(t, c, tk, legacy)
			orig := string(blobs.m[c.key(tk)])
			if _, ok, err := c.Peek(context.Background(), tk); err != nil || ok {
				t.Fatalf("Peek replayed: ok=%v err=%v", ok, err)
			}
			r, err := c.Verify(context.Background(), tk)
			if err != nil || r.Cached || inner.calls != 1 || r.Verdict.SupportingRuling != "N350001" {
				t.Fatalf("replayed or not re-verified: %+v calls=%d err=%v", r, inner.calls, err)
			}
			if string(blobs.m[c.key(tk)]) != orig {
				t.Error("original slot was overwritten")
			}
			if _, ok := blobs.m[c.repairSlot(c.key(tk))]; !ok {
				t.Error("repair slot not written")
			}
			r, _ = c.Verify(context.Background(), tk)
			if !r.Cached || inner.calls != 1 {
				t.Errorf("repaired verdict not reused: cached=%v calls=%d", r.Cached, inner.calls)
			}
			// A later, different fresh verdict must not overwrite the repair slot either.
			if err := c.blobs.Put(context.Background(), c.repairSlot(c.key(tk)), []byte("{}")); err != nil {
				t.Fatal(err)
			}
			if b := blobs.m[c.repairSlot(c.key(tk))]; string(b) == "{}" {
				t.Error("repair slot is not immutable")
			}
		})
	}
}

func TestCacheHitWithCurrentProof(t *testing.T) {
	c, inner, _ := newFixedCache()
	tk := cacheTask()
	seed(t, c, tk, goodResult(tk))
	// Same task, same key.
	tk.Exclude = []string{"N328954"}
	r, err := c.Verify(context.Background(), tk)
	if err != nil || !r.Cached || inner.calls != 0 || r.Verdict.SupportingRuling != "N350001" || r.ModelCalls != 0 {
		t.Errorf("valid proof not replayed: %+v calls=%d err=%v", r, inner.calls, err)
	}
	// A verdict that cites nothing has nothing to prove and is replayed.
	tk2 := cacheTask()
	tk2.FactsB = "none"
	seed(t, c, tk2, Result{Verdict: domain.Verdict{Confidence: 0.3}})
	if r, _ := c.Verify(context.Background(), tk2); !r.Cached || inner.calls != 0 {
		t.Errorf("citation-free verdict missed: %+v", r)
	}
}

// A fresh verdict that fails its own validation is never returned as
// supported and never stored.
func TestFreshUnprovenVerdictIsSanitizedAndNotStored(t *testing.T) {
	tk := cacheTask()
	for name, bad := range map[string]Result{
		"no proof": {Verdict: domain.Verdict{PrecedentSupported: true, SupportingRuling: "N999999", CounterRuling: "N340000", Confidence: 0.97}},
		"excluded support": {Verdict: domain.Verdict{PrecedentSupported: true, SupportingRuling: "N328954", Confidence: 0.97},
			Proof: ProofFor(tk, "N328954")},
	} {
		bad = withDistinctOrigins(bad)
		t.Run(name, func(t *testing.T) {
			blobs := &memBlobs{m: map[string][]byte{}}
			c := NewCacheIn(&fixedChecker{make: func(Task) Result { return bad }}, blobs, "m")
			r, err := c.Verify(context.Background(), tk)
			if err != nil || r.Verdict.PrecedentSupported || r.Verdict.SupportingRuling != "" || r.Verdict.CounterRuling != "" || len(r.Dropped) == 0 {
				t.Errorf("unsafe supported verdict returned: %+v err=%v", r, err)
			}
			if len(blobs.m) != 0 {
				t.Error("unsafe fresh verdict was stored")
			}
		})
	}
}

// Verify produces the proof itself from direct retrievals only.
func TestVerifyRecordsProofFromDirectRetrievalsOnly(t *testing.T) {
	s := &scriptSearch{
		hits:  []Hit{{ID: "N350001", Date: d2025, Subject: "dryer", Origin: "Thailand"}, {ID: "N360000", Date: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)}},
		reads: map[string]string{"N350001": "[N350001 | 2025-03-01] x\nFACTS: cf N328954 and N999999.", "N370000": "[N370001 | 2025-03-01] mismatched header", "N380000": "no header"},
	}
	s.bypass = true
	res := verifyScript(t, s, task(), answerFor("N350001", ""),
		call("c1", "search_rulings", `{"query":"q"}`), call("c2", "read_ruling", `{"ruling":"N350001"}`),
		call("c3", "read_ruling", `{"ruling":"N370000"}`), call("c4", "read_ruling", `{"ruling":"N380000"}`))
	if res.Proof == nil || len(res.Proof.Eligible) != 1 || res.Proof.Eligible[0] != "N350001" || res.Proof.Epoch != ValidationEpoch ||
		res.Proof.Before != "2026-01-14" || len(res.Proof.Exclude) != 1 || res.Proof.Exclude[0] != "N328954" {
		t.Errorf("proof = %+v", res.Proof)
	}
	if cachedViolation(res, task()) != "" {
		t.Errorf("fresh valid verdict fails own validation: %s", cachedViolation(res, task()))
	}
}

// A bounded task needs the read header date; no unknown-date precedents.
func TestReadWithoutHeaderDateIsNotEligibleWhenBounded(t *testing.T) {
	s := &scriptSearch{reads: map[string]string{"N350001": "[N350001] FACTS: no date"}}
	res := verifyScript(t, s, task(), answerFor("N350001", ""), call("c1", "read_ruling", `{"ruling":"N350001"}`))
	if res.Verdict.PrecedentSupported {
		t.Errorf("undated read accepted: %+v", res.Verdict)
	}
	tk := task()
	tk.Before = time.Time{}
	res = verifyScript(t, s, tk, answerFor("N350001", ""), call("c1", "read_ruling", `{"ruling":"N350001"}`))
	if !res.Verdict.PrecedentSupported {
		t.Errorf("unbounded task rejected undated read: %+v", res.Verdict)
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

// supported=true with an empty supporting ruling is never supported, whether
// stored (legacy or with current proof) or fresh.
func TestSupportedWithoutSupportingRulingIsNotReplayedOrReturned(t *testing.T) {
	tk := cacheTask()
	empty := Result{Verdict: domain.Verdict{PrecedentSupported: true, Confidence: 0.97}}
	withProof := empty
	withProof.Proof = ProofFor(tk)
	for name, stored := range map[string]Result{"legacy": empty, "current proof": withProof} {
		stored = withDistinctOrigins(stored) // only the missing citation is under test
		t.Run("stored "+name, func(t *testing.T) {
			c, inner, _ := newFixedCache()
			seed(t, c, tk, stored)
			if _, ok, _ := c.Peek(context.Background(), tk); ok {
				t.Fatal("Peek replayed supported verdict with empty support")
			}
			if r, _ := c.Verify(context.Background(), tk); r.Cached || inner.calls != 1 {
				t.Errorf("replayed: %+v calls=%d", r, inner.calls)
			}
		})
		t.Run("fresh "+name, func(t *testing.T) {
			blobs := &memBlobs{m: map[string][]byte{}}
			c := NewCacheIn(&fixedChecker{make: func(Task) Result { return stored }}, blobs, "m")
			r, err := c.Verify(context.Background(), tk)
			if err != nil || r.Verdict.PrecedentSupported || r.Verdict.SupportingRuling != "" || len(r.Dropped) != 0 {
				t.Errorf("returned supported or invented a drop: %+v err=%v", r, err)
			}
			if len(blobs.m) != 0 {
				t.Error("stored")
			}
		})
	}
	// Controls: a legitimate unsupported verdict with no citations replays; proven support hits.
	c, inner, _ := newFixedCache()
	seed(t, c, tk, Result{Verdict: domain.Verdict{Confidence: 0.3}})
	if r, _ := c.Verify(context.Background(), tk); !r.Cached || inner.calls != 0 {
		t.Errorf("unsupported no-citation verdict missed: %+v", r)
	}
	tk2 := cacheTask()
	tk2.FactsB = "proven"
	seed(t, c, tk2, goodResult(tk2))
	if r, _ := c.Verify(context.Background(), tk2); !r.Cached || !r.Verdict.PrecedentSupported || inner.calls != 0 {
		t.Errorf("proven support missed: %+v", r)
	}
}
