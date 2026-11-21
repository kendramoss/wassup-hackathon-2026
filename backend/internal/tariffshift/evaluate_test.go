package tariffshift

import (
	"slices"
	"strings"
	"testing"
)

func f(v float64) *float64 { return &v }
func b(v bool) *bool       { return &v }

// N337260 (26 Jan 2024): thermal modules of 8473.30 assembled in Mexico from
// Chinese fins, heat pipe, base, plate and fasteners, all classified outside
// heading 8473. CBP: the shift is met, so Mexico. The component headings are
// illustrative (the ruling says only "outside heading 8473").
func TestThermalModuleN337260(t *testing.T) {
	in := Input{GoodHTS: "8473.30.5100", Country: "Mexico", Materials: []Material{
		{Name: "aluminum fins", HTS: "7616.99", Origin: "China"},
		{Name: "copper heat pipe", HTS: "7411.10", Origin: "China"},
		{Name: "aluminum base", HTS: "7616.99", Origin: "China"},
		{Name: "fasteners", HTS: "7318.15", Origin: "China"},
	}}
	res := mustDefault(t).Evaluate(in)
	if res.Status != Decided || res.Origin != "Mexico" || !strings.Contains(res.Basis, "102.11(a)(3)") || res.Rule.ID != "19 CFR 102.20(o), 8473" {
		t.Fatalf("%+v", res)
	}
	for _, m := range res.Materials {
		if m.Status != MatShift || m.Alternative != 1 || !m.Foreign {
			t.Fatalf("%+v", m)
		}
	}

	// A Chinese part already of heading 8473 does not shift; it is then the
	// only material classified where the rule allows no change, so it gives
	// the origin under 102.11(b) and 102.18(b)(1)(iii).
	in.Materials = append(in.Materials, Material{Name: "finished heat sink", HTS: "8473.30.11", Origin: "China"})
	res = mustDefault(t).Evaluate(in)
	if res.Status != Undetermined || res.IfNotDeMinimis == nil || res.IfNotDeMinimis.Origin != "China" {
		t.Fatalf("value unknown: %+v", res)
	}
	in.Materials[4].ValueShare = f(0.6)
	res = mustDefault(t).Evaluate(in)
	if res.Status != Decided || res.Origin != "China" || !strings.Contains(res.Basis, "102.18(b)(1)(iii)") {
		t.Fatalf("%+v", res)
	}
	// Worth 5% of the good, it is disregarded (102.13).
	in.Materials[4].ValueShare = f(0.05)
	res = mustDefault(t).Evaluate(in)
	if res.Status != Decided || res.Origin != "Mexico" || !strings.Contains(res.Basis, "102.13") {
		t.Fatalf("%+v", res)
	}
}

// 8504.40 power supply from a PCBA of 8504.90: "from any other subheading
// outside that group", and 8504.90 is outside 8504.10-8504.50.
func TestPowerSupply(t *testing.T) {
	res := mustDefault(t).Evaluate(Input{GoodHTS: "8504.40.95", Country: "Vietnam", Materials: []Material{
		{Name: "printed circuit assembly", HTS: "8504.90.75", Origin: "China"},
		{Name: "plastic housing", HTS: "3926.90", Origin: "Vietnam"},
	}})
	if res.Status != Decided || res.Origin != "Vietnam" {
		t.Fatalf("%+v", res)
	}
	if res.Materials[1].Foreign {
		t.Fatal("the housing is domestic")
	}
	// A transformer of 8504.31 is inside the group: no shift.
	res = mustDefault(t).Evaluate(Input{GoodHTS: "8504.40", Country: "Vietnam", Materials: []Material{
		{Name: "transformer", HTS: "8504.31", Origin: "China", ValueShare: f(0.3)},
		{Name: "housing", HTS: "3926.90", Origin: "China"},
	}})
	if res.Status != Decided || res.Origin != "China" || res.Materials[0].Status != MatNoShift || res.Materials[1].Status != MatShift {
		t.Fatalf("%+v", res)
	}
}

// 8544.42 cable assemblies: every alternative is "except when resulting from
// simple assembly", so the answer turns on that fact.
func TestCableNeedsSimpleAssemblyFact(t *testing.T) {
	in := Input{GoodHTS: "8544.42.90", Country: "Mexico", Materials: []Material{
		{Name: "insulated wire", HTS: "8544.49", Origin: "China"},
		{Name: "connectors", HTS: "8536.69", Origin: "China"},
	}}
	res := mustDefault(t).Evaluate(in)
	if res.Status != Undetermined || res.Materials[0].Status != MatNeedsFact || !slices.Contains(res.Materials[0].Needs, needSimple) {
		t.Fatalf("%+v", res)
	}
	in.SimpleAssembly = b(false)
	if res = mustDefault(t).Evaluate(in); res.Status != Decided || res.Origin != "Mexico" {
		t.Fatalf("%+v", res)
	}
	in.SimpleAssembly = b(true)
	if res = mustDefault(t).Evaluate(in); res.Status == Decided && res.Origin == "Mexico" {
		t.Fatalf("simple assembly cannot confer origin here: %+v", res)
	}
}

func TestBendingCondition(t *testing.T) {
	in := Input{GoodHTS: "8402.90", Country: "Canada", Materials: []Material{{Name: "steel tube", HTS: "7304.31", Origin: "Japan"}}}
	if res := mustDefault(t).Evaluate(in); res.Status != Undetermined || res.Materials[0].Needs[0] != needBending {
		t.Fatalf("%+v", res)
	}
	in.BendingToShape = b(true)
	if res := mustDefault(t).Evaluate(in); res.Status != Decided || res.Origin != "Canada" {
		t.Fatalf("%+v", res)
	}
}

func TestUndeterminedAndScope(t *testing.T) {
	r := mustDefault(t)
	for name, c := range map[string]struct {
		in     Input
		status string
		reason string
	}{
		"out of scope": {Input{GoodHTS: "3926.90", Country: "Mexico", Materials: []Material{{Name: "resin", HTS: "3901.10", Origin: "China"}}}, OutOfScope, "not in the encoded chapters"},
		"no code":      {Input{Country: "Mexico", Materials: []Material{{Name: "x", HTS: "7616", Origin: "China"}}}, Undetermined, "missing"},
		"short code":   {Input{GoodHTS: "8504", Country: "Mexico", Materials: []Material{{Name: "x", HTS: "7616", Origin: "China"}}}, Undetermined, "too short"},
		"no materials": {Input{GoodHTS: "8501.10", Country: "Mexico"}, Undetermined, "no materials"},
		"material without code": {Input{GoodHTS: "8501.10", Country: "Mexico", Materials: []Material{{Name: "stator", Origin: "China"}}},
			Undetermined, "cannot be decided for stator"},
		"two failing materials from two countries": {Input{GoodHTS: "8501.10", Country: "Mexico", Materials: []Material{
			{Name: "motor A", HTS: "8501.10", Origin: "China", ValueShare: f(0.4)},
			{Name: "motor B", HTS: "8501.10", Origin: "Japan", ValueShare: f(0.4)},
		}}, Undetermined, "must be judged"},
	} {
		res := r.Evaluate(c.in)
		if res.Status != c.status || !strings.Contains(res.Reason, c.reason) {
			t.Errorf("%s: %+v", name, res)
		}
	}
	// Every material from the country of production: 102.11(a)(2).
	res := r.Evaluate(Input{GoodHTS: "8501.10", Country: "Mexico", Materials: []Material{{Name: "stator", Origin: "mexico"}}})
	if res.Status != Decided || res.Origin != "Mexico" || res.Basis != "19 CFR 102.11(a)(2)" {
		t.Fatalf("%+v", res)
	}
}

// An alternative written by goods description is never decided by code.
func TestDescribedAlternativeNeedsPerson(t *testing.T) {
	res := mustDefault(t).Evaluate(Input{GoodHTS: "8528.42", Country: "Mexico", Materials: []Material{{Name: "panel", HTS: "8524.11", Origin: "China"}}})
	if res.Status != Undetermined || !slices.Contains(res.Materials[0].Needs, needPerson) {
		t.Fatalf("%+v", res)
	}
}

// CBP's own wording in N337260: the components "are each classified outside
// of heading 8473". That settles "from any other heading", but not the
// simple-assembly exception for 8414, 8501, 8504, 8534, 8541 and 8542, which
// a component outside 8473 could still be in.
func TestNotInHeading(t *testing.T) {
	in := Input{GoodHTS: "8473.30.5100", Country: "Mexico", Materials: []Material{
		{Name: "components", NotIn: "8473", Origin: "China"},
	}}
	res := mustDefault(t).Evaluate(in)
	if res.Status != Undetermined || !slices.Contains(res.Materials[0].Needs, needSimple) {
		t.Fatalf("%+v", res)
	}
	in.SimpleAssembly = b(false)
	if res = mustDefault(t).Evaluate(in); res.Status != Decided || res.Origin != "Mexico" {
		t.Fatalf("%+v", res)
	}
	// Outside a subheading says nothing about the heading.
	in.Materials[0].NotIn = "8473.30"
	if res = mustDefault(t).Evaluate(in); res.Status != Undetermined {
		t.Fatalf("%+v", res)
	}
}

// A material whose origin is not given is fine when it shifts, and leaves
// the answer open when it does not.
func TestUnknownOrigin(t *testing.T) {
	in := Input{GoodHTS: "8501.10", Country: "Mexico", Materials: []Material{
		{Name: "steel laminations", HTS: "7326.90", Origin: ""},
		{Name: "copper wire", HTS: "7408.11", Origin: "Chile"},
	}}
	if res := mustDefault(t).Evaluate(in); res.Status != Decided || res.Origin != "Mexico" {
		t.Fatalf("%+v", res)
	}
	in.Materials[0].HTS = "8501.10"
	if res := mustDefault(t).Evaluate(in); res.Status != Undetermined || !strings.Contains(res.Reason, "origin is not given") {
		t.Fatalf("%+v", res)
	}
}
