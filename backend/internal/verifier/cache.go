package verifier

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/blob"
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

func (c *Cache) key(t Task) string {
	ex := slices.Clone(t.Exclude)
	slices.Sort(ex)
	b, _ := json.Marshal(struct {
		Prompt, Model, FactsA, FactsB, Target, Before, KnownOriginA, KnownRulingA string
		Exclude                                                                   []string
	}{PromptVersion, c.model, t.FactsA, t.FactsB, t.Target, t.Before.Format(time.DateOnly), t.KnownOriginA, t.KnownRulingA, ex})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Verify returns the stored verdict for t, or runs inner and stores it.
// A cached result reports no model calls, tool steps or usage, and Cached.
// A failed cache read is an error (re-running could give a different
// verdict); a failed write only loses the cache entry.
func (c *Cache) Verify(ctx context.Context, t Task) (Result, error) {
	key := c.key(t)
	if b, ok, err := c.blobs.Get(ctx, key); err != nil {
		return Result{}, fmt.Errorf("verifier: read cache: %w", err)
	} else if ok {
		var r Result
		if json.Unmarshal(b, &r) == nil {
			r.Steps, r.ModelCalls, r.Cached = nil, 0, true
			r.Usage.InputTokens, r.Usage.OutputTokens, r.Usage.TotalTokens = 0, 0, 0
			return r, nil
		}
	}
	r, err := c.inner.Verify(ctx, t)
	if err != nil {
		return r, err
	}
	if b, err := json.Marshal(r); err == nil {
		_ = c.blobs.Put(ctx, key, b) // best effort
	}
	return r, nil
}
