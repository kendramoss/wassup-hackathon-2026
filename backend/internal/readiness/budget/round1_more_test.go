package budget

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// C1: the permit cannot be forged from outside the package.
func TestPermitUnforgeable(t *testing.T) {
	var zero Permit
	if err := zero.Take(); !errors.Is(err, ErrNoPermit) {
		t.Fatalf("zero value permit: %v", err)
	}
	if err := (&Permit{}).Take(); !errors.Is(err, ErrNoPermit) {
		t.Fatalf("literal permit: %v", err)
	}
	// Copies share one state, so a copy cannot replay a used permit.
	p := mintPermit()
	cp := *p
	if err := p.Take(); err != nil {
		t.Fatal(err)
	}
	if err := cp.Take(); !errors.Is(err, ErrPermitUsed) {
		t.Fatalf("copy replayed the permit: %v", err)
	}
	// A context without the guard's permit yields none.
	if PermitFrom(context.Background()) != nil {
		t.Fatal("permit from nowhere")
	}
	// API shape: no exported way to build or attach one, no exported fields.
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	if len(files) == 0 {
		t.Fatal("no non-test source files parsed")
	}
	for _, f := range files {
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && d.Name.IsExported() && (strings.Contains(d.Name.Name, "Permit") && d.Name.Name != "PermitFrom") {
					t.Errorf("exported permit constructor/attacher %s", d.Name.Name)
				}
			case *ast.GenDecl:
				for _, sp := range d.Specs {
					if ts, ok := sp.(*ast.TypeSpec); ok && ts.Name.Name == "Permit" {
						for _, fld := range ts.Type.(*ast.StructType).Fields.List {
							for _, n := range fld.Names {
								if n.IsExported() {
									t.Errorf("Permit has exported field %s", n.Name)
								}
							}
						}
					}
				}
			}
		}
	}
}

func setField(t *testing.T, path, key, val string) {
	t.Helper()
	b, _ := os.ReadFile(path)
	s := string(b)
	if i := strings.Index(s, `"`+key+`":`); i >= 0 {
		j := i + len(key) + 3
		k := j
		for k < len(s) && s[k] != ',' && s[k] != '\n' && s[k] != '}' {
			k++
		}
		s = s[:j] + " " + val + s[k:]
	} else {
		s = strings.Replace(s, `"events"`, `"`+key+`": `+val+`,"events"`, 1)
	}
	must(t, os.WriteFile(path, []byte(s), 0o600))
}

// C2: external money is monotonic across settles and restarts; a fall is a conflict, never a reset.
func TestExternalSpendMonotonicAcrossRestartAndSettle(t *testing.T) {
	r := newRig(t, 20, true)
	setField(t, r.shr, "spent_usd", "5.000000000")
	_, err := r.g.Generate(context.Background(), req(10))
	must(t, err)
	if r.l.Snapshot().ExtSpent != 5_000_000_000 {
		t.Fatalf("external floor not remembered in the chain: %+v", r.l.Snapshot())
	}
	// Lowered during a run: the settle of an in-flight call records the chain
	// charge, fails closed, and the file is not rewritten.
	r.inner.respond = func(context.Context, llm.Request) (llm.Response, error) {
		setField(t, r.shr, "spent_usd", "1.000000000")
		return llm.Response{Text: "x", ModelID: "m", Usage: llm.Usage{InputTokens: 10, OutputTokens: 1}}, nil
	}
	if _, err := r.g.Generate(context.Background(), req(10)); err == nil {
		t.Fatal("conflicting edit during a settle was accepted")
	}
	sh, _ := readShared(r.shr)
	if sh.spentNanos != 1_000_000_000 {
		t.Fatalf("the guard overwrote the conflicting file value: %d", sh.spentNanos)
	}
	if st := r.l.Snapshot(); st.OpenNanos != 0 || st.SpentNanos == 0 {
		t.Fatalf("chain must still hold the settle: %+v", st)
	}
	if _, err := r.g.Generate(context.Background(), req(10)); err == nil {
		t.Fatal("guard continued after a conflict")
	}
	// Restart with the file still lowered: refused (floor is in the chain).
	r.l.Close()
	if _, err := Open(r.att, r.shr); !errors.Is(err, ErrTamper) {
		t.Fatalf("restart with lowered external spend: %v", err)
	}
	// A person reconciles the file (stale reservation cleared, spend at least the floor).
	setField(t, r.shr, "reserved_usd", "0.000000000")
	setField(t, r.shr, "spent_usd", "5.100000000")
	l, err := Open(r.att, r.shr)
	must(t, err)
	defer l.Close()
	if l.Snapshot().ExtSpent < 5_000_000_000 {
		t.Fatal("floor lost")
	}
}

// C4: lowering external_reserved_usd needs an audited reconciliation that does not return budget.
func TestExternalReservedReconcile(t *testing.T) {
	r := newRig(t, 20, true)
	setField(t, r.shr, "external_reserved_usd", "5.000000000")
	_, err := r.g.Generate(context.Background(), req(10))
	must(t, err)
	if r.l.Snapshot().ExtRes != 5_000_000_000 {
		t.Fatalf("%+v", r.l.Snapshot())
	}
	setField(t, r.shr, "external_reserved_usd", "0.000000000")
	r.l.Close()
	l, err := Open(r.att, r.shr) // Open tolerates it so the audited fix can run
	must(t, err)
	defer l.Close()
	g := NewGuard(r.inner, l, r.tbl, Opts{AllowModelCalls: true, Now: func() time.Time { return t0 }, RunEndsBy: t0.Add(time.Hour)})
	if _, err := g.Generate(context.Background(), req(10)); !errors.Is(err, ErrExtReservedLowered) || !errors.Is(err, ErrTamper) {
		t.Fatalf("silent lowering accepted: %v", err)
	}
	// Reservation fell with no matching rise in spend: would hand budget back.
	long := strings.Repeat("evidence ", 6)
	if err := l.ReconcileExternalReserved(long); !errors.Is(err, ErrTamper) {
		t.Fatalf("budget-returning reconcile accepted: %v", err)
	}
	if err := l.ReconcileExternalReserved("short"); err == nil {
		t.Fatal("unaudited reconcile accepted")
	}
	// The CI run spent 5 USD: spent rose by the amount the reservation fell.
	setField(t, r.shr, "spent_usd", "5.100000000")
	must(t, l.ReconcileExternalReserved(long))
	st := l.Snapshot()
	if st.ExtRes != 0 || st.ExtSpent < 5_000_000_000 {
		t.Fatalf("%+v", st)
	}
	g2 := NewGuard(r.inner, l, r.tbl, Opts{AllowModelCalls: true, Now: func() time.Time { return t0 }, RunEndsBy: t0.Add(time.Hour)})
	if _, err := g2.Generate(context.Background(), req(10)); err != nil {
		t.Fatalf("after reconcile: %v", err)
	}
	// Total committed money never fell across the reconcile.
	if st.SpentNanos+st.ExtSpent+st.ExtRes < 5_000_000_000 {
		t.Fatal("budget returned")
	}
	// A hand-edited chain that lowers the floor is corrupt.
	r.l.Close()
	l.Close()
}

func TestChainRejectsLoweredExternalFloor(t *testing.T) {
	c := newChain()
	apply := func(rec Record) error {
		rec.Seq, rec.Prev = c.seq+1, c.lastHash
		return c.apply(rec, []byte("x"))
	}
	if err := apply(Record{Type: TypeExt, ExtSpentNanos: 10, ExtResNanos: 10}); err != nil {
		t.Fatal(err)
	}
	if err := apply(Record{Type: TypeExt, ExtSpentNanos: 5, ExtResNanos: 10}); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("lowered floor accepted: %v", err)
	}
	if err := apply(Record{Type: TypeExtReconcile, ExtSpentNanos: 10, ExtResNanos: 0, Note: "short"}); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("unaudited reconcile accepted: %v", err)
	}
}

// C8: known-unsent errors cost nothing and do not touch the streak; ambiguous ones cost in full.
func TestKnownUnsentVersusAmbiguous(t *testing.T) {
	r := newRig(t, 20, true)
	r.inner.respond = func(context.Context, llm.Request) (llm.Response, error) {
		return llm.Response{}, errors.Join(ErrNotSent, errors.New("refused before the wire"))
	}
	for i := 0; i < 5; i++ {
		r.g.Generate(WithCase(context.Background(), "u"), req(10))
	}
	st := r.l.Snapshot()
	if st.SpentNanos != 0 || st.FailuresRaw != 0 || st.FailureStreak != 0 || st.AttemptsRaw != 5 || st.Halted {
		t.Fatalf("%+v", st)
	}
	r.inner.respond = func(context.Context, llm.Request) (llm.Response, error) {
		return llm.Response{}, errors.New("connection reset while reading the response")
	}
	rq := req(10)
	r.g.Generate(WithCase(context.Background(), "v"), rq)
	st = r.l.Snapshot()
	if st.SpentNanos != reservation(rq) || st.FailureStreak != 1 {
		t.Fatalf("ambiguous failure must be charged in full: %+v", st)
	}
}

// C8: a clean Ctrl-C: the in-flight attempt is settled (charged in full) before returning.
func TestInterruptSettlesCleanly(t *testing.T) {
	r := newRig(t, 20, true)
	ctx, cancel := context.WithCancel(context.Background())
	entered := make(chan struct{})
	r.inner.respond = func(ctx context.Context, _ llm.Request) (llm.Response, error) {
		close(entered)
		<-ctx.Done()
		return llm.Response{}, ctx.Err()
	}
	done := make(chan error, 1)
	rq := req(10)
	go func() { _, err := r.g.Generate(ctx, rq); done <- err }()
	<-entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	st := r.l.Snapshot()
	if st.OpenNanos != 0 || st.SpentNanos != reservation(rq) || st.OpenAttempts != 0 {
		t.Fatalf("interrupt left the ledger inconsistent: %+v", st)
	}
	if err := r.l.Close(); err != nil {
		t.Fatal(err)
	}
	l2, err := Open(r.att, r.shr)
	must(t, err)
	defer l2.Close()
	if l2.Snapshot().Halted {
		t.Fatal("a clean interrupt must not need an ack-halt")
	}
}

func TestRunWindowPast(t *testing.T) {
	r := newRig(t, 20, true)
	r.g.o.RunEndsBy = t0.Add(-time.Minute)
	if _, err := r.g.Generate(context.Background(), req(10)); !errors.Is(err, ErrNoRunWindow) {
		t.Fatal(err)
	}
	r.g.o.RunEndsBy = time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC) // price proof valid until 11-30
	if _, err := r.g.Generate(context.Background(), req(10)); !errors.Is(err, ErrUnknownPrice) {
		t.Fatalf("proof expiring before the run ends: %v", err)
	}
}
