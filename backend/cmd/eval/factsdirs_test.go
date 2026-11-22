package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// D612: a -facts directory that does not exist stops the run instead of
// leaving its rulings out of the precedent pool in silence.
func TestExistingDirsRejectsAMissingFactsDirectory(t *testing.T) {
	tmp := t.TempDir()
	a := filepath.Join(tmp, "a")
	if err := os.Mkdir(a, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := existingDirs(a)
	if err != nil || len(got) != 1 || got[0] != a {
		t.Fatalf("existingDirs(%q) = %v, %v", a, got, err)
	}
	missing := filepath.Join(tmp, "data", "facts")
	if _, err := existingDirs(missing + "," + a); err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("a missing directory gave %v, want an error naming it", err)
	}
	f := filepath.Join(tmp, "f.txt")
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := existingDirs(f); err == nil {
		t.Fatal("a file was accepted as a facts directory")
	}
	if _, err := existingDirs(""); err == nil {
		t.Fatal("an empty -facts was accepted")
	}
}

// The window-2 Part 102 command as D612 corrects it: from backend/, both
// directories exist in a clean checkout (the test runs in cmd/eval, two
// levels down).
func TestWindow2FactsPathsExist(t *testing.T) {
	if _, err := existingDirs("../../data/facts,../../../eval/data/facts_final2"); err != nil {
		t.Fatal(err)
	}
	if _, err := existingDirs("../../../data/facts"); err == nil {
		t.Fatal("../data/facts exists from backend/; D602's note no longer holds")
	}
}
