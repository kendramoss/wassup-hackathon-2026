package usmca

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
)

func synth(t *testing.T) *RuleSet {
	t.Helper()
	rs, err := LoadRules([]byte(syntheticRules))
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func fullProfile() Profile {
	na := ProvisionNotApplicable
	return Profile{SimpleGoods: true, MaterialsComplete: true,
		Sets: na, AccessoriesPacking: na, Fungibles: na, IntermediateMaterials: na,
		CostAveraging: na, DeMinimis: na, OutsidePartyOperations: na, SpecialValuation: na, PartsRoute: na}
}

func mat(hs string, c OriginClaim) Material { return Material{HS: hs, Originating: c} }

func nonOrig(hs ...string) []Material {
	var out []Material
	for _, h := range hs {
		out = append(out, mat(h, ClaimNonOriginating))
	}
	return out
}

func base(hs string, mats []Material) Input {
	return Input{Destination: "US", Regime: "usmca", GoodHS: hs, Materials: mats, Profile: fullProfile()}
}

func withRVC(in Input, method, tv, nc, vnm string) Input {
	in.RVCMethod = method
	if tv != "" {
		in.TransactionValue = amt2(tv)
	}
	if nc != "" {
		in.NetCost = amt2(nc)
	}
	if vnm != "" {
		in.VNM = amt2(vnm)
	}
	return in
}

func amt2(s string) Amount {
	a, err := ParseAmount(s)
	if err != nil {
		panic(err)
	}
	return a
}

func mustCheck(t *testing.T, rs *RuleSet, in Input) Output {
	t.Helper()
	out, err := rs.Check(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return out
}

func hasMissing(o Output, field string) bool {
	for _, m := range o.MissingFacts {
		if m.Field == field {
			return true
		}
	}
	return false
}

func TestShiftLevelsAreDistinct(t *testing.T) {
	rs := synth(t)
	tests := []struct {
		name string
		good string
		mats []Material
		want RuleStatus
	}{
		// chapter: same chapter other heading fails; other chapter passes
		{"chapter other heading same chapter", "770100", nonOrig("770200"), RuleNotMet},
		{"chapter other chapter", "770100", nonOrig("880100"), RuleMet},
		{"chapter mixed", "770100", nonOrig("880100", "770299"), RuleNotMet},
		// heading
		{"heading same heading", "770210", nonOrig("770290"), RuleNotMet},
		{"heading other heading same chapter", "770210", nonOrig("770390"), RuleMet},
		{"heading 4-digit material other heading", "770210", nonOrig("7703"), RuleMet},
		{"heading 4-digit material same heading", "770210", nonOrig("7702"), RuleNotMet},
		// subheading
		{"subheading same code", "770331", nonOrig("770331"), RuleNotMet},
		{"subheading sibling subheading", "770331", nonOrig("770339"), RuleMet},
		{"subheading other heading", "770331", nonOrig("770490"), RuleMet},
		{"subheading 4-digit same heading unknown", "770331", nonOrig("7703"), RuleNeedsInformation},
		{"subheading 4-digit other heading", "770331", nonOrig("7704"), RuleMet},
		// tariff item
		{"item same 8 digit", "77041010", nonOrig("77041010"), RuleNotMet},
		{"item sibling item same subheading", "77041010", nonOrig("77041090"), RuleMet},
		{"item 6-digit same subheading unknown", "77041010", nonOrig("770410"), RuleNeedsInformation},
		{"item 6-digit other subheading", "77041010", nonOrig("770420"), RuleMet},
		{"item 10-digit sibling", "7704101010", nonOrig("7704109010"), RuleMet},
		{"item 10-digit same item different stat suffix", "7704101010", nonOrig("7704101099"), RuleNotMet},
		// no materials with complete list: nothing to shift
		{"no non-originating materials", "770100", nil, RuleMet},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := mustCheck(t, rs, base(tc.good, tc.mats))
			if out.Status != tc.want {
				t.Fatalf("status = %s, want %s (%+v)", out.Status, tc.want, out.Alternatives)
			}
		})
	}
}

func TestExcludePrefixes(t *testing.T) {
	rs := synth(t)
	if got := mustCheck(t, rs, base("771010", nonOrig("771190"))).Status; got != RuleNotMet {
		t.Fatalf("excluded sibling should fail shift, got %s", got)
	}
	if got := mustCheck(t, rs, base("771010", nonOrig("771100"))).Status; got != RuleMet {
		t.Fatalf("other heading should pass, got %s", got)
	}
}

func TestMostSpecificRowWins(t *testing.T) {
	rs := synth(t)
	// Parent row 7709 (heading shift) would pass 770300; child 770910 (chapter shift) fails.
	out := mustCheck(t, rs, base("77091000", nonOrig("770300")))
	if out.Status != RuleNotMet || out.RowID != "S-CHILD" {
		t.Fatalf("got %s row %s", out.Status, out.RowID)
	}
	out = mustCheck(t, rs, base("77099000", nonOrig("770300")))
	if out.Status != RuleMet || out.RowID != "S-PARENT" {
		t.Fatalf("got %s row %s", out.Status, out.RowID)
	}
}

func TestSiblingParts(t *testing.T) {
	rs := synth(t)
	tests := []struct {
		name string
		mats []Material
		want RuleStatus
	}{
		{"sibling plus set plus other chapter", nonOrig("770539", "770590", "880100"), RuleMet},
		{"sibling only", nonOrig("770539"), RuleMet},
		{"no sibling present", nonOrig("770590"), RuleNotMet},
		{"no sibling, only other chapter", nonOrig("880100"), RuleNotMet},
		{"no materials", nil, RuleNotMet},
		{"sibling plus same-chapter other heading", nonOrig("770539", "770611"), RuleNotMet},
		{"sibling plus unknown-hs other material", append(nonOrig("770539"), mat("", ClaimNonOriginating)), RuleNeedsInformation},
		{"unknown-origin sibling cannot satisfy existence", []Material{mat("770539", ClaimUnknown)}, RuleNeedsInformation},
		{"originating sibling does not count", []Material{mat("770539", ClaimOriginating)}, RuleNotMet},
		{"originating bad other ignored", []Material{mat("770539", ClaimNonOriginating), mat("770611", ClaimOriginating)}, RuleMet},
		{"sibling 4-digit hs is imprecise", nonOrig("7705"), RuleNotMet}, // alt a2 fails (same heading); a1 unknown -> see below
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := mustCheck(t, rs, withRVC(base("770531", tc.mats), "transaction_value", "100", "", "90"))
			if tc.name == "sibling 4-digit hs is imprecise" {
				if out.Status != RuleNeedsInformation {
					t.Fatalf("status = %s", out.Status)
				}
				return
			}
			if out.Status != tc.want {
				t.Fatalf("status = %s, want %s: %+v", out.Status, tc.want, out.Alternatives)
			}
		})
	}
	// missing fact names the offending material
	out := mustCheck(t, rs, base("770531", append(nonOrig("770539"), mat("", ClaimNonOriginating))))
	if !hasMissing(out, "materials[1].hs") {
		t.Fatalf("missing facts: %+v", out.MissingFacts)
	}
}

func TestForbiddenInputs(t *testing.T) {
	rs := synth(t)
	hi := func(mats []Material) Input {
		return withRVC(base("770800", mats), "transaction_value", "100", "", "10")
	}
	if got := mustCheck(t, rs, hi(nonOrig("770100"))).Status; got != RuleNotMet {
		t.Fatalf("forbidden same-chapter input must fail despite 90%% RVC, got %s", got)
	}
	if got := mustCheck(t, rs, hi(nonOrig("880100"))).Status; got != RuleMet {
		t.Fatalf("got %s", got)
	}
	if got := mustCheck(t, rs, hi([]Material{mat("", ClaimNonOriginating)})).Status; got != RuleNeedsInformation {
		t.Fatalf("unknown HS must be indeterminate, got %s", got)
	}
	// unknown HS but RVC definitely fails -> False dominates
	low := withRVC(base("770800", []Material{mat("", ClaimNonOriginating)}), "transaction_value", "100", "", "70")
	if got := mustCheck(t, rs, low).Status; got != RuleNotMet {
		t.Fatalf("got %s", got)
	}
	// forbidden input that is claimed originating does not block
	if got := mustCheck(t, rs, hi([]Material{mat("770100", ClaimOriginating)})).Status; got != RuleMet {
		t.Fatalf("got %s", got)
	}
	// forbidden input with unknown origin -> needs information
	if got := mustCheck(t, rs, hi([]Material{mat("770100", ClaimUnknown)})).Status; got != RuleNeedsInformation {
		t.Fatalf("got %s", got)
	}
	// sibling-row alt a2: same-heading forbidden but not same-chapter-other-heading
	in := withRVC(base("770531", nonOrig("770611")), "transaction_value", "100", "", "10")
	if got := mustCheck(t, rs, in).Status; got != RuleMet {
		t.Fatalf("different heading input, high rvc: got %s", got)
	}
}

func TestOrAndMissingFacts(t *testing.T) {
	rs := synth(t)
	// shift alt satisfied, no RVC inputs at all -> met
	out := mustCheck(t, rs, base("770600", nonOrig("880100")))
	if out.Status != RuleMet || out.RVC != nil || len(out.MissingFacts) != 0 {
		t.Fatalf("%+v", out)
	}
	// no-shift RVC alt satisfied, material hs unknown and shift alt unknown -> met
	in := withRVC(base("770600", []Material{mat("", ClaimNonOriginating)}), "transaction_value", "100", "", "40")
	if got := mustCheck(t, rs, in).Status; got != RuleMet {
		t.Fatalf("got %s", got)
	}
	// both unresolved
	out = mustCheck(t, rs, base("770600", []Material{mat("", ClaimNonOriginating)}))
	if out.Status != RuleNeedsInformation || !hasMissing(out, "rvc_method") || !hasMissing(out, "materials[0].hs") {
		t.Fatalf("%+v", out.MissingFacts)
	}
	// shift fails, rvc inputs missing
	out = mustCheck(t, rs, base("770600", nonOrig("770699")))
	if out.Status != RuleNeedsInformation || !hasMissing(out, "rvc_method") {
		t.Fatalf("%+v", out)
	}
	// shift fails and rvc fails -> not met
	in = withRVC(base("770600", nonOrig("770699")), "transaction_value", "100", "", "41")
	if got := mustCheck(t, rs, in).Status; got != RuleNotMet {
		t.Fatalf("got %s", got)
	}
	// method given, denominator for that method missing
	in = withRVC(base("770600", nonOrig("770699")), "net_cost", "100", "", "10")
	out = mustCheck(t, rs, in)
	if out.Status != RuleNeedsInformation || !hasMissing(out, "net_cost") {
		t.Fatalf("%+v", out)
	}
	// vnm omitted is unknown, not zero
	in = withRVC(base("770600", nonOrig("770699")), "transaction_value", "100", "", "")
	out = mustCheck(t, rs, in)
	if out.Status != RuleNeedsInformation || !hasMissing(out, "vnm") {
		t.Fatalf("%+v", out)
	}
}

func TestRVCBoundaries(t *testing.T) {
	rs := synth(t)
	fail := nonOrig("770699") // shift alternative fails
	run := func(method, tv, nc, vnm string) Output {
		return mustCheck(t, rs, withRVC(base("770600", fail), method, tv, nc, vnm))
	}
	if got := run("transaction_value", "100", "", "40").Status; got != RuleMet {
		t.Fatalf("exact TV threshold 60 should pass, got %s", got)
	}
	out := run("transaction_value", "100", "", "40.000000000001")
	if out.Status != RuleNotMet || out.RVC.Display != "60.00" {
		t.Fatalf("display 60.00 must not pass: %+v", out.RVC)
	}
	if got := run("net_cost", "", "100", "50").Status; got != RuleMet {
		t.Fatalf("exact NC threshold 50 should pass, got %s", got)
	}
	if got := run("net_cost", "", "100", "50.000000000001").Status; got != RuleNotMet {
		t.Fatalf("got %s", got)
	}
	// separate thresholds: 55% passes NC (50) but not TV (60)
	if got := run("net_cost", "", "100", "45").Status; got != RuleMet {
		t.Fatalf("got %s", got)
	}
	if got := run("transaction_value", "100", "", "45").Status; got != RuleNotMet {
		t.Fatalf("got %s", got)
	}
	// the other method's value is ignored
	if got := run("transaction_value", "100", "1", "40").Status; got != RuleMet {
		t.Fatalf("got %s", got)
	}
	// zero known VNM is 100%
	out = run("transaction_value", "100", "", "0")
	if out.Status != RuleMet || out.RVC.Exact != "100" {
		t.Fatalf("%+v", out)
	}
}

func TestRVCMethodWithoutThreshold(t *testing.T) {
	rs := synth(t)
	in := withRVC(base("770700", nil), "net_cost", "", "100", "0")
	out := mustCheck(t, rs, in)
	if out.Status != RuleNotMet {
		t.Fatalf("net_cost has no threshold in this row: %s", out.Status)
	}
	in = withRVC(base("770700", nil), "transaction_value", "8", "", "3")
	if got := mustCheck(t, rs, in).Status; got != RuleMet { // 62.5 exactly
		t.Fatalf("got %s", got)
	}
	out = mustCheck(t, rs, base("770700", nil))
	if out.Status != RuleNeedsInformation || !hasMissing(out, "rvc_method") {
		t.Fatalf("%+v", out)
	}
}

func TestZeroDenominatorAndNegativeRVC(t *testing.T) {
	rs := synth(t)
	in := withRVC(base("770600", nonOrig("770699")), "transaction_value", "0", "", "0")
	out := mustCheck(t, rs, in)
	if out.Status != RuleNeedsInformation || out.RVC.Status != StatusUndefinedDenominator || out.RVC.Exact != "" {
		t.Fatalf("%+v", out)
	}
	if len(out.MissingFacts) == 0 || out.MissingFacts[len(out.MissingFacts)-1].Code != "rvc_undefined" {
		t.Fatalf("%+v", out.MissingFacts)
	}
	// zero denominator does not block an independent shift alternative
	in = withRVC(base("770600", nonOrig("880100")), "transaction_value", "0", "", "0")
	if got := mustCheck(t, rs, in).Status; got != RuleMet {
		t.Fatalf("got %s", got)
	}
	// negative RVC is reported, not clamped, and does not block the shift alt
	in = withRVC(base("770600", nonOrig("880100")), "transaction_value", "10", "", "20")
	out = mustCheck(t, rs, in)
	if out.Status != RuleMet || out.RVC.Exact != "-100" || out.RVC.Display != "-100.00" {
		t.Fatalf("%+v", out.RVC)
	}
	// negative RVC alone fails the RVC alt
	in = withRVC(base("770600", nonOrig("770699")), "transaction_value", "10", "", "20")
	if got := mustCheck(t, rs, in).Status; got != RuleNotMet {
		t.Fatalf("got %s", got)
	}
}

func TestOriginClaims(t *testing.T) {
	rs := synth(t)
	if got := mustCheck(t, rs, base("770100", []Material{mat("770200", ClaimOriginating)})).Status; got != RuleMet {
		t.Fatalf("originating material is ignored by shift, got %s", got)
	}
	if got := mustCheck(t, rs, base("770100", []Material{mat("770200", ClaimUnknown)})).Status; got != RuleNeedsInformation {
		t.Fatalf("got %s", got)
	}
	if got := mustCheck(t, rs, base("770100", []Material{mat("880100", ClaimUnknown)})).Status; got != RuleMet {
		t.Fatalf("unknown-origin material that would pass anyway: got %s", got)
	}
	out := mustCheck(t, rs, base("770100", []Material{{HS: "770200"}})) // omitted claim == unknown
	if out.Status != RuleNeedsInformation || !hasMissing(out, "materials[0].originating") {
		t.Fatalf("%+v", out)
	}
}

func TestMaterialsListCompleteness(t *testing.T) {
	rs := synth(t)
	in := base("770100", nonOrig("880100"))
	in.Profile.MaterialsComplete = false
	out := mustCheck(t, rs, in)
	if out.Status != RuleNeedsInformation || !hasMissing(out, "profile.materials_list_complete") {
		t.Fatalf("%+v", out)
	}
	// a listed failing material is a definitive failure of the shift alternative
	in = base("770100", nonOrig("770200"))
	in.Profile.MaterialsComplete = false
	out = mustCheck(t, rs, in)
	if out.Status != RuleNotMet { // a listed failing material cannot be cured by more materials
		t.Fatalf("%+v", out)
	}
	if out.Alternatives[0].Result != OutcomeNotSatisfied {
		t.Fatalf("%+v", out.Alternatives)
	}
}

func TestProfileGates(t *testing.T) {
	rs := synth(t)
	good := func() Input {
		return withRVC(base("770600", nonOrig("880100")), "transaction_value", "100", "", "10")
	}
	provs := map[string]func(*Profile){
		"sets":                     func(p *Profile) { p.Sets = ProvisionApplies },
		"accessories_packing":      func(p *Profile) { p.AccessoriesPacking = ProvisionApplies },
		"fungibles":                func(p *Profile) { p.Fungibles = ProvisionApplies },
		"intermediate_materials":   func(p *Profile) { p.IntermediateMaterials = ProvisionApplies },
		"cost_averaging":           func(p *Profile) { p.CostAveraging = ProvisionApplies },
		"de_minimis":               func(p *Profile) { p.DeMinimis = ProvisionApplies },
		"outside_party_operations": func(p *Profile) { p.OutsidePartyOperations = ProvisionApplies },
		"special_valuation":        func(p *Profile) { p.SpecialValuation = ProvisionApplies },
		"parts_route":              func(p *Profile) { p.PartsRoute = ProvisionApplies },
	}
	for name, set := range provs {
		in := good()
		set(&in.Profile)
		out := mustCheck(t, rs, in)
		if out.Status != RuleNotSupported || len(out.UnsupportedReasons) == 0 || !strings.Contains(strings.Join(out.UnsupportedReasons, " "), name) {
			t.Fatalf("%s: %+v", name, out)
		}
		if out.RVC == nil || out.RVC.Exact != "90" {
			t.Fatalf("%s: arithmetic should still be shown: %+v", name, out.RVC)
		}
		if len(out.Alternatives) != 0 {
			t.Fatalf("%s: rule must not be evaluated", name)
		}
	}
	// unknown applicability must not become positive
	in := good()
	in.Profile.Fungibles = ""
	out := mustCheck(t, rs, in)
	if out.Status != RuleNeedsInformation || !hasMissing(out, "profile.fungibles") {
		t.Fatalf("%+v", out)
	}
	in = good()
	in.Profile.SimpleGoods = false
	out = mustCheck(t, rs, in)
	if out.Status != RuleNeedsInformation || !hasMissing(out, "profile.simple_goods_confirmed") {
		t.Fatalf("%+v", out)
	}
	// a failing rule with unconfirmed profile is not a definitive not_met either
	in = base("770100", nonOrig("770200"))
	in.Profile.CostAveraging = ""
	if got := mustCheck(t, rs, in).Status; got != RuleNeedsInformation {
		t.Fatalf("got %s", got)
	}
	// applies wins over unconfirmed
	in.Profile.Sets = ProvisionApplies
	if got := mustCheck(t, rs, in).Status; got != RuleNotSupported {
		t.Fatalf("got %s", got)
	}
}

func TestUnsupportedGoods(t *testing.T) {
	rs := synth(t)
	out := mustCheck(t, rs, base("880100", nonOrig("770100")))
	if out.Status != RuleNotSupported || out.RowID != "" || len(out.UnsupportedReasons) == 0 {
		t.Fatalf("%+v", out)
	}
	// good code too short to pick the more specific row
	out = mustCheck(t, rs, base("7705", nil))
	if out.Status != RuleNeedsInformation || !hasMissing(out, "good_hs") {
		t.Fatalf("%+v", out)
	}
	// 4-digit good matching a 4-digit row is fine
	if got := mustCheck(t, rs, base("7701", nonOrig("880100"))).Status; got != RuleMet {
		t.Fatalf("got %s", got)
	}
}

func TestEmbeddedRulesLoad(t *testing.T) {
	rs, err := DefaultRules()
	if err != nil {
		t.Fatal(err)
	}
	if rs.Version() == "" || len(rs.Rows()) != 3 {
		t.Fatalf("version %q rows %d", rs.Version(), len(rs.Rows()))
	}
	out, err := Check(base("8202390000", nil))
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != RuleNotSupported || out.Source.DataVersion != rs.Version() || len(out.Limits) == 0 {
		t.Fatalf("%+v", out)
	}
}

func TestOutputJSON(t *testing.T) {
	rs := synth(t)
	in := withRVC(base("770600", nonOrig("770699")), "transaction_value", "100", "", "40")
	out := mustCheck(t, rs, in)
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"status":"met"`, `"data_version":"synthetic-test-1"`, `"limits"`, `"exact":"60"`} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("missing %s in %s", want, b)
		}
	}
}

func TestInputValidation(t *testing.T) {
	rs := synth(t)
	many := func(n int) []Material {
		m := make([]Material, n)
		for i := range m {
			m[i] = mat("880100", ClaimNonOriginating)
		}
		return m
	}
	bad := map[string]func(*Input){
		"destination":     func(i *Input) { i.Destination = "CA" },
		"destination-lc":  func(i *Input) { i.Destination = "us" },
		"empty dest":      func(i *Input) { i.Destination = "" },
		"regime":          func(i *Input) { i.Regime = "rcep" },
		"hs empty":        func(i *Input) { i.GoodHS = "" },
		"hs alpha":        func(i *Input) { i.GoodHS = "77O100" },
		"hs dotted":       func(i *Input) { i.GoodHS = "7701.00" },
		"hs 5 digits":     func(i *Input) { i.GoodHS = "77010" },
		"hs 2 digits":     func(i *Input) { i.GoodHS = "77" },
		"hs 11 digits":    func(i *Input) { i.GoodHS = "77010000000" },
		"hs space":        func(i *Input) { i.GoodHS = " 770100" },
		"hs non-ascii":    func(i *Input) { i.GoodHS = "７７０１００" },
		"method":          func(i *Input) { i.RVCMethod = "tv" },
		"claim":           func(i *Input) { i.Materials = []Material{{HS: "880100", Originating: "maybe"}} },
		"material hs":     func(i *Input) { i.Materials = []Material{{HS: "88x1", Originating: ClaimNonOriginating}} },
		"material hs odd": func(i *Input) { i.Materials = []Material{{HS: "88011", Originating: ClaimNonOriginating}} },
		"material id":     func(i *Input) { i.Materials = []Material{{ID: strings.Repeat("a", 65)}} },
		"too many":        func(i *Input) { i.Materials = many(MaxMaterials + 1) },
		"provision":       func(i *Input) { i.Profile.Sets = "no" },
	}
	for name, mod := range bad {
		t.Run(name, func(t *testing.T) {
			in := base("770100", nil)
			mod(&in)
			_, err := rs.Check(in)
			if !errors.Is(err, ErrInput) {
				t.Fatalf("err = %v, want ErrInput", err)
			}
			var ie *InputError
			if !errors.As(err, &ie) || ie.Field == "" {
				t.Fatalf("want *InputError with field, got %v", err)
			}
		})
	}
	in := base("770100", many(MaxMaterials))
	if _, err := rs.Check(in); err != nil {
		t.Fatalf("exactly %d materials must be accepted: %v", MaxMaterials, err)
	}
	for _, hs := range []string{"7701", "770100", "77010000", "7701000000"} {
		if _, err := rs.Check(base(hs, nil)); err != nil {
			t.Fatalf("%s: %v", hs, err)
		}
	}
}

func TestDecodeInput(t *testing.T) {
	ok := `{"destination":"US","regime":"usmca","good_hs":"770100","transaction_value":"100","vnm":"0","rvc_method":"transaction_value","materials":[{"hs":"880100","originating":"non_originating"}],"profile":{"simple_goods_confirmed":true}}`
	in, err := DecodeInput([]byte(ok))
	if err != nil {
		t.Fatal(err)
	}
	if !in.TransactionValue.IsKnown() || in.NetCost.IsKnown() || in.VNM.String() != "0" {
		t.Fatalf("%+v", in)
	}
	bad := map[string]string{
		"numeric amount":  `{"destination":"US","regime":"usmca","good_hs":"770100","transaction_value":100}`,
		"negative amount": `{"destination":"US","regime":"usmca","good_hs":"770100","vnm":"-1"}`,
		"exponent":        `{"destination":"US","regime":"usmca","good_hs":"770100","vnm":"1e3"}`,
		"unknown field":   `{"destination":"US","regime":"usmca","good_hs":"770100","surprise":1}`,
		"trailing data":   `{"destination":"US","regime":"usmca","good_hs":"770100"} {}`,
		"not json":        `nope`,
		"hs as number":    `{"destination":"US","regime":"usmca","good_hs":770100}`,
		"huge decimal":    `{"destination":"US","regime":"usmca","good_hs":"770100","vnm":"` + strings.Repeat("9", 40) + `"}`,
	}
	for name, body := range bad {
		if _, err := DecodeInput([]byte(body)); !errors.Is(err, ErrInput) {
			t.Fatalf("%s: err = %v", name, err)
		}
	}
}

func TestLoadRulesRejects(t *testing.T) {
	mut := func(old, new string) []byte { return []byte(strings.Replace(syntheticRules, old, new, 1)) }
	bad := map[string][]byte{
		"not json":          []byte(`{`),
		"unknown field":     mut(`"schema_version": 1,`, `"schema_version": 1, "bogus": 1,`),
		"schema version":    mut(`"schema_version": 1`, `"schema_version": 2`),
		"empty version":     mut(`"data_version": "synthetic-test-1"`, `"data_version": ""`),
		"dup id":            mut(`"id":"S-HD"`, `"id":"S-CH"`),
		"dup code":          mut(`"code":"7702"`, `"code":"7701"`),
		"bad code":          mut(`"code":"7702"`, `"code":"77x2"`),
		"code len":          mut(`"code":"7702"`, `"code":"770"`),
		"bad kind":          mut(`"change_from_chapter"`, `"change_from_anything"`),
		"bad threshold":     mut(`"transaction_value":"60"`, `"transaction_value":"6e1"`),
		"threshold >100":    mut(`"transaction_value":"60"`, `"transaction_value":"101"`),
		"no source excerpt": mut(`"excerpt":"A change from any other chapter."`, `"excerpt":""`),
		"no snapshot rev":   mut(`"revision": "t1"`, `"revision": ""`),
		"bad exclude":       mut(`"771190"`, `"77x190"`),
		"bad level":         mut(`"same_as_good_level":"heading"`, `"same_as_good_level":"section"`),
		"stray field":       mut(`{"kind":"change_from_chapter"}`, `{"kind":"change_from_chapter","prefixes":["77"]}`),
		"rvc no thresholds": mut(`"thresholds":{"transaction_value":"62.5"}`, `"thresholds":{}`),
		"outside chapter":   mut(`"others_outside_chapter":"77"`, `"others_outside_chapter":"7"`),
	}
	for name, data := range bad {
		if _, err := LoadRules(data); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	if _, err := LoadRules([]byte(syntheticRules)); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentCheck(t *testing.T) {
	rs := synth(t)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			in := withRVC(base("770600", nonOrig("770699")), "transaction_value", "100", "", "40")
			if out, err := rs.Check(in); err != nil || out.Status != RuleMet {
				t.Errorf("%v %v", out.Status, err)
			}
		}()
	}
	wg.Wait()
}

func TestInputNotMutated(t *testing.T) {
	rs := synth(t)
	in := withRVC(base("770600", nonOrig("770699")), "transaction_value", "100", "", "40")
	before, _ := json.Marshal(in)
	mustCheck(t, rs, in)
	after, _ := json.Marshal(in)
	if string(before) != string(after) {
		t.Fatal("input mutated")
	}
}
