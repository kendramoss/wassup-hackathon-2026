package verifier

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/blob"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"os"
	"slices"
	"time"
)

// Checker is anything that verifies a move (Verifier, Cache).
type Checker interface {
	Verify(ctx context.Context, t Task) (Result, error)
}

// Blobs stores cached verdicts by key (blob.Dir on disk, store.FirestoreBlobs
// shared between Cloud Run instances, D50).
type Blobs = blob.Store

// DirBlobs stores each blob as <dir>/<key>.json.
type DirBlobs = blob.Dir

// Cache stores verdicts so the same task gets the same verdict on every run.
// The model is called at temperature 0, but the agent's tool path still
// varies through the provider, which moved a heat-gun plan between 0.95 and
// 0.92 on consecutive runs (D31). Clear the store to re-verify.
type Cache struct {
	inner Checker
	blobs Blobs
	model string
}

// NewCache wraps inner with an on-disk cache in dir, keyed by the task, the
// verifier model and PromptVersion.
func NewCache(inner Checker, dir, model string) (*Cache, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("verifier: cache dir: %w", err)
	}
	return NewCacheIn(inner, DirBlobs{Dir: dir}, model), nil
}

// NewCacheIn wraps inner with a cache in blobs.
func NewCacheIn(inner Checker, blobs Blobs, model string) *Cache {
	if m, ok := inner.(*Majority); ok {
		model = fmt.Sprintf("%s×%d", model, m.n) // a single-run verdict is not reused for a majority
	}
	return &Cache{inner: inner, blobs: blobs, model: model}
}

// versioned is a Checker that reports its prompt version.
type versioned interface{ Version() string }

// checkerVersion is inner's prompt version: the Verifier's, through a
// Majority or a Panel, else verifier-v1. A Panel keeps its first checker's
// key on purpose (D83): turning the panel on must not re-run verdicts
// already stored, demo verdicts included (D56); it applies to new checks.
func checkerVersion(inner Checker) string {
	if m, ok := inner.(*Majority); ok {
		inner = m.inner
	}
	if p, ok := inner.(*Panel); ok {
		inner = p.first
	}
	if v, ok := inner.(versioned); ok {
		return v.Version()
	}
	return PromptVersion
}

func (c *Cache) key(t Task) string {
	ex := slices.Clone(t.Exclude)
	slices.Sort(ex)
	b, _ := json.Marshal(struct {
		Prompt, Model, FactsA, FactsB, Target, Before, KnownOriginA, KnownRulingA string
		Exclude                                                                   []string
	}{checkerVersion(c.inner), c.model, t.FactsA, t.FactsB, t.Target, t.Before.Format(time.DateOnly), t.KnownOriginA, t.KnownRulingA, ex})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// ValidationEpoch names the acceptance rules a cached verdict is re-checked
// against on lookup (cachedViolation). It is NOT part of the cache key and is
// not a prompt version: the prompts did not change, so keys and traces keep
// their verifier-vN names (a stored safe verdict still hits). It only names
// the slot where a re-run replaces an unsafe legacy verdict, because the blob
// stores never overwrite a key. Bump it when cachedViolation gains a rule that
// older repaired entries might break (#297 adds origin consistency).
const ValidationEpoch = "val1"

// repairKey is where a verdict that replaced an unsafe legacy one is stored.
func repairKey(key string) string { return key + "-" + ValidationEpoch }

// cachedViolation reports why a stored verdict must not be replayed for a
// task whose exclusions are exclude, or "" when it is safe. This is the
// shared cache-compatibility contract (#295, reused by #297): add new
// acceptance rules here and every lookup, Peek and Verify applies them to
// stored payloads. Rules, all independent of what the verdict says about
// origin:
//   - the supporting or counter ruling must not be in the task's Exclude;
//   - when the verdict has a trail, its supporting ruling must be an identity
//     in it (search hit or ruling read), so a ruling only mentioned in text
//     is not replayed as supported.
//
// Verdicts stored before D80 have no trail; they cannot be checked against
// one and are kept (only the exclusion rule applies), so seeded demo verdicts
// keep replaying (D56). A verdict that cites nothing has nothing to check. A
// counter-ruling is only checked against Exclude: a panel can take it from
// another run's trail.
func cachedViolation(v domain.Verdict, exclude []string) string {
	for _, id := range []string{v.SupportingRuling, v.CounterRuling} {
		if id != "" && slices.Contains(exclude, id) {
			return "cites excluded ruling " + id
		}
	}
	if v.SupportingRuling != "" && len(v.Trail) > 0 {
		inTrail := false
		for _, st := range v.Trail {
			if slices.Contains(st.Rulings, v.SupportingRuling) {
				inTrail = true
			}
		}
		if !inTrail {
			return "supporting ruling " + v.SupportingRuling + " not in the retrieved trail"
		}
	}
	return ""
}

// Verify returns the stored verdict for t, or runs inner and stores it.
// A cached result reports no model calls, tool steps or usage, and Cached.
// A failed cache read is an error (re-running could give a different
// verdict); a failed write only loses the cache entry. A stored verdict that
// fails cachedViolation for t counts as absent.
func (c *Cache) Verify(ctx context.Context, t Task) (Result, error) {
	key := c.key(t)
	r, ok, stale, err := c.lookup(ctx, key, t.Exclude)
	if err != nil {
		return Result{}, err
	}
	if ok {
		return r, nil
	}
	r, err = c.inner.Verify(ctx, t)
	if err != nil {
		return r, err
	}
	if r.ToolErrors > 0 { // reached without some evidence: let the next run verify again
		return r, nil
	}
	// A fresh verdict must itself pass the stored-payload rules, or the next
	// lookup would throw it away; it is returned either way.
	if cachedViolation(r.Verdict, t.Exclude) != "" {
		return r, nil
	}
	if b, err := json.Marshal(r); err == nil {
		if stale {
			key = repairKey(key)
		}
		_ = c.blobs.Put(ctx, key, b) // best effort
	}
	return r, nil
}

// Peek returns the stored verdict for t without running the agent; ok is
// false when none is stored or the stored one is unsafe for t. The plan-search
// estimate uses it read-only (D87).
func (c *Cache) Peek(ctx context.Context, t Task) (Result, bool, error) {
	r, ok, _, err := c.lookup(ctx, c.key(t), t.Exclude)
	return r, ok, err
}

// lookup reads the verdict stored under key. An unsafe one (cachedViolation)
// is skipped: stale reports that, and the replacement slot is read instead.
func (c *Cache) lookup(ctx context.Context, key string, exclude []string) (r Result, ok, stale bool, err error) {
	for _, k := range []string{key, repairKey(key)} {
		b, found, err := c.blobs.Get(ctx, k)
		if err != nil {
			return Result{}, false, false, fmt.Errorf("verifier: read cache: %w", err)
		}
		var got Result
		if !found || json.Unmarshal(b, &got) != nil {
			if k == key {
				return Result{}, false, false, nil
			}
			return Result{}, false, true, nil
		}
		if cachedViolation(got.Verdict, exclude) != "" {
			stale = true
			continue
		}
		got.Steps, got.ModelCalls, got.Cached = nil, 0, true
		got.Usage.InputTokens, got.Usage.OutputTokens, got.Usage.TotalTokens = 0, 0, 0
		return got, true, stale, nil
	}
	return Result{}, false, true, nil
}
