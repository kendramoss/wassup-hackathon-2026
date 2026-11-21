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

// ValidationEpoch names the acceptance rules a cached verdict is validated
// against (cachedViolation). It is a validation namespace only: it is NOT part
// of the cache key and NOT a prompt version (prompts did not change; keys and
// traces keep their verifier-vN names). Fresh verdicts store a Proof stamped
// with it; a verdict that cites a ruling is replayed only with a Proof of the
// current epoch. Blob stores never overwrite a key, so a fresh verdict that
// replaces an unusable stored one goes to repairKey. Bump it when
// cachedViolation gains a rule (val2: origin consistency, #297).
const ValidationEpoch = "val2"

// repairKey is where a verdict that replaced an unusable stored one is kept.
func repairKey(key string) string { return key + "-" + ValidationEpoch }

// cachedViolation reports why a verdict may not be replayed (or returned) for
// task t, or "" when it is safe. This is the shared cache-compatibility
// contract (#295, reused by #297). PrecedentSupported requires a non-empty
// supporting ruling. A verdict that cites no ruling and claims no support has
// nothing to prove. One that cites a supporting or counter ruling needs current proof:
//   - r.Proof present, of the current ValidationEpoch, recorded for the same
//     Before date and the same exclusion set as t;
//   - every cited ruling is in Proof.Eligible (validated direct retrievals of
//     that run), not in t.Exclude, and not revoked or modified by CBP on or
//     before t.Before (D320; checked first, so an old verdict citing a
//     ruling revoked since is a miss).
//
// The display Trail is never consulted: it lists rejected, future and
// mismatched reads. Entries without proof, including legacy and seeded demo
// verdicts, are a miss.
func cachedViolation(r Result, t Task) string {
	v := r.Verdict
	cited := []string{}
	for _, id := range []string{v.SupportingRuling, v.CounterRuling} {
		if id != "" {
			cited = append(cited, id)
		}
	}
	for _, id := range cited {
		if slices.Contains(t.Exclude, id) {
			return "cites excluded ruling " + id
		}
		if s, ok := domain.SupersededAsOf(id, t.Before); ok {
			return "cites ruling " + id + " " + s.Note()
		}
	}
	if v.PrecedentSupported && v.SupportingRuling == "" {
		return "supported without a supporting ruling" // support needs a validated direct citation
	}
	if v.PrecedentSupported {
		// val2: the origins stored with the verdict must show a real change.
		// A stored verdict without them (legacy) is not checkable: a miss.
		if !r.ChangesOrigin {
			return "supported without a changes_origin answer"
		}
		if why := originViolation(r.OriginA, r.OriginB, t); why != "" {
			return why
		}
	}
	if len(cited) == 0 {
		return ""
	}
	p := r.Proof
	switch {
	case p == nil:
		return "no validation proof"
	case p.Epoch != ValidationEpoch:
		return "proof from epoch " + p.Epoch
	case p.Before != t.Before.Format(time.DateOnly):
		return "proof for a different date bound"
	case !slices.Equal(p.Exclude, sortedCopy(t.Exclude)):
		return "proof for different exclusions"
	}
	for _, id := range cited {
		if !slices.Contains(p.Eligible, id) {
			return "cited ruling " + id + " not validated"
		}
	}
	return ""
}

// blocked reports whether task t may never cite ruling id: it is excluded,
// or CBP revoked or modified it on or before t.Before (D320).
func blocked(t Task, id string) bool {
	return slices.Contains(t.Exclude, id) || superseded(id, t.Before)
}

// sanitize removes citations that fail cachedViolation from a fresh result so
// an unsafe supported verdict is never returned; it reports whether it changed
// anything. Dropped names what was removed.
func sanitize(r Result, t Task) (Result, bool) {
	changed := false
	for cachedViolation(r, t) != "" {
		changed = true
		v := r.Verdict
		switch {
		case v.PrecedentSupported && v.SupportingRuling == "":
			// Nothing to drop and nothing invented: the flag is simply not
			// backed by a citation, so it is cleared.
			r.Verdict.PrecedentSupported = false
		case v.PrecedentSupported && (!r.ChangesOrigin || originViolation(r.OriginA, r.OriginB, t) != ""):
			// The ruling may be real, but the claimed change is not (equivalent
			// origins, unknown origin, or contradicting the known baseline):
			// clear the flag only. Citations are kept as they were validated;
			// nothing is dropped or invented.
			r.Verdict.PrecedentSupported = false
		case v.CounterRuling != "" && (r.Proof == nil || !slices.Contains(r.Proof.Eligible, v.CounterRuling) || blocked(t, v.CounterRuling)):
			r.Dropped = append(r.Dropped, v.CounterRuling)
			r.Verdict.CounterRuling = ""
		case v.SupportingRuling != "" && (r.Proof == nil || !slices.Contains(r.Proof.Eligible, v.SupportingRuling) || blocked(t, v.SupportingRuling)):
			r.Dropped = append(r.Dropped, v.SupportingRuling)
			r.Verdict.SupportingRuling, r.Verdict.PrecedentSupported = "", false
		default: // proof itself is for another task or epoch
			if v.CounterRuling != "" {
				r.Dropped = append(r.Dropped, v.CounterRuling)
				r.Verdict.CounterRuling = ""
			}
			if v.SupportingRuling != "" {
				r.Dropped = append(r.Dropped, v.SupportingRuling)
				r.Verdict.SupportingRuling, r.Verdict.PrecedentSupported = "", false
			}
		}
	}
	return r, changed
}

// Verify returns the stored verdict for t, or runs inner and stores it.
// A cached result reports no model calls, tool steps or usage, and Cached.
// A failed cache read is an error (re-running could give a different
// verdict); a failed write only loses the cache entry. A stored verdict that
// fails cachedViolation for t counts as absent. A fresh verdict that fails it
// is sanitized (unproven citations removed, no longer supported) and not
// stored.
func (c *Cache) Verify(ctx context.Context, t Task) (Result, error) {
	key := c.key(t)
	r, ok, stale, err := c.lookup(ctx, key, t)
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
	if clean, changed := sanitize(r, t); changed {
		return clean, nil
	}
	if r.ToolErrors > 0 { // reached without some evidence: let the next run verify again
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
	r, ok, _, err := c.lookup(ctx, c.key(t), t)
	return r, ok, err
}

// lookup reads the verdict stored under key, then under its repair slot. An
// unusable or unproven one is skipped; stale reports that, so a fresh verdict
// is written to the repair slot (stores never overwrite a key).
func (c *Cache) lookup(ctx context.Context, key string, t Task) (r Result, ok, stale bool, err error) {
	for i, k := range []string{key, repairKey(key)} {
		b, found, err := c.blobs.Get(ctx, k)
		if err != nil {
			return Result{}, false, false, fmt.Errorf("verifier: read cache: %w", err)
		}
		if !found {
			return Result{}, false, stale || i == 1, nil
		}
		var got Result
		if json.Unmarshal(b, &got) != nil || cachedViolation(got, t) != "" {
			stale = true
			continue
		}
		got.Steps, got.ModelCalls, got.Cached = nil, 0, true
		got.Usage.InputTokens, got.Usage.OutputTokens, got.Usage.TotalTokens = 0, 0, 0
		return got, true, stale, nil
	}
	return Result{}, false, true, nil
}
