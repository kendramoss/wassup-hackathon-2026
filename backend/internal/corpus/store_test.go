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

// The pharma split is dated: dev is Jul–Dec 2025, final is 2026, both drawn
// from the pharma pool, and the server's list is the same pool (D62).
func TestPharmaSplitIsDated(t *testing.T) {
	sp, err := LoadSplits(filepath.Join("..", "..", "..", "eval", "splits", "splits_pharma.json"))
	if err != nil {
		t.Fatal(err)
	}
	labels, err := LoadLabels(filepath.Join("..", "..", "..", "eval", "data", "labels.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Label{}
	for _, l := range labels {
		byID[l.Ruling] = l
	}
	pool := map[string]bool{}
	for _, id := range sp.Pharma {
		pool[id] = true
		if l := byID[id]; l.Regime != "pharma_api" || l.Origin() == "" {
			t.Errorf("%s in the pharma pool: regime %q origin %q", id, l.Regime, l.Origin())
		}
	}
	check := func(name string, ids []string, from, to string) {
		for _, id := range ids {
			if d := byID[id].Date; !pool[id] || d < from || d >= to {
				t.Errorf("%s: %s dated %s (in pool %v), want [%s, %s)", name, id, d, pool[id], from, to)
			}
		}
	}
	check("dev", sp.Dev, "2025-07-01", "2026-01-01")
	check("final", sp.Final, "2026-01-01", "2027-01-01")
	if len(sp.Pharma) != 187 || len(sp.Dev) != 35 || len(sp.Final) != 56 {
		t.Errorf("pool %d, dev %d, final %d", len(sp.Pharma), len(sp.Dev), len(sp.Final))
	}
	server, err := LoadIDList(filepath.Join("..", "..", "data", "pharma_rulings.json"))
	if err != nil || len(server) != len(sp.Pharma) {
		t.Errorf("server list: %d rulings, %v", len(server), err)
	}
}
