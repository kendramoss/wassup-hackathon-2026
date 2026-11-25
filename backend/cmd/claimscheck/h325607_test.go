package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestH325607DevRuns recomputes the NUMBERS row for H325607 (#566): wrong in
// 26 of 26 development engine runs, 25 Spain and 1 China, each showing
// N331974. The rows are in the development engine result files; since D370
// the supersession impact report no longer lists them.
func TestH325607DevRuns(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "..", "eval", "results", "dev-*-engine-v1*.jsonl"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no development engine files: %v", err)
	}
	var runs, wrong, shown int
	preds := map[string]int{}
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 1<<20), 1<<26)
		for sc.Scan() {
			if !strings.Contains(sc.Text(), `"H325607"`) {
				continue
			}
			var r struct {
				Ruling     string          `json:"ruling"`
				Pred       string          `json:"pred"`
				Correct    bool            `json:"correct"`
				Precedents json.RawMessage `json:"precedents"`
			}
			if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			if r.Ruling != "H325607" {
				continue
			}
			runs++
			preds[r.Pred]++
			if !r.Correct {
				wrong++
			}
			if strings.Contains(string(r.Precedents), "N331974") {
				shown++
			}
		}
		fh.Close()
		if err := sc.Err(); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
	}
	if runs != 26 || wrong != 26 || shown != 26 || preds["Spain"] != 25 || preds["China"] != 1 {
		t.Fatalf("H325607 development runs: %d runs, %d wrong, %d showed N331974, predictions %v; docs/NUMBERS.md says 26, 26, 26, 25 Spain and 1 China", runs, wrong, shown, preds)
	}
}
