package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// maxTracked is the largest file the repository may track outside a data/
// directory. The largest tracked source or result file is under 3 MB.
const maxTracked = 5 << 20

// TestNoBuildOutputIsTracked fails when a tracked file is an executable
// (ELF, Mach-O or PE) or is over maxTracked outside data/ (#578: a 20 MB
// macOS build of cmd/tariffshift was committed), and when a `go build
// ./cmd/<name>` in backend/ would leave an untracked, unignored binary.
// It reads only the checkout; it skips outside a git work tree.
func TestNoBuildOutputIsTracked(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("not a git work tree: %v", err)
	}
	for _, f := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if f == "" {
			continue
		}
		path := filepath.Join(root, f)
		st, err := os.Lstat(path)
		if err != nil || !st.Mode().IsRegular() {
			continue // deleted in the work tree, or a symlink
		}
		if st.Size() > maxTracked && !inDataDir(f) {
			t.Errorf("%s is tracked and is %d bytes (limit %d outside data/)", f, st.Size(), maxTracked)
		}
		if kind := executableKind(path); kind != "" {
			t.Errorf("%s is a tracked %s executable; build outputs belong in .gitignore", f, kind)
		}
	}
	cmds, _ := os.ReadDir(filepath.Join(root, "backend", "cmd"))
	for _, d := range cmds {
		if !d.IsDir() || d.Name() == "data" { // backend/data is the data directory
			continue
		}
		bin := "backend/" + d.Name()
		c := exec.Command("git", "check-ignore", "-q", "--no-index", bin)
		c.Dir = root
		if c.Run() != nil {
			t.Errorf("%s (go build ./cmd/%s) is not in .gitignore", bin, d.Name())
		}
	}
}

func inDataDir(f string) bool {
	return strings.HasPrefix(f, "data/") || strings.Contains(f, "/data/")
}

// executableKind names the executable format from the file's first bytes.
func executableKind(path string) string {
	fh, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer fh.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(fh, head)
	head = head[:n]
	switch {
	case bytes.HasPrefix(head, []byte{0x7f, 'E', 'L', 'F'}):
		return "ELF"
	case bytes.HasPrefix(head, []byte{0xcf, 0xfa, 0xed, 0xfe}), bytes.HasPrefix(head, []byte{0xce, 0xfa, 0xed, 0xfe}),
		bytes.HasPrefix(head, []byte{0xfe, 0xed, 0xfa, 0xcf}), bytes.HasPrefix(head, []byte{0xfe, 0xed, 0xfa, 0xce}):
		return "Mach-O"
	case bytes.HasPrefix(head, []byte("MZ")) && bytes.IndexByte(head, 0) >= 0:
		return "PE"
	}
	return ""
}

func TestExecutableKind(t *testing.T) {
	dir := t.TempDir()
	cases := map[string][]byte{
		"ELF":    {0x7f, 'E', 'L', 'F', 2, 1, 1, 0},
		"Mach-O": {0xcf, 0xfa, 0xed, 0xfe, 0x0c, 0, 0, 1},
		"PE":     append([]byte("MZ"), 0x90, 0, 3, 0),
		"":       []byte("MZ is how this line of text starts\n"),
	}
	for want, b := range cases {
		p := filepath.Join(dir, "f"+want)
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		if got := executableKind(p); got != want {
			t.Errorf("executableKind(%q) = %q, want %q", b, got, want)
		}
	}
}
