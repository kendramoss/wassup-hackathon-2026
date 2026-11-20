package opcost

import (
	"math"
	"strings"
	"testing"
)

func env() ValidateEnv {
	return ValidateEnv{HasProduct: true,
		Groups:       map[string]bool{"fa": true, "pcba": true},
		Plants:       map[string]Ref{"p1": rp1, "p2": rp2, "p3": rp3},
		CurrentPlant: map[string]string{"fa": "p1", "pcba": "p1"},
		GroupCosts:   map[string]map[string]float64{"fa": {"p1": 300_000, "p2": 320_000}}}
}

func valid() *Operations {
	o := costly(5).Ops
	return o
}

func slugs(ps []Problem) string {
	var s []string
	for _, p := range ps {
		s = append(s, p.Slug)
	}
	return strings.Join(s, ",")
}

func TestValidateOK(t *testing.T) {
	if ps := Validate(valid(), env()); len(ps) != 0 {
		t.Fatalf("%v", ps)
	}
	if ps := Validate(nil, env()); len(ps) != 0 {
		t.Fatal("inactive is valid")
	}
	// without a product references are not checked
	if ps := Validate(valid(), ValidateEnv{}); len(ps) != 0 {
		t.Fatalf("%v", ps)
	}
}

func TestValidateSlugs(t *testing.T) {
	big := math.Nextafter(MaxValue, math.Inf(1))
	long := strings.Repeat("x", 501)
	tests := []struct {
		name string
		mut  func(*Operations, *ValidateEnv)
		want string
	}{
		{"horizon zero", func(o *Operations, _ *ValidateEnv) { o.HorizonYears = f(0) }, "horizon_out_of_range"},
		{"horizon negative", func(o *Operations, _ *ValidateEnv) { o.HorizonYears = f(-1) }, "horizon_out_of_range"},
		{"horizon NaN", func(o *Operations, _ *ValidateEnv) { o.HorizonYears = f(math.NaN()) }, "horizon_out_of_range"},
		{"horizon Inf", func(o *Operations, _ *ValidateEnv) { o.HorizonYears = f(math.Inf(1)) }, "horizon_out_of_range"},
		{"horizon over 30", func(o *Operations, _ *ValidateEnv) { o.HorizonYears = f(30.0001) }, "horizon_out_of_range"},
		{"horizon missing with destination capex", func(o *Operations, _ *ValidateEnv) { o.HorizonYears = nil }, "horizon_required"},
		{"horizon too small", func(o *Operations, _ *ValidateEnv) { o.HorizonYears = f(1e-300) }, "horizon_too_small"},
		{"lead negative", func(o *Operations, _ *ValidateEnv) { o.MaxLeadTimeDays = n(-1) }, "lead_time_out_of_range"},
		{"lead over", func(o *Operations, _ *ValidateEnv) { o.MaxLeadTimeDays = n(3651) }, "lead_time_out_of_range"},
		{"plant lead over", func(o *Operations, _ *ValidateEnv) {
			p := o.Groups["fa"].Plants["p2"]
			p.LeadTimeDays = n(4000)
			o.Groups["fa"].Plants["p2"] = p
		}, "lead_time_out_of_range"},
		{"basis missing", func(o *Operations, _ *ValidateEnv) { g := o.Groups["fa"]; g.CostBasis = ""; o.Groups["fa"] = g }, "cost_basis_required"},
		{"basis invalid", func(o *Operations, _ *ValidateEnv) { g := o.Groups["fa"]; g.CostBasis = "both"; o.Groups["fa"] = g }, "cost_basis_invalid"},
		{"included exceeds", func(o *Operations, _ *ValidateEnv) {
			g := o.Groups["fa"]
			g.CostBasis = BasisIncluded
			p := g.Plants["p2"]
			p.Labour, p.Logistics = f(300_000), f(45_000)
			g.Plants["p2"] = p
			o.Groups["fa"] = g
		}, "components_exceed_group_cost"},
		{"included without group cost", func(o *Operations, e *ValidateEnv) {
			g := o.Groups["fa"]
			g.CostBasis = BasisIncluded
			o.Groups["fa"] = g
			e.GroupCosts = nil
		}, "included_requires_group_cost"},
		{"demand without unit", func(o *Operations, _ *ValidateEnv) {
			g := o.Groups["fa"]
			g.AnnualDemandUnits = f(5)
			o.Groups["fa"] = g
		}, "unit_required"},
		{"moq without unit", func(o *Operations, _ *ValidateEnv) {
			p := o.Groups["fa"].Plants["p2"]
			p.MOQUnits = f(5)
			o.Groups["fa"].Plants["p2"] = p
		}, "moq_unit_required"},
		{"orders zero", func(o *Operations, _ *ValidateEnv) { g := o.Groups["fa"]; g.OrdersPerYear = f(0); o.Groups["fa"] = g }, "orders_per_year_out_of_range"},
		{"orders over", func(o *Operations, _ *ValidateEnv) {
			g := o.Groups["fa"]
			g.OrdersPerYear = f(1e6 + 1)
			o.Groups["fa"] = g
		}, "orders_per_year_out_of_range"},
		{"negative money", func(o *Operations, _ *ValidateEnv) {
			p := o.Groups["fa"].Plants["p2"]
			p.Capex = f(-1)
			o.Groups["fa"].Plants["p2"] = p
		}, "value_out_of_range"},
		{"money over bound", func(o *Operations, _ *ValidateEnv) {
			p := o.Groups["fa"].Plants["p2"]
			p.Capex = f(big)
			o.Groups["fa"].Plants["p2"] = p
		}, "value_out_of_range"},
		{"money NaN", func(o *Operations, _ *ValidateEnv) {
			p := o.Groups["fa"].Plants["p2"]
			p.Labour = f(math.NaN())
			o.Groups["fa"].Plants["p2"] = p
		}, "value_out_of_range"},
		{"ref required", func(o *Operations, _ *ValidateEnv) {
			p := o.Groups["fa"].Plants["p2"]
			p.Ref = nil
			o.Groups["fa"].Plants["p2"] = p
		}, "plant_ref_required"},
		{"ref mismatch", func(o *Operations, _ *ValidateEnv) {
			p := o.Groups["fa"].Plants["p2"]
			p.Ref = &rp3
			o.Groups["fa"].Plants["p2"] = p
		}, "plant_ref_mismatch"},
		{"unknown group", func(o *Operations, _ *ValidateEnv) { o.Groups["zz"] = GroupOps{Unit: "pcs"} }, "unknown_group"},
		{"unknown plant", func(o *Operations, _ *ValidateEnv) { o.Groups["fa"].Plants["p9"] = PlantOps{Ref: &rp3} }, "unknown_plant"},
		{"too many groups", func(o *Operations, _ *ValidateEnv) {
			for i := 0; i <= MaxGroups; i++ {
				o.Groups[string(rune('A'+i%26))+strings.Repeat("a", i/26)] = GroupOps{Unit: "x"}
			}
		}, "too_many_groups"},
		{"too many plants", func(o *Operations, _ *ValidateEnv) {
			for i := 0; i <= MaxPlants; i++ {
				o.Groups["fa"].Plants[strings.Repeat("q", i+1)] = PlantOps{Ref: &rp3}
			}
		}, "too_many_plants"},
		{"source key", func(o *Operations, _ *ValidateEnv) {
			p := o.Groups["fa"].Plants["p2"]
			p.Sources = map[string]string{"price": "x"}
			o.Groups["fa"].Plants["p2"] = p
		}, "unknown_source_key"},
		{"note long", func(o *Operations, _ *ValidateEnv) { o.Note = long }, "string_too_long"},
		{"unit long", func(o *Operations, _ *ValidateEnv) {
			g := o.Groups["fa"]
			g.Unit = strings.Repeat("u", 25)
			o.Groups["fa"] = g
		}, "string_too_long"},
	}
	for _, tc := range tests {
		o, e := valid(), env()
		tc.mut(o, &e)
		got := slugs(Validate(o, e))
		if !strings.Contains(got, tc.want) {
			t.Errorf("%s: slugs %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestValidateBaselineCapitalDoesNotRequireHorizon(t *testing.T) {
	o := &Operations{Groups: map[string]GroupOps{"fa": {Plants: map[string]PlantOps{"p1": {Ref: &rp1, Capex: f(5_000_000), Retooling: f(1)}}}}}
	if ps := Validate(o, env()); len(ps) != 0 {
		t.Fatalf("baseline-only one-time must not need a horizon: %v", ps)
	}
	// the same entry at a potential destination does
	o.Groups["fa"].Plants["p2"] = PlantOps{Ref: &rp2, Capex: f(1)}
	if got := slugs(Validate(o, env())); got != "horizon_required" {
		t.Fatalf("%q", got)
	}
	// an ambiguous baseline (no current plant known) counts every entry
	e := env()
	e.CurrentPlant = nil
	o2 := &Operations{Groups: map[string]GroupOps{"fa": {Plants: map[string]PlantOps{"p1": {Ref: &rp1, Capex: f(1)}}}}}
	if got := slugs(Validate(o2, e)); got != "horizon_required" {
		t.Fatalf("ambiguous baseline: %q", got)
	}
}

func TestValidateIncludedBreakdownAtLimit(t *testing.T) {
	o := valid()
	g := o.Groups["fa"]
	g.CostBasis = BasisIncluded
	p := g.Plants["p2"]
	p.Labour, p.Logistics = f(320_000), f(0)
	g.Plants["p2"] = p
	p1 := g.Plants["p1"]
	p1.Labour, p1.Logistics = f(100_000), f(10_000)
	g.Plants["p1"] = p1
	o.Groups["fa"] = g
	if ps := Validate(o, env()); len(ps) != 0 {
		t.Fatalf("breakdown equal to the total is allowed: %v", ps)
	}
}
