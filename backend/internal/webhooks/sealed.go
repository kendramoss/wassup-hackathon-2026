package webhooks

import (
	"context"
	"fmt"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/envelope"
)

// SealPurpose labels the sealing key derived from WEBHOOKS_SECRET_KEY.
const SealPurpose = "webhooks-endpoint-secret"

// SealedStore wraps a Store so an endpoint's signing secret and its previous
// secret are written as AES-256-GCM sealed values and opened on the way out.
// Everything else passes through. It fails closed: a record whose secret does
// not open (another key, a value moved between endpoints or fields, a record
// written before sealing existed) is an error, never a plaintext fallback.
type SealedStore struct {
	Store
	sealer *envelope.Sealer
}

// NewSealedStore returns a Store that seals secrets before they reach inner.
func NewSealedStore(inner Store, s *envelope.Sealer) *SealedStore {
	return &SealedStore{Store: inner, sealer: s}
}

func sealCtx(id, field string) string { return "webhook-endpoint:" + id + ":" + field }

// PutEndpoint implements Store.
func (s *SealedStore) PutEndpoint(ctx context.Context, e Endpoint) error {
	var err error
	if e.Secret, err = s.sealer.Seal(e.Secret, sealCtx(e.ID, "secret")); err != nil {
		return fmt.Errorf("webhooks: seal secret: %w", err)
	}
	if e.PrevSecret != "" {
		if e.PrevSecret, err = s.sealer.Seal(e.PrevSecret, sealCtx(e.ID, "prev_secret")); err != nil {
			return fmt.Errorf("webhooks: seal secret: %w", err)
		}
	}
	return s.Store.PutEndpoint(ctx, e)
}

func (s *SealedStore) open(e Endpoint) (Endpoint, error) {
	var err error
	if e.Secret, err = s.sealer.Open(e.Secret, sealCtx(e.ID, "secret")); err != nil {
		return Endpoint{}, fmt.Errorf("webhooks: endpoint %s: its secret cannot be opened (WEBHOOKS_SECRET_KEY changed, or the record was written before secrets were sealed): %w", e.ID, err)
	}
	if e.PrevSecret != "" {
		if e.PrevSecret, err = s.sealer.Open(e.PrevSecret, sealCtx(e.ID, "prev_secret")); err != nil {
			return Endpoint{}, fmt.Errorf("webhooks: endpoint %s: its previous secret cannot be opened: %w", e.ID, err)
		}
	}
	return e, nil
}

// GetEndpoint implements Store.
func (s *SealedStore) GetEndpoint(ctx context.Context, id string) (Endpoint, error) {
	e, err := s.Store.GetEndpoint(ctx, id)
	if err != nil {
		return Endpoint{}, err
	}
	return s.open(e)
}

// ListEndpoints implements Store.
func (s *SealedStore) ListEndpoints(ctx context.Context, workspace string) ([]Endpoint, error) {
	eps, err := s.Store.ListEndpoints(ctx, workspace)
	if err != nil {
		return nil, err
	}
	for i := range eps {
		if eps[i], err = s.open(eps[i]); err != nil {
			return nil, err
		}
	}
	return eps, nil
}

var _ Store = (*SealedStore)(nil)
