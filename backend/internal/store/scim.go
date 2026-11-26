package store

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/scim"
)

// SCIMTokenCollection holds one SCIM token hash per workspace (#869), keyed
// by workspace ID.
const SCIMTokenCollection = "scim_tokens"

// SCIMTokens is the SCIM token store on the same client.
func (f *Firestore) SCIMTokens() *FirestoreSCIMTokens {
	return &FirestoreSCIMTokens{col: f.client.Collection(SCIMTokenCollection)}
}

// FirestoreSCIMTokens implements scim.Store.
type FirestoreSCIMTokens struct{ col *firestore.CollectionRef }

func decodeSCIMToken(snap *firestore.DocumentSnapshot, err error) (scim.Token, error) {
	if status.Code(err) == codes.NotFound {
		return scim.Token{}, scim.ErrNotFound
	}
	if err != nil {
		return scim.Token{}, fmt.Errorf("store: read scim token: %w", err)
	}
	var t scim.Token
	if err := snap.DataTo(&t); err != nil {
		return scim.Token{}, fmt.Errorf("store: decode scim token: %w", err)
	}
	return t, nil
}

// Get implements scim.Store.
func (s *FirestoreSCIMTokens) Get(ctx context.Context, ws string) (scim.Token, error) {
	if !caseIDPattern.MatchString(ws) {
		return scim.Token{}, scim.ErrNotFound
	}
	return decodeSCIMToken(s.col.Doc(ws).Get(ctx))
}

// ByHash implements scim.Store.
func (s *FirestoreSCIMTokens) ByHash(ctx context.Context, hash string) (scim.Token, error) {
	it := s.col.Where("hash", "==", hash).Limit(1).Documents(ctx)
	defer it.Stop()
	snap, err := it.Next()
	if err == iterator.Done {
		return scim.Token{}, scim.ErrNotFound
	}
	return decodeSCIMToken(snap, err)
}

// Put implements scim.Store.
func (s *FirestoreSCIMTokens) Put(ctx context.Context, t scim.Token) error {
	if !caseIDPattern.MatchString(t.Workspace) {
		return fmt.Errorf("store: bad workspace id")
	}
	if _, err := s.col.Doc(t.Workspace).Set(ctx, t); err != nil {
		return fmt.Errorf("store: write scim token: %w", err)
	}
	return nil
}

// Delete implements scim.Store.
func (s *FirestoreSCIMTokens) Delete(ctx context.Context, ws string) error {
	if !caseIDPattern.MatchString(ws) {
		return nil
	}
	if _, err := s.col.Doc(ws).Delete(ctx); err != nil && status.Code(err) != codes.NotFound {
		return fmt.Errorf("store: delete scim token: %w", err)
	}
	return nil
}

var _ scim.Store = (*FirestoreSCIMTokens)(nil)
