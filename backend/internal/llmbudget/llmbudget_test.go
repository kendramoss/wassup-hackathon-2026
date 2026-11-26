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
var bind = Binding{Model: "m", Rates: rates}

func open(t *testing.T, capUSD float64) *Ledger {
	t.Helper()
	l, err := Create(filepath.Join(t.TempDir(), "ledger.jsonl"), capUSD, bind, nil)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestCapBounds(t *testing.T) {
	for _, c := range []float64{0, -1, 60.01, 1000} {
		if _, err := Create(filepath.Join(t.TempDir(), "l"), c, bind, nil); err == nil {
			t.Errorf("cap %v accepted", c)
		}
	}
	if _, err := Create(filepath.Join(t.TempDir(), "l"), MaxCapUSD, bind, nil); err != nil {
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
	if _, err := Create(path, 5, bind, nil); err != nil {
		t.Fatal(err)
	}
	for p := 0; p < 3; p++ { // three "processes" sharing one file
		l, err := Open(path, 5, bind)
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
	l, _ := Open(path, 5, bind)
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
		{"reported zero without usage is never free", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Litellm-Response-Cost", "0")
			w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{}"},"finish_reason":"stop"}]}`))
		}, UnreportedWorstCase, func(w, _ float64) float64 { return w }, false},
		{"reported zero with usage is not trusted as free", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Litellm-Response-Cost", "0")
			w.Write([]byte(okBody))
		}, EstimatedFromUsage, func(_, _ float64) float64 { return (1000*1 + 500*10) / 1e6 }, false},
		{"absent header is estimated from usage, not free", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(okBody))
		}, EstimatedFromUsage, func(_, _ float64) float64 { return (1000*1 + 500*10) / 1e6 }, false},
		{"403 without header", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "denied for key sk-secret-key at "+r.Host, http.StatusForbidden)
		}, UnreportedWorstCase, func(w, _ float64) float64 { return w }, true},
		{"400 after generation is not proof of no charge", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "content policy", http.StatusBadRequest)
		}, UnreportedWorstCase, func(w, _ float64) float64 { return w }, true},
		{"200 with zero header and no usage keeps the worst case", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Litellm-Response-Cost", "0")
			w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`))
		}, UnreportedWorstCase, func(w, _ float64) float64 { return w }, false},
		{"truncated body keeps the worst case", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "500")
			w.Write([]byte(`{"choices"`))
		}, UnreportedWorstCase, func(w, _ float64) float64 { return w }, true},
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

func TestOverrunStopsTheLedgerDurably(t *testing.T) {
	l := open(t, 10)
	r, err := l.Reserve("a", 1)
	if err != nil {
		t.Fatal(err)
	}
	other, err := l.Reserve("b", 1) // already in flight when the overrun lands
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Settle(3, Reported); !errors.Is(err, ErrOverrun) {
		t.Fatalf("settle = %v, want ErrOverrun", err)
	}
	if c, _ := l.Committed(); c != 4 { // 3 truly charged + 1 still reserved
		t.Fatalf("committed %v, want the real charge recorded (4)", c)
	}
	if _, err := l.Reserve("c", 0.01); !errors.Is(err, ErrStopped) {
		t.Fatalf("reservation after the overrun = %v, want ErrStopped", err)
	}
	reopened, err := Open(l.path, 10, bind) // a new process sees the same stop
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.Reserve("d", 0.01); !errors.Is(err, ErrStopped) {
		t.Fatalf("reservation after reopening = %v, want ErrStopped", err)
	}
	if err := other.Settle(0.5, Reported); err != nil { // in-flight work can still settle
		t.Fatal(err)
	}
	if on, _ := reopened.Stopped(); !on {
		t.Fatal("stop lost")
	}
}

func TestBindingIsImmutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	if _, err := Open(path, 5, bind); !errors.Is(err, ErrNoLedger) {
		t.Fatalf("Open on a missing file = %v, want ErrNoLedger", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("Open created a file")
	}
	if _, err := Create(path, 5, bind, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(path, 5, bind, nil); err == nil {
		t.Fatal("Create over an existing ledger")
	}
	empty := filepath.Join(t.TempDir(), "empty.jsonl")
	_ = os.WriteFile(empty, nil, 0o600)
	if _, err := Open(empty, 5, bind); !errors.Is(err, ErrNoLedger) {
		t.Fatalf("Open on an empty file = %v, want ErrNoLedger", err)
	}
	for name, change := range map[string]func() (*Ledger, error){
		"higher cap":  func() (*Ledger, error) { return Open(path, 50, bind) },
		"lower cap":   func() (*Ledger, error) { return Open(path, 4, bind) },
		"other model": func() (*Ledger, error) { return Open(path, 5, Binding{Model: "other", Rates: rates}) },
		"input rate":  func() (*Ledger, error) { b := bind; b.Rates.InputPerMTok = 0.5; return Open(path, 5, b) },
		"output rate": func() (*Ledger, error) { b := bind; b.Rates.OutputPerMTok = 20; return Open(path, 5, b) },
		"source":      func() (*Ledger, error) { b := bind; b.Rates.Source = "another quote"; return Open(path, 5, b) },
	} {
		if _, err := change(); !errors.Is(err, ErrBinding) {
			t.Errorf("%s: error %v, want ErrBinding", name, err)
		}
	}
	if _, err := Open(path, 5, bind); err != nil {
		t.Errorf("same binding refused: %v", err)
	}
	// A file with spending but no header, or a second header, fails closed.
	body, _ := os.ReadFile(path)
	headerless := filepath.Join(t.TempDir(), "h.jsonl")
	_ = os.WriteFile(headerless, []byte(`{"t":"reserve","id":"x","usd":1,"at":"t"}`+"\n"), 0o600)
	if _, err := Open(headerless, 5, bind); err == nil {
		t.Error("headerless ledger accepted")
	}
	twice := filepath.Join(t.TempDir(), "t.jsonl")
	_ = os.WriteFile(twice, append(append([]byte{}, body...), body...), 0o600)
	if _, err := Open(twice, 5, bind); err == nil {
		t.Error("second header accepted")
	}
	// A half-written last line fails closed and is not repaired.
	half := filepath.Join(t.TempDir(), "half.jsonl")
	_ = os.WriteFile(half, append(append([]byte{}, body...), []byte(`{"t":"reserve","id":"y","us`)...), 0o600)
	before, _ := os.ReadFile(half)
	if _, err := Open(half, 5, bind); err == nil {
		t.Error("half-written ledger accepted")
	}
	if after, _ := os.ReadFile(half); string(after) != string(before) {
		t.Error("the ledger was rewritten")
	}
}

func TestClaimIsDurableAndExclusive(t *testing.T) {
	l := open(t, 5)
	if err := l.Claim("D1|held_out"); err != nil {
		t.Fatal(err)
	}
	if err := l.Claim("D1|synthetic"); err != nil {
		t.Fatal(err)
	}
	again, _ := Open(l.path, 5, bind)
	if err := again.Claim("D1|held_out"); !errors.Is(err, ErrClaimed) {
		t.Fatalf("second claim = %v, want ErrClaimed", err)
	}
}

func TestOverrunStopIsDerivedWhenTheStopLineIsMissing(t *testing.T) {
	l := open(t, 10)
	r, _ := l.Reserve("a", 1)
	_ = r.Settle(3, Reported)
	body, _ := os.ReadFile(l.path)
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if !strings.Contains(lines[len(lines)-1], `"t":"stop"`) {
		t.Fatalf("last line %q is not the stop", lines[len(lines)-1])
	}
	// A crash between the settlement and the stop line:
	_ = os.WriteFile(l.path, []byte(strings.Join(lines[:len(lines)-1], "\n")+"\n"), 0o600)
	again, err := Open(l.path, 10, bind)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := again.Reserve("b", 0.01); !errors.Is(err, ErrStopped) {
		t.Fatalf("reservation = %v, want ErrStopped", err)
	}
}

func TestDuplicateReservationIDFailsClosed(t *testing.T) {
	l := open(t, 10)
	_, _ = l.Reserve("a", 1)
	body, _ := os.ReadFile(l.path)
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	_ = os.WriteFile(l.path, []byte(strings.Join(append(lines, lines[len(lines)-1]), "\n")+"\n"), 0o600)
	if _, err := l.Reserve("b", 1); err == nil {
		t.Fatal("duplicate reservation id accepted")
	}
}

func TestPlanMustFitAndEveryPhaseIsAffordedTogether(t *testing.T) {
	phases := map[string]Phase{"smoke": {WorstUSD: 1, Requests: 1, RequestsSHA256: "a"}, "held_out": {WorstUSD: 30, Requests: 9, RequestsSHA256: "b"}, "synthetic": {WorstUSD: 15, Requests: 8, RequestsSHA256: "c"}}
	path := filepath.Join(t.TempDir(), "l.jsonl")
	if _, err := Create(path, 40, bind, phases); !errors.Is(err, ErrBudget) {
		t.Fatalf("an oversized plan was created: %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("a ledger file exists for an oversized plan")
	}
	l, err := Create(path, 50, bind, phases)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Affordable("nope"); !errors.Is(err, ErrNotPlanned) {
		t.Fatalf("unplanned phase = %v", err)
	}
	if err := l.Affordable("held_out"); err != nil {
		t.Fatal(err)
	}
	// Spending that leaves held_out alone but starves the others is refused.
	r, _ := l.Reserve("x", 6)
	_ = r.Settle(6, Reported)
	if err := l.Affordable("held_out"); !errors.Is(err, ErrBudget) {
		t.Fatalf("held_out with synthetic and smoke still to run = %v, want ErrBudget", err)
	}
	// Once smoke and synthetic are claimed, their reserve is no longer held back.
	_ = l.Claim("smoke|smoke")
	_ = l.Claim("D1|synthetic")
	if err := l.Affordable("held_out"); err != nil {
		t.Fatalf("after the other phases started: %v", err)
	}
	reopened, err := Open(path, 50, bind)
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := reopened.Phase("held_out"); !ok || p.Requests != 9 {
		t.Fatalf("manifest not loaded from the header: %+v", p)
	}
}

func TestConcurrentReserveSettleAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "l.jsonl")
	if _, err := Create(path, 5, bind, nil); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for p := 0; p < 2; p++ {
		l, _ := Open(path, 5, bind)
		for w := 0; w < 6; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 8; i++ {
					if r, err := l.Reserve("x", 0.4); err == nil {
						_ = r.Settle(0.1, Reported)
					}
				}
			}()
		}
	}
	wg.Wait()
	l, _ := Open(path, 5, bind)
	if c, _ := l.Committed(); c > 5+1e-9 {
		t.Fatalf("committed %v above the cap", c)
	}
}

func TestConcurrentClaimsHaveOneWinner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "l.jsonl")
	if _, err := Create(path, 5, bind, nil); err != nil {
		t.Fatal(err)
	}
	var won atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		l, _ := Open(path, 5, bind)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l.Claim("D1|held_out") == nil {
				won.Add(1)
			}
		}()
	}
	wg.Wait()
	if won.Load() != 1 {
		t.Fatalf("%d winners", won.Load())
	}
}
