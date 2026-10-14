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

// fakeEngine says Thailand only when both the motor and the PCBA are Thai.
type fakeEngine struct {
	mu    sync.Mutex
	cases []domain.Case
}

func (f *fakeEngine) Predict(_ context.Context, c domain.Case) (engine.Result, error) {
	f.mu.Lock()
	f.cases = append(f.cases, c)
	f.mu.Unlock()
	origin := "China"
	if strings.Contains(c.Facts, "In Thailand, the electric motor, fan and motor support will be produced in-house") &&
		strings.Contains(c.Facts, "In Thailand, the printed circuit board assembly (PCBA) will be produced in-house") {
		origin = "Thailand"
	}
	return engine.Result{Prediction: domain.Prediction{Origin: origin, Confidence: 0.97, CitedRulings: []string{"N340000"}},
		Usage: llm.Usage{InputTokens: 3000, OutputTokens: 200}}, nil
}

type fakeVerifier struct {
	supported  bool
	confidence float64
	tasks      []verifier.Task
	mu         sync.Mutex
}

func (f *fakeVerifier) Verify(_ context.Context, t verifier.Task) (verifier.Result, error) {
	f.mu.Lock()
	f.tasks = append(f.tasks, t)
	f.mu.Unlock()
	v := domain.Verdict{PrecedentSupported: f.supported, Confidence: f.confidence, CounterRuling: "N340000"}
	if f.supported {
		v.SupportingRuling = "N350001"
	}
	return verifier.Result{Verdict: v, ModelCalls: 5, Steps: make([]verifier.Step, 4), Usage: llm.Usage{InputTokens: 20000, OutputTokens: 900}}, nil
}

func runPlan(t *testing.T, v *fakeVerifier) (Result, *fakeEngine) {
	t.Helper()
	r, _ := fixture(t, "heatgun_2022.json")
	tmpls, _ := LoadTemplates()
	e := &fakeEngine{}
	res, err := New(tmpls, e, v, Options{PlanConfidence: 0.95, Concurrency: 3}).Plan(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	return res, e
}

func TestPlanShowsSupportedMoveAndRejectsTheRest(t *testing.T) {
	v := &fakeVerifier{supported: true, confidence: 0.97}
	res, e := runPlan(t, v)

	if len(res.Plans) != 1 {
		t.Fatalf("shown plans = %d, want 1 (only motor+pcba reaches Thailand): %+v", len(res.Plans), res.Plans)
	}
	best := res.Plans[0]
	if best.Move.Key() != "motor_fan_assembly@th+pcba@th" || !best.Shown || best.Verdict.SupportingRuling != "N350001" {
		t.Errorf("best plan = %+v", best)
	}
	// China 41.7% -> Thailand 16.7% on $2M = $500k; motor +120k, pcba +60k.
	if best.DutyEffectYear != 500000 || best.AddedCostYear != 180000 || best.NetYear != 320000 {
		t.Errorf("economics = %v %v %v", best.DutyEffectYear, best.AddedCostYear, best.NetYear)
	}
	if res.Rejected[0].Description != "No change: keep the current setup" || !strings.Contains(res.Rejected[0].RejectionReason, "China") {
		t.Errorf("first rejected should be the current setup: %+v", res.Rejected[0])
	}
	for _, pl := range res.Rejected[1:] {
		if pl.RejectionReason == "" || pl.Shown {
			t.Errorf("rejected plan without reason: %+v", pl)
		}
	}
	// Only moves the engine says reach Thailand are verified, with the case's
	// own rulings excluded and the case date as the evidence bound.
	if len(v.tasks) != 1 {
		t.Errorf("verifier runs = %d, want 1", len(v.tasks))
	}
	for _, tk := range v.tasks {
		if tk.Before.Format("2006-01-02") != "2026-01-14" || len(tk.Exclude) != 2 || tk.Target != "Thailand" {
			t.Errorf("verifier task = %+v", tk)
		}
	}
	if len(e.cases) != 1+6 || res.Stats.EngineCalls != 7 || res.Stats.VerifierRuns != 1 || res.Stats.ToolCalls != 4 {
		t.Errorf("stats = %+v", res.Stats)
	}
	if res.Stats.InputTokens != 7*3000+20000 {
		t.Errorf("input tokens = %d", res.Stats.InputTokens)
	}
	bt := res.Backtest
	if bt == nil || !strings.Contains(strings.Join(bt.MovedByPlan, ","), "electric motor") ||
		!strings.Contains(strings.Join(bt.NotMovedByPlan, ","), "knob") || !strings.Contains(strings.Join(bt.NotMovedByPlan, ","), "outer shells") {
		t.Errorf("backtest = %+v", bt)
	}
}

func TestPlanNeedsVerifierConfidence(t *testing.T) {
	res, _ := runPlan(t, &fakeVerifier{supported: true, confidence: 0.9})
	if len(res.Plans) != 0 {
		t.Fatalf("plans below verifier confidence must not be shown: %+v", res.Plans)
	}
	found := false
	for _, pl := range res.Rejected {
		if strings.Contains(pl.RejectionReason, "confidence 0.90 is below 0.95") {
			found = true
		}
	}
	if !found {
		t.Error("missing low-confidence rejection reason")
	}
	if res.Backtest == nil || res.Backtest.PlanMoves != "no precedent-supported plan" {
		t.Errorf("backtest = %+v", res.Backtest)
	}
}

func TestPlanUnsupportedMoveIsRejectedWithCounterRuling(t *testing.T) {
	res, _ := runPlan(t, &fakeVerifier{supported: false, confidence: 0.99})
	if len(res.Plans) != 0 {
		t.Fatal("unsupported moves must not be shown")
	}
	var withVerdict int
	for _, pl := range res.Rejected {
		if pl.Verdict != nil {
			withVerdict++
			if pl.Verdict.CounterRuling != "N340000" {
				t.Errorf("counter ruling missing: %+v", pl.Verdict)
			}
		}
	}
	if withVerdict != 1 {
		t.Errorf("rejected with verdict = %d, want 1", withVerdict)
	}
}

func TestPlanUnknownFamily(t *testing.T) {
	r, _ := fixture(t, "heatgun_2022.json")
	r.Family = "spaceships"
	tmpls, _ := LoadTemplates()
	if _, err := New(tmpls, &fakeEngine{}, &fakeVerifier{}, Options{}).Plan(context.Background(), r); err == nil {
		t.Fatal("want ErrUnknownFamily")
	}
}

func TestPlanUsesKnownCurrentOrigin(t *testing.T) {
	r, _ := fixture(t, "heatgun_2022.json")
	tmpls, _ := LoadTemplates()
	// An engine that thinks everything is Thai: without the known origin no
	// move would be a change.
	e := &thaiEngine{}
	v := &fakeVerifier{supported: true, confidence: 0.97}
	res, err := New(tmpls, e, v, Options{PlanConfidence: 0.95, Concurrency: 2}).Plan(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if res.CurrentOrigin == nil || res.CurrentOrigin.Country != "China" || res.Baseline.Prediction.Origin != "Thailand" {
		t.Errorf("current origin %+v, engine baseline %s", res.CurrentOrigin, res.Baseline.Prediction.Origin)
	}
	if !strings.Contains(res.Rejected[0].RejectionReason, "CBP held China (N328954); engine predicts Thailand") {
		t.Errorf("baseline reason = %q", res.Rejected[0].RejectionReason)
	}
	for _, tk := range v.tasks {
		if tk.KnownOriginA != "China" || tk.KnownRulingA != "N328954" {
			t.Errorf("verifier task missing known origin: %+v", tk)
		}
	}
	if len(res.Plans) == 0 || res.Plans[0].DutyEffectYear != 500000 {
		t.Errorf("economics should start from the known China origin: %+v", res.Plans)
	}
}

type thaiEngine struct{}

func (thaiEngine) Predict(context.Context, domain.Case) (engine.Result, error) {
	return engine.Result{Prediction: domain.Prediction{Origin: "Thailand", Confidence: 0.8}}, nil
}
