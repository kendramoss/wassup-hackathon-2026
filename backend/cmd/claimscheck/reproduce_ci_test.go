//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestReproduceRunsInCI runs the exact offline reproduce command used by the
// presentation, so a green ci-ok really means the published numbers still
// reconcile (#430).
//
// CI already runs `go test ./...` on ubuntu-latest, but editing
// .github/workflows/ci.yml to add a reproduce step needs the GitHub Actions
// `workflow` scope, which the automation token here does not have. This
// Linux-only test is the equivalent hook: GitHub sets CI=true in Actions, so
// the command runs there. Locally it is opt-in (RUN_REPRODUCE=1) and on Windows
// the build tag excludes it, keeping `go test ./...` portable.
func TestReproduceRunsInCI(t *testing.T) {
	if os.Getenv("CI") == "" && os.Getenv("RUN_REPRODUCE") == "" {
		t.Skip("set CI=1 or RUN_REPRODUCE=1 to run make reproduce")
	}
	// go test runs with the package directory as its working directory:
	// backend/cmd/claimscheck -> repository root is three levels up.
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "Makefile")); err != nil {
		t.Fatalf("no Makefile at %s: %v", root, err)
	}

	if before := gitStatus(t, root); before != "" {
		t.Fatalf("working tree was not clean before reproduce:\n%s", before)
	}
	cmd := exec.Command("make", "reproduce")
	cmd.Dir = root
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	t.Logf("make reproduce output:\n%s", out)
	if err != nil {
		t.Fatalf("make reproduce failed: %v\n%s", err, out)
	}
	// The commands are read-only; a clean tree afterward is part of the
	// promise, so check it in the same run rather than trusting the caller.
	if after := gitStatus(t, root); after != "" {
		t.Fatalf("make reproduce modified tracked files:\n%s", after)
	}
}

func gitStatus(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	return string(out)
}
