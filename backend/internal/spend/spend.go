// Package spend caps the day's model spend (D59). Every model call's
// estimated cost (evalkit.PromoPrices, the same as the history's cost_usd)
// is added to a running total for the UTC day, shared between instances
// through a Store. Once the total reaches the cap, Over reports true and the
// API refuses new model-calling requests until the next UTC day; cached
// replays and reads keep working.
package spend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
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

	// share is the fraction of cap one client may spend in a day (D137);
	// 0 disables the per-client cap.
	share float64

	mu    sync.Mutex
	day   string
	total float64 // last known total for day (this instance's view)
	debt  float64 // spend counted locally after a failed store write; the store never saw it
	// clients is this instance's view of each client's spend for day: what
	// the store last said, plus anything the store did not take. Reset each
	// day and capped in size.
	clients map[string]float64
	// clientDebt is each client's spend counted locally after a failed
	// store write, like debt for the global meter (#313). It only has keys
	// that are in clients, so the same cap bounds it, and it resets with it.
	clientDebt map[string]float64

	// observe, when set, is told every charge with its request context, so
	// per-workspace metering (#886) sees exactly what the cap counts.
	observe func(ctx context.Context, usd float64)
}

// maxLocalClients caps the per-client spend this instance remembers in a day.
const maxLocalClients = 10000

// New returns a meter; capUSD <= 0 disables the cap (Over is always false).
func New(store Store, capUSD float64) *Meter {
	return &Meter{store: store, cap: capUSD, now: time.Now}
}

// WithClientShare sets the share of the cap one client may spend in a UTC
// day (D137), so one caller cannot use up the day for everyone. share <= 0
// (or no cap) disables it. It returns m.
func (m *Meter) WithClientShare(share float64) *Meter {
	m.share = share
	return m
}

type clientCtx struct{}

// Observe sets a function told every charge Add records, with the
// context of the model call. It returns m.
func (m *Meter) Observe(fn func(ctx context.Context, usd float64)) *Meter {
	m.observe = fn
	return m
}

// WithClient marks ctx's model calls as made for client (an API key or an
// address), so they count toward its share of the cap.
func WithClient(ctx context.Context, client string) context.Context {
	return context.WithValue(ctx, clientCtx{}, client)
}

func clientFrom(ctx context.Context) string {
	c, _ := ctx.Value(clientCtx{}).(string)
	return c
}

// ClientDay is the store key for client's spend on day: the day, then a
// hash of the client, so the store never holds an address and the key is a
// valid document ID.
func ClientDay(day, client string) string {
	h := sha256.Sum256([]byte(client))
	return day + "_c_" + hex.EncodeToString(h[:12])
}

func (m *Meter) clientCap() float64 {
	if m.cap <= 0 || m.share <= 0 {
		return 0
	}
	return m.cap * m.share
}

// resetDayLocked starts a new day's view. m.mu must be held.
func (m *Meter) resetDayLocked(day string) {
	if m.day != day {
		m.day, m.total, m.debt, m.clients, m.clientDebt = day, 0, 0, nil, nil
	}
}

// addClient adds usd to client's spend for day (best effort: a store error
// keeps the amount in this instance's view only).
func (m *Meter) addClient(ctx context.Context, day, client string, usd float64) {
	if m.clientCap() <= 0 || client == "" {
		return
	}
	total, err := m.store.Add(context.WithoutCancel(ctx), ClientDay(day, client), usd)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resetDayLocked(day)
	if m.clients == nil {
		m.clients = map[string]float64{}
	}
	if _, ok := m.clients[client]; !ok && len(m.clients) >= maxLocalClients {
		return
	}
	if err == nil {
		// The store's total covers every write that reached it; keep this
		// client's unsent debt on top, and never lower the view.
		m.clients[client] = max(total+m.clientDebt[client], m.clients[client])
	} else {
		m.clients[client] += usd
		if m.clientDebt == nil {
			m.clientDebt = map[string]float64{}
		}
		m.clientDebt[client] += usd
	}
}

// ClientOver reports whether client has spent its share of today's cap,
// with what it spent and its share in USD. With no per-client cap it is
// always false. A store error falls back to this instance's view.
func (m *Meter) ClientOver(ctx context.Context, client string) (bool, float64, float64) {
	limit := m.clientCap()
	if limit <= 0 || client == "" {
		return false, 0, 0
	}
	day := m.today()
	total, err := m.store.Total(ctx, ClientDay(day, client))
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resetDayLocked(day)
	spent := m.clients[client]
	if err == nil {
		spent = max(total+m.clientDebt[client], spent)
	}
	return spent >= limit, spent, limit
}

func (m *Meter) today() string { return m.now().UTC().Format(time.DateOnly) }

// Add records usd spent now.
func (m *Meter) Add(ctx context.Context, usd float64) {
	if usd <= 0 {
		return
	}
	if m.observe != nil {
		m.observe(ctx, usd)
	}
	day := m.today()
	m.addClient(ctx, day, clientFrom(ctx), usd)
	total, err := m.store.Add(context.WithoutCancel(ctx), day, usd)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resetDayLocked(day)
	if err == nil {
		// The store's total covers every write that reached it. Our debt is
		// still missing from that number, so keep it on top. max also keeps
		// spend counted by an earlier failed write whose retry landed after
		// this one, so recovery can never lower the meter.
		m.total = max(total+m.debt, m.total)
	} else {
		m.total += usd
		m.debt += usd // store unavailable: keep counting locally
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
	m.resetDayLocked(day)
	if err == nil {
		m.total = max(total+m.debt, m.total)
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

// Attempts wraps a raw provider client and adds each call's cost to the meter.
// Install it directly over each raw provider client, below pace, retry and
// fallback, so every provider call is charged once at its own model's price
// (even when it returns an unusable answer or fails after reporting usage),
// and do not wrap the result again. Wrapping an already metered client is a
// no-op, and a nil meter leaves inner unchanged.
func (m *Meter) Attempts(inner llm.Model) llm.Model {
	if m == nil {
		return inner
	}
	if x, ok := inner.(*metered); ok && x.m == m {
		return inner
	}
	return &metered{inner: inner, m: m}
}

type metered struct {
	inner llm.Model
	m     *Meter
}

func (x *metered) Provider() string { return x.inner.Provider() }

func (x *metered) Generate(ctx context.Context, req llm.Request) (llm.Response, error) {
	resp, err := x.inner.Generate(ctx, req)
	// Zero usage (a transport error, or no usageMetadata) costs 0, so Add
	// ignores it; an unpriced model also costs 0 (evalkit.Cost).
	model := resp.ModelID
	if model == "" {
		model = req.Model
	}
	x.m.Add(ctx, evalkit.Cost(evalkit.PromoPrices, model, resp.Usage.InputTokens, resp.Usage.OutputTokens))
	return resp, err
}

// Memory is an in-process Store (local runs and tests). It keeps only the
// newest day's totals (the day's own and each client's, see ClientDay).
type Memory struct {
	mu     sync.Mutex
	totals map[string]float64
	day    string
}

// NewMemory returns an empty store.
func NewMemory() *Memory { return &Memory{totals: map[string]float64{}} }

// Add implements Store.
func (s *Memory) Add(_ context.Context, day string, usd float64) (float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, _, _ := strings.Cut(day, "_"); d > s.day { // a new day: forget the ended ones
		s.day = d
		for k := range s.totals {
			if kd, _, _ := strings.Cut(k, "_"); kd < d {
				delete(s.totals, k)
			}
		}
	}
	s.totals[day] += usd
	return s.totals[day], nil
}

// Total implements Store.
func (s *Memory) Total(_ context.Context, day string) (float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.totals[day], nil
}
