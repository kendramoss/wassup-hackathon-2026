package usmca

import (
	"strings"
	"testing"
)

func ext(t *testing.T) *RuleSet {
	t.Helper()
	rs, err := ExtendedRules()
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func run(t *testing.T, rs *RuleSet, in Input) Output {
	t.Helper()
	out, err := rs.Check(in)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestExtendedRulesLoad(t *testing.T) {
	rs := ext(t)
	if n := len(rs.Rows()); n < 1000 {
		t.Fatalf("extended rows = %d, want the generated GN 11(o) rows", n)
	}
	// The pilot row wins for its own code.
	out := run(t, rs, base("820231", nil))
	if out.RowID != "gn11o-8202.31" {
		t.Fatalf("8202.31 row = %q, want the pilot row", out.RowID)
	}
}

// One test per encoded GN 11(o) form, on real generated rows.
func TestGeneratedRuleForms(t *testing.T) {
	rs := ext(t)
	cases := []struct {
		name, good, row string
		in              func(Input) Input
		mats            []Material
		want            RuleStatus
	}{
		{"heading shift met", "270400", "gn11o-27-2-2704", nil, nonOrig("2701"), RuleMet},
		{"heading shift not met", "270400", "gn11o-27-2-2704", nil, nonOrig("2704"), RuleNotMet},
		{"chapter shift on a range row", "152200", "gn11o-15-3-1521-1522", nil, nonOrig("1521"), RuleNotMet},
		{"outside that group: inside the group", "722000", "gn11o-72-9-7218-7222", nil, nonOrig("7219"), RuleNotMet},
		{"outside that group: outside the group", "722000", "gn11o-72-9-7218-7222", nil, nonOrig("7206"), RuleMet},
		{"except from a tariff item", "841430", "gn11o-84-44-841430", nil, nonOrig("84149030"), RuleNotMet},
		{"except from another tariff item is fine", "841430", "gn11o-84-44-841430", nil, nonOrig("84149010"), RuleMet},
		{"except from a heading range", "400911", "gn11o-40-5-400911", nil, nonOrig("4011"), RuleNotMet},
		{"from a subheading plus RVC: met", "840310", "gn11o-84-5-840310", func(in Input) Input { return withRVC(in, "transaction_value", "100", "", "30") }, nonOrig("840390"), RuleMet},
		{"from a subheading plus RVC: below", "840310", "gn11o-84-5-840310", func(in Input) Input { return withRVC(in, "transaction_value", "100", "", "50") }, nonOrig("840390"), RuleNotMet},
		{"from a subheading plus RVC: no method", "840310", "gn11o-84-5-840310", nil, nonOrig("840390"), RuleNeedsInformation},
		{"no change plus RVC", "852910", "gn11o-85-65-852910", func(in Input) Input { return withRVC(in, "net_cost", "", "100", "40") }, nonOrig("852910"), RuleMet},
		{"product condition stays unknown", "180610", "gn11o-18-3-180610", nil, nonOrig("1701"), RuleNeedsInformation},
		{"range row lookup", "940520", "gn11o-94-8-940510-940540", nil, nonOrig("7616"), RuleMet},
	}
	for _, c := range cases {
		in := base(c.good, c.mats)
		if c.in != nil {
			in = c.in(in)
		}
		out := run(t, rs, in)
		if out.RowID != c.row || out.Status != c.want {
			t.Errorf("%s: row %q status %s, want %q %s", c.name, out.RowID, out.Status, c.row, c.want)
		}
	}
}

func TestUnsupportedRows(t *testing.T) {
	rs := ext(t)
	for _, good := range []string{"870600", "870829"} {
		out := run(t, rs, base(good, nonOrig("7208")))
		if out.Status != RuleNotSupported || len(out.UnsupportedReasons) == 0 {
			t.Errorf("%s: status %s, want not_supported with a reason", good, out.Status)
		}
	}
}

func TestPilotDefaultUnchanged(t *testing.T) {
	SetExtended(false)
	out, err := Check(base("270400", nonOrig("2701")))
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != RuleNotSupported {
		t.Fatalf("flag off: status %s, want not_supported (pilot rows only)", out.Status)
	}
	SetExtended(true)
	defer SetExtended(false)
	out, err = Check(base("270400", nonOrig("2701")))
	if err != nil || out.Status != RuleMet {
		t.Fatalf("flag on: status %s, %v; want met", out.Status, err)
	}
}

func deMinimisInput(value string) Input {
	m := mat("84149030", ClaimNonOriginating)
	if value != "" {
		m.Value = amt2(value)
	}
	in := base("841430", []Material{m, mat("7318", ClaimNonOriginating)})
	in.Profile.DeMinimis = ProvisionApplies
	in.DeMinimisBasis = DeMinimisBasisTransactionValue
	in.TransactionValue = amt2("100")
	return in
}

func TestDeMinimis(t *testing.T) {
	rs := ext(t)
	if out := run(t, rs, deMinimisInput("10")); out.Status != RuleMet {
		t.Errorf("10 of 100: %s, want met through de minimis", out.Status)
	}
	if out := run(t, rs, deMinimisInput("10.01")); out.Status != RuleNotMet {
		t.Errorf("10.01 of 100: %s, want not_met", out.Status)
	}
	out := run(t, rs, deMinimisInput(""))
	if out.Status != RuleNeedsInformation || !hasFact(out, "material_value_unknown") {
		t.Errorf("no value: %s %v, want needs_information asking for the value", out.Status, out.MissingFacts)
	}
	in := deMinimisInput("5")
	in.DeMinimisBasis = ""
	if out := run(t, rs, in); out.Status != RuleNeedsInformation || !hasFact(out, "de_minimis_basis_required") {
		t.Errorf("no basis: %s, want needs_information", out.Status)
	}
	// Not invoked: the pilot behaviour, no rescue.
	in = deMinimisInput("5")
	in.Profile.DeMinimis = ProvisionNotApplicable
	if out := run(t, rs, in); out.Status != RuleNotMet {
		t.Errorf("not invoked: %s, want not_met", out.Status)
	}
	// Chapters 1 to 27 are outside what this check models.
	food := base("270400", []Material{{HS: "2704", Originating: ClaimNonOriginating, Value: amt2("1")}})
	food.Profile.DeMinimis = ProvisionApplies
	if out := run(t, rs, food); out.Status != RuleNotSupported {
		t.Errorf("chapter 27 de minimis: %s, want not_supported", out.Status)
	}
	// The pilot rule set still refuses de minimis.
	pilot, _ := DefaultRules()
	if out := run(t, pilot, deMinimisInput("5")); out.Status != RuleNotSupported {
		t.Errorf("pilot de minimis: %s, want not_supported", out.Status)
	}
}

func hasFact(out Output, code string) bool {
	for _, f := range out.MissingFacts {
		if f.Code == code {
			return true
		}
	}
	return false
}

func partsInput(basis string, produced bool, vnm string) Input {
	in := withRVC(base("841430", nonOrig("841430")), "transaction_value", "100", "", vnm)
	in.Profile.PartsRoute = ProvisionApplies
	in.Profile.PartsRouteBasis = basis
	in.Profile.ProducedInUSMCA = produced
	return in
}

func TestPartsRoute(t *testing.T) {
	rs := ext(t)
	if out := run(t, rs, partsInput(PartsBasisSameClassification, true, "40")); out.Status != RuleMet {
		t.Errorf("same classification, RVC 60: %s, want met", out.Status)
	}
	if out := run(t, rs, partsInput(PartsBasisSameClassification, true, "41")); out.Status != RuleNotMet {
		t.Errorf("RVC 59: %s, want not_met", out.Status)
	}
	if out := run(t, rs, partsInput(PartsBasisSameClassification, false, "40")); out.Status != RuleNeedsInformation {
		t.Errorf("production unconfirmed: %s, want needs_information", out.Status)
	}
	if out := run(t, rs, partsInput("", true, "40")); out.Status != RuleNeedsInformation || !hasFact(out, "parts_route_basis_required") {
		t.Errorf("no basis: %s, want needs_information", out.Status)
	}
	in := partsInput(PartsBasisSameClassification, true, "40")
	in.Materials = nonOrig("7318")
	// 7318 shifts, so alternative A is met anyway; the parts-route alternative
	// itself finds no part sharing the classification.
	out := run(t, rs, in)
	for _, a := range out.Alternatives {
		if a.ID == "parts_route" && a.Result != OutcomeNotSatisfied {
			t.Errorf("parts route with no shared part: %s, want not_satisfied", a.Result)
		}
	}
	apparel := partsInput(PartsBasisUnassembled, true, "40")
	apparel.GoodHS = "610910"
	if out := run(t, rs, apparel); out.Status != RuleNotSupported {
		t.Errorf("chapter 61 parts route: %s, want not_supported", out.Status)
	}
}

func TestDeemedOriginating(t *testing.T) {
	rs := ext(t)
	in := base("847170", nonOrig("847170"))
	if out := run(t, rs, in); out.Status != RuleNeedsInformation || !hasFact(out, "imported_from_usmca_unconfirmed") {
		t.Errorf("GN 11(p) unconfirmed: %s, want needs_information", out.Status)
	}
	in.Profile.ImportedFromUSMCA = true
	out := run(t, rs, in)
	if out.Status != RuleMet {
		t.Fatalf("GN 11(p) storage unit from Mexico: %s, want met", out.Status)
	}
	last := out.Alternatives[len(out.Alternatives)-1]
	if last.ID != "gn11p" || !strings.Contains(last.Label, "Storage units") {
		t.Errorf("deemed alternative = %+v", last)
	}
	// Not listed: no GN 11(p) alternative.
	for _, a := range run(t, rs, base("841430", nonOrig("7318"))).Alternatives {
		if a.ID == "gn11p" {
			t.Error("841430 is not a GN 11(p) good")
		}
	}
}

func TestInRange(t *testing.T) {
	cases := []struct {
		code, spec string
		want       Tri
	}{
		{"84012000", "840110-840130", True},
		{"840140", "840110-840130", False},
		{"8401", "840110-840130", Unknown3},
		{"8402", "840110-840130", False},
		{"7219", "7218-7222", True},
		{"72", "7218-7222", Unknown3},
		{"", "7218-7222", Unknown3},
		{"84149030", "84149030", True},
	}
	for _, c := range cases {
		if got := inRange(c.code, c.spec); got != c.want {
			t.Errorf("inRange(%q, %q) = %v, want %v", c.code, c.spec, got, c.want)
		}
	}
}
