package demos

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

func TestDemosAreValidRequests(t *testing.T) {
	ds, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(ds) != 4 {
		t.Fatalf("demos = %d, want 4", len(ds))
	}
	for _, d := range ds {
		var p struct {
			Date    string          `json:"date"`
			Facts   string          `json:"facts"`
			Product *domain.Product `json:"product"`
			Family  string          `json:"family"`
			Exclude []string        `json:"exclude_rulings"`
		}
		if err := json.Unmarshal(d.Predict, &p); err != nil {
			t.Fatalf("%s: %v", d.ID, err)
		}
		if p.Date == "" || (p.Facts == "" && p.Product == nil) || len(p.Exclude) == 0 || d.CBPHeld == "" {
			t.Errorf("%s: incomplete predict request %+v", d.ID, p)
		}
		if len([]rune(p.Facts)) > domain.MaxFactsRunes {
			t.Errorf("%s: facts too long", d.ID)
		}
		if p.Product != nil {
			if err := p.Product.Validate(); err != nil {
				t.Errorf("%s: %v", d.ID, err)
			}
		}
	}
	hg := ds[0]
	if hg.ID != "heatgun-2022" || !strings.Contains(string(hg.Predict), "Each heat gun is assembled in Thailand using 24 components") {
		t.Errorf("heat gun demo should predict from CBP's 2022 facts text: %s", hg.ID)
	}
	if !Summaries(ds)[0].HasPlan || Summaries(ds)[3].HasPlan {
		t.Errorf("summaries = %+v", Summaries(ds))
	}
}
