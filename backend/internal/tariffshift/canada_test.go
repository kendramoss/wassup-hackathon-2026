package tariffshift

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func mustCanada(t *testing.T) *Rules {
	t.Helper()
	r, err := CanadaRules()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCAHeadings(t *testing.T) {
	for in, want := range map[string]string{
		"A change to headings 01.01 through 01.06 from any other chapter.":           "A change to headings 0101 through 0106 from any other chapter.",
		"A change to subheading 0210.91 from heading 02.10.":                         "A change to subheading 0210.91 from heading 0210.",
		"except from tariff item 9808.00.00 or heading 84.73; or subheading 8473.30": "except from tariff item 9808.00.00 or heading 8473; or subheading 8473.30",
	} {
		if got := CAHeadings(in); got != want {
			t.Errorf("CAHeadings(%q) = %q, want %q", in, got, want)
		}
	}
	if got := caColumn("01.01 – 01.06"); got != "0101-0106" {
		t.Errorf("caColumn: %q", got)
	}
}

// The committed table's alternatives are what the parser gives for its
// text today, and its coverage is the one docs/NUMBERS.md states.
func TestRulesCAJSONMatchesParser(t *testing.T) {
	r := mustCanada(t)
	tab := r.Table()
	if tab.Version != CanadaTableVersion || tab.Source.SHA256 == "" || !strings.Contains(tab.Source.Licence, "SI/97-5") {
		t.Fatalf("source: %+v version %q", tab.Source, tab.Version)
	}
	for _, rule := range tab.Rules {
		if rule.Alternatives == nil {
			if !rule.NoteOverrides {
				t.Errorf("%s: no alternatives and no note", rule.ID)
			}
			continue
		}
		if got := ParseRule(rule.Text); !reflect.DeepEqual(got, rule.Alternatives) {
			t.Errorf("%s: committed alternatives differ from ParseRule; rebuild with go run ./cmd/tariffshift rules-ca", rule.ID)
		}
	}
	c := CoverageOf(tab)
	want := Coverage{Rows: 572, RowsFull: 544, RowsPartial: 4, RowsNone: 24, Alternatives: 596, Encoded: 548}
	if c != want {
		t.Fatalf("coverage %+v, want %+v (update docs/NUMBERS.md and D720)", c, want)
	}
	if len(tab.Chapters) != 96 {
		t.Errorf("%d chapters, want 96 (1 to 97 without 77)", len(tab.Chapters))
	}
}

// Every row's code column is inside the chapter it is listed under, and no
// two rows cover the same code.
func TestRulesCAShape(t *testing.T) {
	r := mustCanada(t)
	for _, rule := range r.Table().Rules {
		if rule.Applies.Lo[:2] != rule.Paragraph || rule.Applies.Hi[:2] != rule.Paragraph {
			t.Errorf("%s: outside chapter %s", rule.ID, rule.Paragraph)
		}
	}
	for _, code := range []string{"0101", "030621", "610910", "6309", "847130", "850440", "940320", "970110"} {
		if _, why, ok := r.Lookup(code); !ok {
			t.Errorf("%s: %s", code, why)
		}
	}
}

// One case per kind of rule, on Canada's table and general rules.
func TestCanadaRuleKinds(t *testing.T) {
	ca := mustCanada(t)
	cn := func(name, hts string) Material { return Material{Name: name, HTS: hts, Origin: "China"} }
	for _, c := range []struct {
		name    string
		in      Input
		status  string
		origin  string
		basis   string
		matStat []string
	}{
		{"chapter shift: a horse from a foal of the same chapter does not shift; chapter 1 has no de minimis, so the foal gives the origin (s. 5)",
			Input{GoodHTS: "0101.21", Country: "Mexico", Materials: []Material{cn("foal", "0101.29")}},
			Decided, "China", "SOR/94-23 s. 5(1); SOR/94-23 s. 2(2)(a); SOR/94-23 Sch. III, 0101-0106", []string{MatNoShift}},
		{"heading shift including the group: 6002 fabric from 6004 fabric",
			Input{GoodHTS: "6002.40", Country: "Mexico", Materials: []Material{cn("fabric", "6004.10"), cn("yarn", "5509.11")}},
			Decided, "Mexico", "SOR/94-23 s. 4(1)(c); SOR/94-23 Sch. III, 6002-6006", []string{MatShift, MatShift}},
		{"same subheading allowed (s. 12(2)): a pump part from a pump-part casting",
			Input{GoodHTS: "8414.90", Country: "Mexico", Materials: []Material{cn("casting", "8414.90")}},
			Decided, "Mexico", "", []string{MatShift}},
		{"exception: a blanket from woven cotton of 5208 does not shift",
			Input{GoodHTS: "6301.30", Country: "Mexico", Materials: []Material{cn("cotton fabric", "5208.11")}},
			Undetermined, "", "", []string{MatNoShift}},
		{"exception: a blanket from yarn shifts",
			Input{GoodHTS: "6301.30", Country: "Mexico", Materials: []Material{cn("cotton yarn", "5205.11")}},
			Decided, "Mexico", "", []string{MatShift}},
		{"goods described by words: a pillow cover needs a person",
			Input{GoodHTS: "6304.93", Country: "Mexico", Materials: []Material{cn("fabric", "5407.61")}},
			Undetermined, "", "", []string{MatNeedsFact}},
		{"note row 63.09: no tariff shift; all domestic still decides",
			Input{GoodHTS: "6309.00", Country: "Mexico", Materials: []Material{{Name: "used coats", HTS: "6201.30", Origin: "Mexico"}}},
			Decided, "Mexico", "SOR/94-23 s. 4(1)(b)", []string{MatNoShift}},
		{"note row 63.09 with foreign goods: a person applies the note",
			Input{GoodHTS: "6309.00", Country: "Mexico", Materials: []Material{cn("used coats", "6201.30")}},
			Undetermined, "", "", []string{MatNoShift}},
	} {
		t.Run(c.name, func(t *testing.T) {
			res := ca.Evaluate(c.in)
			if res.Status != c.status || res.Origin != c.origin || (c.basis != "" && res.Basis != c.basis) {
				t.Fatalf("%s %q %q: %s", res.Status, res.Origin, res.Basis, res.Reason)
			}
			for i, m := range res.Materials {
				if m.Status != c.matStat[i] {
					t.Errorf("material %d: %s, want %s", i, m.Status, c.matStat[i])
				}
			}
			if res.Status == Decided && !slices.Contains(res.Assumes, Canada.ImportedFrom) {
				t.Errorf("assumes %v", res.Assumes)
			}
		})
	}
}

// The same pump part under 19 CFR 102.20 fails: the U.S. row has no "any
// other good of that subheading".
func TestSameSubheadingDiffersFromUS(t *testing.T) {
	in := Input{GoodHTS: "8414.90", Country: "Mexico", Materials: []Material{{Name: "casting", HTS: "8414.90", Origin: "China"}}}
	us := mustDefault(t).Evaluate(in)
	if us.Materials[0].Status != MatNoShift {
		t.Fatalf("US: %+v", us.Materials[0])
	}
	if ca := mustCanada(t).Evaluate(in); ca.Status != Decided || ca.Origin != "Mexico" {
		t.Fatalf("CA: %+v", ca)
	}
}

// s. 11(1)(a): chapters 50 to 63 are tested by weight, not value.
func TestCanadaDeMinimisByWeight(t *testing.T) {
	in := Input{GoodHTS: "6301.30", Country: "Mexico", Materials: []Material{
		{Name: "cotton yarn", HTS: "5205.11", Origin: "China"},
		{Name: "woven trim", HTS: "5208.11", Origin: "China", ValueShare: f(0.05)},
	}}
	res := mustCanada(t).Evaluate(in)
	if res.Status != Undetermined || !strings.Contains(res.Reason, "weigh no more than 7%") || !strings.Contains(res.Reason, "(s. 11)") {
		t.Fatalf("value only: %+v", res)
	}
	in.Materials[1].WeightShare = f(0.05)
	res = mustCanada(t).Evaluate(in)
	if res.Status != Decided || res.Origin != "Mexico" || !strings.HasPrefix(res.Basis, "SOR/94-23 s. 11(1)") || !strings.Contains(res.Reason, "good's weight") {
		t.Fatalf("%+v", res)
	}
}

// s. 11(1)(b) leaves chapter 6 out of de minimis; 102.13(b) does not. The
// same Canadian row read under each set of general rules shows it.
func TestCanadaDeMinimisChapter6(t *testing.T) {
	in := Input{GoodHTS: "0602.90", Country: "Mexico", Materials: []Material{
		{Name: "rooted cutting", HTS: "0602.10", Origin: "China", ValueShare: f(0.03)},
		{Name: "pot", HTS: "3924.90", Origin: "Mexico"},
	}}
	tab := mustCanada(t).Table()
	if res := New(tab).Evaluate(in); res.Status != Undetermined || strings.Contains(res.Basis, "11(1)") {
		t.Fatalf("Canada: %+v", res)
	}
	tab.Regime = ""
	if res := New(tab).Evaluate(in); res.Status != Decided || !strings.Contains(res.Basis, "102.13") {
		t.Fatalf("U.S. general rules: %+v", res)
	}
}

// s. 9: an answer of Canada for a good produced in the United States
// becomes the United States.
func TestCanadaProductionOutside(t *testing.T) {
	in := Input{GoodHTS: "6301.30", Country: "United States", Materials: []Material{
		{Name: "woven cotton", HTS: "5208.11", Origin: "Canada", WeightShare: f(0.9)},
		{Name: "thread", HTS: "5204.11", Origin: "Mexico", WeightShare: f(0.1)},
	}}
	res := mustCanada(t).Evaluate(in)
	if res.Status != Decided || res.Origin != "United States" || !strings.HasSuffix(res.Basis, "s. 9") {
		t.Fatalf("%+v", res)
	}
	// Under s. 5 alone the cotton (the only material where the rule allows
	// no change) gives Canada.
	if !strings.Contains(res.Basis, "s. 5(1)") {
		t.Fatalf("basis %q", res.Basis)
	}
}

// s. 4(1)(d): a chapter-29 good is decided by the tariff shift only on the
// assumption that the chemical-reaction note does not give another country.
func TestCanadaNoteOverride(t *testing.T) {
	in := Input{GoodHTS: "2915.21", Country: "Mexico", Materials: []Material{{Name: "ethanol", HTS: "2207.10", Origin: "Brazil"}}}
	res := mustCanada(t).Evaluate(in)
	if res.Status != Decided || res.Origin != "Mexico" {
		t.Fatalf("%+v", res)
	}
	found := false
	for _, a := range res.Assumes {
		found = found || strings.Contains(a, "sets origin by a process")
	}
	if !found {
		t.Fatalf("assumes %v", res.Assumes)
	}
}

// s. 8 is noted (not decided) when the answer is not a CUSMA country.
func TestCanadaPreferenceOverrideNoted(t *testing.T) {
	in := Input{GoodHTS: "6301.30", Country: "Mexico", Materials: []Material{
		{Name: "woven cotton", HTS: "5208.11", Origin: "China", WeightShare: f(0.9)},
	}}
	res := mustCanada(t).Evaluate(in)
	if res.Status != Decided || res.Origin != "China" {
		t.Fatalf("%+v", res)
	}
	if !strings.Contains(strings.Join(res.Assumes, "|"), "s. 8") {
		t.Fatalf("assumes %v", res.Assumes)
	}
}

// The U.S. answers do not move: every string of a U.S. answer is the one
// the package gave before rule sets existed (same checks as -check runs).
func TestUSGeneralRulesUnchanged(t *testing.T) {
	r := mustDefault(t)
	if r.General().ID != US.ID {
		t.Fatal(r.General().ID)
	}
	res := r.Evaluate(Input{GoodHTS: "8473.30.5100", Country: "Mexico", Materials: []Material{{Name: "fins", HTS: "7616.99", Origin: "China"}}})
	if res.Basis != "19 CFR 102.11(a)(3); 19 CFR 102.20(o), 8473" || res.Assumes[0] != "the work in Mexico is more than the operations 102.17 says do not count (for example simple packing, dilution or disassembly)" {
		t.Fatalf("%+v", res)
	}
}

var rePlural = regexp.MustCompile(`\b(sub)?headings\b`)

// normRow is a row's text with "headings" read as "heading" and spaces
// folded: Canada writes "subheadings 8424.10 through 8424.89" where the
// eCFR writes "subheading 8424.10 through 8424.89".
func normRow(s string) string {
	s = rePlural.ReplaceAllStringFunc(s, func(w string) string { return strings.TrimSuffix(w, "s") })
	return strings.Join(strings.Fields(s), " ")
}

// identicalRows are the pairs of U.S. and Canadian rows that cover the
// same codes with the same text (up to normRow).
func identicalRows(t *testing.T) [][2]Rule {
	t.Helper()
	ca := map[Range]Rule{}
	for _, r := range mustCanada(t).Table().Rules {
		ca[r.Applies] = r
	}
	var out [][2]Rule
	for _, u := range mustDefault(t).Table().Rules {
		if c, ok := ca[u.Applies]; ok && normRow(c.Text) == normRow(u.Text) {
			out = append(out, [2]Rule{u, c})
		}
	}
	return out
}

// Parity: where the Canadian and U.S. rows are the same text, both rule
// sets give the same answer on the same inputs. Fixtures are built from
// each row: the good at the row's first code, and one foreign material at
// a time from codes the row's own text names (its target range, its
// exceptions and listed sources), a code in the good's own heading, and two
// far codes. The production country (Mexico) and the material origin
// (China) keep s. 8 and s. 9 out, and no value is given, so de minimis
// stays open in both; only the citations and assumptions may differ.
func TestParityOnIdenticalRows(t *testing.T) {
	pairs := identicalRows(t)
	if len(pairs) != 116 {
		t.Fatalf("%d identical rows, want 116 (update D724 and NUMBERS)", len(pairs))
	}
	us, ca := mustDefault(t), mustCanada(t)
	n, decided, rows := 0, 0, 0
	for _, p := range pairs {
		u := p[0]
		if len(u.Textile) > 0 || u.FallbackNote || p[1].NoteOverrides {
			continue // a note sends the good elsewhere in one set
		}
		rows++
		good := pad6(u.Applies.Lo)
		codes := []string{"3926.90", "7318.15", good[:4] + "99", good}
		for _, a := range u.Alternatives {
			for _, r := range a.To {
				codes = append(codes, pad6(r.Lo), pad6(r.Hi))
			}
			for _, e := range a.Except {
				for _, r := range e.From {
					codes = append(codes, pad6(r.Lo))
				}
			}
			for _, s := range a.Also {
				for _, r := range s.From {
					codes = append(codes, pad6(r.Lo))
				}
			}
		}
		for _, code := range codes {
			for _, sa := range []*bool{nil, b(true), b(false)} {
				in := Input{GoodHTS: good, Country: "Mexico", SimpleAssembly: sa, GRI2a: sa, BendingToShape: sa,
					Materials: []Material{{Name: "part", HTS: code, Origin: "China"}}}
				ru, rc := us.Evaluate(in), ca.Evaluate(in)
				n++
				if ru.Status == Decided {
					decided++
				}
				if got, want := answerKey(rc), answerKey(ru); got != want {
					t.Errorf("%s good %s material %s: Canada %s, U.S. %s", u.HTS, good, code, got, want)
				}
			}
		}
	}
	t.Logf("%d rows, %d inputs, %d decided by the U.S. rules", rows, n, decided)
	if n != 2094 || decided != 1029 {
		t.Fatalf("only %d inputs (%d decided); the fixtures are broken", n, decided)
	}
}

func pad6(c string) string {
	if len(c) >= 6 {
		return c[:6]
	}
	return c + strings.Repeat("0", 6-len(c))
}

// answerKey is an answer without its citations and assumptions.
func answerKey(r Result) string {
	k := r.Status + "|" + r.Origin
	for _, m := range r.Materials {
		k += fmt.Sprintf("|%s:%d:%v", m.Status, m.Alternative, m.Needs)
	}
	if r.IfNotDeMinimis != nil {
		k += "|if:" + r.IfNotDeMinimis.Origin
	}
	return k
}

// rules_ca.json is valid JSON with the regime set (a guard for hand edits).
func TestRulesCAFile(t *testing.T) {
	b, err := os.ReadFile("rules_ca.json")
	if err != nil {
		t.Fatal(err)
	}
	var tab Table
	if err := json.Unmarshal(b, &tab); err != nil || tab.Regime != Canada.ID {
		t.Fatalf("%v %q", err, tab.Regime)
	}
}
