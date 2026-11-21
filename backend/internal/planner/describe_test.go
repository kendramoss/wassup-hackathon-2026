package planner

import (
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// Move descriptions read as a sentence (D220): "the" before each group, no
// capital mid-sentence, acronyms kept.
func TestDescribeReadsAsASentence(t *testing.T) {
	tms, err := LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	p := domain.Product{Plants: []domain.Plant{{ID: "cn", Country: "China"}, {ID: "id", Country: "Indonesia"}, {ID: "vn", Country: "Vietnam"}}}
	for _, c := range []struct {
		family string
		move   domain.Move
		want   string
	}{
		{"gas_grills_cooking", domain.Move{{Group: "lid_and_firebox", ToPlant: "id"}, {Group: "burners_and_gas_train", ToPlant: "id"}, {Group: "cooking_surface_frame", ToPlant: "id"}},
			"Move the lid and firebox forming, the burners and gas train and the cooking surface, panels and frame to Indonesia"},
		{"led_lights", domain.Move{{Group: "led_pcba", ToPlant: "vn"}}, "Move the LED PCBA (SMT) to Vietnam"},
		{"lidar_modules", domain.Move{{Group: "rx_tx_boards", ToPlant: "vn"}}, "Move the RX and TX PCBAs to Vietnam"},
		{"motorised_tools_small_appliances", domain.Move{{Group: "pcba", ToPlant: "id"}, {Group: "motor_fan_assembly", ToPlant: "vn"}},
			"Move the PCBA (SMT) to Indonesia and the motor-fan assembly to Vietnam"},
		{"motorised_tools_small_appliances", nil, "No change: keep the current setup"},
	} {
		tm, ok := tms[c.family]
		if !ok {
			t.Fatalf("no template %s", c.family)
		}
		if got := Describe(p, tm, c.move); got != c.want {
			t.Errorf("%s: Describe = %q, want %q", c.family, got, c.want)
		}
	}
}

// A stored search keeps its moves and verdicts; only the words change.
func TestRedescribeRewritesStoredWords(t *testing.T) {
	tms, err := LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	p := domain.Product{Plants: []domain.Plant{{ID: "th", Country: "Thailand"}}}
	m := domain.Move{{Group: "motor_fan_assembly", ToPlant: "th"}}
	stored := []domain.Plan{{Move: m, Description: "Make Motor-fan assembly in Thailand", Shown: true}}
	r := Result{Family: "motorised_tools_small_appliances", Plans: stored, Backtest: &Backtest{PlanMoves: "Make Motor-fan assembly in Thailand"}}
	Redescribe(&r, &p, tms)
	if r.Plans[0].Description != "Move the motor-fan assembly to Thailand" || r.Backtest.PlanMoves != r.Plans[0].Description || r.Baseline.Description != "No change: keep the current setup" {
		t.Errorf("redescribed = %+v / %+v", r.Plans[0], r.Backtest)
	}
	if stored[0].Description != "Make Motor-fan assembly in Thailand" {
		t.Error("Redescribe wrote into the stored slice")
	}
	unknown := Result{Family: "nope", Plans: stored}
	Redescribe(&unknown, &p, tms)
	if unknown.Plans[0].Description != "Make Motor-fan assembly in Thailand" {
		t.Error("a family without a template must be left as stored")
	}
}

// A scenario and a plan row that move the same groups to the same plants are
// one move (D220), whatever order the groups are listed in.
func TestMatchMove(t *testing.T) {
	cur := domain.Product{
		Plants:     []domain.Plant{{ID: "cn", Country: "China"}, {ID: "th", Country: "Thailand"}},
		Components: []domain.Component{{Name: "motor", Group: "motor_fan_assembly", Plant: "cn", MadeIn: "China"}, {Name: "board", Group: "pcba", Plant: "cn", MadeIn: "China"}},
	}
	both := Apply(cur, domain.Move{{Group: "pcba", ToPlant: "th"}, {Group: "motor_fan_assembly", ToPlant: "th"}})
	motor := Apply(cur, domain.Move{{Group: "motor_fan_assembly", ToPlant: "th"}})
	r := &Result{
		Plans:    []domain.Plan{{Move: domain.Move{{Group: "motor_fan_assembly", ToPlant: "th"}, {Group: "pcba", ToPlant: "th"}}, Shown: true}},
		Rejected: []domain.Plan{{Move: domain.Move{}, RejectionKind: domain.RejectCurrentSetup}, {Move: domain.Move{{Group: "motor_fan_assembly", ToPlant: "th"}}, RejectionKind: domain.RejectBelowConfidence}},
	}
	if got := MatchMove(r, cur, both); got == nil || !got.Shown {
		t.Errorf("both groups: %+v", got)
	}
	if got := MatchMove(r, cur, motor); got == nil || got.RejectionKind != domain.RejectBelowConfidence {
		t.Errorf("motor only: %+v", got)
	}
	if got := MatchMove(r, cur, cur); got != nil {
		t.Errorf("nothing moves: %+v", got)
	}
	if got := MatchMove(nil, cur, both); got != nil {
		t.Error("no search")
	}
}
