package budget

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// R1: whether anything may have been sent is the guard-owned permit state, not an error tag.
func TestTakenPermitIsChargedWhateverTheErrorSays(t *testing.T) {
	for name, inner := range map[string]func(context.Context, llm.Request) (llm.Response, error){
		"unsent claim after take": func(ctx context.Context, _ llm.Request) (llm.Response, error) {
			if err := PermitFrom(ctx).Take(); err != nil { // the wire attempt
				return llm.Response{}, err
			}
			return llm.Response{}, errors.Join(ErrNotSent, errors.New("pretends nothing was sent"))
		},
		"replay after one wire call": func(ctx context.Context, _ llm.Request) (llm.Response, error) {
			p := PermitFrom(ctx)
			if err := p.Take(); err != nil {
				return llm.Response{}, err
			}
			return llm.Response{}, p.Take() // second attempt: ErrPermitUsed
		},
		"replay tagged unsent": func(ctx context.Context, _ llm.Request) (llm.Response, error) {
			p := PermitFrom(ctx)
			p.Take()
			return llm.Response{}, errors.Join(ErrNotSent, p.Take())
		},
	} {
		r := newRig(t, 20, true)
		r.inner.respond = inner
		rq := req(50)
		before := r.inner.n.Load()
		r.g.Generate(WithCase(context.Background(), "c"), rq)
		st := r.l.Snapshot()
		if r.inner.n.Load()-before != 1 || st.SpentNanos != reservation(rq) || st.SpentNanos == 0 || st.FailuresRaw != 1 || st.OpenNanos != 0 {
			t.Errorf("%s: spent=%d want %d failures=%d", name, st.SpentNanos, reservation(rq), st.FailuresRaw)
		}
	}
	// A fresh, never-consumed permit with a local refusal still costs nothing.
	r := newRig(t, 20, true)
	r.inner.respond = func(context.Context, llm.Request) (llm.Response, error) {
		return llm.Response{}, errors.Join(ErrNotSent, errors.New("bad target"))
	}
	r.g.Generate(WithCase(context.Background(), "c"), req(50))
	if st := r.l.Snapshot(); st.SpentNanos != 0 || st.FailuresRaw != 0 {
		t.Fatalf("%+v", st)
	}
}

// crashAfterChainSettle leaves the chain one settle ahead of the shared file,
// exactly as a crash between the durable append and the JSON rewrite would.
func crashAfterChainSettle(t *testing.T, r *rig, charged int64) {
	t.Helper()
	res, err := r.l.Reserve(ReserveReq{Alias: alias, CaseID: "c", Nanos: 1_000_000, MaxOut: 1})
	must(t, err)
	r.l.mu.Lock()
	err = r.l.appendLocked(Record{Type: TypeSettle, AttemptID: res.ID, Outcome: OutcomeOK, ChargedNanos: charged})
	r.l.mu.Unlock()
	must(t, err) // chain written, shared file NOT rewritten
	r.l.Close()
}

func TestCrashBetweenChainSettleAndFileWriteReopensIdempotently(t *testing.T) {
	r := newRig(t, 20, true)
	crashAfterChainSettle(t, r, 400_000)
	// The file is behind the chain (spent 0, reserved still 1_000_000).
	sh, _ := readShared(r.shr)
	if sh.spentNanos != 0 || sh.resNanos != 1_000_000 {
		t.Fatalf("setup did not reproduce the crash window: %+v", sh)
	}
	var want Status
	for i := 0; i < 3; i++ { // reopen repeatedly: safe and idempotent
		l, err := Open(r.att, r.shr)
		if err != nil {
			t.Fatalf("reopen %d needs no hand edit: %v", i, err)
		}
		st := l.Snapshot()
		sh, _ = readShared(r.shr)
		if st.SpentNanos != 400_000 || st.OpenNanos != 0 || st.Halted || sh.spentNanos != 400_000 || sh.resNanos != 0 || sh.attemptsRaw != 1 {
			t.Fatalf("reopen %d: chain %+v file %+v", i, st, sh)
		}
		if i == 0 {
			want = st
		} else if st.Records != want.Records || st.SpentNanos != want.SpentNanos {
			t.Fatalf("reopen %d changed the chain: %+v vs %+v", i, st, want)
		}
		l.Close()
	}
}

func TestCrashBetweenReserveAndFileWriteChargesFullAndHeals(t *testing.T) {
	r := newRig(t, 20, true)
	r.l.mu.Lock()
	must(t, r.l.appendLocked(Record{Type: TypeReserve, AttemptID: "a000001", CaseID: "c", ReservedNanos: 2_000_000, MaxOut: 1}))
	r.l.mu.Unlock()
	r.l.Close() // reserve in the chain, file never told
	l, err := Open(r.att, r.shr)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer l.Close()
	st := l.Snapshot()
	sh, _ := readShared(r.shr)
	if st.SpentNanos != 2_000_000 || !st.Halted || sh.spentNanos != 2_000_000 || sh.attemptsRaw != 1 || sh.failuresRaw != 1 {
		t.Fatalf("chain %+v file %+v", st, sh)
	}
}

func TestCrashInsideOpenRecoveryIsSafeToRepeat(t *testing.T) {
	r := newRig(t, 20, true)
	r.l.mu.Lock()
	must(t, r.l.appendLocked(Record{Type: TypeReserve, AttemptID: "a000001", CaseID: "c", ReservedNanos: 2_000_000, MaxOut: 1}))
	// What Open's recovery appends, but dying before it syncs the file.
	must(t, r.l.appendLocked(Record{Type: TypeSettle, AttemptID: "a000001", Outcome: OutcomeCrashRecovery, ChargedNanos: 2_000_000}))
	must(t, r.l.appendLocked(Record{Type: TypeHalt, Note: "unreconciled"}))
	r.l.mu.Unlock()
	r.l.Close()
	l, err := Open(r.att, r.shr)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	st := l.Snapshot()
	if st.SpentNanos != 2_000_000 || st.OpenNanos != 0 || !st.Halted {
		t.Fatalf("%+v", st)
	}
}

// The crash tolerance is only the unsynced tail: a real downward edit is still a conflict.
func TestLagToleranceDoesNotHideALoweredFile(t *testing.T) {
	r := newRig(t, 20, true)
	for i := 0; i < 3; i++ {
		_, err := r.g.Generate(WithCase(context.Background(), string(rune('a'+i))), req(10))
		must(t, err)
	}
	crashAfterChainSettle(t, r, 400_000)
	setField(t, r.shr, "spent_usd", "0.000000000") // below chain minus the tail
	if _, err := Open(r.att, r.shr); !errors.Is(err, ErrTamper) {
		t.Fatalf("lowered file hidden by the lag window: %v", err)
	}
	if _, _, _, err := Inspect(r.att, r.shr); !errors.Is(err, ErrTamper) {
		t.Fatalf("inspect: %v", err)
	}
}

// R2: an operator's spend rise is recorded before the lowered reservation is refused, and the file is left alone.
func TestReconcileAcrossRestart(t *testing.T) {
	r := newRig(t, 20, true)
	setField(t, r.shr, "external_reserved_usd", "5.000000000")
	_, err := r.g.Generate(context.Background(), req(10))
	must(t, err)
	before := r.l.Snapshot()
	r.l.Close()
	// While stopped, the CI smoke finishes: 5 USD move from reserved to spent.
	setField(t, r.shr, "external_reserved_usd", "0.000000000")
	setField(t, r.shr, "spent_usd", "5.100000000")
	l, err := Open(r.att, r.shr)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	sh, _ := readShared(r.shr)
	if sh.spentNanos != 5_100_000_000 || sh.extResNanos != 0 {
		t.Fatalf("Open overwrote the operator's edit: %+v", sh)
	}
	st := l.Snapshot()
	if st.ExtSpent < 5_000_000_000 || st.ExtRes != before.ExtRes {
		t.Fatalf("spend rise not recorded first: %+v", st)
	}
	g := NewGuard(r.inner, l, r.tbl, Opts{AllowModelCalls: true, Now: func() time.Time { return t0 }, RunEndsBy: t0.Add(time.Hour)})
	if _, err := g.Generate(context.Background(), req(10)); !errors.Is(err, ErrExtReservedLowered) {
		t.Fatalf("pending lowered reservation must refuse: %v", err)
	}
	totalBefore := before.SpentNanos + before.ExtBase + before.ExtRes
	must(t, l.ReconcileExternalReserved(strings.Repeat("ci smoke settled evidence ", 3)))
	st = l.Snapshot()
	if st.ExtRes != 0 || st.SpentNanos+st.ExtSpent+st.ExtRes < totalBefore {
		t.Fatalf("budget returned by the reconcile: before total %d, after %+v", totalBefore, st)
	}
	// The refusal latched that guard (sticky by design); a restarted run proceeds.
	g = NewGuard(r.inner, l, r.tbl, Opts{AllowModelCalls: true, Now: func() time.Time { return t0 }, RunEndsBy: t0.Add(time.Hour)})
	if _, err := g.Generate(context.Background(), req(10)); err != nil {
		t.Fatalf("after reconcile: %v", err)
	}
	sh, _ = readShared(r.shr)
	if sh.spentNanos < 5_100_000_000 || sh.extResNanos != 0 {
		t.Fatalf("file lost money after reconcile: %+v", sh)
	}
	// Crash right after the reconcile record, before the file write: reopen is clean.
	l.Close()
	l2, err := Open(r.att, r.shr)
	if err != nil {
		t.Fatal(err)
	}
	l2.Close()
}

func TestPendingReconcileRefusesAReturnOfBudgetAfterRestart(t *testing.T) {
	r := newRig(t, 20, true)
	setField(t, r.shr, "external_reserved_usd", "5.000000000")
	_, err := r.g.Generate(context.Background(), req(10))
	must(t, err)
	r.l.Close()
	setField(t, r.shr, "external_reserved_usd", "0.000000000") // lowered, spend NOT raised
	l, err := Open(r.att, r.shr)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if err := l.ReconcileExternalReserved(strings.Repeat("claims it was returned ", 3)); !errors.Is(err, ErrTamper) {
		t.Fatalf("budget given back: %v", err)
	}
}

// O2: public hashes are recorded in the chain; rebinding to a different value is refused.
func TestBindRecordsPublicHashesOnly(t *testing.T) {
	r := newRig(t, 20, true)
	h1, h2 := strings.Repeat("a", 64), strings.Repeat("b", 64)
	must(t, r.l.Bind(map[string]string{"pricing_proof_sha256": h1, "guard_commit": "7bed855"}))
	must(t, r.l.Bind(map[string]string{"pricing_proof_sha256": h1})) // same value: fine
	if err := r.l.Bind(map[string]string{"pricing_proof_sha256": h2}); err == nil {
		t.Fatal("rebinding to a different value accepted")
	}
	if err := r.l.Bind(map[string]string{"x_sha256": "short"}); err == nil {
		t.Fatal("malformed hash accepted")
	}
	if err := r.l.Bind(nil); err == nil {
		t.Fatal("empty bind accepted")
	}
	r.l.Close()
	l, err := Open(r.att, r.shr)
	must(t, err)
	defer l.Close()
	if got := l.Bound(); got["pricing_proof_sha256"] != h1 || got["guard_commit"] != "7bed855" {
		t.Fatalf("%v", got)
	}
	if err := l.Bind(map[string]string{"guard_commit": "other"}); err == nil {
		t.Fatal("rebinding after restart accepted")
	}
	b, _ := os.ReadFile(r.att)
	if strings.Contains(strings.ToLower(string(b)), "key") && strings.Contains(string(b), "sk-") {
		t.Fatal("secret-looking content in the ledger")
	}
}
