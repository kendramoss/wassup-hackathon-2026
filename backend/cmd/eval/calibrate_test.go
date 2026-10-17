package main

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/triage"
)

// rowsAt makes n in-scope rows of fam at confidence c, the first k correct.
func rowsAt(fam string, c float64, n, k int) []calRow {
	out := make([]calRow, n)
	for i := range out {
		out[i] = calRow{Subfamily: fam, InScope: true, Confidence: c, Correct: i < k}
	}
	return out
}

func concat(parts ...[]calRow) []calRow {
	var out []calRow
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func TestFitFamilyBars(t *testing.T) {
	tests := []struct {
		name   string
		rows   []calRow
		bar    float64
		fitted bool
		note   string
	}{
		{"lowers the bar while accuracy holds",
			concat(rowsAt("electronics", 0.98, 20, 20), rowsAt("electronics", 0.85, 5, 4)), 0.85, true, ""},
		{"stops at the first bar below target",
			concat(rowsAt("electronics", 0.98, 20, 20), rowsAt("electronics", 0.90, 10, 7), rowsAt("electronics", 0.80, 30, 30)), 0.98, true, ""},
		{"raises the bar above the global one",
			concat(rowsAt("electronics", 0.97, 20, 20), rowsAt("electronics", 0.92, 10, 7)), 0.97, true, ""},
		{"bars with fewer than min-n cases are skipped, not fitted",
			concat(rowsAt("electronics", 0.99, 5, 4), rowsAt("electronics", 0.95, 20, 20)), 0.95, true, ""},
		{"too few cases keeps the global bar",
			rowsAt("electronics", 0.99, 19, 19), 0.92, false, "fewer than 20"},
		{"never 95% keeps the global bar",
			rowsAt("electronics", 0.99, 40, 30), 0.92, false, "no bar with ≥ 20 cases"},
		{"out of scope and failed rows do not count",
			concat(rowsAt("electronics", 0.98, 20, 20), []calRow{{Subfamily: "electronics", Confidence: 0.5}, {Subfamily: "electronics", InScope: true, Confidence: 0.4, Failed: true}}), 0.98, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fits := fitFamilyBars(tt.rows, 0.95, 20, 0.92)
			var got familyFit
			for _, f := range fits {
				if f.Subfamily == "electronics" {
					got = f
				} else if f.Fitted || f.Bar != 0.92 {
					t.Errorf("%s with no rows = %+v", f.Subfamily, f)
				}
			}
			if got.Bar != tt.bar || got.Fitted != tt.fitted || !strings.Contains(got.Note, tt.note) {
				t.Errorf("fit = %+v, want bar %.2f fitted %v note %q", got, tt.bar, tt.fitted, tt.note)
			}
		})
	}
}

func TestEvaluateBars(t *testing.T) {
	bars := triage.FamilyBars{Bars: map[string]float64{"electronics": 0.80, "other": 0.96}}
	rows := []calRow{
		{Subfamily: "electronics", InScope: true, Confidence: 0.85, Correct: true},  // family only
		{Subfamily: "electronics", InScope: true, Confidence: 0.82, Correct: false}, // family only, wrong
		{Subfamily: "electronics", InScope: true, Confidence: 0.95, Correct: true},  // both
		{Subfamily: "other", InScope: true, Confidence: 0.94, Correct: false},       // global only, wrong
		{Subfamily: "other", InScope: true, Confidence: 0.97, Correct: true},        // both
		{Subfamily: "medical", InScope: true, Confidence: 0.93, Correct: true},      // both (no family bar)
		{Subfamily: "", InScope: true, Confidence: 0.93, Correct: true},             // both (no label)
		{Subfamily: "electronics", InScope: false, Confidence: 0.99, Correct: true}, // neither: out of scope
		{Subfamily: "electronics", InScope: true, Confidence: 0.99, Failed: true},   // neither: failed call
	}
	got := evaluateBars("final", rows, bars, 0.92)
	if got.N != 9 || got.Global.Auto != 5 || got.Global.Correct != 4 || got.Family.Auto != 6 || got.Family.Correct != 5 {
		t.Fatalf("counts = %+v", got)
	}
	if got.Global.WrongAccept != 1 || got.Family.WrongAccept != 1 {
		t.Errorf("wrong accepted = %d %d", got.Global.WrongAccept, got.Family.WrongAccept)
	}
	if c := got.CoverageTest; c.OnlyFamily != 2 || c.OnlyGlobal != 1 || c.P != 1 {
		t.Errorf("coverage test = %+v", c)
	}
	if w := got.WrongAcceptTest; w.OnlyFamily != 1 || w.OnlyGlobal != 1 {
		t.Errorf("wrong-accept test = %+v", w)
	}
	if !near(got.Family.Coverage, 6.0/9) || got.Family.CoverageLo >= got.Family.Coverage || got.Family.AccuracyHi <= got.Family.Accuracy {
		t.Errorf("family Wilson = %+v", got.Family)
	}
	names := map[string]bool{}
	for _, f := range got.ByFamily {
		names[f.Subfamily] = true
	}
	if !names["(no label)"] || !names["electronics"] || len(got.ByFamily) != 4 {
		t.Errorf("by family = %+v", got.ByFamily)
	}
}

func near(a, b float64) bool { return a-b < 1e-9 && b-a < 1e-9 }

func TestJoinScope(t *testing.T) {
	dir := t.TempDir()
	results := filepath.Join(dir, "dev.jsonl")
	scope := filepath.Join(dir, "scope.jsonl")
	write := func(path string, lines ...string) {
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(results,
		`{"ruling":"N1","confidence":0.9,"correct":true}`,
		`{"ruling":"N2","confidence":0.95,"correct":false}`,
		`{"ruling":"N3","confidence":0.99,"correct":true,"error":"timeout"}`,
		`{"ruling":"N4","confidence":0.8,"correct":true}`)
	write(scope,
		`{"ruling":"N1","in_scope":true,"router":{"subfamily":"motorised"}}`,
		`{"ruling":"N2","in_scope":false,"router":{"subfamily":"other"}}`,
		`{"ruling":"N3","in_scope":true,"router":null}`)
	rows, err := joinScope(results, scope, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []calRow{
		{Ruling: "N1", Subfamily: "motorised", InScope: true, Confidence: 0.9, Correct: true},
		{Ruling: "N2", Subfamily: "other", InScope: false, Confidence: 0.95},
		{Ruling: "N3", InScope: true, Confidence: 0.99, Failed: true},
		{Ruling: "N4", InScope: true, Confidence: 0.8, Correct: true}, // no router label: in scope, no subfamily
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %+v", rows)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, rows[i], want[i])
		}
	}
}

// The bars the server embeds must be the ones the committed calibration run
// fitted (regenerate both with `go run ./cmd/eval calibrate`).
func TestEmbeddedBarsMatchCalibration(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "eval", "results", "calibration-by-family.json"))
	if err != nil {
		t.Skip("no calibration result:", err)
	}
	var cal calibration
	if err := json.Unmarshal(b, &cal); err != nil {
		t.Fatal(err)
	}
	emb, err := triage.LoadFamilyBars()
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(cal.Bars, emb.Bars) || emb.Rule != cal.Rule {
		t.Errorf("embedded bars %v differ from calibration %v", emb.Bars, cal.Bars)
	}
}
