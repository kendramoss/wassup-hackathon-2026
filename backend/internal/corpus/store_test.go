package corpus

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadStore(t *testing.T) {
	dir := t.TempDir()
	write := func(id, facts string) {
		if err := os.WriteFile(filepath.Join(dir, id+".txt"), []byte(facts), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("N100001", "Heat gun facts\n")
	write("N100002", "No-origin facts\n")
	// N100003 has a label but no facts file.
	th, cn := "Thailand", "PRC"
	labels := []Label{
		{Ruling: "N100001", Date: "2025-01-02", PrimaryOrigin: &th, Evidence: []string{"Origin is", "Thailand."}},
		{Ruling: "N100002", Date: "2025-01-03"},
		{Ruling: "N100003", Date: "2025-01-04", PrimaryOrigin: &cn},
	}
	s, err := Load(labels, dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Len() != 1 {
		t.Fatalf("Len = %d, want 1 (only rulings with facts and a single origin)", s.Len())
	}
	r, ok := s.Get("N100001")
	if !ok || r.Facts != "Heat gun facts\n" || r.Holding != "Origin is Thailand." || r.Origin != "Thailand" || r.Date.Format("2006-01-02") != "2025-01-02" {
		t.Errorf("ruling = %+v", r)
	}
	if _, ok := s.Label("N100002"); !ok {
		t.Error("labels without an origin should still be available")
	}
	if _, ok := s.Get("N100003"); ok {
		t.Error("ruling without facts should not be in the store")
	}
}

func TestLoadSplitsAndLabelsFromRepo(t *testing.T) {
	sp, err := LoadSplits(filepath.Join("..", "..", "..", "eval", "splits", "splits_final.json"))
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]int{"final": 377, "dev": 469, "pilot": 12} {
		ids, err := sp.IDs(name)
		if err != nil || len(ids) != want {
			t.Errorf("%s: %d ids (%v), want %d", name, len(ids), err, want)
		}
	}
	if _, err := sp.IDs("test"); err == nil {
		t.Error("unknown split should error")
	}
	labels, err := LoadLabels(filepath.Join("..", "..", "..", "eval", "data", "labels.jsonl"))
	if err != nil || len(labels) != 2790 {
		t.Fatalf("labels: %d, %v", len(labels), err)
	}
}
