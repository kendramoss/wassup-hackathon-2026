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
	f, err := NewFirestore(ctx, "demo-origin-proof", "")
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
