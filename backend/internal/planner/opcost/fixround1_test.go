package opcost

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func negz() *float64 { z := math.Copysign(0, -1); return &z }

func marshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// F1: the unknown-as-0 legacy figures are private mechanics, never JSON.
func TestF1LegacyFiguresNeverMarshal(t *testing.T) {
	in := costly(4)
	in.plant("fa", "p2", func(p *PlantOps) { p.Labour = nil })
	r := eval(t, in) // unpriced
	if r.Priced {
		t.Fatal("premise: unpriced")
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(marshal(t, r)), &top); err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{"components": true, "net_modelled_per_year": true, "priced": true, "horizon_years": true,
		"unknown": true, "not_modelled": true, "sunk_ignored": true, "stale_refs": true, "feasibility": true,
		"feasibility_assessed": true, "checks": true, "assumptions": true, "decision": true, "baseline": true, "proposed": true}
	for k := range top {
		if !allowed[k] {
			t.Fatalf("undocumented key %q in the marshaled result", k)
		}
	}
	for _, k := range []string{"DutyEffectYear", "AddedCostYear", "NetYear"} {
		if _, ok := top[k]; ok {
			t.Fatalf("%s leaked", k)
		}
	}
	if string(top["net_modelled_per_year"]) != "null" {
		t.Fatalf("authoritative net must be null: %s", top["net_modelled_per_year"])
	}
	// the private figures still exist for the planner to copy
	if r.NetYear == 0 && r.AddedCostYear == 0 {
		t.Fatal("legacy figures must still be computed")
	}
}

// F2: exact decimal ties for fractional orders per year.
func TestF2MOQDecimalTies(t *testing.T) {
	moq := func(d, o, q float64) Check {
		in := base()
		in.Ops.Groups["fa"] = GroupOps{Unit: "pcs", AnnualDemandUnits: f(d), OrdersPerYear: f(o), Plants: map[string]PlantOps{
			"p1": {Ref: &rp1}, "p2": {Ref: &rp2, MOQUnits: f(q), MOQUnit: "pcs"}}}
		return eval(t, in).Checks[0]
	}
	tests := []struct {
		d, o, q float64
		want    string
	}{
		{55, 2.2, 25, Feasible}, {54.99, 2.2, 25, Infeasible}, {55.01, 2.2, 25, Feasible},
		{110, 4.4, 25, Feasible}, {220, 8.8, 25, Feasible}, {249, 8.3, 30, Feasible}, {248.99, 8.3, 30, Infeasible},
		{0.3, 0.1, 3, Feasible}, {0.29, 0.1, 3, Infeasible}, {3e-7, 1e-7, 3, Feasible}, {2.9999999e-7, 1e-7, 3, Infeasible},
		{9e12, 3, 3e12, Feasible}, {9e12 - 1, 3, 3e12, Infeasible}, {24000, 6, 4000, Feasible}, {24000, 6, 4000.01, Infeasible},
		{24000, 6, 5000, Infeasible}, {48000, 0.5, 96000, Feasible}, {48000, 0.5, 96000.5, Infeasible},
		{1, 1e6, 1e-6, Feasible}, {0.999999, 1e6, 1, Infeasible},
	}
	for _, tc := range tests {
		if got := moq(tc.d, tc.o, tc.q); got.Status != tc.want {
			t.Errorf("D=%v N=%v Q=%v: %s, want %s", tc.d, tc.o, tc.q, got.Status, tc.want)
		}
	}
	// sweep: every one-decimal orders/year and integer MOQ; D = Q*N exactly in decimal
	bad := 0
	for q := 1; q <= 200; q++ {
		for n10 := 1; n10 <= 99; n10++ {
			x := q * n10 // D = x/10
			d, _ := strconv.ParseFloat(fmt.Sprintf("%d.%d", x/10, x%10), 64)
			o, _ := strconv.ParseFloat(fmt.Sprintf("%d.%d", n10/10, n10%10), 64)
			if moq(d, o, float64(q)).Status != Feasible {
				bad++
			}
			dm, _ := strconv.ParseFloat(fmt.Sprintf("%d.%d", (x-1)/10, (x-1)%10), 64)
			if moq(dm, o, float64(q)).Status != Infeasible {
				bad++
			}
		}
	}
	if bad != 0 {
		t.Fatalf("%d sweep cases wrong", bad)
	}
}

// F3: -0 is +0 everywhere; absent stays absent; the caller's data is not changed.
func TestF3NegativeZero(t *testing.T) {
	in := base()
	in.Ops.HorizonYears = f(1)
	in.Ops.Groups["fa"] = GroupOps{Unit: "pcs", AnnualDemandUnits: negz(), OrdersPerYear: f(1), Plants: map[string]PlantOps{
		"p1": {Ref: &rp1, Labour: negz()}, "p2": {Ref: &rp2, MOQUnits: negz(), MOQUnit: "pcs", Capex: negz(), Labour: negz(), LeadTimeDays: n(0)}}}
	in.group("fa", func(g *GroupOps) { g.CostBasis = BasisAdd })
	in.GroupCosts = map[string]map[string]float64{"fa": {"p1": math.Copysign(0, -1), "p2": math.Copysign(0, -1)}}
	in.AnnualImportValueUSD = 1000
	in.Ops.MaxLeadTimeDays = n(0)
	r := eval(t, in)
	if s := marshal(t, r); negZero.MatchString(s) || strings.Contains(s, "-0.") && strings.Contains(s, "-0,") {
		t.Fatalf("negative zero in %s", s)
	}
	for _, ch := range r.Checks {
		for _, p := range []*float64{ch.Value, ch.Limit} {
			if p != nil && math.Signbit(*p) {
				t.Fatalf("check %+v", ch)
			}
		}
	}
	// the demand -0 case of the review: order size and limit
	in2 := base()
	in2.Ops.Groups["fa"] = GroupOps{Unit: "pcs", AnnualDemandUnits: negz(), OrdersPerYear: f(1), Plants: map[string]PlantOps{
		"p1": {Ref: &rp1}, "p2": {Ref: &rp2, MOQUnits: f(5), MOQUnit: "pcs"}}}
	c := eval(t, in2).Checks[0]
	if c.Status != Infeasible || c.Value == nil || math.Signbit(*c.Value) || negZero.MatchString(marshal(t, c)) {
		t.Fatalf("%+v %s", c, marshal(t, c))
	}
	// Normalize copies, keeps entered 0 non-nil, keeps absent nil, never mutates the caller
	o := &Operations{Groups: map[string]GroupOps{"fa": {AnnualDemandUnits: negz(), Plants: map[string]PlantOps{"p1": {Labour: negz(), Capex: nil}}}}}
	nrm := Normalize(o)
	pl := nrm.Groups["fa"].Plants["p1"]
	if pl.Labour == nil || math.Signbit(*pl.Labour) || pl.Capex != nil || math.Signbit(*nrm.Groups["fa"].AnnualDemandUnits) {
		t.Fatalf("normalize: %+v", pl)
	}
	if !math.Signbit(*o.Groups["fa"].Plants["p1"].Labour) || !math.Signbit(*o.Groups["fa"].AnnualDemandUnits) {
		t.Fatal("caller's data was mutated")
	}
	// hash: -0 equals +0, both differ from absent
	mk := func(l *float64) string {
		return BasisHash(MoneyBasis{Operations: &Operations{Groups: map[string]GroupOps{"fa": {Plants: map[string]PlantOps{"p1": {Labour: l, Capex: f(1)}}}}}})
	}
	if mk(negz()) != mk(f(0)) || mk(f(0)) == mk(nil) {
		t.Fatal("hash must treat -0 as 0 and keep 0 distinct from absent")
	}
	m1 := MoneyBasis{AnnualImportValueUSD: *negz(), DutyRates: map[string]float64{"c": *negz()}, GroupCosts: map[string]map[string]float64{"g": {"p": *negz()}}}
	m2 := MoneyBasis{AnnualImportValueUSD: 0, DutyRates: map[string]float64{"c": 0}, GroupCosts: map[string]map[string]float64{"g": {"p": 0}}}
	if BasisHash(m1) != BasisHash(m2) {
		t.Fatal("money -0 must hash as 0")
	}
}

// F4: currency breakdown compared in cents, rounded once.
func TestF4IncludedBreakdownCents(t *testing.T) {
	tests := []struct {
		l, lg, gc float64
		bad       bool
	}{
		{0.1, 0.2, 0.3, false}, {100.10, 200.20, 300.30, false}, {0.004, 0.004, 0.01, false},
		{0.304, 0, 0.3, false}, {0.31, 0, 0.3, true}, {0.2, 0.2, 0.3, true}, {1e12 - 0.1, 0.1, 1e12, false},
	}
	for _, tc := range tests {
		o := &Operations{Groups: map[string]GroupOps{"fa": {CostBasis: BasisIncluded, Plants: map[string]PlantOps{
			"p1": {Ref: &rp1, Labour: f(tc.l), Logistics: f(tc.lg)}}}}}
		e := env()
		e.GroupCosts = map[string]map[string]float64{"fa": {"p1": tc.gc}}
		got := strings.Contains(slugs(Validate(o, e)), "components_exceed_group_cost")
		if got != tc.bad {
			t.Errorf("%v+%v vs %v: exceed=%v want %v", tc.l, tc.lg, tc.gc, got, tc.bad)
		}
	}
}

// F5: strict single-value decode with meaningful slugs.
func TestF5Decode(t *testing.T) {
	tests := []struct{ in, slug string }{
		{`{"horizon_years":5} {"junk":1}`, "trailing_data"},
		{`{"horizon_years":5}x`, "trailing_data"},
		{`{"horizon_years":"five"}`, "invalid_json"},
		{`{"horizon_years":1e999}`, "invalid_json"},
		{`{"horizon_years":`, "invalid_json"},
		{``, "invalid_json"},
		{`{"cost_bais":1}`, "unknown_field"},
		{`{"groups":{"fa":{"cost_bais":"included"}}}`, "unknown_field"},
	}
	for _, tc := range tests {
		_, err := Decode([]byte(tc.in))
		if err == nil || !strings.HasPrefix(err.Error(), "operations."+tc.slug+":") {
			t.Errorf("%q: %v, want slug %s", tc.in, err, tc.slug)
		}
	}
	if o, err := Decode([]byte(" {\"horizon_years\":5}\n")); err != nil || o.HorizonYears == nil {
		t.Fatalf("whitespace around one value is fine: %v", err)
	}
}

// F6: null means "no usable number" and the reason is in unknown or not_modelled.
func TestF6NullIsExplained(t *testing.T) {
	cases := map[string]Input{}
	cases["worked"] = costly(4)
	only := base()
	only.Ops.HorizonYears = f(5)
	cases["horizon only"] = only
	inc := costly(10)
	inc.group("fa", func(g *GroupOps) { g.CostBasis = BasisIncluded })
	cases["included"] = inc
	nt := costly(4)
	nt.GroupCosts = nil
	nt.group("fa", func(g *GroupOps) { g.CostBasis = BasisAdd })
	cases["no table"] = nt
	um := costly(4)
	um.plant("fa", "p2", func(p *PlantOps) { p.Labour, p.Capex = nil, nil })
	cases["unknown"] = um
	for name, in := range cases {
		r := eval(t, in)
		c := r.Components
		check := func(field string, p *float64, factor string, notModelled ...string) {
			if p != nil {
				return
			}
			for _, u := range r.Unknown {
				if u.Category == "financial" && u.Factor == factor {
					return
				}
			}
			for _, nm := range notModelled {
				if has(r.NotModelled, nm) {
					return
				}
			}
			t.Errorf("%s: %s is null with no unknown/not_modelled entry (%v / %v)", name, field, r.Unknown, r.NotModelled)
		}
		check("group_cost", c.GroupCostDelta, "group_cost", "group_cost")
		check("labour", c.LabourDelta, "labour", "labour:fa", "labour_in_group_cost:fa")
		check("logistics", c.LogisticsDelta, "logistics", "logistics:fa", "logistics_in_group_cost:fa")
		check("one_time", c.OneTimeTotal, "capex", "capex:fa")
		check("amortized", c.AmortizedOneTimePerYear, "capex", "capex:fa")
	}
	// exact semantics
	r := eval(t, only)
	isNil(t, "operating", r.Components.OperatingDelta)
	isNil(t, "one-time", r.Components.OneTimeTotal)
	val(t, "net", r.NetModelledPerYear, 200_000)
	if !r.Priced || !has(r.NotModelled, "group_cost") {
		t.Fatalf("horizon-only: %+v", r)
	}
	r = eval(t, cases["included"])
	if !has(r.NotModelled, "labour_in_group_cost:fa") || r.Components.LabourDelta != nil {
		t.Fatalf("included breakdown is not a separate number: %v", r.NotModelled)
	}
	// an entered zero is a number
	z := costly(4)
	z.plant("fa", "p1", func(p *PlantOps) { p.Logistics = f(0) })
	z.plant("fa", "p2", func(p *PlantOps) { p.Logistics = f(0) })
	val(t, "known zero", eval(t, z).Components.LogisticsDelta, 0)
}

// Baseline and proposed totals come from the same Evaluate (modelled terms of the moved groups only).
func TestBaselineProposedTotals(t *testing.T) {
	r := eval(t, costly(4))
	b, p := r.Baseline, r.Proposed
	if b == nil || p == nil {
		t.Fatal("sides missing")
	}
	val(t, "base duty", b.DutyUSD, 250_000)
	val(t, "base operating", b.OperatingUSD, 410_000)
	val(t, "base total", b.TotalUSD, 660_000)
	val(t, "prop duty", p.DutyUSD, 50_000)
	val(t, "prop operating", p.OperatingUSD, 425_000)
	val(t, "prop amortized", p.AmortizedOneTimeUSD, 250_000)
	val(t, "prop total", p.TotalUSD, 725_000)
	if *b.TotalUSD-*p.TotalUSD != *r.NetModelledPerYear {
		t.Fatal("baseline.total - proposed.total must equal the net")
	}
	// unpriced: totals unknown, never partial sums
	u := costly(4)
	u.plant("fa", "p2", func(pp *PlantOps) { pp.Labour = nil })
	ru := eval(t, u)
	if ru.Baseline.TotalUSD != nil || ru.Proposed.TotalUSD != nil || ru.Baseline.OperatingUSD != nil {
		t.Fatalf("unpriced totals: %+v %+v", ru.Baseline, ru.Proposed)
	}
	// the identity holds across cent-valued sweeps
	for v := 1; v <= 40; v++ {
		in := costly(float64(1 + v%9))
		in.AnnualImportValueUSD = float64(v) * 12_345
		in.GroupCosts["fa"]["p2"] = 300_000 + float64(v)*17.25
		rr := eval(t, in)
		if d := *rr.Baseline.TotalUSD - *rr.Proposed.TotalUSD - *rr.NetModelledPerYear; math.Abs(d) > 0.011 {
			t.Fatalf("v=%d identity off by %v", v, d)
		}
	}
}

// One shared canonicalisation: the legacy shape is unchanged and the ops part is CanonicalOps.
func TestCanonicalMoneyShape(t *testing.T) {
	type legacy struct {
		Illustrative bool
		Value        float64
		Rates        map[string]float64
		Groups       map[string]map[string]float64
	}
	m := MoneyBasis{Illustrative: true, AnnualImportValueUSD: 5, DutyRates: map[string]float64{"c": 0.1},
		GroupCosts: map[string]map[string]float64{"g": {"p": 1}, "empty": {}}}
	want := marshal(t, legacy{true, 5, map[string]float64{"c": 0.1}, map[string]map[string]float64{"g": {"p": 1}}})
	if got := marshal(t, CanonicalMoney(m)); got != want {
		t.Fatalf("legacy shape changed:\n got %s\nwant %s", got, want)
	}
	m.Operations = costly(5).Ops
	c := CanonicalMoney(m)
	if !reflect.DeepEqual(c.Ops, CanonicalOps(m.Operations)) || c.Ops == nil {
		t.Fatal("ops part must be CanonicalOps")
	}
}
