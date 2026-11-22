package planner

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/blob"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
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
	req.Product.HTS = "8516.79" // the engine's facts carry the codes; the check's do not (D640)
	dir := blob.Dir{Dir: t.TempDir()}
	answer := `{"origin_country":"Thailand","essential_character":"motor","decisive_step":"step","flip":null,"confidence":0.95,"reasoning":"r"}`
	gf, uf := llm.FakeText(answer), llm.FakeText(answer)
	general := engine.NewCache(engine.New(gf, nil, engine.Options{Model: "m"}), dir)
	part102 := engine.NewCache(engine.New(uf, nil, engine.Options{Model: "m", System: engine.USMCASystem, Version: engine.USMCAVersion}), dir)
	tmpls, _ := LoadTemplates()
	fv := &fakeVerifier{supported: true, confidence: 0.97}
	p := New(tmpls, general, verifier.NewCacheIn(fv, dir, "m"),
		Options{PlanConfidence: 0.92, Engines: map[string]Predictor{"usmca_part102": part102}, CodesInFacts: map[string]bool{"usmca_part102": true}})
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
	if len(fv.tasks) == 0 || e.ChecksCached != len(fv.tasks) || e.ChecksFresh.Max != 0 {
		t.Errorf("Part 102 search: %d checks run, estimate found %d cached and %v fresh", len(fv.tasks), e.ChecksCached, e.ChecksFresh)
	}
	req.Regime = "" // the general engine's cache is empty: a different prompt version
	if e, err = p.Estimate(context.Background(), req); err != nil || e.EngineCached != 0 {
		t.Errorf("general search after a Part 102 one: %d cached, %v", e.EngineCached, err)
	}
}

// D602: a search on a regime in CodesInFacts states the product's own codes
// in the facts of the current setup and of every move, with the move's
// countries; other searches do not, and the estimate reads the same facts.
func TestCodesInFactsOnPart102SearchesOnly(t *testing.T) {
	req, _ := fixture(t, "heatgun_2022.json")
	req.Product.HTS = "8516.79"
	req.Product.Components[0].HTS = "8501.10"
	tmpls, _ := LoadTemplates()
	general, part102 := &fakeEngine{}, &fakeEngine{}
	p := New(tmpls, general, &fakeVerifier{supported: true, confidence: 0.97}, Options{PlanConfidence: 0.95,
		Engines: map[string]Predictor{"usmca_part102": part102}, CodesInFacts: map[string]bool{"usmca_part102": true}})
	req.Regime = "usmca_part102"
	if _, err := p.Plan(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	const note = "Tariff classifications given with the product"
	for _, c := range part102.cases {
		if !strings.Contains(c.Facts, note) || !strings.Contains(c.Facts, "8516.79") {
			t.Fatalf("%s: facts without the product's codes", c.ID)
		}
	}
	req.Regime = ""
	if _, err := p.Plan(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	for _, c := range general.cases {
		if strings.Contains(c.Facts, note) {
			t.Fatalf("%s: a general search states the codes", c.ID)
		}
	}
}

// recordingChecker records the tasks it checks and answers as confByMove.
type recordingChecker struct {
	mu    sync.Mutex
	tasks []verifier.Task
}

func (r *recordingChecker) Verify(ctx context.Context, t verifier.Task) (verifier.Result, error) {
	r.mu.Lock()
	r.tasks = append(r.tasks, t)
	r.mu.Unlock()
	return confByMove{}.Verify(ctx, t)
}

// D640 (#642): the codes paragraph reaches the Part 102 engine only. The
// precedent check, near-miss advice and the stored VariantFacts (the broker
// draft's facts) read the plain rendered facts, as D600 to D602 measured.
func TestAudit638CodesReachOnlyTheEngine(t *testing.T) {
	req, _ := fixture(t, "heatgun_2022.json")
	req.Product.HTS = "8516.79"
	req.Product.Components[0].HTS = "8501.10"
	tmpls, _ := LoadTemplates()
	part102 := &recordingEngine{inner: allThaiEngine{}}
	v := &recordingChecker{}
	adv := &fakeAdvisor{}
	p := New(tmpls, &fakeEngine{}, v, Options{PlanConfidence: 0.92, Concurrency: 3, NearMiss: adv,
		Engines: map[string]Predictor{"usmca_part102": part102}, CodesInFacts: map[string]bool{"usmca_part102": true}})
	req.Regime = "usmca_part102"
	res, err := p.Plan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	const note = "Tariff classifications given with the product"
	if len(part102.cases) == 0 {
		t.Fatal("the Part 102 engine was not called")
	}
	for _, c := range part102.cases {
		if !strings.Contains(c.Facts, note) {
			t.Errorf("%s: the engine's facts lack the codes paragraph", c.ID)
		}
	}
	if len(v.tasks) == 0 || len(adv.tasks) == 0 {
		t.Fatalf("verifier tasks %d, advice tasks %d: the test needs both", len(v.tasks), len(adv.tasks))
	}
	for _, tk := range v.tasks {
		if strings.Contains(tk.FactsA, note) || strings.Contains(tk.FactsB, note) {
			t.Errorf("precedent check reads the codes paragraph (FactsA %v, FactsB %v)", strings.Contains(tk.FactsA, note), strings.Contains(tk.FactsB, note))
		}
	}
	for _, tk := range adv.tasks {
		if strings.Contains(tk.Facts, note) {
			t.Error("near-miss advice reads the codes paragraph")
		}
	}
	all := append(append(append([]domain.Plan{}, res.Plans...), res.Rejected...), res.Baseline)
	for _, pl := range all {
		if pl.VariantFacts == "" || strings.Contains(pl.VariantFacts, note) {
			t.Errorf("%q: VariantFacts (draft facts, near-miss input) = %q", pl.Description, pl.VariantFacts)
		}
	}
	// The plain facts are exactly Render: the precedent check's input is as
	// it was before D602.
	if want := Render(req.Product, tmpls[res.Family]); res.Baseline.VariantFacts != want {
		t.Errorf("baseline VariantFacts differ from Render")
	}
}

type recordingEngine struct {
	mu    sync.Mutex
	inner Predictor
	cases []domain.Case
}

func (r *recordingEngine) Predict(ctx context.Context, c domain.Case) (engine.Result, error) {
	r.mu.Lock()
	r.cases = append(r.cases, c)
	r.mu.Unlock()
	return r.inner.Predict(ctx, c)
}
