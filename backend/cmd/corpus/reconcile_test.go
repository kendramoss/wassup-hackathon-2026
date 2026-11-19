package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
)

// Every test works in t.TempDir fixtures; none touches data/corpus.

func writeFile(t *testing.T, path, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool { _, err := os.Lstat(path); return err == nil }

func rawRuling(t *testing.T, raw, id string) {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"rulingNumber": id, "text": "You requested a ruling. The " + id + " lamp is made in Vietnam. HOLDING: The country of origin is Vietnam."})
	writeFile(t, filepath.Join(raw, id+".json"), string(b))
}

func runBuild(t *testing.T, raw, facts, keep string) {
	t.Helper()
	if err := build([]string{"-raw", raw, "-facts", facts, "-keep-labels", keep}); err != nil {
		t.Fatal(err)
	}
}

func splitMetaIDs(t *testing.T, facts string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(facts, "split_meta.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, ln := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var m struct{ Ruling string }
		if err := json.Unmarshal([]byte(ln), &m); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, m.Ruling)
	}
	return ids
}

func TestBuildRemovesObsoleteGeneratedFactsOnly(t *testing.T) {
	root := t.TempDir()
	raw, facts := filepath.Join(root, "raw"), filepath.Join(root, "facts")
	keep := filepath.Join(root, "labels_usmca.jsonl")
	writeFile(t, keep, `{"ruling":"N300001","date":"2025-01-01"}`+"\n")
	rawRuling(t, raw, "N100001")
	rawRuling(t, raw, "N100002")
	runBuild(t, raw, facts, keep)

	// Separately generated Part 102 facts, a user file, and a legacy-looking
	// ruling-shaped file that was never in a manifest.
	writeFile(t, filepath.Join(facts, "N300001.txt"), "part 102 facts\n")
	writeFile(t, filepath.Join(facts, "N999999.txt"), "hand-placed ruling-shaped file\n")
	writeFile(t, filepath.Join(facts, "notes.txt"), "user notes\n")
	writeFile(t, filepath.Join(facts, "sub", "N100002.txt"), "nested\n")

	if err := os.Remove(filepath.Join(raw, "N100002.json")); err != nil {
		t.Fatal(err)
	}
	runBuild(t, raw, facts, keep)

	if exists(filepath.Join(facts, "N100002.txt")) {
		t.Error("obsolete generated facts file should be removed")
	}
	if !exists(filepath.Join(facts, "N100001.txt")) {
		t.Error("current facts file removed")
	}
	for _, keepName := range []string{"N300001.txt", "N999999.txt", "notes.txt", "sub/N100002.txt"} {
		if !exists(filepath.Join(facts, keepName)) {
			t.Errorf("%s must be preserved", keepName)
		}
	}
	if ids := splitMetaIDs(t, facts); len(ids) != 1 || ids[0] != "N100001" {
		t.Errorf("split_meta = %v, want [N100001]", ids)
	}

	// corpus.Load membership follows the rebuilt directory.
	one, two := "Vietnam", "Vietnam"
	labels := []corpus.Label{
		{Ruling: "N100001", Date: "2025-01-01", PrimaryOrigin: &one, Evidence: []string{"h"}},
		{Ruling: "N100002", Date: "2025-01-02", PrimaryOrigin: &two, Evidence: []string{"h"}},
		{Ruling: "N300001", Date: "2025-01-03", PrimaryOrigin: &two, Evidence: []string{"h"}},
	}
	st, err := corpus.Load(labels, facts)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st.Get("N100002"); ok {
		t.Error("corpus.Load must exclude the pruned ruling")
	}
	if _, ok := st.Get("N100001"); !ok {
		t.Error("corpus.Load lost a current ruling")
	}
	if _, ok := st.Get("N300001"); !ok {
		t.Error("corpus.Load lost the Part 102 ruling")
	}
	// The label stays even though its facts are gone.
	if _, ok := st.Label("N100002"); !ok {
		t.Error("label should be retained")
	}

	// Idempotent.
	before := st.Revision()
	runBuild(t, raw, facts, keep)
	st2, _ := corpus.Load(labels, facts)
	if st2.Revision() != before {
		t.Error("rebuild with unchanged inputs changed the corpus revision")
	}
}

func TestBuildProtectsSeparatelyGeneratedIDEvenIfOwned(t *testing.T) {
	root := t.TempDir()
	raw, facts := filepath.Join(root, "raw"), filepath.Join(root, "facts")
	keep := filepath.Join(root, "labels_usmca.jsonl")
	writeFile(t, keep, `{"ruling":"N100002","date":"2025-01-01"}`+"\n")
	rawRuling(t, raw, "N100002")
	runBuild(t, raw, facts, keep)
	os.Remove(filepath.Join(raw, "N100002.json"))
	runBuild(t, raw, facts, keep)
	if !exists(filepath.Join(facts, "N100002.txt")) {
		t.Error("an id in the Part 102 labels must never be pruned")
	}
}

func legacyFixture(t *testing.T) (raw, facts, keep string) {
	root := t.TempDir()
	raw, facts, keep = filepath.Join(root, "raw"), filepath.Join(root, "facts"), filepath.Join(root, "labels_usmca.jsonl")
	writeFile(t, keep, `{"ruling":"N300001","date":"2025-01-01"}`+"\n")
	rawRuling(t, raw, "N100001")
	// Output of an old build: facts + split_meta, no manifest; N100002's raw is gone.
	writeFile(t, filepath.Join(facts, "N100001.txt"), "old\n")
	writeFile(t, filepath.Join(facts, "N100002.txt"), "stale generated\n")
	writeFile(t, filepath.Join(facts, "split_meta.jsonl"), `{"ruling":"N100001"}`+"\n"+`{"ruling":"N100002"}`+"\n")
	writeFile(t, filepath.Join(facts, "N300001.txt"), "part 102\n")
	writeFile(t, filepath.Join(facts, "notes.txt"), "user\n")
	return
}

func TestLegacyDirBootstrapsOwnershipFromSplitMeta(t *testing.T) {
	raw, facts, keep := legacyFixture(t)
	runBuild(t, raw, facts, keep)
	if exists(filepath.Join(facts, "N100002.txt")) {
		t.Error("stale legacy generated file should be pruned (proven by old split_meta)")
	}
	for _, n := range []string{"N100001.txt", "N300001.txt", "notes.txt", buildManifest} {
		if !exists(filepath.Join(facts, n)) {
			t.Errorf("%s missing", n)
		}
	}
	one := "Vietnam"
	labels := []corpus.Label{
		{Ruling: "N100001", Date: "2025-01-01", PrimaryOrigin: &one, Evidence: []string{"h"}},
		{Ruling: "N100002", Date: "2025-01-02", PrimaryOrigin: &one, Evidence: []string{"h"}},
		{Ruling: "N300001", Date: "2025-01-03", PrimaryOrigin: &one, Evidence: []string{"h"}},
	}
	st, _ := corpus.Load(labels, facts)
	if _, ok := st.Get("N100002"); ok || st.Len() != 2 {
		t.Errorf("obsolete precedent still loaded, len=%d", st.Len())
	}
}

func TestLegacyDirCorruptMetaStopsBeforeMutation(t *testing.T) {
	raw, facts, keep := legacyFixture(t)
	writeFile(t, filepath.Join(facts, "split_meta.jsonl"), "{\"ruling\":\"N100001\"}\n{garbage\n")
	if err := build([]string{"-raw", raw, "-facts", facts, "-keep-labels", keep}); err == nil {
		t.Fatal("corrupt legacy metadata must stop the build")
	}
	if b, _ := os.ReadFile(filepath.Join(facts, "N100001.txt")); string(b) != "old\n" || !exists(filepath.Join(facts, "N100002.txt")) || exists(filepath.Join(facts, buildManifest)) {
		t.Error("nothing may change when legacy metadata is corrupt")
	}
}

func TestLegacyDirWithoutEvidenceFailsRatherThanStayStale(t *testing.T) {
	raw, facts, keep := legacyFixture(t)
	os.Remove(filepath.Join(facts, "split_meta.jsonl"))
	err := build([]string{"-raw", raw, "-facts", facts, "-keep-labels", keep})
	if err == nil || !strings.Contains(err.Error(), "cannot be proven") {
		t.Fatalf("want actionable ownership error, got %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(facts, "N100001.txt")); string(b) != "old\n" || !exists(filepath.Join(facts, "N100002.txt")) {
		t.Error("nothing may change on refusal")
	}
	// A clean empty dir and a dir holding only protected/produced files are fine.
	root := t.TempDir()
	rawRuling(t, filepath.Join(root, "raw"), "N100001")
	runBuild(t, filepath.Join(root, "raw"), filepath.Join(root, "facts"), keep)
}

func TestBundleLegacyDirRefusesUnexplainedFiles(t *testing.T) {
	root := t.TempDir()
	from, to := filepath.Join(root, "facts"), filepath.Join(root, "bundle")
	keep := filepath.Join(root, "k.jsonl")
	writeFile(t, keep, `{"ruling":"N300001","date":"2025-01-01"}`+"\n")
	writeFile(t, filepath.Join(from, "N100001.txt"), "a\n")
	writeFile(t, filepath.Join(to, "N100001.txt"), "old\n")
	writeFile(t, filepath.Join(to, "N300001.txt"), "part 102\n")
	if err := bundle([]string{"-from", from, "-to", to, "-keep-labels", keep}); err != nil {
		t.Fatalf("explained legacy bundle should succeed: %v", err)
	}
	writeFile(t, filepath.Join(to, buildManifest), "")
	os.Remove(filepath.Join(to, bundleManifest))
	writeFile(t, filepath.Join(to, "N100002.txt"), "stale bundled\n")
	os.Remove(filepath.Join(to, bundleManifest))
	if err := bundle([]string{"-from", from, "-to", to, "-keep-labels", keep}); err == nil || !exists(filepath.Join(to, "N100002.txt")) {
		t.Errorf("unexplained bundled file must fail the bundle untouched: %v", err)
	}
}

func TestReconcileRefusesBadManifestAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "victim.txt")
	writeFile(t, outside, "must survive\n")

	// Path traversal ids are rejected before anything changes.
	writeFile(t, filepath.Join(dir, buildManifest), `{"version":1,"ids":["../victim"]}`)
	writeFile(t, filepath.Join(dir, "N100001.txt"), "old\n")
	if _, err := reconcile(dir, buildManifest, "split_meta.jsonl", map[string][]byte{"N100002": []byte("new\n")}, nil, nil); err == nil {
		t.Fatal("manifest with a path-like id must be refused")
	}
	if exists(filepath.Join(dir, "N100002.txt")) || !exists(filepath.Join(dir, "N100001.txt")) || !exists(outside) {
		t.Error("a refused reconcile must change nothing")
	}
	// Corrupt manifest.
	writeFile(t, filepath.Join(dir, buildManifest), `{not json`)
	if _, err := reconcile(dir, buildManifest, "split_meta.jsonl", nil, nil, nil); err == nil {
		t.Error("corrupt manifest must be refused")
	}
	// Bad ids to write.
	if _, err := reconcile(t.TempDir(), buildManifest, "split_meta.jsonl", map[string][]byte{"../x": nil}, nil, nil); err == nil {
		t.Error("bad output id must be refused")
	}
	if _, err := reconcile(t.TempDir(), buildManifest, "split_meta.jsonl", nil, map[string][]byte{"../x": nil}, nil); err == nil {
		t.Error("extra name with a path must be refused")
	}

	// An owned name that is now a symlink is left alone, target untouched.
	dir2 := t.TempDir()
	writeFile(t, filepath.Join(dir2, buildManifest), `{"version":1,"ids":["N100001"]}`)
	if err := os.Symlink(outside, filepath.Join(dir2, "N100001.txt")); err != nil {
		t.Skip("symlinks unavailable")
	}
	res, err := reconcile(dir2, buildManifest, "split_meta.jsonl", map[string][]byte{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skipped) != 1 || !exists(filepath.Join(dir2, "N100001.txt")) || !exists(outside) {
		t.Errorf("symlink must be skipped, not removed: %+v", res)
	}
}

func TestBuildFailureKeepsLastGoodOutput(t *testing.T) {
	root := t.TempDir()
	raw, facts := filepath.Join(root, "raw"), filepath.Join(root, "facts")
	none := filepath.Join(root, "none.jsonl")
	rawRuling(t, raw, "N100001")
	runBuild(t, raw, facts, none)
	good, _ := os.ReadFile(filepath.Join(facts, "N100001.txt"))
	meta, _ := os.ReadFile(filepath.Join(facts, "split_meta.jsonl"))

	writeFile(t, filepath.Join(raw, "N100002.json"), `{corrupt`)
	if err := build([]string{"-raw", raw, "-facts", facts, "-keep-labels", none}); err == nil {
		t.Fatal("corrupt raw input should fail the build")
	}
	got, _ := os.ReadFile(filepath.Join(facts, "N100001.txt"))
	gotMeta, _ := os.ReadFile(filepath.Join(facts, "split_meta.jsonl"))
	if string(got) != string(good) || string(gotMeta) != string(meta) || exists(filepath.Join(facts, "N100002.txt")) {
		t.Error("a failed build must leave the last good output intact")
	}
	if ents, _ := filepath.Glob(filepath.Join(facts, "*"+stageSuffix)); len(ents) != 0 {
		t.Errorf("stage files left behind: %v", ents)
	}
}

func TestBundleReconcilesOnlyBundledFiles(t *testing.T) {
	root := t.TempDir()
	from, to := filepath.Join(root, "facts"), filepath.Join(root, "bundle")
	writeFile(t, filepath.Join(from, "N100001.txt"), "a\n")
	writeFile(t, filepath.Join(from, "N100002.txt"), "b\n")
	writeFile(t, filepath.Join(from, "split_meta.jsonl"), "{}\n")
	writeFile(t, filepath.Join(from, buildManifest), "{}")
	writeFile(t, filepath.Join(to, "N300001.txt"), "separately bundled part 102\n")
	writeFile(t, filepath.Join(to, "keep.txt"), "user\n")
	keep := filepath.Join(root, "k.jsonl")
	writeFile(t, keep, `{"ruling":"N300001","date":"2025-01-01"}`+"\n")
	b := func() {
		if err := bundle([]string{"-from", from, "-to", to, "-keep-labels", keep}); err != nil {
			t.Fatal(err)
		}
	}
	b()
	if !exists(filepath.Join(to, "N100002.txt")) || exists(filepath.Join(to, "split_meta.jsonl")) || exists(filepath.Join(to, buildManifest)) {
		t.Error("bundle should copy facts files only")
	}
	os.Remove(filepath.Join(from, "N100002.txt"))
	b()
	if exists(filepath.Join(to, "N100002.txt")) {
		t.Error("bundled file whose source is gone should be removed")
	}
	for _, n := range []string{"N100001.txt", "N300001.txt", "keep.txt"} {
		if !exists(filepath.Join(to, n)) {
			t.Errorf("%s must be preserved", n)
		}
	}
}
