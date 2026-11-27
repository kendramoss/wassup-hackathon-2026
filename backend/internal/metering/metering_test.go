package metering

import (
	"context"
	"errors"
	"testing"
	"time"
)

func clock(s string) func() time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return func() time.Time { return t }
}

func TestAddChargesOnlyTheContextWorkspace(t *testing.T) {
	st := NewMemory()
	m := New(st).WithClock(clock("2026-11-27T10:00:00Z"))
	ctx := context.Background()
	m.Add(ctx, Delta{Predictions: 1, ModelUSD: 1}) // no workspace: dropped
	wctx := WithWorkspace(ctx, "ws1")
	m.Add(wctx, Delta{Predictions: 1})
	m.Charge(wctx, 0.25)
	m.Charge(wctx, -3) // negative ignored
	m.Add(wctx, Delta{PlanSearches: 1, ModelUSD: 0.5})
	s, err := m.Status(ctx, "ws1", "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	want := Totals{Month: "2026-11", Predictions: 1, PlanSearches: 1, ModelUSD: 0.75}
	if s.Totals != want {
		t.Fatalf("totals %+v, want %+v", s.Totals, want)
	}
	if s.Budget != nil || s.RemainingUSD != nil || s.Blocked || s.Alert {
		t.Fatalf("no budget expected, got %+v", s)
	}
	other, _ := m.Status(ctx, "ws2", "2026-11")
	if other.Totals.ModelUSD != 0 {
		t.Fatalf("ws2 charged: %+v", other)
	}
}

func TestMonthsAreSeparate(t *testing.T) {
	st := NewMemory()
	m := New(st).WithClock(clock("2026-11-30T23:59:00Z"))
	ctx := WithWorkspace(context.Background(), "w")
	m.Charge(ctx, 1)
	m.WithClock(clock("2026-12-01T00:01:00Z"))
	m.Charge(ctx, 2)
	nov, _ := m.Status(ctx, "w", "2026-11")
	dec, _ := m.Status(ctx, "w", "2026-12")
	if nov.Totals.ModelUSD != 1 || dec.Totals.ModelUSD != 2 {
		t.Fatalf("nov %v dec %v", nov.Totals.ModelUSD, dec.Totals.ModelUSD)
	}
}

func TestBudgetAlertAndHardCap(t *testing.T) {
	st := NewMemory()
	m := New(st).WithClock(clock("2026-11-27T10:00:00Z"))
	ctx := WithWorkspace(context.Background(), "w")
	if err := st.SetBudget(ctx, "w", Budget{MonthlyUSD: 10, HardCap: true}); err != nil {
		t.Fatal(err)
	}
	m.Charge(ctx, 7.9)
	s, _ := m.Status(ctx, "w", "2026-11")
	if s.Alert || s.Blocked || *s.RemainingUSD != 2.1 {
		t.Fatalf("at 7.9: %+v rem %v", s, *s.RemainingUSD)
	}
	if !s.WouldExceed(2.2) || s.WouldExceed(2.0) {
		t.Fatal("WouldExceed wrong around the remaining 2.1")
	}
	m.Charge(ctx, 0.2)
	s, _ = m.Status(ctx, "w", "2026-11")
	if !s.Alert || s.Blocked {
		t.Fatalf("at 8.1 want alert only: %+v", s)
	}
	m.Charge(ctx, 2)
	s, _ = m.Status(ctx, "w", "2026-11")
	if !s.Blocked || *s.RemainingUSD != 0 {
		t.Fatalf("at 10.1 want blocked, remaining 0: %+v", s)
	}
	// Soft budget: reported, never blocking.
	_ = st.SetBudget(ctx, "w", Budget{MonthlyUSD: 10, AlertPercent: 50})
	s, _ = m.Status(ctx, "w", "2026-11")
	if s.Blocked || !s.Alert || s.WouldExceed(100) {
		t.Fatalf("soft budget: %+v", s)
	}
}

func TestBudgetValidation(t *testing.T) {
	for _, b := range []Budget{{MonthlyUSD: 0}, {MonthlyUSD: -1}, {MonthlyUSD: MaxMonthlyUSD + 1}, {MonthlyUSD: 5, AlertPercent: 101}, {MonthlyUSD: 5, AlertPercent: -1}} {
		if err := NewMemory().SetBudget(context.Background(), "w", b); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v: want ErrInvalid, got %v", b, err)
		}
	}
	if err := (Budget{MonthlyUSD: 5, AlertPercent: 100}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestParseMonth(t *testing.T) {
	for _, ok := range []string{"2026-01", "2026-12"} {
		if _, err := ParseMonth(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"2026-13", "2026-1", "26-01", "2026/01", ""} {
		if _, err := ParseMonth(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q accepted", bad)
		}
	}
}

type failing struct{ *Memory }

func (failing) Add(context.Context, string, string, Delta) error { return errors.New("down") }

func TestFailedWritesAreCounted(t *testing.T) {
	m := New(failing{NewMemory()})
	m.Charge(WithWorkspace(context.Background(), "w"), 1)
	if m.Failed() != 1 {
		t.Fatalf("failed = %d", m.Failed())
	}
	var nilMeter *Meter
	nilMeter.Add(context.Background(), Delta{Predictions: 1}) // must not panic
}

func TestCrossingTellsOncePerLevel(t *testing.T) {
	st := NewMemory()
	var got []Crossing
	m := New(st).WithClock(clock("2026-11-27T10:00:00Z")).OnCross(func(_ context.Context, c Crossing) { got = append(got, c) })
	ctx := WithWorkspace(context.Background(), "w")
	m.Charge(ctx, 9) // no budget yet: nothing to cross
	if err := st.SetBudget(ctx, "w", Budget{MonthlyUSD: 20, AlertPercent: 80}); err != nil {
		t.Fatal(err)
	}
	m.Charge(ctx, 6) // 15 of 20: below the alert at 16
	if len(got) != 0 {
		t.Fatalf("told early: %+v", got)
	}
	m.Charge(ctx, 2) // 17: passes 16
	if len(got) != 1 || got[0].Level != LevelAlert || got[0].SpentUSD != 17 || got[0].Month != "2026-11" || got[0].Workspace != "w" || got[0].Budget.MonthlyUSD != 20 {
		t.Fatalf("alert: %+v", got)
	}
	m.Charge(ctx, 1) // 18: nothing new
	m.Charge(ctx, 5) // 23: passes 20
	if len(got) != 2 || got[1].Level != LevelCap || got[1].SpentUSD != 23 {
		t.Fatalf("cap: %+v", got)
	}
	m.Charge(ctx, 5)
	if len(got) != 2 {
		t.Fatalf("told again: %+v", got)
	}
	// One charge that jumps both levels tells both, alert first.
	got = nil
	st2 := NewMemory()
	m2 := New(st2).WithClock(clock("2026-11-27T10:00:00Z")).OnCross(func(_ context.Context, c Crossing) { got = append(got, c) })
	ctx2 := WithWorkspace(context.Background(), "x")
	_ = st2.SetBudget(ctx2, "x", Budget{MonthlyUSD: 10})
	m2.Charge(ctx2, 12)
	if len(got) != 2 || got[0].Level != LevelAlert || got[1].Level != LevelCap {
		t.Fatalf("jump: %+v", got)
	}
	// A new month starts again; a charge without a workspace is no crossing.
	got = nil
	m2.WithClock(clock("2026-12-01T00:00:01Z"))
	m2.Charge(ctx2, 9)
	m2.Charge(context.Background(), 50)
	if len(got) != 1 || got[0].Level != LevelAlert || got[0].Month != "2026-12" {
		t.Fatalf("next month: %+v", got)
	}
}
