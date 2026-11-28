package tariffwatch

import "testing"

// A defect found on the development cases of the legal-rules challenge study
// (eval/challenges/legal-rules). It failed before the fix.

// Validate accepts a dated action rate up to 500 percent, but the additional
// duty index the evaluation builds from the same action refused any rate above
// 100 percent, so a data set that loaded (or that Revise produced from a
// heading reading "+ 120%") could not be evaluated.
func TestDatedActionAbove100PercentIsEvaluated(t *testing.T) {
	for _, rate := range []float64{1.2, 1.5} {
		ds := embedded(t)
		ds.Actions = []Action{{Kind: KindSection301, Heading: "9903.91.12", Name: "synthetic", Origin: "China", Rate: rate,
			EffectiveFrom: "2027-01-10", Subheadings: []string{"8716.90.30"}}}
		if err := ds.Validate(); err != nil {
			t.Fatalf("rate %v: %v", rate, err)
		}
		ev, err := NewEvaluator(ds)
		if err != nil {
			t.Fatal(err)
		}
		mfn := 0.0
		ob, err := ev.Evaluate("8716.90.30", "China", &mfn, day("2027-02-01"))
		if err != nil {
			t.Fatalf("rate %v: %v", rate, err)
		}
		var got *Component
		for i := range ob.Components {
			if ob.Components[i].Heading == "9903.91.12" {
				got = &ob.Components[i]
			}
		}
		if got == nil || got.Rate != rate {
			t.Fatalf("rate %v: component %+v, want the action's own rate", rate, got)
		}
		sum := 0.0
		for _, c := range ob.Components {
			sum += c.Rate
		}
		if d := ob.Rate - sum; d > 1e-9 || d < -1e-9 {
			t.Fatalf("rate %v: total %v is not the sum of the components %v", rate, ob.Rate, sum)
		}
		before, err := ev.Evaluate("8716.90.30", "China", &mfn, day("2027-01-09"))
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range before.Components {
			if c.Heading == "9903.91.12" {
				t.Fatalf("the action is not in force on 2027-01-09: %+v", c)
			}
		}
	}
}
