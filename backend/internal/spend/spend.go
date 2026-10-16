// Package spend caps the day's model spend (D59). Every model call's
// estimated cost (evalkit.PromoPrices, the same as the history's cost_usd)
// is added to a running total for the UTC day, shared between instances
// through a Store. Once the total reaches the cap, Over reports true and the
// API refuses new model-calling requests until the next UTC day; cached
// replays and reads keep working.
package spend

import (
	"context"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// Store keeps the daily totals: Add returns the new total for day.
type Store interface {
	Add(ctx context.Context, day string, usd float64) (float64, error)
	Total(ctx context.Context, day string) (float64, error)
}

// Meter tracks the day's spend against a cap.
type Meter struct {
	store Store
	cap   float64
	now   func() time.Time

	mu    sync.Mutex
	day   string
	total float64 // last known total for day (this instance's view)
}

// New returns a meter; capUSD <= 0 disables the cap (Over is always false).
func New(store Store, capUSD float64) *Meter {
	return &Meter{store: store, cap: capUSD, now: time.Now}
}

func (m *Meter) today() string { return m.now().UTC().Format(time.DateOnly) }

// Add records usd spent now.
func (m *Meter) Add(ctx context.Context, usd float64) {
	if usd <= 0 {
		return
	}
	day := m.today()
	total, err := m.store.Add(context.WithoutCancel(ctx), day, usd)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.day != day {
		m.day, m.total = day, 0
	}
	if err == nil {
		m.total = total
	} else {
		m.total += usd // store unavailable: keep counting locally
	}
}

// Over reports whether today's spend has reached the cap. It refreshes from
// the store, so spend on other instances counts.
func (m *Meter) Over(ctx context.Context) (bool, float64) {
	if m.cap <= 0 {
		return false, 0
	}
	day := m.today()
	total, err := m.store.Total(ctx, day)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.day != day {
		m.day, m.total = day, 0
	}
	if err == nil && total > m.total {
		m.total = total
	}
	return m.total >= m.cap, m.total
}

// Cap is the configured daily cap in USD (0: none).
func (m *Meter) Cap() float64 { return m.cap }

// UntilReset is the time until the next UTC day.
func (m *Meter) UntilReset() time.Duration {
	now := m.now().UTC()
	return now.Truncate(24 * time.Hour).Add(24 * time.Hour).Sub(now)
}

// Model wraps an llm.Model and adds each call's cost to the meter.
func (m *Meter) Model(inner llm.Model) llm.Model { return &metered{inner: inner, m: m} }

type metered struct {
	inner llm.Model
	m     *Meter
}

func (x *metered) Provider() string { return x.inner.Provider() }

func (x *metered) Generate(ctx context.Context, req llm.Request) (llm.Response, error) {
	resp, err := x.inner.Generate(ctx, req)
	model := resp.ModelID
	if model == "" {
		model = req.Model
	}
	x.m.Add(ctx, evalkit.Cost(evalkit.PromoPrices, model, resp.Usage.InputTokens, resp.Usage.OutputTokens))
	return resp, err
}

// Memory is an in-process Store (local runs and tests).
type Memory struct {
	mu     sync.Mutex
	totals map[string]float64
}

// NewMemory returns an empty store.
func NewMemory() *Memory { return &Memory{totals: map[string]float64{}} }

// Add implements Store.
func (s *Memory) Add(_ context.Context, day string, usd float64) (float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.totals[day] += usd
	return s.totals[day], nil
}

// Total implements Store.
func (s *Memory) Total(_ context.Context, day string) (float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.totals[day], nil
}
