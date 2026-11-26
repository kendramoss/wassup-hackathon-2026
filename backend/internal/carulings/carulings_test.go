package carulings

import (
	"bufio"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

func TestEmbeddedMatchesData(t *testing.T) {
	f, err := os.Open("../../../data/ca/rulings.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var rows []Ruling
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var r Ruling
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, r)
	}
	if !reflect.DeepEqual(rows, All()) {
		t.Fatal("rulings.json differs from data/ca/rulings.jsonl; run eval/ca/parse.py")
	}
	if len(rows) != 14 {
		t.Fatalf("got %d rulings, want 14", len(rows))
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if seen[r.ID] || r.Holding == "" || r.URL == "" || len(r.HS) == 0 {
			t.Fatalf("bad row %+v", r)
		}
		seen[r.ID] = true
	}
}

func ids(rs []domain.CanadaRuling) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.ID)
	}
	return out
}

func TestForCodeMatchFirst(t *testing.T) {
	got := For(&domain.Product{Name: "ABS resin", HTS: "3903.90.00"})
	if len(got) == 0 || len(got) > MaxShown {
		t.Fatalf("got %v", ids(got))
	}
	// Both 3903.90 rulings, newest first.
	if got[0].ID != "8000005476" || got[1].ID != "8000005665" {
		t.Fatalf("got %v", ids(got))
	}
	if got[0].Holding != "Not originating under CUSMA" || got[0].Kind != "origin" {
		t.Fatalf("got %+v", got[0])
	}
}

func TestForHeadingBeforeChapter(t *testing.T) {
	got := ids(For(&domain.Product{Name: "Throw pillow", HTS: "9404.90.10"}))
	if len(got) < 2 || !(got[0] == "8000002867" || got[0] == "8000002155") {
		t.Fatalf("got %v", got)
	}
}

func TestForWordsWithoutCode(t *testing.T) {
	got := ids(For(&domain.Product{Name: "Linen blouse", Description: "woven linen blouse",
		Components: []domain.Component{{Name: "linen fabric"}}}))
	if len(got) == 0 || got[0] != "8000003153" {
		t.Fatalf("got %v", got)
	}
}

func TestForNothingRelated(t *testing.T) {
	if got := For(&domain.Product{Name: "Laptop computer", HTS: "8471.30",
		Components: []domain.Component{{Name: "Battery", HTS: "8507.60"}}}); got != nil {
		t.Fatalf("got %v", ids(got))
	}
	if For(nil) != nil {
		t.Fatal("nil product must give nil")
	}
}
