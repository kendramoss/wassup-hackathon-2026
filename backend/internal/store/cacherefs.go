package store

import (
	"context"
	"fmt"
	"slices"

	"cloud.google.com/go/firestore"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/blob"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CacheRefCollection holds one document per cache entry with the cases that
// use it (#875, CACHE_PURGE). It holds IDs only, no case content.
const CacheRefCollection = "cache_refs"

// CacheRefs is the cache-entry index on the same client.
func (f *Firestore) CacheRefs() *FirestoreCacheRefs {
	return &FirestoreCacheRefs{col: f.client.Collection(CacheRefCollection), client: f.client}
}

// FirestoreCacheRefs implements blob.Refs.
type FirestoreCacheRefs struct {
	col    *firestore.CollectionRef
	client *firestore.Client
}

var _ blob.Refs = (*FirestoreCacheRefs)(nil)

type cacheRefDoc struct {
	Store string   `firestore:"store"`
	Key   string   `firestore:"key"`
	Cases []string `firestore:"cases"`
}

// Add implements blob.Refs.
func (r *FirestoreCacheRefs) Add(ctx context.Context, ref blob.Ref, caseID string) error {
	_, err := r.col.Doc(ref.ID()).Set(ctx, map[string]any{"store": ref.Store, "key": ref.Key, "cases": firestore.ArrayUnion(caseID)}, firestore.MergeAll)
	if err != nil {
		return fmt.Errorf("store: cache ref: %w", err)
	}
	return nil
}

// Release implements blob.Refs. Each entry is updated in its own
// transaction, so a case added by another request between the read and the
// write is not lost.
func (r *FirestoreCacheRefs) Release(ctx context.Context, caseID string) ([]blob.Ref, int, error) {
	it := r.col.Where("cases", "array-contains", caseID).Documents(ctx)
	var ids []string
	for {
		snap, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			it.Stop()
			return nil, 0, fmt.Errorf("store: list cache refs: %w", err)
		}
		ids = append(ids, snap.Ref.ID)
	}
	it.Stop()
	var orphaned []blob.Ref
	kept := 0
	for _, id := range ids {
		doc := r.col.Doc(id)
		var gone *blob.Ref
		err := r.client.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
			gone = nil
			snap, err := tx.Get(doc)
			if status.Code(err) == codes.NotFound {
				return nil
			}
			if err != nil {
				return err
			}
			var d cacheRefDoc
			if err := snap.DataTo(&d); err != nil {
				return err
			}
			d.Cases = slices.DeleteFunc(d.Cases, func(c string) bool { return c == caseID })
			if len(d.Cases) == 0 {
				gone = &blob.Ref{Store: d.Store, Key: d.Key}
				return tx.Delete(doc)
			}
			return tx.Update(doc, []firestore.Update{{Path: "cases", Value: d.Cases}})
		})
		if err != nil {
			return orphaned, kept, fmt.Errorf("store: release cache ref: %w", err)
		}
		if gone != nil {
			orphaned = append(orphaned, *gone)
		} else {
			kept++
		}
	}
	return orphaned, kept, nil
}

// Cases implements blob.Refs.
func (r *FirestoreCacheRefs) Cases(ctx context.Context) ([]string, error) {
	it := r.col.Documents(ctx)
	defer it.Stop()
	set := map[string]bool{}
	for {
		snap, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("store: list cache refs: %w", err)
		}
		var d cacheRefDoc
		if err := snap.DataTo(&d); err != nil {
			return nil, err
		}
		for _, c := range d.Cases {
			set[c] = true
		}
	}
	out := make([]string, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	slices.Sort(out)
	return out, nil
}

// Delete implements blob.Deleter. A missing document is not an error.
func (b *FirestoreBlobs) Delete(ctx context.Context, key string) error {
	_, err := b.col.Doc(key).Delete(ctx)
	if err != nil && status.Code(err) != codes.NotFound {
		return fmt.Errorf("store: delete cache entry: %w", err)
	}
	return nil
}

var _ blob.Deleter = (*FirestoreBlobs)(nil)

// InUse implements blob.Refs.
func (r *FirestoreCacheRefs) InUse(ctx context.Context, ref blob.Ref) (bool, error) {
	snap, err := r.col.Doc(ref.ID()).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: read cache ref: %w", err)
	}
	var d cacheRefDoc
	if err := snap.DataTo(&d); err != nil {
		return false, err
	}
	for _, c := range d.Cases {
		if c != blob.RetryOwner {
			return true, nil
		}
	}
	return false, nil
}
