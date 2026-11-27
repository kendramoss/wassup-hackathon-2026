package blob

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
)

// Case-linked caches (#875, CACHE_PURGE). Cache entries are keyed by a hash of
// content, not by case, so two cases with the same facts share one entry. To
// delete what a case derived without deleting what another case still uses,
// each entry records which cases read or wrote it. A case's entries are
// removed only when no other case is left in the set.

type ownerKey struct{}

var casePattern = regexp.MustCompile(`^case_[0-9a-f]{12}$`)

// WithOwner marks ctx as working for a case: a Tracked store records the
// case against every entry it reads or writes under that ctx.
func WithOwner(ctx context.Context, caseID string) context.Context {
	if !casePattern.MatchString(caseID) {
		return ctx
	}
	return context.WithValue(ctx, ownerKey{}, caseID)
}

// OwnerOf returns the case ctx works for, if any.
func OwnerOf(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(ownerKey{}).(string)
	return id, ok && id != ""
}

// Names of the stores a Ref can point into.
const (
	StoreVerdicts = "verdicts" // verifier verdicts, near-miss advice, supplier documents
	StoreEngine   = "engine"   // engine predictions
)

// Ref names one entry of one named store.
type Ref struct {
	Store string `json:"store" firestore:"store"`
	Key   string `json:"key" firestore:"key"`
}

// ID is a stable, document-safe name for the entry.
func (r Ref) ID() string {
	h := sha256.Sum256([]byte(r.Store + "\x00" + r.Key))
	return hex.EncodeToString(h[:])
}

// Refs records which cases use which entries.
type Refs interface {
	// Add records that caseID uses the entry.
	Add(ctx context.Context, r Ref, caseID string) error
	// Release removes caseID from every entry that lists it and returns the
	// entries no case uses any more (they are gone from the index) and how
	// many entries other cases still use.
	Release(ctx context.Context, caseID string) (orphaned []Ref, kept int, err error)
	// InUse reports whether some case (other than the retry marker) is listed
	// for the entry.
	InUse(ctx context.Context, r Ref) (bool, error)
	// Cases lists every case that some entry still names.
	Cases(ctx context.Context) ([]string, error)
}

// Tracked wraps a Store so entries read or written for a case (see
// WithOwner) are linked to it. The link is written before the entry, so an
// entry never exists without one; if the link cannot be written the write is
// refused and a read is treated as a miss, never served untracked.
type Tracked struct {
	inner Store
	name  string
	refs  Refs
}

// Track wraps inner as the store called name. Nil refs: inner unchanged.
func Track(inner Store, name string, refs Refs) Store {
	if inner == nil || refs == nil {
		return inner
	}
	return &Tracked{inner: inner, name: name, refs: refs}
}

// Get implements Store.
func (t *Tracked) Get(ctx context.Context, key string) ([]byte, bool, error) {
	b, ok, err := t.inner.Get(ctx, key)
	if err != nil || !ok {
		return b, ok, err
	}
	if id, owned := OwnerOf(ctx); owned {
		if err := t.refs.Add(ctx, Ref{Store: t.name, Key: key}, id); err != nil {
			return nil, false, nil
		}
	}
	return b, true, nil
}

// Put implements Store.
func (t *Tracked) Put(ctx context.Context, key string, b []byte) error {
	if id, owned := OwnerOf(ctx); owned {
		if err := t.refs.Add(ctx, Ref{Store: t.name, Key: key}, id); err != nil {
			return err
		}
	}
	return t.inner.Put(ctx, key, b)
}

// Delete implements Deleter when the wrapped store does.
func (t *Tracked) Delete(ctx context.Context, key string) error {
	d, ok := t.inner.(Deleter)
	if !ok {
		return ErrNoDelete
	}
	return d.Delete(ctx, key)
}

// ErrNoDelete means a store cannot remove entries.
var ErrNoDelete = errors.New("blob: this store cannot delete")

// PurgeReport is what purging one case's entries did.
type PurgeReport struct {
	// Deleted entries were used by no other case.
	Deleted int `json:"deleted"`
	// Shared entries are still used by another case and were kept.
	Shared int `json:"shared"`
	// Failed entries could not be deleted (no Deleter, or an error); they
	// are no longer in the index.
	Failed int `json:"failed"`
}

// RetryOwner stands in for a case on entries that could not be deleted, so
// the next sweep finds them again.
const RetryOwner = "purge-retry"

// Add folds o into r.
func (r *PurgeReport) Add(o PurgeReport) {
	r.Deleted += o.Deleted
	r.Shared += o.Shared
	r.Failed += o.Failed
}

// Purger removes the entries a case alone used.
type Purger struct {
	Refs   Refs
	Stores map[string]Store // by Ref.Store
}

// PurgeCase releases caseID and deletes the entries nobody else uses. extra
// names entries known from the case itself (for example a supplier document
// stored before links existed); they are deleted unless the index lists
// another case for them.
func (p Purger) PurgeCase(ctx context.Context, caseID string, extra []Ref) (PurgeReport, error) {
	var rep PurgeReport
	orphaned, kept, err := p.Refs.Release(ctx, caseID)
	if err != nil {
		return rep, err
	}
	rep.Shared = kept
	seen := map[string]bool{}
	for _, r := range orphaned {
		seen[r.ID()] = true
	}
	for _, r := range extra {
		if seen[r.ID()] {
			continue
		}
		seen[r.ID()] = true
		used, err := p.Refs.InUse(ctx, r)
		if err != nil {
			return rep, err
		}
		if used {
			rep.Shared++
			continue
		}
		orphaned = append(orphaned, r)
	}
	var firstErr error
	for _, r := range orphaned {
		d, ok := p.Stores[r.Store].(Deleter)
		if !ok {
			rep.Failed++
			_ = p.Refs.Add(ctx, r, RetryOwner)
			continue
		}
		if err := d.Delete(ctx, r.Key); err != nil {
			rep.Failed++
			_ = p.Refs.Add(ctx, r, RetryOwner) // keep it findable: a later sweep tries again
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		rep.Deleted++
	}
	return rep, firstErr
}

// MemoryRefs is an in-process Refs. With a path it persists to one JSON file
// for a single local process.
type MemoryRefs struct {
	mu   sync.Mutex
	m    map[string]*memRef
	path string
}

type memRef struct {
	Ref   Ref      `json:"ref"`
	Cases []string `json:"cases"`
}

// NewMemoryRefs returns an empty in-process index.
func NewMemoryRefs() *MemoryRefs { return &MemoryRefs{m: map[string]*memRef{}} }

// NewFileRefs opens (or starts) dir/cache-refs.json.
func NewFileRefs(dir string) (*MemoryRefs, error) {
	r := NewMemoryRefs()
	r.path = filepath.Join(dir, "cache-refs.json")
	b, err := os.ReadFile(r.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return r, nil
	case err != nil:
		return nil, err
	}
	var list []*memRef
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, err
	}
	for _, x := range list {
		r.m[x.Ref.ID()] = x
	}
	return r, nil
}

func (r *MemoryRefs) save() error {
	if r.path == "" {
		return nil
	}
	list := make([]*memRef, 0, len(r.m))
	for _, x := range r.m {
		list = append(list, x)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Ref.ID() < list[j].Ref.ID() })
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, r.path)
}

// Add implements Refs.
func (r *MemoryRefs) Add(_ context.Context, ref Ref, caseID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	x := r.m[ref.ID()]
	if x == nil {
		x = &memRef{Ref: ref}
		r.m[ref.ID()] = x
	}
	for _, c := range x.Cases {
		if c == caseID {
			return nil
		}
	}
	x.Cases = append(x.Cases, caseID)
	return r.save()
}

// Release implements Refs.
func (r *MemoryRefs) Release(_ context.Context, caseID string) ([]Ref, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var orphaned []Ref
	kept := 0
	for id, x := range r.m {
		i := -1
		for j, c := range x.Cases {
			if c == caseID {
				i = j
			}
		}
		if i < 0 {
			continue
		}
		x.Cases = append(x.Cases[:i:i], x.Cases[i+1:]...)
		if len(x.Cases) == 0 {
			orphaned = append(orphaned, x.Ref)
			delete(r.m, id)
		} else {
			kept++
		}
	}
	sort.Slice(orphaned, func(i, j int) bool { return orphaned[i].ID() < orphaned[j].ID() })
	return orphaned, kept, r.save()
}

// Cases implements Refs.
func (r *MemoryRefs) Cases(context.Context) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	set := map[string]bool{}
	for _, x := range r.m {
		for _, c := range x.Cases {
			set[c] = true
		}
	}
	out := make([]string, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	sort.Strings(out)
	return out, nil
}

// OrphanReport is what a sweep for entries of vanished cases found.
type OrphanReport struct {
	// Cases is how many cases the index names that no longer exist (plus the
	// retry marker when entries failed to delete before).
	Cases int `json:"cases"`
	PurgeReport
}

// Orphans purges the entries of every case the index names that exists
// reports false for: a delete whose purge failed, a case that was never saved,
// a case removed by hand. dryRun only counts the cases.
func (p Purger) Orphans(ctx context.Context, exists func(ctx context.Context, caseID string) (bool, error), dryRun bool) (OrphanReport, error) {
	var rep OrphanReport
	ids, err := p.Refs.Cases(ctx)
	if err != nil {
		return rep, err
	}
	for _, id := range ids {
		if id != RetryOwner {
			ok, err := exists(ctx, id)
			if err != nil {
				return rep, err
			}
			if ok {
				continue
			}
		}
		rep.Cases++
		if dryRun {
			continue
		}
		pr, err := p.PurgeCase(ctx, id, nil)
		rep.Add(pr)
		if err != nil {
			return rep, err
		}
	}
	return rep, nil
}

// InUse implements Refs.
func (r *MemoryRefs) InUse(_ context.Context, ref Ref) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	x := r.m[ref.ID()]
	if x == nil {
		return false, nil
	}
	for _, c := range x.Cases {
		if c != RetryOwner {
			return true, nil
		}
	}
	return false, nil
}
