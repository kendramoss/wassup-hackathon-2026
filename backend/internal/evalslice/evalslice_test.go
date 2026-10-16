package evalslice

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
)

func readFixture(t *testing.T, name string) []evalkit.Row {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var rows []evalkit.Row
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var r evalkit.Row
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		rows = append(rows, r)
	}
	return rows
}

var defaultTol = Tolerance{MaxDrop: 2, BandMin: 0.95}

func TestCompareFixtures(t *testing.T) {
	base, err := LoadBaseline("testdata/baseline.json")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		file             string
		tol              Tolerance
		pass             bool
		accPass, bandOK  bool
		correct, errors  int
		band95N, band95K int
		fixed, broken    []string
	}{
		{"same.jsonl", defaultTol, true, true, true, 5, 0, 4, 4, nil, nil},
		// Two fewer correct is inside the tolerance; one more is not.
		{"drop2.jsonl", defaultTol, true, true, true, 3, 0, 3, 3, nil, []string{"N100004", "N100005"}},
		{"drop3.jsonl", defaultTol, false, false, true, 2, 0, 2, 2, nil, []string{"N100003", "N100004", "N100005"}},
		{"drop2.jsonl", Tolerance{MaxDrop: 0, BandMin: 0.95}, false, false, true, 3, 0, 3, 3, nil, []string{"N100004", "N100005"}},
		// Same accuracy, but a wrong answer now sits at ≥ 0.95: 5/6 < 95%.
		{"overconfident.jsonl", defaultTol, false, true, false, 5, 0, 6, 5, nil, nil},
		{"overconfident.jsonl", Tolerance{MaxDrop: 2, BandMin: 0.8}, true, true, true, 5, 0, 6, 5, nil, nil},
		// A failed call and a missing case count as wrong; an extra ruling is ignored.
		{"errors.jsonl", defaultTol, true, true, true, 4, 2, 2, 2, []string{"N100006"}, []string{"N100003", "N100004"}},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s drop%d band%.2f", tt.file, tt.tol.MaxDrop, tt.tol.BandMin), func(t *testing.T) {
			rep := Compare(base, readFixture(t, tt.file), tt.tol)
			if rep.Passed() != tt.pass || rep.Checks[0].Pass != tt.accPass || rep.Checks[1].Pass != tt.bandOK {
				t.Errorf("passed=%v checks=%+v, want %v (accuracy %v, band %v)", rep.Passed(), rep.Checks, tt.pass, tt.accPass, tt.bandOK)
			}
			if rep.N != 6 || rep.Correct != tt.correct || rep.Errors != tt.errors || rep.Band95N != tt.band95N || rep.Band95K != tt.band95K {
				t.Errorf("report = %+v", rep)
			}
			if !slices.Equal(rep.Fixed, tt.fixed) || !slices.Equal(rep.Broken, tt.broken) {
				t.Errorf("fixed %v broken %v, want %v %v", rep.Fixed, rep.Broken, tt.fixed, tt.broken)
			}
			md := rep.Markdown("t", base)
			if want := map[bool]string{true: "PASS", false: "FAIL"}[tt.pass]; !strings.Contains(md, "## t: "+want) {
				t.Errorf("markdown:\n%s", md)
			}
		})
	}
}

func TestCompareEmptyBandFails(t *testing.T) {
	base, err := LoadBaseline("testdata/baseline.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []evalkit.Row
	for _, c := range base.Cases {
		rows = append(rows, evalkit.Row{Ruling: c.Ruling, Pred: c.Gold, Gold: c.Gold, Correct: true, Confidence: 0.9})
	}
	rep := Compare(base, rows, defaultTol)
	if rep.Checks[0].Pass != true || rep.Checks[1].Pass || !strings.Contains(rep.Checks[1].Detail, "fast path would be empty") {
		t.Errorf("all-correct run with no confident case: %+v", rep.Checks)
	}
}

func row(id string, conf float64, correct bool) evalkit.Row {
	return evalkit.Row{Ruling: id, Split: "dev", Gold: "China", Pred: "China", Correct: correct, Confidence: conf}
}

func TestPickStratifiesAndSkips(t *testing.T) {
	var rows []evalkit.Row
	for i := range 60 {
		rows = append(rows, row(fmt.Sprintf("N2%05d", i), 0.97, i%10 != 0))
	}
	for i := range 20 {
		rows = append(rows, row(fmt.Sprintf("N3%05d", i), 0.92, true))
	}
	for i := range 8 {
		rows = append(rows, row(fmt.Sprintf("N4%05d", i), 0.85, false))
	}
	rows = append(rows, row("N500001", 0.5, false)) // the only < 0.80 row
	errRow := row("N500002", 0.5, false)
	errRow.Error = "timeout"
	final := row("N500003", 0.5, true)
	final.Split = "final"
	rows = append(rows, errRow, final, row("N500004", 0.5, true), row("N500005", 0.5, true))
	adj := evalkit.Adjustments{Exclude: map[string]string{"N500004": "multi-scenario"}, Alternatives: map[string][]string{"N500005": {"China", "Vietnam"}}}

	s, err := Pick(rows, adj, 10)
	if err != nil {
		t.Fatal(err)
	}
	var ns, eligible []int
	for _, b := range s.Bands {
		ns, eligible = append(ns, b.N), append(eligible, b.Eligible)
	}
	if !slices.Equal(eligible, []int{60, 20, 8, 1}) || !slices.Equal(ns, []int{6, 2, 1, 1}) {
		t.Errorf("eligible %v, n %v", eligible, ns)
	}
	if len(s.IDs) != 10 || !slices.Contains(s.IDs, "N500001") {
		t.Errorf("ids = %v", s.IDs)
	}
	for _, id := range []string{"N500002", "N500003", "N500004", "N500005"} {
		if slices.Contains(s.IDs, id) {
			t.Errorf("%s (error, final or adjusted) was picked", id)
		}
	}
	reversed := slices.Clone(rows)
	slices.Reverse(reversed)
	again, _ := Pick(reversed, adj, 10) // input order does not matter
	if !slices.Equal(slices.Sorted(slices.Values(again.IDs)), slices.Sorted(slices.Values(s.IDs))) {
		t.Errorf("pick depends on row order: %v vs %v", again.IDs, s.IDs)
	}
	// Correctness does not steer the pick.
	flipped := slices.Clone(rows)
	for i := range flipped {
		flipped[i].Correct = !flipped[i].Correct
	}
	if f, _ := Pick(flipped, adj, 10); !slices.Equal(f.IDs, s.IDs) {
		t.Errorf("pick depends on correctness")
	}
	if _, err := Pick(rows[:5], adj, 10); err == nil {
		t.Error("too few rows accepted")
	}

	base, err := BaselineFrom(rows, s, "fixture.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if base.N != 10 || len(base.Cases) != 10 || SameRulings(s, base) != nil {
		t.Errorf("baseline = %+v", base)
	}
	if rep := Compare(base, rows, Tolerance{MaxDrop: 0, BandMin: 0.5}); rep.Correct != base.Correct || len(rep.Broken) != 0 {
		t.Errorf("source run against its own baseline: %+v", rep)
	}
	if _, err := BaselineFrom(rows[:3], s, "short.jsonl"); err == nil {
		t.Error("baseline from a run missing slice rulings accepted")
	}
}

func TestAllocate(t *testing.T) {
	tests := []struct {
		eligible []int
		n        int
		want     []int
	}{
		{[]int{332, 85, 28, 17}, 30, []int{22, 5, 2, 1}}, // the committed slice
		{[]int{10, 10, 10, 0}, 6, []int{2, 2, 2, 0}},
		{[]int{100, 1, 1, 1}, 5, []int{2, 1, 1, 1}}, // minimums taken back from the largest band
		{[]int{3, 0, 0, 0}, 3, []int{3, 0, 0, 0}},
	}
	for _, tt := range tests {
		bands := Bands()
		for i, e := range tt.eligible {
			bands[i].Eligible = e
		}
		if got := allocate(bands, tt.n); !slices.Equal(got, tt.want) {
			t.Errorf("allocate(%v, %d) = %v, want %v", tt.eligible, tt.n, got, tt.want)
		}
	}
}

func TestLoadRejectsMismatch(t *testing.T) {
	base, err := LoadBaseline("testdata/baseline.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := SameRulings(Slice{IDs: []string{"N100001"}}, base); err == nil {
		t.Error("slice/baseline mismatch accepted")
	}
	dir := t.TempDir()
	bad := filepath.Join(dir, "b.json")
	_ = WriteJSON(bad, Baseline{N: 3, Cases: []Case{{Ruling: "N1"}}})
	if _, err := LoadBaseline(bad); err == nil {
		t.Error("baseline with n != len(cases) accepted")
	}
	empty := filepath.Join(dir, "s.json")
	_ = WriteJSON(empty, Slice{})
	if _, err := LoadSlice(empty); err == nil {
		t.Error("empty slice accepted")
	}
}

// The committed slice and baseline must agree and stay dev-only.
func TestCommittedSlice(t *testing.T) {
	s, err := LoadSlice("../../../eval/splits/slice30.json")
	if err != nil {
		t.Fatal(err)
	}
	base, err := LoadBaseline("../../../eval/results/slice30-baseline.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.IDs) != 30 || SameRulings(s, base) != nil {
		t.Fatalf("slice %d rulings; baseline mismatch: %v", len(s.IDs), SameRulings(s, base))
	}
	var splits struct {
		Final, Dev []string
	}
	b, err := os.ReadFile("../../../eval/splits/splits_final.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &splits); err != nil {
		t.Fatal(err)
	}
	for _, id := range s.IDs {
		if slices.Contains(splits.Final, id) || !slices.Contains(splits.Dev, id) {
			t.Errorf("%s is not a dev ruling", id)
		}
		// CI has only the bundled corpus (the raw corpus is not in git).
		if _, err := os.Stat(filepath.Join("../../data/facts", id+".txt")); err != nil {
			t.Errorf("%s has no bundled facts: %v", id, err)
		}
	}
	if Compare(base, nil, defaultTol).Passed() {
		t.Error("an empty run passed")
	}
}
