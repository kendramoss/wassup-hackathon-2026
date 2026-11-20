package budget

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

var t0 = time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)

const alias = "gw-test"

type rig struct {
	dir, att, shr string
	l             *Ledger
	tbl           *PriceTable
	inner         *countModel
	g             *Guard
}

type countModel struct {
	n       atomic.Int64
	respond func(ctx context.Context, req llm.Request) (llm.Response, error)
}

func (c *countModel) Provider() string { return "fake" }
func (c *countModel) Generate(ctx context.Context, req llm.Request) (llm.Response, error) {
	c.n.Add(1)
	return c.respond(ctx, req)
}

func okResp(in, out int) func(context.Context, llm.Request) (llm.Response, error) {
	return func(context.Context, llm.Request) (llm.Response, error) {
		return llm.Response{Text: "x", ModelID: "m", Usage: llm.Usage{InputTokens: in, OutputTokens: out}}, nil
	}
}

func priceJSON(verified, until string, in, out float64) string {
	return fmt.Sprintf(`{"schema":2,"entries":[{"alias":%q,"resolved_model_ids":["m"],"list_usd_per_mtok_in":%v,"list_usd_per_mtok_out":%v,"gateway_usd_per_mtok_in":%v,"gateway_usd_per_mtok_out":%v,"source":"test-proof","verified_at":%q,"valid_until":%q,"max_output_enforced":true,"output_includes_reasoning":true}]}`, alias, in, out, in/2, out/2, verified, until)
}

func sharedJSON(capUSD float64, auth bool) string {
	return fmt.Sprintf(`{"cap_usd":%s,"spent_usd":0,"reserved_usd":0,"attempts_raw":0,"failures_raw":0,"calls_authorized":%v,"extra":{"keep":"me"},"events":[]}`, strconv.FormatFloat(capUSD, 'f', 9, 64), auth)
}

func newRig(t *testing.T, capUSD float64, auth bool) *rig {
	t.Helper()
	d := t.TempDir()
	r := &rig{dir: d, att: filepath.Join(d, "attempts.jsonl"), shr: filepath.Join(d, "SHAREDBUDGET.json")}
	must(t, os.WriteFile(r.shr, []byte(sharedJSON(capUSD, auth)), 0o600))
	tbl, err := ParsePriceTable([]byte(priceJSON("2026-11-21T00:00:00Z", "2026-11-30T00:00:00Z", 4, 20)), 0)
	must(t, err)
	r.tbl = tbl
	r.open(t)
	return r
}

func (r *rig) open(t *testing.T) {
	t.Helper()
	l, err := Open(r.att, r.shr)
	must(t, err)
	r.l = l
	r.inner = &countModel{respond: okResp(100, 10)}
	r.g = NewGuard(r.inner, l, r.tbl, Opts{AllowModelCalls: true, RunID: "t", Now: func() time.Time { return t0 }})
	t.Cleanup(func() { l.Close() })
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func req(maxOut int) llm.Request {
	return llm.Request{Model: alias, System: "sys", Messages: []llm.Message{{Role: llm.RoleUser, Content: "hello"}}, MaxOutputTokens: maxOut}
}

func TestPriceRefusals(t *testing.T) {
	good := priceJSON("2026-11-21T00:00:00Z", "2026-11-30T00:00:00Z", 4, 20)
	cases := map[string]string{
		"zero in":       priceJSON("2026-11-21T00:00:00Z", "2026-11-30T00:00:00Z", 0, 20),
		"negative out":  priceJSON("2026-11-21T00:00:00Z", "2026-11-30T00:00:00Z", 4, -1),
		"unknown field": strings.Replace(good, `"source"`, `"surprise":1,"source"`, 1),
		"missing price": strings.Replace(good, `"list_usd_per_mtok_in":4,`, ``, 1),
		"no attest":     strings.Replace(good, `"max_output_enforced":true`, `"max_output_enforced":false`, 1),
		"no reasoning":  strings.Replace(good, `"output_includes_reasoning":true`, `"output_includes_reasoning":false`, 1),
		"empty source":  strings.Replace(good, `"test-proof"`, `""`, 1),
		"bad date":      strings.Replace(good, `2026-11-21T00:00:00Z`, `yesterday`, 1),
	}
	for name, js := range cases {
		if _, err := ParsePriceTable([]byte(js), 0); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	entry := good[strings.Index(good, `{"alias"`) : len(good)-2]
	dup := `{"schema":1,"entries":[` + entry + `,` + entry + `]}`
	if _, err := ParsePriceTable([]byte(dup), 0); err == nil {
		t.Error("duplicate alias accepted")
	}
	tbl, _ := ParsePriceTable([]byte(good), 0)
	for name, now := range map[string]time.Time{
		"stale":   t0.Add(30 * 24 * time.Hour),
		"expired": time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
		"future":  time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC),
	} {
		if _, err := tbl.Lookup(alias, now, time.Time{}); !errors.Is(err, ErrUnknownPrice) {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, a := range []string{"unknown", "gw", "gw-test-2", ""} {
		if _, err := tbl.Lookup(a, t0, time.Time{}); !errors.Is(err, ErrUnknownPrice) {
			t.Errorf("alias %q: %v", a, err)
		}
	}
	r := newRig(t, 20, true)
	_, err := r.g.Generate(context.Background(), llm.Request{Model: "other", MaxOutputTokens: 10})
	if !errors.Is(err, ErrUnknownPrice) || r.inner.n.Load() != 0 || r.l.Snapshot().Records != 0 {
		t.Fatalf("unknown alias: err=%v calls=%d", err, r.inner.n.Load())
	}
}

func TestOutputCapRequired(t *testing.T) {
	r := newRig(t, 20, true)
	if _, err := r.g.Generate(context.Background(), req(0)); !errors.Is(err, ErrNoOutputCap) {
		t.Fatal(err)
	}
	if _, err := r.g.Generate(context.Background(), req(100000)); !errors.Is(err, ErrOutputCap) {
		t.Fatal(err)
	}
	big := req(10)
	big.System = strings.Repeat("a", 100000)
	if _, err := r.g.Generate(context.Background(), big); !errors.Is(err, ErrInputBound) {
		t.Fatal(err)
	}
	if r.inner.n.Load() != 0 || r.l.Snapshot().AttemptsRaw != 0 {
		t.Fatal("refusals reached the transport or reserved")
	}
	// An over-bound request is a sticky halt: later valid requests are refused too.
	if _, err := r.g.Generate(context.Background(), req(10)); !errors.Is(err, ErrInputBound) {
		t.Fatalf("input-bound halt must stick: %v", err)
	}
}

func reservation(rq llm.Request) int64 {
	inUp, _ := InputTokensUpper(rq)
	base := CostNanos(inUp, 4_000_000_000) + CostNanos(int64(rq.MaxOutputTokens), 20_000_000_000)
	margin := base / 50
	if margin < 1_000_000 {
		margin = 1_000_000
	}
	return base + margin
}

func TestExactReserveSettleArithmetic(t *testing.T) {
	r := newRig(t, 20, true)
	rq := req(1000)
	_, err := r.g.Generate(context.Background(), rq)
	must(t, err)
	st := r.l.Snapshot()
	want := CostNanos(100, 4_000_000_000) + CostNanos(10, 20_000_000_000)
	if st.SpentNanos != want || want != 600000 || st.OpenNanos != 0 || st.AttemptsRaw != 1 {
		t.Fatalf("spent=%d want=%d open=%d", st.SpentNanos, want, st.OpenNanos)
	}
	var seen int64
	r.inner.respond = func(context.Context, llm.Request) (llm.Response, error) {
		seen = r.l.Snapshot().OpenNanos
		return llm.Response{Text: "x", ModelID: "m", Usage: llm.Usage{InputTokens: 100, OutputTokens: 10}}, nil
	}
	_, err = r.g.Generate(context.Background(), rq)
	must(t, err)
	if seen != reservation(rq) {
		t.Fatalf("in-flight reservation %d, want %d", seen, reservation(rq))
	}
	s, err := readShared(r.shr)
	must(t, err)
	if s.spentNanos != 1_200_000 || s.resNanos != 0 || s.attemptsRaw != 2 || string(s.raw["extra"]) == "" {
		t.Fatalf("shared file %+v", s)
	}
	if CostNanos(1_000_000, 750_000_000) != 750_000_000 || CostNanos(1, 750_000_000) != 750 || CostNanos(1, 1) != 1 {
		t.Fatal("cost rounding must round up")
	}
}

func TestInvariantRandomized(t *testing.T) {
	for run := 0; run < 100; run++ {
		rnd := rand.New(rand.NewSource(int64(run)))
		capUSD := 0.01 + rnd.Float64()*0.05
		r := newRig(t, capUSD, true)
		capM, _ := ParseDecimalNanos(strconv.FormatFloat(capUSD, 'f', 9, 64), 9)
		for i := 0; i < 40; i++ {
			out := 1 + rnd.Intn(2000)
			r.inner.respond = func(ctx context.Context, rq llm.Request) (llm.Response, error) {
				switch rnd.Intn(3) {
				case 0:
					return llm.Response{}, &llm.APIError{Provider: "p", StatusCode: 504}
				case 1:
					return llm.Response{}, errors.New("reset")
				}
				return llm.Response{Text: "x", ModelID: "m", Usage: llm.Usage{InputTokens: 50 + rnd.Intn(100), OutputTokens: rnd.Intn(out + 1)}}, nil
			}
			before := r.inner.n.Load()
			_, err := r.g.Generate(WithCase(context.Background(), fmt.Sprint("c", i)), req(out))
			if errors.Is(err, ErrBudget) && r.inner.n.Load() != before {
				t.Fatalf("run %d: over-cap attempt made an HTTP call", run)
			}
			st := r.l.Snapshot()
			if st.SpentNanos+st.OpenNanos > capM {
				t.Fatalf("run %d: spent %d + open %d > cap %d", run, st.SpentNanos, st.OpenNanos, capM)
			}
			if r.inner.n.Load() != st.AttemptsRaw {
				t.Fatalf("run %d: %d HTTP calls but %d reservations", run, r.inner.n.Load(), st.AttemptsRaw)
			}
			if errors.Is(err, ErrFailureStreak) {
				must(t, r.l.AckFailures("test"))
			}
		}
	}
}

func TestOverCapMakesNoCall(t *testing.T) {
	r := newRig(t, 0.0005, true)
	_, err := r.g.Generate(context.Background(), req(1000))
	if !errors.Is(err, ErrBudget) || r.inner.n.Load() != 0 || r.l.Snapshot().Records != 0 {
		t.Fatalf("err=%v calls=%d", err, r.inner.n.Load())
	}
}

func TestFailuresKeepFullReservation(t *testing.T) {
	for name, fn := range map[string]func(context.Context, llm.Request) (llm.Response, error){
		"504": func(context.Context, llm.Request) (llm.Response, error) {
			return llm.Response{}, &llm.APIError{Provider: "p", StatusCode: 504}
		},
		"ambiguous": func(context.Context, llm.Request) (llm.Response, error) {
			return llm.Response{}, errors.New("EOF mid-body")
		},
		"badresp": func(context.Context, llm.Request) (llm.Response, error) { return llm.Response{}, llm.ErrBadResponse },
		"timeout": func(ctx context.Context, _ llm.Request) (llm.Response, error) {
			<-ctx.Done()
			return llm.Response{}, ctx.Err()
		},
		"cancel": func(ctx context.Context, _ llm.Request) (llm.Response, error) {
			return llm.Response{}, context.Canceled
		},
		"nousage": func(context.Context, llm.Request) (llm.Response, error) { return llm.Response{Text: "x"}, nil },
	} {
		r := newRig(t, 20, true)
		r.g.o.AttemptTimeout = 50 * time.Millisecond
		r.inner.respond = fn
		rq := req(500)
		r.g.Generate(context.Background(), rq)
		st := r.l.Snapshot()
		if st.SpentNanos != reservation(rq) || st.OpenNanos != 0 {
			t.Errorf("%s: spent %d want full reservation %d", name, st.SpentNanos, reservation(rq))
		}
	}
}

func TestRetryAboveGuardOneReservationPerAttempt(t *testing.T) {
	r := newRig(t, 20, true)
	r.inner.respond = func(context.Context, llm.Request) (llm.Response, error) {
		return llm.Response{}, &llm.APIError{Provider: "p", StatusCode: 504}
	}
	m := llm.WithRetry(r.g, 1, time.Millisecond)
	if _, err := m.Generate(context.Background(), req(100)); err == nil {
		t.Fatal("want failure")
	}
	st := r.l.Snapshot()
	if r.inner.n.Load() != 2 || st.AttemptsRaw != 2 || st.FailuresRaw != 2 {
		t.Fatalf("calls=%d attempts=%d failures=%d", r.inner.n.Load(), st.AttemptsRaw, st.FailuresRaw)
	}
}

func TestPerCaseCapAndFailureStreak(t *testing.T) {
	r := newRig(t, 20, true)
	r.g.o.MaxCaseAttempts = 3
	ctx := WithCase(context.Background(), "case-1")
	for i := 0; i < 3; i++ {
		if _, err := r.g.Generate(ctx, req(10)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.g.Generate(ctx, req(10)); !errors.Is(err, ErrCaseCap) {
		t.Fatalf("want case cap, got %v", err)
	}
	r.inner.respond = func(context.Context, llm.Request) (llm.Response, error) { return llm.Response{}, errors.New("boom") }
	for i := 0; i < 3; i++ {
		r.g.Generate(WithCase(context.Background(), fmt.Sprint("f", i)), req(10))
	}
	before := r.inner.n.Load()
	if _, err := r.g.Generate(WithCase(context.Background(), "f9"), req(10)); !errors.Is(err, ErrFailureStreak) || r.inner.n.Load() != before {
		t.Fatalf("want streak stop with no call, got %v", err)
	}
	if r.g.Halted() == nil {
		t.Fatal("failure streak must be a sticky halt for the runner")
	}
	must(t, r.l.AckFailures("operator reviewed"))
	r.inner.respond = okResp(10, 1)
	r.g = NewGuard(r.inner, r.l, r.tbl, Opts{AllowModelCalls: true, Now: func() time.Time { return t0 }}) // a human restarted the run
	if _, err := r.g.Generate(WithCase(context.Background(), "f9"), req(10)); err != nil {
		t.Fatal(err)
	}
}

func TestCancelSemantics(t *testing.T) {
	r := newRig(t, 20, true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.g.Generate(ctx, req(10)); !errors.Is(err, context.Canceled) || r.l.Snapshot().Records != 0 || r.inner.n.Load() != 0 {
		t.Fatal("pre-reservation cancel must cost nothing")
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	r.g.hookAfterReserve = cancel2
	_, err := r.g.Generate(ctx2, req(10))
	st := r.l.Snapshot()
	if !errors.Is(err, context.Canceled) || r.inner.n.Load() != 0 || st.SpentNanos != 0 || st.OpenNanos != 0 || st.AttemptsRaw != 1 || st.FailuresRaw != 0 {
		t.Fatalf("err=%v st=%+v", err, st)
	}
	r.g.hookAfterReserve = nil
	ctx3, cancel3 := context.WithCancel(context.Background())
	r.inner.respond = func(ctx context.Context, _ llm.Request) (llm.Response, error) {
		cancel3()
		<-ctx.Done()
		return llm.Response{}, ctx.Err()
	}
	r.g.Generate(ctx3, req(10))
	if r.l.Snapshot().SpentNanos == 0 {
		t.Fatal("in-flight cancel must be charged in full")
	}
}

func TestCrashLeavesOpenReservationChargedInFull(t *testing.T) {
	r := newRig(t, 20, true)
	_, err := r.l.Reserve(ReserveReq{Alias: alias, Nanos: 123456, MaxOut: 10})
	must(t, err)
	r.l.Close()
	l2, err := Open(r.att, r.shr)
	must(t, err)
	defer l2.Close()
	st := l2.Snapshot()
	if st.SpentNanos != 123456 || st.OpenNanos != 0 || !st.Halted {
		t.Fatalf("%+v", st)
	}
	if _, err := l2.Reserve(ReserveReq{Alias: alias, Nanos: 1, MaxOut: 1}); !errors.Is(err, ErrHalted) {
		t.Fatalf("unreconciled crash must halt: %v", err)
	}

	s, _ := readShared(r.shr)
	if s.spentNanos != 123456 || s.resNanos != 0 {
		t.Fatalf("shared %+v", s)
	}
	must(t, l2.AckHalt("reconciled with gateway key spend"))
	if _, err := l2.Reserve(ReserveReq{Alias: alias, Nanos: 1, MaxOut: 1}); err != nil {
		t.Fatal(err)
	}
}

func TestHelperProcess(t *testing.T) {
	mode := os.Getenv("BUDGET_HELPER")
	if mode == "" {
		t.Skip("helper")
	}
	l, err := Open(os.Getenv("BUDGET_ATT"), os.Getenv("BUDGET_SHR"))
	if err != nil {
		os.Exit(7)
	}
	switch mode {
	case "crash":
		if _, err := l.Reserve(ReserveReq{Alias: alias, Nanos: 777000, MaxOut: 1}); err != nil {
			os.Exit(8)
		}
		os.Exit(3)
	case "hold":
		os.Stdout.WriteString("held\n")
		time.Sleep(5 * time.Second)
	}
}

func helper(r *rig, mode string) *exec.Cmd {
	c := exec.Command(os.Args[0], "-test.run=^TestHelperProcess$")
	c.Env = append(os.Environ(), "BUDGET_HELPER="+mode, "BUDGET_ATT="+r.att, "BUDGET_SHR="+r.shr)
	return c
}

func TestRealProcessCrashThenResume(t *testing.T) {
	r := newRig(t, 20, true)
	r.l.Close()
	err := helper(r, "crash").Run()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 3 {
		t.Fatalf("helper exit: %v", err)
	}
	l, err := Open(r.att, r.shr)
	must(t, err)
	defer l.Close()
	if st := l.Snapshot(); st.SpentNanos != 777000 || st.OpenNanos != 0 || st.AttemptsRaw != 1 || st.FailuresRaw != 1 {
		t.Fatalf("%+v", st)
	}
}

func TestSecondProcessFailsOnLock(t *testing.T) {
	r := newRig(t, 20, true)
	r.l.Close()
	c := helper(r, "hold")
	out, _ := c.StdoutPipe()
	must(t, c.Start())
	defer c.Process.Kill()
	buf := make([]byte, 5)
	if _, err := out.Read(buf); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(r.att, r.shr); !errors.Is(err, ErrLocked) {
		t.Fatalf("second opener got %v", err)
	}
}

func TestCorruptLedgerStops(t *testing.T) {
	mk := func() *rig {
		r := newRig(t, 20, true)
		for i := 0; i < 3; i++ {
			r.g.Generate(WithCase(context.Background(), fmt.Sprint(i)), req(10))
		}
		r.l.Close()
		return r
	}
	read := func(r *rig) []string {
		b, _ := os.ReadFile(r.att)
		return strings.SplitAfter(strings.TrimSuffix(string(b), "\n"), "\n")
	}
	t.Run("truncated", func(t *testing.T) {
		r := mk()
		b, _ := os.ReadFile(r.att)
		os.WriteFile(r.att, b[:len(b)-20], 0o600)
		if _, err := Open(r.att, r.shr); !errors.Is(err, ErrCorrupt) {
			t.Fatal(err)
		}
	})
	t.Run("garbage", func(t *testing.T) {
		r := mk()
		f, _ := os.OpenFile(r.att, os.O_APPEND|os.O_WRONLY, 0)
		f.WriteString("not json\n")
		f.Close()
		if _, err := Open(r.att, r.shr); !errors.Is(err, ErrCorrupt) {
			t.Fatal(err)
		}
	})
	t.Run("edited-field", func(t *testing.T) {
		r := mk()
		ls := read(r)
		ls[1] = strings.Replace(ls[1], `"charged_nanos":`, `"charged_nanos":1`, 1)
		os.WriteFile(r.att, []byte(strings.Join(ls, "")+"\n"), 0o600)
		if _, err := Open(r.att, r.shr); !errors.Is(err, ErrCorrupt) {
			t.Fatal(err)
		}
	})
	t.Run("deleted-line", func(t *testing.T) {
		r := mk()
		ls := read(r)
		os.WriteFile(r.att, []byte(ls[0]+strings.Join(ls[2:], "")+"\n"), 0o600)
		if _, err := Open(r.att, r.shr); !errors.Is(err, ErrCorrupt) {
			t.Fatal(err)
		}
	})
	t.Run("reordered", func(t *testing.T) {
		r := mk()
		ls := read(r)
		ls[2], ls[3] = ls[3], ls[2]
		os.WriteFile(r.att, []byte(strings.Join(ls, "")+"\n"), 0o600)
		if _, err := Open(r.att, r.shr); !errors.Is(err, ErrCorrupt) {
			t.Fatal(err)
		}
	})
	t.Run("status-also-detects", func(t *testing.T) {
		r := mk()
		b, _ := os.ReadFile(r.att)
		os.WriteFile(r.att, b[:len(b)-3], 0o600)
		if _, _, _, err := Inspect(r.att, r.shr); !errors.Is(err, ErrCorrupt) {
			t.Fatal(err)
		}
	})
}

func TestSharedEditedDownward(t *testing.T) {
	r := newRig(t, 20, true)
	_, err := r.g.Generate(context.Background(), req(10))
	must(t, err)
	must(t, os.WriteFile(r.shr, []byte(sharedJSON(20, true)), 0o600))
	if _, err := r.g.Generate(context.Background(), req(10)); !errors.Is(err, ErrTamper) {
		t.Fatalf("mid-run: %v", err)
	}
	r.l.Close()
	if _, err := Open(r.att, r.shr); !errors.Is(err, ErrTamper) {
		t.Fatalf("at open: %v", err)
	}
	if _, _, _, err := Inspect(r.att, r.shr); !errors.Is(err, ErrTamper) {
		t.Fatalf("inspect: %v", err)
	}
}

func TestExternalSpendAndReservationCounted(t *testing.T) {
	d := t.TempDir()
	shr := filepath.Join(d, "s.json")
	must(t, os.WriteFile(shr, []byte(`{"cap_usd":20,"spent_usd":19.999,"reserved_usd":0,"attempts_raw":5,"failures_raw":1,"calls_authorized":true}`), 0o600))
	l, err := Open(filepath.Join(d, "a.jsonl"), shr)
	must(t, err)
	defer l.Close()
	tbl, _ := ParsePriceTable([]byte(priceJSON("2026-11-21T00:00:00Z", "2026-11-30T00:00:00Z", 4, 20)), 0)
	m := &countModel{respond: okResp(1, 1)}
	g := NewGuard(m, l, tbl, Opts{AllowModelCalls: true, Now: func() time.Time { return t0 }})
	if _, err := g.Generate(context.Background(), req(1000)); !errors.Is(err, ErrBudget) || m.n.Load() != 0 {
		t.Fatalf("%v", err)
	}
	l.Close()
	must(t, os.WriteFile(shr, []byte(`{"cap_usd":20,"spent_usd":19.999,"reserved_usd":1,"attempts_raw":5,"failures_raw":1,"calls_authorized":true}`), 0o600))
	if _, err := Open(filepath.Join(d, "a.jsonl"), shr); !errors.Is(err, ErrTamper) {
		t.Fatalf("%v", err)
	}
}

func TestAuthorizationGatesAndHardCap(t *testing.T) {
	r := newRig(t, 20, false)
	if _, err := r.g.Generate(context.Background(), req(10)); !errors.Is(err, ErrUnauthorized) || r.inner.n.Load() != 0 {
		t.Fatalf("calls_authorized=false: %v", err)
	}
	r.g.o.AllowModelCalls = false
	if _, err := r.g.Generate(context.Background(), req(10)); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("flag: %v", err)
	}
	r.g.o.AllowModelCalls = true
	must(t, os.WriteFile(r.shr, []byte(`{"cap_usd":20,"spent_usd":0,"reserved_usd":0,"attempts_raw":0,"failures_raw":0,"calls_authorized":"true"}`), 0o600))
	if _, err := r.g.Generate(context.Background(), req(10)); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("string true: %v", err)
	}
	// cap_usd above 20 is clamped to the hard cap.
	r.l.Close()
	must(t, os.WriteFile(r.shr, []byte(`{"cap_usd":500,"spent_usd":19.95,"reserved_usd":0,"attempts_raw":1,"failures_raw":0,"calls_authorized":true}`), 0o600))
	l, err := Open(r.att, r.shr)
	must(t, err)
	defer l.Close()
	g := NewGuard(r.inner, l, r.tbl, Opts{AllowModelCalls: true, Now: func() time.Time { return t0 }})
	if _, err := g.Generate(context.Background(), req(8000)); !errors.Is(err, ErrBudget) {
		t.Fatalf("hard cap must clamp cap_usd=500: %v", err)
	}
}

func TestOverrunLatchesClosed(t *testing.T) {
	r := newRig(t, 20, true)
	r.inner.respond = okResp(100, 9999)
	_, err := r.g.Generate(context.Background(), req(50))
	if !errors.Is(err, ErrHalted) {
		t.Fatal(err)
	}
	if st := r.l.Snapshot(); st.SpentNanos < CostNanos(9999, 20_000_000_000) {
		t.Fatalf("actual overrun not recorded: %+v", st)
	}
	before := r.inner.n.Load()
	if _, err := r.g.Generate(context.Background(), req(50)); !errors.Is(err, ErrHalted) || r.inner.n.Load() != before {
		t.Fatalf("latched guard made a call: %v", err)
	}
	// The halt survives a restart until a human acknowledges it.
	r.l.Close()
	l2, err := Open(r.att, r.shr)
	must(t, err)
	defer l2.Close()
	if !l2.Snapshot().Halted {
		t.Fatal("halt lost across restart")
	}
}

func TestLedgerWriteFailureFailsClosed(t *testing.T) {
	r := newRig(t, 20, true)
	r.l.f.Close()
	_, err := r.g.Generate(context.Background(), req(10))
	if !errors.Is(err, ErrPoisoned) || r.inner.n.Load() != 0 {
		t.Fatalf("reserve write failure must not send: %v calls=%d", err, r.inner.n.Load())
	}
	r2 := newRig(t, 20, true)
	r2.inner.respond = func(context.Context, llm.Request) (llm.Response, error) {
		r2.l.f.Close()
		return llm.Response{Text: "x", ModelID: "m", Usage: llm.Usage{InputTokens: 10, OutputTokens: 1}}, nil
	}
	if _, err := r2.g.Generate(context.Background(), req(10)); err == nil {
		t.Fatal("settle failure returned a clean response")
	}
	if _, err := r2.g.Generate(context.Background(), req(10)); !errors.Is(err, ErrPoisoned) || r2.inner.n.Load() != 1 {
		t.Fatalf("after settle failure: %v", err)
	}
	r2.l.lock.Close()
	l, err := Open(r2.att, r2.shr)
	must(t, err)
	defer l.Close()
	if st := l.Snapshot(); st.SpentNanos == 0 || st.OpenNanos != 0 {
		t.Fatalf("%+v", st)
	}
}

func TestSecretsAndBodiesNeverInLedger(t *testing.T) {
	const key, secretText = "sk-SENTINEL-KEY-12345", "SENSITIVE-PROMPT-TEXT"
	r := newRig(t, 20, true)
	r.inner.respond = func(context.Context, llm.Request) (llm.Response, error) {
		return llm.Response{}, &llm.APIError{Provider: "p", StatusCode: 500, Body: "echo " + key}
	}
	rq := req(10)
	rq.Messages[0].Content = secretText
	r.g.Generate(context.Background(), rq)
	for _, p := range []string{r.att, r.shr} {
		b, _ := os.ReadFile(p)
		if strings.Contains(string(b), key) || strings.Contains(string(b), secretText) || strings.Contains(strings.ToLower(string(b)), "authorization") {
			t.Fatalf("%s leaks", p)
		}
	}
}

func TestNilTableNeverPrices(t *testing.T) {
	var nilTable *PriceTable
	if _, err := nilTable.Lookup(alias, t0, time.Time{}); !errors.Is(err, ErrUnknownPrice) {
		t.Fatal(err)
	}
}
