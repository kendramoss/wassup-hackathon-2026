package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/blob"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/cachepurge"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retention"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

func TestRunDryRunThenApply(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	now := time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)
	cs, err := store.NewFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	old := store.Case{ID: "case_000000000001", Workspace: "ws_aaaaaaaaaaaa", CreatedAt: now.AddDate(0, 0, -60), UpdatedAt: now.AddDate(0, 0, -60)}
	fresh := store.Case{ID: "case_000000000002", Workspace: "ws_aaaaaaaaaaaa", CreatedAt: now, UpdatedAt: now}
	for _, c := range []store.Case{old, fresh} {
		if err := cs.Put(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	rs, _ := retention.NewFile(dir)
	if err := rs.SetPolicy(ctx, retention.Policy{Workspace: "ws_aaaaaaaaaaaa", CaseDays: 30}); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Retention: true, CasesStore: config.CasesFile, CasesDir: dir}

	var out bytes.Buffer
	if err := run(ctx, cfg, nil, &out, now); err != nil {
		t.Fatal(err)
	}
	var rep retention.Report
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil || !rep.DryRun || len(rep.Deleted) != 1 {
		t.Fatalf("dry run %+v %v", rep, err)
	}
	if _, err := cs.Get(ctx, old.ID); err != nil {
		t.Fatal("dry run deleted a case")
	}

	out.Reset()
	if err := run(ctx, cfg, []string{"-apply"}, &out, now); err != nil {
		t.Fatal(err)
	}
	cs2, _ := store.NewFile(dir)
	if _, err := cs2.Get(ctx, old.ID); err == nil {
		t.Fatal("expired case still there")
	}
	if _, err := cs2.Get(ctx, fresh.ID); err != nil {
		t.Fatal("fresh case deleted")
	}
	rs2, _ := retention.NewFile(dir)
	if ts, _ := rs2.Tombstones(ctx, "ws_aaaaaaaaaaaa", 0); len(ts) != 1 || ts[0].CaseID != old.ID {
		t.Fatalf("tombstones %v", ts)
	}
}

func TestRunRefusesWhenOff(t *testing.T) {
	if err := run(context.Background(), config.Config{}, nil, &bytes.Buffer{}, time.Now()); err == nil {
		t.Fatal("ran with RETENTION off")
	}
	if err := run(context.Background(), config.Config{Retention: true, CasesStore: config.CasesMemory}, nil, &bytes.Buffer{}, time.Now()); err == nil {
		t.Fatal("ran on a memory store")
	}
}

func TestRunPurgesDerivedCachesAndOrphansEndToEnd(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	now := time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)
	cs, _ := store.NewFile(dir)
	const ws = "ws_aaaaaaaaaaaa"
	docSHA := "d0c"
	old := store.Case{ID: "case_000000000001", Workspace: ws, CreatedAt: now.AddDate(0, 0, -60), UpdatedAt: now.AddDate(0, 0, -60),
		SupplierRequests: []store.SupplierRequest{{ID: "r", Answers: []store.SupplierAnswer{{Document: &store.SupplierDocument{SHA256: docSHA}}}}}}
	fresh := store.Case{ID: "case_000000000002", Workspace: ws, CreatedAt: now, UpdatedAt: now}
	for _, c := range []store.Case{old, fresh} {
		if err := cs.Put(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	rs, _ := retention.NewFile(dir)
	_ = rs.SetPolicy(ctx, retention.Policy{Workspace: ws, CaseDays: 30})
	cacheDir := t.TempDir() + "/verifier-cache"
	cfg := config.Config{Retention: true, CachePurge: true, CasesStore: config.CasesFile, CasesDir: dir,
		VerifierCacheStore: config.VerdictsFile, VerifierCacheDir: cacheDir}

	// What the server would have written: entries linked to the cases.
	setup, err := cachepurge.Open(ctx, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	verdicts := blob.Track(setup.Stores[blob.StoreVerdicts], blob.StoreVerdicts, setup.Refs)
	oldCtx, freshCtx := blob.WithOwner(ctx, old.ID), blob.WithOwner(ctx, fresh.ID)
	_ = verdicts.Put(oldCtx, "only-old", []byte("1"))
	_ = verdicts.Put(oldCtx, "shared", []byte("2"))
	_, _, _ = verdicts.Get(freshCtx, "shared")
	_ = verdicts.Put(ctx, "supplier-doc-"+docSHA, []byte("pdf")) // stored before links existed
	_ = verdicts.Put(blob.WithOwner(ctx, "case_00000000dead"), "gone", []byte("3"))
	exists := func(k string) bool { _, ok, _ := setup.Stores[blob.StoreVerdicts].Get(ctx, k); return ok }

	var out bytes.Buffer
	if err := run(ctx, cfg, nil, &out, now); err != nil { // dry run
		t.Fatal(err)
	}
	if !exists("only-old") || !exists("supplier-doc-"+docSHA) {
		t.Fatal("a dry run removed cache entries")
	}
	out.Reset()
	if err := run(ctx, cfg, []string{"-apply", "-orphans"}, &out, now); err != nil {
		t.Fatal(err)
	}
	var rep struct {
		Caches  blob.PurgeReport
		Orphans *blob.OrphanReport
	}
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if exists("only-old") || exists("supplier-doc-"+docSHA) {
		t.Fatal("the expired case's own entries are still there")
	}
	if !exists("shared") {
		t.Fatal("an entry the fresh case still uses was removed")
	}
	if exists("gone") || rep.Orphans == nil || rep.Orphans.Cases != 1 || rep.Orphans.Deleted != 1 {
		t.Fatalf("orphan entry: %+v", rep.Orphans)
	}
	if rep.Caches.Deleted != 2 || rep.Caches.Shared != 1 {
		t.Fatalf("caches %+v", rep.Caches)
	}
}

func TestOrphansNeedCachePurge(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{Retention: true, CasesStore: config.CasesFile, CasesDir: dir}
	if err := run(context.Background(), cfg, []string{"-orphans"}, &bytes.Buffer{}, time.Now()); err == nil {
		t.Fatal("-orphans ran without CACHE_PURGE")
	}
}

func TestRunAppliesAWorkspaceAuditWindow(t *testing.T) {
	ctx := context.Background()
	dir, auditDir := t.TempDir(), t.TempDir()
	now := time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)
	const ws = "ws_aaaaaaaaaaaa"
	rs, _ := retention.NewFile(dir)
	_ = rs.SetPolicy(ctx, retention.Policy{Workspace: ws, AuditDays: 30})
	cfg := config.Config{Retention: true, CasesStore: config.CasesFile, CasesDir: dir, Audit: true, AuditDir: auditDir,
		AuditActorKey: strings.Repeat("k", 40), AuditActorKeyID: "1", AuditRetentionDays: 90}
	ps, _ := audit.NewPseudonymizer("1", []byte(cfg.AuditActorKey))
	as, _ := audit.NewFile(auditDir)
	clock := now
	svc := audit.NewService(as, ps, audit.Options{Retention: 90 * 24 * time.Hour, Now: func() time.Time { return clock }})
	for _, age := range []int{80, 10} {
		clock = now.AddDate(0, 0, -age)
		obj, _ := audit.ObjectRef("workspace", ws)
		if _, err := svc.Record(ctx, audit.Draft{Event: audit.Event{Tenant: "ws:" + ws, ActorKind: audit.ActorSystem, Action: audit.WorkspaceCreate,
			Phase: audit.PhaseSingle, Outcome: audit.Allowed, Object: obj}}); err != nil {
			t.Fatal(err)
		}
	}
	clock = now
	var out bytes.Buffer
	if err := run(ctx, cfg, []string{"-apply"}, &out, now); err != nil {
		t.Fatal(err)
	}
	var rep retention.Report
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil || len(rep.Audit) != 1 || rep.Audit[0].Events != 1 || rep.Audit[0].Days != 30 {
		t.Fatalf("%s %v", out.String(), err)
	}
	as2, _ := audit.NewFile(auditDir) // the command wrote through its own handle
	v, err := audit.NewService(as2, ps, audit.Options{Retention: 90 * 24 * time.Hour, Now: func() time.Time { return now }}).Verify(ctx, "ws:"+ws, "")
	if err != nil || !v.OK || v.Checkpoint == nil || v.Checkpoint.ThroughSeq != 1 {
		t.Fatalf("verify %+v %v", v, err)
	}
}
