package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func metric(t *testing.T, kv []KV, key string) string {
	t.Helper()
	for _, e := range kv {
		if e.Key == key {
			return e.Value
		}
	}
	t.Fatalf("metric %q not computed", key)
	return ""
}

func TestComputeMatchesHandCountedFixture(t *testing.T) {
	f := newFixture(t)
	kv, problems := Compute(f.root)
	if len(problems) != 0 {
		t.Fatalf("problems: %v", problems)
	}
	want := map[string]string{
		"final.rows":                   "10",
		"final.scored":                 "10",
		"final.correct":                "8",
		"final.accuracy_pct":           "80.0",
		"final.error_rows":             "0",
		"final.date_min":               "2026-01-02",
		"final.date_max":               "2026-09-25",
		"final.fast.n":                 "6",
		"final.fast.correct":           "5",
		"final.fast.coverage_pct":      "60.0",
		"final.fast.accuracy_pct":      "83.3",
		"final.fast_conf_only.n":       "7",
		"final.fast_conf_only.correct": "6",
		"final.out_of_scope":           "1",
		"final.band_ge95.n":            "5",
		"final.band_ge95.correct":      "5",
		"final.band_90_95.n":           "3",
		"final.band_80_90.n":           "0",
		"final.band_lt80.n":            "2",
		"dev.rows":                     "4",
		"dev.excluded":                 "1",
		"dev.scored":                   "3",
		"dev.correct":                  "2", // D2 is right only through the alternatives entry
		"labels.rows":                  "5",
		"labels.with_primary_origin":   "4",
		"labels.date_min":              "2023-01-10",
		"labels.date_max":              "2026-01-05",
		"usmca.rows":                   "3",
		"usmca.overlap_with_labels":    "1",
		"usmca.not_in_labels":          "2",
		"pool.union":                   "7",
		"facts.files":                  "7",
		"splits.final":                 "10",
		"splits.dev":                   "4",
		"splits.pilot":                 "1",
	}
	for k, v := range want {
		if got := metric(t, kv, k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

func TestCheckPassesOnConsistentFixture(t *testing.T) {
	f := newFixture(t)
	r := Check(f.root, f.doc)
	if len(r.Failures) != 0 {
		t.Fatalf("unexpected failures: %v", r.Failures)
	}
}

func hasFailure(r Report, substr string) bool {
	for _, s := range r.Failures {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

func TestCheckFailsWhenDocNumberDiffersFromRecomputed(t *testing.T) {
	f := newFixture(t)
	f.setDoc(t, strings.Replace(f.docText(t), "| `final.correct` | 8 |", "| `final.correct` | 9 |", 1))
	r := Check(f.root, f.doc)
	if !hasFailure(r, "final.correct") {
		t.Fatalf("want a final.correct mismatch, got %v", r.Failures)
	}
}

func TestCheckFailsWhenArtifactChangesUnderTheDoc(t *testing.T) {
	// Flip one engine row to wrong: counts move, the doc (and manifest) did not.
	f := newFixture(t)
	f.edit(t, "eval/results/final-gemini-4-argon-prec-engine-v1-vertex.jsonl", func(s string) string {
		return strings.Replace(s, `"F1","date":"2026-01-02","correct":true`, `"F1","date":"2026-01-02","correct":false`, 1)
	})
	r := Check(f.root, f.doc)
	if !hasFailure(r, "final.correct") {
		t.Fatalf("want recomputed mismatch, got %v", r.Failures)
	}
	if !hasFailure(r, "checksum") {
		t.Fatalf("want a manifest checksum failure too, got %v", r.Failures)
	}
}

func TestCheckFailsOnMissingArtifact(t *testing.T) {
	for _, rel := range []string{
		"eval/results/scope-final-router-v2-vertex.jsonl",
		"eval/data/labels.jsonl",
		"eval/SHA256SUMS",
		"eval/RESULTS.md",
		"backend/data/facts",
	} {
		t.Run(rel, func(t *testing.T) {
			f := newFixture(t)
			if err := os.RemoveAll(filepath.Join(f.root, filepath.FromSlash(rel))); err != nil {
				t.Fatal(err)
			}
			r := Check(f.root, f.doc)
			if len(r.Failures) == 0 {
				t.Fatal("a missing file must fail the check")
			}
			if !hasFailure(r, rel) {
				t.Fatalf("failure should name %s, got %v", rel, r.Failures)
			}
		})
	}
}

func TestCheckFailsOnMissingDoc(t *testing.T) {
	f := newFixture(t)
	if err := os.Remove(f.doc); err != nil {
		t.Fatal(err)
	}
	if r := Check(f.root, f.doc); !hasFailure(r, "claims-reconciliation.md") {
		t.Fatalf("got %v", r.Failures)
	}
}

func TestCheckFailsOnTamperedPinnedFile(t *testing.T) {
	f := newFixture(t)
	// The pinned file changes but eval/SHA256SUMS does not: tampered data.
	f.edit(t, "eval/data/labels.jsonl", func(s string) string { return s + fxLabel("Z9", "2026-01-01", "China") })
	r := Check(f.root, f.doc)
	if !hasFailure(r, "eval/SHA256SUMS") || !hasFailure(r, "data/labels.jsonl") {
		t.Fatalf("want an eval/SHA256SUMS mismatch naming data/labels.jsonl, got %v", r.Failures)
	}
}

func TestCheckFailsOnTamperedChecksumLine(t *testing.T) {
	f := newFixture(t)
	f.edit(t, "eval/SHA256SUMS", func(s string) string { return strings.Replace(s, s[:4], "0000", 1) })
	r := Check(f.root, f.doc)
	if !hasFailure(r, "eval/SHA256SUMS") {
		t.Fatalf("got %v", r.Failures)
	}
}

func TestCheckFailsOnTamperedDocManifestLine(t *testing.T) {
	f := newFixture(t)
	doc := f.docText(t)
	i := strings.Index(doc, "```\n") + 4
	f.setDoc(t, doc[:i]+"ffff"+doc[i+4:])
	r := Check(f.root, f.doc)
	if !hasFailure(r, "checksum") {
		t.Fatalf("a wrong hash in the doc manifest must fail, got %v", r.Failures)
	}
}

func TestCheckFailsWhenManifestDropsARequiredFile(t *testing.T) {
	f := newFixture(t)
	var kept []string
	for _, l := range strings.Split(f.docText(t), "\n") {
		if strings.Contains(l, "eval/results/RUNLOG.jsonl") {
			continue
		}
		kept = append(kept, l)
	}
	f.setDoc(t, strings.Join(kept, "\n"))
	if r := Check(f.root, f.doc); !hasFailure(r, "RUNLOG.jsonl") {
		t.Fatalf("got %v", r.Failures)
	}
}

func TestCheckFailsOnMissingAndUnknownKeys(t *testing.T) {
	f := newFixture(t)
	var kept []string
	for _, l := range strings.Split(f.docText(t), "\n") {
		if !strings.HasPrefix(l, "| `final.rows` |") {
			kept = append(kept, l)
		}
	}
	f.setDoc(t, strings.Join(kept, "\n")+"\n| `final.invented` | 1 | x |\n")
	r := Check(f.root, f.doc)
	if !hasFailure(r, "final.rows") || !hasFailure(r, "final.invented") {
		t.Fatalf("want missing final.rows and unknown final.invented, got %v", r.Failures)
	}
}

func TestCheckFailsWhenCalibrationArtifactDisagreesWithRows(t *testing.T) {
	f := newFixture(t)
	f.edit(t, "eval/results/calibration-by-family.json", func(s string) string {
		return strings.Replace(s, `"auto":6,"correct":5`, `"auto":7,"correct":5`, 1)
	})
	r := Check(f.root, f.doc)
	if !hasFailure(r, "calibration-by-family.json") {
		t.Fatalf("got %v", r.Failures)
	}
}

func TestCheckFailsWhenRunlogDisagreesWithRows(t *testing.T) {
	f := newFixture(t)
	f.edit(t, "eval/results/RUNLOG.jsonl", func(s string) string { return strings.Replace(s, `"correct":8`, `"correct":9`, 1) })
	if r := Check(f.root, f.doc); !hasFailure(r, "RUNLOG") {
		t.Fatalf("got %v", r.Failures)
	}
}

func TestCheckFailsWhenResultsMarkdownStopsStatingTheNumbers(t *testing.T) {
	f := newFixture(t)
	f.edit(t, "eval/RESULTS.md", func(s string) string { return strings.Replace(s, "5 / 6 = 83.3%", "5 / 6 = 84.0%", 1) })
	if r := Check(f.root, f.doc); !hasFailure(r, "eval/RESULTS.md") {
		t.Fatalf("got %v", r.Failures)
	}
}

func TestCheckFailsWhenResultIDsLeaveTheSplit(t *testing.T) {
	f := newFixture(t)
	f.edit(t, "eval/splits/splits_final.json", func(s string) string { return strings.Replace(s, `"F10"`, `"F11"`, 1) })
	r := Check(f.root, f.doc)
	if !hasFailure(r, "splits_final.json") {
		t.Fatalf("got %v", r.Failures)
	}
}

func TestCheckFailsWhenAPrecedentIsNotEarlierThanItsCase(t *testing.T) {
	f := newFixture(t)
	// F1 is dated 2026-01-02; F2 is dated 2026-01-05, so citing it is a leak.
	f.edit(t, "eval/results/final-gemini-4-argon-prec-engine-v1-vertex.jsonl", func(s string) string {
		return strings.Replace(s, `"precedents":["X1"]`, `"precedents":["X1","F2"]`, 1)
	})
	r := Check(f.root, f.doc)
	if !hasFailure(r, "not dated strictly before their case") {
		t.Fatalf("got %v", r.Failures)
	}
}

func TestCheckFailsWhenBackendLabelsDifferFromEvalLabels(t *testing.T) {
	f := newFixture(t)
	f.edit(t, "backend/data/labels.jsonl", func(s string) string { return s + fxLabel("Z9", "2026-01-01", "China") })
	if r := Check(f.root, f.doc); !hasFailure(r, "backend/data/labels.jsonl") {
		t.Fatalf("got %v", r.Failures)
	}
}

func TestCheckFailsWhenFactsAndLabelsDisagree(t *testing.T) {
	f := newFixture(t)
	if err := os.Remove(filepath.Join(f.root, "backend/data/facts/X1.txt")); err != nil {
		t.Fatal(err)
	}
	if r := Check(f.root, f.doc); !hasFailure(r, "facts") {
		t.Fatalf("got %v", r.Failures)
	}
}

func TestBaselineWordingLint(t *testing.T) {
	cases := map[string]string{
		"verified 60.5":       "The heuristic baseline is 60.5% on 377 cases.",
		"lift stated plainly": "The lift is 35.5 points over the baseline.",
		"all-case lift":       "Overall that is 29.7 points above the baseline.",
	}
	for name, line := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.setDoc(t, f.docText(t)+"\n"+line+"\n")
			if r := Check(f.root, f.doc); !hasFailure(r, "wording") {
				t.Fatalf("want a wording failure, got %v", r.Failures)
			}
		})
	}
	// The same numbers pass when the line carries its qualifier.
	f := newFixture(t)
	f.setDoc(t, f.docText(t)+"\nThe 60.5 baseline is unverified; the 35.5 lift is unsupported; 29.7 is arithmetic on an unverified figure.\n")
	if r := Check(f.root, f.doc); len(r.Failures) != 0 {
		t.Fatalf("qualified wording should pass, got %v", r.Failures)
	}
}

func TestBaselineSourceAppearingFailsCheck(t *testing.T) {
	// If the research baseline file is ever committed, the "unverifiable"
	// finding is stale and the doc must be revisited.
	f := newFixture(t)
	write(t, f.root, "eval/RESULTS-v2.md", "baseline")
	if r := Check(f.root, f.doc); !hasFailure(r, "RESULTS-v2.md") {
		t.Fatalf("got %v", r.Failures)
	}
}

func TestPrivateReferencesAreRefused(t *testing.T) {
	f := newFixture(t)
	f.setDoc(t, f.docText(t)+"\nSee /Users/someone/work/notes.md\n")
	if r := Check(f.root, f.doc); !hasFailure(r, "private") {
		t.Fatalf("got %v", r.Failures)
	}
}

func TestTraceRefsMustResolve(t *testing.T) {
	cases := map[string][2]string{
		"missing file":    {"backend/internal/thing/thing.go:5#Retrieve", "backend/internal/thing/gone.go:5#Retrieve"},
		"missing symbol":  {"backend/internal/thing/thing.go:5#Retrieve", "backend/internal/thing/thing.go:5#Vanished"},
		"missing test":    {"thing_test.go#TestRetrieveWorks", "thing_test.go#TestRetrieveBreaks"},
		"line beyond EOF": {"thing.go:5#Retrieve", "thing.go:900#Retrieve"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			doc := f.docText(t)
			if !strings.Contains(doc, c[0]) {
				t.Fatalf("fixture doc lacks %q", c[0])
			}
			f.setDoc(t, strings.Replace(doc, c[0], c[1], 1))
			if r := Check(f.root, f.doc); !hasFailure(r, "trace") {
				t.Fatalf("want a trace failure, got %v", r.Failures)
			}
		})
	}
}

func TestTraceLineDriftIsANoteNotAFailure(t *testing.T) {
	// Other work in the same integration may move code; the symbol is the
	// contract, the line is a hint.
	f := newFixture(t)
	f.setDoc(t, strings.Replace(f.docText(t), "thing.go:5#Retrieve", "thing.go:3#Retrieve", 1))
	r := Check(f.root, f.doc)
	if len(r.Failures) != 0 {
		t.Fatalf("drift must not fail: %v", r.Failures)
	}
	if len(r.Notes) == 0 {
		t.Fatal("drift should be reported as a note")
	}
}

func TestRunExitCodes(t *testing.T) {
	f := newFixture(t)
	var out, errw bytes.Buffer
	if code := run([]string{"-root", f.root}, &out, &errw); code != 0 {
		t.Fatalf("consistent fixture exit %d\n%s%s", code, out.String(), errw.String())
	}
	f.setDoc(t, strings.Replace(f.docText(t), "| `final.correct` | 8 |", "| `final.correct` | 9 |", 1))
	out.Reset()
	if code := run([]string{"-root", f.root}, &out, &errw); code != 1 {
		t.Fatalf("mismatch exit %d, want 1", code)
	}
	if !strings.Contains(out.String(), "FAIL") {
		t.Fatalf("output should say FAIL: %s", out.String())
	}
	if code := run([]string{"-nope"}, &out, &errw); code != 2 {
		t.Fatalf("bad flag exit %d, want 2", code)
	}
}

func TestPrintEmitsTheTableTheDocParses(t *testing.T) {
	f := newFixture(t)
	var out, errw bytes.Buffer
	if code := run([]string{"-root", f.root, "-print"}, &out, &errw); code != 0 {
		t.Fatalf("exit %d: %s", code, errw.String())
	}
	if !strings.Contains(out.String(), "| `final.fast.correct` | 5 |") {
		t.Fatalf("printed table lacks fast-path row:\n%s", out.String())
	}
}

func TestPrintManifestListsEveryRequiredFile(t *testing.T) {
	f := newFixture(t)
	var out, errw bytes.Buffer
	if code := run([]string{"-root", f.root, "-print-manifest"}, &out, &errw); code != 0 {
		t.Fatalf("exit %d: %s", code, errw.String())
	}
	for _, rel := range requiredManifest {
		if !strings.Contains(out.String(), "  "+rel+"\n") {
			t.Errorf("manifest lacks %s", rel)
		}
	}
}

// TestRealRepository runs the check against the committed artifacts and
// docs/claims-reconciliation.md, from backend/ as documented.
func TestRealRepository(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	if _, err := os.Stat(filepath.Join(root, "eval", "SHA256SUMS")); err != nil {
		t.Skipf("not inside the repository checkout: %v", err)
	}
	doc := filepath.Join(root, "docs", "claims-reconciliation.md")
	r := Check(root, doc)
	if len(r.Failures) != 0 {
		t.Fatalf("committed claims do not reconcile:\n%s", strings.Join(r.Failures, "\n"))
	}
	kv, _ := Compute(root)
	for k, want := range map[string]string{
		"final.rows": "377", "final.correct": "340", "final.fast.n": "300", "final.fast.correct": "288",
		"final.accuracy_pct": "90.2", "final.fast.accuracy_pct": "96.0", "final.fast.coverage_pct": "79.6",
		"labels.rows": "2790", "pool.union": "3081", "usmca.not_in_labels": "291",
	} {
		if got := metric(t, kv, k); got != want {
			t.Errorf("%s = %s, want %s", k, got, want)
		}
	}
}

// TestPlannerPostFigures recomputes the post-cutoff flip-pair figures (D170):
// a move counts as shown only at the plan bar, and as right only when the
// pair is a real change and the check named CBP's later origin.
func TestPlannerPostFigures(t *testing.T) {
	f := newFixture(t)
	kv, problems := Compute(f.root)
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	for k, want := range map[string]string{
		"planner_post.pairs": "3", "planner_post.real_changes": "2", "planner_post.controls": "1", "planner_post.core_pairs": "2",
		"planner_post.known_a.shown": "1", "planner_post.known_a.shown_right": "1", "planner_post.known_a.controls_shown": "0",
		"planner_post.known_a.recall_pct": "50.0",
	} {
		if got := lookup(kv, k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	f.edit(t, "eval/data/flips_post.jsonl", func(s string) string { return strings.Replace(s, "2026-10-01", "2026-09-01", 1) })
	if _, problems := Compute(f.root); len(problems) == 0 {
		t.Error("a pair with no post-cutoff ruling was accepted")
	}
}

// TestRepeatabilityFigures recomputes the D190 figures from three dev runs:
// scoring adjustments apply, a resumed ruling counts once (its last row),
// and a flip is any difference from run 1.
func TestRepeatabilityFigures(t *testing.T) {
	f := newFixture(t)
	kv, problems := Compute(f.root)
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	for k, want := range map[string]string{
		"repeat.n": "3", "repeat.run1.correct": "2", "repeat.run2.correct": "1", "repeat.run3.correct": "2",
		"repeat.accuracy_min_pct": "33.3", "repeat.accuracy_max_pct": "66.7",
		"repeat.answer_flips": "1", "repeat.correctness_flips": "1", "repeat.fast_path_flips": "1",
	} {
		if got := lookup(kv, k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

// TestHeldoutRepeatFigures recomputes the D241 figures: official run plus two
// repeats per held-out split, the last row of a resumed ruling counting.
func TestHeldoutRepeatFigures(t *testing.T) {
	f := newFixture(t)
	kv, problems := Compute(f.root)
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	for k, want := range map[string]string{
		"repeat_heldout.final.n": "10", "repeat_heldout.final.official.correct": "8",
		"repeat_heldout.final.repeat1.correct": "8", "repeat_heldout.final.repeat2.correct": "9",
		"repeat_heldout.final.repeat_mean_pct": "85.00", "repeat_heldout.final.answer_flips": "1",
		"repeat_heldout.final.correctness_flips": "1",
		"repeat_heldout.final2.n":                "2", "repeat_heldout.final2.official.correct": "1",
		"repeat_heldout.final2.repeat1.correct": "2", "repeat_heldout.final2.repeat2.accuracy_pct": "50.0",
		"repeat_heldout.final2.repeat_mean_pct": "75.00", "repeat_heldout.final2.answer_flips": "1",
	} {
		if got := lookup(kv, k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

// D160: a calibration map whose bar would accept a different set of cases
// than the stated 0.92 bar is a problem, not a silently new figure.
func TestCalibratedBarMustMatchStatedBar(t *testing.T) {
	f := newFixture(t)
	f.edit(t, "backend/internal/calibrate/map.json", func(s string) string {
		return strings.Replace(s, `"from":0.92`, `"from":0.95`, 1)
	})
	_, problems := Compute(f.root)
	found := false
	for _, p := range problems {
		if strings.Contains(p, "accept different") {
			found = true
		}
	}
	if !found {
		t.Fatalf("problems = %v, want a calibrated-bar mismatch", problems)
	}
}

// TestAllRegimesFigures recomputes the D210 figures: per regime and window,
// the sum of the splits and the de-duplicated pool, where a ruling in both
// the textile and the Part 102 split counts once, under Part 102.
func TestAllRegimesFigures(t *testing.T) {
	f := newFixture(t)
	kv, problems := Compute(f.root)
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	for k, want := range map[string]string{
		"regimes.w1.st.correct": "8", "regimes.w1.textile.correct": "2", "regimes.w1.part102.correct": "1",
		"regimes.w1.overlap": "1", "regimes.w1.sum.n": "15", "regimes.w1.sum.correct": "12",
		"regimes.w1.dedup.n": "14", "regimes.w1.dedup.correct": "11",
		"regimes.w2.part102.correct": "1", "regimes.w2.sum.n": "5", "regimes.w2.sum.correct": "3", "regimes.w2.overlap": "0",
		"regimes.all.sum.pct": "75.0", "regimes.all.dedup.n": "19", "regimes.all.dedup.correct": "14", "regimes.all.dedup.pct": "73.7",
	} {
		if got := lookup(kv, k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	f.edit(t, "eval/splits/splits_pharma_final2.json", func(string) string { return `{"final2":["P2","P3"]}` })
	if _, problems := Compute(f.root); len(problems) == 0 {
		t.Error("a split ruling missing from its result file was accepted")
	}
}

// TestRouterFigures recomputes the router's dev routing per gold engine and
// router-v2's routed path on the post-cutoff rulings (D230-D232).
func TestRouterFigures(t *testing.T) {
	f := newFixture(t)
	kv, problems := Compute(f.root)
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	for k, want := range map[string]string{
		"router.dev.v2.all.correct": "1", "router.dev.v2.all.n": "2", "router.dev.v2.specialist.correct": "0",
		"router.dev.v3.general.correct": "0", "router.dev.v3.usmca_part102.correct": "1",
		"router.dev.v4.all.pct": "100.0", "router.dev.v4.all.n": "2",
		"router.w2.routed.v2.correct": "3", "router.w2.routed.v2.n": "5", "router.w2.routed.v2.pct": "60.0", "router.w2.routed.v2.sent_elsewhere": "2",
		"router.final.v2.all.correct": "1", "router.final.v4.all.correct": "2", "router.final2.v4.general.correct": "1",
		"router.w2.routed.v4.correct": "3", "router.w2.routed.v4.sent_elsewhere": "1",
	} {
		if got := lookup(kv, k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	// A routed ruling with no prediction from the engine it was sent to.
	f.edit(t, "eval/results/final2-gemini-4-argon-prec-engine-v1-litellm-routed-v2.jsonl", func(string) string {
		return fxEngine("P2", "2026-10-04", true, "Australia", .9)
	})
	if _, problems := Compute(f.root); len(problems) == 0 {
		t.Fatal("a missing routed prediction passed")
	}
}
