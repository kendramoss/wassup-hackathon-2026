package atiga

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func f(v float64) *float64 { return &v }

func mustCheck(t *testing.T, in Input) Result {
	t.Helper()
	res, err := Check(in)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if _, err := json.Marshal(res); err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return res
}

func want(t *testing.T, res Result, outcome, applied string) {
	t.Helper()
	if res.Outcome != outcome || res.Applied != applied {
		t.Fatalf("got %s/%s, want %s/%s; reasons: %s", res.Outcome, res.Applied, outcome, applied, strings.Join(res.Reasons, " | "))
	}
	if res.Regime != Regime || res.Note == "" {
		t.Fatalf("regime %q note %q", res.Regime, res.Note)
	}
}

func TestTableCoversTheWholeAnnex(t *testing.T) {
	tb := Default()
	if len(tb.Rules) != 2868 {
		t.Fatalf("rules %d, want 2868 (rows of the ASEAN Secretariat workbook)", len(tb.Rules))
	}
	var src struct {
		SHA256 string `json:"sha256"`
		URL    string `json:"url"`
	}
	if err := json.Unmarshal(tb.Source, &src); err != nil || len(src.SHA256) != 64 || !strings.HasPrefix(src.URL, "https://asean.org/") {
		t.Fatalf("source %+v %v", src, err)
	}
	for _, r := range tb.Rules {
		alts := r.Alternatives
		for _, v := range r.Variants {
			alts = append(alts, v.Alternatives...)
		}
		for _, a := range alts {
			for _, q := range a.Requires {
				switch q.Kind {
				case "wo":
				case "rvc":
					if q.Threshold != 40 && q.Threshold != 35 {
						t.Errorf("%s: threshold %d", r.Code, q.Threshold)
					}
				case "ctc":
					if levelDigits[q.Level] == 0 {
						t.Errorf("%s: level %q", r.Code, q.Level)
					}
				case "process":
					if Processes[q.Process] == "" {
						t.Errorf("%s: process %q", r.Code, q.Process)
					}
				default:
					t.Errorf("%s: kind %q", r.Code, q.Kind)
				}
			}
		}
	}
}

func TestGeneralRuleForUnlistedGoods(t *testing.T) {
	// 8471.30 is listed; 3926.90 is not in Annex 3, so Art. 28.1 applies.
	in := Input{HS: "3926.90", ProducedIn: "Malaysia", FOB: f(100),
		Materials: []Material{{Name: "resin", HS: "3901.10", Value: f(70), Country: "CN"}}}
	res := mustCheck(t, in)
	want(t, res, Yes, "CTH")
	if res.Rule == nil || res.Rule.Listed || res.Rule.Code != "Art. 28.1" {
		t.Fatalf("rule %+v", res.Rule)
	}
	// Same heading: CTH fails, RVC 30 % fails.
	in.Materials[0].HS = "3926.10"
	want(t, mustCheck(t, in), No, "")
}

func TestRVC40Indirect(t *testing.T) {
	in := Input{HS: "3926.90", ProducedIn: "TH", FOB: f(100),
		Materials: []Material{{Name: "part", HS: "3926.90", Value: f(60), Country: "JP"}}}
	want(t, mustCheck(t, in), Yes, "RVC40")
	in.Materials[0].Value = f(61)
	want(t, mustCheck(t, in), No, "")
}

func TestDeMinimisValueOnly(t *testing.T) {
	// 0201.10 is "RVC40 or CC"; a 9 % non-originating input from chapter 02
	// fails the CC but is within the 10 % de minimis (Art. 33).
	in := Input{HS: "0201.10", ProducedIn: "VN", FOB: f(100),
		Materials: []Material{{Name: "beef trim", HS: "0201.20", Value: f(9), Country: "AU"}}}
	res := mustCheck(t, in)
	want(t, res, Yes, "RVC40")
	cc := res.Alternatives[1]
	if cc.Rule != "CC" || cc.Outcome != Yes || !strings.Contains(strings.Join(cc.Reasons, " "), "Art. 33") {
		t.Fatalf("CC alternative %+v", cc)
	}
	// Without values the CC cannot be decided.
	in.Materials[0].Value = nil
	if res := mustCheck(t, in); res.Outcome != Undetermined {
		t.Fatalf("got %s", res.Outcome)
	}
}

func TestWhollyObtainedRow(t *testing.T) {
	in := Input{HS: "0101.21", ProducedIn: "PH", Materials: []Material{{Name: "foal", HS: "0101.21", Country: "AU", Value: f(10)}}, FOB: f(100)}
	want(t, mustCheck(t, in), No, "")
	in.WhollyObtained = true
	want(t, mustCheck(t, in), Yes, "WO")
}

func TestExceptFromHeadings(t *testing.T) {
	// 7317.00 "Of steel: RVC40 or CC except from 7213-7217".
	in := Input{HS: "7317.00", ProducedIn: "ID", FOB: f(100), Variant: "of_steel",
		Materials: []Material{{Name: "wire rod", HS: "7213.91", Value: f(80), Country: "CN"}}}
	res := mustCheck(t, in)
	want(t, res, No, "")
	if len(res.Failing) != 1 || !strings.Contains(res.Failing[0].Reason, "excludes") {
		t.Fatalf("failing %+v", res.Failing)
	}
	in.Variant = "of_stainless_steel"
	want(t, mustCheck(t, in), Yes, "CC")
	// No variant: branches disagree.
	in.Variant = ""
	if res := mustCheck(t, in); res.Outcome != Undetermined || !strings.Contains(strings.Join(res.Reasons, " "), "Pick a variant") {
		t.Fatalf("got %s %v", res.Outcome, res.Reasons)
	}
	if _, err := Check(Input{HS: "7317.00", ProducedIn: "ID", Variant: "nope", Materials: in.Materials}); !errors.Is(err, ErrInput) {
		t.Fatalf("bad variant: %v", err)
	}
}

func TestRVC35PlusCTSH(t *testing.T) {
	// 8541.42: "RVC40 or CTH or RVC35+CTSH except from 8541.43".
	in := Input{HS: "8541.42", ProducedIn: "SG", FOB: f(100),
		Materials: []Material{{Name: "wafer", HS: "8541.90", Value: f(62), Country: "US"}}}
	res := mustCheck(t, in)
	want(t, res, Yes, "RVC35+CTSH except from 8541.43")
	in.Materials[0].HS = "8541.43"
	want(t, mustCheck(t, in), No, "")
}

func TestCutAndSewnConjunction(t *testing.T) {
	// 6101.20: "RVC40 or CC and cut and sewn in any AMS or textile process rule".
	in := Input{HS: "6101.20", ProducedIn: "KH", FOB: f(100),
		Materials: []Material{{Name: "cotton fabric", HS: "5208.12", Value: f(70), Country: "CN"}}}
	if res := mustCheck(t, in); res.Outcome != Undetermined {
		t.Fatalf("process not stated: got %s", res.Outcome)
	}
	in.Processes = map[string]bool{"cut_and_sewn": true}
	want(t, mustCheck(t, in), Yes, "CC and cut and sewn in any AMS")
	in.Processes = map[string]bool{"cut_and_sewn": false, "textile_process": false}
	want(t, mustCheck(t, in), No, "")
	in.Processes = map[string]bool{"textile_process": true, "cut_and_sewn": false}
	want(t, mustCheck(t, in), Yes, "textile process rule")
	if _, err := Check(Input{HS: "6101.20", ProducedIn: "KH", Processes: map[string]bool{"sewing": true}}); !errors.Is(err, ErrInput) {
		t.Fatalf("unknown process: %v", err)
	}
}

func TestCumulation(t *testing.T) {
	in := Input{HS: "3926.10", ProducedIn: "MY", FOB: f(100),
		Materials: []Material{{Name: "sheet", HS: "3926.10", Value: f(80), Country: "Thailand", Status: StatusOriginating}}}
	res := mustCheck(t, in)
	want(t, res, Yes, "PE")
	if !res.Accumulation {
		t.Fatal("accumulation not reported")
	}
	// Declared originating but from a non-Member: counts as non-originating.
	in.Materials[0].Country = "China"
	want(t, mustCheck(t, in), No, "")
}

func TestPartialCumulation(t *testing.T) {
	// A Vietnamese material with 30 % own RVC adds 30 % of its value as
	// ASEAN content: VNM 70 - 15 = 55 -> RVC 45 %.
	in := Input{HS: "3926.90", ProducedIn: "MY", FOB: f(100),
		Materials: []Material{
			{Name: "moulding", HS: "3926.90", Value: f(50), Country: "VN", RVCPct: f(30)},
			{Name: "insert", HS: "3926.90", Value: f(20), Country: "CN"},
		}}
	res := mustCheck(t, in)
	want(t, res, Yes, "RVC40")
	if !res.PartialCumulation || res.RVC.Partial != 15 {
		t.Fatalf("rvc %+v", res.RVC)
	}
	// Below 20 % it does not count.
	in.Materials[0].RVCPct = f(19)
	want(t, mustCheck(t, in), No, "")
	// A non-Member material never cumulates.
	in.Materials[0].RVCPct, in.Materials[0].Country = f(30), "CN"
	want(t, mustCheck(t, in), No, "")
}

func TestMinimalOperationsAndMembers(t *testing.T) {
	in := Input{HS: "3926.90", ProducedIn: "SG", FOB: f(100), Operations: []string{"packaging"},
		Materials: []Material{{Name: "kit", HS: "3926.10", Value: f(10), Country: "CN"}}}
	want(t, mustCheck(t, in), No, "")
	in.ProducedIn = "China"
	want(t, mustCheck(t, in), No, "")
	in.ProducedIn = "Timor-Leste"
	want(t, mustCheck(t, in), No, "")
	in.ProducedIn = ""
	if res := mustCheck(t, in); res.Outcome != Undetermined {
		t.Fatalf("got %s", res.Outcome)
	}
	for s, code := range map[string]string{"Viet Nam": "VN", "lao pdr": "LA", "BN": "BN", "Brunei Darussalam": "BN", "kr": ""} {
		if got := Member(s); got != code {
			t.Errorf("Member(%q) = %q, want %q", s, got, code)
		}
	}
}

func TestHeadingNeedsSubheadingWhenListed(t *testing.T) {
	in := Input{HS: "8541", ProducedIn: "TH", FOB: f(100), Materials: []Material{{HS: "8541.90", Value: f(10), Country: "US"}}}
	res := mustCheck(t, in)
	if res.Outcome != Undetermined || !strings.Contains(strings.Join(res.Reasons, " "), "6-digit") {
		t.Fatalf("got %s %v", res.Outcome, res.Reasons)
	}
	// Heading 39.26 has no Annex 3 row, so 4 digits are enough.
	in.HS = "3926"
	want(t, mustCheck(t, in), Yes, "RVC40")
}

func TestMethod(t *testing.T) {
	in := Input{HS: "3926.90", ProducedIn: "TH", FOB: f(100), Method: "direct",
		Materials: []Material{
			{Name: "local", HS: "3926.90", Value: f(20), Country: "TH", Status: StatusOriginating},
			{Name: "import", HS: "3926.90", Value: f(50), Country: "JP"},
		}}
	if res := mustCheck(t, in); res.Outcome != Undetermined {
		t.Fatalf("direct without costs: %s", res.Outcome)
	}
	in.Costs = &Costs{Labour: 10, Overhead: 5, Profit: 5, Other: 0}
	res := mustCheck(t, in)
	want(t, res, Yes, "RVC40")
	if res.RVC.Direct == nil || *res.RVC.Direct != 40 || res.RVC.Indirect != nil {
		t.Fatalf("rvc %+v", res.RVC)
	}
	if _, err := Check(Input{HS: "3926.90", ProducedIn: "TH", Method: "both"}); !errors.Is(err, ErrInput) {
		t.Fatal("method not validated")
	}
}

func TestValidate(t *testing.T) {
	for _, in := range []Input{
		{HS: "39"},
		{HS: "3926.90", FOB: f(0)},
		{HS: "3926.90", Costs: &Costs{Labour: -1}},
		{HS: "3926.90", Materials: []Material{{Status: "maybe"}}},
		{HS: "3926.90", Materials: []Material{{HS: "3"}}},
		{HS: "3926.90", Materials: []Material{{RVCPct: f(101)}}},
	} {
		if _, err := Check(in); !errors.Is(err, ErrInput) {
			t.Errorf("%+v: %v", in, err)
		}
	}
	huge := Input{HS: "3926.90", ProducedIn: "TH", FOB: f(1e-300), Materials: []Material{{HS: "3926.90", Value: f(1e300), Country: "CN"}}}
	if _, err := Check(huge); !errors.Is(err, ErrInput) {
		t.Fatalf("overflow: %v", err)
	}
}
