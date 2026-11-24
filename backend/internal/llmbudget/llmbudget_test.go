package llmbudget

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

var rates = Rates{InputPerMTok: 1, OutputPerMTok: 10, Source: "test quote"}

func open(t *testing.T, capUSD float64) *Ledger {
	t.Helper()
	l, err := Open(filepath.Join(t.TempDir(), "ledger.jsonl"), capUSD)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestCapBounds(t *testing.T) {
	for _, c := range []float64{0, -1, 50.01, 1000} {
		if _, err := Open(filepath.Join(t.TempDir(), "l"), c); err == nil {
			t.Errorf("cap %v accepted", c)
		}
	}
	if _, err := Open(filepath.Join(t.TempDir(), "l"), MaxCapUSD); err != nil {
		t.Errorf("cap %v refused: %v", MaxCapUSD, err)
	}
}

func TestRatesValidate(t *testing.T) {
	for _, r := range []Rates{{}, {InputPerMTok: 1, OutputPerMTok: 0, Source: "x"}, {InputPerMTok: -1, OutputPerMTok: 1, Source: "x"}, {InputPerMTok: 1, OutputPerMTok: 1}, {InputPerMTok: 1, OutputPerMTok: 1, Source: "  "}} {
		if r.Validate() == nil {
			t.Errorf("%+v accepted", r)
		}
	}
	if err := rates.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestReserveSettleAndCap(t *testing.T) {
	l := open(t, 10)
	a, err := l.Reserve("a", 6)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Reserve("b", 5); !errors.Is(err, ErrBudget) {
		t.Fatalf("second reservation = %v, want ErrBudget", err)
	}
	if err := a.Settle(1, Reported); err != nil {
		t.Fatal(err)
	}
	if err := a.Settle(9, Reported); err != nil { // second settle has no effect
		t.Fatal(err)
	}
	if c, _ := l.Committed(); c != 1 {
		t.Fatalf("committed %v, want 1", c)
	}
	if _, err := l.Reserve("b", 9); err != nil {
		t.Fatalf("reservation after settlement: %v", err)
	}
	if c, _ := l.Committed(); c != 10 {
		t.Fatalf("committed %v, want 10 (unsettled reservations count in full)", c)
	}
}

func TestConcurrentReservationsAcrossLedgersNeverPassTheCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	var ok atomic.Int64
	var wg sync.WaitGroup
	for p := 0; p < 3; p++ { // three "processes" sharing one file
		l, err := Open(path, 5)
		if err != nil {
			t.Fatal(err)
		}
		for w := 0; w < 8; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 10; i++ {
					if _, err := l.Reserve("x", 0.3); err == nil {
						ok.Add(1)
					}
				}
			}()
		}
	}
	wg.Wait()
	l, _ := Open(path, 5)
	c, _ := l.Committed()
	if c > 5+1e-9 || ok.Load() != 16 {
		t.Fatalf("granted %d reservations, committed %v; want 16 and at most 5", ok.Load(), c)
	}
}

func TestGarbledLedgerFailsClosed(t *testing.T) {
	l := open(t, 5)
	if err := os.WriteFile(l.path, []byte("not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Reserve("x", 0.01); err == nil || errors.Is(err, ErrBudget) {
		t.Fatalf("garbled ledger error = %v", err)
	}
	if err := os.WriteFile(l.path, []byte(`{"t":"refund","id":"a","usd":1,"at":"x"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Reserve("x", 0.01); err == nil {
		t.Fatal("unknown entry kind accepted")
	}
}

func gateway(t *testing.T, h http.HandlerFunc) (*llm.LiteLLM, *atomic.Int64, string) {
	t.Helper()
	var n atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	g := llm.NewLiteLLM(srv.URL, "sk-secret-key", 2*time.Second)
	g.SkipCache(true)
	return g, &n, srv.URL
}

func wrap(g *llm.LiteLLM, l *Ledger) *Model {
	return &Model{Inner: g, Ledger: l, Rates: rates, Size: func(r llm.Request) (int, error) {
		b, err := g.RequestBody(r)
		return len(b), err
	}}
}

const okBody = `{"model":"m","choices":[{"message":{"role":"assistant","content":"{}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1000,"completion_tokens":500,"total_tokens":1500}}`

func TestSettlementProvenance(t *testing.T) {
	req := llm.Request{Model: "m", MaxOutputTokens: 8192, Messages: []llm.Message{{Role: llm.RoleUser, Content: "hello"}}}
	cases := []struct {
		name    string
		h       http.HandlerFunc
		want    string
		usd     func(worst, input float64) float64
		wantErr bool
	}{
		{"reported cost", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Litellm-Response-Cost", "0.0123")
			w.Write([]byte(okBody))
		}, Reported, func(_, _ float64) float64 { return 0.0123 }, false},
		{"reported zero is still reported", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Litellm-Response-Cost", "0")
			w.Write([]byte(okBody))
		}, Reported, func(_, _ float64) float64 { return 0 }, false},
		{"absent header is estimated from usage, not free", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(okBody))
		}, EstimatedFromUsage, func(_, _ float64) float64 { return (1000*1 + 500*10) / 1e6 }, false},
		{"403 without header", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "denied for key sk-secret-key at "+r.Host, http.StatusForbidden)
		}, UnreportedRejection, func(_, in float64) float64 { return in }, true},
		{"500 keeps the worst case", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}, UnreportedWorstCase, func(w, _ float64) float64 { return w }, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := open(t, 5)
			g, n, _ := gateway(t, c.h)
			m := wrap(g, l)
			worst, size, err := m.WorstCase(req)
			if err != nil {
				t.Fatal(err)
			}
			outer := &llm.CostTally{}
			var cost Cost
			_, err = m.Generate(WithSink(llm.ContextWithCostTally(context.Background(), outer), &cost), req)
			if (err != nil) != c.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, c.wantErr)
			}
			if n.Load() != 1 {
				t.Fatalf("gateway saw %d requests, want exactly 1 (no retries)", n.Load())
			}
			want := c.usd(worst, rates.WorstCase(size, 0))
			if cost.Provenance != c.want || cost.USD < want-1e-12 || cost.USD > want+1e-12 {
				t.Fatalf("cost %+v, want %s %v", cost, c.want, want)
			}
			if got, _ := l.Committed(); got < want-1e-12 || got > want+1e-12 {
				t.Fatalf("ledger committed %v, want %v", got, want)
			}
			if err != nil && (strings.Contains(err.Error(), "sk-secret-key") || strings.Contains(err.Error(), "127.0.0.1")) {
				t.Errorf("error leaks the key or gateway address: %v", err)
			}
			if c.want == Reported && outer.USD() != cost.USD {
				t.Errorf("caller tally %v, want %v", outer.USD(), cost.USD)
			}
		})
	}
}

func TestOverCapCallMakesNoRequest(t *testing.T) {
	g, n, _ := gateway(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(okBody)) })
	l := open(t, 0.0001)
	_, err := wrap(g, l).Generate(context.Background(), llm.Request{Model: "m", MaxOutputTokens: 8192, Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}}})
	if !errors.Is(err, ErrBudget) || n.Load() != 0 {
		t.Fatalf("err %v, requests %d; want ErrBudget and none", err, n.Load())
	}
}

func TestRequestWithoutOutputCapIsRefused(t *testing.T) {
	g, n, _ := gateway(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(okBody)) })
	_, err := wrap(g, open(t, 5)).Generate(context.Background(), llm.Request{Model: "m", Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}}})
	if err == nil || n.Load() != 0 {
		t.Fatalf("err %v, requests %d", err, n.Load())
	}
}
