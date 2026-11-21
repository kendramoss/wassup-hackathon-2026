package planner

import (
	"context"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/blob"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/verifier"
)

// cachedPlanner is a planner over real engine and verifier caches in a temp
// directory; the engine says Thailand for everything.
func cachedPlanner(t *testing.T, opts Options) (*Planner, *llm.Fake, *fakeVerifier) {
	t.Helper()
	f := llm.FakeText(`{"origin_country":"Thailand","essential_character":"motor","decisive_step":"step","flip":null,"confidence":0.95,"reasoning":"r"}`)
	dir := blob.Dir{Dir: t.TempDir()}
	eng := engine.NewCache(engine.New(f, nil, engine.Options{Model: "m"}), dir)
	fv := &fakeVerifier{supported: true, confidence: 0.97}
	ver := verifier.NewCacheIn(fv, dir, "m")
	tmpls, _ := LoadTemplates()
	if opts.PlanConfidence == 0 {
		opts.PlanConfidence = 0.92
	}
	return New(tmpls, eng, ver, opts), f, fv
}

func TestEstimateFreshSearchMatchesMeasuredRange(t *testing.T) {
	req, _ := fixture(t, "heatgun_2022.json")
	p, f, _ := cachedPlanner(t, Options{MaxInFlight: 4})
	e, err := p.Estimate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Calls()) != 0 {
		t.Fatalf("estimate made %d model calls", len(f.Calls()))
	}
	// Four movable groups since the cord, knobs and covers were split from the
	// switch and wiring (D330): 4 singles and 6 pairs.
	if e.Moves != 10 || e.EngineCalls != 11 || e.EngineCached != 0 || e.ChecksCached != 0 || e.ChecksFresh != (Range{5, 10}) {
		t.Errorf("counts = %+v", e)
	}
	// Fresh searches of the 6-move heat gun measured $0.15–0.22
	// (eval/results/plan-heatgun-2022-*). This search has 10 moves, so the
	// estimate must start no higher than that and reach at least 1.5 times its top.
	if e.CostUSD.Min > 0.22 || e.CostUSD.Max < 0.224*1.5 || e.CostUSD.Max > 0.5 {
		t.Errorf("cost = %+v, want a range from at most $0.22 to at least $0.34", e.CostUSD)
	}
	if e.WallSeconds.Min <= 0 || e.WallSeconds.Max < e.WallSeconds.Min || e.WallSeconds.Max > 150 {
		t.Errorf("wall = %+v", e.WallSeconds)
	}
}

func TestEstimateAfterASearchIsNearlyFree(t *testing.T) {
	req, _ := fixture(t, "heatgun_2022.json")
	p, f, fv := cachedPlanner(t, Options{})
	if _, err := p.Plan(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	calls, checks := len(f.Calls()), len(fv.tasks)
	e, err := p.Estimate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Calls()) != calls || len(fv.tasks) != checks {
		t.Fatal("estimate called the model or the verifier")
	}
	if e.EngineCached != 11 || e.ChecksCached != 10 || e.ChecksFresh != (Range{}) || e.CostUSD != (Range{}) || e.ModelCalls != (Range{}) || e.WallSeconds.Max > 10 {
		t.Errorf("all cached = %+v", e)
	}
}

func TestEstimatePanelAndNearMissRaiseTheCost(t *testing.T) {
	req, _ := fixture(t, "heatgun_2022.json")
	plain, _, _ := cachedPlanner(t, Options{})
	panel, _, _ := cachedPlanner(t, Options{Panel: true, NearMiss: &fakeAdvisor{}})
	a, _ := plain.Estimate(context.Background(), req)
	b, _ := panel.Estimate(context.Background(), req)
	if !(b.CostUSD.Max > a.CostUSD.Max && b.ModelCalls.Max > a.ModelCalls.Max) || !b.Panel || !b.NearMiss || b.NearMissCalls.Max != 2 {
		t.Errorf("plain %+v, panel %+v", a, b)
	}
}

func TestEstimateUnknownFamily(t *testing.T) {
	req, _ := fixture(t, "heatgun_2022.json")
	req.Family = "nope"
	p, _, _ := cachedPlanner(t, Options{})
	if _, err := p.Estimate(context.Background(), req); err == nil {
		t.Error("unknown family accepted")
	}
}
