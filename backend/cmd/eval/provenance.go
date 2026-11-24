package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// datasetSHA256 hashes each input file a run reads, keyed by its path from
// the repository root (D840), so a RUNLOG row names the exact labels and
// splits its figures came from. A comma-separated path is split.
func datasetSHA256(paths ...string) (map[string]string, error) {
	top := repoTop()
	out := map[string]string{}
	for _, p := range paths {
		for _, one := range strings.Split(p, ",") {
			one = strings.TrimSpace(one)
			if one == "" {
				continue
			}
			b, err := os.ReadFile(one)
			if err != nil {
				return nil, err
			}
			key := one
			if abs, err := filepath.Abs(one); err == nil && top != "" {
				if rel, err := filepath.Rel(top, abs); err == nil {
					key = filepath.ToSlash(rel)
				}
			}
			out[key] = fmt.Sprintf("%x", sha256.Sum256(b))
		}
	}
	return out, nil
}

// textSHA256 hashes a list of strings joined by newlines: the exact inputs a
// run sent, for inputs built from many files (familycov's case texts).
func textSHA256(parts []string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(parts, "\n"))))
}

// codeDirty reports uncommitted changes to tracked files anywhere in the
// repository (untracked files, such as a new result file, do not count).
// An error reading git counts as dirty.
func codeDirty() bool {
	out, err := exec.Command("git", "status", "--porcelain", "--untracked-files=no").Output()
	return err != nil || len(strings.TrimSpace(string(out))) > 0
}

func repoTop() string {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
