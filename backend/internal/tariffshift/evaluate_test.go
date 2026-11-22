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
		"out of scope": {Input{GoodHTS: "6402.99", Country: "Mexico", Materials: []Material{{Name: "rubber sole", HTS: "4016.99", Origin: "China"}}}, OutOfScope, "not in the encoded chapters"},
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

// #550: a limitation that follows the last listed source with no comma
// ("from any other heading, or from subheading 8481.90 except when resulting
// from a simple assembly") limits that source only. A material from any
// other heading shifts whatever the facts.
func TestTrailingLimitationAttachesToTheLastSource(t *testing.T) {
	r := mustDefault(t)
	cases := []struct {
		name, good, country, mat string
		sa, gri                  *bool
		want                     string
	}{
		{"8481 fitting, no fact", "8481.80", "Mexico", "7307.99", nil, nil, MatShift},
		{"8481 fitting, simple assembly", "8481.80", "Mexico", "7307.99", b(true), nil, MatShift},
		{"8481 valve part, simple assembly", "8481.80", "Mexico", "8481.90", b(true), nil, MatNoShift},
		{"8481 valve part, not simple assembly", "8481.80", "Mexico", "8481.90", b(false), nil, MatShift},
		{"8481 valve part, no fact", "8481.80", "Mexico", "8481.90", nil, nil, MatNeedsFact},
		{"8716 frame, no fact", "8716.39", "Canada", "7308.90", nil, nil, MatShift},
		{"8716 frame, GRI 2(a)", "8716.39", "Canada", "7308.90", nil, b(true), MatShift},
		{"8716 trailer part, GRI 2(a)", "8716.39", "Canada", "8716.90", nil, b(true), MatNoShift},
		{"8716 trailer part, not GRI 2(a)", "8716.39", "Canada", "8716.90", nil, b(false), MatShift},
	}
	for _, c := range cases {
		res := r.Evaluate(Input{GoodHTS: c.good, Country: c.country, SimpleAssembly: c.sa, GRI2a: c.gri,
			Materials: []Material{{Name: "material", HTS: c.mat, Origin: "China"}}})
		if got := res.Materials[0].Status; got != c.want {
			t.Errorf("%s: material %s, want %s (%+v)", c.name, got, c.want, res)
		}
	}
	// With a value share above de minimis the old reading decided China;
	// the material shifts, so the good is the producing country's.
	for _, c := range []struct {
		good, country string
		in            Input
	}{
		{"8481.80", "Mexico", Input{SimpleAssembly: b(true)}},
		{"8716.39", "Canada", Input{GRI2a: b(true)}},
	} {
		mat := map[string]string{"8481.80": "7307.99", "8716.39": "7308.90"}[c.good]
		in := c.in
		in.GoodHTS, in.Country = c.good, c.country
		in.Materials = []Material{{Name: "material", HTS: mat, Origin: "China", ValueShare: f(0.40)}}
		res := r.Evaluate(in)
		if res.Status != Decided || res.Origin != c.country || !strings.Contains(res.Basis, "102.11(a)(3)") {
			t.Errorf("%s: %+v, want %s under 102.11(a)(3)", c.good, res, c.country)
		}
	}
	// The parse: a source condition, no whole-alternative flag.
	a := ParseAlternative("A change to subheading 8481.10 through 8481.80 from any other heading, or from subheading 8481.90 except when resulting from a simple assembly")
	if !a.Encoded || a.NotSimpleAssembly || len(a.Also) != 1 || a.Also[0].Condition != CondNotSimpleAssembly || a.From != 4 {
		t.Errorf("8481 parse = %+v", a)
	}
	a = ParseAlternative("A change to subheading 8716.10 through 8716.80 from any other heading, or from subheading 8716.90 except when that change is pursuant to General Rule of Interpretation 2(a)")
	if !a.Encoded || a.NotGRI2a || len(a.Also) != 1 || a.Also[0].Condition != CondNotGRI2a {
		t.Errorf("8716 parse = %+v", a)
	}
	// A limitation set off by a comma, or after the only source, still
	// limits the whole alternative.
	for _, s := range []string{
		"A change to subheading 8409.91 through 8409.99 from any other heading, except a change resulting from a simple assembly",
		"A change to subheading 8544.11 through 8544.70 from any other subheading, including another subheading within that group, except when resulting from simple assembly",
	} {
		if a := ParseAlternative(s); !a.Encoded || !a.NotSimpleAssembly {
			t.Errorf("whole-alternative limitation lost: %+v", a)
		}
	}
	if a := ParseAlternative("A change to subheading 7315.11 through 7315.12 from subheading 7315.19 or 7315.90, except when that change is pursuant to General Rule of Interpretation 2(a)"); !a.Encoded || !a.NotGRI2a {
		t.Errorf("7315 = %+v", a)
	}
}

// #551: each "from ..." of an exception list is its own exception, with or
// without the comma, so a code-only exception decides.
func TestExceptionListsSplitWithoutAComma(t *testing.T) {
	r := mustDefault(t)
	for _, c := range []struct{ good, mat, want string }{
		{"9002.11", "9001.90", MatNoShift},
		{"9030.89", "9030.90", MatNoShift},
		{"9030.90", "9030.89", MatNoShift},
		{"8485.10", "8486.20", MatNoShift},
		{"8485.30", "8475.21", MatNoShift},
		{"8485.30", "8486.30", MatNoShift},
		{"8485.80", "8486.40", MatNoShift},
		{"9030.32", "9030.82", MatNoShift},   // control: the comma form
		{"9002.11", "7014.00", MatNeedsFact}, // lens blanks of 7014: described
		{"9030.89", "9030.20", MatNeedsFact}, // non-CRT oscilloscopes of 9030.20: described
		{"8485.10", "8456.90", MatNeedsFact}, // water-jet cutting machines of 8456.90: described
	} {
		res := r.Evaluate(Input{GoodHTS: c.good, Country: "Mexico", Materials: []Material{{Name: "material", HTS: c.mat, Origin: "China"}}})
		if got := res.Materials[0].Status; got != c.want {
			t.Errorf("good %s, material %s: %s, want %s (%+v)", c.good, c.mat, got, c.want, res.Materials[0])
		}
	}
	a := ParseAlternative("A change to subheading 9030.89 from any other subheading, except from non-cathode ray tube oscilloscopes or oscillographs of subheading 9030.20 or from subheading 9030.90")
	if len(a.Except) != 2 || !a.Except[0].Described || a.Except[1].Described || a.Except[1].From[0].Lo != "903090" {
		t.Errorf("9030.89 except = %+v", a.Except)
	}
}
