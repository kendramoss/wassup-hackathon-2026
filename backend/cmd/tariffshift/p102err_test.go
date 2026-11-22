package main

import "testing"

// D590: how a row's engine answer is compared with the rule.
func TestP102RowAgreement(t *testing.T) {
	for _, tt := range []struct {
		name                string
		r                   p102Row
		agree, decided, has bool
	}{
		{"move the rule says shifts, engine predicts the target", p102Row{Kind: "move", Target: "Canada", Status: "shifts", Origin: "Canada", Engine: "Canada"}, true, true, true},
		{"move the rule says shifts, engine predicts another country", p102Row{Kind: "move", Target: "Canada", Status: "shifts", Origin: "Canada", Engine: "China"}, false, true, true},
		{"move finished elsewhere, engine predicts the target", p102Row{Kind: "move", Target: "China", Status: "other_country", Origin: "Mexico", Engine: "China"}, false, true, true},
		{"move finished elsewhere, engine names a third country", p102Row{Kind: "move", Target: "China", Status: "other_country", Origin: "Mexico", Engine: "India"}, true, true, true},
		{"lean away from the target", p102Row{Kind: "move", Target: "Canada", Status: "undetermined", Lean: "China", Engine: "China"}, true, false, true},
		{"lean to the target", p102Row{Kind: "move", Target: "Japan", Status: "undetermined", Lean: "Japan", Engine: "Japan"}, true, false, true},
		{"current setup decided", p102Row{Kind: "baseline", Status: "other_country", Origin: "Mexico", Engine: "South Korea"}, false, true, true},
		{"failed call counts against", p102Row{Kind: "move", Target: "Canada", Status: "shifts", Error: "timeout"}, false, true, true},
		{"undetermined, no lean: not compared", p102Row{Kind: "move", Target: "Canada", Status: "undetermined", Engine: "China"}, false, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a, d, ok := tt.r.agrees()
			if a != tt.agree || d != tt.decided || ok != tt.has {
				t.Fatalf("agrees() = %v %v %v, want %v %v %v", a, d, ok, tt.agree, tt.decided, tt.has)
			}
		})
	}
}
