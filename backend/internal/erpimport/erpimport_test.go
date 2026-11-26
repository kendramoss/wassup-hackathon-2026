package erpimport

import (
	"encoding/csv"
	"os"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

func readFixture(t *testing.T, name string) [][]string {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func byID(items []Item) map[string]Item {
	m := map[string]Item{}
	for _, it := range items {
		m[it.ExternalID] = it
	}
	return m
}

func TestGenericFixture(t *testing.T) {
	res, err := Parse(Generic, readFixture(t, "generic.csv"))
	if err != nil {
		t.Fatal(err)
	}
	items := byID(res.Items)
	if len(res.Items) != 2 || res.Lines != 6 {
		t.Fatalf("items %d lines %d: %+v", len(res.Items), res.Lines, res)
	}
	fl := items["FL-50"]
	if fl.Name != "LED floodlight" || fl.HTS != "9405.42.8440" || fl.Product.Description != "50 W outdoor floodlight" {
		t.Fatalf("FL-50 header: %+v", fl)
	}
	if len(fl.Product.Components) != 3 || fl.Product.Components[1].HTS != "8504.40.9580" || fl.Product.Components[2].MadeIn != "Mexico" {
		t.Fatalf("FL-50 parts: %+v", fl.Product.Components)
	}
	if fl.Product.Plants[0].Country != "Mexico" || len(fl.Product.Operations) != 1 {
		t.Fatalf("FL-50 plant: %+v", fl.Product.Plants)
	}
	if err := fl.Product.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(res.Problems) != 1 || res.Problems[0].ExternalID != "BAD-1" || res.Problems[0].Row != 7 {
		t.Fatalf("problems: %+v", res.Problems)
	}
}

func TestSAPFixture(t *testing.T) {
	res, err := Parse(SAPS4, readFixture(t, "sap_s4.csv"))
	if err != nil {
		t.Fatal(err)
	}
	items := byID(res.Items)
	hb, ok := items["100045"]
	if !ok || len(res.Problems) != 0 {
		t.Fatalf("items %+v problems %+v", res.Items, res.Problems)
	}
	if hb.Name != "Hand blender" || hb.HTS != "8509.40.00" || hb.Product.Plants[0].Country != "Germany" {
		t.Fatalf("header: %+v", hb)
	}
	// The housing appears on two BOM positions; it is one component.
	if len(hb.Product.Components) != 3 {
		t.Fatalf("components: %+v", hb.Product.Components)
	}
	want := []domain.Component{{Name: "Motor unit", MadeIn: "China", HTS: "8501.31.00"}, {Name: "Stainless shaft", MadeIn: "Germany", HTS: "7326.90.98"}, {Name: "Housing", MadeIn: "Poland", HTS: "3926.90.97"}}
	for i, c := range want {
		if g := hb.Product.Components[i]; g.Name != c.Name || g.MadeIn != c.MadeIn || g.HTS != c.HTS {
			t.Errorf("component %d = %+v, want %+v", i, hb.Product.Components[i], c)
		}
	}
	if k := items["100046"]; k.HTS != "" || len(k.Product.Components) != 1 {
		t.Fatalf("kettle: %+v", k)
	}
}

func TestNetSuiteFixture(t *testing.T) {
	res, err := Parse(NetSuite, readFixture(t, "netsuite.csv"))
	if err != nil || len(res.Items) != 1 || len(res.Problems) != 0 {
		t.Fatalf("%v %+v", err, res)
	}
	sc := res.Items[0]
	if sc.ExternalID != "ASM-77" || sc.Name != "Electric scooter" || sc.HTS != "8711.60.0050" || sc.Product.Plants[0].Country != "United States" {
		t.Fatalf("scooter: %+v", sc)
	}
	if len(sc.Product.Components) != 3 || sc.Product.Components[0].Name != "Lithium battery pack" || sc.Product.Components[1].MadeIn != "Taiwan" {
		t.Fatalf("parts: %+v", sc.Product.Components)
	}
}

func TestHeaderErrors(t *testing.T) {
	if _, err := Parse("oracle", [][]string{{"a"}, {"b"}}); err == nil {
		t.Fatal("unknown format accepted")
	}
	if _, err := Parse(Generic, [][]string{{"product_id", "product_name"}, {"1", "x"}}); err == nil || !strings.Contains(err.Error(), "assembled_in") {
		t.Fatalf("missing columns: %v", err)
	}
	if _, err := Parse(SAPS4, [][]string{{"Material", "Material Description", "Plant Country", "Country of Origin"}, {"1", "x", "DE", "CN"}}); err == nil || !strings.Contains(err.Error(), "component") {
		t.Fatalf("SAP without component columns: %v", err)
	}
	if _, err := Parse(Generic, [][]string{{"product_id"}}); err == nil {
		t.Fatal("header only accepted")
	}
}

func TestRowProblemsKeepOtherAssemblies(t *testing.T) {
	rows := [][]string{
		{"product_id", "product_name", "assembled_in", "component", "made_in", "component_hts", "product_hts"},
		{"A", "Alpha", "Mexico", "Gear", "China", "8483.40", ""},
		{"A", "Alpha", "Canada", "Shaft", "China", "", ""},    // assembly country changes: A dropped
		{"B", "Beta", "Mexico", "Board", "ZZ", "", ""},        // unknown country code: B dropped
		{"C", "Gamma", "Mexico", "Cable", "China", "abc", ""}, // bad code: C dropped
		{"D", "Delta", "Mexico", "Spring", "China", "", "12"}, // bad product code: D dropped
		{"E", "Echo", "Mexico", "Nut", "China", "7318.16", ""},
		{"", "Nameless", "Mexico", "Nut", "China", "", ""},
	}
	res, err := Parse(Generic, rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Items[0].ExternalID != "E" {
		t.Fatalf("items: %+v", res.Items)
	}
	ids := map[string]bool{}
	for _, p := range res.Problems {
		ids[p.ExternalID] = true
		if p.Row == 0 {
			t.Errorf("problem without a row: %+v", p)
		}
	}
	for _, id := range []string{"A", "B", "C", "D", ""} {
		if !ids[id] {
			t.Errorf("no problem reported for %q: %+v", id, res.Problems)
		}
	}
}

func TestCleanHTS(t *testing.T) {
	for in, want := range map[string]string{"8504409580": "8504.40.9580", "8504.40.9580": "8504.40.9580", "8504 40 95": "8504.40.95", "850440": "8504.40", "8504": "8504", "": ""} {
		got, ok := cleanHTS(in)
		if !ok || got != want {
			t.Errorf("%q = %q %v, want %q", in, got, ok, want)
		}
	}
	for _, bad := range []string{"85", "850", "85044", "8504409580123", "8504-4x", "abcd"} {
		if _, ok := cleanHTS(bad); ok {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestFromJSON(t *testing.T) {
	var items []JSONItem
	ok := JSONItem{ExternalID: "J1", Name: "Pump", AssembledIn: "MX", HTS: "8413.70"}
	ok.Components = append(ok.Components, struct {
		Name   string `json:"name"`
		MadeIn string `json:"made_in"`
		HTS    string `json:"hts"`
	}{"Impeller", "CN", "8413.91"})
	items = append(items, ok, ok, JSONItem{ExternalID: "J2", Name: "No parts", AssembledIn: "Mexico"})
	res := FromJSON(items)
	if len(res.Items) != 1 || res.Items[0].Product.Plants[0].Country != "Mexico" || res.Items[0].Product.Components[0].MadeIn != "China" {
		t.Fatalf("items: %+v", res.Items)
	}
	if len(res.Problems) != 2 {
		t.Fatalf("problems: %+v", res.Problems)
	}
}

func TestFingerprintIgnoresComponentOrder(t *testing.T) {
	a := build("X", "", "", "Mexico", []domain.Component{{Name: "a", MadeIn: "China"}, {Name: "b", MadeIn: "Mexico"}})
	b := build("X", "", "", "Mexico", []domain.Component{{Name: "b", MadeIn: "Mexico"}, {Name: "a", MadeIn: "China"}})
	c := build("X", "", "", "Mexico", []domain.Component{{Name: "a", MadeIn: "Vietnam"}, {Name: "b", MadeIn: "Mexico"}})
	if Fingerprint(a) != Fingerprint(b) || Fingerprint(a) == Fingerprint(c) {
		t.Fatal("fingerprint order or change detection wrong")
	}
}

func TestFacts(t *testing.T) {
	p := build("desk fan", "A 30 cm fan.", "", "Vietnam", []domain.Component{{Name: "Motor", MadeIn: "China"}, {Name: "Blade", MadeIn: "Vietnam"}, {Name: "Grille", MadeIn: "China"}})
	got := Facts(p)
	for _, want := range []string{"The merchandise under consideration is desk fan.", "A 30 cm fan.", "assembled in Vietnam", "Made in China: Motor, Grille.", "Made in Vietnam: Blade."} {
		if !strings.Contains(got, want) {
			t.Errorf("facts missing %q:\n%s", want, got)
		}
	}
	if !strings.HasSuffix(got, "\n") || strings.HasSuffix(got, "\n\n") {
		t.Errorf("facts must end with one newline: %q", got)
	}
}
