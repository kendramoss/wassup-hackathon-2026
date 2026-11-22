package main

import (
	"math"
	"testing"
)

func rowsAt(conf float64, n, right int) []g102Row {
	out := make([]g102Row, n)
	for i := range out {
		out[i] = g102Row{Ruling: "x", Confidence: conf, Correct: i < right}
	}
	return out
}

// D610: the cal-v2 procedure pools small blocks before the isotonic fit,
// joins a short last block to the one below, and caps at 0.99.
func TestFitMapPoolsSmallBlocks(t *testing.T) {
	var rows []g102Row
	rows = append(rows, rowsAt(0.80, 12, 6)...)
	rows = append(rows, rowsAt(0.90, 10, 7)...)  // 0.80+0.90: 22 cases, 13 right
	rows = append(rows, rowsAt(0.95, 25, 25)...) // its own block, capped
	rows = append(rows, rowsAt(0.98, 5, 5)...)   // short last block joins 0.95
	got := fitMap(rows)
	if len(got) != 2 {
		t.Fatalf("got %d steps, want 2: %+v", len(got), got)
	}
	if got[0].From != 0.80 || got[0].To != 0.90 || got[0].N != 22 || got[0].Correct != 13 {
		t.Errorf("first step %+v", got[0])
	}
	if got[1].From != 0.95 || got[1].To != 0.98 || got[1].N != 30 || got[1].Calibrated != 0.99 {
		t.Errorf("second step %+v", got[1])
	}
}

// A block more accurate than the one above it is pooled with it.
func TestFitMapPoolsViolators(t *testing.T) {
	var rows []g102Row
	rows = append(rows, rowsAt(0.80, 20, 18)...)
	rows = append(rows, rowsAt(0.90, 20, 12)...)
	got := fitMap(rows)
	if len(got) != 1 || got[0].N != 40 || got[0].Calibrated != 0.75 {
		t.Fatalf("got %+v, want one pooled step at 0.75", got)
	}
}

// The bar is the lowest value at least 95% right on at least 20 cases.
func TestCurveBarNeedsTargetAndCount(t *testing.T) {
	var rows []g102Row
	rows = append(rows, rowsAt(0.98, 10, 10)...)
	rows = append(rows, rowsAt(0.95, 28, 26)...)
	_, bar := curve(rows, stated)
	if bar != nil {
		t.Fatalf("bar %v: 0.98 has 10 cases, 0.95 is 36 of 38", *bar)
	}
	rows = append(rows, rowsAt(0.99, 12, 12)...)
	_, bar = curve(rows, stated)
	if bar == nil || *bar != 0.95 {
		t.Fatalf("bar %v, want 0.95 (48 of 50; 0.98 is 22 of 22 but higher)", bar)
	}
}

func TestSpearman(t *testing.T) {
	if r := spearman([]float64{1, 2, 3, 4}, []float64{10, 20, 30, 40}); math.Abs(r-1) > 1e-9 {
		t.Errorf("monotone: %v", r)
	}
	if r := spearman([]float64{1, 2, 3, 4}, []float64{4, 3, 2, 1}); math.Abs(r+1) > 1e-9 {
		t.Errorf("reversed: %v", r)
	}
}
