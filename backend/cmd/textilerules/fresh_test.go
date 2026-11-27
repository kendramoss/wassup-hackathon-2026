package main

import "testing"

func TestWilsonKnown(t *testing.T) {
	lo, hi := wilson(13, 14)
	if lo != 68.5 || hi != 98.7 {
		t.Fatalf("wilson(13,14) = %v, %v; D952 reports 68.5-98.7", lo, hi)
	}
}

func TestFreshReportPoolsSetsAndShare(t *testing.T) {
	cells := []Cell{
		{Split: "older", Source: "facts+suggested", N: 10, Extracted: 10, OutOfScope: 4, Decided: 3, AgreeCBP: 3, Reasons: map[string]int{"x": 3}, ByPara: map[string]int{"(c)(2)": 3}},
		{Split: "pool", Source: "facts+suggested", N: 20, Extracted: 19, OutOfScope: 9, Decided: 5, AgreeCBP: 4, Reasons: map[string]int{"extraction_failed": 1, "y": 4}, ByPara: map[string]int{"(c)(2)": 5}},
	}
	got := freshReport(cells)
	if len(got) != 3 {
		t.Fatalf("want 2 per-set cells and 1 pooled, got %d", len(got))
	}
	p := got[2]
	if p.Split != "pooled" || p.N != 30 || p.Decided != 8 || p.AgreeCBP != 7 {
		t.Fatalf("pooled = %+v", p.Cell)
	}
	if p.InScope != 17 { // (10-4) + (19-9) + 1 failed extraction
		t.Fatalf("in scope = %d, want 17", p.InScope)
	}
	if p.DecidedShare != 47.1 {
		t.Fatalf("decided share = %v, want 47.1", p.DecidedShare)
	}
	if p.AgreeCBPCI == nil || p.AgreeCBPCI[0] != 52.9 {
		t.Fatalf("interval = %v", p.AgreeCBPCI)
	}
	if got[0].DecidedShare != 50 { // 3 of 6
		t.Fatalf("older share = %v", got[0].DecidedShare)
	}
}
