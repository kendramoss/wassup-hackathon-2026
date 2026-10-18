package planner

import (
	"context"
	"fmt"
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
	// Current setup first, then the precedent-supported move below the bar.
	if res.Rejected[0].RejectionKind != domain.RejectCurrentSetup {
		t.Errorf("first rejected kind = %q", res.Rejected[0].RejectionKind)
	}
	below := res.Rejected[1]
	if below.RejectionKind != domain.RejectBelowConfidence || below.Move.Key() != "motor_fan_assembly@th+pcba@th" ||
		!strings.Contains(below.RejectionReason, "confidence 0.90 is below 0.95") || below.Verdict == nil || !below.Verdict.PrecedentSupported {
		t.Errorf("second rejected = %+v", below)
	}
	for _, pl := range res.Rejected[2:] {
		if pl.RejectionKind != domain.RejectOtherOrigin {
			t.Errorf("remaining rejections should be engine misses: %+v", pl.RejectionKind)
		}
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
		if pl.RejectionKind == "" {
			t.Errorf("rejected plan without kind: %+v", pl.Description)
		}
		if pl.Verdict != nil {
			withVerdict++
			if pl.RejectionKind != domain.RejectNoPrecedent {
				t.Errorf("kind = %q, want no_supporting_precedent", pl.RejectionKind)
			}
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

func TestPlanWithProgressReportsEveryMove(t *testing.T) {
	r, _ := fixture(t, "heatgun_2022.json")
	tmpls, _ := LoadTemplates()
	var events []Event
	res, err := New(tmpls, &fakeEngine{}, &fakeVerifier{supported: true, confidence: 0.97}, Options{PlanConfidence: 0.95, Concurrency: 8}).
		PlanWithProgress(context.Background(), r, func(e Event) { events = append(events, e) })
	if err != nil {
		t.Fatal(err)
	}
	first, last := events[0], events[len(events)-1]
	if first.Kind != EventStarted || len(first.Moves) != 6 || first.Target != "Thailand" || last.Kind != EventDone || last.Result == nil || last.Done != 6 {
		t.Fatalf("first/last = %+v / %+v", first, last)
	}
	kinds := map[string]int{}
	doneKeys := map[string]bool{}
	for i, e := range events {
		kinds[e.Kind]++
		if e.Total != 6 || (i > 0 && e.Done < events[i-1].Done) {
			t.Errorf("event %d = %+v", i, e)
		}
		if e.Kind == EventMoveDone {
			doneKeys[e.Move] = true
			if e.Plan == nil || e.Plan.Move.Key() != e.Move {
				t.Errorf("move_done without its plan: %+v", e)
			}
			if e.Plan.Move.Key() == "motor_fan_assembly@th+pcba@th" && (e.Plan.NetYear != 320000 || !e.Plan.Shown) {
				t.Errorf("streamed plan economics = %+v", e.Plan)
			}
		}
	}
	if kinds[EventBaseline] != 1 || kinds[EventMoveStarted] != 6 || len(doneKeys) != 6 {
		t.Errorf("event kinds = %v", kinds)
	}
	if len(res.Plans) != 1 || res.Plans[0].NetYear != 320000 || len(last.Result.Plans) != 1 {
		t.Errorf("result changed with progress: %+v", res.Plans)
	}
}

// flakyEngine is fakeEngine, but the provider fails for moves whose facts
// contain fail (with err), and for the baseline when failBase.
type flakyEngine struct {
	fakeEngine
	fail     string
	failBase bool
	err      error
}

func (f *flakyEngine) Predict(ctx context.Context, c domain.Case) (engine.Result, error) {
	if (f.fail != "" && strings.Contains(c.Facts, f.fail) && strings.Contains(c.ID, "@")) || (f.failBase && strings.HasSuffix(c.ID, "/baseline")) {
		return engine.Result{}, f.err
	}
	return f.fakeEngine.Predict(ctx, c)
}

func TestPlanSurvivesAProviderOutageOnOneMove(t *testing.T) {
	r, _ := fixture(t, "heatgun_2022.json")
	tmpls, _ := LoadTemplates()
	for _, err := range []error{
		fmt.Errorf("llm: %w after 1 attempts: x", llm.ErrUnavailable),
		&llm.APIError{Provider: "vertex", StatusCode: 504},
		&llm.APIError{Provider: "vertex", StatusCode: 429},
	} {
		e := &flakyEngine{fail: "the switch, power cord", err: err}
		res, perr := New(tmpls, e, &fakeVerifier{supported: true, confidence: 0.97}, Options{PlanConfidence: 0.95, Concurrency: 3}).Plan(context.Background(), r)
		if perr != nil {
			t.Fatalf("%v: one failed move should not fail the search: %v", err, perr)
		}
		failed := 0
		for _, pl := range res.Rejected {
			if pl.RejectionKind == domain.RejectCheckFailed {
				failed++
				if !strings.Contains(pl.RejectionReason, "search again") || pl.Prediction.Origin != "" || pl.NetYear != 0 || pl.AddedCostYear != 0 {
					t.Errorf("failed move = %+v", pl)
				}
			}
		}
		if failed == 0 || len(res.Plans) != 1 || res.Plans[0].Move.Key() != "motor_fan_assembly@th+pcba@th" {
			t.Errorf("%v: failed %d, plans %+v", err, failed, res.Plans)
		}
	}
}

func TestPlanStillFailsOnBadRequestsAndBaselineOutage(t *testing.T) {
	r, _ := fixture(t, "heatgun_2022.json")
	tmpls, _ := LoadTemplates()
	for name, e := range map[string]*flakyEngine{
		"400 on a move":      {fail: "the switch, power cord", err: &llm.APIError{Provider: "vertex", StatusCode: 400}},
		"outage on baseline": {failBase: true, err: &llm.APIError{Provider: "vertex", StatusCode: 504}},
	} {
		if _, err := New(tmpls, e, &fakeVerifier{supported: true, confidence: 0.97}, Options{PlanConfidence: 0.95, Concurrency: 3}).Plan(context.Background(), r); err == nil {
			t.Errorf("%s: the search should fail", name)
		}
	}
}
