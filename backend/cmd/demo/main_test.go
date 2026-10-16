package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestMovedCopiesAndMovesGroups(t *testing.T) {
	p := map[string]any{"components": []any{
		map[string]any{"name": "motor", "group": "motor_fan_assembly", "plant": "cn", "made_in": "China"},
		map[string]any{"name": "switch", "group": "switch", "plant": "cn", "made_in": "China"},
	}}
	m := moved(p, []string{"motor_fan_assembly"})
	got := m["components"].([]any)
	if got[0].(map[string]any)["plant"] != "th" || got[1].(map[string]any)["plant"] != "cn" {
		t.Errorf("moved = %v", got)
	}
	if p["components"].([]any)[0].(map[string]any)["plant"] != "cn" {
		t.Error("original product was modified")
	}
}

func TestBestOfAndVerdicts(t *testing.T) {
	shown := planResult{Plans: []plan{{Description: "A", Shown: true, Net: 320000, Verdict: &verdict{Confidence: 0.93}}}}
	if b, s := bestOf(shown); s != "shown" || b != "A (check 0.93, net $320000/yr)" {
		t.Errorf("bestOf shown = %q %q", b, s)
	}
	below := planResult{Rejected: []plan{{Description: "B", Kind: "below_confidence_bar", Verdict: &verdict{Confidence: 0.8}}}}
	if _, s := bestOf(below); s != "below_bar" {
		t.Errorf("bestOf below = %q", s)
	}
	if _, s := bestOf(planResult{}); s != "no_plan" {
		t.Errorf("bestOf none = %q", s)
	}
	if v := verdicts(below); v["B"] != "below_confidence_bar 0.80" {
		t.Errorf("verdicts = %v", v)
	}
}

func TestClosestMoveIsTheHighestCheck(t *testing.T) {
	b, err := os.ReadFile("../../../eval/results/demo-seed-heatgun-plan-vertex.json")
	if err != nil {
		t.Skip("recorded Vertex seed not present:", err)
	}
	var resp struct {
		Plan planResult `json:"plan"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatal(err)
	}
	best, status := bestOf(resp.Plan)
	if status != "below_bar" || !strings.HasPrefix(best, "Make Motor-fan assembly and PCBA (SMT) in Thailand (check 0.90") {
		t.Errorf("closest = %q (%s); the Plan screen shows motor-fan + PCBA at 0.90", best, status)
	}
}

func TestClosestMoveTieKeepsListOrder(t *testing.T) {
	v := func(c float64) *verdict { return &verdict{Confidence: c} }
	p := planResult{Rejected: []plan{
		{Description: "a", Kind: "below_confidence_bar", Verdict: v(0.85)},
		{Description: "b", Kind: "no_supporting_precedent", Verdict: v(0.95)},
		{Description: "c", Kind: "below_confidence_bar", Verdict: v(0.85)},
	}}
	if x := closestMove(p); x == nil || x.Description != "a" {
		t.Errorf("closest = %+v", x)
	}
}
