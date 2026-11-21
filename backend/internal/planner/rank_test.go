package planner

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/verifier"
)

// motorEngine says Thailand whenever the motor-fan assembly moves there.
type motorEngine struct{}

func (motorEngine) Predict(_ context.Context, c domain.Case) (engine.Result, error) {
	origin := "China"
	if strings.Contains(c.Facts, "In Thailand, the electric motor, fan and motor support will be produced in-house") {
		origin = "Thailand"
	}
	return engine.Result{Prediction: domain.Prediction{Origin: origin, Confidence: 0.9}}, nil
}

// scriptedVerifier answers each heat-gun move with a set check and
// counter-ruling, keyed by the groups the move puts in Thailand.
type scriptedVerifier struct {
	mu      sync.Mutex
	verdict map[string]domain.Verdict
}

func heatGunMoveKey(facts string) string {
	var k []string
	for _, g := range []struct{ id, phrase string }{
		{"motor", "the electric motor, fan and motor support will be produced"},
		{"pcba", "the printed circuit board assembly (PCBA) will be produced"},
		{"switch", "the switch, wiring and support ring, heat insulator and fasteners will be produced"},
		{"cord", "the power cord, knob and back cover will be produced"},
	} {
		if strings.Contains(facts, "In Thailand, "+g.phrase) {
			k = append(k, g.id)
		}
	}
	return strings.Join(k, "+")
}

func (s *scriptedVerifier) Verify(_ context.Context, t verifier.Task) (verifier.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.verdict[heatGunMoveKey(t.FactsB)]
	if !ok {
		v = domain.Verdict{Confidence: 0.5}
	}
	return verifier.Result{Verdict: v, Usage: llm.Usage{}}, nil
}

func supportedAt(conf float64, counter string) domain.Verdict {
	return domain.Verdict{PrecedentSupported: true, SupportingRuling: "N347421", CounterRuling: counter, Confidence: conf}
}

// The checks of D332 run 2 (after the split), replayed: under net alone the
// cheaper moves at 0.92 rank first; under D341 CBP's own move (0.94, no
// counter-ruling) is the only clear-margin plan and ranks first.
func d332Run2() map[string]domain.Verdict {
	return map[string]domain.Verdict{
		"motor":             {PrecedentSupported: true, SupportingRuling: "N345859", Confidence: 0.90},
		"motor+cord":        supportedAt(0.92, "N345859"),
		"motor+switch":      supportedAt(0.92, "N345859"),
		"motor+pcba":        supportedAt(0.92, "N345859"),
		"motor+pcba+cord":   supportedAt(0.94, ""),
		"motor+pcba+switch": supportedAt(0.93, "N345859"),
	}
}

func rankHeatGun(t *testing.T, verdicts map[string]domain.Verdict, opts Options) Result {
	t.Helper()
	r, _ := fixture(t, "heatgun_2022.json")
	tmpls, _ := LoadTemplates()
	opts.PlanConfidence, opts.Concurrency = 0.92, 2
	res, err := New(tmpls, motorEngine{}, &scriptedVerifier{verdict: verdicts}, opts).Plan(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func keys(plans []domain.Plan) []string {
	var out []string
	for _, p := range plans {
		out = append(out, p.Move.Key())
	}
	return out
}

func TestShownPlansRankClearMarginFirstThenNet(t *testing.T) {
	res := rankHeatGun(t, d332Run2(), Options{MaxMoves: DefaultMaxMoves})
	got := keys(res.Plans)
	want := []string{
		"motor_fan_assembly@th+pcba@th+cord_knobs_covers@th", // clear margin, $305k
		"motor_fan_assembly@th+cord_knobs_covers@th",         // at the bar, $365k
		"motor_fan_assembly@th+switch_cord_minor_parts@th",   // $360k
		"motor_fan_assembly@th+pcba@th",                      // $320k
		"motor_fan_assembly@th+pcba@th+switch_cord_minor_parts@th",
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("order = %v\nwant    %v", got, want)
	}
	if res.Plans[0].Tier != domain.TierClearMargin {
		t.Errorf("CBP's move tier = %q", res.Plans[0].Tier)
	}
	for _, p := range res.Plans[1:] {
		if p.Tier != domain.TierAtBar {
			t.Errorf("%s tier = %q, want at_bar", p.Move.Key(), p.Tier)
		}
	}
	if res.Ranking == nil || res.Ranking.Rule != RankingRule || res.Ranking.Bar != 0.92 || res.Ranking.ClearFrom != 0.93 {
		t.Errorf("ranking = %+v", res.Ranking)
	}
	// The backtest reads the top plan: CBP's move, all 6 changed parts and
	// none it kept (the split was made after seeing N357771, D330).
	bt := res.Backtest
	if bt == nil || len(bt.NotMovedByPlan) != 0 || len(bt.AlsoMovesKept) != 0 || len(bt.MovedByPlan) != 6 {
		t.Errorf("backtest = %+v", bt)
	}
}

func TestACounterRulingKeepsAPlanAtTheBar(t *testing.T) {
	v := d332Run2()
	v["motor+pcba+cord"] = supportedAt(0.94, "N345859") // as in D332 run 1
	res := rankHeatGun(t, v, Options{MaxMoves: DefaultMaxMoves})
	for _, p := range res.Plans {
		if p.Tier != domain.TierAtBar {
			t.Errorf("%s tier = %q, want at_bar", p.Move.Key(), p.Tier)
		}
	}
	// With every plan at the bar the order is net, as before D341.
	if got := res.Plans[0].Move.Key(); got != "motor_fan_assembly@th+cord_knobs_covers@th" {
		t.Errorf("top = %s", got)
	}
}

func TestSmallestSupportedMoveIsALabelNotASortKey(t *testing.T) {
	res := rankHeatGun(t, d332Run2(), Options{MaxMoves: DefaultMaxMoves})
	var marked []string
	for _, p := range res.Plans {
		if p.SmallestSupported {
			marked = append(marked, p.Move.Key())
		}
	}
	// motor + PCBA moves 4 parts; motor + cord and motor + switch move 6.
	if len(marked) != 1 || marked[0] != "motor_fan_assembly@th+pcba@th" {
		t.Errorf("smallest = %v", marked)
	}
	parts := map[string]int{}
	for _, p := range res.Plans {
		parts[p.Move.Key()] = p.PartsMoved
	}
	if parts["motor_fan_assembly@th+pcba@th"] != 4 || parts["motor_fan_assembly@th+pcba@th+cord_knobs_covers@th"] != 7 {
		t.Errorf("parts moved = %v", parts)
	}
	// One shown plan: nothing to compare, no label.
	one := rankHeatGun(t, map[string]domain.Verdict{"motor+pcba": supportedAt(0.96, "")}, Options{MaxMoves: DefaultMaxMoves})
	if len(one.Plans) != 1 || one.Plans[0].SmallestSupported {
		t.Errorf("single plan = %+v", one.Plans)
	}
}

func TestClearMarginOption(t *testing.T) {
	// A wider margin puts CBP's 0.94 move at the bar too.
	res := rankHeatGun(t, d332Run2(), Options{MaxMoves: DefaultMaxMoves, ClearMargin: 0.03})
	if res.Ranking.ClearFrom != 0.95 || res.Plans[0].Tier != domain.TierAtBar {
		t.Errorf("ranking %+v, top %s %q", res.Ranking, res.Plans[0].Move.Key(), res.Plans[0].Tier)
	}
}

func TestSmallestSupportedTieBreaks(t *testing.T) {
	p := func(parts int, conf, net float64) domain.Plan {
		return domain.Plan{PartsMoved: parts, NetYear: net, Verdict: &domain.Verdict{Confidence: conf}}
	}
	cases := []struct {
		plans []domain.Plan
		want  int
	}{
		{nil, -1},
		{[]domain.Plan{p(2, 0.9, 1)}, -1},
		{[]domain.Plan{p(3, 0.96, 10), p(2, 0.92, 5)}, 1},
		{[]domain.Plan{p(2, 0.92, 10), p(2, 0.94, 5)}, 1},
		{[]domain.Plan{p(2, 0.94, 5), p(2, 0.94, 10)}, 1},
		{[]domain.Plan{p(2, 0.94, 10), p(2, 0.94, 10)}, 0},
	}
	for i, c := range cases {
		if got := SmallestSupported(c.plans); got != c.want {
			t.Errorf("case %d: %d, want %d", i, got, c.want)
		}
	}
}

func TestOperationsModeRanksTierThenPricedThenNet(t *testing.T) {
	yes, no := true, false
	pl := func(name string, conf float64, priced *bool, net float64) domain.Plan {
		return domain.Plan{Description: name, NetYear: net, NetPriced: priced, Shown: true,
			Verdict: &domain.Verdict{PrecedentSupported: true, SupportingRuling: "N347421", Confidence: conf}}
	}
	res := &Result{Objective: newObjective(), Plans: []domain.Plan{
		pl("at bar, priced, best net", 0.92, &yes, 300),
		pl("clear, not priced", 0.96, &no, 900),
		pl("clear, priced", 0.95, &yes, 50),
	}}
	p := New(nil, nil, nil, Options{PlanConfidence: 0.92})
	r, _ := fixture(t, "heatgun_2022.json")
	p.order(r, res)
	var got []string
	for _, x := range res.Plans {
		got = append(got, x.Description)
	}
	if strings.Join(got, "|") != "clear, priced|clear, not priced|at bar, priced, best net" {
		t.Errorf("order = %v", got)
	}
}
