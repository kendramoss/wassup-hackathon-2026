package main

import (
	"strings"
	"testing"
)

// The per-move count reads the development suggestion run only (D440).
func TestPlansEvalReadsOnlyTheDevelopmentRun(t *testing.T) {
	if !strings.HasSuffix(plansSuggest, "/suggest-dev-v1.jsonl") || strings.Contains(plansSuggest, "final") {
		t.Fatalf("plans-eval reads %s", plansSuggest)
	}
}

// The committed rows recompute byte for byte.
func TestPlansEvalCheck(t *testing.T) {
	t.Chdir("../..") // the command runs from backend/
	if err := cmdPlansEval([]string{"-check"}); err != nil {
		t.Fatal(err)
	}
}
