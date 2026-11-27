package store

import (
	"context"
	"fmt"
	"sort"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retention"
)

// Retention collections (#875): one policy document per workspace, one
// tombstone per hard-deleted case.
const (
	RetentionPolicyCollection = "retention_policies"
	TombstoneCollection       = "case_tombstones"
)

// maxTombstoneRead bounds one tombstone listing; the query has a single
// equality filter so it needs no composite index, and is sorted here.
const maxTombstoneRead = 2000

// Retention is the retention store on the same client.
func (f *Firestore) Retention() *FirestoreRetention {
	return &FirestoreRetention{policies: f.client.Collection(RetentionPolicyCollection), tombs: f.client.Collection(TombstoneCollection)}
}

// FirestoreRetention implements retention.Store.
type FirestoreRetention struct{ policies, tombs *firestore.CollectionRef }

var _ retention.Store = (*FirestoreRetention)(nil)

// Policy implements retention.Store.
func (r *FirestoreRetention) Policy(ctx context.Context, ws string) (retention.Policy, bool, error) {
	snap, err := r.policies.Doc(ws).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return retention.Policy{}, false, nil
	}
	if err != nil {
		return retention.Policy{}, false, fmt.Errorf("store: read retention policy: %w", err)
	}
	var p retention.Policy
	if err := snap.DataTo(&p); err != nil {
		return retention.Policy{}, false, fmt.Errorf("store: decode retention policy: %w", err)
	}
	return p, true, nil
}

// SetPolicy implements retention.Store.
func (r *FirestoreRetention) SetPolicy(ctx context.Context, p retention.Policy) error {
	if err := p.Validate(); err != nil {
		return err
	}
	_, err := r.policies.Doc(p.Workspace).Set(ctx, p)
	return err
}

// Policies implements retention.Store.
func (r *FirestoreRetention) Policies(ctx context.Context) ([]retention.Policy, error) {
	it := r.policies.Documents(ctx)
	defer it.Stop()
	var out []retention.Policy
	for {
		snap, err := it.Next()
		if err == iterator.Done {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("store: list retention policies: %w", err)
		}
		var p retention.Policy
		if err := snap.DataTo(&p); err != nil {
			return nil, fmt.Errorf("store: decode retention policy: %w", err)
		}
		out = append(out, p)
	}
}

// AddTombstone implements retention.Store.
func (r *FirestoreRetention) AddTombstone(ctx context.Context, t retention.Tombstone) error {
	id := fmt.Sprintf("%s_%d", t.CaseID, t.DeletedAt.UnixNano())
	_, err := r.tombs.Doc(id).Create(ctx, t)
	if status.Code(err) == codes.AlreadyExists {
		return nil
	}
	return err
}

// Tombstones implements retention.Store.
func (r *FirestoreRetention) Tombstones(ctx context.Context, ws string, limit int) ([]retention.Tombstone, error) {
	it := r.tombs.Where("workspace", "==", ws).Limit(maxTombstoneRead).Documents(ctx)
	defer it.Stop()
	var out []retention.Tombstone
	for {
		snap, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("store: list tombstones: %w", err)
		}
		var t retention.Tombstone
		if err := snap.DataTo(&t); err != nil {
			return nil, fmt.Errorf("store: decode tombstone: %w", err)
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DeletedAt.After(out[j].DeletedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// RetentionCases adapts a Cases store to retention.Cases.
type RetentionCases struct{ Cases Cases }

// WorkspaceCases implements retention.Cases.
func (rc RetentionCases) WorkspaceCases(ctx context.Context, ws string) ([]retention.Case, error) {
	cs, err := ListWorkspace(ctx, rc.Cases, ws)
	if err != nil {
		return nil, err
	}
	out := make([]retention.Case, 0, len(cs))
	for _, c := range cs {
		out = append(out, retention.Case{ID: c.ID, Workspace: c.Workspace, UpdatedAt: c.UpdatedAt, Pinned: c.Pinned, Derived: c.DerivedBlobs()})
	}
	return out, nil
}

// DeleteCase implements retention.Cases.
func (rc RetentionCases) DeleteCase(ctx context.Context, id string) error {
	return rc.Cases.Delete(ctx, id)
}
