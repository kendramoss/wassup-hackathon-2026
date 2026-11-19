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

func revisionFixture(t *testing.T, facts map[string]string, labels []Label) *Store {
	t.Helper()
	dir := t.TempDir()
	for id, f := range facts {
		if err := os.WriteFile(filepath.Join(dir, id+".txt"), []byte(f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s, err := Load(labels, dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func revLabel(id, date, origin, holding, subject string) Label {
	return Label{Ruling: id, Date: date, Subject: subject, PrimaryOrigin: &origin, Evidence: []string{holding}}
}

func TestStoreRevision(t *testing.T) {
	facts := map[string]string{"N100001": "alpha facts\n", "N100002": "beta facts\n"}
	base := []Label{
		revLabel("N100001", "2025-01-02", "Thailand", "Origin is Thailand.", "Heat gun"),
		revLabel("N100002", "2025-01-03", "Vietnam", "Origin is Vietnam.", "Lamp"),
	}
	r0 := revisionFixture(t, facts, base).Revision()
	if len(r0) != 64 {
		t.Fatalf("revision %q is not hex sha256", r0)
	}
	if again := revisionFixture(t, facts, base).Revision(); again != r0 {
		t.Error("same inputs must give the same revision (no run paths or timestamps)")
	}
	// A label without a usable origin or facts is not evidence.
	extra := append(append([]Label{}, base...), Label{Ruling: "N100009", Date: "2025-02-01"}, revLabel("N100008", "2025-02-01", "PRC", "x", "no facts file"))
	if got := revisionFixture(t, facts, extra).Revision(); got != r0 {
		t.Error("rulings outside the store must not change the revision")
	}

	changed := map[string]struct {
		facts  map[string]string
		labels []Label
	}{
		"facts corrected": {map[string]string{"N100001": "alpha facts corrected\n", "N100002": "beta facts\n"}, base},
		"origin":          {facts, []Label{revLabel("N100001", "2025-01-02", "PRC", "Origin is Thailand.", "Heat gun"), base[1]}},
		"holding":         {facts, []Label{revLabel("N100001", "2025-01-02", "Thailand", "Origin is Thailand!", "Heat gun"), base[1]}},
		"date":            {facts, []Label{revLabel("N100001", "2025-01-04", "Thailand", "Origin is Thailand.", "Heat gun"), base[1]}},
		"subject":         {facts, []Label{revLabel("N100001", "2025-01-02", "Thailand", "Origin is Thailand.", "Heater"), base[1]}},
		"removed":         {map[string]string{"N100001": "alpha facts\n"}, base},
		"added":           {map[string]string{"N100001": "alpha facts\n", "N100002": "beta facts\n", "N100003": "g\n"}, append(append([]Label{}, base...), revLabel("N100003", "2025-01-05", "India", "h", "s"))},
		// BM25 breaks score ties by corpus position, so order is evidence.
		"order": {facts, []Label{base[1], base[0]}},
	}
	for name, c := range changed {
		if got := revisionFixture(t, c.facts, c.labels).Revision(); got == r0 {
			t.Errorf("%s: revision unchanged", name)
		}
	}
	// Field boundaries are unambiguous.
	a := revisionFixture(t, map[string]string{"N100001": "ab\n"}, []Label{revLabel("N100001", "2025-01-02", "Thailand", "c", "")})
	b := revisionFixture(t, map[string]string{"N100001": "a\n"}, []Label{revLabel("N100001", "2025-01-02", "Thailand", "bc", "")})
	if a.Revision() == b.Revision() {
		t.Error("field boundaries must be part of the hash")
	}
}

func TestRevisionOfCoversReasoning(t *testing.T) {
	s := revisionFixture(t, map[string]string{"N100001": "f\n"}, []Label{revLabel("N100001", "2025-01-02", "Thailand", "h", "s")})
	if RevisionOf(s.Rulings()) != s.Revision() {
		t.Error("RevisionOf over the store rulings must equal Store.Revision")
	}
	with := WithReasoning(s.Rulings(), map[string]string{"N100001": "CBP reasoning"})
	if RevisionOf(with) == s.Revision() {
		t.Error("attached reasoning is shown in the prompt, so it must change the revision")
	}
}
