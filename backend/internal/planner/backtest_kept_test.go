package planner

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// #519: a part CBP's change kept in the old country counts as kept when the
// demo maps it (still_from_china_as) or when a component has the same name.
// The gas-grill plan from the committed live check moves the burners,
// control panels and cart frames, which N335085's change kept in China, so
// it is not a match.
func TestBacktestFindsKeptPartsByComponentName(t *testing.T) {
	r, _ := fixture(t, "families/gas_grill_2023.json")
	m := domain.Move{{Group: "lid_and_firebox", ToPlant: "id"}, {Group: "burners_and_gas_train", ToPlant: "id"}, {Group: "cooking_surface_frame", ToPlant: "id"}}
	bt := backtest(r, []domain.Plan{{Move: m, Description: "three groups"}})
	if want := []string{"Burners", "Control panels", "Cart frames"}; !slices.Equal(bt.AlsoMovesKept, want) {
		t.Errorf("plan_also_moves_kept = %v, want %v", bt.AlsoMovesKept, want)
	}
	if len(bt.NotMovedByPlan) != 0 {
		t.Errorf("missed = %v", bt.NotMovedByPlan)
	}
	// The exact move: the lids and fireboxes alone.
	exact := backtest(r, []domain.Plan{{Move: domain.Move{{Group: "lid_and_firebox", ToPlant: "id"}}}})
	if len(exact.AlsoMovesKept) != 0 || len(exact.NotMovedByPlan) != 0 {
		t.Errorf("exact move: %+v", exact)
	}
}

func TestBacktestTubLightLEDAloneStillMatches(t *testing.T) {
	r, _ := fixture(t, "tublight_2025.json")
	bt := backtest(r, []domain.Plan{{Move: domain.Move{{Group: "led_pcba", ToPlant: "th"}}}})
	if len(bt.AlsoMovesKept) != 0 || len(bt.NotMovedByPlan) != 0 || len(bt.MovedByPlan) != 1 {
		t.Errorf("LED PCBA alone: %+v", bt)
	}
	all := backtest(r, []domain.Plan{{Move: domain.Move{{Group: "led_pcba", ToPlant: "th"}, {Group: "lens", ToPlant: "th"}, {Group: "base_harness_seals", ToPlant: "th"}}}})
	if want := []string{"lens", "lamp base", "wiring harness", "PVC sealing and O-ring"}; !slices.Equal(all.AlsoMovesKept, want) {
		t.Errorf("all three groups: plan_also_moves_kept = %v, want %v", all.AlsoMovesKept, want)
	}
}

// Every fixture that records CBP's real change names each kept part in a way
// the backtest can find, except the heat gun's terminal blocks, which the
// product has no component for (D310).
func TestEveryKeptPartResolvesToAComponent(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("..", "..", "testdata", "*.json"))
	more, _ := filepath.Glob(filepath.Join("..", "..", "testdata", "*", "*.json"))
	checked := 0
	for _, f := range append(files, more...) {
		r, err := LoadRequest(f)
		if err != nil || r.ActualChange == nil {
			continue
		}
		checked++
		for _, w := range r.ActualChange.StillImported {
			if _, ok := r.ActualChange.KeptPart(r.Product, w); !ok && w != "terminal blocks" {
				t.Errorf("%s: kept part %q matches no component", filepath.Base(f), w)
			}
		}
	}
	if checked < 8 {
		t.Errorf("only %d fixtures with an actual change", checked)
	}
}
