package store

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/metering"
)

// MeteringCollection holds one document per workspace and UTC month with its
// use (metering.Store, #886): predictions, plan_searches and model_usd, each
// incremented atomically. BudgetCollection holds one document per workspace
// with its monthly budget.
const (
	MeteringCollection = "workspace_usage"
	BudgetCollection   = "workspace_budgets"
)

// Metering returns the metering store on the same database.
func (f *Firestore) Metering() *FirestoreMetering {
	return &FirestoreMetering{usage: f.client.Collection(MeteringCollection), budgets: f.client.Collection(BudgetCollection)}
}

// FirestoreMetering implements metering.Store.
type FirestoreMetering struct{ usage, budgets *firestore.CollectionRef }

func usageDoc(workspace, month string) string { return workspace + "_" + month }

// Add implements metering.Store.
func (s *FirestoreMetering) Add(ctx context.Context, workspace, month string, d metering.Delta) error {
	fields := map[string]any{"workspace": workspace, "month": month, "updated_at": firestore.ServerTimestamp}
	if d.Predictions != 0 {
		fields["predictions"] = firestore.Increment(d.Predictions)
	}
	if d.PlanSearches != 0 {
		fields["plan_searches"] = firestore.Increment(d.PlanSearches)
	}
	if d.ModelUSD > 0 {
		fields["model_usd"] = firestore.Increment(d.ModelUSD)
	}
	if _, err := s.usage.Doc(usageDoc(workspace, month)).Set(ctx, fields, firestore.MergeAll); err != nil {
		return fmt.Errorf("store: add usage: %w", err)
	}
	return nil
}

// Totals implements metering.Store.
func (s *FirestoreMetering) Totals(ctx context.Context, workspace, month string) (metering.Totals, error) {
	t := metering.Totals{Month: month}
	snap, err := s.usage.Doc(usageDoc(workspace, month)).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return t, nil
	}
	if err != nil {
		return t, fmt.Errorf("store: read usage: %w", err)
	}
	t.Predictions = int(number(snap, "predictions"))
	t.PlanSearches = int(number(snap, "plan_searches"))
	t.ModelUSD = number(snap, "model_usd")
	return t, nil
}

func number(snap *firestore.DocumentSnapshot, field string) float64 {
	v, err := snap.DataAt(field)
	if err != nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	}
	return 0
}

type budgetDoc struct {
	MonthlyUSD   float64   `firestore:"monthly_usd"`
	HardCap      bool      `firestore:"hard_cap"`
	AlertPercent int       `firestore:"alert_percent"`
	UpdatedBy    string    `firestore:"updated_by"`
	UpdatedAt    time.Time `firestore:"updated_at"`
}

// Budget implements metering.Store.
func (s *FirestoreMetering) Budget(ctx context.Context, workspace string) (metering.Budget, bool, error) {
	snap, err := s.budgets.Doc(workspace).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return metering.Budget{}, false, nil
	}
	if err != nil {
		return metering.Budget{}, false, fmt.Errorf("store: read budget: %w", err)
	}
	var d budgetDoc
	if err := snap.DataTo(&d); err != nil {
		return metering.Budget{}, false, fmt.Errorf("store: decode budget: %w", err)
	}
	return metering.Budget{MonthlyUSD: d.MonthlyUSD, HardCap: d.HardCap, AlertPercent: d.AlertPercent, UpdatedBy: d.UpdatedBy, UpdatedAt: d.UpdatedAt}, true, nil
}

// SetBudget implements metering.Store.
func (s *FirestoreMetering) SetBudget(ctx context.Context, workspace string, b metering.Budget) error {
	if err := b.Validate(); err != nil {
		return err
	}
	d := budgetDoc{MonthlyUSD: b.MonthlyUSD, HardCap: b.HardCap, AlertPercent: b.AlertPercent, UpdatedBy: b.UpdatedBy, UpdatedAt: b.UpdatedAt}
	if _, err := s.budgets.Doc(workspace).Set(ctx, d); err != nil {
		return fmt.Errorf("store: set budget: %w", err)
	}
	return nil
}

// DeleteBudget implements metering.Store.
func (s *FirestoreMetering) DeleteBudget(ctx context.Context, workspace string) error {
	if _, err := s.budgets.Doc(workspace).Delete(ctx); err != nil && status.Code(err) != codes.NotFound {
		return fmt.Errorf("store: delete budget: %w", err)
	}
	return nil
}

var _ metering.Store = (*FirestoreMetering)(nil)
