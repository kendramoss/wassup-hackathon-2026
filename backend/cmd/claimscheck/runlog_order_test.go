package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// #637 (D623): the RUNLOG rows of one decision are the record a reader checks
// a pre-registration against, so they must be in run order and each must name
// the revision it ran at. Older rows carry no decision field and are not
// checked.
func TestRunlogDecisionRowsAreInTimeOrder(t *testing.T) {
	f, err := os.Open(filepath.Join("..", "..", "..", "eval", "results", "RUNLOG.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	last := map[string]time.Time{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for n := 1; sc.Scan(); n++ {
		var r struct {
			Time        string `json:"time"`
			Run         string `json:"run"`
			Decision    string `json:"decision"`
			GitRevision string `json:"git_revision"`
		}
		if len(sc.Bytes()) == 0 {
			continue
		}
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatalf("line %d: %v", n, err)
		}
		if r.Decision == "" {
			continue
		}
		at, err := time.Parse(time.RFC3339, r.Time)
		if err != nil {
			t.Errorf("line %d (%s): time %q: %v", n, r.Run, r.Time, err)
			continue
		}
		if p, ok := last[r.Decision]; ok && at.Before(p) {
			t.Errorf("line %d (%s, %s): %s is before the previous %s row (%s)", n, r.Run, r.Decision, r.Time, r.Decision, p.Format(time.RFC3339))
		}
		last[r.Decision] = at
		if r.GitRevision == "" {
			t.Errorf("line %d (%s, %s): no git_revision", n, r.Run, r.Decision)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
}
