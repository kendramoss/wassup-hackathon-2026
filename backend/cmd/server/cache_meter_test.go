package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/httpapi"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/spend"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// flakyStore fails Add while down, like a spend store outage (#313 / D104).
type flakyStore struct {
	spend.Store
	mu   sync.Mutex
	down bool
}

func (f *flakyStore) set(down bool) { f.mu.Lock(); f.down = down; f.mu.Unlock() }
func (f *flakyStore) Add(ctx context.Context, day string, usd float64) (float64, error) {
	f.mu.Lock()
	down := f.down
	f.mu.Unlock()
	if down {
		return 0, errors.New("spend store unavailable")
	}
	return f.Store.Add(ctx, day, usd)
}

// engineEnv builds the four real engines (origin, textile, USMCA, pharma)
// through buildModels and newDeps, exactly as run does, over the repository's
// corpus, with a local fake provider (no network, no key) and a file cache.
// engineEnvLogger, when set, replaces the discarded logger engineEnv gives the
// production wiring (see logging_test.go). Tests are not parallel.
var engineEnvLogger func(config.Config) *slog.Logger

func engineEnv(t *testing.T, fallbackModel string, base llm.Model, meter *spend.Meter) httpapi.Deps {
	t.Helper()
	data, _ := filepath.Abs("../../data")
	t.Setenv("LLM_PROVIDER", "fake")
	t.Setenv("LABELS_PATH", filepath.Join(data, "labels.jsonl"))
	t.Setenv("CORPUS_FACTS_DIR", filepath.Join(data, "facts"))
	t.Setenv("RAW_DIR", t.TempDir())
	t.Setenv("TEXTILE_RULINGS", filepath.Join(data, "textile_rulings.json"))
	t.Setenv("USMCA_LABELS", filepath.Join(data, "labels_usmca.jsonl"))
	t.Setenv("PHARMA_RULINGS", filepath.Join(data, "pharma_rulings.json"))
	t.Setenv("TEXTILE_ENGINE", "on")
	t.Setenv("USMCA_ENGINE", "on")
	t.Setenv("PHARMA_ENGINE", "on")
	t.Setenv("VERIFIER_CACHE_STORE", "file")
	t.Setenv("VERIFIER_CACHE_DIR", filepath.Join(t.TempDir(), "verdicts"))
	t.Setenv("VERIFIER_SEARCH", "local")
	t.Setenv("NEAR_MISS_ADVICE", "off")
	if fallbackModel == "" {
		t.Setenv("FALLBACK_ENGINE_MODEL", "off")
	} else {
		t.Setenv("FALLBACK_ENGINE_MODEL", fallbackModel)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if engineEnvLogger != nil {
		logger = engineEnvLogger(cfg)
	}
	model, engineModel, err := buildModels(context.Background(), cfg, base, meter, logger, false)
	if err != nil {
		t.Fatal(err)
	}
	deps, err := newDeps(cfg, model, engineModel, store.NewMemory(), meter, logger)
	if err != nil {
		t.Fatal(err)
	}
	return deps
}

func predictors(d httpapi.Deps) map[string]httpapi.Predictor {
	return map[string]httpapi.Predictor{"origin": d.Engine, "textile": d.Textile, "usmca": d.USMCA, "pharma": d.Pharma}
}

func testCase(id string) domain.Case {
	return domain.Case{ID: id, Date: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		Facts: "A cotton knitted garment, cut in one country and sewn in another, imported into the United States."}
}

// A cache miss charges the answering model once; a repeat hit never reaches
// the raw client and leaves the meter unchanged. All four engines are built
// by the production wiring.
func TestEnginesCacheMissChargesOnceAndHitIsFree(t *testing.T) {
	usage := llm.Usage{InputTokens: 10000, OutputTokens: 1000}
	var mu sync.Mutex
	var calls int
	base := &llm.Fake{Respond: func(r llm.Request) (llm.Response, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		return llm.Response{Text: `{"origin_country":"China","confidence":0.5}`, Usage: usage}, nil
	}}
	meter := spend.New(spend.NewMemory(), 1000)
	deps := engineEnv(t, "", base, meter)
	total := func() float64 { _, v := meter.Over(context.Background()); return v }
	for name, p := range predictors(deps) {
		if p == nil {
			t.Fatalf("%s engine not constructed", name)
		}
		beforeCalls, beforeTotal := calls, total()
		res, err := p.Predict(context.Background(), testCase("case_"+name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		model := res.Prediction.Trace.ModelID
		missCalls := calls - beforeCalls
		if missCalls < 1 {
			t.Fatalf("%s: a miss made no model call", name)
		}
		want := float64(missCalls) * evalkit.Cost(evalkit.PromoPrices, model, usage.InputTokens, usage.OutputTokens)
		if got := total() - beforeTotal; math.Abs(got-want) > 1e-9 || got <= 0 {
			t.Fatalf("%s: miss charged %v for %d calls, want %v (model %q)", name, got, missCalls, want, model)
		}
		afterCalls, afterTotal := calls, total()
		hit, err := p.Predict(context.Background(), testCase("case_"+name))
		if err != nil {
			t.Fatalf("%s hit: %v", name, err)
		}
		if !hit.Prediction.Trace.Cached || calls != afterCalls || total() != afterTotal {
			t.Fatalf("%s: repeat was not a free cache hit (cached=%v calls %d->%d total %v->%v)", name, hit.Prediction.Trace.Cached, afterCalls, calls, afterTotal, total())
		}
	}
}

// Through the real engine chain, a primary that reports usage and fails
// over to the fallback charges each model at its own price, once.
func TestEngineFallbackChargesEachModelOnce(t *testing.T) {
	usage := llm.Usage{InputTokens: 10000, OutputTokens: 1000}
	var mu sync.Mutex
	counts := map[string]int{}
	base := &llm.Fake{Respond: func(r llm.Request) (llm.Response, error) {
		mu.Lock()
		counts[r.Model]++
		mu.Unlock()
		if r.Model != "gemini-3.8-flash" {
			return llm.Response{Usage: usage}, &llm.APIError{Provider: "vertex", StatusCode: 429}
		}
		return llm.Response{Text: `{"origin_country":"China","confidence":0.5}`, Usage: usage}, nil
	}}
	meter := spend.New(spend.NewMemory(), 1000)
	deps := engineEnv(t, "gemini-3.8-flash", base, meter)
	if _, err := deps.Engine.Predict(context.Background(), testCase("case_fb")); err != nil {
		t.Fatal(err)
	}
	primary := 0
	for m, n := range counts {
		if m != "gemini-3.8-flash" {
			primary += n
		}
	}
	var primaryModel string
	for m := range counts {
		if m != "gemini-3.8-flash" {
			primaryModel = m
		}
	}
	want := float64(primary)*evalkit.Cost(evalkit.PromoPrices, primaryModel, 10000, 1000) +
		float64(counts["gemini-3.8-flash"])*evalkit.Cost(evalkit.PromoPrices, "gemini-3.8-flash", 10000, 1000)
	_, got := meter.Over(context.Background())
	if counts["gemini-3.8-flash"] != 1 || primary < 2 || math.Abs(got-want) > 1e-9 {
		t.Fatalf("calls %v: total %v want %v", counts, got, want)
	}
}

// #340: a spend-store outage followed by recovery must not reopen a reached
// cap, now with charges arriving from the raw attempt meter.
func TestAttemptChargesKeepOutageDebt(t *testing.T) {
	usage := llm.Usage{InputTokens: 10000, OutputTokens: 1000}
	base := &llm.Fake{Respond: func(r llm.Request) (llm.Response, error) {
		return llm.Response{Text: "{}", Usage: usage}, nil
	}}
	fs := &flakyStore{Store: spend.NewMemory()}
	one := evalkit.Cost(evalkit.PromoPrices, "gemini-3.8-flash", 10000, 1000)
	meter := spend.New(fs, 1.5*one) // reached after two charges
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	model, _, err := buildModels(context.Background(), config.Config{LLMProvider: config.ProviderFake}, base, meter, logger, false)
	if err != nil {
		t.Fatal(err)
	}
	gen := func() {
		if _, err := model.Generate(context.Background(), llm.Request{Model: "gemini-3.8-flash"}); err != nil {
			t.Fatal(err)
		}
	}
	fs.set(true)
	gen()
	gen() // counted locally only: cap reached
	if over, _ := meter.Over(context.Background()); !over {
		t.Fatal("cap not reached during the outage")
	}
	fs.set(false)
	gen() // the store recovers with a smaller total
	if over, total := meter.Over(context.Background()); !over || total < 3*one-1e-9 {
		t.Fatalf("recovery reopened the cap or lost spend: over=%v total=%v want >= %v", over, total, 3*one)
	}
}

// An attempt that fails without reporting usage (an unavailable provider, a
// bad response with no metadata) charges nothing: no invented cost.
func TestAttemptWithoutUsageChargesNothing(t *testing.T) {
	base := &llm.Fake{Respond: func(r llm.Request) (llm.Response, error) {
		return llm.Response{}, &llm.APIError{Provider: "vertex", StatusCode: 503}
	}}
	meter := spend.New(spend.NewMemory(), 100)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Config{FallbackEngineModel: "gemini-3.8-flash", EngineAttempts: 2, LLMProvider: config.ProviderFake}
	_, engineModel, err := buildModels(context.Background(), cfg, base, meter, logger, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engineModel.Generate(context.Background(), llm.Request{Model: "gemini-4-argon"}); err == nil {
		t.Fatal("expected the failure to surface")
	}
	if _, total := meter.Over(context.Background()); total != 0 {
		t.Fatalf("charged %v for attempts that reported no usage", total)
	}
}
