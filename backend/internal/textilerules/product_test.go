package textilerules

import (
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

func TestFromProduct(t *testing.T) {
	p := domain.Product{
		Name: "Dress", HTS: "6104.43.2020",
		Plants: []domain.Plant{{ID: "a", Country: "China"}, {ID: "b", Country: "Vietnam"}},
		Operations: []domain.Operation{
			{Name: "Knit the fabric", Plant: "a"},
			{Name: "Cut and sew", Plant: "b"},
			{Name: "Pack for export", Plant: "b"},
		},
		Textile: &domain.TextileFacts{KnitToShape: "no", TwoOrMoreComponents: "yes"},
	}
	f, notes := FromProduct(p)
	if len(notes) != 0 || !f.Complete {
		t.Fatalf("notes %v complete %v", notes, f.Complete)
	}
	want := []Stage{{Process: ProcFabricMaking, Country: "China"}, {Process: StageCutting, Country: "Vietnam"}, {Process: ProcAssembly, Country: "Vietnam"}, {Process: StagePacking, Country: "Vietnam"}}
	if len(f.Stages) != len(want) {
		t.Fatalf("stages %+v", f.Stages)
	}
	for i, s := range want {
		if f.Stages[i] != s {
			t.Errorf("stage %d = %+v, want %+v", i, f.Stages[i], s)
		}
	}
	if f.KnitToShape != No || f.TwoOrMoreComponents != Yes || f.CottonWool16 != Unknown {
		t.Errorf("facts %+v", f)
	}
	// "knit to shape" is its own operation, not also fabric-making.
	p.Operations = []domain.Operation{{Name: "Knit to shape (fully fashioned)", Plant: "a"}}
	f, _ = FromProduct(p)
	if len(f.Stages) != 1 || f.Stages[0].Process != ProcKnitToShape {
		t.Errorf("stages %+v", f.Stages)
	}
	// An operation the rules do not read leaves the chain incomplete.
	p.Operations = []domain.Operation{{Name: "Laser engraving", Plant: "a"}}
	f, notes = FromProduct(p)
	if f.Complete || len(notes) != 1 {
		t.Errorf("complete %v notes %v", f.Complete, notes)
	}
}
