package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpcorpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpengine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jprules"
)

func TestDiagnoseRefusesNonDevelopmentSplitsBeforeOpeningAnything(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-data")
	rules, err := jprules.Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, split := range []string{"held_out", "synthetic", "bogus", ""} {
		_, err := diagnoseDevelopment(split, missing, rules)
		if err == nil || !strings.Contains(err.Error(), "refused") || errors.Is(err, os.ErrNotExist) {
			t.Errorf("%q: want a refusal, got %v", split, err)
		}
	}
	// the command from a directory with no data at all
	old, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	for _, split := range []string{"held_out", "synthetic", "x"} {
		err := diagnoseCommand([]string{"-split", split, "-out", "report.json"})
		if err == nil || !strings.Contains(err.Error(), "refused") {
			t.Errorf("%q: want a refusal, got %v", split, err)
		}
	}
	if _, err := os.Stat("report.json"); err == nil {
		t.Error("a refused split wrote a file")
	}
}

func writeFixture(t *testing.T, dir string) {
	t.Helper()
	rows := []map[string]any{
		{"id": "A1", "date": "2024-03-01", "hs": "8501.10", "agreement": "AJCEP"},
		{"id": "A2", "date": "2024-03-02", "hs": "0101.21", "agreement": "AJCEP"},
		{"id": "H1", "date": "2024-03-03", "hs": "7102.39", "agreement": "GSP"},
	}
	var rl, tr strings.Builder
	for _, r := range rows {
		b, _ := json.Marshal(r)
		rl.Write(b)
		rl.WriteString("\n")
		tb, _ := json.Marshal(map[string]any{"id": r["id"], "en": map[string]any{"product": "thing " + r["id"].(string)}})
		tr.Write(tb)
		tr.WriteString("\n")
	}
	must := func(name, s string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must("rulings.jsonl", rl.String())
	must("translations-en.jsonl", tr.String())
	must("splits.json", `{"dev":["A1","A2"],"dev_eval":["A1"],"held_out":["H1"]}`)
}

func TestDiagnoseNeverRetainsHeldOutRows(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir)
	rules, err := jprules.Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, split := range []string{"dev_eval", "dev"} {
		r, err := diagnoseDevelopment(split, dir, rules)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(r)
		for _, leak := range []string{"H1", "GSP", "710239", "7102"} {
			if strings.Contains(string(b), leak) {
				t.Errorf("%s: output carries held-out value %q: %s", split, leak, b)
			}
		}
	}
	r, _ := diagnoseDevelopment("dev", dir, rules)
	if r.Cases != 2 || r.DevEvalOverlap == nil || *r.DevEvalOverlap != 1 {
		t.Errorf("dev: cases=%d overlap=%v", r.Cases, r.DevEvalOverlap)
	}
	e, _ := diagnoseDevelopment("dev_eval", dir, rules)
	if e.Cases != 1 || e.DevEvalOverlap != nil {
		t.Errorf("dev_eval: cases=%d overlap=%v", e.Cases, e.DevEvalOverlap)
	}
}

func TestDiagnoseDevEvalMatchesTheRequestsActuallyPlanned(t *testing.T) {
	items, pool, _, err := loadCandidate("dev_eval")
	if err != nil {
		t.Fatal(err)
	}
	rules, err := jprules.Default()
	if err != nil {
		t.Fatal(err)
	}
	p, err := planItems("dev_eval", items, pool, rules)
	if err != nil {
		t.Fatal(err)
	}
	// per-case set from a counting fake model
	m := newPlanModel()
	eng := jpengine.New(m, jpengine.DefaultModel, pool, rules, jpengine.DefaultK)
	var called []string
	for _, it := range items {
		before := len(m.sizes)
		if _, err := eng.Predict(context.Background(), it.caseValue); err != nil && !errors.Is(err, errPlanOnly) {
			t.Fatal(err)
		}
		if len(m.sizes) > before {
			called = append(called, it.caseValue.ID)
		}
	}
	d, err := diagnoseDevelopment("dev_eval", dataDir, rules)
	if err != nil {
		t.Fatal(err)
	}
	if d.Eligible.Total != p.Calls || d.Eligible.PSR+d.Eligible.General != p.Calls || d.Abstained != p.AbstainedBeforeCall || d.Cases != p.Cases {
		t.Errorf("diagnose %+v abstained=%d cases=%d vs plan calls=%d abstained=%d cases=%d", d.Eligible, d.Abstained, d.Cases, p.Calls, p.AbstainedBeforeCall, p.Cases)
	}
	got := slices.Clone(d.eligibleCaseIDs)
	slices.Sort(got)
	slices.Sort(called)
	if !slices.Equal(got, called) {
		t.Errorf("eligible id sets differ: diagnose %d, planned %d", len(got), len(called))
	}
	sum := 0
	for _, byReason := range d.ReasonByAgree {
		for _, n := range byReason {
			sum += n
		}
	}
	if sum != d.Cases {
		t.Errorf("reason counts sum to %d, not %d", sum, d.Cases)
	}
}

func TestDiagnoseCasesCarryTheSameFactsAsLoadCandidate(t *testing.T) {
	items, _, _, err := loadCandidate("dev_eval")
	if err != nil {
		t.Fatal(err)
	}
	c, err := jpcorpus.Load(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	sp, err := jpcorpus.LoadSplits(filepath.Join(dataDir, "splits.json"))
	if err != nil {
		t.Fatal(err)
	}
	cases, err := diagnoseCases(c, sp.DevEval)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != len(items) {
		t.Fatalf("%d cases vs %d", len(cases), len(items))
	}
	for i, it := range items {
		a, b := cases[i], it.caseValue
		if a.ID != b.ID || a.Facts != b.Facts || !a.Date.Equal(b.Date) || *a.Japan != *b.Japan || a.Product != nil {
			t.Fatalf("case %s differs from loadCandidate", b.ID)
		}
	}
}

func TestDiagnoseIsDeterministicAndExclusiveCreate(t *testing.T) {
	rules, err := jprules.Default()
	if err != nil {
		t.Fatal(err)
	}
	a, err := diagnoseDevelopment("dev_eval", dataDir, rules)
	if err != nil {
		t.Fatal(err)
	}
	b, err := diagnoseDevelopment("dev_eval", dataDir, rules)
	if err != nil {
		t.Fatal(err)
	}
	ja, _ := json.MarshalIndent(a, "", " ")
	jb, _ := json.MarshalIndent(b, "", " ")
	if string(ja) != string(jb) {
		t.Error("two runs differ")
	}
	if a.Provenance.RealInference || a.Provenance.GatewayInitialized {
		t.Error("provenance claims inference")
	}
	out := filepath.Join(t.TempDir(), "d.json")
	devnull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	oldStdout := os.Stdout
	os.Stdout = devnull
	defer func() { os.Stdout = oldStdout }()
	if err := diagnoseCommand([]string{"-out", out}); err != nil {
		t.Fatal(err)
	}
	if err := diagnoseCommand([]string{"-out", out}); err == nil {
		t.Error("second write to the same path succeeded")
	}
	for _, p := range []string{"../data/jp/x.json", "../eval/jp/x.json"} {
		if err := diagnoseCommand([]string{"-out", p}); err == nil || !strings.Contains(err.Error(), "never written") {
			t.Errorf("%s: %v", p, err)
		}
	}
}
