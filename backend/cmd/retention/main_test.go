package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

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
