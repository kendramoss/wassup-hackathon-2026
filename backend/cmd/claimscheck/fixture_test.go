package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fixture is a miniature repository with the same layout as the real one
// and numbers small enough to verify by hand. Final split (10 rows):
//
//	F1 .98 right in-scope    F6 .99 right OUT of scope
//	F2 .97 right in-scope    F7 .91 right in-scope
//	F3 .95 right in-scope    F8 .70 wrong in-scope
//	F4 .93 wrong in-scope    F9 .96 right in-scope
//	F5 .92 right in-scope    F10 .50 right in-scope
//
// So: 8/10 correct; fast path (>= 0.92 and in scope) = F1-F5, F9 = 5/6;
// confidence-only fast path adds F6 = 6/7; >= 0.95 band = F1 F2 F3 F6 F9 = 5/5.
// Dev split (4 rows): D1 excluded; D2 "Indonesia / Malaysia" is right under an
// alternatives entry; D3 right; D4 wrong -> 3 scored, 2 correct.

type fixture struct {
	root string
	doc  string
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sum(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func fxEngine(id, date string, correct bool, pred string, conf float64, prec ...string) string {
	method := "regex"
	if id == "F5" {
		method = "manual"
	}
	if prec == nil {
		prec = []string{}
	}
	pj, _ := json.Marshal(prec)
	return fmt.Sprintf(`{"ruling":%q,"date":%q,"correct":%t,"pred":%q,"confidence":%g,"retrieval":"bm25","label_method":%q,"precedents":%s}`+"\n",
		id, date, correct, pred, conf, method, pj)
}

func fxScope(id string, in bool) string {
	return fmt.Sprintf(`{"ruling":%q,"in_scope":%t}`+"\n", id, in)
}

func fxLabel(id, date string, origin string) string {
	if origin == "" {
		return fmt.Sprintf(`{"ruling":%q,"date":%q,"primary_origin":null}`+"\n", id, date)
	}
	return fmt.Sprintf(`{"ruling":%q,"date":%q,"primary_origin":%q}`+"\n", id, date, origin)
}

// fxCalMap is a two-step calibration map whose bar accepts exactly the
// stated 0.92 cases.
const fxCalMap = `{"version":"cal-fx","engine_prompt":"engine-v1","model":"gemini-4-argon","bar":0.9,
 "steps":[{"from":0.5,"calibrated":0.6,"n":10,"correct":6},{"from":0.92,"calibrated":0.9,"n":10,"correct":9}]}`

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	final := fxEngine("F1", "2026-01-02", true, "China", .98, "X1") +
		fxEngine("F2", "2026-01-05", true, "China", .97, "F1", "X1") +
		fxEngine("F3", "2026-02-01", true, "China", .95, "F2", "D2") +
		fxEngine("F4", "2026-02-02", false, "China", .93) +
		fxEngine("F5", "2026-03-01", true, "China", .92) +
		fxEngine("F6", "2026-03-02", true, "China", .99) +
		fxEngine("F7", "2026-04-01", true, "China", .91) +
		fxEngine("F8", "2026-04-02", false, "China", .70) +
		fxEngine("F9", "2026-09-25", true, "China", .96) +
		fxEngine("F10", "2026-05-01", true, "China", .50)
	finalScope := fxScope("F1", true) + fxScope("F2", true) + fxScope("F3", true) + fxScope("F4", true) +
		fxScope("F5", true) + fxScope("F6", false) + fxScope("F7", true) + fxScope("F8", true) +
		fxScope("F9", true) + fxScope("F10", true)
	dev := fxEngine("D1", "2025-07-01", false, "China", .9) +
		fxEngine("D2", "2025-08-01", false, "Indonesia / Malaysia", .95) +
		fxEngine("D3", "2025-09-01", true, "China", .8) +
		fxEngine("D4", "2025-12-31", false, "China", .99)
	devScope := fxScope("D1", true) + fxScope("D2", true) + fxScope("D3", true) + fxScope("D4", true)

	write(t, root, "eval/results/final-gemini-4-argon-prec-engine-v1-vertex.jsonl", final)
	write(t, root, "eval/results/scope-final-router-v2-vertex.jsonl", finalScope)
	write(t, root, "eval/results/dev-gemini-4-argon-prec-engine-v1-vertex.jsonl", dev)
	write(t, root, "eval/results/scope-dev-router-v2-vertex.jsonl", devScope)
	// Repeatability (D190): run 1 is the dev rows; run 2 gets D3 wrong (after
	// a failed first attempt that a resume replaced); run 3 drops D4 under the
	// fast-path bar. So 2, 1, 2 correct of 3; D3 flips answer and
	// correctness, D4 flips fast-path membership.
	write(t, root, "eval/results/dev-gemini-4-argon-prec-engine-v1-litellm-gw.jsonl", dev)
	write(t, root, "eval/results/dev-gemini-4-argon-prec-engine-v1-litellm-router-rep2.jsonl",
		strings.Replace(dev, fxEngine("D3", "2025-09-01", true, "China", .8),
			`{"ruling":"D3","date":"2025-09-01","correct":false,"pred":"","confidence":0,"error":"timeout"}`+"\n"+
				fxEngine("D3", "2025-09-01", false, "Japan", .8), 1))
	write(t, root, "eval/results/dev-gemini-4-argon-prec-engine-v1-litellm-router-rep3.jsonl",
		strings.Replace(dev, fxEngine("D4", "2025-12-31", false, "China", .99), fxEngine("D4", "2025-12-31", false, "China", .90), 1))
	write(t, root, "eval/data/scoring_adjustments.json",
		`{"exclude":{"D1":"multi-scenario"},"alternatives":{"D2":["Indonesia","Malaysia"]}}`)
	write(t, root, "eval/splits/splits_final.json",
		`{"final":["F1","F2","F3","F4","F5","F6","F7","F8","F9","F10"],"dev":["D1","D2","D3","D4"],"pilot":["P1"]}`)
	labels := fxLabel("F1", "2026-01-02", "China") + fxLabel("F2", "2026-01-05", "China") + fxLabel("D1", "2025-07-01", "") +
		fxLabel("D2", "2025-08-01", "Indonesia") + fxLabel("X1", "2023-01-10", "Japan")
	usmca := fxLabel("F1", "2026-01-02", "China") + fxLabel("U1", "2015-01-05", "") + fxLabel("U2", "2026-09-22", "Canada")
	write(t, root, "eval/data/labels.jsonl", labels)
	write(t, root, "eval/data/labels_usmca.jsonl", usmca)
	write(t, root, "backend/data/labels.jsonl", labels)
	write(t, root, "backend/data/labels_usmca.jsonl", usmca)
	for _, id := range []string{"F1", "F2", "D1", "D2", "X1", "U1", "U2"} {
		write(t, root, "backend/data/facts/"+id+".txt", "facts "+id)
	}
	write(t, root, "eval/results/calibration-by-family.json",
		`{"splits":[{"split":"dev","n":3,"global":{"auto":2,"correct":1}},{"split":"final","n":10,"global":{"auto":6,"correct":5}}]}`)
	write(t, root, "backend/internal/calibrate/map.json", fxCalMap)
	write(t, root, "eval/results/slice30-baseline.json",
		`{"source":"dev-gemini-4-argon-prec-engine-v1-vertex.jsonl","n":3,"correct":2,"band95_n":2,"band95_correct":2}`)
	write(t, root, "eval/results/RUNLOG.jsonl",
		`{"run":"final-gemini-4-argon-prec-engine-v1-vertex","cases":10,"correct":8,"fast_path_coverage":7,"fast_path_correct":6,"time":"2026-10-15T18:29:23Z"}`+"\n")
	write(t, root, "eval/RESULTS.md", "final 6 / 10 = 60.0% fast; 5 / 6 = 83.3% accuracy; overall 80.0%\n")
	// Post-cutoff flip pairs (D170): two real changes and a control.
	write(t, root, "eval/data/flips_post.jsonl",
		`{"id":"P1","date":"2026-10-01","tier":"core","origin_changed":true}`+"\n"+
			`{"id":"P2","date":"2026-10-02","tier":"edge","origin_changed":true}`+"\n"+
			`{"id":"C1","date":"2026-10-03","tier":"core","origin_changed":false}`+"\n")
	write(t, root, "eval/results/flips-post-verifier-v2-known-a-litellm.jsonl",
		`{"id":"P1","precedent_supported":true,"confidence":0.95,"origin_b_ok":true}`+"\n"+
			`{"id":"P2","precedent_supported":true,"confidence":0.85,"origin_b_ok":true}`+"\n"+
			`{"id":"C1","precedent_supported":false,"confidence":0.9,"origin_b_ok":true}`+"\n")
	// Other regimes, both windows (D210). Window 1: textile T1 right and B1
	// right, pharma P1 right, Part 102 B1 wrong and U3 right; B1 is in both
	// the textile and the Part 102 split. Window 2: substantial
	// transformation S1 right and S2 wrong, textile T2 right, pharma P2
	// wrong, Part 102 U4 right (after a failed first attempt).
	write(t, root, "eval/splits/splits_textile.json", `{"final":["T1","B1"]}`)
	write(t, root, "eval/splits/splits_pharma.json", `{"final":["P1"]}`)
	write(t, root, "eval/splits/splits_usmca.json", `{"final":["B1","U3"]}`)
	write(t, root, "eval/splits/splits_final2.json", `{"final2":["S1","S2"]}`)
	write(t, root, "eval/splits/splits_textile_final2.json", `{"final2":["T2"]}`)
	write(t, root, "eval/splits/splits_pharma_final2.json", `{"final2":["P2"]}`)
	write(t, root, "eval/splits/splits_usmca_final2.json", `{"final2":["U4"]}`)
	write(t, root, "eval/results/textile-final-gemini-4-argon-prec-textile-v1-vertex.jsonl",
		fxEngine("T1", "2026-02-01", true, "China", .9)+fxEngine("B1", "2026-03-01", true, "Canada", .9))
	write(t, root, "eval/results/pharma-final-gemini-4-argon-prec-pharma-v1-litellm.jsonl", fxEngine("P1", "2026-02-01", true, "India", .9))
	write(t, root, "eval/results/usmca-final-gemini-4-argon-prec-usmca-v1-litellm.jsonl",
		fxEngine("B1", "2026-03-01", false, "China", .9)+fxEngine("U3", "2026-04-01", true, "Mexico", .9))
	write(t, root, "eval/results/final2-gemini-4-argon-prec-engine-v1-litellm-router.jsonl",
		fxEngine("S1", "2026-10-01", true, "China", .9)+fxEngine("S2", "2026-10-02", false, "China", .9))
	write(t, root, "eval/results/textile-final2-gemini-4-argon-prec-textile-v2-litellm-reasoning-v1-router.jsonl", fxEngine("T2", "2026-10-03", true, "China", .9))
	write(t, root, "eval/results/pharma-final2-gemini-4-argon-prec-pharma-v1-litellm-router.jsonl", fxEngine("P2", "2026-10-04", false, "India", .9))
	write(t, root, "eval/results/usmca-final2-gemini-4-argon-prec-usmca-v1-litellm-router.jsonl",
		`{"ruling":"U4","date":"2026-10-05","correct":false,"pred":"","confidence":0,"error":"timeout"}`+"\n"+fxEngine("U4", "2026-10-05", true, "Canada", .9))
	// eval/SHA256SUMS pins data files relative to eval/.
	var sums strings.Builder
	for _, rel := range []string{"data/labels.jsonl", "data/labels_usmca.jsonl", "splits/splits_final.json"} {
		b, err := os.ReadFile(filepath.Join(root, "eval", filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&sums, "%s  %s\n", sum(string(b)), rel)
	}
	write(t, root, "eval/SHA256SUMS", sums.String())

	// Code the doc traces.
	write(t, root, "backend/internal/thing/thing.go", "package thing\n\n// Pad.\n\nfunc Retrieve() {}\n")
	write(t, root, "backend/internal/thing/thing_test.go", "package thing\n\nimport \"testing\"\n\nfunc TestRetrieveWorks(t *testing.T) {}\n")

	f := &fixture{root: root, doc: filepath.Join(root, "docs", "claims-reconciliation.md")}
	f.regen(t)
	return f
}

// regen writes a doc that agrees with the repository as it is now.
func (f *fixture) regen(t *testing.T) {
	t.Helper()
	kv, problems := Compute(f.root)
	if len(problems) > 0 {
		t.Fatalf("fixture does not compute: %v", problems)
	}
	man, err := Manifest(f.root)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString("# Claims reconciliation\n\n## Recomputed figures\n\n| key | value | meaning |\n|---|---|---|\n")
	b.WriteString(RenderTable(kv))
	b.WriteString("\n## Baseline\n\nThe 60.5 baseline is reported, unverified. The 35.5 lift is unsupported as like-for-like.\n")
	b.WriteString("\n## Code trace\n\n| capability | code | tests |\n|---|---|---|\n")
	b.WriteString("| retrieval | `backend/internal/thing/thing.go:5#Retrieve` | `backend/internal/thing/thing_test.go#TestRetrieveWorks` |\n")
	b.WriteString("\n## Checksum manifest\n\n```\n" + RenderManifest(man) + "```\n")
	write(t, f.root, "docs/claims-reconciliation.md", b.String())
}

func (f *fixture) docText(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(f.doc)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func (f *fixture) setDoc(t *testing.T, s string) {
	t.Helper()
	write(t, f.root, "docs/claims-reconciliation.md", s)
}

func (f *fixture) edit(t *testing.T, rel string, fn func(string) string) {
	t.Helper()
	p := filepath.Join(f.root, filepath.FromSlash(rel))
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(fn(string(b))), 0o644); err != nil {
		t.Fatal(err)
	}
}
