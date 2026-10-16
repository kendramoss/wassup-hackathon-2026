package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/blob"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// Cache stores engine predictions so the same facts, date and exclusions get
// the same answer without another model call (D54). Plan searches and the
// demo replay then depend on the model only the first time, which matters
// when the provider is short of capacity (429 and 504 from Vertex), and a
// replayed case gives exactly the recorded answer. The key includes the
// model, prompt version, K and voting, so a prompt change re-predicts.
type Cache struct {
	inner *Engine
	blobs blob.Store
}

// NewCache wraps e with a prediction cache in blobs.
func NewCache(e *Engine, blobs blob.Store) *Cache { return &Cache{inner: e, blobs: blobs} }

func (c *Cache) key(model string, k domain.Case) string {
	ex := slices.Clone(k.Exclude)
	slices.Sort(ex)
	b, _ := json.Marshal(struct {
		Model, Version, Facts, Before string
		K, Votes                      int
		VoteBelow                     float64
		Exclude                       []string
	}{model, c.inner.promptVersion(), k.Facts, k.Date.Format(time.DateOnly), c.inner.opts.K, c.inner.opts.Votes, c.inner.opts.VoteBelow, ex})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

type cached struct {
	Prediction domain.Prediction `json:"prediction"`
	Flip       string            `json:"flip"`
	Votes      int               `json:"votes"`
	VoteCounts map[string]int    `json:"vote_counts"`
}

// Predict returns the stored prediction for k, or predicts and stores it. A
// cached prediction reports no tokens, latency or usage, and Trace.Cached.
// A case whose ID is a ruling number is not cached: its precedents exclude
// that ruling, which the key does not cover (eval rows only).
func (c *Cache) Predict(ctx context.Context, k domain.Case) (Result, error) {
	if domain.ValidRulingID(k.ID) || k.Facts == "" || k.Date.IsZero() {
		return c.inner.Predict(ctx, k)
	}
	// Look up the primary model's key. A fallback answer is stored under the
	// model that answered, so it does not satisfy this lookup and a later
	// primary success is not blocked (D56).
	key := c.key(c.inner.opts.Model, k)
	if b, ok, err := c.blobs.Get(ctx, key); err != nil {
		return Result{}, fmt.Errorf("engine: read cache: %w", err)
	} else if ok {
		var v cached
		if json.Unmarshal(b, &v) == nil && v.Prediction.Origin != "" {
			p := v.Prediction
			p.Trace.InputTokens, p.Trace.OutputTokens, p.Trace.LatencyMS, p.Trace.Cached = 0, 0, 0, true
			return Result{Prediction: p, Flip: v.Flip, Votes: v.Votes, VoteCounts: v.VoteCounts}, nil
		}
	}
	res, err := c.inner.Predict(ctx, k)
	if err != nil {
		return res, err
	}
	answered := res.Prediction.Trace.ModelID
	if answered == "" {
		answered = c.inner.opts.Model
	}
	if b, err := json.Marshal(cached{Prediction: res.Prediction, Flip: res.Flip, Votes: res.Votes, VoteCounts: res.VoteCounts}); err == nil {
		_ = c.blobs.Put(ctx, c.key(answered, k), b) // best effort; fallback model id ≠ primary key
	}
	return res, nil
}
