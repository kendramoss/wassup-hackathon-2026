package planner

import (
	"slices"
	"strings"
	"testing"
)

// The heat gun's real 2026 change (N357771) moved the motor, fan, PCBA, knob,
// power cord and back cover to Thailand and kept the switch, wiring and
// fasteners in China. With the power cord, knobs and covers in their own group
// (D330), the plan search lists exactly that move among its candidates, and
// it moves none of the parts CBP's change kept in China. This was changed
// after seeing N357771, so it is not evidence of accuracy.
func TestHeatGunSearchListsCBPsSmallerMove(t *testing.T) {
	r, tm := fixture(t, "heatgun_2022.json")
	moves := EnumerateUpTo(r.Product, tm, "Thailand", r.Economics.GroupCosts, DefaultMaxMoves)
	if len(moves) != DefaultMaxMoves {
		t.Errorf("%d moves, want %d", len(moves), DefaultMaxMoves)
	}
	want := []string{"cord_knobs_covers", "motor_fan_assembly", "pcba"}
	all := []string{"cord_knobs_covers", "motor_fan_assembly", "pcba", "switch_cord_minor_parts"}
	found := false
	for _, m := range moves {
		g := groupsOf(m)
		slices.Sort(g)
		if slices.Equal(g, all) {
			t.Errorf("the four-group move is listed: only the follow-advice step may add a fourth group")
		}
		if !slices.Equal(g, want) {
			continue
		}
		found = true
		for _, gm := range m {
			if gm.ToPlant != "th" {
				t.Errorf("%s goes to %s, want th", gm.Group, gm.ToPlant)
			}
		}
		moved := map[string]bool{}
		for i, c := range Apply(r.Product, m).Components {
			if c.MadeIn != r.Product.Components[i].MadeIn {
				moved[c.Name] = true
			}
		}
		a := r.ActualChange
		for _, name := range a.MovedComponents {
			if !slices.Contains(a.AlreadyInTarget, name) && !moved[name] {
				t.Errorf("CBP moved %q; this move does not", name)
			}
		}
		for word, part := range a.StillImportedAs {
			if moved[part] {
				t.Errorf("CBP kept %q (%s) in China; this move moves it", word, part)
			}
		}
		if d := Describe(r.Product, tm, m); !strings.Contains(d, "power cord") || strings.Contains(d, "switch") {
			t.Errorf("Describe = %q", d)
		}
	}
	if !found {
		t.Fatalf("CBP's move (motor-fan, PCBA, cord/knobs/covers to Thailand) is not among %d candidates", len(moves))
	}
}
