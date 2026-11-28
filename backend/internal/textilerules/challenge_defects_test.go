package textilerules

import "testing"

// A defect found on the development cases of the legal-rules challenge study
// (eval/challenges/legal-rules). It failed before the fix.

// Headings 6309 and 6310: the country of origin is where the good was last
// collected and packaged for shipment. The packing stage the facts carry
// ("packing") was never matched to the rule's packaging requirement, so the
// check always answered undetermined.
func TestPackagingRuleReadsThePackingStage(t *testing.T) {
	r := rules(t)
	f := Facts{GoodHTS: "6309000000", Stages: []Stage{
		{Process: ProcFabricMaking, Country: "India"}, {Process: ProcAssembly, Country: "India"}, {Process: StagePacking, Country: "Vietnam"}}}
	got := r.Evaluate(f)
	if got.Status != Decided || got.Origin != "Vietnam" || got.Para != "(c)(2)" {
		t.Fatalf("6309 packed in Vietnam: %+v, want decided Vietnam under (c)(2)", got)
	}
	f.Stages = append(f.Stages, Stage{Process: StagePacking, Country: "Canada"})
	if got := r.Evaluate(f); got.Status == Decided && got.Para == "(c)(2)" {
		t.Fatalf("packed in two countries must not be one packaging country: %+v", got)
	}
	f.Stages = []Stage{{Process: ProcAssembly, Country: "India"}}
	if got := r.Evaluate(f); got.Status != Undetermined {
		t.Fatalf("no packing stage stated: %+v, want undetermined", got)
	}
}

// Everything made in one country but the good last packed in another: the
// packaging rule names the packing country. The single-country shortcut must
// not take that answer away, even when the list of operations is complete.
func TestPackagingRuleBeatsTheSingleCountryShortcut(t *testing.T) {
	r := rules(t)
	f := Facts{GoodHTS: "6309000000", Complete: true, Stages: []Stage{
		{Process: ProcFabricMaking, Country: "India"}, {Process: ProcAssembly, Country: "India"}, {Process: StagePacking, Country: "Vietnam"}}}
	if got := r.Evaluate(f); got.Status != Decided || got.Origin != "Vietnam" {
		t.Fatalf("made in India, packed in Vietnam: %+v, want Vietnam", got)
	}
	f.Stages[2].Country = "India"
	if got := r.Evaluate(f); got.Status != Decided || got.Origin != "India" {
		t.Fatalf("everything in India: %+v, want India", got)
	}
	// Other headings keep the shortcut: packing is not a manufacturing operation.
	g := Facts{GoodHTS: "6203424011", Complete: true, Stages: []Stage{
		{Process: ProcFabricMaking, Country: "China"}, {Process: ProcAssembly, Country: "China"}, {Process: StagePacking, Country: "Vietnam"}}}
	if got := r.Evaluate(g); got.Status != Decided || got.Origin != "China" {
		t.Fatalf("trousers made in China, packed in Vietnam: %+v, want China", got)
	}
}
