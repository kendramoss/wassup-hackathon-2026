package tariffshift

import (
	"math"
	"strings"
	"testing"
)

// Issue #711: a share outside 0..1 (or non-finite) is invalid input. Evaluate
// must not turn it into a de minimis decision.
func TestEvaluateRejectsOutOfRangeShares(t *testing.T) {
	rules, err := CanadaRules()
	if err != nil {
		t.Fatal(err)
	}
	// 8501.10 from China into an 8501.10 good: does not shift, so only de
	// minimis could rescue it.
	motor := func(vs, ws *float64) Input {
		return Input{GoodHTS: "8501.10", Country: "Mexico", Materials: []Material{
			{Name: "motor", HTS: "8501.10", Origin: "China", ValueShare: vs, WeightShare: ws}}}
	}
	for name, in := range map[string]Input{
		"value -1":            motor(f(-1), nil),
		"value 1.5":           motor(f(1.5), nil),
		"value NaN":           motor(f(math.NaN()), nil),
		"value +Inf":          motor(f(math.Inf(1)), nil),
		"value -Inf":          motor(f(math.Inf(-1)), nil),
		"weight -1":           motor(nil, f(-1)),
		"weight 1.5":          motor(nil, f(1.5)),
		"weight NaN":          motor(nil, f(math.NaN())),
		"weight +Inf":         motor(nil, f(math.Inf(1))),
		"value ok, weight -1": motor(f(0.05), f(-1)),
	} {
		res := rules.Evaluate(in)
		if res.Status != Undetermined || res.Origin != "" || res.Basis != "" {
			t.Errorf("%s: status %q origin %q basis %q (%s)", name, res.Status, res.Origin, res.Basis, res.Reason)
		}
	}
	// Chapter 63 is tested by weight: a negative weight share must not pass.
	blanket := Input{GoodHTS: "6301.30", Country: "Mexico", Materials: []Material{
		{Name: "woven cotton", HTS: "5208.11", Origin: "China", WeightShare: f(-1)}}}
	if res := rules.Evaluate(blanket); res.Status != Undetermined {
		t.Errorf("chapter 63 weight -1: %q (%s)", res.Status, res.Reason)
	}
	// An invalid share on a material that shifts is still invalid input.
	shifting := Input{GoodHTS: "6301.30", Country: "Mexico", Materials: []Material{
		{Name: "cotton yarn", HTS: "5205.11", Origin: "China", ValueShare: f(-1)}}}
	if res := rules.Evaluate(shifting); res.Status != Undetermined {
		t.Errorf("shifting material, share -1: %q", res.Status)
	}
}

func TestEvaluateAcceptsBoundaryShares(t *testing.T) {
	rules, err := CanadaRules()
	if err != nil {
		t.Fatal(err)
	}
	motor := func(vs *float64) Input {
		return Input{GoodHTS: "8501.10", Country: "Mexico", Materials: []Material{
			{Name: "motor", HTS: "8501.10", Origin: "China", ValueShare: vs}}}
	}
	// 0.5 and 1 exceed the 7% limit: decided, origin China (no de minimis).
	for _, v := range []*float64{f(0.5), f(1)} {
		if res := rules.Evaluate(motor(v)); res.Status != Decided || res.Origin != "China" {
			t.Errorf("share %v: %q %q (%s)", *v, res.Status, res.Origin, res.Reason)
		}
	}
	// 0 and 0.05 are within the limit: de minimis, origin Mexico.
	for _, v := range []*float64{f(0), f(0.05)} {
		if res := rules.Evaluate(motor(v)); res.Status != Decided || res.Origin != "Mexico" {
			t.Errorf("share %v: %q %q (%s)", *v, res.Status, res.Origin, res.Reason)
		}
	}
	// nil stays unknown, not invalid: the existing open-de-minimis path.
	if res := rules.Evaluate(motor(nil)); res.Status == Decided && res.Origin == "Mexico" {
		t.Errorf("nil share decided by de minimis: %q", res.Origin)
	}
	// No sum-of-shares restriction: shares that together exceed 1 are not rejected here.
	two := Input{GoodHTS: "8501.10", Country: "Mexico", Materials: []Material{
		{Name: "a", HTS: "8501.10", Origin: "China", ValueShare: f(0.8)},
		{Name: "b", HTS: "8501.10", Origin: "Japan", ValueShare: f(0.8)}}}
	if res := rules.Evaluate(two); res.Reason != "" && strings.Contains(res.Reason, "_share") {
		t.Errorf("sum over 1 rejected: %s", res.Reason)
	}
}
