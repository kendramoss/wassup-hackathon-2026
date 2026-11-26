// Package metering counts each workspace's use of the product per calendar
// month (UTC): predictions, plan searches and model spend in USD, and holds
// the workspace's budget (#886). A budget with a hard cap refuses new
// model-calling work once the month's spend reaches it; without a hard cap it
// only reports. Nothing here bills anyone: there are no prices beyond the
// model cost the spend meter already computes (evalkit.PromoPrices).
//
// Spend reaches the meter through the daily spend meter's observer
// (spend.Meter.Observe), so every model call is charged once, at the raw
// client, to the workspace in its request context.
package metering

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sync"
	"sync/atomic"
	"time"
)

// Delta is what one event adds to a workspace's month.
type Delta struct {
	Predictions  int
	PlanSearches int
	ModelUSD     float64
}

// Totals is a workspace's use in one month.
type Totals struct {
	Month        string  `json:"month"`
	Predictions  int     `json:"predictions"`
	PlanSearches int     `json:"plan_searches"`
	ModelUSD     float64 `json:"model_usd"`
}

// Budget is a workspace's monthly model-spend budget.
type Budget struct {
	// MonthlyUSD is the month's budget; it must be positive.
	MonthlyUSD float64 `json:"monthly_usd"`
	// HardCap refuses new model-calling requests once the month's spend
	// reaches MonthlyUSD, and plan searches whose estimate would pass it.
	// Off: the budget is reported, never enforced.
	HardCap bool `json:"hard_cap"`
	// AlertPercent marks the status as alerting from this share of the
	// budget (1-100; 0 means 80).
	AlertPercent int       `json:"alert_percent"`
	UpdatedBy    string    `json:"updated_by,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// DefaultAlertPercent is the alert threshold when a budget does not set one.
const DefaultAlertPercent = 80

// MaxMonthlyUSD bounds a budget so a typo cannot set an effectively infinite one.
const MaxMonthlyUSD = 1_000_000

// ErrInvalid marks a budget or month the caller must fix.
var ErrInvalid = errors.New("metering: invalid")

// Validate checks b.
func (b Budget) Validate() error {
	switch {
	case math.IsNaN(b.MonthlyUSD) || b.MonthlyUSD <= 0 || b.MonthlyUSD > MaxMonthlyUSD:
		return fmt.Errorf("%w: monthly_usd must be more than 0 and at most %d", ErrInvalid, MaxMonthlyUSD)
	case b.AlertPercent < 0 || b.AlertPercent > 100:
		return fmt.Errorf("%w: alert_percent must be 0 to 100", ErrInvalid)
	}
	return nil
}

func (b Budget) alertAt() float64 {
	p := b.AlertPercent
	if p == 0 {
		p = DefaultAlertPercent
	}
	return b.MonthlyUSD * float64(p) / 100
}

// Store keeps the monthly totals and the budgets. Add must be atomic per
// workspace and month across instances.
type Store interface {
	Add(ctx context.Context, workspace, month string, d Delta) error
	Totals(ctx context.Context, workspace, month string) (Totals, error)
	// Budget returns the workspace's budget; ok is false when none is set.
	Budget(ctx context.Context, workspace string) (b Budget, ok bool, err error)
	SetBudget(ctx context.Context, workspace string, b Budget) error
	DeleteBudget(ctx context.Context, workspace string) error
}

var monthPattern = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)

// ParseMonth checks a YYYY-MM month.
func ParseMonth(s string) (string, error) {
	if !monthPattern.MatchString(s) {
		return "", fmt.Errorf("%w: month must be YYYY-MM, got %q", ErrInvalid, s)
	}
	return s, nil
}

// Meter charges use to the workspace in the request context.
type Meter struct {
	store Store
	now   func() time.Time
	// failed counts writes the store refused; the use is then not counted.
	failed atomic.Int64
}

// New returns a meter over store.
func New(store Store) *Meter { return &Meter{store: store, now: time.Now} }

// WithClock sets the clock (tests).
func (m *Meter) WithClock(now func() time.Time) *Meter {
	m.now = now
	return m
}

// Month is the current UTC month, YYYY-MM.
func (m *Meter) Month() string { return m.now().UTC().Format("2006-01") }

// Failed is how many writes the store refused since start.
func (m *Meter) Failed() int64 { return m.failed.Load() }

type wsKey struct{}

// WithWorkspace marks ctx's work as done for workspace.
func WithWorkspace(ctx context.Context, workspace string) context.Context {
	if workspace == "" {
		return ctx
	}
	return context.WithValue(ctx, wsKey{}, workspace)
}

// WorkspaceFrom is the workspace ctx's work is charged to ("" for none).
func WorkspaceFrom(ctx context.Context) string {
	w, _ := ctx.Value(wsKey{}).(string)
	return w
}

// Add charges d to ctx's workspace; without one it does nothing.
func (m *Meter) Add(ctx context.Context, d Delta) {
	if m == nil {
		return
	}
	ws := WorkspaceFrom(ctx)
	if ws == "" || (d.Predictions == 0 && d.PlanSearches == 0 && d.ModelUSD <= 0) {
		return
	}
	if d.ModelUSD < 0 || math.IsNaN(d.ModelUSD) {
		d.ModelUSD = 0
	}
	if err := m.store.Add(context.WithoutCancel(ctx), ws, m.Month(), d); err != nil {
		m.failed.Add(1)
	}
}

// Charge adds model spend; it has the signature of spend.Meter's observer.
func (m *Meter) Charge(ctx context.Context, usd float64) { m.Add(ctx, Delta{ModelUSD: usd}) }

// Status is a workspace's month against its budget.
type Status struct {
	Totals Totals  `json:"totals"`
	Budget *Budget `json:"budget"`
	// RemainingUSD is the budget left this month (0 when spent; nil
	// without a budget).
	RemainingUSD *float64 `json:"remaining_usd"`
	// Alert is true from the budget's alert share on.
	Alert bool `json:"alert"`
	// Blocked is true when a hard cap is reached: model-calling requests
	// are refused until the next month or a larger budget.
	Blocked bool `json:"blocked"`
}

// Status reads workspace's month and budget.
func (m *Meter) Status(ctx context.Context, workspace, month string) (Status, error) {
	t, err := m.store.Totals(ctx, workspace, month)
	if err != nil {
		return Status{}, err
	}
	t.Month = month
	t.ModelUSD = round6(t.ModelUSD)
	st := Status{Totals: t}
	b, ok, err := m.store.Budget(ctx, workspace)
	if err != nil {
		return Status{}, err
	}
	if !ok {
		return st, nil
	}
	rem := round6(math.Max(0, b.MonthlyUSD-t.ModelUSD))
	st.Budget, st.RemainingUSD = &b, &rem
	st.Alert = t.ModelUSD >= b.alertAt()
	st.Blocked = b.HardCap && t.ModelUSD >= b.MonthlyUSD
	return st, nil
}

// WouldExceed reports whether spending up to estUSD more this month would
// pass a hard cap; false without one.
func (s Status) WouldExceed(estUSD float64) bool {
	if s.Budget == nil || !s.Budget.HardCap {
		return false
	}
	return s.Blocked || s.Totals.ModelUSD+estUSD > s.Budget.MonthlyUSD
}

func round6(v float64) float64 { return math.Round(v*1e6) / 1e6 }

// Memory is an in-process Store (local runs and tests).
type Memory struct {
	mu      sync.Mutex
	totals  map[string]Totals // workspace + "/" + month
	budgets map[string]Budget
}

// NewMemory returns an empty store.
func NewMemory() *Memory {
	return &Memory{totals: map[string]Totals{}, budgets: map[string]Budget{}}
}

// Add implements Store.
func (s *Memory) Add(_ context.Context, workspace, month string, d Delta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := workspace + "/" + month
	t := s.totals[k]
	t.Month = month
	t.Predictions += d.Predictions
	t.PlanSearches += d.PlanSearches
	t.ModelUSD += d.ModelUSD
	s.totals[k] = t
	return nil
}

// Totals implements Store.
func (s *Memory) Totals(_ context.Context, workspace, month string) (Totals, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.totals[workspace+"/"+month]
	t.Month = month
	return t, nil
}

// Budget implements Store.
func (s *Memory) Budget(_ context.Context, workspace string) (Budget, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.budgets[workspace]
	return b, ok, nil
}

// SetBudget implements Store.
func (s *Memory) SetBudget(_ context.Context, workspace string, b Budget) error {
	if err := b.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.budgets[workspace] = b
	return nil
}

// DeleteBudget implements Store.
func (s *Memory) DeleteBudget(_ context.Context, workspace string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.budgets, workspace)
	return nil
}

// SetBudget stores workspace's budget, stamped with who set it and when.
func (m *Meter) SetBudget(ctx context.Context, workspace, by string, b Budget) (Budget, error) {
	if err := b.Validate(); err != nil {
		return Budget{}, err
	}
	b.UpdatedBy, b.UpdatedAt = by, m.now().UTC()
	if err := m.store.SetBudget(ctx, workspace, b); err != nil {
		return Budget{}, err
	}
	return b, nil
}

// DeleteBudget removes workspace's budget; its use is still counted.
func (m *Meter) DeleteBudget(ctx context.Context, workspace string) error {
	return m.store.DeleteBudget(ctx, workspace)
}
