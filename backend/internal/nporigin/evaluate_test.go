package nporigin

import (
	"strings"
	"testing"
)

// A synthetic table (not a published list): one chapter with a residual
// rule by value, one with a mixture rule and a residual rule by weight, and
// rows of each kind the evaluator reads.
func testRules(g General) *Rules {
	row := func(code, desc, text string) Row {
		applies, ex, split, key, err := ParseCode(code)
		if err != nil {
			panic(err)
		}
		return Row{Key: key, Code: code, Applies: applies, Ex: ex, Split: split, Description: desc, Text: text, Alts: ParseRule(key, text)}
	}
	return New(Table{Market: "test", Chapters: []Chapter{
		{Chapter: "09", Residual: "weight", Mixture: "(2) ... more than 50 % by weight ...", Rows: []Row{
			row("0901 21", "Coffee, roasted", "CTSH"),
		}},
		{Chapter: "85", Residual: "value", ChapterRules: []string{"Primary Rule: blanks"}, Notes: []string{"Note 1"}, Rows: []Row{
			row("8527", "Radios", "CTH, except from heading 8529"),
			row("ex 8536", "Semiconductor switches", "CTH, except from heading 8538 ; or Assembly of semi-conductor products"),
			row("ex 8541 (a)", "Photovoltaic cells", "CTH"),
			row("ex 8541 (b)", "Other", "CTH or Assembly of semi-conductor products"),
		}},
		{Chapter: "72", Residual: "value", Rows: []Row{
			row("7217", "Wire", "CTH, except from headings 7213 to 7215 ; or change from headings 7213 to 7215 , provided the material has been cold-formed."),
		}},
		{Chapter: "63", Residual: "value", Rows: []Row{
			row("6301", "Blankets", "Manufacture from yarn"),
			row("6302", "Bed linen", "Manufacture in which the value of the materials used does not exceed 40 % of the ex-works price of the product"),
		}},
		{Chapter: "02", Residual: "weight", Fallback: "Where the primary rule for headings 0201 until 0206 is not met, ...", Rows: []Row{
			row("0201", "Beef", "The origin of the goods of this heading shall be the country in which the animal was fattened for a period of at least three months before slaughtering."),
		}},
	}}, g)
}

var testGeneral = General{
	Market: "test", ListName: "the list", RowRef: "List, ", Listed: "Art. L", Residual: "residual rule", Mixture: "mixture rule",
	Tolerance: "note 2.5", ChapterAlt: "note 2.6", General: "Art. G", Judgement: "A person decides",
	MinimalFallback: true, MinimalRef: "Art. M", MinimalOps: "work in %s is more than minimal", Uneconomic: "economic", Neutral: "neutral",
}

func f(v float64) *float64 { return &v }

func needKeys(r Result) []string {
	var out []string
	for _, n := range r.Needs {
		out = append(out, n.Kind+":"+n.Key)
	}
	return out
}

func TestEvaluateChange(t *testing.T) {
	r := testRules(testGeneral)
	// Radio made in Malaysia from Chinese parts of other headings: CTH met.
	res := r.Evaluate(Input{GoodHTS: "8527.13", Country: "Malaysia", Materials: []Material{
		{Name: "board", HTS: "8534.00", Origin: "China"}, {Name: "speaker", HTS: "8518.29", Origin: "China"}}})
	if res.Status != Decided || res.Origin != "Malaysia" || !strings.Contains(res.Basis, "List, 8527") || len(res.Assumes) != 4 {
		t.Fatalf("got %+v", res)
	}
	// A part of heading 8529 is excepted: not met, then the residual rule by
	// value needs shares.
	in := Input{GoodHTS: "8527.13", Country: "Malaysia", Materials: []Material{
		{Name: "tuner", HTS: "8529.90", Origin: "China"}, {Name: "case", HTS: "3926.90", Origin: "Malaysia"}}}
	res = r.Evaluate(in)
	// The tuner's value decides the 10% tolerance.
	if res.Status != Undetermined || strings.Join(needKeys(res), ",") != "share:value_share" || *res.Needs[0].Material != 0 {
		t.Fatalf("want the tuner's value share, got %+v %v", res, needKeys(res))
	}
	// Tuner at 8%: within the 10% tolerance, so CTH is met.
	in.Materials[0].ValueShare = f(0.08)
	if res = r.Evaluate(in); res.Status != Decided || res.Origin != "Malaysia" {
		t.Fatalf("tolerance: %+v", res)
	}
	// Tuner at 60%: CTH is not met, so the chapter rule (blanks) is asked
	// before the residual rule.
	in.Materials[0].ValueShare = f(0.6)
	if res = r.Evaluate(in); res.Status != Undetermined || strings.Join(needKeys(res), ",") != "fact:ch85.r" {
		t.Fatalf("chapter rule must be asked: %+v", res)
	}
	in.Answers = map[string]bool{"ch85.r": false}
	if res = r.Evaluate(in); res.Status != Undetermined || strings.Join(needKeys(res), ",") != "share:value_share" || *res.Needs[0].Material != 1 {
		t.Fatalf("residual needs the case's share: %+v %v", res, needKeys(res))
	}
	// Case 30%: residual rule by value gives China.
	in.Materials[1].ValueShare = f(0.3)
	res = r.Evaluate(in)
	if res.Status != Decided || res.Origin != "China" || res.Basis != "residual rule" {
		t.Fatalf("residual: %+v", res)
	}
	if res.Materials[0].Status != "no_change" || res.Materials[1].Status != "originating" {
		t.Errorf("materials %+v", res.Materials)
	}
	// Equal shares: no single major portion.
	in.Materials[1].ValueShare = f(0.6)
	if res = r.Evaluate(in); res.Status != Undetermined {
		t.Fatalf("tie: %+v", res)
	}
}

func TestEvaluateNeeds(t *testing.T) {
	r := testRules(testGeneral)
	// No country, no code.
	if res := r.Evaluate(Input{}); res.Status != Undetermined || strings.Join(needKeys(res), ",") != "code:country,code:good_code" {
		t.Fatalf("got %+v", res)
	}
	// A material with no code and one with no origin.
	res := r.Evaluate(Input{GoodHTS: "8527", Country: "Vietnam", Materials: []Material{{Name: "a", Origin: "China"}, {Name: "b", HTS: "8518"}}})
	if res.Status != Undetermined || strings.Join(needKeys(res), ",") != "code:material_hts,code:material_origin" {
		t.Fatalf("got %+v %v", res, needKeys(res))
	}
	// No materials.
	if res := r.Evaluate(Input{GoodHTS: "8527", Country: "Vietnam"}); res.Status != Undetermined || needKeys(res)[0] != "code:materials" {
		t.Fatalf("got %+v", res)
	}
	// CTSH on a 4-digit coffee code: the list is by subheading here.
	if res := r.Evaluate(Input{GoodHTS: "0901", Country: "Italy", Materials: []Material{{HTS: "0901.11", Origin: "Brazil"}}}); res.Status != Undetermined || needKeys(res)[0] != "code:good_code" {
		t.Fatalf("got %+v", res)
	}
}

func TestEvaluateRowChoice(t *testing.T) {
	r := testRules(testGeneral)
	in := Input{GoodHTS: "8541.40", Country: "Malaysia", Materials: []Material{{Name: "wafer", HTS: "3818.00", Origin: "China"}}}
	res := r.Evaluate(in)
	// The splits end in "Other", so they cover the heading: no "none".
	if res.Status != Undetermined || res.Needs[0].Kind != NeedRow || len(res.Needs[0].Options) != 2 {
		t.Fatalf("want a row choice: %+v", res)
	}
	// An "ex" row covers only some goods: "none" is offered.
	res = r.Evaluate(Input{GoodHTS: "8536.50", Country: "Malaysia", Materials: in.Materials})
	if opts := res.Needs[0].Options; len(opts) != 2 || opts[1].Key != RowNone {
		t.Fatalf("ex row options %+v", res.Needs)
	}
	in.Row = "8541xa"
	if res = r.Evaluate(in); res.Status != Decided || res.Rule.Code != "ex 8541 (a)" {
		t.Fatalf("got %+v", res)
	}
	// An "ex" row the good is not: no list rule, so a judgement.
	res = r.Evaluate(Input{GoodHTS: "8536.50", Country: "Malaysia", Row: RowNone, Materials: in.Materials})
	if res.Status != NeedsJudgement || res.Basis != "Art. G" || !strings.Contains(res.Reason, "A person decides") {
		t.Fatalf("got %+v", res)
	}
	// Assembly of semi-conductor products, answered yes, decides the
	// semiconductor switch even with an excepted part.
	res = r.Evaluate(Input{GoodHTS: "8536.50", Country: "Malaysia", Row: "8536x", Answers: map[string]bool{"8536x.2": true},
		Materials: []Material{{Name: "frame", HTS: "8538.90", Origin: "China", ValueShare: f(0.5)}}})
	if res.Status != Decided || !strings.Contains(res.Reason, "Assembly of semi-conductor products") {
		t.Fatalf("got %+v", res)
	}
}

func TestEvaluateNotListed(t *testing.T) {
	r := testRules(testGeneral)
	mats := []Material{{Name: "a", HTS: "8471.70", Origin: "China", ValueShare: f(0.7), WeightShare: f(0.2)}, {Name: "b", HTS: "8473.30", Origin: "Vietnam", ValueShare: f(0.3), WeightShare: f(0.8)}}
	// A chapter the list does not have.
	res := r.Evaluate(Input{GoodHTS: "8471.30", Country: "Vietnam", Materials: mats})
	if res.Status != NeedsJudgement || res.Covered || res.Origin != "" {
		t.Fatalf("got %+v", res)
	}
	// A heading of a listed chapter without a row.
	if res = r.Evaluate(Input{GoodHTS: "8504.40", Country: "Vietnam", Materials: mats}); res.Status != NeedsJudgement || !res.Covered {
		t.Fatalf("got %+v", res)
	}
	// Only minimal operations: chapter 84 counts by value (Art. 34 DA, last
	// paragraph), so China.
	yes := true
	res = r.Evaluate(Input{GoodHTS: "8471.30", Country: "Vietnam", Materials: mats, MinimalOnly: &yes})
	if res.Status != Decided || res.Origin != "China" || res.Basis != "Art. M, last paragraph" {
		t.Fatalf("got %+v", res)
	}
	// Chapter 1 to 29 counts by weight.
	res = r.Evaluate(Input{GoodHTS: "2106.90", Country: "Vietnam", Materials: mats, MinimalOnly: &yes})
	if res.Status != Decided || res.Origin != "Vietnam" {
		t.Fatalf("got %+v", res)
	}
	// A market without that rule sends it to a person.
	g := testGeneral
	g.MinimalFallback, g.MinimalJudgement = false, "needs a person"
	if res = testRules(g).Evaluate(Input{GoodHTS: "8471.30", Country: "Vietnam", Materials: mats, MinimalOnly: &yes}); res.Status != NeedsJudgement || res.Reason != "needs a person" || res.Basis != "Art. M" {
		t.Fatalf("got %+v", res)
	}
}

func TestEvaluateProcessAndResidual(t *testing.T) {
	r := testRules(testGeneral)
	in := Input{GoodHTS: "6301.40", Country: "Turkey", Materials: []Material{{Name: "yarn", HTS: "5509.21", Origin: "India", ValueShare: f(0.7)}, {Name: "trim", HTS: "5808.10", Origin: "Turkey", ValueShare: f(0.1)}}}
	res := r.Evaluate(in)
	if res.Status != Undetermined || needKeys(res)[0] != "fact:6301.1" || res.Needs[0].Label != "In Turkey: Manufacture from yarn?" {
		t.Fatalf("want the process question, naming the work: %+v", res)
	}
	in.Answers = map[string]bool{"6301.1": true}
	if res = r.Evaluate(in); res.Status != Decided || res.Origin != "Turkey" {
		t.Fatalf("got %+v", res)
	}
	in.Answers["6301.1"] = false
	if res = r.Evaluate(in); res.Status != Decided || res.Origin != "India" || res.Basis != "residual rule" {
		t.Fatalf("residual: %+v", res)
	}
	// Max value rule, read as non-originating materials only.
	g := testGeneral
	g.MaxValueNonOriginating = true
	in2 := Input{GoodHTS: "6302.21", Country: "Turkey", Materials: []Material{{Name: "fabric", HTS: "5208.11", Origin: "India", ValueShare: f(0.35)}}}
	if res = testRules(g).Evaluate(in2); res.Status != Decided {
		t.Fatalf("max value: %+v", res)
	}
	in2.Materials[0].ValueShare = f(0.45)
	in2.Answers = nil
	if res = testRules(g).Evaluate(in2); res.Status != Decided || res.Origin != "India" {
		t.Fatalf("max value not met, residual: %+v", res)
	}
	// Without that definition the rule is a question.
	if res = r.Evaluate(in2); res.Status != Undetermined || needKeys(res)[0] != "fact:6302.1" {
		t.Fatalf("got %+v", res)
	}
}

func TestEvaluateChangeFromAndFallback(t *testing.T) {
	r := testRules(testGeneral)
	in := Input{GoodHTS: "7217.10", Country: "Poland", Materials: []Material{{Name: "rod", HTS: "7213.91", Origin: "Ukraine", ValueShare: f(0.8)}}}
	res := r.Evaluate(in)
	if res.Status != Undetermined || needKeys(res)[0] != "fact:7217.2.c" {
		t.Fatalf("want the cold-forming question: %+v", res)
	}
	in.Answers = map[string]bool{"7217.2.c": true}
	if res = r.Evaluate(in); res.Status != Decided || res.Origin != "Poland" {
		t.Fatalf("got %+v", res)
	}
	// Chapter 2: an identifying rule not met, and a note that names the
	// origin by facts: never residual by code.
	res = r.Evaluate(Input{GoodHTS: "0201.10", Country: "Germany", Answers: map[string]bool{"0201.1": false}, Materials: []Material{{Name: "cattle", HTS: "0102.29", Origin: "France", WeightShare: f(1)}}})
	if res.Status != Undetermined || res.Needs[0].Kind != NeedReview {
		t.Fatalf("got %+v", res)
	}
}

func TestEvaluateMixture(t *testing.T) {
	r := testRules(testGeneral)
	in := Input{GoodHTS: "0901.21", Country: "Italy", Materials: []Material{
		{Name: "beans", HTS: "0901.21", Origin: "Brazil", WeightShare: f(0.6), ValueShare: f(0.5)}, {Name: "beans", HTS: "0901.21", Origin: "Vietnam", WeightShare: f(0.4), ValueShare: f(0.3)}}}
	res := r.Evaluate(in)
	if res.Status != Undetermined || needKeys(res)[0] != "fact:ch09.mix" {
		t.Fatalf("got %+v", res)
	}
	in.Answers = map[string]bool{"ch09.mix": true}
	if res = r.Evaluate(in); res.Status != Decided || res.Origin != "Brazil" || res.Basis != "mixture rule" {
		t.Fatalf("got %+v", res)
	}
	in.Materials[0].WeightShare, in.Materials[1].WeightShare = f(0.5), f(0.5)
	if res = r.Evaluate(in); res.Status != Decided || res.Origin != "Italy" {
		t.Fatalf("no majority: where mixed, got %+v", res)
	}
}

func TestAnswerKeys(t *testing.T) {
	r := testRules(testGeneral)
	for _, k := range []string{"ch85.r", "ch09.mix", "8536x.2", "6301.1", "7217.2.c", "0201.1"} {
		if !r.ValidAnswer(k) {
			t.Errorf("%s should be a valid answer key", k)
		}
	}
	for _, k := range []string{"8527.1", "simple_assembly", ""} {
		if r.ValidAnswer(k) {
			t.Errorf("%q should not be a valid answer key", k)
		}
	}
	if !r.ValidRow("8541xa") || !r.ValidRow(RowNone) || r.ValidRow("8541") {
		t.Error("row keys")
	}
}
