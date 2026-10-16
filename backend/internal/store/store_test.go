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

func TestStoresDeleteAndPinnedFirst(t *testing.T) {
	f, err := NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	for _, s := range []Cases{NewMemory(), f} {
		_ = s.Put(ctx, Case{ID: "case_demo", UpdatedAt: now.Add(-time.Hour), Pinned: true})
		_ = s.Put(ctx, Case{ID: "case_new", UpdatedAt: now})
		_ = s.Put(ctx, Case{ID: "case_old", UpdatedAt: now.Add(-2 * time.Hour)})
		cs, _ := s.List(ctx)
		if len(cs) != 3 || cs[0].ID != "case_demo" || cs[1].ID != "case_new" {
			t.Errorf("%T order = %s, %s", s, cs[0].ID, cs[1].ID)
		}
		if err := s.Delete(ctx, "case_new"); err != nil {
			t.Errorf("%T delete: %v", s, err)
		}
		if _, err := s.Get(ctx, "case_new"); !errors.Is(err, ErrNotFound) {
			t.Errorf("%T get after delete = %v", s, err)
		}
		if err := s.Delete(ctx, "case_new"); !errors.Is(err, ErrNotFound) {
			t.Errorf("%T second delete = %v", s, err)
		}
	}
	if err := f.Delete(ctx, "../etc/passwd"); !errors.Is(err, ErrNotFound) {
		t.Errorf("path traversal not rejected: %v", err)
	}
}

func TestUpdateAppliesOnlyItsChange(t *testing.T) {
	f, err := NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	for _, s := range []Cases{NewMemory(), f} {
		if err := s.Put(ctx, Case{ID: "case_a", Facts: "f", UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Update(ctx, "missing", func(*Case) error { return nil }); !errors.Is(err, ErrNotFound) {
			t.Errorf("%T missing = %v", s, err)
		}
		if _, err := s.Update(ctx, "case_a", func(c *Case) error {
			c.Facts = "nope"
			return errors.New("stop")
		}); err == nil {
			t.Errorf("%T fn error was ignored", s)
		}
		if c, _ := s.Get(ctx, "case_a"); c.Facts != "f" {
			t.Errorf("%T fn error wrote %q", s, c.Facts)
		}
		if _, err := s.Update(ctx, "case_a", func(c *Case) error {
			c.Chat = append(c.Chat, ChatTurn{Question: "q", Answer: "a"})
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		exp := now
		if _, err := s.Update(ctx, "case_a", func(c *Case) error {
			c.ExportedAt = &exp
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		c, err := s.Get(ctx, "case_a")
		if err != nil || c.Facts != "f" || c.ExportedAt == nil || len(c.Chat) != 1 || c.Chat[0].Question != "q" {
			t.Errorf("%T merged = %+v %v", s, c, err)
		}
	}
	if _, err := f.Update(ctx, "../etc/passwd", func(*Case) error { return nil }); !errors.Is(err, ErrNotFound) {
		t.Errorf("path traversal not rejected: %v", err)
	}
}
