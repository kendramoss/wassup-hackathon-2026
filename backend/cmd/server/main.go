// Command server is the Origin Proof API: origin prediction, plan search and
// precedent verification for CBP substantial-transformation cases.
//
// Configuration comes from the environment (see internal/config). Flags:
//
//	-smoke  make one live call to the engine model and print provider, model,
//	        tokens and latency as JSON, then exit
//
// Endpoints (contract: docs/api/openapi.yaml):
//
//	GET  /health                 liveness (not /healthz: Cloud Run reserves paths ending in "z")
//	POST /v1/cases/predict       origin prediction
//	GET  /v1/cases               case list, newest first
//	GET  /v1/cases/{id}          a case
//	POST /v1/cases/{id}/plan     plan search
//	POST /v1/cases/{id}/export   draft ruling request
//	POST /v1/cases/{id}/answers  answers to follow-up questions; re-predicts
//	GET  /v1/rulings/{id}        ruling text with rulings.cbp.gov link
//	GET  /v1/eval/summary        results-page numbers
//	GET  /v1/demos[/{id}]        demo cases for the UI
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/blob"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/caseagent"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/clarify"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/cross"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/demos"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/duty"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalsummary"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/extract"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/httpapi"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/spend"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/verifier"
)

func main() {
	smoke := flag.Bool("smoke", false, "make one live engine-model call and exit")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger, *smoke); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger, smoke bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger.Info("config", "summary", cfg.String())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	model, err := newModel(ctx, cfg, logger)
	if err != nil {
		return err
	}
	if smoke {
		return runSmoke(ctx, model, cfg)
	}

	deps, err := newDeps(cfg, model, logger)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           httpapi.NewHandler(logger, httpapi.Info{Provider: model.Provider(), Version: os.Getenv("K_REVISION")}, deps),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	logger.Info("listening", "port", cfg.Port, "llm_provider", model.Provider())
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server: %w", err)
	}
	return nil
}

// newModel builds the configured provider wrapped with retry and logging.
func newModel(ctx context.Context, cfg config.Config, logger *slog.Logger) (llm.Model, error) {
	var base llm.Model
	switch cfg.LLMProvider {
	case config.ProviderVertex:
		v, err := llm.NewVertex(ctx, llm.VertexOptions{Project: cfg.GCPProject, Location: cfg.GCPLocation, Timeout: cfg.LLMTimeout})
		if err != nil {
			return nil, err
		}
		base = v
	case config.ProviderFake:
		// Every call gets the engine's JSON, except the case agent, which
		// gets a plain sentence so Ask shows an answer in local runs.
		base = &llm.Fake{Respond: func(req llm.Request) (llm.Response, error) {
			if strings.Contains(req.System, "trade-compliance lead") {
				return llm.Response{Text: "This is the local fake model, so there is no real answer here. With LLM_PROVIDER=vertex the agent reads the case and earlier CBP rulings and answers in plain words."}, nil
			}
			// Report some usage so the daily spend cap (D59) can be exercised locally.
			return llm.Response{Text: `{"origin_country":"China","confidence":0.5}`, Usage: llm.Usage{InputTokens: 1000, OutputTokens: 100, TotalTokens: 1100}}, nil
		}}
	default:
		return nil, fmt.Errorf("unknown LLM provider %q", cfg.LLMProvider)
	}
	return llm.WithLogging(llm.WithRetry(base, cfg.LLMMaxRetries, time.Second), logger), nil
}

// newDeps loads the corpus and builds engine, router, verifier and planner.
func newDeps(cfg config.Config, model llm.Model, logger *slog.Logger) (httpapi.Deps, error) {
	cases, err := newCases(context.Background(), cfg, logger)
	if err != nil {
		return httpapi.Deps{}, err
	}
	// Every model call counts toward the daily spend cap (D59), on Firestore
	// when cases are there so all instances share one total.
	var spendStore spend.Store = spend.NewMemory()
	if fs, ok := cases.(*store.Firestore); ok {
		spendStore = fs.Spend()
	}
	meter := spend.New(spendStore, cfg.DailySpendCapUSD)
	model = meter.Model(model)
	if cfg.DailySpendCapUSD > 0 {
		logger.Info("daily model spend cap", "usd", cfg.DailySpendCapUSD)
	}
	labels, err := corpus.LoadLabels(cfg.LabelsPath)
	if err != nil {
		return httpapi.Deps{}, err
	}
	pool, err := corpus.Load(labels, cfg.FactsDir)
	if err != nil {
		return httpapi.Deps{}, err
	}
	if pool.Len() == 0 {
		return httpapi.Deps{}, fmt.Errorf("corpus: no precedent facts in %s (run: go run ./cmd/corpus fetch && go run ./cmd/corpus build)", cfg.FactsDir)
	}
	logger.Info("corpus loaded", "precedents", pool.Len(), "facts_dir", cfg.FactsDir)
	eng := engine.New(model, retrieval.NewBM25(pool.Rulings(), engine.PrecedentText),
		engine.Options{Model: cfg.EngineModel, K: cfg.PrecedentK, MaxOutputTokens: cfg.MaxOutputTokens})
	crossClient := cross.NewHTTPClient("origin-proof (Google Cloud AI Builder Cup 2026 prototype)")
	var searcher verifier.Searcher = verifier.CROSS{Client: crossClient}
	if cfg.VerifierSearch == "local" {
		if searcher, err = verifier.NewLocal(labels, cfg.FactsDir, cfg.RawDir); err != nil {
			return httpapi.Deps{}, err
		}
	}
	var ver verifier.Checker = verifier.New(model, searcher, verifier.Options{Model: cfg.VerifierModel, MaxToolCalls: cfg.VerifierMaxToolCalls, MaxOutputTokens: 6000})
	ver = verifier.NewMajority(ver, cfg.VerifierVotes) // the cache below stores the majority verdict
	var textileEng *engine.Engine
	if cfg.TextileEngine {
		ids, err := corpus.LoadIDList(cfg.TextileRulings)
		if err != nil {
			return httpapi.Deps{}, err
		}
		var tpool []domain.Ruling
		for _, id := range ids {
			if r, ok := pool.Get(id); ok {
				tpool = append(tpool, r)
			}
		}
		opts := engine.Options{Model: cfg.EngineModel, K: cfg.PrecedentK, MaxOutputTokens: cfg.MaxOutputTokens,
			System: engine.TextileSystem, Version: engine.TextileVersion}
		if cfg.TextilePrompt == "v2" {
			reasons, err := corpus.LoadReasoning(cfg.TextileReasoning)
			if err != nil {
				return httpapi.Deps{}, err
			}
			tpool = corpus.WithReasoning(tpool, reasons)
			opts.System, opts.Version, opts.PrecedentReasoning = engine.TextileSystem2, engine.TextileVersion2, true
		}
		textileEng = engine.New(model, retrieval.NewBM25(tpool, engine.PrecedentText), opts)
		logger.Info("textile engine on", "precedents", len(tpool), "prompt", opts.Version, "fast_path", cfg.TextileFastPath)
	}
	var usmcaEng *engine.Engine
	if cfg.USMCAEngine {
		ulabels, err := corpus.LoadLabels(cfg.USMCALabels)
		if err != nil {
			return httpapi.Deps{}, err
		}
		upool, err := corpus.Load(ulabels, cfg.FactsDir)
		if err != nil {
			return httpapi.Deps{}, err
		}
		if upool.Len() == 0 {
			return httpapi.Deps{}, fmt.Errorf("USMCA engine: no ruling in %s has facts in %s", cfg.USMCALabels, cfg.FactsDir)
		}
		usmcaEng = engine.New(model, retrieval.NewBM25(upool.Rulings(), engine.PrecedentText), engine.Options{Model: cfg.EngineModel, K: cfg.PrecedentK,
			MaxOutputTokens: cfg.MaxOutputTokens, System: engine.USMCASystem, Version: engine.USMCAVersion})
		logger.Info("USMCA engine on", "precedents", upool.Len(), "prompt", engine.USMCAVersion, "fast_path", cfg.USMCAFastPath)
	}
	dutyTable, err := duty.Load()
	if err != nil {
		return httpapi.Deps{}, err
	}
	tmpls, err := planner.LoadTemplates()
	if err != nil {
		return httpapi.Deps{}, err
	}
	var engineBlobs blob.Store
	switch cfg.VerifierCacheStore {
	case config.VerdictsFirestore:
		fs, ok := cases.(*store.Firestore)
		if !ok { // cases elsewhere: open a client just for the verdicts
			if fs, err = store.NewFirestore(context.Background(), cfg.GCPProject, cfg.FirestoreDatabase); err != nil {
				return httpapi.Deps{}, err
			}
		}
		ver = verifier.NewCacheIn(ver, fs.Verdicts(), cfg.VerifierModel)
		engineBlobs = fs.Blobs(store.EngineCollection)
		logger.Info("verifier verdicts and engine predictions cached in Firestore", "collections", []string{store.VerdictCollection, store.EngineCollection})
	case config.VerdictsFile:
		vc, err := verifier.NewCache(ver, cfg.VerifierCacheDir, cfg.VerifierModel)
		if err != nil {
			return httpapi.Deps{}, err
		}
		ver = vc
		dir := filepath.Join(filepath.Dir(filepath.Clean(cfg.VerifierCacheDir)), "engine-cache")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return httpapi.Deps{}, fmt.Errorf("engine cache dir: %w", err)
		}
		engineBlobs = blob.Dir{Dir: dir}
		logger.Info("verifier verdicts and engine predictions cached", "dirs", []string{cfg.VerifierCacheDir, dir})
	}
	// Engine predictions are cached in the same store as verdicts (D54).
	var predictor httpapi.Predictor = eng
	var textile, usmca httpapi.Predictor
	if textileEng != nil {
		textile = textileEng
	}
	if usmcaEng != nil {
		usmca = usmcaEng
	}
	if engineBlobs != nil {
		predictor = engine.NewCache(eng, engineBlobs)
		if textileEng != nil {
			textile = engine.NewCache(textileEng, engineBlobs)
		}
		if usmcaEng != nil {
			usmca = engine.NewCache(usmcaEng, engineBlobs)
		}
	}
	ds, err := demos.Load()
	if err != nil {
		return httpapi.Deps{}, err
	}
	return httpapi.Deps{
		Demos:           ds,
		Engine:          predictor,
		Router:          router.New(model, cfg.RouterModel, 2000),
		Agent:           caseagent.New(model, cfg.EngineModel, searcher, predictor),
		Extractor:       extract.New(model, cfg.EngineModel),
		Clarifier:       clarify.New(model, cfg.EngineModel, 3000),
		Precedents:      pool,
		Planner:         planner.New(tmpls, predictor, ver, planner.Options{PlanConfidence: cfg.PlanConfidence, Concurrency: cfg.PlannerConcurrency}),
		Rulings:         crossClient,
		Cases:           cases,
		Templates:       tmpls,
		Summary:         evalsummary.Load,
		FastPathConf:    cfg.FastPathConfidence,
		PlanConfidence:  cfg.PlanConfidence,
		Provider:        model.Provider(),
		EngineModel:     cfg.EngineModel,
		RouterModel:     cfg.RouterModel,
		VerifierModel:   cfg.VerifierModel,
		Logger:          logger,
		Limits:          httpapi.Limits{LightPerMin: cfg.RateLightPerMin, HeavyPerMin: cfg.RateHeavyPerMin, GlobalPerMin: cfg.RateGlobalPerMin, LookupPerMin: cfg.RateLookupPerMin},
		CasesListLimit:  cfg.CasesListLimit,
		CaseOwnership:   cfg.CaseOwnership,
		Spend:           meter,
		AdminToken:      cfg.AdminToken,
		Duty:            dutyTable,
		Textile:         textile,
		TextileFastPath: cfg.TextileFastPath,
		USMCA:           usmca,
		USMCAFastPath:   cfg.USMCAFastPath,
		Checks:          []httpapi.HealthCheck{httpapi.StoreCheck(cases), httpapi.ModelCheck("model", model, cfg.RouterModel)},
	}, nil
}

// newCases builds the configured case store.
func newCases(ctx context.Context, cfg config.Config, logger *slog.Logger) (store.Cases, error) {
	switch {
	case cfg.CasesStore == config.CasesFirestore:
		fs, err := store.NewFirestore(ctx, cfg.GCPProject, cfg.FirestoreDatabase)
		if err != nil {
			return nil, err
		}
		logger.Info("cases persisted in Firestore", "project", cfg.GCPProject, "database", cfg.FirestoreDatabase)
		return fs, nil
	case cfg.CasesStore == config.CasesFile && cfg.CasesDir != "":
		fc, err := store.NewFile(cfg.CasesDir)
		if err != nil {
			return nil, err
		}
		logger.Info("cases persisted", "dir", cfg.CasesDir)
		return fc, nil
	}
	return store.NewMemory(), nil
}

// smokeSchema is a minimal origin answer, enough to prove JSON-schema output
// works end to end.
var smokeSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"origin_country": map[string]any{"type": "string"},
		"confidence":     map[string]any{"type": "number"},
	},
	"required":             []string{"origin_country", "confidence"},
	"additionalProperties": false,
}

func runSmoke(ctx context.Context, model llm.Model, cfg config.Config) error {
	ctx, cancel := context.WithTimeout(ctx, cfg.LLMTimeout)
	defer cancel()
	resp, err := model.Generate(ctx, llm.Request{
		Model:  cfg.EngineModel,
		System: "You are a US customs origin analyst. Answer with JSON only.",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "A heat gun is assembled in Thailand from a Thai housing and heating element " +
			"and a Chinese motor, fan and circuit board. Which country of origin would CBP most likely assign?"}},
		JSONSchema:      smokeSchema,
		SchemaName:      "smoke",
		MaxOutputTokens: cfg.MaxOutputTokens,
	})
	if err != nil {
		return fmt.Errorf("smoke: %w", err)
	}
	var answer map[string]any
	if err := resp.DecodeJSON(&answer); err != nil {
		return fmt.Errorf("smoke: %w", err)
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"llm_provider": resp.Provider, "model_id": resp.ModelID, "usage": resp.Usage,
		"latency_ms": resp.Latency.Milliseconds(), "finish_reason": resp.FinishReason, "answer": answer,
	})
}
