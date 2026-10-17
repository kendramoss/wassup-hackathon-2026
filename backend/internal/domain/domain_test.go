package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func heatGun() Product {
	return Product{
		Name:   "heat gun",
		Plants: []Plant{{ID: "th", Country: "Thailand"}, {ID: "cn", Country: "China"}},
		Components: []Component{
			{Name: "housing", MadeIn: "Thailand", Plant: "th", Group: "housing_moulding"},
			{Name: "motor", MadeIn: "China", Group: "motor_fan_assembly"},
		},
		Operations: []Operation{{Name: "final assembly", Plant: "th", Group: "final_assembly"}},
	}
}

// wantProblems asserts err is a ValidationError wrapping ErrInvalid whose
// messages contain each fragment; no fragments means err must be nil.
func wantProblems(t *testing.T, err error, fragments ...string) {
	t.Helper()
	if len(fragments) == 0 {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("error %v does not wrap ErrInvalid", err)
	}
	for _, f := range fragments {
		if !strings.Contains(err.Error(), f) {
			t.Errorf("error %q missing %q", err, f)
		}
	}
}

func TestProductValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Product)
		want   []string
	}{
		{"valid", func(*Product) {}, nil},
		{"no name", func(p *Product) { p.Name = " " }, []string{"product name"}},
		{"no components", func(p *Product) { p.Components = nil }, []string{"at least one component"}},
		{"unknown component plant", func(p *Product) { p.Components[0].Plant = "vn" }, []string{`unknown plant "vn"`}},
		{"unknown operation plant", func(p *Product) { p.Operations[0].Plant = "" }, []string{"operations[0]: unknown plant"}},
		{"duplicate plant", func(p *Product) { p.Plants[1].ID = "th" }, []string{`duplicate id "th"`}},
		{"plant without country", func(p *Product) { p.Plants[0].Country = "" }, []string{"plants[0]: country"}},
		{"component without origin", func(p *Product) { p.Components[1].MadeIn = "" }, []string{"components[1]: made_in"}},
		{"several problems at once", func(p *Product) { p.Name = ""; p.Components[0].Name = "" }, []string{"product name", "components[0]: name"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := heatGun()
			tt.mutate(&p)
			wantProblems(t, p.Validate(), tt.want...)
		})
	}
}

func TestCaseValidate(t *testing.T) {
	day := time.Date(2022, 6, 1, 0, 0, 0, 0, time.UTC)
	hg := heatGun()
	bad := heatGun()
	bad.Name = ""
	tests := []struct {
		name string
		c    Case
		want []string
	}{
		{"facts only", Case{Date: day, Facts: "assembled in Thailand"}, nil},
		{"product only", Case{Date: day, Product: &hg}, nil},
		{"no date", Case{Facts: "x"}, []string{"date is required"}},
		{"nothing", Case{Date: day}, []string{"facts or product"}},
		{"facts too long", Case{Date: day, Facts: strings.Repeat("a", MaxFactsRunes+1)}, []string{"max 12000"}},
		{"invalid product", Case{Date: day, Product: &bad}, []string{"product name"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { wantProblems(t, tt.c.Validate(), tt.want...) })
	}
}

func TestRuling(t *testing.T) {
	jan14 := time.Date(2026, 1, 14, 0, 0, 0, 0, time.UTC)
	for _, id := range []string{"N357771", "H334437"} {
		if !ValidRulingID(id) {
			t.Errorf("ValidRulingID(%q) = false", id)
		}
	}
	for _, id := range []string{"", "n357771", "N35777", "X357771", "N3577710"} {
		if ValidRulingID(id) {
			t.Errorf("ValidRulingID(%q) = true", id)
		}
	}
	wantProblems(t, Ruling{ID: "bad"}.Validate(), "not a CBP ruling number", "date is required")

	tests := []struct {
		name string
		r    Ruling
		want bool
	}{
		{"earlier", Ruling{ID: "N350000", Date: jan14.AddDate(0, 0, -1)}, true},
		{"same day is excluded", Ruling{ID: "N350000", Date: jan14}, false},
		{"later", Ruling{ID: "N350000", Date: jan14.AddDate(0, 0, 1)}, false},
		{"the case itself", Ruling{ID: "N357771", Date: jan14.AddDate(-1, 0, 0)}, false},
	}
	for _, tt := range tests {
		if got := tt.r.UsableAsPrecedentFor("N357771", jan14); got != tt.want {
			t.Errorf("%s: UsableAsPrecedentFor = %v, want %v", tt.name, got, tt.want)
		}
	}
	if got := RulingURL("N357771"); got != "https://rulings.cbp.gov/ruling/N357771" {
		t.Errorf("RulingURL = %q", got)
	}
}

func TestPredictionAndTriageValidate(t *testing.T) {
	ok := Prediction{Origin: "China", Confidence: 0.9, CitedRulings: []string{"N350000"}}
	wantProblems(t, ok.Validate())
	wantProblems(t, Prediction{Confidence: 1.2, CitedRulings: []string{"x"}}.Validate(),
		"origin_country is empty", "outside [0, 1]", `"x" is not`)
	wantProblems(t, Triage{Route: RouteAuto, Reason: "confidence 0.97"}.Validate())
	wantProblems(t, Triage{Route: "maybe"}.Validate(), `unknown route "maybe"`, "reason is required")
}

func TestMove(t *testing.T) {
	double := Move{{Group: "motor_fan_assembly", ToPlant: "th"}, {Group: "pcba", ToPlant: "th"}}
	wantProblems(t, double.Validate())
	if got, want := double.Key(), "motor_fan_assembly@th+pcba@th"; got != want {
		t.Errorf("Key = %q, want %q", got, want)
	}
	wantProblems(t, Move{}.Validate(), "1 to 4 operation groups, got 0")
	wantProblems(t, append(double, GroupMove{Group: "x", ToPlant: "th"}).Validate())
	wantProblems(t, append(double, GroupMove{Group: "x", ToPlant: "th"}, GroupMove{Group: "y", ToPlant: "th"}, GroupMove{Group: "z", ToPlant: "th"}).Validate(), "got 5")
	wantProblems(t, Move{{Group: "pcba", ToPlant: "th"}, {Group: "pcba", ToPlant: "vn"}}.Validate(), `"pcba" moved twice`)
	wantProblems(t, Move{{Group: "pcba"}}.Validate(), "needs group and to_plant")
	wantProblems(t, OperationGroup{ID: "pcba"}.Validate(), "has no operations")
}

func TestVerdictValidate(t *testing.T) {
	wantProblems(t, Verdict{PrecedentSupported: true, SupportingRuling: "N350000", Confidence: 0.96}.Validate())
	wantProblems(t, Verdict{PrecedentSupported: false, CounterRuling: "N340000", Confidence: 0.9}.Validate())
	wantProblems(t, Verdict{PrecedentSupported: true, Confidence: 0.96}.Validate(), "requires a supporting ruling")
	wantProblems(t, Verdict{CounterRuling: "abc", Confidence: -0.1}.Validate(), "outside [0, 1]", `"abc"`)
}
