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
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
)

// Cache stores engine predictions so the same facts, date and exclusions get
// the same answer without another model call (D54). Plan searches and the
// demo replay then depend on the model only the first time, which matters
// when the provider is short of capacity (429 and 504 from Vertex), and a
// replayed case gives exactly the recorded answer. The key includes the
// model, prompt version, K and voting, so a prompt change re-predicts, and
// the revision of the evidence the retriever searches, so an added, removed
// or corrected precedent re-predicts instead of replaying a stale citation.
// A retriever that cannot name its evidence revision (no Revision method, or
// an empty one) makes the cache a pass-through: nothing is read or written,
// because any fixed marker would reuse answers across different evidence.
// An engine with no retriever (or K = 0) has no evidence and keeps caching.
type Cache struct {
	inner *Engine
	blobs blob.Store
}

// NewCache wraps e with a prediction cache in blobs.
func NewCache(e *Engine, blobs blob.Store) *Cache { return &Cache{inner: e, blobs: blobs} }

// evidence returns the evidence revision for the key and whether the cache
// may be used at all.
func (c *Cache) evidence() (rev string, ok bool) {
	if c.inner.retr == nil || c.inner.opts.K <= 0 {
		return "", true
	}
	rev = retrieval.RevisionOf(c.inner.retr)
	return rev, rev != ""
}

func (c *Cache) key(model string, k domain.Case) string {
	rev, _ := c.evidence()
	ex := slices.Clone(k.Exclude)
	slices.Sort(ex)
	b, _ := json.Marshal(struct {
		Model, Version, Facts, Before string
		K, Votes                      int
		VoteBelow                     float64
		Exclude                       []string
		Evidence                      string `json:",omitempty"` // empty only without a retriever
		RuleText                      string `json:",omitempty"` // empty without Options.RuleText (D550)
	}{model, c.inner.promptVersion(), k.Facts, k.Date.Format(time.DateOnly), c.inner.opts.K, c.inner.opts.Votes, c.inner.opts.VoteBelow, ex, rev, c.inner.ruleText(k)})
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
	if _, ok := c.evidence(); !cacheable(k) || !ok {
		return c.inner.Predict(ctx, k)
	}
	if r, ok, err := c.Peek(ctx, k); err != nil {
		return Result{}, err
	} else if ok {
		return r, nil
	}
	res, err := c.inner.Predict(ctx, k)
	if err != nil {
		return res, err
	}
	// Store under the configured model that answered (the primary alias, or
	// the fallback model), never the provider's resolved version: Peek reads
	// the primary alias, so an alias answered as "<alias>-001" must land there.
	answered := res.Prediction.Trace.Identity()
	if answered == "" {
		answered = c.inner.opts.Model
	}
	if b, err := json.Marshal(cached{Prediction: res.Prediction, Flip: res.Flip, Votes: res.Votes, VoteCounts: res.VoteCounts}); err == nil {
		_ = c.blobs.Put(ctx, c.key(answered, k), b) // best effort; fallback model id ≠ primary key
	}
	return res, nil
}

func cacheable(k domain.Case) bool {
	return !domain.ValidRulingID(k.ID) && k.Facts != "" && !k.Date.IsZero()
}

// Peek returns the stored prediction for k without calling the model; ok is
// false when none is stored or k is not cached. It reads the primary
// model's key: a fallback answer is stored under the model that answered,
// so it does not satisfy this lookup and a later primary success is not
// blocked (D56). The plan-search estimate uses it read-only (D87).
func (c *Cache) Peek(ctx context.Context, k domain.Case) (Result, bool, error) {
	if _, ok := c.evidence(); !cacheable(k) || !ok {
		return Result{}, false, nil
	}
	b, ok, err := c.blobs.Get(ctx, c.key(c.inner.opts.Model, k))
	if err != nil {
		return Result{}, false, fmt.Errorf("engine: read cache: %w", err)
	}
	var v cached
	if !ok || json.Unmarshal(b, &v) != nil || v.Prediction.Origin == "" {
		return Result{}, false, nil
	}
	p := v.Prediction
	// A stored answer whose precedents include a ruling CBP revoked or
	// modified before the case date (D370) was made from evidence the
	// precedent filter no longer shows (D320): predict again.
	for _, id := range p.Precedents {
		if _, superseded := domain.SupersededAsOf(id, k.Date); superseded {
			return Result{}, false, nil
		}
	}
	if p.Trace.RequestedModel == "" {
		// Stored before RequestedModel existed: this key is the primary's.
		p.Trace.RequestedModel = c.inner.opts.Model
	}
	p.Trace.InputTokens, p.Trace.OutputTokens, p.Trace.LatencyMS, p.Trace.Cached = 0, 0, 0, true
	return Result{Prediction: p, Flip: v.Flip, Votes: v.Votes, VoteCounts: v.VoteCounts}, true, nil
}
