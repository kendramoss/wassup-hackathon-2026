package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

func TestMemory(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	if _, err := m.Get(ctx, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get missing = %v", err)
	}
	if err := m.Put(ctx, Case{ID: "x", Facts: "f"}); err != nil {
		t.Fatal(err)
	}
	c, err := m.Get(ctx, "x")
	if err != nil || c.Facts != "f" {
		t.Fatalf("Get = %+v, %v", c, err)
	}
}

func TestStoresListNewestFirstAndFilePersists(t *testing.T) {
	dir := t.TempDir()
	f, err := NewFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	for _, s := range []Cases{NewMemory(), f} {
		_ = s.Put(ctx, Case{ID: "case_a", UpdatedAt: now.Add(-time.Hour)})
		_ = s.Put(ctx, Case{ID: "case_b", UpdatedAt: now, Plan: &planner.Result{}})
		cs, err := s.List(ctx)
		if err != nil || len(cs) != 2 || cs[0].ID != "case_b" || cs[0].Step() != "plan" || cs[1].Step() != "product" {
			t.Errorf("%T list = %+v %v", s, cs, err)
		}
	}
	f2, _ := NewFile(dir) // a restart
	if c, err := f2.Get(ctx, "case_b"); err != nil || c.Plan == nil {
		t.Errorf("after restart: %+v %v", c, err)
	}
	if _, err := f2.Get(ctx, "../etc/passwd"); !errors.Is(err, ErrNotFound) {
		t.Errorf("path traversal not rejected: %v", err)
	}
}
