package httpapi

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/ratelimit"
)

// countingStore wraps a store and counts the writes that reach it.
type countingStore struct {
	ratelimit.Store
	mu    sync.Mutex
	calls int
}

func (c *countingStore) Add(ctx context.Context, cs []ratelimit.Counter, exp time.Time) ([]int64, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return c.Store.Add(ctx, cs, exp)
}

func TestSharedLimiterTwoInstancesOneLimit(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 10, 0, time.UTC)
	clock := func() time.Time { return now }
	tests := []struct {
		name    string
		lim     Limits
		class   class
		allowed int // requests allowed in one window across both instances
	}{
		{"light per client", Limits{LightPerMin: 3, HeavyPerMin: 1, GlobalPerMin: 1000}, light, 3},
		{"heavy per client", Limits{LightPerMin: 100, HeavyPerMin: 2, GlobalPerMin: 1000}, heavy, 2},
		{"lookup per client", Limits{LookupPerMin: 4, GlobalPerMin: 20}, lookup, 4},                           // lookups skip the global cap
		{"global cap in heavy units", Limits{LightPerMin: 100, HeavyPerMin: 100, GlobalPerMin: 22}, heavy, 4}, // 4×5 = 20 ≤ 22
		{"bulk takes the heavy burst", Limits{LightPerMin: 100, HeavyPerMin: 4, GlobalPerMin: 1000}, bulk, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := ratelimit.NewMemory()
			a, b := newSharedLimiter(tt.lim, store), newSharedLimiter(tt.lim, store)
			a.now, b.now = clock, clock
			got := 0
			for i := 0; i < 10; i++ {
				l := a
				if i%2 == 1 {
					l = b
				}
				ok, wait, err := l.allow(context.Background(), "203.0.113.1", tt.class)
				if err != nil {
					t.Fatal(err)
				}
				if ok {
					got++
				} else if wait != 50*time.Second {
					t.Errorf("wait = %v, want the 50 s left in the window", wait)
				}
			}
			if got != tt.allowed {
				t.Errorf("allowed %d across two instances, want %d", got, tt.allowed)
			}
		})
	}
}

func TestSharedLimiterRefusalsTakeNothing(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	store := ratelimit.NewMemory()
	l := newSharedLimiter(Limits{LightPerMin: 100, HeavyPerMin: 100, GlobalPerMin: 7}, store)
	l.now = func() time.Time { return now }
	ctx := context.Background()
	if ok, _, _ := l.allow(ctx, "a", heavy); !ok { // takes 5 of 7
		t.Fatal("first heavy refused")
	}
	if ok, _, _ := l.allow(ctx, "b", heavy); ok {
		t.Fatal("second heavy allowed with 2 of 7 left")
	}
	if ok, _, _ := l.allow(ctx, "b", light); !ok {
		t.Fatal("light refused: the refused heavy request kept its 5")
	}
	if ok, _, _ := l.allow(ctx, "b", light); !ok {
		t.Fatal("light refused at 7 of 7")
	}
	if ok, _, _ := l.allow(ctx, "c", light); ok {
		t.Fatal("over the global cap")
	}
}

func TestSharedLimiterWindowRollover(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 59, 0, time.UTC)
	store := &countingStore{Store: ratelimit.NewMemory()}
	l := newSharedLimiter(Limits{LightPerMin: 2, HeavyPerMin: 1, GlobalPerMin: 100}, store)
	l.now = func() time.Time { return now }
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if ok, _, _ := l.allow(ctx, "a", light); !ok {
			t.Fatalf("request %d refused", i)
		}
	}
	ok, wait, _ := l.allow(ctx, "a", light)
	if ok || wait != time.Second {
		t.Fatalf("third = %v, wait %v; want refused until the minute ends", ok, wait)
	}
	calls := store.calls
	if ok, _, _ := l.allow(ctx, "a", light); ok || store.calls != calls {
		t.Errorf("a full client cost %d store writes; want it refused locally", store.calls-calls)
	}
	now = now.Add(time.Second) // 12:01:00, a new window
	if ok, _, _ := l.allow(ctx, "a", light); !ok {
		t.Fatal("refused after the window rolled over")
	}
}

// Fail open: when the shared store is down, requests fall back to this
// instance's own buckets (still limited, not refused outright), and the
// outage is logged once.
func TestSharedLimiterFailsOpenToLocalLimits(t *testing.T) {
	store := ratelimit.NewMemory()
	var logs bytes.Buffer
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		d.Limits = Limits{LightPerMin: 2, HeavyPerMin: 1, GlobalPerMin: 100}
		d.RateStore = store
		d.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	})
	body := heatGunBody(t)
	alice := map[string]string{"X-Client-IP": "203.0.113.1"}
	if code, _ := hs.doAs(t, "POST", "/v1/cases/predict", body, alice); code != http.StatusCreated {
		t.Fatalf("predict with the store up = %d", code)
	}
	store.Fail(ratelimit.ErrUnavailable)
	codes := []int{}
	for i := 0; i < 3; i++ {
		code, _ := hs.doAs(t, "POST", "/v1/cases/predict", body, alice)
		codes = append(codes, code)
	}
	if codes[0] != http.StatusCreated || codes[1] != http.StatusCreated || codes[2] != http.StatusTooManyRequests {
		t.Errorf("with the store down = %v, want the local limit of 2 (201 201 429)", codes)
	}
	if n := strings.Count(logs.String(), "shared rate limiter unavailable"); n != 1 {
		t.Errorf("logged the outage %d times, want once", n)
	}
	if code, _ := hs.doAs(t, "POST", "/v1/cases/predict", body, map[string]string{"X-Client-IP": "203.0.113.1", "X-Admin-Token": testAdmin}); code != http.StatusCreated {
		t.Errorf("admin limited = %d", code)
	}
}

// Two servers (two Cloud Run instances) with one store: the client limit
// holds across both, with 429 and Retry-After as before, and IPv6 clients
// are still grouped by /64.
func TestSharedLimiterAcrossServers(t *testing.T) {
	store := ratelimit.NewMemory()
	adjust := func(d *Deps) {
		d.Limits = Limits{LightPerMin: 2, HeavyPerMin: 1, GlobalPerMin: 100}
		d.RateStore = store
	}
	one := newHarnessWith(t, "substantial_transformation", adjust)
	two := newHarnessWith(t, "substantial_transformation", adjust)
	body := heatGunBody(t)
	if code, _ := one.doAs(t, "POST", "/v1/cases/predict", body, map[string]string{"X-Client-IP": "2001:db8:1:2::1"}); code != http.StatusCreated {
		t.Fatalf("first = %d", code)
	}
	if code, _ := two.doAs(t, "POST", "/v1/cases/predict", body, map[string]string{"X-Client-IP": "2001:db8:1:2::99"}); code != http.StatusCreated {
		t.Fatalf("second = %d", code)
	}
	req := httptest.NewRequest("POST", "/v1/cases/predict", strings.NewReader(body))
	req.Header.Set("X-Client-IP", "2001:db8:1:2:ffff::7")
	rec := httptest.NewRecorder()
	one.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" || !strings.Contains(rec.Body.String(), `"rate_limited"`) {
		t.Fatalf("third (same /64, either server) = %d %s Retry-After %q", rec.Code, rec.Body, rec.Header().Get("Retry-After"))
	}
	if code, _ := two.doAs(t, "POST", "/v1/cases/predict", body, map[string]string{"X-Client-IP": "203.0.113.9"}); code != http.StatusCreated {
		t.Errorf("another client limited = %d", code)
	}
}
