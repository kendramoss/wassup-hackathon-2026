package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/sso"
)

// SSO collections (#868).
const (
	SSOConnectionCollection = "sso_connections"
	SSOOneTimeCollection    = "sso_onetime"
)

// SSOConnections is the SSO connection store on the same client.
func (f *Firestore) SSOConnections() *FirestoreSSOConnections {
	return &FirestoreSSOConnections{client: f.client, col: f.client.Collection(SSOConnectionCollection)}
}

// SSOOneTime is the one-time sign-in state store on the same client.
func (f *Firestore) SSOOneTime() *FirestoreOneTime {
	return &FirestoreOneTime{client: f.client, col: f.client.Collection(SSOOneTimeCollection)}
}

// FirestoreSSOConnections implements sso.Store.
type FirestoreSSOConnections struct {
	client *firestore.Client
	col    *firestore.CollectionRef
}

// Get implements sso.Store.
func (s *FirestoreSSOConnections) Get(ctx context.Context, id string) (sso.Connection, error) {
	if !caseIDPattern.MatchString(id) {
		return sso.Connection{}, sso.ErrNotFound
	}
	return decodeConnection(s.col.Doc(id).Get(ctx))
}

func decodeConnection(snap *firestore.DocumentSnapshot, err error) (sso.Connection, error) {
	if status.Code(err) == codes.NotFound {
		return sso.Connection{}, sso.ErrNotFound
	}
	if err != nil {
		return sso.Connection{}, fmt.Errorf("store: read sso connection: %w", err)
	}
	var c sso.Connection
	if err := snap.DataTo(&c); err != nil {
		return sso.Connection{}, fmt.Errorf("store: decode sso connection %s: %w", snap.Ref.ID, err)
	}
	return c, nil
}

func (s *FirestoreSSOConnections) query(ctx context.Context, q firestore.Query) ([]sso.Connection, error) {
	it := q.Limit(50).Documents(ctx)
	defer it.Stop()
	var out []sso.Connection
	for {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		c, err := decodeConnection(snap, err)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
}

// List implements sso.Store.
func (s *FirestoreSSOConnections) List(ctx context.Context, workspace string) ([]sso.Connection, error) {
	return s.query(ctx, s.col.Where("workspace_id", "==", workspace))
}

// ByDomain implements sso.Store.
func (s *FirestoreSSOConnections) ByDomain(ctx context.Context, domain string) ([]sso.Connection, error) {
	return s.query(ctx, s.col.Where("domains", "array-contains", domain))
}

// Put implements sso.Store: the domain clash check reads the other
// connections inside the transaction that writes this one.
func (s *FirestoreSSOConnections) Put(ctx context.Context, c sso.Connection) error {
	if !caseIDPattern.MatchString(c.ID) {
		return sso.ErrInvalid
	}
	err := s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		for d := range c.Verified {
			snaps, err := tx.Documents(s.col.Where("domains", "array-contains", d).Limit(50)).GetAll()
			if err != nil {
				return err
			}
			for _, snap := range snaps {
				o, err := decodeConnection(snap, nil)
				if err != nil {
					return err
				}
				if o.ID != c.ID && sso.VerifiedClash(o, c) {
					return sso.ErrDomainTaken
				}
			}
		}
		return tx.Set(s.col.Doc(c.ID), c)
	})
	if errors.Is(err, sso.ErrDomainTaken) {
		return err
	}
	if err != nil {
		return fmt.Errorf("store: put sso connection: %w", err)
	}
	return nil
}

// Delete implements sso.Store.
func (s *FirestoreSSOConnections) Delete(ctx context.Context, id string) error {
	if !caseIDPattern.MatchString(id) {
		return sso.ErrNotFound
	}
	if _, err := s.col.Doc(id).Get(ctx); status.Code(err) == codes.NotFound {
		return sso.ErrNotFound
	} else if err != nil {
		return fmt.Errorf("store: read sso connection: %w", err)
	}
	if _, err := s.col.Doc(id).Delete(ctx); err != nil {
		return fmt.Errorf("store: delete sso connection: %w", err)
	}
	return nil
}

// FirestoreOneTime implements sso.OneTime. Keys are hashed into document IDs,
// so a relay state or assertion ID is never a document name. Expired documents
// are ignored on read and overwritten on the next Put; a TTL policy on
// expires_at can sweep them.
type FirestoreOneTime struct {
	client *firestore.Client
	col    *firestore.CollectionRef
}

type oneTimeDoc struct {
	Val     []byte    `firestore:"val"`
	Expires time.Time `firestore:"expires_at"`
}

func oneTimeID(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

// Put implements sso.OneTime.
func (s *FirestoreOneTime) Put(ctx context.Context, key string, val []byte, ttl time.Duration) error {
	ref := s.col.Doc(oneTimeID(key))
	err := s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		switch {
		case err == nil:
			var d oneTimeDoc
			if err := snap.DataTo(&d); err == nil && time.Now().Before(d.Expires) {
				return sso.ErrExists
			}
		case status.Code(err) != codes.NotFound:
			return err
		}
		return tx.Set(ref, oneTimeDoc{Val: val, Expires: time.Now().Add(ttl).UTC()})
	})
	if errors.Is(err, sso.ErrExists) {
		return err
	}
	if err != nil {
		return fmt.Errorf("store: put sso state: %w", err)
	}
	return nil
}

// Take implements sso.OneTime.
func (s *FirestoreOneTime) Take(ctx context.Context, key string) ([]byte, error) {
	ref := s.col.Doc(oneTimeID(key))
	var out []byte
	err := s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if status.Code(err) == codes.NotFound {
			return sso.ErrNoState
		}
		if err != nil {
			return err
		}
		var d oneTimeDoc
		if err := snap.DataTo(&d); err != nil {
			return err
		}
		if err := tx.Delete(ref); err != nil {
			return err
		}
		if !time.Now().Before(d.Expires) {
			return sso.ErrNoState
		}
		out = d.Val
		return nil
	})
	if errors.Is(err, sso.ErrNoState) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("store: take sso state: %w", err)
	}
	return out, nil
}

var (
	_ sso.Store   = (*FirestoreSSOConnections)(nil)
	_ sso.OneTime = (*FirestoreOneTime)(nil)
)
