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

func TestPharmaRulingsShipWithLabelsAndFacts(t *testing.T) {
	ids, err := LoadIDList("../../data/pharma_rulings.json")
	if err != nil {
		t.Fatal(err)
	}
	labels, err := LoadLabels("../../data/labels.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	regime := map[string]string{}
	for _, l := range labels {
		regime[l.Ruling] = l.Regime
	}
	s, err := Load(labels, "../../data/facts")
	if err != nil {
		t.Fatal(err)
	}
	missing, other := 0, 0
	for _, id := range ids {
		if _, ok := s.Get(id); !ok {
			missing++
		}
		if regime[id] != "pharma_api" {
			other++
		}
	}
	if len(ids) < 180 || missing > 0 || other > 0 {
		t.Errorf("%d pharma rulings, %d without facts or label in the image's data, %d not labelled pharma_api", len(ids), missing, other)
	}
}

func TestTextileReasoningShips(t *testing.T) {
	m, err := LoadReasoning("../../data/textile_reasoning.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(m) < 350 {
		t.Errorf("reasoning for %d textile rulings", len(m))
	}
}
