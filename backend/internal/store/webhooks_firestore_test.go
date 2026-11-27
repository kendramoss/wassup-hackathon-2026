package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/webhooks"
)

// TestFirestoreWebhooks runs the webhook store (#915) against the emulator;
// see TestFirestoreEmulator.
func TestFirestoreWebhooks(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	ctx := context.Background()
	f, err := NewFirestore(ctx, "demo-unde", "")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s := f.Webhooks()
	base := time.Now().UTC().Truncate(time.Second)
	e := webhooks.Endpoint{ID: webhooks.NewID("wh_"), Workspace: "ws_emu", URL: "https://x.example.com/in", Events: []string{webhooks.EventBatchFinished}, Enabled: true, Secret: "whsec_x", CreatedAt: base}
	if err := s.PutEndpoint(ctx, e); err != nil {
		t.Fatal(err)
	}
	if got, err := s.ListEndpoints(ctx, "ws_emu"); err != nil || len(got) != 1 || got[0].Secret != "whsec_x" {
		t.Fatalf("list = %+v %v", got, err)
	}
	if _, err := s.GetEndpoint(ctx, "wh_missing"); !errors.Is(err, webhooks.ErrNotFound) {
		t.Errorf("missing = %v", err)
	}
	d := webhooks.Delivery{ID: webhooks.NewID("dl_"), EndpointID: e.ID, Workspace: "ws_emu", Event: webhooks.EventBatchFinished, Status: webhooks.StatusPending, CreatedAt: base, Payload: "{}"}
	if err := s.CreateDelivery(ctx, d); err != nil {
		t.Fatal(err)
	}
	if p, err := s.ListPending(ctx); err != nil || len(p) == 0 {
		t.Fatalf("pending = %v %v", p, err)
	}
	got, err := s.UpdateDelivery(ctx, d.ID, func(x *webhooks.Delivery) error {
		x.Status = webhooks.StatusDelivered
		x.Attempts = append(x.Attempts, webhooks.Attempt{At: base, StatusCode: 200})
		return nil
	})
	if err != nil || got.Status != webhooks.StatusDelivered || len(got.Attempts) != 1 {
		t.Fatalf("update = %+v %v", got, err)
	}
	if l, _ := s.ListDeliveries(ctx, e.ID, 5); len(l) != 1 {
		t.Errorf("log = %+v", l)
	}
	if err := s.DeleteEndpoint(ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	if l, _ := s.ListDeliveries(ctx, e.ID, 5); len(l) != 0 {
		t.Errorf("deliveries outlive the endpoint: %+v", l)
	}
}
