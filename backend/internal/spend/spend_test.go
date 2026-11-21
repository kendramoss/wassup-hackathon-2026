package spend

import (
	"context"
	"errors"
	"strings"
	"sync"
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
