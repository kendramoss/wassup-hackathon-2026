package textilerules

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func rules(t *testing.T) *Rules {
	t.Helper()
	r, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// The committed rules.json is what Build makes from the committed source.
func TestRulesJSONIsBuiltFromTheSource(t *testing.T) {
	ecfr, fr, err := SourceFiles()
	if err != nil {
		t.Fatal(err)
	}
	want, err := Build(ecfr, fr, SourceInfo)
	if err != nil {
		t.Fatal(err)
	}
	r := rules(t)
	if !reflect.DeepEqual(want, r.T) {
		a, _ := json.Marshal(want)
		b, _ := json.Marshal(r.T)
		t.Fatalf("rules.json is stale: run go run ./cmd/textilerules rules\nbuilt  %d bytes\ncommit %d bytes", len(a), len(b))
	}
}

// Counts that pin the encoding to the regulation: 28 entries in (b)(5), 16
// codes in (e)(2), and the 6201-6208 entry added from the Federal Register.
func TestEncodingCounts(t *testing.T) {
	r := rules(t)
	if len(r.T.Scope) != 28 {
		t.Errorf("(b)(5) entries = %d, want 28", len(r.T.Scope))
	}
	if len(r.T.E2.Codes) != 16 {
		t.Errorf("(e)(2) codes = %d, want 16", len(r.T.E2.Codes))
	}
	var fr *Row
	for i, row := range r.T.Rows {
		if row.Origin != "eCFR" {
			fr = &r.T.Rows[i]
		}
	}
	if fr == nil || fr.Code != "6201-6208" || len(fr.Alts) != 2 {
		t.Fatalf("the 6201-6208 entry of 87 FR 68356 is missing or wrong: %+v", fr)
	}
	// The eCFR has no 6201-6208 row (its editorial note says so).
	for _, row := range r.T.Rows {
		if row.Code == "6201-6208" && row.Origin == "eCFR" {
			t.Error("the eCFR does not carry 6201-6208")
		}
	}
	// Rows are in code order and overlap only when one lies inside the
	// other (9619.00.31-33 inside 9619).
	for i := 1; i < len(r.T.Rows); i++ {
		a, b := r.T.Rows[i-1], r.T.Rows[i]
		if b.Lo < a.Lo {
			t.Errorf("rows %s and %s are out of order", a.Code, b.Code)
		}
		if b.Lo <= a.Hi && b.Hi > a.Hi {
			t.Errorf("rows %s and %s overlap without nesting", a.Code, b.Code)
		}
	}
}

func row(t *testing.T, r *Rules, code string) Row {
	t.Helper()
	for _, x := range r.T.Rows {
		if x.Code == code {
			return x
		}
	}
	t.Fatalf("no row %s", code)
	return Row{}
}

func alt(t *testing.T, x Row, id string) Alt {
	t.Helper()
	for _, a := range x.Alts {
		if a.ID == id {
			return a
		}
	}
	t.Fatalf("row %s has no alternative %s: %+v", x.Code, id, x.Alts)
	return Alt{}
}

// Rows read from the text of (e)(1).
func TestRowsFromTheText(t *testing.T) {
	r := rules(t)

	// 6101-6117 (1): not knit to shape, two or more parts: wholly assembled.
	a := alt(t, row(t, r, "6101-6117"), "1")
	want := []Cond{{Fact: FactNotE2, Want: "yes"}, {Fact: FactKnitToShape, Want: "no"}, {Fact: FactComponents, Want: "yes"}}
	if !reflect.DeepEqual(a.Conds, want) || a.Req.Process != ProcAssembly {
		t.Errorf("6101-6117 (1) = %+v", a)
	}
	// (2): one part: fabric-making, with the listed fabric headings excepted.
	a = alt(t, row(t, r, "6101-6117"), "2")
	if a.Req.Process != ProcFabricMaking || a.Req.Shift == nil || a.Req.Shift.From != "any heading outside that group" {
		t.Errorf("6101-6117 (2) = %+v", a)
	}
	if !contains(a.Req.Shift.Except, "5208-5212") || !contains(a.Req.Shift.Except, "6001-6006") || !contains(a.Req.Shift.Except, "6307.90") {
		t.Errorf("6101-6117 (2) exceptions = %v", a.Req.Shift.Except)
	}
	// (3): knit to shape.
	if a = alt(t, row(t, r, "6101-6117"), "3"); a.Req.Process != ProcKnitToShape {
		t.Errorf("6101-6117 (3) = %+v", a)
	}

	// 5004-5006: staple fibres are spun, filaments are extruded.
	x := row(t, r, "5004-5006")
	if a = alt(t, x, "1"); a.Req.Process != ProcSpinning || a.Conds[0].Want != "staple" {
		t.Errorf("5004-5006 (1) = %+v", a)
	}
	if a = alt(t, x, "2"); a.Req.Process != ProcExtrusion || a.Conds[0].Want != "filaments" {
		t.Errorf("5004-5006 (2) = %+v", a)
	}

	// 5208-5212: dyeing and printing with finishing, and if that does not
	// decide, fabric-making.
	x = row(t, r, "5208-5212")
	if a = alt(t, x, "1"); a.Req.Process != ProcDyePrint {
		t.Errorf("5208-5212 (1) = %+v", a)
	}
	if a = alt(t, x, "2"); a.After != "1" || a.Req.Process != ProcFabricMaking {
		t.Errorf("5208-5212 (2) = %+v", a)
	}

	// 5609 reads the extrusion or spinning country.
	x = row(t, r, "5609")
	if a = alt(t, x, "1"); a.Req.Process != ProcExtrusion {
		t.Errorf("5609 (1) = %+v", a)
	}
	// 6309-6310: last collected and packaged.
	if a = alt(t, row(t, r, "6309-6310"), "1"); a.Req.Process != ProcPackaging {
		t.Errorf("6309-6310 = %+v", a)
	}
	// 5810.10: where the embroidery was performed.
	if a = alt(t, row(t, r, "5810.10"), "1"); a.Req.Process != ProcEmbroidery {
		t.Errorf("5810.10 = %+v", a)
	}
	// 6301-6306 gives way to (e)(2) for the goods it names.
	if a = alt(t, row(t, r, "6301-6306"), "1"); a.Conds[0].Fact != FactNotE2 || a.Req.Process != ProcFabricMaking {
		t.Errorf("6301-6306 = %+v", a)
	}
	// A range with a suffix: 4202.12.40-89.
	x = row(t, r, "4202.12.40-4202.12.89")
	if x.Lo != "4202124000" || x.Hi != "4202128999" {
		t.Errorf("4202.12.40-4202.12.89 = %s..%s", x.Lo, x.Hi)
	}
	// 5904 (2) sends the good to (c)(4).
	if a = alt(t, row(t, r, "5904"), "2"); a.Req.Kind != ReqToC4 {
		t.Errorf("5904 (2) = %+v", a)
	}
	// A rule worded as a description of goods is not read by code.
	if a = alt(t, row(t, r, "5601"), "1"); !hasUnmodeled(a) {
		t.Errorf("5601 (1) should have a description condition: %+v", a)
	}
	// 9619 (7): only a baby diaper of cotton.
	if a = alt(t, row(t, r, "9619"), "7"); !hasUnmodeled(a) {
		t.Errorf("9619 (7) should carry the goods description: %+v", a)
	}
}

func contains(s []string, x string) bool {
	for _, y := range s {
		if y == x {
			return true
		}
	}
	return false
}

func hasUnmodeled(a Alt) bool {
	for _, c := range a.Conds {
		if c.Unmodeled != "" {
			return true
		}
	}
	return false
}

// (e)(2): the sixteen codes, and its four alternatives are the text's words.
func TestE2FromTheText(t *testing.T) {
	r := rules(t)
	for _, c := range []string{"6213", "6214", "6117.10", "6302.22", "9404.90.85", "9404.90.95", "6304.99"} {
		if !contains(r.T.E2.Codes, c) {
			t.Errorf("(e)(2) codes lack %s: %v", c, r.T.E2.Codes)
		}
	}
	for _, a := range r.T.E2.Alts {
		if !strings.Contains(r.T.E2.Text, a.Text) {
			t.Errorf("(e)(2) %s is not the regulation's words", a.ID)
		}
	}
}

func st(p, country string) Stage { return Stage{Process: p, Country: country} }
func fin(country string, ops ...string) []Stage {
	var out []Stage
	for _, o := range ops {
		out = append(out, Stage{Process: StageFinish, Op: o, Country: country})
	}
	return out
}

func decided(t *testing.T, res Result, origin, para string) {
	t.Helper()
	if res.Status != Decided || res.Origin != origin || res.Para != para {
		t.Errorf("got %+v, want %s under %s", res, origin, para)
	}
}

func TestEvaluate(t *testing.T) {
	r := rules(t)

	// 6104 (two parts, not knit to shape): the country where the good was
	// wholly assembled, 6101-6117 (1) and (c)(2).
	res := r.Evaluate(Facts{GoodHTS: "6104.43.2020", KnitToShape: No, TwoOrMoreComponents: Yes, Complete: true,
		Stages: []Stage{st(ProcFabricMaking, "China"), st(StageCutting, "China"), st(ProcAssembly, "Vietnam")}})
	decided(t, res, "Vietnam", "(c)(2)")
	if res.Row != "6101-6117" || res.Alt != "1" {
		t.Errorf("row/alt = %s/%s", res.Row, res.Alt)
	}

	// 6117.80 scarf of one piece of knit fabric: the fabric-making country.
	res = r.Evaluate(Facts{GoodHTS: "6117.80.95", KnitToShape: No, TwoOrMoreComponents: No, CottonWool16: No, Complete: true,
		Stages: []Stage{st(ProcFabricMaking, "China"), st(StageCutting, "Vietnam"), st(ProcAssembly, "Vietnam")}})
	decided(t, res, "China", "(c)(2)")

	// 6116 gloves knit to shape: where they were knit, 6101-6117 (3).
	res = r.Evaluate(Facts{GoodHTS: "6116.10.55", KnitToShape: Yes, CottonWool16: No, Complete: true,
		Stages: []Stage{st(ProcKnitToShape, "Vietnam"), st(ProcAssembly, "China")}})
	decided(t, res, "Vietnam", "(c)(2)")

	// 5208 fabric dyed, printed and finished in China from greige fabric woven
	// in India: 5208-5212 (1).
	f := Facts{GoodHTS: "5208.52.30", FabricOfWool: No, Complete: true,
		Stages: append([]Stage{st(ProcFabricMaking, "India"), st(StageDyeing, "China"), st(StagePrint, "China")}, fin("China", "bleaching", "shrinking")...)}
	decided(t, r.Evaluate(f), "China", "(c)(2)")
	// With one finishing operation, (1) fails and (2) names the fabric-making country.
	f.Stages = append([]Stage{st(ProcFabricMaking, "India"), st(StageDyeing, "China"), st(StagePrint, "China")}, fin("China", "bleaching")...)
	res = r.Evaluate(f)
	decided(t, res, "India", "(c)(2)")
	if res.Alt != "2" {
		t.Errorf("alt = %s, want 2", res.Alt)
	}

	// 6302.22 sheets of man-made fibre: (e)(2) applies; dyed and printed with
	// two finishing operations in Vietnam.
	f = Facts{GoodHTS: "6302.22.20", CottonWool16: No, Complete: true,
		Stages: append([]Stage{st(ProcFabricMaking, "China"), st(StageDyeing, "Vietnam"), st(StagePrint, "Vietnam"), st(ProcAssembly, "India")}, fin("Vietnam", "napping", "decating")...)}
	res = r.Evaluate(f)
	decided(t, res, "Vietnam", "(c)(2)")
	if res.Row != "(e)(2)" {
		t.Errorf("row = %s, want (e)(2)", res.Row)
	}
	// Of cotton, (e)(2) does not apply: the fabric-making country, 6301-6306.
	f.CottonWool16 = Yes
	res = r.Evaluate(f)
	decided(t, res, "China", "(c)(2)")
	if res.Row != "6301-6306" {
		t.Errorf("row = %s, want 6301-6306", res.Row)
	}
	// Fibre content not stated: undetermined, not a guess.
	f.CottonWool16 = Unknown
	if res = r.Evaluate(f); res.Status != Undetermined || res.Class != ClassFact {
		t.Errorf("got %+v", res)
	}

	// 6307.90 face mask: where the fabric was formed.
	decided(t, r.Evaluate(Facts{GoodHTS: "6307.90.98", Complete: true,
		Stages: []Stage{st(ProcFabricMaking, "China"), st(ProcAssembly, "Vietnam")}}), "China", "(c)(2)")

	// 6103 suit assembled in two countries: (c)(2) and (c)(3) give no country,
	// and (c)(4) is a judgment; the last important operation is only shown.
	res = r.Evaluate(Facts{GoodHTS: "6103.23.0055", KnitToShape: No, TwoOrMoreComponents: Yes, Complete: true,
		Stages: []Stage{st(ProcFabricMaking, "China"), st(ProcAssembly, "Vietnam"), st(ProcAssembly, "Cambodia")}})
	if res.Status != Undetermined || res.Class != ClassJudgment || res.Candidate != "Cambodia" {
		t.Errorf("got %+v", res)
	}

	// Everything in one country: that country.
	decided(t, r.Evaluate(Facts{GoodHTS: "6204.62.4031", TwoOrMoreComponents: Yes, Complete: true,
		Stages: []Stage{st(ProcFabricMaking, "Pakistan"), st(StageCutting, "Pakistan"), st(ProcAssembly, "Pakistan")}}), "Pakistan", "single country")
	// Wholly obtained: raw material and operations in one country.
	decided(t, r.Evaluate(Facts{GoodHTS: "5201.00", Complete: true, RawMaterialCountry: "India",
		Stages: []Stage{st(ProcSpinning, "India")}}), "India", "(c)(1)")

	// 5904 (1) turns on lamination, which the facts do not give.
	res = r.Evaluate(Facts{GoodHTS: "5904.10", Complete: true, Stages: []Stage{st(ProcFabricMaking, "China"), st(ProcAssembly, "Vietnam")}})
	if res.Status != Undetermined {
		t.Errorf("got %+v", res)
	}
}

func TestScopeAndCodes(t *testing.T) {
	r := rules(t)
	if res := r.Evaluate(Facts{GoodHTS: "8501.10"}); res.Status != OutOfScope {
		t.Errorf("8501.10: %+v", res)
	}
	// 4202.12.40-89 is in (b)(5); 4202.12.10 is not.
	if in, ok := r.InScope("4202.12.60"); !in || !ok {
		t.Error("4202.12.60 is a textile product under (b)(5)")
	}
	if in, ok := r.InScope("4202.12.10"); in || !ok {
		t.Error("4202.12.10 is not a textile product under (b)(5)")
	}
	// 4202 alone cannot be told.
	if _, ok := r.InScope("4202"); ok {
		t.Error("4202 alone is too short to tell")
	}
	if res := r.Evaluate(Facts{}); res.Class != ClassNoCode {
		t.Errorf("no code: %+v", res)
	}
	// 9619.00.32 is under the specific row 9619.00.31-9619.00.33, not under 9619.
	decided(t, r.Evaluate(Facts{GoodHTS: "9619.00.32", Complete: true,
		Stages: []Stage{st(ProcFabricMaking, "China"), st(ProcAssembly, "Vietnam")}}), "China", "(c)(2)")
	// A heading that holds the (e)(2) subheadings cannot pick between rules.
	if res := r.Evaluate(Facts{GoodHTS: "6117", Complete: false, Stages: []Stage{st(ProcFabricMaking, "China"), st(ProcAssembly, "Vietnam")}}); res.Status != Undetermined {
		t.Errorf("6117: %+v", res)
	}
	// An (e) row of 6201-6208 exists although the eCFR text lacks it.
	decided(t, r.Evaluate(Facts{GoodHTS: "6203.42.40", TwoOrMoreComponents: Yes, Complete: true,
		Stages: []Stage{st(ProcFabricMaking, "China"), st(ProcAssembly, "Vietnam")}}), "Vietnam", "(c)(2)")
}

// The shift clauses are the regulation's.
func TestShiftAllows(t *testing.T) {
	r := rules(t)
	x := row(t, r, "6101-6117")
	a2 := alt(t, x, "2")
	for _, c := range []struct {
		material string
		want     Tri
	}{
		{"5509.21", Yes}, // yarn of heading 5509
		{"5208.52", No},  // excepted: 5208 through 5212
		{"6006.32", No},  // excepted: 6001 through 6006
		{"6105.10", No},  // inside the group 6101-6117
		{"6307.90", No},  // excepted: subheading 6307.90
	} {
		if got := ShiftAllows(x, a2, "6104.43", c.material); got != c.want {
			t.Errorf("material %s = %v, want %v", c.material, got, c.want)
		}
	}
	// 5503-5504: from any other chapter, except from Chapter 54.
	y := row(t, r, "5503-5504")
	a := y.Alts[0]
	if ShiftAllows(y, a, "5503.20", "5402.31") != No || ShiftAllows(y, a, "5503.20", "5201.00") != Yes {
		t.Error("5503-5504: Chapter 54 is excepted, Chapter 52 is not")
	}
}

// Every source row text is in the committed file and every alternative is
// either read or marked as not read by code; nothing is dropped.
func TestEveryRowHasAnAlternative(t *testing.T) {
	r := rules(t)
	for _, x := range r.T.Rows {
		if len(x.Alts) == 0 {
			t.Errorf("row %s has no alternatives", x.Code)
		}
		for _, a := range x.Alts {
			if a.ID == "" || a.Text == "" || a.Req.Kind == "" {
				t.Errorf("row %s: incomplete alternative %+v", x.Code, a)
			}
		}
	}
}

func TestSourceFilesPresent(t *testing.T) {
	for _, f := range []string{"source/ecfr-102.21.xml", "source/fr-87-68356-6201-6208.txt"} {
		if _, err := os.Stat(f); err != nil {
			t.Error(err)
		}
	}
}

// 5604 (1) reads "a change of those filaments": extruded in Turkey and then
// gimped around rubber thread in Switzerland, the change to heading 5604 is
// not the result of extrusion alone, so (c)(2) names no country and (c)(3)(ii)
// gives the country of assembly (N352976, a development ruling).
func TestChangeOfThoseFilaments(t *testing.T) {
	r := rules(t)
	if a := alt(t, row(t, r, "5604"), "1"); !a.Req.Last {
		t.Fatal("5604 (1) should read as a change of those filaments")
	}
	f := Facts{GoodHTS: "5604.10", KnitToShape: No, TwoOrMoreComponents: Yes, Fibre: "filaments", Complete: true,
		Stages: []Stage{st(ProcExtrusion, "Turkey"), st(ProcAssembly, "Switzerland")}}
	decided(t, r.Evaluate(f), "Switzerland", "(c)(3)(ii)")
	// With the extrusion the last operation, the filaments became the good in Turkey.
	f.Stages = []Stage{st(ProcExtrusion, "Turkey"), st(StagePacking, "Switzerland")}
	f.Complete = false
	decided(t, r.Evaluate(f), "Turkey", "(c)(2)")
}

// Each foreign material must change (102.21(c)(2)): one whose code the rule's
// tariff shift excepts, or that is already inside the group, did not.
func TestForeignMaterialInAnExceptedHeading(t *testing.T) {
	r := rules(t)
	x := row(t, r, "6101-6117")
	a2 := alt(t, x, "2")
	f := Facts{GoodHTS: "6117.80", Materials: []Material{{Name: "knit fabric", HTS: "6006.32", Country: "India"}}}
	if v := f.vetoShift(x, a2, "6117.80", "China"); v == "" {
		t.Error("fabric of 6006 from another country is in an excepted heading")
	}
	f.Materials[0].Country = "China" // made in the country itself: not foreign
	if v := f.vetoShift(x, a2, "6117.80", "China"); v != "" {
		t.Errorf("a domestic material was vetoed: %s", v)
	}
	f.Materials = []Material{{Name: "yarn", HTS: "5509.21", Country: "India"}}
	if v := f.vetoShift(x, a2, "6117.80", "China"); v != "" {
		t.Errorf("yarn of 5509 is outside the group and not excepted: %s", v)
	}
	// A foreign material with no code leaves the decision resting on an assumption.
	res := r.Evaluate(Facts{GoodHTS: "6117.80.95", KnitToShape: No, TwoOrMoreComponents: No, CottonWool16: No, Complete: true,
		Stages: []Stage{st(ProcFabricMaking, "China"), st(ProcAssembly, "Vietnam")}})
	if res.Status != Decided || len(res.Assumes) != 1 {
		t.Errorf("got %+v", res)
	}
	// A vetoed process leaves the hierarchy to go on (here to (c)(4)).
	res = r.Evaluate(Facts{GoodHTS: "6117.80.95", KnitToShape: No, TwoOrMoreComponents: No, CottonWool16: No, Complete: true,
		Materials: []Material{{Name: "fabric", HTS: "6006", Country: "India"}},
		Stages:    []Stage{st(ProcFabricMaking, "China"), st(ProcAssembly, "Vietnam")}})
	if res.Status != Undetermined {
		t.Errorf("got %+v", res)
	}
}
