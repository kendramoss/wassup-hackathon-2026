package store

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/webhooks"
)

// Webhook collections (#915): one document per endpoint, and one per
// delivery. Queries filter on one field and sort in memory, so no composite
// index is needed; an endpoint keeps at most webhooks.MaxDeliveriesKept
// finished deliveries.
const (
	WebhookEndpointCollection = "webhook_endpoints"
	WebhookDeliveryCollection = "webhook_deliveries"
)

// Webhooks is the webhook store on the same client.
func (f *Firestore) Webhooks() *FirestoreWebhooks {
	return &FirestoreWebhooks{client: f.client, eps: f.client.Collection(WebhookEndpointCollection), dls: f.client.Collection(WebhookDeliveryCollection)}
}

// FirestoreWebhooks implements webhooks.Store.
type FirestoreWebhooks struct {
	client   *firestore.Client
	eps, dls *firestore.CollectionRef
}

func validWebhookID(id string) bool { return caseIDPattern.MatchString(id) }

// PutEndpoint implements webhooks.Store.
func (s *FirestoreWebhooks) PutEndpoint(ctx context.Context, e webhooks.Endpoint) error {
	if !validWebhookID(e.ID) {
		return fmt.Errorf("store: bad webhook id")
	}
	if _, err := s.eps.Doc(e.ID).Set(ctx, e); err != nil {
		return fmt.Errorf("store: write webhook endpoint: %w", err)
	}
	return nil
}

// GetEndpoint implements webhooks.Store.
func (s *FirestoreWebhooks) GetEndpoint(ctx context.Context, id string) (webhooks.Endpoint, error) {
	if !validWebhookID(id) {
		return webhooks.Endpoint{}, webhooks.ErrNotFound
	}
	snap, err := s.eps.Doc(id).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return webhooks.Endpoint{}, webhooks.ErrNotFound
	}
	if err != nil {
		return webhooks.Endpoint{}, fmt.Errorf("store: read webhook endpoint: %w", err)
	}
	var e webhooks.Endpoint
	if err := snap.DataTo(&e); err != nil {
		return webhooks.Endpoint{}, fmt.Errorf("store: decode webhook endpoint: %w", err)
	}
	return e, nil
}

// ListEndpoints implements webhooks.Store.
func (s *FirestoreWebhooks) ListEndpoints(ctx context.Context, workspace string) ([]webhooks.Endpoint, error) {
	it := s.eps.Where("workspace", "==", workspace).Documents(ctx)
	defer it.Stop()
	var out []webhooks.Endpoint
	for {
		snap, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("store: list webhook endpoints: %w", err)
		}
		var e webhooks.Endpoint
		if err := snap.DataTo(&e); err != nil {
			return nil, fmt.Errorf("store: decode webhook endpoint: %w", err)
		}
		out = append(out, e)
	}
	slices.SortFunc(out, func(a, b webhooks.Endpoint) int {
		if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
			return c
		}
		return cmpID(a.ID, b.ID)
	})
	return out, nil
}

func cmpID(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// DeleteEndpoint implements webhooks.Store.
func (s *FirestoreWebhooks) DeleteEndpoint(ctx context.Context, id string) error {
	if !validWebhookID(id) {
		return nil
	}
	if _, err := s.eps.Doc(id).Delete(ctx); err != nil && status.Code(err) != codes.NotFound {
		return fmt.Errorf("store: delete webhook endpoint: %w", err)
	}
	ds, err := s.deliveriesOf(ctx, id)
	if err != nil {
		return err
	}
	for _, d := range ds {
		if _, err := s.dls.Doc(d.ID).Delete(ctx); err != nil && status.Code(err) != codes.NotFound {
			return fmt.Errorf("store: delete webhook delivery: %w", err)
		}
	}
	return nil
}

func (s *FirestoreWebhooks) deliveriesOf(ctx context.Context, endpointID string) ([]webhooks.Delivery, error) {
	it := s.dls.Where("endpoint_id", "==", endpointID).Documents(ctx)
	defer it.Stop()
	var out []webhooks.Delivery
	for {
		snap, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("store: list webhook deliveries: %w", err)
		}
		var d webhooks.Delivery
		if err := snap.DataTo(&d); err != nil {
			return nil, fmt.Errorf("store: decode webhook delivery: %w", err)
		}
		out = append(out, d)
	}
	return out, nil
}

// CreateDelivery implements webhooks.Store.
func (s *FirestoreWebhooks) CreateDelivery(ctx context.Context, d webhooks.Delivery) error {
	if !validWebhookID(d.ID) {
		return fmt.Errorf("store: bad webhook delivery id")
	}
	if _, err := s.dls.Doc(d.ID).Create(ctx, d); err != nil {
		return fmt.Errorf("store: write webhook delivery: %w", err)
	}
	all, err := s.deliveriesOf(ctx, d.EndpointID)
	if err != nil {
		return err
	}
	all = slices.DeleteFunc(all, func(x webhooks.Delivery) bool { return x.Status == webhooks.StatusPending })
	if len(all) <= webhooks.MaxDeliveriesKept {
		return nil
	}
	slices.SortFunc(all, func(a, b webhooks.Delivery) int { return a.CreatedAt.Compare(b.CreatedAt) })
	for _, x := range all[:len(all)-webhooks.MaxDeliveriesKept] {
		if _, err := s.dls.Doc(x.ID).Delete(ctx); err != nil && status.Code(err) != codes.NotFound {
			return fmt.Errorf("store: trim webhook deliveries: %w", err)
		}
	}
	return nil
}

// UpdateDelivery implements webhooks.Store.
func (s *FirestoreWebhooks) UpdateDelivery(ctx context.Context, id string, fn func(*webhooks.Delivery) error) (webhooks.Delivery, error) {
	if !validWebhookID(id) {
		return webhooks.Delivery{}, webhooks.ErrNotFound
	}
	var out webhooks.Delivery
	ref := s.dls.Doc(id)
	err := s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if status.Code(err) == codes.NotFound {
			return webhooks.ErrNotFound
		}
		if err != nil {
			return err
		}
		var d webhooks.Delivery
		if err := snap.DataTo(&d); err != nil {
			return err
		}
		if err := fn(&d); err != nil {
			return err
		}
		out = d
		return tx.Set(ref, d)
	})
	if err != nil {
		if errors.Is(err, webhooks.ErrNotFound) {
			return webhooks.Delivery{}, err
		}
		return webhooks.Delivery{}, fmt.Errorf("store: update webhook delivery: %w", err)
	}
	return out, nil
}

// ListDeliveries implements webhooks.Store.
func (s *FirestoreWebhooks) ListDeliveries(ctx context.Context, endpointID string, limit int) ([]webhooks.Delivery, error) {
	out, err := s.deliveriesOf(ctx, endpointID)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b webhooks.Delivery) int {
		if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
			return c
		}
		return cmpID(b.ID, a.ID)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ListPending implements webhooks.Store.
func (s *FirestoreWebhooks) ListPending(ctx context.Context) ([]webhooks.Delivery, error) {
	it := s.dls.Where("status", "==", webhooks.StatusPending).Documents(ctx)
	defer it.Stop()
	var out []webhooks.Delivery
	for {
		snap, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("store: list pending webhook deliveries: %w", err)
		}
		var d webhooks.Delivery
		if err := snap.DataTo(&d); err != nil {
			return nil, fmt.Errorf("store: decode webhook delivery: %w", err)
		}
		out = append(out, d)
	}
	return out, nil
}

var _ webhooks.Store = (*FirestoreWebhooks)(nil)
