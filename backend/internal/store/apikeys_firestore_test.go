package store

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/apikey"
)

// TestFirestoreAPIKeys runs the API key store (D69) against the emulator;
// see TestFirestoreEmulator.
func TestFirestoreAPIKeys(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	ctx := context.Background()
	f, err := NewFirestore(ctx, "demo-unde", "")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	keys := apikey.New(f.APIKeys())
	secret, k, err := keys.Issue(ctx, "Emulator broker", 2)
	if err != nil {
		t.Fatal(err)
	}
	got, err := keys.Authenticate(ctx, secret)
	if err != nil || got.ID != k.ID || got.Name != "Emulator broker" || got.DailyQuota != 2 {
		t.Fatalf("Authenticate = %+v %v", got, err)
	}
	for i := 1; i <= 3; i++ {
		ok, n, _, err := keys.Use(ctx, got)
		if err != nil || n != int64(i) || ok != (i <= 2) {
			t.Errorf("use %d = %v %d %v", i, ok, n, err)
		}
	}
	// A self-service key carries an owner; the query and the scoped disable
	// must not touch an admin key or another account's key (#788).
	ownedSecret, owned, err := keys.IssueFor(ctx, "u_alice", "Alice's agent", 5)
	if err != nil {
		t.Fatal(err)
	}
	mine, err := keys.ListForOwner(ctx, "u_alice")
	if err != nil || len(mine) != 1 || mine[0].ID != owned.ID || mine[0].Owner != "u_alice" {
		t.Fatalf("ListForOwner = %+v %v", mine, err)
	}
	if err := keys.ExistsForOwner(ctx, "u_bob", owned.ID); !errors.Is(err, apikey.ErrNotFound) {
		t.Errorf("another account sees the key: %v", err)
	}
	if err := keys.DisableForOwner(ctx, "u_bob", owned.ID); !errors.Is(err, apikey.ErrNotFound) {
		t.Errorf("another account revoked the key: %v", err)
	}
	if err := keys.DisableForOwner(ctx, "u_alice", owned.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := keys.Authenticate(ctx, ownedSecret); !errors.Is(err, apikey.ErrInvalid) {
		t.Errorf("revoked self-service key still works: %v", err)
	}

	if err := keys.Disable(ctx, k.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := keys.Authenticate(ctx, secret); !errors.Is(err, apikey.ErrInvalid) {
		t.Errorf("disabled key = %v", err)
	}
	if err := keys.Disable(ctx, "0123456789abcdef"); !errors.Is(err, apikey.ErrNotFound) {
		t.Errorf("disable unknown = %v", err)
	}
	ks, err := keys.List(ctx)
	if err != nil || len(ks) == 0 {
		t.Errorf("List = %d %v", len(ks), err)
	}
}
