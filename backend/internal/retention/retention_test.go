package retention

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
)

var now = time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)

const (
	wsA = "ws_aaaaaaaaaaaa"
	wsB = "ws_bbbbbbbbbbbb"
)

type fakeCases struct {
	cases   map[string]Case
	deleted []string
	failOn  string
}

func (f *fakeCases) WorkspaceCases(_ context.Context, ws string) ([]Case, error) {
	var out []Case
	for _, c := range f.cases {
		if c.Workspace == ws {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeCases) DeleteCase(_ context.Context, id string) error {
	if id == f.failOn {
		return errors.New("boom")
	}
	delete(f.cases, id)
	f.deleted = append(f.deleted, id)
	return nil
}

func days(n int) time.Time { return now.Add(-time.Duration(n) * 24 * time.Hour) }

func setup(t *testing.T) (*Memory, *fakeCases) {
	t.Helper()
	st := NewMemory()
	ctx := context.Background()
	if err := st.SetPolicy(ctx, Policy{Workspace: wsA, CaseDays: 30}); err != nil {
		t.Fatal(err)
	}
	cs := &fakeCases{cases: map[string]Case{
		"case_000000000001": {ID: "case_000000000001", Workspace: wsA, UpdatedAt: days(40)},
		"case_000000000002": {ID: "case_000000000002", Workspace: wsA, UpdatedAt: days(10)},
		"case_000000000003": {ID: "case_000000000003", Workspace: wsA, UpdatedAt: days(90), Pinned: true},
		"case_000000000004": {ID: "case_000000000004", Workspace: wsB, UpdatedAt: days(400)}, // no policy
		"demo-kobalt":       {ID: "demo-kobalt", Workspace: wsA, UpdatedAt: days(400)},
	}}
	return st, cs
}

func TestPurgeDeletesOnlyExpiredUnpinnedPolicyCases(t *testing.T) {
	st, cs := setup(t)
	var audited []string
	rec := func(_ context.Context, tb Tombstone) error { audited = append(audited, tb.CaseID); return nil }
	rep, err := Purge(context.Background(), st, cs, rec, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(cs.deleted) != 1 || cs.deleted[0] != "case_000000000001" {
		t.Fatalf("deleted %v", cs.deleted)
	}
	if rep.Skipped != 2 || len(audited) != 1 {
		t.Fatalf("report %+v audited %v", rep, audited)
	}
	ts, _ := st.Tombstones(context.Background(), wsA, 10)
	if len(ts) != 1 || ts[0].Reason != ReasonPolicy || ts[0].PolicyDays != 30 || !ts[0].LastUpdated.Equal(days(40)) {
		t.Fatalf("tombstones %+v", ts)
	}
}

func TestDryRunDeletesNothing(t *testing.T) {
	st, cs := setup(t)
	rep, err := Purge(context.Background(), st, cs, nil, Options{Now: now, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Deleted) != 1 || len(cs.deleted) != 0 {
		t.Fatalf("dry run: %+v, deleted %v", rep, cs.deleted)
	}
	if ts, _ := st.Tombstones(context.Background(), wsA, 0); len(ts) != 0 {
		t.Fatalf("dry run wrote tombstones %v", ts)
	}
}

func TestFailureLeavesTombstoneBeforeDelete(t *testing.T) {
	st, cs := setup(t)
	cs.failOn = "case_000000000001"
	if _, err := Purge(context.Background(), st, cs, nil, Options{Now: now}); err == nil {
		t.Fatal("want error")
	}
	if _, ok := cs.cases["case_000000000001"]; !ok {
		t.Fatal("case gone after failed delete")
	}
	// An audit failure stops before the delete.
	st2, cs2 := setup(t)
	_, err := Purge(context.Background(), st2, cs2, func(context.Context, Tombstone) error { return errors.New("audit down") }, Options{Now: now})
	if err == nil || len(cs2.deleted) != 0 {
		t.Fatalf("err=%v deleted=%v", err, cs2.deleted)
	}
}

func TestLimitAndWorkspaceFilter(t *testing.T) {
	st, cs := setup(t)
	cs.cases["case_000000000005"] = Case{ID: "case_000000000005", Workspace: wsA, UpdatedAt: days(50)}
	rep, err := Purge(context.Background(), st, cs, nil, Options{Now: now, Limit: 1})
	if err != nil || len(rep.Deleted) != 1 || !rep.Truncated || rep.Deleted[0].CaseID != "case_000000000005" {
		t.Fatalf("limit: %+v %v (oldest first)", rep, err)
	}
	rep, err = Purge(context.Background(), st, cs, nil, Options{Now: now, Workspace: wsB})
	if err != nil || rep.Workspaces != 0 || len(rep.Deleted) != 0 {
		t.Fatalf("filter: %+v %v", rep, err)
	}
}

func TestPolicyValidation(t *testing.T) {
	for _, p := range []Policy{{Workspace: "x", CaseDays: 30}, {Workspace: wsA, CaseDays: 3}, {Workspace: wsA, CaseDays: 5000}} {
		if p.Validate() == nil {
			t.Fatalf("accepted %+v", p)
		}
	}
	for _, p := range []Policy{{Workspace: wsA}, {Workspace: wsA, CaseDays: 7}, {Workspace: wsA, CaseDays: 3650}} {
		if err := p.Validate(); err != nil {
			t.Fatalf("rejected %+v: %v", p, err)
		}
	}
}

func TestFileStorePersists(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	f, err := NewFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.SetPolicy(ctx, Policy{Workspace: wsA, CaseDays: 30}); err != nil {
		t.Fatal(err)
	}
	if err := f.AddTombstone(ctx, Tombstone{CaseID: "case_000000000001", Workspace: wsA, Reason: ReasonUser}); err != nil {
		t.Fatal(err)
	}
	g, err := NewFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p, ok, _ := g.Policy(ctx, wsA); !ok || p.CaseDays != 30 {
		t.Fatalf("policy %+v %v", p, ok)
	}
	if ts, _ := g.Tombstones(ctx, wsA, 0); len(ts) != 1 {
		t.Fatalf("tombstones %v", ts)
	}
}

func TestAuditRecorderWritesASystemDelete(t *testing.T) {
	ps, err := audit.NewPseudonymizer("1", []byte(strings.Repeat("k", 40)))
	if err != nil {
		t.Fatal(err)
	}
	svc := audit.NewService(audit.NewMemory(), ps, audit.Options{Now: func() time.Time { return now }})
	st, cs := setup(t)
	if _, err := Purge(context.Background(), st, cs, AuditRecorder(svc), Options{Now: now}); err != nil {
		t.Fatal(err)
	}
	p, err := svc.List(context.Background(), "ws:"+wsA, audit.ListQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Events) != 1 || p.Events[0].Action != audit.CaseDelete || p.Events[0].Detail != "retention" ||
		p.Events[0].ActorKind != audit.ActorSystem || p.Events[0].Object != "case:case_000000000001" {
		t.Fatalf("events %+v", p.Events)
	}
}
