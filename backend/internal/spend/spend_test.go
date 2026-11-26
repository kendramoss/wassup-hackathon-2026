package spend

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// scriptedStore fails the next len(errs) writes, then behaves like Memory.
type scriptedStore struct {
	*Memory
	mu   sync.Mutex
	errs []error
}

func (s *scriptedStore) Add(ctx context.Context, day string, usd float64) (float64, error) {
	s.mu.Lock()
	var err error
	if len(s.errs) > 0 {
		err, s.errs = s.errs[0], s.errs[1:]
	}
	s.mu.Unlock()
	if err != nil {
		return 0, err
	}
	return s.Memory.Add(ctx, day, usd)
}

func TestMeterCapsTheDayAndResets(t *testing.T) {
	s := NewMemory()
	m := New(s, 0.05)
	day := time.Date(2026, 10, 17, 23, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return day }
	ctx := context.Background()
	// gemini-4-argon promo: $2 in / $10 out per 1M; 10k in + 1k out = $0.03.
	f := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) {
		return llm.Response{Text: "{}", Usage: llm.Usage{InputTokens: 10000, OutputTokens: 1000}}, nil
	}}
	model := m.Attempts(f)
	model.Generate(ctx, llm.Request{Model: "gemini-4-argon"})
	if over, total := m.Over(ctx); over || total != 0.03 {
		t.Fatalf("after one call: over %v, total %v", over, total)
	}
	model.Generate(ctx, llm.Request{Model: "gemini-4-argon"})
	if over, _ := m.Over(ctx); !over {
		t.Fatal("over the cap but not refused")
	}
	if got := m.UntilReset(); got != time.Hour {
		t.Errorf("until reset = %v, want 1h", got)
	}
	// Another instance sharing the store sees the same total.
	other := New(s, 0.05)
	other.now = m.now
	if over, _ := other.Over(ctx); !over {
		t.Error("a second instance does not see the shared total")
	}
	day = day.Add(2 * time.Hour) // next UTC day
	if over, total := m.Over(ctx); over || total != 0 {
		t.Errorf("new day: over %v, total %v", over, total)
	}
}

func TestMeterKeepsCountedSpendAfterStoreRecovery(t *testing.T) {
	s := &scriptedStore{Memory: NewMemory(), errs: []error{errors.New("store down")}}
	m := New(s, 0.05)
	ctx := context.Background()
	m.Add(ctx, 0.06) // write fails: counted locally, invisible to the store
	if over, total := m.Over(ctx); !over || total < 0.06 {
		t.Fatalf("while down: over %v, total %v", over, total)
	}
	m.Add(ctx, 0.01) // store has recovered but only knows this 0.01
	if over, total := m.Over(ctx); !over || total+1e-9 < 0.07 {
		t.Fatalf("after recovery: over %v, total %v, want >= 0.07", over, total)
	}
}

func TestMeterConcurrentFailureAndSuccessAreMonotonic(t *testing.T) {
	s := &scriptedStore{Memory: NewMemory(), errs: []error{errors.New("store down")}}
	m := New(s, 0.05)
	ctx := context.Background()
	var wg sync.WaitGroup
	for _, usd := range []float64{0.06, 0.01} {
		wg.Add(1)
		go func(usd float64) {
			defer wg.Done()
			m.Add(ctx, usd) // exactly one write fails, whichever arrives first
		}(usd)
	}
	wg.Wait()
	if over, total := m.Over(ctx); !over || total+1e-9 < 0.07 {
		t.Fatalf("over %v, total %v, want >= 0.07", over, total)
	}
}

func TestMeterDebtClearsOnNewDay(t *testing.T) {
	s := &scriptedStore{Memory: NewMemory(), errs: []error{errors.New("store down")}}
	m := New(s, 0.05)
	day := time.Date(2026, 10, 17, 23, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return day }
	ctx := context.Background()
	m.Add(ctx, 0.06)
	day = day.Add(2 * time.Hour)
	if over, total := m.Over(ctx); over || total != 0 {
		t.Fatalf("new day: over %v, total %v", over, total)
	}
	m.Add(ctx, 0.01)
	if over, total := m.Over(ctx); over || total != 0.01 {
		t.Fatalf("new day after recovery: over %v, total %v, want 0.01", over, total)
	}
}

func TestNoCapNeverRefuses(t *testing.T) {
	m := New(NewMemory(), 0)
	m.Add(context.Background(), 1000)
	if over, _ := m.Over(context.Background()); over {
		t.Error("cap 0 refused")
	}
}

// failTotals is a store whose writes and reads fail.
type failTotals struct{}

func (failTotals) Add(context.Context, string, float64) (float64, error) {
	return 0, errors.New("down")
}
func (failTotals) Total(context.Context, string) (float64, error) { return 0, errors.New("down") }

// The per-client share (D137): spend made under WithClient counts toward
// that client's share, kept in the store under a hashed key, with this
// instance's view as the fallback when the store fails.
func TestClientShare(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name     string
		store    Store
		capUSD   float64
		share    float64
		spendA   float64
		wantOver bool
	}{
		{"under the share", NewMemory(), 1, 0.1, 0.05, false},
		{"at the share", NewMemory(), 1, 0.1, 0.1, true},
		{"no share configured", NewMemory(), 1, 0, 0.5, false},
		{"no cap configured", NewMemory(), 0, 0.1, 0.5, false},
		{"store down: this instance's view", failTotals{}, 1, 0.1, 0.2, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(tt.store, tt.capUSD).WithClientShare(tt.share)
			m.Add(WithClient(ctx, "203.0.113.1"), tt.spendA)
			over, spent, _ := m.ClientOver(ctx, "203.0.113.1")
			if over != tt.wantOver {
				t.Errorf("over = %t (spent %v), want %t", over, spent, tt.wantOver)
			}
			if over, _, _ := m.ClientOver(ctx, "203.0.113.2"); over {
				t.Error("another client is over")
			}
		})
	}
	if k := ClientDay("2026-11-21", "203.0.113.1"); !strings.HasPrefix(k, "2026-11-21_c_") || strings.Contains(k, "203.0.113.1") || strings.Contains(k, "/") {
		t.Errorf("ClientDay = %q: the address must be hashed and the key a valid document ID", k)
	}
}

// Memory keeps only the newest day's totals.
func TestMemoryForgetsEndedDays(t *testing.T) {
	ctx := context.Background()
	s := NewMemory()
	_, _ = s.Add(ctx, "2026-11-20", 1)
	_, _ = s.Add(ctx, ClientDay("2026-11-20", "a"), 1)
	_, _ = s.Add(ctx, "2026-11-21", 2)
	if len(s.totals) != 1 {
		t.Errorf("totals = %v, want only 2026-11-21", s.totals)
	}
	if got, _ := s.Total(ctx, "2026-11-21"); got != 2 {
		t.Errorf("today = %v", got)
	}
}

// clientFaultStore is Memory with switchable failures: failClientWrite fails
// only per-client keys (ClientDay's "_c_"), failWrites fails every Add and
// failReads fails every Total.
type clientFaultStore struct {
	*Memory
	mu              sync.Mutex
	failClientWrite bool
	failWrites      bool
	failReads       bool
}

func (f *clientFaultStore) set(clientWrite, writes, reads bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failClientWrite, f.failWrites, f.failReads = clientWrite, writes, reads
}

func (f *clientFaultStore) Add(ctx context.Context, key string, usd float64) (float64, error) {
	f.mu.Lock()
	fail := f.failWrites || (f.failClientWrite && strings.Contains(key, "_c_"))
	f.mu.Unlock()
	if fail {
		return 0, errors.New("injected write failure")
	}
	return f.Memory.Add(ctx, key, usd)
}

func (f *clientFaultStore) Total(ctx context.Context, key string) (float64, error) {
	f.mu.Lock()
	fail := f.failReads
	f.mu.Unlock()
	if fail {
		return 0, errors.New("injected read failure")
	}
	return f.Memory.Total(ctx, key)
}

// Issue #713: a client's counted spend is the store total plus whatever the
// store never took, like the global meter's debt (#313). Daily cap 10, share
// 0.10 (limit 1.00): 0.50 ok, 0.30 fails, 0.10 ok, 0.30 ok is 1.20 in truth.
func TestClientShareKeepsSpendAfterFailedWrite(t *testing.T) {
	modes := []struct {
		name string
		set  func(f *clientFaultStore, fail bool)
	}{
		{"client write fails", func(f *clientFaultStore, fail bool) { f.set(fail, false, false) }},
		{"whole store write fails", func(f *clientFaultStore, fail bool) { f.set(false, fail, false) }},
		{"whole store read and write fail", func(f *clientFaultStore, fail bool) { f.set(false, fail, fail) }},
	}
	steps := []struct {
		fail     bool
		usd      float64
		wantOver bool
	}{{false, 0.5, false}, {true, 0.3, false}, {false, 0.1, false}, {false, 0.3, true}}
	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			f := &clientFaultStore{Memory: NewMemory()}
			m := New(f, 10).WithClientShare(0.10)
			ctx := WithClient(context.Background(), "203.0.113.1")
			var truth float64
			for i, s := range steps {
				mode.set(f, s.fail)
				m.Add(ctx, s.usd)
				mode.set(f, false)
				truth += s.usd
				over, spent, limit := m.ClientOver(context.Background(), "203.0.113.1")
				if spent+1e-9 < truth {
					t.Errorf("step %d: counted %.2f, charged %.2f", i+1, spent, truth)
				}
				if over != s.wantOver {
					t.Errorf("step %d: over %v (counted %.2f of %.2f), want %v", i+1, over, spent, limit, s.wantOver)
				}
			}
			// The global meter still equals true spend.
			if _, total := m.Over(context.Background()); total+1e-9 < truth {
				t.Errorf("global meter %.2f, true spend %.2f", total, truth)
			}
		})
	}
}

// The failed amount stays counted while the store is still unreadable, and a
// read failure alone (writes landing) loses nothing either.
func TestClientShareReadFailureKeepsDebt(t *testing.T) {
	f := &clientFaultStore{Memory: NewMemory()}
	m := New(f, 10).WithClientShare(0.10)
	ctx := WithClient(context.Background(), "c")
	f.set(true, false, false)
	m.Add(ctx, 0.6) // client write fails
	f.set(false, false, false)
	m.Add(ctx, 0.2) // store knows only 0.2
	f.set(false, false, true)
	over, spent, _ := m.ClientOver(context.Background(), "c")
	if spent+1e-9 < 0.8 || over {
		t.Fatalf("read failure: over %v spent %.2f, want 0.80 and under", over, spent)
	}
	m.Add(ctx, 0.3) // landed in store; reads still fail
	if over, spent, _ := m.ClientOver(context.Background(), "c"); !over || spent+1e-9 < 1.1 {
		t.Fatalf("read failure after more spend: over %v spent %.2f, want >= 1.10 and over", over, spent)
	}
}

func TestClientShareNoFaultIsExact(t *testing.T) {
	m := New(NewMemory(), 10).WithClientShare(0.10)
	ctx := WithClient(context.Background(), "c")
	for _, usd := range []float64{0.5, 0.3, 0.1} {
		m.Add(ctx, usd)
	}
	over, spent, _ := m.ClientOver(context.Background(), "c")
	if over || spent < 0.9-1e-9 || spent > 0.9+1e-9 {
		t.Fatalf("no faults: over %v spent %v, want exactly 0.90 and under", over, spent)
	}
	m.Add(ctx, 0.3)
	if over, spent, _ := m.ClientOver(context.Background(), "c"); !over || spent < 1.2-1e-9 || spent > 1.2+1e-9 {
		t.Fatalf("no faults: over %v spent %v, want exactly 1.20 and over", over, spent)
	}
}

// Debt is per client: one client's failed write does not count against another.
func TestClientShareDebtIsPerClient(t *testing.T) {
	f := &clientFaultStore{Memory: NewMemory()}
	m := New(f, 10).WithClientShare(0.10)
	f.set(true, false, false)
	m.Add(WithClient(context.Background(), "a"), 0.9)
	f.set(false, false, false)
	m.Add(WithClient(context.Background(), "b"), 0.5)
	m.Add(WithClient(context.Background(), "a"), 0.2)
	if over, spent, _ := m.ClientOver(context.Background(), "b"); over || spent > 0.5+1e-9 {
		t.Errorf("client b: over %v spent %v, want 0.50 and under", over, spent)
	}
	if over, _, _ := m.ClientOver(context.Background(), "a"); !over {
		t.Error("client a (0.9 failed + 0.2) should be over")
	}
}

// The client debt resets on the UTC day roll, like the client view.
func TestClientShareDebtClearsOnNewDay(t *testing.T) {
	f := &clientFaultStore{Memory: NewMemory()}
	m := New(f, 10).WithClientShare(0.10)
	day := time.Date(2026, 10, 17, 23, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return day }
	ctx := WithClient(context.Background(), "c")
	f.set(true, false, false)
	m.Add(ctx, 0.9)
	f.set(false, false, false)
	day = day.Add(2 * time.Hour)
	if over, spent, _ := m.ClientOver(context.Background(), "c"); over || spent != 0 {
		t.Fatalf("new day: over %v spent %v, want 0", over, spent)
	}
	m.Add(ctx, 0.2)
	if over, spent, _ := m.ClientOver(context.Background(), "c"); over || spent < 0.2-1e-9 || spent > 0.2+1e-9 {
		t.Fatalf("new day after recovery: over %v spent %v, want 0.20 and under", over, spent)
	}
}

// Both per-client maps stay within maxLocalClients.
func TestClientShareDebtIsBoundedByClientMap(t *testing.T) {
	f := &clientFaultStore{Memory: NewMemory()}
	m := New(f, 1e9).WithClientShare(0.5)
	f.set(true, false, false)
	for i := 0; i < maxLocalClients+25; i++ {
		m.Add(WithClient(context.Background(), "client-"+strconv.Itoa(i)), 0.01)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.clients) != maxLocalClients || len(m.clientDebt) > maxLocalClients {
		t.Fatalf("clients %d, debt %d, want <= %d", len(m.clients), len(m.clientDebt), maxLocalClients)
	}
}

// Concurrent writers with a flapping store: after the store recovers, the
// counted spend is at least what the store holds plus every failed amount.
func TestClientShareConcurrentFlappingKeepsFailedWrites(t *testing.T) {
	const n = 50
	f := &clientFaultStore{Memory: NewMemory()}
	m := New(f, 1000).WithClientShare(0.5)
	ctx := WithClient(context.Background(), "c")
	failing := &flakyClientStore{clientFaultStore: f}
	m.store = failing
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			m.Add(ctx, 0.05)
		}(i)
	}
	wg.Wait()
	_, spent, _ := m.ClientOver(context.Background(), "c")
	want := 0.05 * n
	if spent+1e-9 < want {
		t.Fatalf("counted %.2f, charged %.2f (%d client writes failed)", spent, want, failing.failed.Load())
	}
}

// flakyClientStore fails every 7th per-client write, deterministically by
// call count, so concurrent runs always include failures.
type flakyClientStore struct {
	*clientFaultStore
	calls  atomic.Int64
	failed atomic.Int64
}

func (s *flakyClientStore) Add(ctx context.Context, key string, usd float64) (float64, error) {
	if strings.Contains(key, "_c_") && s.calls.Add(1)%7 == 0 {
		s.failed.Add(1)
		return 0, errors.New("injected write failure")
	}
	return s.Memory.Add(ctx, key, usd)
}

func TestObserveSeesEveryCharge(t *testing.T) {
	type k struct{}
	var got []float64
	var seen []any
	m := New(NewMemory(), 0).Observe(func(ctx context.Context, usd float64) {
		got = append(got, usd)
		seen = append(seen, ctx.Value(k{}))
	})
	ctx := context.WithValue(context.Background(), k{}, "ws")
	m.Add(ctx, 0.5)
	m.Add(ctx, 0) // nothing spent: not observed
	m.Add(ctx, 0.25)
	if len(got) != 2 || got[0] != 0.5 || got[1] != 0.25 || seen[0] != "ws" {
		t.Fatalf("observed %v %v", got, seen)
	}
}
