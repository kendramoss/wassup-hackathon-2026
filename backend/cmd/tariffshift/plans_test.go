package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
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

// The recorded Part 102 plan page recomputes byte for byte (D532).
func TestPlans102ExampleCheck(t *testing.T) {
	t.Chdir("../..")
	if err := cmdPlans102Example([]string{"-check"}); err != nil {
		t.Fatal(err)
	}
}

// D532: the Part 102 fixtures name the product, so the facts a plan search
// renders from them no longer read "a The country of origin of ...".
func TestPart102FixtureFactsReadCleanly(t *testing.T) {
	tmpls, err := planner.LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	paths, _ := filepath.Glob("../../testdata/planner_part102/*.json")
	if len(paths) != 3 {
		t.Fatalf("want 3 fixtures, got %d", len(paths))
	}
	for _, path := range paths {
		r, err := planner.LoadRequest(path)
		if err != nil {
			t.Fatal(err)
		}
		facts := planner.Render(r.Product, tmpls[r.Family])
		if l := strings.ToLower(facts); strings.Contains(l, "country of origin") || strings.Contains(facts, " a The ") || strings.Contains(facts, "The The ") {
			t.Errorf("%s: facts still carry a ruling subject:\n%s", filepath.Base(path), facts)
		}
	}
}
