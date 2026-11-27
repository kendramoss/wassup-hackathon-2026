package aseanfta

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/atiga"
)

func f(v float64) *float64 { return &v }
func b(v bool) *bool       { return &v }

func check(t *testing.T, a Agreement, in atiga.Input) atiga.Result {
	t.Helper()
	res, err := Check(a, in)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if _, err := json.Marshal(res); err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return res
}

func want(t *testing.T, res atiga.Result, outcome, applied string) {
	t.Helper()
	if res.Outcome != outcome || res.Applied != applied {
		t.Fatalf("got %s/%s, want %s/%s; reasons: %s", res.Outcome, res.Applied, outcome, applied, strings.Join(res.Reasons, " | "))
	}
	if res.Regime == "" || res.Note == "" {
		t.Fatalf("regime %q note %q", res.Regime, res.Note)
	}
}

func srcOf(t *testing.T, tb *atiga.Table) map[string]any {
	t.Helper()
	var src map[string]any
	if err := json.Unmarshal(tb.Source, &src); err != nil {
		t.Fatal(err)
	}
	return src
}

func TestTablesCoverTheSources(t *testing.T) {
	ak, ac := TableFor(AKFTA), TableFor(ACFTA)
	if len(ak.Rules) != 535 {
		t.Fatalf("AKFTA rows %d, want 535 (serial numbers 1 to 535 of Appendix 2)", len(ak.Rules))
	}
	if len(ac.Rules) != 2045 {
		t.Fatalf("ACFTA rows %d, want 2045 (Attachment B lines)", len(ac.Rules))
	}
	if s := srcOf(t, ak); len(s["sha256"].(string)) != 64 || !strings.HasPrefix(s["url"].(string), "https://") {
		t.Fatalf("AKFTA source %v", s)
	}
	copies := srcOf(t, ac)["copies"].([]any)
	if len(copies) != 2 {
		t.Fatalf("ACFTA source copies %v", copies)
	}
	for _, c := range copies {
		if len(c.(map[string]any)["sha256"].(string)) != 64 {
			t.Fatalf("ACFTA copy %v", c)
		}
	}
	for _, tb := range []*atiga.Table{ak, ac} {
		p := tb.Profile()
		for _, r := range tb.Rules {
			alts := r.Alternatives
			for _, v := range r.Variants {
				alts = append(alts, v.Alternatives...)
			}
			for _, a := range alts {
				for _, q := range a.Requires {
					switch q.Kind {
					case "wo", "wo_any":
					case "rvc":
						switch q.Threshold {
						case 35, 40, 45, 50, 55, 60, 70:
						default:
							t.Errorf("%s %s: threshold %d", p.Name, r.Code, q.Threshold)
						}
					case "ctc":
						if q.Level != "chapter" && q.Level != "heading" && q.Level != "subheading" {
							t.Errorf("%s %s: level %q", p.Name, r.Code, q.Level)
						}
					case "process":
						if p.Processes[q.Process] == "" {
							t.Errorf("%s %s: process %q", p.Name, r.Code, q.Process)
						}
					case "materials":
						if len(q.Scope) == 0 || q.Origin == "" {
							t.Errorf("%s %s: materials condition %+v", p.Name, r.Code, q)
						}
					case "value_cap":
						if len(q.Scope) == 0 || q.Percent <= 0 {
							t.Errorf("%s %s: value cap %+v", p.Name, r.Code, q)
						}
					default:
						t.Errorf("%s %s: kind %q", p.Name, r.Code, q.Kind)
					}
				}
			}
		}
	}
}

// --- AKFTA ---

func TestAKFTAChapterRowWhollyObtained(t *testing.T) {
	// Chapter 02 is "Wholly-Obtained or Produced in the territory of the
	// exporting Party" for the whole chapter.
	in := atiga.Input{HS: "0201.10", ProducedIn: "Thailand", FOB: f(100),
		Materials: []atiga.Material{{Name: "carcass", HS: "0201.10", Value: f(50), Country: "Australia"}}}
	res := check(t, AKFTA, in)
	want(t, res, atiga.No, "")
	if res.Rule == nil || res.Rule.Code != "Chapter 02" || !res.Rule.Listed {
		t.Fatalf("rule %+v", res.Rule)
	}
	in.WhollyObtained = true
	want(t, check(t, AKFTA, in), atiga.Yes, "WO")
}

func TestAKFTAChangeOfChapterOrRVC(t *testing.T) {
	// 1103.11: CC or RVC 40.
	in := atiga.Input{HS: "1103.11", ProducedIn: "Vietnam", FOB: f(100),
		Materials: []atiga.Material{{Name: "wheat", HS: "1001.99", Value: f(70), Country: "Australia"}}}
	want(t, check(t, AKFTA, in), atiga.Yes, "CC")
	in.Materials[0].HS = "1102.20" // same chapter: no change
	want(t, check(t, AKFTA, in), atiga.No, "")
	in.Materials[0].Value = f(55) // RVC 45
	want(t, check(t, AKFTA, in), atiga.Yes, "RVC40")
}

func TestAKFTAConditionOnMaterials(t *testing.T) {
	// 1103.19: CC provided the materials of headings 10.03 and 10.06 are
	// wholly obtained in the territory of any Party; or RVC 40 with the same
	// proviso.
	in := atiga.Input{HS: "1103.19", ProducedIn: "Thailand", FOB: f(100),
		Materials: []atiga.Material{{Name: "barley", HS: "1003.90", Value: f(40), Country: "Vietnam", Status: atiga.StatusOriginating}}}
	res := check(t, AKFTA, in)
	if res.Outcome != atiga.Undetermined {
		t.Fatalf("wholly obtained not stated: got %s %v", res.Outcome, res.Reasons)
	}
	in.Materials[0].WhollyObtained = b(true)
	want(t, check(t, AKFTA, in), atiga.Yes, "CC with conditions")
	in.Materials[0].WhollyObtained = b(false) // originating but not wholly obtained
	want(t, check(t, AKFTA, in), atiga.No, "")
	// A Chinese barley is not originating at all.
	in.Materials[0] = atiga.Material{Name: "barley", HS: "1003.90", Value: f(40), Country: "China"}
	want(t, check(t, AKFTA, in), atiga.No, "")
	// No material of those headings: the condition is met.
	in.Materials = []atiga.Material{{Name: "oats", HS: "1004.90", Value: f(40), Country: "China"}}
	want(t, check(t, AKFTA, in), atiga.Yes, "CC with conditions")
}

func TestAKFTAWhollyObtainedInAnyParty(t *testing.T) {
	// 0304.61 needs the good wholly obtained in the territory of any Party:
	// fish caught by a Vietnamese vessel and filleted in Thailand.
	in := atiga.Input{HS: "0304.61", ProducedIn: "Thailand",
		Materials: []atiga.Material{{Name: "tilapia", HS: "0301.93", Country: "Vietnam", Status: atiga.StatusOriginating}}}
	if res := check(t, AKFTA, in); res.Outcome != atiga.Undetermined {
		t.Fatalf("not stated: %s %v", res.Outcome, res.Reasons)
	}
	in.Materials[0].WhollyObtained = b(true)
	res := check(t, AKFTA, in)
	want(t, res, atiga.Yes, "WO-AK")
	if !res.Accumulation {
		t.Error("a Vietnamese material used in Thailand is accumulation")
	}
	// Merely originating (not wholly obtained) fails: no PE shortcut under AKFTA.
	in.Materials[0].WhollyObtained = b(false)
	want(t, check(t, AKFTA, in), atiga.No, "")
	// Third-country material.
	in.Materials[0].Country, in.Materials[0].Status, in.Materials[0].WhollyObtained = "Norway", "", nil
	want(t, check(t, AKFTA, in), atiga.No, "")
}

func TestAKFTAGeneralRuleAndNoPEShortcut(t *testing.T) {
	// 3926.90 is not in Appendix 2: Rule 4.1, RVC 40 or CTH.
	in := atiga.Input{HS: "3926.90", ProducedIn: "Malaysia", FOB: f(100),
		Materials: []atiga.Material{{Name: "resin", HS: "3901.10", Value: f(70), Country: "Japan"}}}
	res := check(t, AKFTA, in)
	want(t, res, atiga.Yes, "CTH")
	if res.Rule == nil || res.Rule.Listed || res.Rule.Code != "Rule 4.1" {
		t.Fatalf("rule %+v", res.Rule)
	}
	in.Materials[0].HS = "3926.10"
	want(t, check(t, AKFTA, in), atiga.No, "")
	// Every material a Korean originating one: under Rule 7 they count as
	// originating, the RVC is 100 % without a FOB value.
	in = atiga.Input{HS: "3926.90", ProducedIn: "Malaysia",
		Materials: []atiga.Material{{Name: "resin", HS: "3926.10", Country: "South Korea", Status: atiga.StatusOriginating}}}
	res = check(t, AKFTA, in)
	want(t, res, atiga.Yes, "RVC40")
	if !res.Accumulation || res.Basis != "PSR" {
		t.Fatalf("accumulation %v basis %q", res.Accumulation, res.Basis)
	}
}

func TestAKFTAParties(t *testing.T) {
	in := atiga.Input{HS: "3926.90", ProducedIn: "Japan", FOB: f(100), Materials: []atiga.Material{{HS: "3901.10", Value: f(10), Country: "CN"}}}
	want(t, check(t, AKFTA, in), atiga.No, "")
	in.ProducedIn = "Timor-Leste"
	want(t, check(t, AKFTA, in), atiga.No, "")
	for _, s := range []string{"Korea", "South Korea", "Republic of Korea", "KR"} {
		in.ProducedIn = s
		if res := check(t, AKFTA, in); res.Outcome != atiga.Yes {
			t.Errorf("%s: %s %v", s, res.Outcome, res.Reasons)
		}
	}
	// China is not an AKFTA Party; Korea is not an ACFTA Party.
	in.ProducedIn = "China"
	want(t, check(t, AKFTA, in), atiga.No, "")
	in.ProducedIn = "Korea"
	want(t, check(t, ACFTA, in), atiga.No, "")
	// A Korean material declared originating counts for an AKFTA good, not an ACFTA one.
	m := atiga.Material{Name: "part", HS: "3926.10", Value: f(80), Country: "Korea", Status: atiga.StatusOriginating}
	in = atiga.Input{HS: "3926.90", ProducedIn: "Vietnam", FOB: f(100), Materials: []atiga.Material{m}}
	if res := check(t, AKFTA, in); res.Outcome != atiga.Yes || !res.Accumulation {
		t.Fatalf("AKFTA: %s %v", res.Outcome, res.Reasons)
	}
	if res := check(t, ACFTA, in); res.Outcome != atiga.No {
		t.Fatalf("ACFTA: %s %v", res.Outcome, res.Reasons)
	}
}

func TestAKFTANonQualifyingOperations(t *testing.T) {
	in := atiga.Input{HS: "3926.90", ProducedIn: "Singapore", FOB: f(100), Operations: []string{"marking", "sorting"},
		Materials: []atiga.Material{{Name: "kit", HS: "3901.10", Value: f(80), Country: "China"}}}
	res := check(t, AKFTA, in)
	want(t, res, atiga.No, "")
	if !strings.Contains(strings.Join(res.Reasons, " "), "Rule 8") {
		t.Errorf("reasons %v", res.Reasons)
	}
	in.Operations = []string{"marking", "moulding"}
	want(t, check(t, AKFTA, in), atiga.Yes, "CTH")
}

func TestAKFTADeMinimisValueAndTextileWeight(t *testing.T) {
	// 0201.10 is covered by the chapter-02 WO row, so use 1103.11 (CC or
	// RVC 40): a 9 % same-chapter input is within de minimis for the CC.
	in := atiga.Input{HS: "1103.11", ProducedIn: "Vietnam", FOB: f(100),
		Materials: []atiga.Material{
			{Name: "wheat", HS: "1001.99", Value: f(60), Country: "AU"},
			{Name: "flour", HS: "1102.20", Value: f(9), Country: "AU"}}}
	res := check(t, AKFTA, in)
	want(t, res, atiga.Yes, "CC")
	if cc := res.Alternatives[0]; cc.Rule != "CC" || cc.Outcome != atiga.Yes || !strings.Contains(strings.Join(cc.Reasons, " "), "Rule 10") {
		t.Fatalf("CC %+v", cc)
	}
	in.Materials[1].Value = f(11)
	if res := check(t, AKFTA, in); res.Alternatives[0].Outcome != atiga.No {
		t.Fatalf("11 %% must fail de minimis: %+v", res.Alternatives[0])
	}

	// Chapters 50 to 63: weight, not value. 6204.xx is "CC with cut and
	// sewn or RVC 40" style; use a chapter-61 row (Chapter 61: CC and cut and
	// sewn in any Party).
	tx := atiga.Input{HS: "6109.10", ProducedIn: "Cambodia", FOB: f(100), Weight: f(10),
		Processes: map[string]bool{"cut_and_sewn": true},
		Materials: []atiga.Material{
			{Name: "cotton jersey", HS: "6110.20", Value: f(30), Weight: f(0.8), Country: "China"}}}
	res = check(t, AKFTA, tx)
	want(t, res, atiga.Yes, "CC with conditions")
	if res.DeMinimis == nil || !res.DeMinimis.Used || res.DeMinimis.WeightShare == nil || *res.DeMinimis.WeightShare != 8 {
		t.Fatalf("de minimis %+v", res.DeMinimis)
	}
	// Without weights the textile de minimis cannot be tested (value is not used).
	tx.Materials[0].Weight, tx.Materials[0].Value = nil, f(70) // RVC 30 fails too
	if res := check(t, AKFTA, tx); res.Outcome != atiga.Undetermined {
		t.Fatalf("no weight: %s %v", res.Outcome, res.Reasons)
	}
	tx.Materials[0].Weight = f(1.5)
	if res := check(t, AKFTA, tx); res.Outcome != atiga.No {
		t.Fatalf("15 %% by weight: %s %v", res.Outcome, res.Reasons)
	}
}

func TestAKFTAVariantBranches(t *testing.T) {
	// 8708.40: A gear boxes (CTH or RVC 40), B parts (RVC 45).
	in := atiga.Input{HS: "8708.40", ProducedIn: "Thailand", FOB: f(100),
		Materials: []atiga.Material{{Name: "gears", HS: "8483.40", Value: f(58), Country: "Japan"}}}
	res := check(t, AKFTA, in)
	if res.Outcome != atiga.Undetermined || !strings.Contains(strings.Join(res.Reasons, " "), "Pick a variant") {
		t.Fatalf("branches differ: %s %v", res.Outcome, res.Reasons)
	}
	in.Variant = "A"
	want(t, check(t, AKFTA, in), atiga.Yes, "CTH")
	in.Variant = "B" // RVC 42 < 45
	want(t, check(t, AKFTA, in), atiga.No, "")
	in.Materials[0].Value = f(54)
	want(t, check(t, AKFTA, in), atiga.Yes, "RVC45 (parts)")
	in.Variant = "C"
	if _, err := Check(AKFTA, in); !errors.Is(err, atiga.ErrInput) {
		t.Fatalf("bad variant: %v", err)
	}
}

func TestAKFTAValueCapAndExcludedHeading(t *testing.T) {
	// 2402.20: change of subheading from any other heading, provided the
	// non-originating materials of heading 24.03 are within 60 % of FOB.
	in := atiga.Input{HS: "2402.20", ProducedIn: "Indonesia", FOB: f(100),
		Materials: []atiga.Material{
			{Name: "tobacco", HS: "2403.19", Value: f(55), Country: "China"},
			{Name: "paper", HS: "4813.10", Value: f(10), Country: "China"}}}
	want(t, check(t, AKFTA, in), atiga.Yes, "CTH with conditions")
	in.Materials[0].Value = f(65)
	want(t, check(t, AKFTA, in), atiga.No, "")
	in.FOB = nil
	if res := check(t, AKFTA, in); res.Outcome != atiga.Undetermined {
		t.Fatalf("no FOB: %s", res.Outcome)
	}
}

func TestAKFTAMissingFactsAreUndetermined(t *testing.T) {
	in := atiga.Input{HS: "1103.11", ProducedIn: "Vietnam",
		Materials: []atiga.Material{{Name: "flour", HS: "1102.20", Country: "AU"}}}
	if res := check(t, AKFTA, in); res.Outcome != atiga.Undetermined {
		t.Fatalf("got %s", res.Outcome)
	}
	if res := check(t, AKFTA, atiga.Input{HS: "1103.11", ProducedIn: ""}); res.Outcome != atiga.Undetermined {
		t.Fatalf("no producer: %s", res.Outcome)
	}
	// Heading 11.05 has subheadings with their own rows: six digits needed.
	res := check(t, AKFTA, atiga.Input{HS: "1105", ProducedIn: "Vietnam", Materials: []atiga.Material{{HS: "1102.20", Value: f(1)}}})
	if res.Outcome != atiga.Undetermined || !strings.Contains(strings.Join(res.Reasons, " "), "6-digit") {
		t.Fatalf("got %s %v", res.Outcome, res.Reasons)
	}
	// Heading 11.06 is a heading-level row: four digits are enough.
	res = check(t, AKFTA, atiga.Input{HS: "1106", ProducedIn: "Vietnam", FOB: f(100), Materials: []atiga.Material{{HS: "0713.10", Value: f(10), Country: "CN"}}})
	want(t, res, atiga.Yes, "CC")
}

func TestAKFTAValidation(t *testing.T) {
	for _, in := range []atiga.Input{
		{HS: "11"},
		{HS: "1103.11", Method: "both"},
		{HS: "1103.11", Processes: map[string]bool{"refining": true}},
		{HS: "1103.11", Weight: f(-1)},
		{HS: "1103.11", Materials: []atiga.Material{{Weight: f(-1)}}},
	} {
		if _, err := Check(AKFTA, in); !errors.Is(err, atiga.ErrInput) {
			t.Errorf("%+v: %v", in, err)
		}
	}
}

// --- ACFTA ---

func TestACFTARulesAndBuildDown(t *testing.T) {
	// 2902.70 is RVC 40 only; the final process must be in a Party.
	in := atiga.Input{HS: "2902.70", ProducedIn: "Malaysia", FOB: f(100),
		Materials: []atiga.Material{{Name: "benzene", HS: "2902.20", Value: f(60), Country: "Japan"}}}
	res := check(t, ACFTA, in)
	want(t, res, atiga.Yes, "RVC40")
	if res.RVC == nil || res.RVC.Direct != nil || res.RVC.Indirect == nil || *res.RVC.Indirect != 40 {
		t.Fatalf("rvc %+v", res.RVC)
	}
	if !strings.Contains(strings.Join(res.Reasons, " "), "final process") {
		t.Errorf("final process not mentioned: %v", res.Reasons)
	}
	in.Materials[0].Value = f(61)
	want(t, check(t, ACFTA, in), atiga.No, "")
	// Only the build-down formula exists; a costs block is ignored, method direct is refused.
	in.Costs = &atiga.Costs{Labour: 50}
	if res := check(t, ACFTA, in); res.Outcome != atiga.No || res.RVC.Direct != nil {
		t.Fatalf("costs must not rescue: %+v", res)
	}
	in.Method = "direct"
	if _, err := Check(ACFTA, in); !errors.Is(err, atiga.ErrInput) {
		t.Fatalf("direct method: %v", err)
	}
}

func TestACFTAExceptFromChapterAndPackaging(t *testing.T) {
	// 1101.00: CC except from Chapter 10.
	in := atiga.Input{HS: "1101.00", ProducedIn: "Thailand", FOB: f(100),
		Materials: []atiga.Material{{Name: "wheat", HS: "1001.99", Value: f(80), Country: "China", Status: atiga.StatusNonOriginating}}}
	res := check(t, ACFTA, in)
	want(t, res, atiga.No, "")
	if len(res.Failing) != 1 || !strings.Contains(res.Failing[0].Reason, "excludes") {
		t.Fatalf("failing %+v", res.Failing)
	}
	in.Materials[0].HS = "1202.41"
	want(t, check(t, ACFTA, in), atiga.Yes, "CC")
	// 3506.10: RVC 40 or CTH, except when the change is only through
	// packaging or presenting goods for sale.
	pk := atiga.Input{HS: "3506.10", ProducedIn: "Singapore", FOB: f(100), Operations: []string{"packaging"},
		Materials: []atiga.Material{{Name: "adhesive", HS: "3901.10", Value: f(80), Country: "Japan"}}}
	want(t, check(t, ACFTA, pk), atiga.No, "")
	pk.Operations = nil
	want(t, check(t, ACFTA, pk), atiga.Yes, "CTH")
}

func TestACFTAMinimalOperationsOnlySpeakToWhollyObtained(t *testing.T) {
	in := atiga.Input{HS: "3926.90", ProducedIn: "Singapore", FOB: f(100), Operations: []string{"packaging"},
		Materials: []atiga.Material{{Name: "kit", HS: "3901.10", Value: f(10), Country: "China", Status: atiga.StatusOriginating}, {Name: "resin", HS: "3901.10", Value: f(50), Country: "Japan"}}}
	// ACFTA Art. 7 does not itself disqualify a good that meets Art. 4.
	res := check(t, ACFTA, in)
	want(t, res, atiga.Yes, "RVC40")
	if !strings.Contains(strings.Join(res.Reasons, " "), "Art. 7") {
		t.Errorf("Art. 7 not mentioned: %v", res.Reasons)
	}
	// A wholly obtained statement is not relied on when the only work is minimal.
	in.WhollyObtained = true
	res = check(t, ACFTA, in)
	if res.Basis == "WO" {
		t.Fatalf("WO relied on: %+v", res)
	}
}

func TestACFTAProcessRulesAndWholly(t *testing.T) {
	// 5205.11: RVC 40 or Process Rule 1 (fibre-making, spinning ...).
	in := atiga.Input{HS: "5205.11", ProducedIn: "Vietnam", FOB: f(100),
		Materials: []atiga.Material{{Name: "cotton yarn", HS: "5205.11", Value: f(70), Country: "India"}}}
	if res := check(t, ACFTA, in); res.Outcome != atiga.Undetermined {
		t.Fatalf("process not stated: %s", res.Outcome)
	}
	in.Processes = map[string]bool{"process_rule_1": true}
	want(t, check(t, ACFTA, in), atiga.Yes, "Process Rule 1")
	in.Processes = map[string]bool{"process_rule_1": false}
	want(t, check(t, ACFTA, in), atiga.No, "")
	// 0101.21 is WO.
	w := atiga.Input{HS: "0101.21", ProducedIn: "Cambodia", WhollyObtained: true}
	want(t, check(t, ACFTA, w), atiga.Yes, "WO")
}

func TestACFTAProduceFromOriginatingMaterials(t *testing.T) {
	// Art. 2(b): produced in a Party exclusively from originating materials of the Parties.
	in := atiga.Input{HS: "8471.30", ProducedIn: "Philippines",
		Materials: []atiga.Material{{Name: "board", HS: "8471.50", Country: "China", Status: atiga.StatusOriginating}}}
	res := check(t, ACFTA, in)
	want(t, res, atiga.Yes, "PE")
	if !res.Accumulation {
		t.Error("accumulation not reported")
	}
}

func TestACFTAGeneralRule(t *testing.T) {
	// 9102.11 (wristwatch): not in Attachment B; chapter 91 is in Art. 4(1)(b).
	in := atiga.Input{HS: "9102.11", ProducedIn: "Singapore", FOB: f(100),
		Materials: []atiga.Material{{Name: "movement", HS: "9108.11", Value: f(70), Country: "Switzerland"}}}
	res := check(t, ACFTA, in)
	want(t, res, atiga.Yes, "CTH")
	if res.Rule.Code != "Art. 4(1)" || res.Rule.Listed {
		t.Fatalf("rule %+v", res.Rule)
	}
	// 8483.40 is not listed and chapter 84 is not in Art. 4(1)(b): RVC 40 only.
	g := atiga.Input{HS: "8483.40", ProducedIn: "Thailand", FOB: f(100),
		Materials: []atiga.Material{{Name: "shaft", HS: "7228.20", Value: f(70), Country: "Japan"}}}
	res = check(t, ACFTA, g)
	want(t, res, atiga.No, "")
	if res.Rule.Concise != "RVC40" {
		t.Fatalf("concise %q", res.Rule.Concise)
	}
	// Heading 29.02 is taken back out of the CTH chapters (note 3).
	if r, listed, ok, _ := TableFor(ACFTA).Lookup("2902.99"); !ok || r == nil || (!listed && r.Concise != "RVC40") {
		t.Fatalf("2902.99: %+v %v", r, listed)
	}
}

func TestACFTAStatesEdition(t *testing.T) {
	res := check(t, ACFTA, atiga.Input{HS: "0101.21", ProducedIn: "Cambodia", WhollyObtained: true})
	if !strings.Contains(res.Note, "HS 2017") || res.Regime != "acfta_preferential" {
		t.Fatalf("note %q regime %q", res.Note, res.Regime)
	}
	res = check(t, AKFTA, atiga.Input{HS: "0101.21", ProducedIn: "Cambodia", WhollyObtained: true})
	if !strings.Contains(res.Note, "HS 2022") || res.Regime != "akfta_preferential" {
		t.Fatalf("note %q regime %q", res.Note, res.Regime)
	}
}
