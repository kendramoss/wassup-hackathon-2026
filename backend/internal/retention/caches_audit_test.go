package retention

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/blob"
)

type fakePurger struct {
	got  []string
	fail bool
	cs   *fakeCases
	// sawCaseGone records whether the case was already deleted when it ran.
	sawCaseGone bool
	extra       map[string][]blob.Ref
}

func (f *fakePurger) PurgeCase(_ context.Context, id string, extra []blob.Ref) (blob.PurgeReport, error) {
	if _, ok := f.cs.cases[id]; !ok {
		f.sawCaseGone = true
	}
	if f.extra == nil {
		f.extra = map[string][]blob.Ref{}
	}
	f.extra[id] = extra
	if f.fail {
		return blob.PurgeReport{}, errors.New("cache store down")
	}
	f.got = append(f.got, id)
	return blob.PurgeReport{Deleted: 2, Shared: 1}, nil
}

func TestPurgeRemovesDerivedCachesBeforeTheCase(t *testing.T) {
	st, cs := setup(t)
	c := cs.cases["case_000000000001"]
	c.Derived = []blob.Ref{{Store: blob.StoreVerdicts, Key: "supplier-doc-abc"}}
	cs.cases["case_000000000001"] = c
	fp := &fakePurger{cs: cs}
	rep, err := Purge(context.Background(), st, cs, nil, Options{Now: now, Caches: fp})
	if err != nil {
		t.Fatal(err)
	}
	if len(fp.got) != 1 || fp.got[0] != "case_000000000001" || fp.sawCaseGone {
		t.Fatalf("purger calls %v (case gone first: %v)", fp.got, fp.sawCaseGone)
	}
	if len(fp.extra["case_000000000001"]) != 1 || fp.extra["case_000000000001"][0].Key != "supplier-doc-abc" {
		t.Fatalf("derived blobs not passed: %v", fp.extra)
	}
	if rep.Caches.Deleted != 2 || rep.Caches.Shared != 1 {
		t.Fatalf("report %+v", rep.Caches)
	}
}

func TestCachePurgeFailureStopsBeforeTheDeleteAndDryRunTouchesNothing(t *testing.T) {
	st, cs := setup(t)
	fp := &fakePurger{cs: cs, fail: true}
	if _, err := Purge(context.Background(), st, cs, nil, Options{Now: now, Caches: fp}); err == nil {
		t.Fatal("want error")
	}
	if len(cs.deleted) != 0 {
		t.Fatalf("case deleted although its caches could not be purged: %v", cs.deleted)
	}
	st2, cs2 := setup(t)
	fp2 := &fakePurger{cs: cs2}
	if _, err := Purge(context.Background(), st2, cs2, nil, Options{Now: now, DryRun: true, Caches: fp2}); err != nil || len(fp2.got) != 0 {
		t.Fatalf("dry run purged caches: %v %v", fp2.got, err)
	}
}

func TestAuditDaysValidation(t *testing.T) {
	for _, d := range []int{0, 7, 90, 3650} {
		if err := (Policy{Workspace: wsA, AuditDays: d}).Validate(); err != nil {
			t.Errorf("%d: %v", d, err)
		}
	}
	for _, d := range []int{-1, 1, 6, 3651} {
		if err := (Policy{Workspace: wsA, AuditDays: d}).Validate(); err == nil {
			t.Errorf("%d accepted", d)
		}
	}
}

func auditSvc(t *testing.T, global time.Duration) (*audit.Service, *audit.Memory, *time.Time) {
	t.Helper()
	ps, err := audit.NewPseudonymizer("1", []byte(strings.Repeat("k", 40)))
	if err != nil {
		t.Fatal(err)
	}
	clock := now.Add(-100 * 24 * time.Hour)
	mem := audit.NewMemory()
	return audit.NewService(mem, ps, audit.Options{Retention: global, Now: func() time.Time { return clock }}), mem, &clock
}

// writeAt records one event for each workspace at the given age.
func writeAt(t *testing.T, svc *audit.Service, clock *time.Time, ws string, ageDays int) {
	t.Helper()
	*clock = now.Add(-time.Duration(ageDays) * 24 * time.Hour)
	obj, _ := audit.ObjectRef("workspace", ws)
	if _, err := svc.Record(context.Background(), audit.Draft{Event: audit.Event{Tenant: "ws:" + ws, ActorKind: audit.ActorSystem,
		Action: audit.WorkspaceCreate, Phase: audit.PhaseSingle, Outcome: audit.Allowed, Object: obj}}); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceAuditWindowIsAppliedAndKeepsTheChainVerifiable(t *testing.T) {
	ctx := context.Background()
	svc, _, clock := auditSvc(t, 180*24*time.Hour)
	for _, age := range []int{150, 120, 60, 20, 5} {
		writeAt(t, svc, clock, wsA, age)
	}
	for _, age := range []int{150, 5} {
		writeAt(t, svc, clock, wsB, age)
	}
	*clock = now
	st := NewMemory()
	_ = st.SetPolicy(ctx, Policy{Workspace: wsA, AuditDays: 30})
	// wsB has no audit window: the global one rules, and the command leaves it.

	rep, err := Purge(ctx, st, &fakeCases{cases: map[string]Case{}}, nil, Options{Now: now, DryRun: true, Audit: svc, AuditWindow: 180 * 24 * time.Hour})
	if err != nil || len(rep.Audit) != 1 || rep.Audit[0].Events != 3 || rep.Audit[0].Days != 30 {
		t.Fatalf("dry run: %+v %v", rep.Audit, err)
	}
	if v, _ := svc.Verify(ctx, "ws:"+wsA, ""); !v.OK {
		t.Fatalf("verify after dry run %+v", v)
	}
	if p, _ := svc.List(ctx, "ws:"+wsA, audit.ListQuery{Limit: 50}); len(p.Events) != 5 {
		t.Fatalf("dry run removed events: %d", len(p.Events))
	}

	rep, err = Purge(ctx, st, &fakeCases{cases: map[string]Case{}}, nil, Options{Now: now, Audit: svc, AuditWindow: 180 * 24 * time.Hour})
	if err != nil || rep.Audit[0].Events != 3 {
		t.Fatalf("apply: %+v %v", rep.Audit, err)
	}
	p, _ := svc.List(ctx, "ws:"+wsA, audit.ListQuery{Limit: 50})
	var kept, marks int
	for _, e := range p.Events {
		if e.Action == audit.AuditPurge {
			marks++
			if e.ActorKind != audit.ActorSystem || e.Detail != "workspace" || e.Object != "workspace:"+wsA {
				t.Fatalf("purge event %+v", e)
			}
		} else {
			kept++
		}
	}
	if kept != 2 || marks != 1 {
		t.Fatalf("kept %d events and %d purge marks, want 2 and 1: %+v", kept, marks, p.Events)
	}
	v, err := svc.Verify(ctx, "ws:"+wsA, "")
	if err != nil || !v.OK || v.Checkpoint == nil || v.Checkpoint.ThroughSeq != 3 {
		t.Fatalf("the retained trail does not verify against its checkpoint: %+v %v", v, err)
	}
	if p, _ := svc.List(ctx, "ws:"+wsB, audit.ListQuery{Limit: 50}); len(p.Events) != 2 {
		t.Fatalf("another workspace's trail was touched: %d events", len(p.Events))
	}
	// Running it again removes nothing and adds no further mark.
	rep, _ = Purge(ctx, st, &fakeCases{cases: map[string]Case{}}, nil, Options{Now: now, Audit: svc, AuditWindow: 180 * 24 * time.Hour})
	if rep.Audit[0].Events != 0 {
		t.Fatalf("second run %+v", rep.Audit)
	}
	if p, _ := svc.List(ctx, "ws:"+wsA, audit.ListQuery{Limit: 50}); len(p.Events) != 3 {
		t.Fatalf("second run changed the trail: %d", len(p.Events))
	}
}

func TestAuditWindowNeverLongerThanTheGlobalOne(t *testing.T) {
	ctx := context.Background()
	svc, _, clock := auditSvc(t, 60*24*time.Hour)
	writeAt(t, svc, clock, wsA, 100)
	writeAt(t, svc, clock, wsA, 10)
	*clock = now
	st := NewMemory()
	_ = st.SetPolicy(ctx, Policy{Workspace: wsA, AuditDays: 365}) // set when the global window was longer
	rep, err := Purge(ctx, st, &fakeCases{cases: map[string]Case{}}, nil, Options{Now: now, Audit: svc, AuditWindow: 60 * 24 * time.Hour})
	if err != nil || len(rep.Audit) != 1 || rep.Audit[0].Days != 60 || rep.Audit[0].Events != 1 {
		t.Fatalf("%+v %v", rep.Audit, err)
	}
}

func TestNoAuditServiceNoAuditPurge(t *testing.T) {
	st := NewMemory()
	_ = st.SetPolicy(context.Background(), Policy{Workspace: wsA, AuditDays: 30})
	rep, err := Purge(context.Background(), st, &fakeCases{cases: map[string]Case{}}, nil, Options{Now: now})
	if err != nil || len(rep.Audit) != 0 {
		t.Fatalf("%+v %v", rep.Audit, err)
	}
}
