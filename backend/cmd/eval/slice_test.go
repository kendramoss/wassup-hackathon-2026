package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalslice"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
)

const fixtures = "../../internal/evalslice/testdata/"

func TestSliceCmdComparesFiles(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "") // -in needs no provider or project
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	base, err := evalslice.LoadBaseline(fixtures + "baseline.json")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	slicePath := filepath.Join(dir, "slice.json")
	ids := make([]string, len(base.Cases))
	for i, c := range base.Cases {
		ids[i] = c.Ruling
	}
	if err := evalslice.WriteJSON(slicePath, evalslice.Slice{IDs: ids}); err != nil {
		t.Fatal(err)
	}
	args := func(in string) []string {
		return []string{"-slice", slicePath, "-baseline", fixtures + "baseline.json", "-in", fixtures + in}
	}
	tests := []struct {
		in, maxDrop string
		fail        bool
	}{
		{"same.jsonl", "", false},
		{"drop2.jsonl", "", false},
		{"drop3.jsonl", "", true},
		{"drop3.jsonl", "3", false},
		{"overconfident.jsonl", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.in+tt.maxDrop, func(t *testing.T) {
			t.Setenv("EVAL_SLICE_MAX_DROP", tt.maxDrop)
			err := sliceCmd(args(tt.in))
			if got := errors.Is(err, errSliceFailed); got != tt.fail || (err != nil && !got) {
				t.Errorf("err = %v, want failure %v", err, tt.fail)
			}
		})
	}
	// A slice the baseline does not cover is refused before comparing.
	other := filepath.Join(dir, "other.json")
	_ = evalslice.WriteJSON(other, evalslice.Slice{IDs: []string{"N100001"}})
	if err := sliceCmd([]string{"-slice", other, "-baseline", fixtures + "baseline.json", "-in", fixtures + "same.jsonl"}); err == nil || !strings.Contains(err.Error(), "same rulings") {
		t.Errorf("mismatched slice: %v", err)
	}
	// The live run spends model calls: off unless EVAL_SLICE=on.
	t.Setenv("LLM_PROVIDER", "fake")
	if err := sliceCmd([]string{"-slice", slicePath, "-baseline", fixtures + "baseline.json"}); err == nil || !strings.Contains(err.Error(), "EVAL_SLICE=on") {
		t.Errorf("live run without the flag: %v", err)
	}
}

// The live path: every slice case goes through the production engine setup
// and comes back in slice order, ready for the comparison.
func TestSliceRunCasesWithFakeModel(t *testing.T) {
	str := func(s string) *string { return &s }
	labels := []corpus.Label{
		{Ruling: "N100001", Date: "2024-01-01", Regime: "substantial_transformation", PrimaryOrigin: str("China"), Evidence: []string{"China."}},
		{Ruling: "N100002", Date: "2025-08-01", Regime: "substantial_transformation", PrimaryOrigin: str("China")},
		{Ruling: "N100003", Date: "2025-09-01", Regime: "substantial_transformation", PrimaryOrigin: str("Vietnam")},
	}
	dir := t.TempDir()
	for _, l := range labels {
		if err := os.WriteFile(filepath.Join(dir, l.Ruling+".txt"), []byte("motor made in China, assembled in Vietnam"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	store, err := corpus.Load(labels, dir)
	if err != nil {
		t.Fatal(err)
	}
	model := llm.FakeText(`{"origin_country":"China","essential_character":"motor","decisive_step":"motor made in China","flip":null,"confidence":0.97,"reasoning":"See N100001."}`)
	cfg := config.Config{LLMProvider: "fake", EngineModel: "gemini-4-argon", PrecedentK: 5, FastPathConfidence: 0.92}
	eng := engine.New(model, retrieval.NewBM25(store.Rulings(), engine.PrecedentText), engine.Options{Model: cfg.EngineModel, K: cfg.PrecedentK})
	var cases []domain.Ruling
	for _, id := range []string{"N100003", "N100002"} {
		r, _ := store.Get(id)
		cases = append(cases, r)
	}
	ctx := context.Background()
	rows := runCases(ctx, cases, 2, func(c domain.Ruling) evalkit.Row { return evaluate(ctx, eng, nil, cfg, store, c, "slice30") })
	if len(rows) != 2 || rows[0].Ruling != "N100003" || rows[1].Ruling != "N100002" {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[0].Correct || !rows[1].Correct || rows[1].Split != "slice30" || rows[1].PromptVersion != engine.PromptVersion || rows[1].Error != "" {
		t.Errorf("rows = %+v", rows)
	}
	if len(model.Calls()) != 2 {
		t.Errorf("model calls = %d", len(model.Calls()))
	}
	path := filepath.Join(t.TempDir(), "run.jsonl")
	if err := writeRows(path, rows); err != nil {
		t.Fatal(err)
	}
	back, err := readRows(path, nil)
	if err != nil || len(back) != 2 {
		t.Fatalf("read back %d rows: %v", len(back), err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if r := runCases(canceled, cases, 1, func(domain.Ruling) evalkit.Row { t.Fatal("ran after cancel"); return evalkit.Row{} }); r[0].Error == "" {
		t.Errorf("cancelled run: %+v", r)
	}
}
