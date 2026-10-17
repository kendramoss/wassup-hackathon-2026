package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalsummary"
)

func writeFlips(t *testing.T, dir, name string, rows ...calibrationRow) {
	t.Helper()
	var b strings.Builder
	for _, r := range rows {
		j, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(j)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func row(id string, changed, supported bool, conf float64) calibrationRow {
	return calibrationRow{ID: id, OriginChanged: changed, Changes: supported, Supported: supported, Confidence: conf}
}

func calSplit(t *testing.T, c *evalsummary.VerifierCalibration, name string) evalsummary.VerifierSplit {
	t.Helper()
	for _, s := range c.Splits {
		if s.Split == name {
			return s
		}
	}
	t.Fatalf("no %s split in %+v", name, c)
	return evalsummary.VerifierSplit{}
}

func TestCalibrationBandEdges(t *testing.T) {
	dir := t.TempDir()
	writeFlips(t, dir, "flips-dev-verifier-v2-known-a-litellm.jsonl",
		row("a", true, true, 1.0),   // ≥ 0.95, includes 1.0
		row("b", true, true, 0.95),  // ≥ 0.95
		row("c", false, true, 0.92), // 0.92–0.95: the bar is inclusive
		row("d", true, true, 0.9199),
		row("e", true, true, 0.85),
		row("f", true, true, 0.8499),
		row("g", true, false, 0.99), // not supported: in no band, not shown
	)
	c, err := verifierCalibration(dir, 0.92)
	if err != nil {
		t.Fatal(err)
	}
	dev := calSplit(t, c, "dev")
	want := []struct{ k, n int }{{2, 2}, {0, 1}, {2, 2}, {1, 1}}
	for i, w := range want {
		if got := dev.Bands[i].Correct; got.K != w.k || got.N != w.n {
			t.Errorf("band %s = %d/%d, want %d/%d", dev.Bands[i].Label, got.K, got.N, w.k, w.n)
		}
	}
	if dev.Bands[0].Max != 1 || dev.Bands[1].Min != 0.92 {
		t.Errorf("band bounds = %+v", dev.Bands)
	}
	if dev.Shown.Real != 2 || dev.Shown.Wrong != 1 || dev.Shown.Recall.N != 6 {
		t.Errorf("shown = %+v", dev.Shown)
	}
	if !dev.DevelopmentOnly || dev.Caveat != "" || c.Prompt != "verifier-v2" || !strings.Contains(c.Note, "LiteLLM") {
		t.Errorf("labels: %+v %+v", dev, c)
	}
}

func TestCalibrationWilson(t *testing.T) {
	r := rate(9, 10)
	if r.Value != 0.9 || math.Abs(r.CILow-0.5958) > 0.0001 || math.Abs(r.CIHigh-0.9821) > 0.0001 {
		t.Errorf("Wilson 9/10 = %+v", r)
	}
	if z := rate(0, 0); z.Value != 0 || z.CILow != 0 || z.CIHigh != 0 {
		t.Errorf("0/0 = %+v", z)
	}
}

func TestCalibrationPairLevelRule(t *testing.T) {
	dir := t.TempDir()
	writeFlips(t, dir, "flips-dev-verifier-v2-known-a-litellm.jsonl",
		row("always", true, true, 0.96), row("once", true, true, 0.96), row("neg", false, false, 0.5))
	writeFlips(t, dir, "flips-dev-verifier-v2-run2-known-a-litellm.jsonl",
		row("always", true, true, 0.93), row("once", true, true, 0.90), row("neg", false, true, 0.94))
	c, err := verifierCalibration(dir, 0.92)
	if err != nil {
		t.Fatal(err)
	}
	p := calSplit(t, c, "dev").PairLevel
	if p.Pairs != 3 || p.RealChangePairs != 2 || p.ShownEveryRun != 1 || p.ShownSomeRuns != 1 || p.WrongAnyRun != 1 {
		t.Errorf("pair level = %+v", p)
	}
	if p.Precision.K != 1 || p.Precision.N != 2 || p.Recall.K != 1 || p.Recall.N != 2 {
		t.Errorf("pair rates = %+v %+v", p.Precision, p.Recall)
	}
	if s := calSplit(t, c, "dev"); s.Rows != 6 || len(s.Runs) != 2 || s.Runs[1].Variant != "run2" || s.Runs[0].Variant != "base" {
		t.Errorf("runs = %+v rows %d", s.Runs, s.Rows)
	}
}

func TestCalibrationSplitsAndFiles(t *testing.T) {
	dir := t.TempDir()
	writeFlips(t, dir, "flips-dev-verifier-v2-seed-run1-known-a-litellm.jsonl", row("d1", true, true, 0.96))
	writeFlips(t, dir, "flips-final-verifier-v2-seed-unknown-a-litellm.jsonl", row("f1", true, true, 0.96), row("f2", false, true, 0.97))
	writeFlips(t, dir, "flips-final-verifier-v2-known-a-litellm.jsonl",
		row("f1", true, true, 0.80), calibrationRow{ID: "f2", Error: "504"})
	// verifier-v1 and v3 files are not v2: left out.
	writeFlips(t, dir, "flips-dev-verifier-known-a-litellm.jsonl", row("x", false, true, 0.99))
	writeFlips(t, dir, "flips-dev-verifier-v3-run1-known-a-litellm.jsonl", row("x", false, true, 0.99))
	c, err := verifierCalibration(dir, 0.92)
	if err != nil {
		t.Fatal(err)
	}
	dev, final := calSplit(t, c, "dev"), calSplit(t, c, "final")
	if dev.Pairs != 1 || dev.Shown.Wrong != 0 || len(dev.Runs) != 1 || dev.Runs[0].Variant != "seed-run1" {
		t.Errorf("dev = %+v", dev)
	}
	if final.Pairs != 2 || final.Rows != 3 || final.Errors != 1 || final.Shown.Real != 1 || final.Shown.Wrong != 1 {
		t.Errorf("final = %+v", final)
	}
	if final.DevelopmentOnly || !strings.Contains(final.Caveat, "D74") {
		t.Errorf("final labels = %+v", final)
	}
	if c.Splits[0].Split != "dev" || c.Splits[1].Split != "final" {
		t.Errorf("split order = %s, %s", c.Splits[0].Split, c.Splits[1].Split)
	}
	if empty, err := verifierCalibration(t.TempDir(), 0.92); err != nil || empty != nil {
		t.Errorf("no files = %+v %v", empty, err)
	}
}
