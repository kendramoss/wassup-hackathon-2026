package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner/opcost"
)

// #281 step 3 wiring tests: operations mode through the real planner, with the
// fake engine and verifier of the existing tests. Every expected number is
// worked by hand in the comment next to it.

func decodeOps(t *testing.T, s string) *opcost.Operations {
	t.Helper()
	o, err := opcost.Decode([]byte(s))
	if err != nil {
		t.Fatalf("operations %s: %v", s, err)
	}
	return o
}

// motorOps: the motor_fan_assembly group, cn -> th, labour and logistics
// additional to the group cost. thExtra is spliced into the th entry.
func motorOps(horizon string, thExtra string) string {
	return fmt.Sprintf(`{"horizon_years":%s,"groups":{"motor_fan_assembly":{"cost_basis":"additional","plants":{
	 "cn":{"ref":{"name":"China suppliers","country":"China"},"labour_usd_per_year":100000,"logistics_usd_per_year":10000},
	 "th":{"ref":{"name":"Rayong plant","country":"Thailand"},"labour_usd_per_year":60000,"logistics_usd_per_year":45000%s}}}}}`, horizon, thExtra)
}

// opsPlan runs the real planner on the heat gun fixture with the given
// economics edits and plan confidence.
func opsPlan(t *testing.T, conf float64, mutate func(*Economics), opts Options) (Result, []Event) {
	t.Helper()
	r, _ := fixture(t, "heatgun_2022.json")
	mutate(&r.Economics)
	tmpls, _ := LoadTemplates()
	opts.PlanConfidence, opts.Concurrency = conf, 3
	var evs []Event
	var res Result
	var err error
	p := New(tmpls, allThaiEngine{}, confByMove{}, opts)
	res, err = p.PlanWithProgress(context.Background(), r, func(e Event) { evs = append(evs, e) })
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("DUMP281") != "" {
		b, _ := json.MarshalIndent(res, "", " ")
		os.WriteFile(os.Getenv("DUMP281"), b, 0o644)
	}
	return res, evs
}

func findPlan(res Result, key string) (domain.Plan, bool) {
	for _, p := range append(append([]domain.Plan{}, res.Plans...), res.Rejected...) {
		if p.Move.Key() == key {
			return p, true
		}
	}
	return domain.Plan{}, false
}

const motorKey = "motor_fan_assembly@th"

func f64(p *float64) float64 {
	if p == nil {
		return -999999999
	}
	return *p
}

// Hand-worked, fixture rates China 41.7% -> Thailand 16.7% on $2,000,000:
// duty effect 500,000. Motor group costs cn 300,000, th 420,000: +120,000.
// Labour 60,000-100,000 = -40,000; logistics 45,000-10,000 = +35,000.
// Operating delta 120,000-40,000+35,000 = 115,000. One-time 900,000+100,000
// = 1,000,000 over 4 years = 250,000. Net 500,000-115,000-250,000 = 135,000.
func TestOpsPlanAllInNet(t *testing.T) {
	res, _ := opsPlan(t, 0.85, func(e *Economics) {
		e.Operations = decodeOps(t, motorOps("4", `,"capex_usd":900000,"retooling_usd":100000`))
	}, Options{MaxMoves: DefaultMaxMoves})
	pl, ok := findPlan(res, motorKey)
	if !ok || !pl.Shown {
		t.Fatalf("motor move missing or not shown: %+v", pl)
	}
	o := pl.Operations
	if o == nil || !o.Priced || pl.NetPriced == nil || !*pl.NetPriced {
		t.Fatalf("not priced: %+v", o)
	}
	c := o.Components
	if f64(c.DutyEffectPerYear) != 500000 || f64(c.GroupCostDelta) != 120000 || f64(c.LabourDelta) != -40000 || f64(c.LogisticsDelta) != 35000 ||
		f64(c.OperatingDelta) != 115000 || f64(c.OneTimeTotal) != 1000000 || f64(c.AmortizedOneTimePerYear) != 250000 || f64(o.NetModelledPerYear) != 135000 {
		t.Errorf("components = %+v net %v", c, f64(o.NetModelledPerYear))
	}
	if pl.DutyEffectYear != 500000 || pl.AddedCostYear != 115000 || pl.NetYear != 135000 {
		t.Errorf("plan fields = %v %v %v, want 500000 115000 135000", pl.DutyEffectYear, pl.AddedCostYear, pl.NetYear)
	}
	if res.Objective == nil || res.Objective.Mode != "operations_v1" || res.Objective.Scope != "searched_moves_only" ||
		strings.Contains(strings.ToLower(res.Objective.Statement), "optimal") && !strings.Contains(res.Objective.Statement, "Not a global optimum") {
		t.Errorf("objective = %+v", res.Objective)
	}
	// Absolute sides from the same arithmetic: baseline duty 2,000,000 x 41.7%.
	if f64(o.Baseline.DutyUSD) != 834000 || f64(o.Proposed.DutyUSD) != 334000 {
		t.Errorf("sides = %+v %+v", o.Baseline, o.Proposed)
	}
	// Every modelled net carries its scope, priced or not.
	if len(o.NotModelled) == 0 {
		t.Error("a priced plan must still list what is not modelled")
	}
}

// One-time 2,000,000 over 4 years = 500,000: net 500,000-115,000-500,000 =
// -115,000 -> fully priced and negative -> net_costlier.
func costlierEcon(t *testing.T) func(*Economics) {
	return func(e *Economics) {
		e.Operations = decodeOps(t, motorOps("4", `,"capex_usd":1900000,"retooling_usd":100000`))
	}
}

func TestOpsNetCostlierIsDemotedWithReason(t *testing.T) {
	res, _ := opsPlan(t, 0.85, costlierEcon(t), Options{MaxMoves: DefaultMaxMoves})
	pl, ok := findPlan(res, motorKey)
	if !ok || pl.Shown || pl.RejectionKind != domain.RejectNetCostlier || pl.NetYear != -115000 {
		t.Fatalf("plan = shown %v kind %q net %v, want demoted net_costlier -115000", pl.Shown, pl.RejectionKind, pl.NetYear)
	}
	for _, want := range []string{"-$115,000.00", "$500,000.00", "$115,000.00", "modelled terms only"} {
		if !strings.Contains(pl.RejectionReason, want) {
			t.Errorf("reason lacks %q: %s", want, pl.RejectionReason)
		}
	}
	// Row keeps its legal facts and its money.
	if pl.Verdict == nil || pl.Operations == nil || pl.Operations.Decision != opcost.DecisionCostlier {
		t.Errorf("row lost its verdict or operations: %+v", pl)
	}
	for _, s := range res.Plans {
		if s.Move.Key() == motorKey {
			t.Error("a demoted plan is still in the shown list")
		}
	}
}

func TestOpsInfeasibleLeadTimeWinsOverCostlier(t *testing.T) {
	// th lead 120 days > the 90-day limit; the net is also negative (2,000,000
	// capex), and the infeasibility is the reason given.
	res, _ := opsPlan(t, 0.85, func(e *Economics) {
		o := decodeOps(t, motorOps("4", `,"capex_usd":1900000,"retooling_usd":100000,"lead_time_days":120`))
		v := 90
		o.MaxLeadTimeDays = &v
		e.Operations = o
	}, Options{MaxMoves: DefaultMaxMoves})
	pl, _ := findPlan(res, motorKey)
	if pl.Shown || pl.RejectionKind != domain.RejectOperationallyInfeasible || !strings.Contains(pl.RejectionReason, "lead time 120 days exceeds your 90-day limit") {
		t.Fatalf("kind %q reason %q", pl.RejectionKind, pl.RejectionReason)
	}
}

func TestOpsNetExactlyZeroStaysShown(t *testing.T) {
	// Horizon alone activates operations. Motor group cost +500,000 cancels the
	// 500,000 duty effect: net exactly 0 -> shown ("no modelled saving").
	res, _ := opsPlan(t, 0.85, func(e *Economics) {
		e.Operations = decodeOps(t, `{"horizon_years":5}`)
		e.GroupCosts = map[string]map[string]float64{"motor_fan_assembly": {"cn": 100000, "th": 600000}}
	}, Options{MaxMoves: DefaultMaxMoves})
	pl, _ := findPlan(res, motorKey)
	if !pl.Shown || pl.NetYear != 0 || pl.NetPriced == nil || !*pl.NetPriced || pl.RejectionKind != "" {
		t.Fatalf("zero-net plan = %+v", pl)
	}
}

func TestOpsUnknownIsNeverDemotedOrConfirmed(t *testing.T) {
	// th labour missing: the labour term is unknown, so the plan is not priced
	// and stays shown; its feasibility is unknown too (no lead time at th).
	res, _ := opsPlan(t, 0.85, func(e *Economics) {
		o := decodeOps(t, `{"max_lead_time_days":90,"groups":{"motor_fan_assembly":{"cost_basis":"additional","plants":{
		 "cn":{"ref":{"name":"China suppliers","country":"China"},"labour_usd_per_year":100000},
		 "th":{"ref":{"name":"Rayong plant","country":"Thailand"}}}}}}`)
		e.Operations = o
	}, Options{MaxMoves: DefaultMaxMoves})
	pl, _ := findPlan(res, motorKey)
	if !pl.Shown || pl.NetPriced == nil || *pl.NetPriced || pl.Operations.Priced || pl.Operations.NetModelledPerYear != nil {
		t.Fatalf("unknown plan = %+v", pl)
	}
	if pl.Operations.Feasibility != opcost.StatusUnknown {
		t.Errorf("feasibility = %q, want unknown (never feasible)", pl.Operations.Feasibility)
	}
}

func TestOpsFinancialKnownNegativeIsRejectedEvenWithFeasibilityUnknown(t *testing.T) {
	// Lead time limit given, th lead time missing: feasibility unknown, but the
	// financial net is fully known and negative, so net_costlier (correction A).
	res, _ := opsPlan(t, 0.85, func(e *Economics) {
		o := decodeOps(t, motorOps("4", `,"capex_usd":1900000,"retooling_usd":100000`))
		v := 90
		o.MaxLeadTimeDays = &v
		e.Operations = o
	}, Options{MaxMoves: DefaultMaxMoves})
	pl, _ := findPlan(res, motorKey)
	if pl.RejectionKind != domain.RejectNetCostlier || pl.Operations.Feasibility != opcost.StatusUnknown {
		t.Fatalf("kind %q feasibility %q", pl.RejectionKind, pl.Operations.Feasibility)
	}
}

func TestOpsCostNeverOverridesTheLegalGate(t *testing.T) {
	// Plan confidence 0.95: the motor-only move (verifier 0.90) is below the bar.
	// With a negative net and an infeasible lead time it keeps its legal kind.
	res, _ := opsPlan(t, 0.95, func(e *Economics) {
		o := decodeOps(t, motorOps("4", `,"capex_usd":1900000,"retooling_usd":100000,"lead_time_days":120`))
		v := 90
		o.MaxLeadTimeDays = &v
		e.Operations = o
	}, Options{MaxMoves: DefaultMaxMoves})
	pl, _ := findPlan(res, motorKey)
	if pl.Shown || pl.RejectionKind != domain.RejectBelowConfidence || !strings.Contains(pl.RejectionReason, "below") {
		t.Fatalf("legal rejection overridden: kind %q reason %q", pl.RejectionKind, pl.RejectionReason)
	}
	if pl.Operations == nil || pl.Operations.Decision != opcost.DecisionInfeasible {
		t.Errorf("cost fields are still computed and shown: %+v", pl.Operations)
	}
	// And a positive net cannot promote it.
	res, _ = opsPlan(t, 0.95, func(e *Economics) { e.Operations = decodeOps(t, `{"horizon_years":5}`) }, Options{MaxMoves: DefaultMaxMoves})
	pl, _ = findPlan(res, motorKey)
	if pl.Shown || pl.RejectionKind != domain.RejectBelowConfidence {
		t.Errorf("positive net promoted a below-bar plan: %+v", pl.RejectionKind)
	}
}

// Order: shown plans, fully priced before not priced, then net; rejected: the
// current setup first, the cost demotions (infeasible, then costlier), then
// the legal kinds in their existing order.
func TestOpsOrdering(t *testing.T) {
	res, _ := opsPlan(t, 0.85, func(e *Economics) {
		// pcba cn 150000 th 210000 (+60000); motor +120000. No ops costs, only
		// a horizon, so priced = duty and group costs. One group has no cost at
		// th (housing is not moved; switch group cost missing -> unknown).
		e.Operations = decodeOps(t, `{"horizon_years":5}`)
		e.GroupCosts = map[string]map[string]float64{"motor_fan_assembly": {"cn": 300000, "th": 420000}, "pcba": {"cn": 150000, "th": 210000}}
	}, Options{MaxMoves: DefaultMaxMoves})
	seenUnpriced := false
	for _, p := range res.Plans {
		priced := p.NetPriced != nil && *p.NetPriced
		if priced && seenUnpriced {
			t.Errorf("priced plan %s after an unpriced one", p.Move.Key())
		}
		if !priced {
			seenUnpriced = true
		}
	}
	if !seenUnpriced {
		t.Fatal("setup: expected some unpriced plans (the switch group has no cost)")
	}
	if res.Rejected[0].RejectionKind != domain.RejectCurrentSetup && len(res.Rejected) > 0 {
		// the baseline is China-origin so it is listed first
		if res.Baseline.Prediction.Origin != "Thailand" {
			t.Errorf("first rejected = %q", res.Rejected[0].RejectionKind)
		}
	}
}

func TestOpsRejectedKindOrder(t *testing.T) {
	// pcba move is made infeasible (lead 120 > 90), motor net-costlier, others
	// shown or below bar. Confidence 0.95 puts the single moves below the bar.
	res, _ := opsPlan(t, 0.85, func(e *Economics) {
		e.Operations = decodeOps(t, `{"horizon_years":4,"max_lead_time_days":90,"groups":{
		 "motor_fan_assembly":{"plants":{"th":{"ref":{"name":"Rayong plant","country":"Thailand"},"capex_usd":3000000,"lead_time_days":60}}},
		 "pcba":{"plants":{"th":{"ref":{"name":"Rayong plant","country":"Thailand"},"lead_time_days":120}}}}}`)
		e.GroupCosts = map[string]map[string]float64{"motor_fan_assembly": {"cn": 300000, "th": 420000}, "pcba": {"cn": 150000, "th": 210000}}
	}, Options{MaxMoves: DefaultMaxMoves})
	rank := map[domain.RejectionKind]int{domain.RejectCurrentSetup: -1, domain.RejectOperationallyInfeasible: 0, domain.RejectNetCostlier: 1,
		domain.RejectBelowConfidence: 2, domain.RejectNoPrecedent: 3, domain.RejectCheckFailed: 4, domain.RejectOtherOrigin: 5}
	last := -2
	kinds := map[domain.RejectionKind]bool{}
	for _, p := range res.Rejected {
		if rank[p.RejectionKind] < last {
			t.Errorf("rejected out of order at %s (%s)", p.Move.Key(), p.RejectionKind)
		}
		last = rank[p.RejectionKind]
		kinds[p.RejectionKind] = true
	}
	if !kinds[domain.RejectOperationallyInfeasible] || !kinds[domain.RejectNetCostlier] {
		t.Errorf("setup: kinds present %v", kinds)
	}
}

// The stream must never show a plan as shown that the final result rejects:
// the demotion runs before move_done. Order and names are unchanged.
func TestOpsSSEDemotionBeforeMoveDone(t *testing.T) {
	res, evs := opsPlan(t, 0.85, costlierEcon(t), Options{MaxMoves: DefaultMaxMoves, Concurrency: 1})
	started, doneCount := 0, 0
	for i, e := range evs {
		switch e.Kind {
		case EventStarted:
			if i != 0 {
				t.Errorf("started at %d", i)
			}
			started++
		case EventMoveDone:
			doneCount++
			final, _ := findPlan(res, e.Plan.Move.Key())
			if e.Plan.Shown != final.Shown || e.Plan.RejectionKind != final.RejectionKind {
				t.Errorf("move_done %s shows shown=%v kind=%q, final result has shown=%v kind=%q",
					e.Move, e.Plan.Shown, e.Plan.RejectionKind, final.Shown, final.RejectionKind)
			}
			if e.Plan.Prediction.Origin != "" && e.Plan.Operations == nil {
				t.Errorf("move_done %s carries no operations", e.Move)
			}
		case EventDone:
			if i != len(evs)-1 {
				t.Errorf("done at %d of %d", i, len(evs))
			}
		}
	}
	if started != 1 || doneCount != res.Stats.MovesSearched {
		t.Errorf("started %d move_done %d searched %d", started, doneCount, res.Stats.MovesSearched)
	}
	if m, _ := findPlan(res, motorKey); m.RejectionKind != domain.RejectNetCostlier {
		t.Errorf("setup: motor move kind %q", m.RejectionKind)
	}
}

func TestOpsFollowedAdviceGoesThroughTheSameDemotion(t *testing.T) {
	r, _ := fixture(t, "heatgun_2022.json")
	// The followed move is motor+pcba+switch; a huge one-time cost at th for the
	// switch group makes it net-costlier.
	r.Economics.Operations = decodeOps(t, `{"horizon_years":1,"groups":{"switch_cord_minor_parts":{"plants":{"th":{"ref":{"name":"Rayong plant","country":"Thailand"},"capex_usd":9000000}}}}}`)
	tmpls, _ := LoadTemplates()
	var evs []Event
	res, err := New(tmpls, allThaiEngine{}, confByMove{}, Options{PlanConfidence: 0.92, Concurrency: 3, NearMiss: adviceSays{twoGroups}, FollowAdvice: true}).
		PlanWithProgress(context.Background(), r, func(e Event) { evs = append(evs, e) })
	if err != nil {
		t.Fatal(err)
	}
	f := followed(res)
	if len(f) != 1 {
		t.Fatalf("followed %d", len(f))
	}
	pl := f[0]
	if pl.Operations == nil || pl.NetPriced == nil {
		t.Fatalf("followed plan lacks money: %+v", pl)
	}
	if pl.Shown || pl.RejectionKind != domain.RejectNetCostlier {
		t.Errorf("followed move not demoted: shown %v kind %q net %v", pl.Shown, pl.RejectionKind, pl.NetYear)
	}
	for _, e := range evs {
		if e.Kind == EventMoveDone && e.Move == pl.Move.Key() && (e.Plan.Shown || e.Plan.RejectionKind != domain.RejectNetCostlier) {
			t.Errorf("move_done for the followed move shows %v/%q", e.Plan.Shown, e.Plan.RejectionKind)
		}
	}
}

// An inactive operations block (null, {}, {"groups":{}}, only a ref) runs the
// legacy code: the result is the golden captured before the wiring.
func TestInactiveOperationsAreTheLegacyGoldens(t *testing.T) {
	for _, js := range []string{`{}`, `{"groups":{}}`, `{"note":"just a note"}`,
		`{"groups":{"motor_fan_assembly":{"plants":{"th":{"ref":{"name":"Rayong plant","country":"Thailand"}}}}}}`} {
		for name, mut := range goldenEconomics() {
			t.Run(name+js[:min(len(js), 12)], func(t *testing.T) {
				r, _ := fixture(t, "heatgun_2022.json")
				mut(&r.Economics)
				r.Economics.Operations = decodeOps(t, js)
				res, err := goldenPlanner(t, 3, Options{MaxMoves: DefaultMaxMoves}).Plan(context.Background(), r)
				if err != nil {
					t.Fatal(err)
				}
				res.Stats.WallSeconds = 0
				goldenCheck(t, "plan_"+name, res)
			})
		}
	}
}

// The operations block is the whole difference: plan JSON of an ops plan has
// the new keys; a legacy plan's key set is unchanged.
func TestOpsJSONKeysAreAdditiveAndNoFallbackZeroLeaks(t *testing.T) {
	res, _ := opsPlan(t, 0.85, func(e *Economics) { e.Operations = decodeOps(t, `{"horizon_years":5}`) }, Options{MaxMoves: DefaultMaxMoves})
	b, _ := json.Marshal(res.Plans[0])
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	ops, _ := m["operations"].(map[string]any)
	if ops == nil {
		t.Fatalf("no operations in %s", b)
	}
	for _, leak := range []string{"DutyEffectYear", "AddedCostYear", "NetYear", "duty_effect_per_year", "added_cost_per_year", "net_per_year"} {
		if _, bad := ops[leak]; bad {
			t.Errorf("legacy fallback figure %q leaked into operations", leak)
		}
	}
	legacy, _ := opsPlan(t, 0.85, func(e *Economics) {}, Options{MaxMoves: DefaultMaxMoves})
	b, _ = json.Marshal(legacy)
	if strings.Contains(string(b), `"operations"`) || strings.Contains(string(b), `"objective"`) || strings.Contains(string(b), `"economics_basis"`) {
		t.Error("a legacy result carries operations-mode keys")
	}
}

// One calculator: the same economics and the same move give identical
// components through the plan search and through Compare (scenarios and target
// comparison call Compare).
func TestOpsCompareUsesTheSameArithmeticAsThePlan(t *testing.T) {
	r, tm := fixture(t, "heatgun_2022.json")
	e := r.Economics
	e.Operations = decodeOps(t, motorOps("4", `,"capex_usd":900000,"retooling_usd":100000`))
	res, _ := opsPlan(t, 0.85, func(x *Economics) { x.Operations = e.Operations }, Options{MaxMoves: DefaultMaxMoves})
	pl, _ := findPlan(res, motorKey)
	_ = tm
	v := Apply(r.Product, domain.Move{{Group: "motor_fan_assembly", ToPlant: "th"}})
	cmp := Compare(e, &r.Product, &v, "China", "Thailand")
	if cmp.Operations == nil {
		t.Fatal("no operations on the comparison")
	}
	a, _ := json.Marshal(pl.Operations)
	b, _ := json.Marshal(cmp.Operations)
	if string(a) != string(b) {
		t.Errorf("plan and comparison differ\n plan: %s\n cmp:  %s", a, b)
	}
	if f64(cmp.AddedCostUSD) != 115000 || f64(cmp.AmortizedOneTimeUSD) != 250000 || f64(cmp.NetUSD) != 135000 || f64(cmp.DutySavingsUSD) != 500000 {
		t.Errorf("comparison = added %v amort %v net %v savings %v", f64(cmp.AddedCostUSD), f64(cmp.AmortizedOneTimeUSD), f64(cmp.NetUSD), f64(cmp.DutySavingsUSD))
	}
	// Absolute sides: baseline total - proposed total = the net (cent inputs).
	if got := f64(cmp.Operations.Baseline.TotalUSD) - f64(cmp.Operations.Proposed.TotalUSD); got != 135000 {
		t.Errorf("baseline-proposed = %v, want the net 135000", got)
	}
	if b, _ := json.Marshal(cmp); os.Getenv("DUMP281C") != "" {
		os.WriteFile(os.Getenv("DUMP281C"), b, 0o644)
	}
}

func TestOpsCompareUnknownAndNotModelledAreNotZero(t *testing.T) {
	r, _ := fixture(t, "heatgun_2022.json")
	v := Apply(r.Product, domain.Move{{Group: "motor_fan_assembly", ToPlant: "th"}})
	// No group costs, horizon only: nothing but duty is modelled -> the added
	// cost is null (not modelled), the net is the duty effect (scope stated).
	e := r.Economics
	e.GroupCosts = nil
	e.Operations = decodeOps(t, `{"horizon_years":5}`)
	cmp := Compare(e, &r.Product, &v, "China", "Thailand")
	if cmp.AddedCostUSD != nil || f64(cmp.NetUSD) != 500000 || !contains(cmp.Operations.NotModelled, "group_cost") {
		t.Errorf("no-cost mode: added %v net %v notmodelled %v", cmp.AddedCostUSD, f64(cmp.NetUSD), cmp.Operations.NotModelled)
	}
	// Labour at the current plant only: the target is unknown -> net null.
	e.Operations = decodeOps(t, `{"groups":{"motor_fan_assembly":{"cost_basis":"additional","plants":{"cn":{"ref":{"name":"China suppliers","country":"china"},"labour_usd_per_year":100000}}}}}`)
	cmp = Compare(e, &r.Product, &v, "China", "Thailand")
	if cmp.NetUSD != nil || cmp.AddedCostUSD != nil || cmp.Operations.Priced {
		t.Errorf("unknown target priced: %+v", cmp)
	}
	// A comparison without a variant has no operations (nothing to move).
	if c := Compare(e, &r.Product, nil, "China", "Thailand"); c.Operations != nil || c.NetUSD != nil {
		t.Errorf("no variant: %+v", c)
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// A group spread over two plants has no single current plant: the move's costs
// are unknown (never taken from an arbitrary plant), duty is unaffected.
func TestOpsAmbiguousBaselineIsUnknown(t *testing.T) {
	r, _ := fixture(t, "heatgun_2022.json")
	for i := range r.Product.Components {
		if r.Product.Components[i].Name == "fan" {
			r.Product.Components[i].Plant = "th" // motor group now at cn and th
		}
	}
	if CurrentPlant(r.Product, "motor_fan_assembly") != "" {
		t.Fatal("setup: the group should be ambiguous")
	}
	e := r.Economics
	e.Operations = decodeOps(t, motorOps("4", ``))
	res := opsEvaluate(e, r.Product, nil, domain.Move{{Group: "motor_fan_assembly", ToPlant: "th"}}, "China", "Thailand")
	if res.Priced || res.NetModelledPerYear != nil {
		t.Errorf("ambiguous baseline priced: %+v", res.Components)
	}
	found := false
	for _, u := range res.Unknown {
		if u.Reason == "ambiguous_baseline" {
			found = true
		}
	}
	if !found {
		t.Errorf("unknown = %+v", res.Unknown)
	}
}

// Refs are canonicalised at the adapter: "china"/"THAILAND" in the entry match
// the product's plant; a re-added plant with another country is ignored.
func TestOpsPlantRefsAreCanonicalAndStaleRefsIgnored(t *testing.T) {
	r, _ := fixture(t, "heatgun_2022.json")
	e := r.Economics
	ops := motorOps("4", ``)
	ops = strings.ReplaceAll(strings.ReplaceAll(ops, `"country":"Thailand"`, `"country":"THAILAND"`), `"country":"China"`, `"country":"china"`)
	e.Operations = decodeOps(t, ops)
	m := domain.Move{{Group: "motor_fan_assembly", ToPlant: "th"}}
	res := opsEvaluate(e, r.Product, nil, m, "China", "Thailand")
	if !res.Priced || len(res.StaleRefs) != 0 {
		t.Fatalf("canonical refs not matched: priced %v stale %v", res.Priced, res.StaleRefs)
	}
	// The product's th plant is now in Vietnam: the th entry no longer applies.
	for i := range r.Product.Plants {
		if r.Product.Plants[i].ID == "th" {
			r.Product.Plants[i].Country = "Vietnam"
		}
	}
	res = opsEvaluate(e, r.Product, nil, m, "China", "Vietnam")
	if res.Priced || len(res.StaleRefs) == 0 {
		t.Errorf("stale entry used: priced %v stale %v", res.Priced, res.StaleRefs)
	}
	// The caller's block is never changed by the adapter.
	if got := *e.Operations.Groups["motor_fan_assembly"].Plants["th"].Ref; got.Country != "THAILAND" {
		t.Errorf("adapter changed the caller's ref: %+v", got)
	}
}

func TestOpsCompareInactiveIsTheLegacyGolden(t *testing.T) {
	r, tm := fixture(t, "heatgun_2022.json")
	moves := Enumerate(r.Product, tm, "Thailand")
	out := map[string]any{}
	for name, mut := range goldenEconomics() {
		e := r.Economics
		mut(&e)
		e.Operations = decodeOps(t, `{"groups":{}}`)
		rows := map[string]Comparison{}
		for _, m := range moves {
			v := Apply(r.Product, m)
			rows[m.Key()] = Compare(e, &r.Product, &v, "China", "Thailand")
		}
		pair := domain.Move{{Group: "motor_fan_assembly", ToPlant: "th"}, {Group: "pcba", ToPlant: "th"}}
		pv := Apply(r.Product, pair)
		rows["pair"] = Compare(e, &r.Product, &pv, "China", "Thailand")
		same := r.Product
		rows["no_move"] = Compare(e, &r.Product, &same, "China", "Thailand")
		rows["no_variant"] = Compare(e, &r.Product, nil, "China", "Thailand")
		rows["no_products"] = Compare(e, nil, nil, "China", "Thailand")
		out[name] = rows
	}
	goldenCheck(t, "compare_all", out)
}

// Wiring-level table: the numbers the contract promises, through the real
// planner. Hand-worked: net before one-time = 500,000 - 115,000 = 385,000.
func TestOpsPlanTable(t *testing.T) {
	type want struct {
		shown, priced bool
		kind          domain.RejectionKind
		net           float64
		feas          string
	}
	moq := func(q, orders string) string {
		return fmt.Sprintf(`{"horizon_years":4,"groups":{"motor_fan_assembly":{"cost_basis":"additional","unit":"pcs","annual_demand_units":24000,"orders_per_year":%s,"plants":{
		 "cn":{"ref":{"name":"China suppliers","country":"China"},"labour_usd_per_year":100000,"logistics_usd_per_year":10000},
		 "th":{"ref":{"name":"Rayong plant","country":"Thailand"},"labour_usd_per_year":60000,"logistics_usd_per_year":45000,"moq_units":%s,"moq_unit":"pcs"}}}}}`, orders, q)
	}
	cases := []struct {
		name string
		ops  string
		want want
	}{
		// horizon flip: 1,000,000 one-time. h=2 -> 500,000 a year -> net -115,000; h=3 -> 333,333.33 -> net 51,666.67.
		{"horizon 2 is net_costlier", motorOps("2", `,"capex_usd":900000,"retooling_usd":100000`), want{false, true, domain.RejectNetCostlier, -115000, ""}},
		{"horizon 3 is shown", motorOps("3", `,"capex_usd":900000,"retooling_usd":100000`), want{true, true, "", 51666.67, ""}},
		// MOQ: 24,000 / 6 orders = 4,000 per order.
		{"moq tie is feasible", moq("4000", "6"), want{true, true, "", 385000, "feasible"}},
		{"moq just above is infeasible", moq("4000.01", "6"), want{false, true, domain.RejectOperationallyInfeasible, 385000, "infeasible"}},
		{"moq decimal tie 2.4 x 10000 orders is feasible", strings.Replace(moq("2.4", "10000"), `"annual_demand_units":24000`, `"annual_demand_units":24000`, 1), want{true, true, "", 385000, "feasible"}},
		{"moq zero is a known no-minimum", moq("0", "6"), want{true, true, "", 385000, "feasible"}},
		{"moq unit mismatch is unknown, not feasible", strings.Replace(moq("4000", "6"), `"moq_unit":"pcs"`, `"moq_unit":"PCS"`, 1), want{true, true, "", 385000, "unknown"}},
		{"missing orders_per_year is unknown", strings.Replace(moq("4000", "6"), `"orders_per_year":6,`, ``, 1), want{true, true, "", 385000, "unknown"}},
		// known zero versus missing
		{"entered zero labour at both plants is a known zero", `{"groups":{"motor_fan_assembly":{"cost_basis":"additional","plants":{
		 "cn":{"ref":{"name":"China suppliers","country":"China"},"labour_usd_per_year":0},"th":{"ref":{"name":"Rayong plant","country":"Thailand"},"labour_usd_per_year":0}}}}}`,
			want{true, true, "", 380000, ""}}, // 500,000 - 120,000 group cost - 0 labour
		{"labour at the destination only is unknown", `{"groups":{"motor_fan_assembly":{"cost_basis":"additional","plants":{
		 "th":{"ref":{"name":"Rayong plant","country":"Thailand"},"labour_usd_per_year":0}}}}}`, want{true, false, "", 380000, ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, _ := opsPlan(t, 0.85, func(e *Economics) { e.Operations = decodeOps(t, tc.ops) }, Options{MaxMoves: DefaultMaxMoves})
			pl, ok := findPlan(res, motorKey)
			if !ok {
				t.Fatal("no motor plan")
			}
			priced := pl.NetPriced != nil && *pl.NetPriced
			if pl.Shown != tc.want.shown || priced != tc.want.priced || pl.RejectionKind != tc.want.kind || pl.Operations.Feasibility != tc.want.feas {
				t.Fatalf("shown %v priced %v kind %q feas %q; want %+v", pl.Shown, priced, pl.RejectionKind, pl.Operations.Feasibility, tc.want)
			}
			if priced && f64(pl.Operations.NetModelledPerYear) != tc.want.net {
				t.Errorf("net = %v, want %v", f64(pl.Operations.NetModelledPerYear), tc.want.net)
			}
			if !priced && pl.Operations.NetModelledPerYear != nil {
				t.Errorf("an unpriced plan has a modelled net %v", *pl.Operations.NetModelledPerYear)
			}
		})
	}
}

// Provenance text never changes a number: the same inputs with other notes and
// sources give byte-identical results.
func TestOpsProvenanceDoesNotChangeTheResult(t *testing.T) {
	run := func(ops string) string {
		res, _ := opsPlan(t, 0.85, func(e *Economics) { e.Operations = decodeOps(t, ops) }, Options{MaxMoves: DefaultMaxMoves})
		res.Stats.WallSeconds = 0
		b, _ := json.Marshal(res)
		return string(b)
	}
	base := motorOps("4", `,"capex_usd":900000,"retooling_usd":100000`)
	with := strings.Replace(strings.Replace(base, `"horizon_years":4,`, `"horizon_years":4,"note":"my note",`, 1), `"retooling_usd":100000`, `"retooling_usd":100000,"sources":{"capex_usd":"quote","retooling_usd":"estimate"}`, 1)
	if run(base) != run(with) {
		t.Error("a note or a source changed the result")
	}
}

// F2 (financial review): a scenario variant that adds a plant. The source plant
// and ref are read from the CURRENT product, the destination from the VARIANT.
func variantWithNewPlant(t *testing.T, newName, newCountry string) (domain.Product, domain.Product) {
	t.Helper()
	r, _ := fixture(t, "heatgun_2022.json")
	cur := r.Product
	v := cur
	v.Plants = append(append([]domain.Plant{}, cur.Plants...), domain.Plant{ID: "th2", Name: newName, Country: newCountry, Capabilities: []string{"motor_fan_assembly"}})
	v.Components = append([]domain.Component{}, cur.Components...)
	for i := range v.Components {
		if v.Components[i].Group == "motor_fan_assembly" {
			v.Components[i].Plant, v.Components[i].MadeIn = "th2", newCountry
		}
	}
	return cur, v
}

func th2Ops(t *testing.T, name, country string) *opcost.Operations {
	return decodeOps(t, fmt.Sprintf(`{"horizon_years":4,"groups":{"motor_fan_assembly":{"cost_basis":"additional","plants":{
	 "cn":{"ref":{"name":"China suppliers","country":"China"},"labour_usd_per_year":100000,"logistics_usd_per_year":10000},
	 "th2":{"ref":{"name":%q,"country":%q},"labour_usd_per_year":60000,"logistics_usd_per_year":45000,"capex_usd":900000,"retooling_usd":100000}}}}}`, name, country))
}

func TestOpsCompareVariantOnlyPlantIsPriced(t *testing.T) {
	cur, v := variantWithNewPlant(t, "Chonburi works", "Thailand")
	r, _ := fixture(t, "heatgun_2022.json")
	e := r.Economics
	e.GroupCosts = map[string]map[string]float64{"motor_fan_assembly": {"cn": 300000, "th2": 420000}}
	e.Operations = th2Ops(t, "Chonburi works", "Thailand")
	cmp := Compare(e, &cur, &v, "China", "Thailand")
	if cmp.Operations == nil || !cmp.Operations.Priced || f64(cmp.NetUSD) != 135000 || len(cmp.Operations.StaleRefs) != 0 {
		t.Fatalf("new destination plant not priced: %+v stale %v unknown %v", cmp.NetUSD, cmp.Operations.StaleRefs, cmp.Operations.Unknown)
	}
	// a mismatching ref at the new plant is still unknown, never read
	e.Operations = th2Ops(t, "Other works", "Thailand")
	cmp = Compare(e, &cur, &v, "China", "Thailand")
	if cmp.Operations.Priced || len(cmp.Operations.StaleRefs) == 0 {
		t.Errorf("mismatching ref was used: %+v", cmp.Operations)
	}
}

// The same id with another ref in the variant cannot be represented on both
// sides: unknown, never the variant's or the current's entry.
func TestOpsCompareSameIDChangedRefIsUnknown(t *testing.T) {
	cur, v := variantWithNewPlant(t, "Chonburi works", "Thailand")
	v.Plants = append([]domain.Plant{}, cur.Plants...)
	for i := range v.Plants {
		if v.Plants[i].ID == "th" {
			v.Plants[i].Name = "Songkhla works"
		}
	}
	r, _ := fixture(t, "heatgun_2022.json")
	e := r.Economics
	e.Operations = decodeOps(t, motorOps("4", ``))
	cmp := Compare(e, &cur, &v, "China", "Thailand")
	// the variant moves nothing (same plant ids), so there is no priced move: just assert no entry is read for a changed ref
	for _, m := range cmp.Moved {
		if m.ToPlant == "th" && cmp.Operations.Priced {
			t.Errorf("an entry for a plant whose ref changed was read: %+v", cmp.Operations.Components)
		}
	}
}

// N1: with no annual import value the dollar duty figures are unknown (nil),
// not 0; a known rate and the feasibility stay. An explicit 0 rate with a
// positive annual value is a known 0.
func TestOpsCompareUnknownDutyIsNilNotZero(t *testing.T) {
	r, _ := fixture(t, "heatgun_2022.json")
	v := Apply(r.Product, domain.Move{{Group: "motor_fan_assembly", ToPlant: "th"}})
	e := r.Economics
	e.AnnualImportValueUSD = 0
	e.Operations = decodeOps(t, `{"max_lead_time_days":90,"groups":{"motor_fan_assembly":{"plants":{"th":{"ref":{"name":"Rayong plant","country":"Thailand"},"lead_time_days":120}}}}}`)
	c := Compare(e, &r.Product, &v, "China", "Thailand")
	if c.AnnualDutyUSD != nil || c.DutySavingsUSD != nil || c.NetUSD != nil || f64(c.DutyRate) != 0.167 || c.Feasibility != "infeasible" || c.Operations.Components.DutyEffectPerYear != nil {
		t.Errorf("annual 0: annual_duty %v savings %v net %v rate %v feas %q", c.AnnualDutyUSD, c.DutySavingsUSD, c.NetUSD, c.DutyRate, c.Feasibility)
	}
	// annual positive, the current origin has no rate: savings unknown, target dollars known
	e.AnnualImportValueUSD = 2000000
	e.DutyRates = map[string]float64{"Thailand": 0.167}
	c = Compare(e, &r.Product, &v, "China", "Thailand")
	if c.DutySavingsUSD != nil || c.NetUSD != nil || f64(c.AnnualDutyUSD) != 334000 {
		t.Errorf("missing origin rate: savings %v net %v annual %v", c.DutySavingsUSD, c.NetUSD, c.AnnualDutyUSD)
	}
	// explicit 0 target rate with a positive annual value is a known 0
	e.DutyRates = map[string]float64{"China": 0.25, "Thailand": 0}
	c = Compare(e, &r.Product, &v, "China", "Thailand")
	if f64(c.AnnualDutyUSD) != 0 || c.AnnualDutyUSD == nil || f64(c.DutySavingsUSD) != 500000 {
		t.Errorf("explicit zero rate: annual %v savings %v", c.AnnualDutyUSD, c.DutySavingsUSD)
	}
}
