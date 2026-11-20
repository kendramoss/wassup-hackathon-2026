package store

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
)

func auditKitEvent(t *testing.T) audit.Event {
	t.Helper()
	d := audit.Event{Tenant: "ws:ws_0123456789ab", ActorKind: audit.ActorUser, Actor: "a1_" + strings.Repeat("a", 32), Action: audit.CaseDelete,
		Phase: audit.PhaseIntent, Outcome: audit.Allowed, Object: "case:case_0123456789ab", RequestID: "0123456789abcdef"}
	e, err := audit.Seal(d, audit.Head{}, time.Date(2026, 11, 20, 10, 0, 0, 123456000, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// The stored document must reproduce the exact event the hash was computed
// over, including microsecond time, or verification would fail after a read.
func TestAuditEventDocRoundTripKeepsTheHash(t *testing.T) {
	e := auditKitEvent(t)
	d, err := toEventDoc(e)
	if err != nil {
		t.Fatal(err)
	}
	if d.Seq != 1 || !d.Time.Equal(e.Time) {
		t.Fatalf("doc = %+v", d)
	}
	got, err := fromEventDoc(d)
	if err != nil || got != e || got.ComputeHash() != e.Hash {
		t.Fatalf("round trip = %+v %v", got, err)
	}
	if eventID(7) != "000000000007" || tenantKey("platform") == tenantKey("ws:ws_0123456789ab") {
		t.Error("ids")
	}
}

func TestAuditTenantDocHeadAndCheckpoint(t *testing.T) {
	var d auditTenantDoc
	if d.checkpoint() != nil {
		t.Error("a tenant that was never purged has a checkpoint")
	}
	d = auditTenantDoc{HeadSeq: 9, HeadHash: strings.Repeat("a", 64), CPSeq: 4, CPHash: strings.Repeat("b", 64)}
	if d.head().Seq != 9 || d.checkpoint().ThroughSeq != 4 {
		t.Errorf("doc = %+v", d)
	}
}

// TestFirestoreAuditEmulator runs the audit store contract against the
// Firestore emulator. It is SKIPPED without FIRESTORE_EMULATOR_HOST; none is
// available in this workspace, so the Firestore adapter is unvalidated.
func TestFirestoreAuditEmulator(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	ctx := context.Background()
	f, err := NewFirestore(ctx, "audit-test-"+time.Now().Format("20060102150405.000000"), "")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st := f.Audit()
	ps, _ := audit.NewPseudonymizer("1", []byte(strings.Repeat("k", 40)))
	now := time.Now().UTC()
	svc := audit.NewService(st, ps, audit.Options{Now: func() time.Time { return now }, Retention: time.Hour, PurgeChunk: 3})
	mk := func() audit.Draft {
		return audit.Draft{Event: audit.Event{Tenant: "ws:ws_0123456789ab", ActorKind: audit.ActorUser, Action: audit.CaseUpdate,
			Phase: audit.PhaseCompletion, Outcome: audit.Allowed, Object: "case:case_0123456789ab"}, ActorID: "alice@example.com"}
	}
	for i := 0; i < 8; i++ {
		if _, err := svc.Record(ctx, mk()); err != nil {
			t.Fatal(err)
		}
		now = now.Add(time.Minute)
	}
	if r, err := svc.Verify(ctx, "ws:ws_0123456789ab", ""); err != nil || !r.OK || r.Checked != 8 {
		t.Fatalf("verify = %+v %v", r, err)
	}
	now = now.Add(2 * time.Hour)
	if n, err := svc.PurgeAll(ctx); err != nil || n != 8 {
		t.Fatalf("purge = %d %v", n, err)
	}
	if e, err := svc.Record(ctx, mk()); err != nil || e.Seq != 9 {
		t.Fatalf("append after purge = %+v %v", e, err)
	}
	if r, err := svc.Verify(ctx, "ws:ws_0123456789ab", ""); err != nil || !r.OK {
		t.Fatalf("verify after purge = %+v %v", r, err)
	}
}
