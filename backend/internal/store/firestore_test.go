package store

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

func TestGzipJSONRoundTrip(t *testing.T) {
	in := Case{ID: "case_a", Facts: strings.Repeat("assembled in Thailand. ", 2000), Plan: &planner.Result{}}
	b, err := gzipJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > len(in.Facts)/10 {
		t.Errorf("gzip barely compressed: %d bytes for %d", len(b), len(in.Facts))
	}
	var out Case
	if err := gunzipJSON(b, &out); err != nil || out.Facts != in.Facts || out.Plan == nil {
		t.Fatalf("round trip = %v (facts equal %t)", err, out.Facts == in.Facts)
	}
}

// TestFirestoreEmulator runs against the Firestore emulator:
//
//	gcloud emulators firestore start --host-port=localhost:8681
//	FIRESTORE_EMULATOR_HOST=localhost:8681 go test ./internal/store
func TestFirestoreEmulator(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	ctx := context.Background()
	f, err := NewFirestore(ctx, "demo-origin-proof", "")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Get(ctx, "missing_case"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get missing = %v", err)
	}
	if _, err := f.Get(ctx, "../etc/passwd"); !errors.Is(err, ErrNotFound) {
		t.Errorf("invalid id not rejected: %v", err)
	}
	_ = f.Put(ctx, Case{ID: "case_gone", UpdatedAt: time.Now().UTC()})
	if err := f.Delete(ctx, "case_gone"); err != nil {
		t.Errorf("Delete = %v", err)
	}
	if err := f.Delete(ctx, "case_gone"); !errors.Is(err, ErrNotFound) {
		t.Errorf("second Delete = %v", err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	_ = f.Put(ctx, Case{ID: "case_a", UpdatedAt: now.Add(-time.Hour)})
	if err := f.Put(ctx, Case{ID: "case_b", UpdatedAt: now, Plan: &planner.Result{}}); err != nil {
		t.Fatal(err)
	}
	f2, err := NewFirestore(ctx, "demo-origin-proof", "") // a restart
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()
	cs, err := f2.List(ctx)
	if err != nil || len(cs) < 2 || cs[0].ID != "case_b" || cs[0].Step() != "plan" || !cs[0].UpdatedAt.Equal(now) {
		t.Errorf("list after restart = %+v %v", cs, err)
	}
}
