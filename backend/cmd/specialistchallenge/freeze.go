package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var shaRE = regexp.MustCompile(`^[0-9a-f]{40}$`)

// freezePaths define the candidate and the data: anything changed there after
// the freeze commit stops a reserved run.
var freezePaths = []string{"eval/challenges/specialists/textile-test.jsonl", "eval/challenges/specialists/pharma-test.jsonl",
	"eval/challenges/specialists/part102-test.jsonl", "eval/challenges/specialists/japan-test.jsonl",
	"eval/challenges/specialists/textile-dev.jsonl", "eval/challenges/specialists/pharma-dev.jsonl",
	"eval/challenges/specialists/part102-dev.jsonl", "eval/challenges/specialists/japan-dev.jsonl",
	"eval/challenges/specialists/routing-dev.jsonl", "eval/challenges/specialists/routing-test.jsonl",
	"backend/internal/engine", "backend/internal/jpengine", "backend/internal/jprules", "backend/internal/router", "backend/internal/domain",
	"backend/internal/retrieval", "backend/internal/triage", "backend/cmd/specialistchallenge", "backend/data", "backend/go.mod", "backend/go.sum"}

// FrozenGuard refuses a reserved-test run unless HEAD is the declared frozen
// commit, the candidate and data are committed unchanged, and no result for
// this scope exists yet.
func FrozenGuard(git func(args ...string) (string, error), sha, resultsPath string) error {
	if !shaRE.MatchString(sha) {
		return fmt.Errorf("-freeze must be the full 40-character commit sha")
	}
	head, err := git("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if strings.TrimSpace(head) != sha {
		return fmt.Errorf("HEAD is %s, not the declared frozen commit %s", strings.TrimSpace(head), sha)
	}
	st, err := git(append([]string{"status", "--porcelain", "--"}, freezePaths...)...)
	if err != nil {
		return err
	}
	if strings.TrimSpace(st) != "" {
		return fmt.Errorf("candidate or dataset files differ from the frozen commit:\n%s", st)
	}
	if _, err := os.Stat(resultsPath); err == nil {
		return fmt.Errorf("%s exists: the reserved test is scored once and never overwritten", filepath.Base(resultsPath))
	}
	return nil
}
