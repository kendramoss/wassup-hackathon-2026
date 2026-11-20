package budget

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

func TestResolvedModelAllowlistHaltsAndKeepsReservation(t *testing.T) {
	for name, id := range map[string]string{"other": "some-other-model", "empty": ""} {
		r := newRig(t, 20, true)
		r.inner.respond = func(context.Context, llm.Request) (llm.Response, error) {
			return llm.Response{Text: "x", ModelID: id, Usage: llm.Usage{InputTokens: 10, OutputTokens: 1}}, nil
		}
		rq := req(100)
		_, err := r.g.Generate(context.Background(), rq)
		if !errors.Is(err, ErrModelNotAllowed) {
			t.Fatalf("%s: %v", name, err)
		}
		if st := r.l.Snapshot(); st.SpentNanos != reservation(rq) || !st.Halted {
			t.Fatalf("%s: %+v", name, st)
		}
		before := r.inner.n.Load()
		if _, err := r.g.Generate(context.Background(), rq); err == nil || r.inner.n.Load() != before {
			t.Fatalf("%s: call after halt", name)
		}
	}
}

func TestUsageAbsentOn200KeepsFullReservationAndHalts(t *testing.T) {
	r := newRig(t, 20, true)
	r.inner.respond = func(context.Context, llm.Request) (llm.Response, error) {
		return llm.Response{Text: "x", ModelID: "m"}, nil
	}
	rq := req(100)
	if _, err := r.g.Generate(context.Background(), rq); !errors.Is(err, ErrHalted) {
		t.Fatal(err)
	}
	if st := r.l.Snapshot(); st.SpentNanos != reservation(rq) || !st.Halted {
		t.Fatalf("%+v", st)
	}
}

func TestPromptTokensAboveEstimateHalts(t *testing.T) {
	r := newRig(t, 20, true)
	r.inner.respond = okResp(10_000_000, 1)
	if _, err := r.g.Generate(context.Background(), req(10)); !errors.Is(err, ErrHalted) {
		t.Fatal(err)
	}
	if !r.l.Snapshot().Halted {
		t.Fatal("no persisted halt")
	}
}

func TestErrBudgetIsNotRetriedAndSticks(t *testing.T) {
	r := newRig(t, 0.0005, true)
	m := llm.WithRetry(r.g, 5, time.Millisecond)
	_, err := m.Generate(context.Background(), req(1000))
	if !errors.Is(err, ErrBudget) || r.inner.n.Load() != 0 {
		t.Fatalf("%v", err)
	}
	if errors.Is(err, llm.ErrUnavailable) || llm.Unavailable(err) {
		t.Fatal("ErrBudget must not look retryable to callers")
	}
	// Even a request that would now fit is refused: the run halts.
	if _, err := r.g.Generate(context.Background(), req(1)); !errors.Is(err, ErrBudget) {
		t.Fatalf("budget halt must stick: %v", err)
	}
	if r.g.Halted() == nil {
		t.Fatal("runner cannot see the halt")
	}
}

func TestCaseSpendCeilingAndEligibility(t *testing.T) {
	r := newRig(t, 20, true)
	r.g.o.MaxCaseNanos = reservation(req(100)) + 10
	ctx := WithCase(context.Background(), "c1")
	if _, err := r.g.Generate(ctx, req(100)); err != nil {
		t.Fatal(err)
	}
	// Settled cost is tiny, so the ceiling is not hit by actual spend; make
	// the second reservation not fit.
	r.g.o.MaxCaseNanos = 5
	if _, err := r.g.Generate(ctx, req(100)); !errors.Is(err, ErrCaseCap) {
		t.Fatalf("%v", err)
	}
	r.g.o.AllowedCases = map[string]bool{"ok": true}
	before := r.inner.n.Load()
	if _, err := r.g.Generate(WithCase(context.Background(), "intruder"), req(10)); !errors.Is(err, ErrCaseNotEligible) || r.inner.n.Load() != before {
		t.Fatalf("%v", err)
	}
	if _, err := r.g.Generate(context.Background(), req(10)); !errors.Is(err, ErrCaseNotEligible) {
		t.Fatalf("untagged request must be refused when a list is frozen: %v", err)
	}
}

func TestConcurrencyOneAndCancelWhileWaiting(t *testing.T) {
	r := newRig(t, 20, true)
	var cur, peak atomic.Int64
	gate := make(chan struct{})
	r.inner.respond = func(ctx context.Context, _ llm.Request) (llm.Response, error) {
		n := cur.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		<-gate
		cur.Add(-1)
		return llm.Response{Text: "x", ModelID: "m", Usage: llm.Usage{InputTokens: 10, OutputTokens: 1}}, nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r.g.Generate(WithCase(context.Background(), fmt.Sprint("w", i)), req(10))
		}(i)
	}
	time.Sleep(100 * time.Millisecond)
	// A waiter that is cancelled leaves no reservation.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	attemptsBefore := r.l.Snapshot().AttemptsRaw
	if _, err := r.g.Generate(ctx, req(10)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("%v", err)
	}
	if r.l.Snapshot().AttemptsRaw != attemptsBefore {
		t.Fatal("a cancelled waiter reserved money")
	}
	for i := 0; i < 8; i++ {
		gate <- struct{}{}
	}
	wg.Wait()
	if peak.Load() != 1 {
		t.Fatalf("peak concurrency %d", peak.Load())
	}
	if st := r.l.Snapshot(); st.AttemptsRaw != 8 || st.OpenNanos != 0 {
		t.Fatalf("%+v", st)
	}
}

func TestNonLocalPathRefused(t *testing.T) {
	for _, p := range []string{
		"/Users/x/Library/Mobile Documents/com~apple~CloudDocs/a.jsonl",
		"/Users/x/Library/CloudStorage/Dropbox/a.jsonl",
	} {
		if err := CheckLocalPath(p); !errors.Is(err, ErrNonLocalFS) {
			t.Errorf("%s: %v", p, err)
		}
	}
	d := t.TempDir()
	if err := CheckLocalPath(d + "/a.jsonl"); err != nil {
		t.Errorf("local temp dir refused: %v", err)
	}
	if _, err := Open("/Users/x/Library/Mobile Documents/a.jsonl", d+"/s.json"); !errors.Is(err, ErrNonLocalFS) {
		t.Errorf("Open: %v", err)
	}
}

func TestExternalReservationCounted(t *testing.T) {
	r := newRig(t, 20, true)
	// A stated sub-allocation (for example the CI smoke) leaves less room.
	b, _ := os.ReadFile(r.shr)
	s := strings.Replace(string(b), `"events"`, `"external_reserved_usd":19.999,"events"`, 1)
	must(t, os.WriteFile(r.shr, []byte(s), 0o600))
	if _, err := r.g.Generate(context.Background(), req(1000)); !errors.Is(err, ErrBudget) || r.inner.n.Load() != 0 {
		t.Fatalf("%v", err)
	}
}

func TestDecimalAndIntegerMoney(t *testing.T) {
	for in, want := range map[string]int64{"0": 0, "20": 20_000_000_000, "19.999": 19_999_000_000, "0.000000001": 1} {
		if got, err := ParseDecimalNanos(in, 9); err != nil || got != want {
			t.Errorf("%q: %d %v", in, got, err)
		}
		if got, _ := ParseDecimalNanos(FormatNanos(want), 9); got != want {
			t.Errorf("format roundtrip %d", want)
		}
	}
	for _, bad := range []string{"", "1e3", "-1", "+1", "0.0000000001", "1.2.3", "abc", " ", "1,5"} {
		if _, err := ParseDecimalNanos(bad, 9); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if CostNanos(1<<62, 1<<40) <= HardCapNanos {
		t.Fatal("overflow must saturate above the cap, not wrap")
	}
	if CostNanos(0, 5) != 0 || CostNanos(-3, 5) != 0 {
		t.Fatal("non-positive tokens")
	}
}

func TestEffectivePriceIsMaximumAndRunWindow(t *testing.T) {
	js := `{"schema":2,"entries":[{"alias":"a","resolved_model_ids":["m"],"list_usd_per_mtok_in":4,"list_usd_per_mtok_out":20,"gateway_usd_per_mtok_in":6.5,"gateway_usd_per_mtok_out":10,"worst_tier_usd_per_mtok_in":8,"worst_tier_usd_per_mtok_out":0.5,"source":"s","verified_at":"2026-11-21T00:00:00Z","valid_until":"2026-11-25T00:00:00Z","max_output_enforced":true,"output_includes_reasoning":true}]}`
	tb, err := ParsePriceTable([]byte(js), 0)
	must(t, err)
	p, err := tb.Lookup("a", t0, time.Time{})
	must(t, err)
	if p.InNanoPerMtok() != 8_000_000_000 || p.OutNanoPerMtok() != 20_000_000_000 {
		t.Fatalf("effective %d %d", p.InNanoPerMtok(), p.OutNanoPerMtok())
	}
	if _, err := tb.Lookup("a", t0, time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)); !errors.Is(err, ErrUnknownPrice) {
		t.Fatalf("price expiring before the run ends accepted: %v", err)
	}
	if _, err := ParsePriceTable([]byte(strings.Replace(js, `"list_usd_per_mtok_in":4,`, ``, 1)), 0); err == nil {
		t.Fatal("list price is required")
	}
	if _, err := ParsePriceTable([]byte(strings.Replace(js, `"schema":2`, `"schema":1`, 1)), 0); err == nil {
		t.Fatal("old schema accepted")
	}
}

func TestPermitIsOneShotAndDefaultTransportDenied(t *testing.T) {
	p := &Permit{}
	if err := p.Take(); err != nil {
		t.Fatal(err)
	}
	if err := p.Take(); !errors.Is(err, ErrPermitUsed) {
		t.Fatal(err)
	}
	var np *Permit
	if err := np.Take(); !errors.Is(err, ErrNoPermit) {
		t.Fatal(err)
	}
	rq, _ := http.NewRequest("GET", "https://example.invalid/", nil)
	if _, err := (DenyTransport{}).RoundTrip(rq); !errors.Is(err, ErrNoPermit) {
		t.Fatal(err)
	}
}

func TestSharedFileFormatPreservesUnknownFields(t *testing.T) {
	r := newRig(t, 20, true)
	r.g.Generate(context.Background(), req(10))
	b, _ := os.ReadFile(r.shr)
	if !strings.Contains(string(b), `"keep": "me"`) || !strings.Contains(string(b), `"spent_usd": 0.0`) {
		t.Fatalf("shared file rewritten badly:\n%s", b)
	}
}
