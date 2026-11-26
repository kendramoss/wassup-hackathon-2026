package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/metering"
)

// TestFirestoreMeteringEmulator runs against the Firestore emulator (see
// TestFirestoreEmulator).
func TestFirestoreMeteringEmulator(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	ctx := context.Background()
	f, err := NewFirestore(ctx, fmt.Sprintf("metering-test-%d", time.Now().UnixNano()), "")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	m := f.Metering()
	if tot, err := m.Totals(ctx, "ws", "2026-11"); err != nil || tot.ModelUSD != 0 {
		t.Fatalf("empty totals = %+v, %v", tot, err)
	}
	for range 3 {
		if err := m.Add(ctx, "ws", "2026-11", metering.Delta{Predictions: 1, ModelUSD: 0.5}); err != nil {
			t.Fatal(err)
		}
	}
	_ = m.Add(ctx, "ws", "2026-11", metering.Delta{PlanSearches: 1})
	tot, err := m.Totals(ctx, "ws", "2026-11")
	if err != nil || tot.Predictions != 3 || tot.PlanSearches != 1 || tot.ModelUSD != 1.5 {
		t.Fatalf("totals = %+v, %v", tot, err)
	}
	if _, ok, _ := m.Budget(ctx, "ws"); ok {
		t.Fatal("budget before set")
	}
	if err := m.SetBudget(ctx, "ws", metering.Budget{MonthlyUSD: 20, HardCap: true, UpdatedBy: "a@b.c", UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	b, ok, err := m.Budget(ctx, "ws")
	if err != nil || !ok || b.MonthlyUSD != 20 || !b.HardCap {
		t.Fatalf("budget = %+v %v %v", b, ok, err)
	}
	if err := m.DeleteBudget(ctx, "ws"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := m.Budget(ctx, "ws"); ok {
		t.Fatal("budget after delete")
	}
}
