package usmca

import (
	"strings"
	"testing"
)

func real(t *testing.T) *RuleSet {
	t.Helper()
	rs, err := DefaultRules()
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func TestRealRowsEnabledOnlyForPilotCodes(t *testing.T) {
	rs := real(t)
	for _, hs := range []string{"8202310000", "82023100", "820231", "8207130000", "820713", "9503000000", "95030000", "950300"} {
		out := mustCheck(t, rs, base(hs, nonOrig("880100")))
		if out.Status == RuleNotSupported || out.RowID == "" {
			t.Errorf("%s should be covered: %+v", hs, out.UnsupportedReasons)
		}
	}
	for _, hs := range []string{
		"8202390000", "820239", "8202100000", "8202990000", "8207190000", "820719", "8207900000", "8207200000",
		"9503100000", "9504000000", "9505000000", "9503900000", "9506000000", "8201000000",
	} {
		out := mustCheck(t, rs, base(hs, nonOrig("880100")))
		if out.Status != RuleNotSupported {
			t.Errorf("%s must stay not_supported, got %s", hs, out.Status)
		}
	}
	// 4-digit goods are too imprecise, never silently supported.
	for _, hs := range []string{"8202", "8207", "9503"} {
		if got := mustCheck(t, rs, base(hs, nil)).Status; got != RuleNeedsInformation {
			t.Errorf("%s: got %s", hs, got)
		}
	}
	for _, hs := range []string{"9504", "9505", "8201"} {
		if got := mustCheck(t, rs, base(hs, nil)).Status; got != RuleNotSupported {
			t.Errorf("%s: got %s", hs, got)
		}
	}
}

func TestRealRowsCarryExcerptAndSnapshot(t *testing.T) {
	rs := real(t)
	out := mustCheck(t, rs, base("8202310000", nonOrig("880100")))
	if out.Source.Row == nil || !strings.Contains(out.Source.Row.Source.Excerpt, "60 percent") ||
		out.Source.Snapshot.SourceHash == "" || !strings.Contains(out.Source.Snapshot.Revision, "Rev 23") {
		t.Fatalf("%+v", out.Source)
	}
	var note string
	for _, a := range out.Alternatives {
		if a.ID == "B" {
			note = a.Note
		}
	}
	if !strings.Contains(note, "GN 11(m)(vii)") {
		t.Fatalf("interpretation note missing: %q", note)
	}
	joined := strings.Join(out.Limits, " ")
	for _, w := range []string{"Art. 4.2(d)", "de minimis", "sets", "Accumulation", "remanufactured", "automotive", "simple_goods_confirmed"} {
		if !strings.Contains(joined, w) {
			t.Errorf("limits missing %q", w)
		}
	}
}

func alt(o Output, id string) Outcome {
	for _, a := range o.Alternatives {
		if a.ID == id {
			return a.Result
		}
	}
	return ""
}

func TestRow820231(t *testing.T) {
	rs := real(t)
	tvHi := func(in Input) Input { return withRVC(in, "transaction_value", "100", "", "5") }
	for _, hs := range []string{"8202310000", "820231"} {
		// A: all inputs outside chapter 82.
		out := mustCheck(t, rs, base(hs, nonOrig("7208000000", "4407")))
		if out.Status != RuleMet || alt(out, "A") != OutcomeSatisfied {
			t.Fatalf("%s: %+v", hs, out.Alternatives)
		}
	}
	// A met with no RVC inputs and no way for B to change the result.
	out := mustCheck(t, rs, base("8202310000", nonOrig("720800")))
	if out.Status != RuleMet || out.RVC != nil || len(out.MissingFacts) != 0 {
		t.Fatalf("%+v", out)
	}
	// Same code input fails A and B regardless of RVC.
	out = mustCheck(t, rs, tvHi(base("8202310000", nonOrig("820231"))))
	if out.Status != RuleNotMet || alt(out, "A") != OutcomeNotSatisfied || alt(out, "B") != OutcomeNotSatisfied {
		t.Fatalf("%+v", out.Alternatives)
	}
	// Known forbidden chapter 82 non-sibling material fails both, regardless of RVC.
	for _, hs := range []string{"820100", "820220", "8203", "820790", "8207130000"} {
		out = mustCheck(t, rs, tvHi(base("8202310000", nonOrig("720800", hs))))
		if out.Status != RuleNotMet {
			t.Fatalf("%s: %s", hs, out.Status)
		}
	}
	// 8202.39 sibling plus other-chapter input passes B (A fails).
	in := tvHi(base("8202310000", nonOrig("8202390000", "720800")))
	out = mustCheck(t, rs, in)
	if out.Status != RuleMet || alt(out, "A") != OutcomeNotSatisfied || alt(out, "B") != OutcomeSatisfied {
		t.Fatalf("%+v", out.Alternatives)
	}
	// B needs no 8202.39 material at all when there are none ch82 inputs, but then A holds anyway.
	// B with RVC just below / at threshold (TV 60, NC 50).
	at := func(method, tv, nc, vnm string) Output {
		return mustCheck(t, rs, withRVC(base("8202310000", nonOrig("820239")), method, tv, nc, vnm))
	}
	if at("transaction_value", "100", "", "40").Status != RuleMet || at("transaction_value", "100", "", "40.000000000001").Status != RuleNotMet {
		t.Fatal("TV 60 boundary")
	}
	if at("net_cost", "", "100", "50").Status != RuleMet || at("net_cost", "", "100", "50.000000000001").Status != RuleNotMet {
		t.Fatal("NC 50 boundary")
	}
	if at("net_cost", "", "100", "45").Status != RuleMet || at("transaction_value", "100", "", "45").Status != RuleNotMet {
		t.Fatal("separate method thresholds")
	}
	// Sibling alone without RVC method: needs the method.
	out = mustCheck(t, rs, base("8202310000", nonOrig("820239")))
	if out.Status != RuleNeedsInformation || !hasMissing(out, "rvc_method") {
		t.Fatalf("%+v", out)
	}
	// Unknown material HS: needs_information when it could decide.
	out = mustCheck(t, rs, base("8202310000", []Material{mat("", ClaimNonOriginating)}))
	if out.Status != RuleNeedsInformation || !hasMissing(out, "materials[0].hs") {
		t.Fatalf("%+v", out)
	}
	// Unknown HS but a known forbidden ch82 input elsewhere fails both despite RVC.
	out = mustCheck(t, rs, tvHi(base("8202310000", []Material{mat("", ClaimNonOriginating), mat("820231", ClaimNonOriginating)})))
	if out.Status != RuleNotMet {
		t.Fatalf("%+v", out)
	}
	// Negative RVC with shift A met.
	out = mustCheck(t, rs, withRVC(base("8202310000", nonOrig("720800")), "transaction_value", "10", "", "20"))
	if out.Status != RuleMet || out.RVC.Exact != "-100" {
		t.Fatalf("%+v", out)
	}
	// Zero denominator: A still resolves; with B-only facts it stays unresolved.
	out = mustCheck(t, rs, withRVC(base("8202310000", nonOrig("720800")), "net_cost", "", "0", "0"))
	if out.Status != RuleMet || out.RVC.Status != StatusUndefinedDenominator {
		t.Fatalf("%+v", out)
	}
	out = mustCheck(t, rs, withRVC(base("8202310000", nonOrig("820239")), "net_cost", "", "0", "0"))
	if out.Status != RuleNeedsInformation {
		t.Fatalf("%+v", out)
	}
	// Originating chapter 82 material is not subject to the shift.
	out = mustCheck(t, rs, base("8202310000", []Material{mat("820231", ClaimOriginating)}))
	if out.Status != RuleMet {
		t.Fatalf("%+v", out)
	}
}

func TestRow820713(t *testing.T) {
	rs := real(t)
	hi := func(in Input) Input { return withRVC(in, "net_cost", "", "100", "10") }
	if got := mustCheck(t, rs, base("8207130000", nonOrig("390100"))).Status; got != RuleMet {
		t.Fatalf("A: %s", got)
	}
	if got := mustCheck(t, rs, hi(base("8207130000", nonOrig("820713")))).Status; got != RuleNotMet {
		t.Fatalf("same code: %s", got)
	}
	if got := mustCheck(t, rs, hi(base("8207130000", nonOrig("820790", "390100")))).Status; got != RuleNotMet {
		t.Fatalf("other ch82 sibling 8207.90: %s", got)
	}
	// 8202.39 is not the 8207 sibling.
	if got := mustCheck(t, rs, hi(base("8207130000", nonOrig("820239")))).Status; got != RuleNotMet {
		t.Fatalf("8202.39 input: %s", got)
	}
	out := mustCheck(t, rs, hi(base("8207130000", nonOrig("8207190000", "390100"))))
	if out.Status != RuleMet || alt(out, "A") != OutcomeNotSatisfied || alt(out, "B") != OutcomeSatisfied {
		t.Fatalf("%+v", out.Alternatives)
	}
	// 49.99... NC fails, exactly 50 passes.
	if got := mustCheck(t, rs, withRVC(base("8207130000", nonOrig("820719")), "net_cost", "", "100", "50.000000000001")).Status; got != RuleNotMet {
		t.Fatalf("got %s", got)
	}
	if got := mustCheck(t, rs, withRVC(base("8207130000", nonOrig("820719")), "net_cost", "", "100", "50")).Status; got != RuleMet {
		t.Fatalf("got %s", got)
	}
}

func TestRow950300(t *testing.T) {
	rs := real(t)
	// A is a subheading shift, not a chapter shift: same-chapter 9504/9505 materials pass.
	out := mustCheck(t, rs, base("9503000000", nonOrig("950400", "950510")))
	if out.Status != RuleMet || alt(out, "A") != OutcomeSatisfied {
		t.Fatalf("%+v", out.Alternatives)
	}
	// Other subheading within 9503 also passes the subheading shift.
	if got := mustCheck(t, rs, base("9503000000", nonOrig("950390"))).Status; got != RuleMet {
		t.Fatalf("950390: %s", got)
	}
	// Same-subheading material fails A; B passes with RVC and needs no material HS.
	in := withRVC(base("9503000000", nonOrig("950300")), "transaction_value", "100", "", "55")
	out = mustCheck(t, rs, in)
	if out.Status != RuleMet || alt(out, "A") != OutcomeNotSatisfied || alt(out, "B") != OutcomeSatisfied {
		t.Fatalf("%+v", out.Alternatives)
	}
	// Same-subheading material with low RVC fails.
	in = withRVC(base("9503000000", nonOrig("950300")), "transaction_value", "100", "", "56")
	if got := mustCheck(t, rs, in).Status; got != RuleNotMet {
		t.Fatalf("got %s", got)
	}
	// B boundaries: TV 45, NC 35, exact and just below.
	run := func(method, tv, nc, vnm string) RuleStatus {
		return mustCheck(t, rs, withRVC(base("9503000000", nonOrig("950300")), method, tv, nc, vnm)).Status
	}
	if run("transaction_value", "100", "", "55") != RuleMet || run("transaction_value", "100", "", "55.000000000001") != RuleNotMet {
		t.Fatal("TV 45")
	}
	if run("net_cost", "", "100", "65") != RuleMet || run("net_cost", "", "100", "65.000000000001") != RuleNotMet {
		t.Fatal("NC 35")
	}
	// B needs no material HS: unknown-HS materials, RVC passes -> met.
	in = withRVC(base("9503000000", []Material{mat("", ClaimNonOriginating)}), "transaction_value", "100", "", "10")
	if got := mustCheck(t, rs, in).Status; got != RuleMet {
		t.Fatalf("got %s", got)
	}
	// B with no materials list at all (not confirmed complete) still computes.
	in = withRVC(base("9503000000", nil), "transaction_value", "100", "", "10")
	in.Profile.MaterialsComplete = false
	if got := mustCheck(t, rs, in).Status; got != RuleMet {
		t.Fatalf("RVC-only branch should resolve: %s", got)
	}
	// Missing material list is not vacuously all-originating unless explicitly complete.
	in = base("9503000000", nil)
	in.Profile.MaterialsComplete = false
	out = mustCheck(t, rs, in)
	if out.Status != RuleNeedsInformation || !hasMissing(out, "profile.materials_list_complete") {
		t.Fatalf("%+v", out)
	}
	// Explicitly complete and empty: A is vacuously satisfied (all inputs originating).
	if got := mustCheck(t, rs, base("9503000000", nil)).Status; got != RuleMet {
		t.Fatalf("got %s", got)
	}
	// Unknown material HS, no RVC: needs information.
	out = mustCheck(t, rs, base("9503000000", []Material{mat("", ClaimNonOriginating)}))
	if out.Status != RuleNeedsInformation || !hasMissing(out, "materials[0].hs") || !hasMissing(out, "rvc_method") {
		t.Fatalf("%+v", out)
	}
	// 4-digit material 9503 shares heading but its subheading is unknown.
	if got := mustCheck(t, rs, base("9503000000", nonOrig("9503"))).Status; got != RuleNeedsInformation {
		t.Fatalf("got %s", got)
	}
	// Negative RVC with A met; zero denominator with A met.
	out = mustCheck(t, rs, withRVC(base("9503000000", nonOrig("950400")), "transaction_value", "10", "", "20"))
	if out.Status != RuleMet || out.RVC.Exact != "-100" {
		t.Fatalf("%+v", out.RVC)
	}
	out = mustCheck(t, rs, withRVC(base("9503000000", nonOrig("950300")), "transaction_value", "0", "", "0"))
	if out.Status != RuleNeedsInformation || out.RVC.Status != StatusUndefinedDenominator {
		t.Fatalf("%+v", out)
	}
}

func TestPartsRouteRelianceOnly(t *testing.T) {
	rs := real(t)
	// A BOM containing 8202.39 parts does not trigger not_supported.
	in := withRVC(base("8202310000", nonOrig("8202390000", "720800")), "transaction_value", "100", "", "10")
	if got := mustCheck(t, rs, in).Status; got != RuleMet {
		t.Fatalf("got %s", got)
	}
	// Relying on the parts route does.
	in.Profile.PartsRoute = ProvisionApplies
	out := mustCheck(t, rs, in)
	if out.Status != RuleNotSupported || !strings.Contains(strings.Join(out.UnsupportedReasons, " "), "parts_route") {
		t.Fatalf("%+v", out)
	}
	// Unconfirmed reliance is not positive.
	in.Profile.PartsRoute = ""
	if got := mustCheck(t, rs, in).Status; got != RuleNeedsInformation {
		t.Fatalf("got %s", got)
	}
}

func TestAllInputsFromLoader(t *testing.T) {
	mut := func(old, new string) []byte { return []byte(strings.Replace(syntheticRules, old, new, 1)) }
	if _, err := LoadRules(mut(`{"kind":"change_from_chapter"}`, `{"kind":"all_inputs_from"}`)); err == nil {
		t.Error("all_inputs_from without sets must be rejected")
	}
	if _, err := LoadRules(mut(`{"kind":"change_from_chapter"}`, `{"kind":"all_inputs_from","others_from":["7702"],"at_least_one_from":["77"]}`)); err == nil {
		t.Error("stray at_least_one_from must be rejected")
	}
}

func TestMutationIsolation(t *testing.T) {
	rs := real(t)
	rows := rs.Rows()
	rows[0].ID = "x"
	rows[0].Alternatives[1].Conditions[1].Thresholds.TransactionValue = ptr("1")
	rows[0].Alternatives[1].Conditions[0].OthersFrom[0] = "99"
	rows[0].Alternatives[0].Label = "x"
	fresh := rs.Rows()
	if fresh[0].ID == "x" || *fresh[0].Alternatives[1].Conditions[1].Thresholds.TransactionValue != "60" ||
		fresh[0].Alternatives[1].Conditions[0].OthersFrom[0] != "820239" || fresh[0].Alternatives[0].Label == "x" {
		t.Fatal("Rows() leaked internal state")
	}
	out := mustCheck(t, rs, base("8202310000", nil))
	out.Limits[0] = "mutated"
	if mustCheck(t, rs, base("8202310000", nil)).Limits[0] == "mutated" || Limits()[0] == "mutated" {
		t.Fatal("Limits shared between outputs")
	}
}

func ptr(s string) *string { return &s }

func TestDuplicateAlternativeIDRejected(t *testing.T) {
	bad := strings.Replace(syntheticRules, `{"id":"a2","label":"rvc no shift"`, `{"id":"a1","label":"rvc no shift"`, 1)
	if _, err := LoadRules([]byte(bad)); err == nil || !strings.Contains(err.Error(), "duplicate alternative id") {
		t.Fatalf("err = %v", err)
	}
}
