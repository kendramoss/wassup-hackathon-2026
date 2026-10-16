package ratelimit

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemoryAddsAtomicallyAndForgetsEndedWindows(t *testing.T) {
	s := NewMemory()
	ctx := context.Background()
	t0 := time.Date(2026, 10, 17, 12, 1, 0, 0, time.UTC)
	got, err := s.Add(ctx, []Counter{{"a-1", 1}, {"global-1", 5}}, t0)
	if err != nil || got[0] != 1 || got[1] != 5 {
		t.Fatalf("first Add = %v %v", got, err)
	}
	got, _ = s.Add(ctx, []Counter{{"a-1", 1}, {"global-1", 5}}, t0)
	if got[0] != 2 || got[1] != 10 {
		t.Fatalf("second Add = %v", got)
	}
	got, _ = s.Add(ctx, []Counter{{"a-1", -1}}, t0)
	if got[0] != 1 {
		t.Fatalf("undo = %v", got)
	}
	if _, err := s.Add(ctx, []Counter{{"a-2", 1}}, t0.Add(time.Minute)); err != nil || s.Len() != 1 {
		t.Errorf("after the next window: %d counters, want only the new one (%v)", s.Len(), err)
	}
	s.Fail(ErrUnavailable)
	if _, err := s.Add(ctx, []Counter{{"a-2", 1}}, t0); !errors.Is(err, ErrUnavailable) {
		t.Errorf("failing store = %v", err)
	}
}
