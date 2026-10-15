package retrieval

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// TestResearchParity checks that BM25 retrieves the same 5 precedents, in the
// same order, as the research run behind the 89.1% headline
// (results/final-gemini-4-argon-prec.jsonl). Opt-in: set
// ORIGINPROOF_RESEARCH_RESULTS to that file and build the corpus first
// (go run ./cmd/corpus build).
func TestResearchParity(t *testing.T) {
	resultsPath := os.Getenv("ORIGINPROOF_RESEARCH_RESULTS")
	if resultsPath == "" {
		t.Skip("set ORIGINPROOF_RESEARCH_RESULTS to run")
	}
	root := filepath.Join("..", "..", "..")
	labels, err := corpus.LoadLabels(filepath.Join(root, "eval", "data", "labels.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := corpus.Load(labels, filepath.Join(root, "data", "corpus", "facts"))
	if err != nil {
		t.Fatal(err)
	}
	idx := NewBM25(store.Rulings(), func(r domain.Ruling) string { return corpus.Truncate(r.Facts, DocRunes) })

	f, err := os.Open(resultsPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	var n, same, sameSet int
	for sc.Scan() {
		var row struct {
			Ruling     string   `json:"ruling"`
			Date       string   `json:"date"`
			Precedents []string `json:"precedents"`
		}
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		r, ok := store.Get(row.Ruling)
		if !ok {
			t.Errorf("%s not in store", row.Ruling)
			continue
		}
		date, _ := time.Parse(time.DateOnly, row.Date)
		hits := idx.Top(r.Facts, 5, PrecedentFilter(domain.Case{ID: row.Ruling, Date: date}))
		got := make([]string, len(hits))
		for i, h := range hits {
			got[i] = h.Ruling.ID
		}
		n++
		if equal(got, row.Precedents) {
			same++
		} else if sameElements(got, row.Precedents) {
			sameSet++
		} else if n-same-sameSet <= 5 {
			t.Logf("%s: got %v, research %v", row.Ruling, got, row.Precedents)
		}
	}
	t.Logf("%d cases: %d identical precedent lists, %d same set in a different order, %d different", n, same, sameSet, n-same-sameSet)
	// Measured 15 Oct: 369 identical, 6 reordered, 2 differ in the 5th
	// precedent. The labels file gained manual labels after the research run,
	// which shifts IDF and document-length statistics slightly.
	if float64(same+sameSet) < 0.97*float64(n) {
		t.Errorf("only %d of %d cases retrieved the research precedent set", same+sameSet, n)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sameElements(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]int{}
	for _, x := range a {
		m[x]++
	}
	for _, x := range b {
		m[x]--
	}
	for _, v := range m {
		if v != 0 {
			return false
		}
	}
	return true
}
