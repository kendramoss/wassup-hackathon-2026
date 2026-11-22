package tariffshift

import (
	"slices"
	"strings"
	"testing"
)

// The recorded example's hose clamps (N341003): the strip's top suggestion
// 7219.33 and its second 7220.20 both shift to heading 7326, so the answer
// is the same under either (D430).
func TestRobustSameUnderEitherCode(t *testing.T) {
	in := Input{GoodHTS: "7326.90.8688", Country: "Canada", Materials: []Material{
		{Name: "flat coiled stainless steel", HTS: "7219.33", Origin: "India"},
		{Name: "steel screws", HTS: "7318.15", Origin: "China"},
	}}
	rb := mustDefault(t).Robust(in, [][]string{{"7220.20"}, nil})
	if rb == nil || !rb.Stable || rb.Combinations != 2 || rb.Evaluated != 2 || rb.Sampled || len(rb.DependsOn) != 0 {
		t.Fatalf("%+v", rb)
	}
	if len(rb.Outcomes) != 1 || rb.Outcomes[0].Origin != "Canada" || rb.Outcomes[0].Count != 2 {
		t.Fatalf("%+v", rb.Outcomes)
	}
	if !strings.HasPrefix(rb.Summary, "the same answer for every suggested code") || !slices.Equal(rb.Parts, []string{"flat coiled stainless steel"}) {
		t.Fatalf("%q %v", rb.Summary, rb.Parts)
	}
}

// A part whose other code is already of the good's heading changes the
// answer; the other part does not.
func TestRobustDependsOnOnePart(t *testing.T) {
	in := Input{GoodHTS: "8473.30.5100", Country: "Mexico", Materials: []Material{
		{Name: "aluminum fins", HTS: "7616.99", Origin: "China"},
		{Name: "heat sink", HTS: "7616.99", Origin: "China"},
	}}
	rb := mustDefault(t).Robust(in, [][]string{{"7604.29", "7616.99"}, {"8473.30.11"}})
	if rb == nil || rb.Stable || rb.Combinations != 4 || rb.Evaluated != 4 {
		t.Fatalf("%+v", rb)
	}
	if !slices.Equal(rb.DependsOn, []string{"heat sink"}) || !strings.Contains(rb.Summary, "depends on the code for heat sink") {
		t.Fatalf("%v %q", rb.DependsOn, rb.Summary)
	}
	// The codes carried come first: the first outcome is the plain check's.
	if base := mustDefault(t).Evaluate(in); rb.Outcomes[0].Status != base.Status || rb.Outcomes[0].Origin != base.Origin {
		t.Fatalf("%+v vs %+v", rb.Outcomes[0], base)
	}
}

func TestRobustNilWithoutAlternatives(t *testing.T) {
	in := Input{GoodHTS: "7326.90", Country: "Canada", Materials: []Material{{Name: "strip", HTS: "7219.33", Origin: "India"}}}
	r := mustDefault(t)
	if rb := r.Robust(in, nil); rb != nil {
		t.Fatalf("%+v", rb)
	}
	// A repeat of the carried code is not an alternative.
	if rb := r.Robust(in, [][]string{{"7219 33", ""}}); rb != nil {
		t.Fatalf("%+v", rb)
	}
	// A part with no code takes no alternatives: the check needs its code.
	in.Materials[0].HTS = ""
	if rb := r.Robust(in, [][]string{{"7220.20"}}); rb != nil {
		t.Fatalf("%+v", rb)
	}
}

// Past MaxCombinations a deterministic sample is run: the codes carried and
// every single-part change are always in it, and it repeats.
func TestRobustSamples(t *testing.T) {
	in := Input{GoodHTS: "8473.30.5100", Country: "Mexico"}
	var alts [][]string
	for i := 0; i < 6; i++ {
		in.Materials = append(in.Materials, Material{Name: "part " + string(rune('a'+i)), HTS: "7616.99", Origin: "China"})
		alts = append(alts, []string{"7604.29", "7318.15"})
	}
	alts[5] = []string{"8473.30.11", "7318.15"}
	r := mustDefault(t)
	rb := r.Robust(in, alts)
	if rb == nil || !rb.Sampled || rb.Combinations != 729 || rb.Evaluated != MaxCombinations || rb.Stable {
		t.Fatalf("%+v", rb)
	}
	if !slices.Equal(rb.DependsOn, []string{"part f"}) || !strings.Contains(rb.Summary, "a sample of 81 of 729 combinations") {
		t.Fatalf("%v %q", rb.DependsOn, rb.Summary)
	}
	if again := r.Robust(in, alts); again.Summary != rb.Summary || !slices.Equal(again.Outcomes, rb.Outcomes) {
		t.Fatalf("not deterministic: %+v vs %+v", again, rb)
	}
}
