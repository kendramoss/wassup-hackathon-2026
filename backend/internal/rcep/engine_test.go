package rcep

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func f(v float64) *float64 { return &v }
func b(v bool) *bool       { return &v }

func check(t *testing.T, in Input) Result {
	t.Helper()
	r, err := Check(in)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if r.Regime != Regime || r.Note == "" {
		t.Fatalf("result not labelled RCEP preferential: %+v", r)
	}
	return r
}

func near(p *float64, want float64) bool { return p != nil && math.Abs(*p-want) < 0.05 }

func nonOrig(name, hs string, value float64) Material {
	return Material{Name: name, HS: hs, Value: f(value), Country: "United States", Status: StatusNonOriginating}
}

// Worked examples from the Australian Border Force guide "Regional
// Comprehensive Economic Partnership (RCEP) Rules of Origin", as at 18
// December 2024 (https://www.abf.gov.au/free-trade-agreements/files/RCEP-rules-of-origin.pdf,
// retrieved 22 Nov 2026). Page numbers are the guide's. Where the guide
// simplifies the Annex 3A rule, the test uses the Annex rule and says so.
func TestABFWorkedExamples(t *testing.T) {
	t.Run("p26 wheat flour CC", func(t *testing.T) {
		r := check(t, Input{HS: "1101.00", ProducedIn: "Malaysia", Materials: []Material{
			nonOrig("wheat", "1001.99", 70),
		}})
		if r.Outcome != Yes || r.Applied != "CC" {
			t.Fatalf("got %s %s: %v", r.Outcome, r.Applied, r.Reasons)
		}
	})
	t.Run("p26 maize flour CC except from chapter 10, non-originating corn", func(t *testing.T) {
		r := check(t, Input{HS: "1102.20", ProducedIn: "Thailand", FOB: f(100), Materials: []Material{
			nonOrig("corn", "1005.90", 60),
		}})
		if r.Outcome != No || len(r.Failing) != 1 || !strings.Contains(r.Failing[0].Reason, "excludes") {
			t.Fatalf("got %s %+v", r.Outcome, r.Failing)
		}
	})
	t.Run("p26 maize flour, originating corn counts as originating", func(t *testing.T) {
		r := check(t, Input{HS: "1102.20", ProducedIn: "Thailand", Materials: []Material{
			{Name: "corn", HS: "1005.90", Country: "Australia", Status: StatusOriginating},
			nonOrig("salt", "2501.00", 1),
		}})
		if r.Outcome != Yes || !r.Accumulation {
			t.Fatalf("got %s accumulation=%v: %v", r.Outcome, r.Accumulation, r.Reasons)
		}
	})
	t.Run("p27 roasted coffee RVC40 only", func(t *testing.T) {
		r := check(t, Input{HS: "0901.21", ProducedIn: "Vietnam", FOB: f(100), Materials: []Material{
			nonOrig("green coffee", "0901.11", 70),
		}})
		if r.Outcome != No || !near(r.RVC.BuildDown, 30) {
			t.Fatalf("got %s %+v", r.Outcome, r.RVC)
		}
	})
	// The guide gives 2009.90 as "CTH or RVC40"; the Annex 3A table says
	// "CC or RVC40". The answer is the same: the citrus is chapter 08.
	t.Run("p29 tropical fruit juice", func(t *testing.T) {
		r := check(t, Input{HS: "2009.90", ProducedIn: "Viet Nam", Materials: []Material{
			{Name: "oranges", HS: "0805.10", Country: "United States", Status: StatusNonOriginating},
			{Name: "limes", HS: "0805.50", Country: "United States", Status: StatusNonOriginating},
			{Name: "bananas", HS: "0803", Country: "Australia", Status: StatusOriginating},
			{Name: "mangoes", HS: "0804", Country: "Australia", Status: StatusOriginating},
		}})
		if r.Outcome != Yes || r.Applied != "CC" || r.Basis != "PSR" {
			t.Fatalf("got %s %s: %v", r.Outcome, r.Applied, r.Reasons)
		}
	})
	t.Run("p32 build-up 45 per cent", func(t *testing.T) {
		r := check(t, Input{HS: "0901.21", ProducedIn: "China", FOB: f(300),
			Costs: &Costs{Labour: 20, Overhead: 20, Profit: 10},
			Materials: []Material{
				{Name: "originating materials", HS: "0901.11", Value: f(85), Country: "China", Status: StatusOriginating},
				{Name: "other materials", HS: "0901.11", Status: StatusNonOriginating},
			}})
		if r.Outcome != Yes || !near(r.RVC.BuildUp, 45) || r.RVC.BuildDown != nil {
			t.Fatalf("got %s %+v", r.Outcome, r.RVC)
		}
	})
	t.Run("p32 build-down 50 per cent", func(t *testing.T) {
		r := check(t, Input{HS: "0901.21", ProducedIn: "Malaysia", FOB: f(100), Materials: []Material{
			nonOrig("non-originating materials", "0901.11", 50),
		}})
		if r.Outcome != Yes || !near(r.RVC.BuildDown, 50) {
			t.Fatalf("got %s %+v", r.Outcome, r.RVC)
		}
	})
	t.Run("p33 benzene chemical reaction", func(t *testing.T) {
		in := Input{HS: "2902.20", ProducedIn: "Japan", Materials: []Material{
			nonOrig("toluene", "2902.30", 70), nonOrig("hydrogen", "2804.10", 5),
		}}
		if r := check(t, in); r.Outcome != Undetermined {
			t.Fatalf("CR not stated: got %s", r.Outcome)
		}
		in.ChemicalReaction = b(true)
		if r := check(t, in); r.Outcome != Yes || r.Applied != "CR" {
			t.Fatalf("got %s %s", r.Outcome, r.Applied)
		}
		in.ChemicalReaction, in.FOB = b(false), f(100)
		if r := check(t, in); r.Outcome != No {
			t.Fatalf("no CR, CTH fails, RVC 25 %% fails: got %s %v", r.Outcome, r.Reasons)
		}
	})
	t.Run("p37 leather jacket from originating materials only", func(t *testing.T) {
		r := check(t, Input{HS: "4203.10", ProducedIn: "Vietnam", Materials: []Material{
			{Name: "tanned sheep leather", HS: "4112.00", Country: "New Zealand", Status: StatusOriginating},
			{Name: "buttons", HS: "9606.21", Country: "China", Status: StatusOriginating},
			{Name: "zippers", HS: "9607.11", Country: "China", Status: StatusOriginating},
		}})
		if r.Outcome != Yes || r.Basis != "PE" || !r.Accumulation {
			t.Fatalf("got %s %s acc=%v", r.Outcome, r.Basis, r.Accumulation)
		}
	})
	// Spice mix 0910.91, CC or RVC40. Ground cinnamon made in Vietnam from
	// Sri Lankan bark fails its own CC rule, so it reaches Malaysia as
	// non-originating at its full $6 (partial cumulation).
	t.Run("p41 spice mix, cinnamon processed in Vietnam", func(t *testing.T) {
		r := check(t, Input{HS: "0910.91", ProducedIn: "Malaysia", FOB: f(18),
			Costs: &Costs{Labour: 3.5, Other: 1, Profit: 2.5},
			Materials: []Material{
				{Name: "vanilla pods", HS: "0905.10", Value: f(5), Country: "Madagascar", Status: StatusNonOriginating},
				{Name: "ground cinnamon", HS: "0906.20", Value: f(6), Country: "Vietnam", Status: StatusNonOriginating},
			}})
		if r.Outcome != No || !near(r.RVC.BuildDown, 38.9) || !near(r.RVC.BuildUp, 38.9) || len(r.Failing) != 2 {
			t.Fatalf("got %s %+v failing=%d", r.Outcome, r.RVC, len(r.Failing))
		}
	})
	t.Run("p42 spice mix, all processing in Malaysia", func(t *testing.T) {
		r := check(t, Input{HS: "0910.91", ProducedIn: "Malaysia", FOB: f(18),
			Costs: &Costs{Labour: 4, Other: 1.3, Profit: 2.7},
			Materials: []Material{
				{Name: "vanilla pods", HS: "0905.10", Value: f(5), Country: "Madagascar", Status: StatusNonOriginating},
				{Name: "cinnamon bark", HS: "0906.11", Value: f(5), Country: "Sri Lanka", Status: StatusNonOriginating},
			}})
		if r.Outcome != Yes || r.Applied != "RVC40" || !near(r.RVC.BuildDown, 44.4) || !near(r.RVC.BuildUp, 44.4) {
			t.Fatalf("got %s %s %+v", r.Outcome, r.Applied, r.RVC)
		}
	})
	t.Run("p46 de minimis by value", func(t *testing.T) {
		// A non-textile good of 8471 (CTH or RVC40): material A changes
		// heading, material B does not and is $5 of a $100 good.
		r := check(t, Input{HS: "8471.30", ProducedIn: "Singapore", FOB: f(100), Materials: []Material{
			nonOrig("material A", "8542.31", 60), nonOrig("material B", "8471.50", 5),
		}})
		if r.Outcome != Yes || r.DeMinimis == nil || !r.DeMinimis.Used || !near(r.DeMinimis.ValueShare, 5) {
			t.Fatalf("got %s %+v", r.Outcome, r.DeMinimis)
		}
	})
	t.Run("p46 de minimis by weight, chapter 55", func(t *testing.T) {
		r := check(t, Input{HS: "5513.11", ProducedIn: "Indonesia", Weight: f(50), Materials: []Material{
			{Name: "X", HS: "5203.00", Weight: f(30), Status: StatusNonOriginating},
			{Name: "Y", HS: "5503.20", Weight: f(18), Status: StatusNonOriginating},
			{Name: "Z", HS: "5513.21", Weight: f(2), Status: StatusNonOriginating},
		}})
		if r.Outcome != Yes || r.DeMinimis == nil || !near(r.DeMinimis.WeightShare, 4) {
			t.Fatalf("got %s %+v %v", r.Outcome, r.DeMinimis, r.Reasons)
		}
	})
}

// Art. 3.2: where the good is made.
func TestArt32ProducedInAParty(t *testing.T) {
	cases := []struct {
		country, want string
	}{{"Vietnam", Yes}, {"VN", Yes}, {"United States", No}, {"Taiwan", No}, {"", Undetermined}}
	for _, c := range cases {
		r := check(t, Input{HS: "1101.00", ProducedIn: c.country, Materials: []Material{nonOrig("wheat", "1001.99", 1)}})
		if r.Outcome != c.want {
			t.Errorf("%q: got %s, want %s", c.country, r.Outcome, c.want)
		}
	}
}

// Art. 3.2(a) and (b), and the WO rule.
func TestArt32Limbs(t *testing.T) {
	r := check(t, Input{HS: "0101.21", ProducedIn: "Australia", WhollyObtained: true})
	if r.Outcome != Yes || r.Basis != "WO" {
		t.Errorf("wholly obtained: %s %s", r.Outcome, r.Basis)
	}
	r = check(t, Input{HS: "0101.21", ProducedIn: "Australia", Materials: []Material{nonOrig("horse", "0101.21", 1)}})
	if r.Outcome != No {
		t.Errorf("WO rule with a non-originating input: %s", r.Outcome)
	}
	r = check(t, Input{HS: "0201.10", ProducedIn: "Australia"})
	if r.Outcome != Undetermined {
		t.Errorf("no materials and not WO: %s", r.Outcome)
	}
}

// Tariff shift at each level, with exclusions.
func TestTariffShift(t *testing.T) {
	cases := []struct {
		name, good, mat, want string
	}{
		{"CC passes from another chapter", "1101.00", "1001.99", Yes},
		{"CC fails within the chapter", "1101.00", "1102.20", No},
		{"CTH passes from another heading", "8471.30", "8473.30", Yes},
		{"CTH fails within the heading", "8471.30", "8471.50", No},
		{"CTSH passes from another subheading", "8516.60", "8516.80", Yes},
		{"CTSH fails within the subheading", "8516.60", "8516.60", No},
		{"CTSH except: excluded subheading", "8504.31", "8504.33", No},
		{"CTSH except: other subheading of the heading", "8504.31", "8504.40", Yes},
		{"CTH except range: excluded heading", "5513.11", "5510.11", No},
		{"CTH except range: outside the range", "5513.11", "5203.00", Yes},
		{"CTH needs 4 digits", "8471.30", "84", Undetermined},
	}
	for _, c := range cases {
		in := Input{HS: c.good, ProducedIn: "Thailand", FOB: f(100), Materials: []Material{nonOrig("part", c.mat, 60)}}
		r := check(t, in)
		// Only the tariff-shift alternative matters here.
		got := ""
		for _, a := range r.Alternatives {
			if !strings.HasPrefix(a.Rule, "RVC") {
				got = a.Outcome
				break
			}
		}
		if got != c.want {
			t.Errorf("%s: %s from %s got %s, want %s (%v)", c.name, c.good, c.mat, got, c.want, r.Reasons)
		}
	}
}

// Art. 3.4: cumulation of originating materials only.
func TestArt34Cumulation(t *testing.T) {
	base := func(status, country string) Input {
		return Input{HS: "0901.21", ProducedIn: "Vietnam", FOB: f(100), Materials: []Material{
			{Name: "green coffee", HS: "0901.11", Value: f(70), Country: country, Status: status},
		}}
	}
	if r := check(t, base(StatusOriginating, "Laos")); r.Outcome != Yes || r.Basis != "PE" || !r.Accumulation {
		t.Errorf("originating in another Party: %s %s %v", r.Outcome, r.Basis, r.Accumulation)
	}
	if r := check(t, base(StatusOriginating, "Vietnam")); r.Accumulation {
		t.Error("same-Party material is not accumulation")
	}
	// Declared originating but from a non-Party: non-originating, RVC 30 %.
	if r := check(t, base(StatusOriginating, "Brazil")); r.Outcome != No || !strings.Contains(strings.Join(r.Reasons, " "), "not an RCEP Party") {
		t.Errorf("non-Party declaration: %s %v", r.Outcome, r.Reasons)
	}
	// Processed in a Party but not originating: full value counts as VNM.
	if r := check(t, base(StatusNonOriginating, "Laos")); r.Outcome != No || !near(r.RVC.BuildDown, 30) {
		t.Errorf("non-originating from a Party: %s %+v", r.Outcome, r.RVC)
	}
}

// Art. 3.5: both formulas, undetermined origin, missing data.
func TestArt35RVC(t *testing.T) {
	in := Input{HS: "0901.21", ProducedIn: "Vietnam", FOB: f(100), Materials: []Material{
		nonOrig("a", "0901.11", 40),
		{Name: "b", HS: "0901.11", Value: f(20), Status: StatusUnknown},
	}}
	r := check(t, in)
	if r.Outcome != Yes || !near(r.RVC.BuildDown, 40) || r.RVC.VNM != 60 {
		t.Fatalf("exactly 40 %% with unknown counted as VNM: %s %+v", r.Outcome, r.RVC)
	}
	in.Materials[1].Value = f(21)
	if r := check(t, in); r.Outcome != No {
		t.Errorf("39 %%: %s", r.Outcome)
	}
	// Build-up rescues it.
	in.Costs = &Costs{Labour: 25, Overhead: 10, Profit: 5}
	if r := check(t, in); r.Outcome != Yes || !near(r.RVC.BuildUp, 40) {
		t.Errorf("build-up 40 %%: %s %+v", r.Outcome, r.RVC)
	}
	// No FOB, or a missing non-originating value: undetermined.
	if r := check(t, Input{HS: "0901.21", ProducedIn: "VN", Materials: []Material{nonOrig("a", "0901.11", 40)}}); r.Outcome != Undetermined {
		t.Errorf("no FOB: %s", r.Outcome)
	}
	if r := check(t, Input{HS: "0901.21", ProducedIn: "VN", FOB: f(100), Materials: []Material{{Name: "a", HS: "0901.11", Status: StatusNonOriginating}}}); r.Outcome != Undetermined {
		t.Errorf("missing VNM value: %s", r.Outcome)
	}
}

// Art. 3.6: minimal operations.
func TestArt36MinimalOperations(t *testing.T) {
	in := Input{HS: "8471.30", ProducedIn: "Malaysia", Operations: []string{"packaging", "marking"},
		Materials: []Material{nonOrig("laptop", "8473.30", 90)}}
	if r := check(t, in); r.Outcome != No || !strings.Contains(r.Reasons[len(r.Reasons)-1], "Art. 3.6") {
		t.Errorf("packaging + marking: %s %v", r.Outcome, r.Reasons)
	}
	in.Operations = []string{"packaging", "assembly"}
	if r := check(t, in); r.Outcome != Yes {
		t.Errorf("real assembly: %s", r.Outcome)
	}
	// Minimal operations on originating materials only: still PE.
	in.Operations = []string{"packaging"}
	in.Materials[0].Status, in.Materials[0].Country = StatusOriginating, "Japan"
	if r := check(t, in); r.Outcome != Yes || r.Basis != "PE" {
		t.Errorf("originating materials: %s %s", r.Outcome, r.Basis)
	}
}

// Art. 3.7: value de minimis, chapter 50-63 weight de minimis, and limits.
func TestArt37DeMinimis(t *testing.T) {
	mk := func(failValue float64) Input {
		return Input{HS: "8471.30", ProducedIn: "Japan", FOB: f(100), Materials: []Material{
			nonOrig("board", "8542.31", 50), nonOrig("same-heading part", "8471.50", failValue),
		}}
	}
	if r := check(t, mk(10)); r.Outcome != Yes || !r.DeMinimis.Used {
		t.Errorf("exactly 10 %%: %s", r.Outcome)
	}
	// 11 % fails CTH; RVC = 100-61 = 39 % fails too.
	if r := check(t, mk(11)); r.Outcome != No || len(r.Failing) != 1 || r.Failing[0].Name != "same-heading part" {
		t.Errorf("11 %%: %s %+v", r.Outcome, r.Failing)
	}
	// Without values the de minimis cannot be tested.
	in := mk(5)
	in.Materials[1].Value = nil
	in.FOB = nil
	if r := check(t, in); r.Outcome != Undetermined {
		t.Errorf("no values: %s", r.Outcome)
	}
	// Weight de minimis is for chapters 50-63 only.
	nonTextile := Input{HS: "8471.30", ProducedIn: "Japan", Weight: f(100), Materials: []Material{
		{Name: "p", HS: "8471.50", Weight: f(1), Status: StatusNonOriginating},
	}}
	if r := check(t, nonTextile); r.Outcome == Yes {
		t.Error("weight de minimis applied outside chapters 50-63")
	}
	textile := Input{HS: "5513.11", ProducedIn: "Japan", Weight: f(100), Materials: []Material{
		{Name: "yarn", HS: "5509.21", Weight: f(11), Status: StatusNonOriginating},
	}}
	if r := check(t, textile); r.Outcome != No {
		t.Errorf("11 %% by weight from an excluded heading: %s", r.Outcome)
	}
}

func TestVariants(t *testing.T) {
	// 1601.00: insects CC or RVC40, others CC. A same-chapter input with
	// RVC 60 % qualifies only for insects.
	in := Input{HS: "1601.00", ProducedIn: "Thailand", FOB: f(100), Materials: []Material{nonOrig("meat", "1602.50", 40)}}
	if r := check(t, in); r.Outcome != Undetermined || !strings.Contains(r.Reasons[len(r.Reasons)-1], `"insect"`) {
		t.Errorf("no variant: %s %v", r.Outcome, r.Reasons)
	}
	in.Variant = "insect"
	if r := check(t, in); r.Outcome != Yes || r.Applied != "RVC40" {
		t.Errorf("insect: %s %s", r.Outcome, r.Applied)
	}
	in.Variant = "other"
	if r := check(t, in); r.Outcome != No {
		t.Errorf("other: %s", r.Outcome)
	}
	// Branches that agree decide without a variant.
	in.Variant = ""
	in.Materials[0].HS = "0207.14"
	if r := check(t, in); r.Outcome != Yes {
		t.Errorf("agreeing branches: %s", r.Outcome)
	}
	in.Variant = "fish"
	if _, err := Check(in); !errors.Is(err, ErrInput) {
		t.Errorf("unknown variant: %v", err)
	}
}

func TestHeading3808Condition(t *testing.T) {
	in := Input{HS: "3808.91", ProducedIn: "China", Materials: []Material{nonOrig("active ingredient", "2930.90", 30)}}
	if r := check(t, in); r.Outcome != Undetermined {
		t.Errorf("condition not stated: %s", r.Outcome)
	}
	in.ActiveIngredientOriginatingPct = f(60)
	if r := check(t, in); r.Outcome != Yes {
		t.Errorf("60 %%: %s", r.Outcome)
	}
	in.ActiveIngredientOriginatingPct, in.FOB = f(40), f(100)
	if r := check(t, in); r.Outcome != Yes || r.Applied != "RVC40" {
		t.Errorf("40 %% active, RVC 70 %%: %s %s", r.Outcome, r.Applied)
	}
}

func TestValidate(t *testing.T) {
	for name, in := range map[string]Input{
		"no hs":        {ProducedIn: "VN"},
		"zero fob":     {HS: "0901.21", FOB: f(0)},
		"bad status":   {HS: "0901.21", Materials: []Material{{HS: "0901", Status: "maybe"}}},
		"negative":     {HS: "0901.21", Materials: []Material{{HS: "0901", Value: f(-1)}}},
		"one-digit hs": {HS: "0901.21", Materials: []Material{{HS: "9"}}},
		"bad pct":      {HS: "3808.91", ActiveIngredientOriginatingPct: f(120)},
	} {
		if _, err := Check(in); !errors.Is(err, ErrInput) {
			t.Errorf("%s: want ErrInput, got %v", name, err)
		}
	}
}

// Every non-yes answer names its reasons; every yes names the rule used.
func TestResultsExplainThemselves(t *testing.T) {
	r := check(t, Input{HS: "8471.30", ProducedIn: "Japan", FOB: f(100), Materials: []Material{nonOrig("p", "8471.50", 70)}})
	if r.Outcome != No || len(r.Reasons) < 2 || r.Rule == nil || r.Rule.Version == "" {
		t.Fatalf("%+v", r)
	}
	for _, s := range r.Reasons {
		if strings.TrimSpace(s) == "" {
			t.Error("empty reason")
		}
	}
}

// #671: a valid 4- or 5-digit good under a heading whose rule uses CTSH must
// not slice the good to six digits. Missing digits make the answer
// undetermined, not a panic or a guess.
func TestShortGoodCTSHIsUndetermined(t *testing.T) {
	for _, tc := range []struct {
		name, hs, country, material string
	}{
		{"heading 28.11, four digits", "2811", "Japan", "2804.10"},
		{"heading 38.08, five digits", "38081", "Vietnam", "2903.11"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := check(t, Input{HS: tc.hs, ProducedIn: tc.country, Materials: []Material{nonOrig("part", tc.material, 10)}})
			if r.Outcome != Undetermined {
				t.Fatalf("got %s, want undetermined: %+v", r.Outcome, r)
			}
			if reasons := strings.Join(r.Reasons, " "); !strings.Contains(reasons, "6-digit subheading") {
				t.Errorf("reasons do not name the missing digits: %s", reasons)
			}
		})
	}
	// Six digits under the same heading still take the ordinary CTSH path.
	r := check(t, Input{HS: "2811.19", ProducedIn: "Japan", Materials: []Material{nonOrig("part", "2804.10", 10)}})
	if r.Outcome != Yes || r.Applied != "CTSH" {
		t.Fatalf("six-digit good: got %s %s: %+v", r.Outcome, r.Applied, r)
	}
}

// Every heading- or chapter-level CTSH rule must survive a 4-digit good.
func TestEveryShortCTSHRuleIsSafe(t *testing.T) {
	tbl := Default()
	seen, checked := map[string]bool{}, 0
	for _, r := range tbl.Rules {
		if r.Level != "heading" && r.Level != "chapter" || seen[r.Code] {
			continue
		}
		seen[r.Code] = true
		alts := append([]Alternative(nil), r.Alternatives...)
		for _, v := range r.Variants {
			alts = append(alts, v.Alternatives...)
		}
		ctsh := false
		for _, a := range alts {
			if a.Criterion == "CTSH" {
				ctsh = true
			}
		}
		if !ctsh {
			continue
		}
		good := digits(r.Code)
		if len(good) == 2 {
			good += "00"
		}
		if len(good) != 4 {
			t.Fatalf("%s: unexpected rule code %q", r.Code, r.Code)
		}
		checked++
		mats := []mat{{Material: Material{HS: "1234.56", Status: StatusNonOriginating}, idx: 0}}
		out, reasons, _, _ := tariffShift(Input{}, good, mats, Alternative{Criterion: "CTSH"})
		if out != Undetermined || len(reasons) == 0 {
			t.Errorf("%s (%s): got %s %v", r.Code, good, out, reasons)
		}
	}
	if checked == 0 {
		t.Fatal("no heading- or chapter-level CTSH rules found")
	}
	t.Logf("checked %d heading/chapter CTSH rules", checked)
}

// #672: finite inputs whose ratios or sums overflow must produce a client
// error, not a result that JSON cannot encode (which the route turns into 500).
func TestComputedNonFiniteIsBadInput(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   Input
	}{
		{"tiny fob", Input{HS: "8471.30", ProducedIn: "Japan", FOB: f(1e-300), Materials: []Material{
			nonOrig("chip", "8471.30", 1e10),
		}}},
		{"materials sum overflows", Input{HS: "8471.30", ProducedIn: "Japan", FOB: f(1e308), Materials: []Material{
			nonOrig("a", "8471.30", 1e308), nonOrig("b", "8471.30", 1e308),
		}}},
		{"costs sum overflows", Input{HS: "8471.30", ProducedIn: "Japan", FOB: f(100),
			Costs:     &Costs{Labour: 1e308, Overhead: 1e308, Profit: 1e308, Other: 1e308},
			Materials: []Material{nonOrig("chip", "8471.50", 10)},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Check(tc.in)
			if !errors.Is(err, ErrInput) {
				t.Fatalf("want ErrInput, got err=%v result=%+v", err, res)
			}
		})
	}
}
