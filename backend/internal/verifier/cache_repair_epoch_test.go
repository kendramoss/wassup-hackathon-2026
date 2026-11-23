package verifier

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// createOnlyDir mirrors store.FirestoreBlobs.Put: Create, AlreadyExists is
// success (first write wins), persisted on disk so a "restart" shares it.
type createOnlyDir struct{ dir string }

func (c createOnlyDir) Get(_ context.Context, k string) ([]byte, bool, error) {
	b, err := os.ReadFile(filepath.Join(c.dir, k+".json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	return b, err == nil, err
}

func (c createOnlyDir) Put(_ context.Context, k string, v []byte) error {
	f, err := os.OpenFile(filepath.Join(c.dir, k+".json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(v)
	return err
}

func (c createOnlyDir) slots(t *testing.T) int {
	t.Helper()
	ents, err := os.ReadDir(c.dir)
	if err != nil {
		t.Fatal(err)
	}
	return len(ents)
}

// citing returns a fresh, proven verdict that cites id.
type citing struct {
	id    string
	calls int
}

func (c *citing) Verify(_ context.Context, t Task) (Result, error) {
	c.calls++
	return Result{OriginA: "China", OriginB: "Thailand", ChangesOrigin: true,
		Verdict: domain.Verdict{PrecedentSupported: true, Confidence: 0.95, SupportingRuling: c.id},
		Proof:   ProofFor(t, c.id)}, nil
}

// cacheOnData builds a Cache that validates and keys repairs with one data
// version: edges maps a ruling to its supersession date.
func cacheOnData(inner Checker, blobs Blobs, fp string, edges map[string]string) *Cache {
	c := NewCacheIn(inner, blobs, "m")
	c.fingerprint = fp
	c.asOf = func(id string, at time.Time) (domain.Supersession, bool) {
		d, ok := edges[id]
		if !ok {
			return domain.Supersession{}, false
		}
		if when, _ := time.Parse(time.DateOnly, d); !at.IsZero() && !when.Before(at) {
			return domain.Supersession{}, false
		}
		return domain.Supersession{Ruling: id, By: "H999999", Date: d, Kind: "revoked"}, true
	}
	return c
}

// The #721 lifecycle over a create-only store: a legacy base slot, a repair
// written under data M1, then M2 supersedes the ruling that repair cites. The
// task is re-verified once under M2, then replays (also after a restart).
func TestCacheRepairSurvivesASecondSupersessionEdge(t *testing.T) {
	ctx := context.Background()
	tk := Task{FactsA: "a", FactsB: "b", Target: "Thailand", Before: time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)}
	store := createOnlyDir{t.TempDir()}
	m1 := map[string]string{}
	m2 := map[string]string{"N350001": "2025-11-24"}

	// Pre-D111 base slot: cites a ruling and carries no Proof.
	base := cacheOnData(&citing{id: "x"}, store, "fp-m1", m1)
	legacy, _ := json.Marshal(Result{OriginA: "China", OriginB: "Thailand", ChangesOrigin: true,
		Verdict: domain.Verdict{PrecedentSupported: true, Confidence: 0.95, SupportingRuling: "N322364"}})
	if err := store.Put(ctx, base.key(tk), legacy); err != nil {
		t.Fatal(err)
	}

	// Phase A (M1): one re-verification, then hits.
	a := &citing{id: "N350001"}
	ca := cacheOnData(a, store, "fp-m1", m1)
	for i, wantCached := range []bool{false, true} {
		r, err := ca.Verify(ctx, tk)
		if err != nil || r.Cached != wantCached || r.Verdict.SupportingRuling != "N350001" {
			t.Fatalf("A call %d: cached=%v cites=%s err=%v", i+1, r.Cached, r.Verdict.SupportingRuling, err)
		}
	}
	if a.calls != 1 {
		t.Fatalf("A inner calls = %d, want 1", a.calls)
	}

	// Phase B (M2, a fresh instance as after a deploy): the stored N350001 is
	// now superseded for this task, so the verdict is never served.
	b := &citing{id: "N340000"}
	for i, wantCached := range []bool{false, true, true} {
		cb := cacheOnData(b, store, "fp-m2", m2) // each call is a restart over the same store
		r, err := cb.Verify(ctx, tk)
		if err != nil || r.Cached != wantCached || r.Verdict.SupportingRuling != "N340000" {
			t.Fatalf("B call %d: cached=%v cites=%s inner=%d err=%v", i+1, r.Cached, r.Verdict.SupportingRuling, b.calls, err)
		}
	}
	if b.calls != 1 {
		t.Fatalf("B inner calls = %d, want 1 (re-verified once under M2)", b.calls)
	}
	if n := store.slots(t); n != 3 {
		t.Errorf("store holds %d slots, want base + old repair + current repair", n)
	}
}

// Same data, a new instance: a repair written under the same fingerprint is
// replayed; a changed fingerprint (another enforced edge) is a fresh epoch.
func TestCacheRepairKeyFollowsTheDataFingerprint(t *testing.T) {
	ctx := context.Background()
	tk := Task{FactsA: "a", FactsB: "b", Target: "Thailand", Before: time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)}
	store := createOnlyDir{t.TempDir()}
	seedBase := cacheOnData(&citing{id: "x"}, store, "fp-1", nil)
	legacy, _ := json.Marshal(Result{Verdict: domain.Verdict{SupportingRuling: "N322364"}})
	_ = store.Put(ctx, seedBase.key(tk), legacy)

	in := &citing{id: "N350001"}
	if _, err := cacheOnData(in, store, "fp-1", nil).Verify(ctx, tk); err != nil {
		t.Fatal(err)
	}
	if r, _ := cacheOnData(in, store, "fp-1", nil).Verify(ctx, tk); !r.Cached || in.calls != 1 {
		t.Fatalf("same fingerprint must replay: cached=%v calls=%d", r.Cached, in.calls)
	}
	if k1, k2 := cacheOnData(in, store, "fp-1", nil).repairSlot("k"), cacheOnData(in, store, "fp-2", nil).repairSlot("k"); k1 == k2 || k1 == repairKey("k") {
		t.Errorf("repair slots must differ per fingerprint and from the legacy slot: %q %q", k1, k2)
	}
}

// A legacy "-val2" repair slot written before fingerprints existed is still
// replayed when it passes every current check, and rejected when it does not.
func TestCacheLegacyRepairSlotStillChecked(t *testing.T) {
	ctx := context.Background()
	tk := Task{FactsA: "a", FactsB: "b", Target: "Thailand", Before: time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)}
	for _, tc := range []struct {
		name   string
		edges  map[string]string
		cached bool
	}{{"still valid", nil, true}, {"superseded since", map[string]string{"N350001": "2025-11-24"}, false}} {
		t.Run(tc.name, func(t *testing.T) {
			store := createOnlyDir{t.TempDir()}
			in := &citing{id: "N340000"}
			c := cacheOnData(in, store, "fp-1", tc.edges)
			legacy, _ := json.Marshal(Result{Verdict: domain.Verdict{SupportingRuling: "N322364"}})
			_ = store.Put(ctx, c.key(tk), legacy)
			old, _ := json.Marshal(in2(tk, "N350001"))
			_ = store.Put(ctx, repairKey(c.key(tk)), old)
			r, err := c.Verify(ctx, tk)
			if err != nil || r.Cached != tc.cached {
				t.Fatalf("cached=%v want %v err=%v", r.Cached, tc.cached, err)
			}
			if tc.cached && r.Verdict.SupportingRuling != "N350001" || !tc.cached && r.Verdict.SupportingRuling != "N340000" {
				t.Errorf("served %s", r.Verdict.SupportingRuling)
			}
		})
	}
}

func in2(tk Task, id string) Result {
	r, _ := (&citing{id: id}).Verify(context.Background(), tk)
	return r
}

// A production cache captures the embedded data's fingerprint at construction.
func TestNewCacheInCapturesTheEmbeddedFingerprint(t *testing.T) {
	c := NewCacheIn(&citing{}, &memBlobs{m: map[string][]byte{}}, "m")
	if c.fingerprint == "" || c.fingerprint != domain.EmbeddedSupersessionFingerprint() {
		t.Errorf("fingerprint = %q", c.fingerprint)
	}
	if got := c.repairSlot("k"); got == repairKey("k") || got[:len(repairKey("k"))] != repairKey("k") {
		t.Errorf("repair slot %q", got)
	}
}
