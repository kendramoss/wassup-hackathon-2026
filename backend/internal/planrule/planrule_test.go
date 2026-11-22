package planrule

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

func rules(t *testing.T) *tariffshift.Rules {
	t.Helper()
	r, err := tariffshift.Default()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// A power supply of 8504.40 made in China from a Chinese PCBA (8504.90) and
// a housing (3926.90). Each group can move to the Vietnam plant. The codes
// are illustrative.
func supply(extra ...domain.Component) domain.Product {
	p := domain.Product{
		Name: "Power supply", HTS: "8504.40.95",
		Plants: []domain.Plant{{ID: "cn", Country: "China"}, {ID: "vn", Country: "Vietnam"}},
		Components: []domain.Component{
			{Name: "printed circuit assembly", MadeIn: "China", Plant: "cn", Group: "pcba", HTS: "8504.90.75"},
			{Name: "plastic housing", MadeIn: "China", Plant: "cn", Group: "housing", HTS: "3926.90"},
		},
		Operations: []domain.Operation{
			{Name: "SMT", Plant: "cn", Group: "pcba"},
			{Name: "mould", Plant: "cn", Group: "housing"},
			{Name: "assemble and test", Plant: "cn", Group: "final_assembly"},
		},
	}
	p.Components = append(p.Components, extra...)
	return p
}

func move(groups ...string) domain.Move {
	var m domain.Move
	for _, g := range groups {
		m = append(m, domain.GroupMove{Group: g, ToPlant: "vn"})
	}
	return m
}

func supported(ok bool) *domain.Verdict {
	return &domain.Verdict{PrecedentSupported: ok, SupportingRuling: "N350383", Confidence: 0.95}
}

func TestEligible(t *testing.T) {
	p := supply()
	if !Eligible(&p, false) {
		t.Fatal("every code given: informational check runs")
	}
	p.Components[1].HTS = ""
	if Eligible(&p, false) {
		t.Fatal("a substantial-transformation case with a part missing its code shows nothing")
	}
	if !Eligible(&p, true) {
		t.Fatal("a Part 102 case always carries the check")
	}
	if Eligible(nil, true) {
		t.Fatal("no product, no check")
	}
}

func TestMoveStatuses(t *testing.T) {
	r := rules(t)
	for _, tt := range []struct {
		name    string
		p       domain.Product
		m       domain.Move
		v       *domain.Verdict
		applies string
		status  string
		why     string
		label   string
		agrees  *bool
		review  bool
	}{
		{name: "final assembly to the target: every foreign part shifts", p: supply(), m: move("final_assembly"), v: supported(true),
			applies: AppliesInformational, status: Shifts, label: "Shifts under 102.20 (", agrees: ptr(true)},
		{name: "everything moves: every part from the target", p: supply(), m: move("pcba", "housing", "final_assembly"), v: supported(true),
			applies: AppliesInformational, status: Shifts, label: "Every part from Vietnam (102.11(a)(2))", agrees: ptr(true)},
		{name: "precedent supports, but the good is still finished in China", p: supply(), m: move("pcba"), v: supported(true),
			applies: AppliesInformational, status: OtherCountry, label: "102.20 gives China: finished there", agrees: ptr(false), review: true},
		{name: "rule shifts, precedent not supported: informational, no flag", p: supply(), m: move("final_assembly"), v: supported(false),
			applies: AppliesInformational, status: Shifts, agrees: ptr(false)},
		{name: "rule shifts, precedent not supported: a Part 102 case flags it", p: supply(), m: move("final_assembly"), v: supported(false),
			applies: AppliesPart102, status: Shifts, agrees: ptr(false), review: true},
		{name: "a part inside the group fails; its value decides (de minimis)", p: supply(domain.Component{Name: "transformer", MadeIn: "China", Plant: "cn", Group: "magnetics", HTS: "8504.31"}),
			m: move("final_assembly"), v: supported(true), applies: AppliesInformational, status: Undetermined, why: WhyDeMinimis, label: "Does not shift under 102.20 (transformer fails) unless"},
		{name: "engine missed: no verdict, no agreement", p: supply(), m: move("pcba"), applies: AppliesInformational, status: OtherCountry},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := Check(r, tt.p, domain.Plan{Move: tt.m, Verdict: tt.v}, "Vietnam", tt.applies)
			if c.Status != tt.status || c.Why != tt.why || c.Review != tt.review || c.Applies != tt.applies {
				t.Fatalf("%+v", c)
			}
			if !strings.HasPrefix(c.Label, tt.label) {
				t.Fatalf("label %q, want prefix %q", c.Label, tt.label)
			}
			if (c.AgreesWithPrecedent == nil) != (tt.agrees == nil) || (tt.agrees != nil && *c.AgreesWithPrecedent != *tt.agrees) {
				t.Fatalf("agrees %v, want %v", c.AgreesWithPrecedent, tt.agrees)
			}
			if c.Review && c.ReviewNote == "" {
				t.Fatal("a flag says why")
			}
		})
	}
}

func TestUndeterminedReasons(t *testing.T) {
	r := rules(t)
	p := supply()
	p.Components[1].HTS = ""
	c := Check(r, p, domain.Plan{Move: move("final_assembly")}, "Vietnam", AppliesPart102)
	if c.Status != Undetermined || c.Why != WhyMissingCode || c.Label != "102.20 undetermined (no code for 1 part)" {
		t.Fatalf("%+v", c)
	}
	p = supply()
	p.HTS = "6307.90.98"
	c = Check(r, p, domain.Plan{Move: move("final_assembly")}, "Vietnam", AppliesInformational)
	if c.Status != Undetermined || c.Why != WhyOutOfScope || c.Label != "102.20 undetermined (chapter 63 not encoded)" {
		t.Fatalf("%+v", c)
	}
	p = supply()
	p.HTS = ""
	c = Check(r, p, domain.Plan{Move: move("final_assembly")}, "Vietnam", AppliesPart102)
	if c.Status != Undetermined || c.Why != WhyGoodCode {
		t.Fatalf("%+v", c)
	}
	p = supply()
	p.Operations, p.Plants = nil, append(p.Plants, domain.Plant{ID: "th", Country: "Thailand"})
	c = Check(r, p, domain.Plan{}, "Vietnam", AppliesPart102)
	if c.Status != Undetermined || c.Why != WhyNoCountry {
		t.Fatalf("%+v", c)
	}
}

// Annotate fills copies: the stored search, its verdicts, what is shown and
// the order are untouched.
func TestAnnotateNeverChangesThePlans(t *testing.T) {
	r := rules(t)
	p := supply()
	res := planner.Result{Target: "Vietnam",
		Baseline: domain.Plan{Description: "current setup"},
		Plans:    []domain.Plan{{Move: move("pcba"), Verdict: supported(true), Shown: true}},
		Rejected: []domain.Plan{{Move: move("final_assembly"), Verdict: supported(false), RejectionKind: domain.RejectNoPrecedent}},
	}
	out := Annotate(r, p, res, false)
	if res.Plans[0].RuleCheck != nil || res.Rejected[0].RuleCheck != nil || res.Baseline.RuleCheck != nil {
		t.Fatal("the input was written to")
	}
	if !out.Plans[0].Shown || out.Plans[0].Verdict != res.Plans[0].Verdict || len(out.Plans) != 1 || len(out.Rejected) != 1 {
		t.Fatal("the plans changed")
	}
	if !out.Plans[0].RuleCheck.Review || out.Rejected[0].RuleCheck.Status != Shifts {
		t.Fatalf("%+v %+v", out.Plans[0].RuleCheck, out.Rejected[0].RuleCheck)
	}
	if b := out.Baseline.RuleCheck; b == nil || b.Label != "102.20 gives China" {
		t.Fatalf("baseline %+v", b)
	}
}

func ptr(b bool) *bool { return &b }

// Every need tariffshift can name has short words for the chip.
func TestShortNeedsCoverTariffshift(t *testing.T) {
	src, err := os.ReadFile("../tariffshift/evaluate.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile(`need[A-Za-z]+\s+= "([^"]+)"`).FindAllStringSubmatch(string(src), -1) {
		if _, ok := shortNeeds[m[1]]; !ok {
			t.Errorf("no short words for %q", m[1])
		}
	}
}
