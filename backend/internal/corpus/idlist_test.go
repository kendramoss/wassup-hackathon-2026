package corpus

import "testing"

func TestTextileRulingsShipWithLabelsAndFacts(t *testing.T) {
	ids, err := LoadIDList("../../data/textile_rulings.json")
	if err != nil {
		t.Fatal(err)
	}
	labels, err := LoadLabels("../../data/labels.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	s, err := Load(labels, "../../data/facts")
	if err != nil {
		t.Fatal(err)
	}
	missing := 0
	for _, id := range ids {
		if _, ok := s.Get(id); !ok {
			missing++
		}
	}
	if len(ids) < 300 || missing > 0 {
		t.Errorf("%d textile rulings, %d without facts or label in the image's data", len(ids), missing)
	}
}
