package export

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

var update = flag.Bool("update", false, "rewrite golden files")

// heatGun is the heat-gun demo's product with the motor-fan assembly and
// PCBA moved to Thailand, trimmed to a few components and operations.
func heatGun() *domain.Product {
	return &domain.Product{
		Name:        "Kobalt 12.5-amp heat gun, model 5208257",
		Description: "A 1500-watt heat gun with two fan speeds and a variable temperature control dial, used for heat shrinking, stripping paint and bending plastic.",
		HTS:         "8419.89.95",
		Plants:      []domain.Plant{{ID: "th", Name: "Rayong plant", Country: "Thailand"}, {ID: "cn", Name: "China suppliers", Country: "China"}},
		Components: []domain.Component{
			{Name: "main housing", MadeIn: "Thailand", Plant: "th"},
			{Name: "electric motor", MadeIn: "Thailand", Plant: "th"},
			{Name: "printed circuit board assembly (PCBA)", MadeIn: "Thailand", Plant: "th"},
			{Name: "power cord", MadeIn: "China", Plant: "cn"},
			{Name: "switch", MadeIn: "Malaysia"},
		},
		Operations: []domain.Operation{
			{Name: "wind the motor stator and assemble the motor-fan unit", Plant: "th"},
			{Name: "fasten the PCBA to the main housing and connect it to the motor", Plant: "th"},
		},
	}
}

func goldenInputs() map[string]Input {
	return map[string]Input{
		"plan": {
			ProductName: "Kobalt 12.5-amp heat gun, model 5208257", Product: heatGun(),
			Facts:     "The motor-fan assembly and PCBA are made in Thailand. Final assembly in Thailand.",
			Requested: "Thailand", PlanSummary: "Make Motor-fan assembly and PCBA (SMT) in Thailand", CurrentRuled: "China (N328954)",
			Prediction: domain.Prediction{KeyFactors: []string{"Motor-fan assembly made in Thailand", "PCBA populated in Thailand"}, CitedRulings: []string{"N350001", "N347421"}},
			Supporting: "N347421", Counter: "N345859",
		},
		"facts_only": {
			Facts:      "Steel brackets are stamped in Vietnam from Chinese coil and zinc plated in Vietnam.",
			Prediction: domain.Prediction{Precedents: []string{"N340187"}},
		},
		"below_bar": {
			ProductName: "heat gun", Facts: "PCBA made in Thailand.", Requested: "Thailand", PlanSummary: "Make PCBA in Thailand",
			CurrentRuled: "China (N328954)", Supporting: "N347421", BelowBar: &BelowBar{Confidence: 0.88, Bar: 0.92},
		},
		"textile": {
			ProductName: "knit neck gaiter", Textile: true,
			Facts:     "Polyester fabric is knit and dyed in China; the gaiters are cut and sewn in Vietnam.",
			Requested: "Vietnam", Prediction: domain.Prediction{KeyFactors: []string{"Cut and sewn in Vietnam"}, CitedRulings: []string{"N364101"}},
		},
	}
}

// TestGolden pins both formats' full text for a few cases; run with -update
// after an intended change and read the diff.
func TestGolden(t *testing.T) {
	for name, in := range goldenInputs() {
		for format, render := range map[string]func(Input) string{"markdown": Markdown, "cfr177": CFR177} {
			got := render(in)
			if got != render(in) {
				t.Fatalf("%s %s is not deterministic", format, name)
			}
			golden := filepath.Join("testdata", format+"_"+name+".golden")
			if *update {
				_ = os.MkdirAll("testdata", 0o755)
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v (run with -update)", err)
			}
			if got != string(want) {
				t.Errorf("%s draft for %s changed; run with -update if intended:\n%s", format, name, got)
			}
		}
	}
}
