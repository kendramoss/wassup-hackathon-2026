package verifier

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// Checker is anything that verifies a move (Verifier, Cache).
type Checker interface {
	Verify(ctx context.Context, t Task) (Result, error)
}

// Cache stores verdicts on disk so the same task gets the same verdict on
// every run. The model is called at temperature 0, but the agent's tool path
// still varies through the provider, which moved a heat-gun plan between 0.95
// and 0.92 on consecutive runs (D31). Delete the directory to re-verify.
type Cache struct {
	inner Checker
	dir   string
	model string
}

// NewCache wraps inner with an on-disk cache in dir, keyed by the task, the
// verifier model and PromptVersion.
func NewCache(inner Checker, dir, model string) (*Cache, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("verifier: cache dir: %w", err)
	}
	return &Cache{inner: inner, dir: dir, model: model}, nil
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
func (c *Cache) Verify(ctx context.Context, t Task) (Result, error) {
	path := filepath.Join(c.dir, c.key(t)+".json")
	if b, err := os.ReadFile(path); err == nil {
		var r Result
		if json.Unmarshal(b, &r) == nil {
			r.Steps, r.ModelCalls, r.Cached = nil, 0, true
			r.Usage.InputTokens, r.Usage.OutputTokens, r.Usage.TotalTokens = 0, 0, 0
			return r, nil
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Result{}, fmt.Errorf("verifier: read cache: %w", err)
	}
	r, err := c.inner.Verify(ctx, t)
	if err != nil {
		return r, err
	}
	b, err := json.Marshal(r)
	if err != nil {
		return r, nil
	}
	tmp, err := os.CreateTemp(c.dir, ".tmp-*")
	if err != nil {
		return r, nil // caching is best effort
	}
	if _, err := tmp.Write(b); err == nil && tmp.Close() == nil {
		_ = os.Rename(tmp.Name(), path)
	}
	_ = os.Remove(tmp.Name())
	return r, nil
}
