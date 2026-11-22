package planner

import (
	"context"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/blob"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/verifier"
)

// D540: a search on a request whose regime has an engine predicts the
// current setup and every move with that engine, and never the general one.
func TestPlanUsesTheRegimeEngine(t *testing.T) {
	req, _ := fixture(t, "heatgun_2022.json")
	tmpls, _ := LoadTemplates()
	general, part102 := &fakeEngine{}, &fakeEngine{}
	v := &fakeVerifier{supported: true, confidence: 0.97}
	p := New(tmpls, general, v, Options{PlanConfidence: 0.95, Concurrency: 3,
		Engines: map[string]Predictor{"usmca_part102": part102}})

	req.Regime = "usmca_part102"
	res, err := p.Plan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(general.cases) != 0 {
		t.Fatalf("general engine called %d times on a Part 102 search", len(general.cases))
	}
	if want := res.Stats.MovesSearched + 1; len(part102.cases) != want || res.Stats.EngineCalls != want {
		t.Fatalf("Part 102 engine calls = %d (stats %d), want the current setup and every move (%d)", len(part102.cases), res.Stats.EngineCalls, want)
	}

	// No regime, or a regime with no engine: the general engine, as before.
	for _, regime := range []string{"", "substantial_transformation", "textile_102_21"} {
		general.cases, part102.cases = nil, nil
		req.Regime = regime
		if _, err := p.Plan(context.Background(), req); err != nil {
			t.Fatal(err)
		}
		if len(part102.cases) != 0 || len(general.cases) == 0 {
			t.Errorf("regime %q: general %d, Part 102 %d calls", regime, len(general.cases), len(part102.cases))
		}
	}
}

// The same search with and without the regime engine differs only in which
// engine predicts: with an engine that agrees, the result is the same.
func TestRegimeEngineKeepsTheSearchOtherwiseUnchanged(t *testing.T) {
	req, _ := fixture(t, "heatgun_2022.json")
	tmpls, _ := LoadTemplates()
	v := &fakeVerifier{supported: true, confidence: 0.97}
	plain, err := New(tmpls, &fakeEngine{}, v, Options{PlanConfidence: 0.95}).Plan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	req.Regime = "usmca_part102"
	routed, err := New(tmpls, &fakeEngine{}, v, Options{PlanConfidence: 0.95,
		Engines: map[string]Predictor{"usmca_part102": &fakeEngine{}}}).Plan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(plain.Plans) != len(routed.Plans) || len(plain.Rejected) != len(routed.Rejected) || plain.Stats.VerifierRuns != routed.Stats.VerifierRuns {
		t.Errorf("plain %d/%d/%d, routed %d/%d/%d", len(plain.Plans), len(plain.Rejected), plain.Stats.VerifierRuns,
			len(routed.Plans), len(routed.Rejected), routed.Stats.VerifierRuns)
	}
}

// The estimate reads the cache of the engine the search will use.
func TestEstimatePeeksTheRegimeEngine(t *testing.T) {
	req, _ := fixture(t, "heatgun_2022.json")
	dir := blob.Dir{Dir: t.TempDir()}
	answer := `{"origin_country":"Thailand","essential_character":"motor","decisive_step":"step","flip":null,"confidence":0.95,"reasoning":"r"}`
	gf, uf := llm.FakeText(answer), llm.FakeText(answer)
	general := engine.NewCache(engine.New(gf, nil, engine.Options{Model: "m"}), dir)
	part102 := engine.NewCache(engine.New(uf, nil, engine.Options{Model: "m", System: engine.USMCASystem, Version: engine.USMCAVersion}), dir)
	tmpls, _ := LoadTemplates()
	p := New(tmpls, general, verifier.NewCacheIn(&fakeVerifier{supported: true, confidence: 0.97}, dir, "m"),
		Options{PlanConfidence: 0.92, Engines: map[string]Predictor{"usmca_part102": part102}})
	req.Regime = "usmca_part102"
	if _, err := p.Plan(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if len(gf.Calls()) != 0 || len(uf.Calls()) == 0 {
		t.Fatalf("general %d, Part 102 %d model calls", len(gf.Calls()), len(uf.Calls()))
	}
	e, err := p.Estimate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if e.EngineCached != e.EngineCalls {
		t.Errorf("Part 102 search: %d of %d predictions found cached", e.EngineCached, e.EngineCalls)
	}
	req.Regime = "" // the general engine's cache is empty: a different prompt version
	if e, err = p.Estimate(context.Background(), req); err != nil || e.EngineCached != 0 {
		t.Errorf("general search after a Part 102 one: %d cached, %v", e.EngineCached, err)
	}
}
