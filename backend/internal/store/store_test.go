package store

import (
	"context"
	"errors"
	"testing"
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
