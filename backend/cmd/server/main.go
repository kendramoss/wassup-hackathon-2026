// Command server is the Unde Labs API: origin prediction, plan search and
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
//	POST /v1/products/hts-suggest  candidate HTS subheadings (HTS_SUGGEST=on, D64)
//	POST|GET /v1/admin/api-keys, DELETE /v1/admin/api-keys/{id}
//	                             broker API keys (API_KEYS=on, admin token)
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

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/apikey"
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
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/familycheck"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/hts"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/httpapi"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/nearmiss"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/ratelimit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/spend"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/triage"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/verifier"
)

func main() {
	smoke := flag.Bool("smoke", false, "make one live engine-model call and exit")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger, *smoke); err != nil {
		logger.Error("fatal", "err", err)
		// The most common first-run failure (#146): signed in to gcloud, but
		// without the separate application-default credentials Vertex needs.
		if strings.Contains(err.Error(), "could not find default credentials") {
			fmt.Fprintln(os.Stderr, "\nVertex AI needs application-default credentials: run `gcloud auth application-default login`.\nFor offline UI work with no model calls, set LLM_PROVIDER=fake.")
		}
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

	base, err := newBase(ctx, cfg)
	if err != nil {
		return err
	}
	// One gate for every client, including the engine's fallback Vertex
	// client, so a plan search's moves do not call the model together (D78).
	gate := llm.NewGate(cfg.LLMMaxInFlight, cfg.LLMCoolDown)
	paced := llm.WithPace(base, cfg.LLMCallGap, gate)
	if cfg.LLMCallGap > 0 || cfg.LLMMaxInFlight > 0 {
		logger.Info("model calls paced", "max_in_flight", cfg.LLMMaxInFlight, "gap_after_overload", cfg.LLMCallGap.String(), "cool_down", cfg.LLMCoolDown.String())
	}
	model := llm.WithLogging(llm.WithRetry(paced, cfg.LLMMaxRetries, time.Second), logger)
	if smoke {
		return runSmoke(ctx, model, cfg)
	}
	engineModel, err := newEngineModel(ctx, cfg, paced, model, gate, logger)
	if err != nil {
		return err
	}

	deps, err := newDeps(cfg, model, engineModel, logger)
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

// newBase builds the configured provider with no retry. The router, verifier,
// clarify and agent share a fully retried client; the engine may use a
// shorter retry plus a fallback (D56).
func newBase(ctx context.Context, cfg config.Config) (llm.Model, error) {
	switch cfg.LLMProvider {
	case config.ProviderVertex:
		return llm.NewVertex(ctx, llm.VertexOptions{Project: cfg.GCPProject, Location: cfg.GCPLocation, Timeout: cfg.LLMTimeout})
	case config.ProviderFake:
		// Every call gets the engine's JSON, except the case agent, which
		// gets a plain sentence so Ask shows an answer in local runs.
		return &llm.Fake{Respond: func(req llm.Request) (llm.Response, error) {
			if strings.Contains(req.System, "trade-compliance lead") {
				return llm.Response{Text: "This is the local fake model, so there is no real answer here. With LLM_PROVIDER=vertex the agent reads the case and earlier CBP rulings and answers in plain words."}, nil
			}
			// Report some usage so the daily spend cap (D59) can be exercised locally.
			return llm.Response{Text: `{"origin_country":"China","confidence":0.5}`, Usage: llm.Usage{InputTokens: 1000, OutputTokens: 100, TotalTokens: 1100}}, nil
		}}, nil
	default:
		return nil, fmt.Errorf("unknown LLM provider %q", cfg.LLMProvider)
	}
}

// newEngineModel is the model the origin, textile, USMCA and pharma engines
// call. With FALLBACK_ENGINE_MODEL=off it is the shared retried client. With
// a fallback (default gemini-3.8-flash), the primary is tried EngineAttempts
// times and then the fallback model is called once, from a client in
// FallbackEngineLocation (global).
func newEngineModel(ctx context.Context, cfg config.Config, base, shared llm.Model, gate *llm.Gate, logger *slog.Logger) (llm.Model, error) {
	if cfg.FallbackEngineModel == "" {
		return shared, nil
	}
	primary := llm.WithLogging(llm.WithRetry(base, cfg.EngineAttempts-1, time.Second), logger)
	var fb llm.Model
	// The fallback gets its own budget. A short LLM_TIMEOUT is what makes the
	// primary hit a deadline; the one fallback call still has time to answer.
	fbTimeout := cfg.LLMTimeout
	if fbTimeout < 60*time.Second {
		fbTimeout = 60 * time.Second
	}
	switch cfg.LLMProvider {
	case config.ProviderVertex:
		v, err := llm.NewVertex(ctx, llm.VertexOptions{Project: cfg.GCPProject, Location: cfg.FallbackEngineLocation, Timeout: fbTimeout})
		if err != nil {
			return nil, err
		}
		fb = llm.WithPace(v, cfg.LLMCallGap, gate)
	default:
		fb = base
	}
	logger.Info("engine fallback on", "model", cfg.FallbackEngineModel, "location", cfg.FallbackEngineLocation, "primary_attempts", cfg.EngineAttempts, "fast_path", cfg.FallbackFastPath)
	return llm.WithFallback(primary, llm.WithLogging(fb, logger), cfg.FallbackEngineModel), nil
}

// newDeps loads the corpus and builds engine, router, verifier and planner.
// model is the shared client. engineModel is the engine's client, which may
// fall back to another model.
func newDeps(cfg config.Config, model, engineModel llm.Model, logger *slog.Logger) (httpapi.Deps, error) {
	cases, err := newCases(context.Background(), cfg, logger)
	if err != nil {
		return httpapi.Deps{}, err
	}
	// Every model call counts toward the daily spend cap (D59), on Firestore
	// when cases are there so all instances share one total. The engine client
	// is wrapped on its own: with a fallback it is not the shared client.
	var spendStore spend.Store = spend.NewMemory()
	if fs, ok := cases.(*store.Firestore); ok {
		spendStore = fs.Spend()
	}
	meter := spend.New(spendStore, cfg.DailySpendCapUSD)
	model = meter.Model(model)
	engineModel = meter.Model(engineModel)
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
	eng := engine.New(engineModel, retrieval.NewBM25(pool.Rulings(), engine.PrecedentText),
		engine.Options{Model: cfg.EngineModel, K: cfg.PrecedentK, MaxOutputTokens: cfg.MaxOutputTokens})
	crossClient := cross.NewHTTPClient(cross.UserAgent)
	local, err := verifier.NewLocal(labels, cfg.FactsDir, cfg.RawDir)
	if err != nil {
		return httpapi.Deps{}, err
	}
	// CROSS search needs every query word to match; Relaxed retries shorter
	// queries and fills from the local corpus so checks see evidence (D89).
	var searcher verifier.Searcher = verifier.Relaxed{Primary: verifier.CROSS{Client: crossClient}, Fill: local}
	if cfg.VerifierSearch == "local" {
		searcher = local
	}
	ver := verifier.NewWithPanel(model, searcher, verifier.Options{Model: cfg.VerifierModel, MaxToolCalls: cfg.VerifierMaxToolCalls, MaxOutputTokens: 6000, Prompt: cfg.VerifierPrompt},
		cfg.VerifierPanel == "on", cfg.VerifierPanelLow, cfg.VerifierPanelHigh)
	if cfg.VerifierPanel == "on" {
		logger.Info("precedent-check panel on", "views", []string{verifier.ViewSkeptic, verifier.ViewProduct}, "low", cfg.VerifierPanelLow, "high", cfg.VerifierPanelHigh)
	}
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
		textileEng = engine.New(engineModel, retrieval.NewBM25(tpool, engine.PrecedentText), opts)
		logger.Info("textile engine on", "precedents", len(tpool), "prompt", opts.Version, "fast_path", cfg.TextileFastPath)
	}
	var precedents httpapi.PrecedentStore = pool
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
		usmcaEng = engine.New(engineModel, retrieval.NewBM25(upool.Rulings(), engine.PrecedentText), engine.Options{Model: cfg.EngineModel, K: cfg.PrecedentK,
			MaxOutputTokens: cfg.MaxOutputTokens, System: engine.USMCASystem, Version: engine.USMCAVersion})
		// Most USMCA precedents (2015-2022) are not in labels.jsonl; the API
		// looks them up here too so their details are not lost.
		precedents = httpapi.PrecedentStores{pool, upool}
		logger.Info("USMCA engine on", "precedents", upool.Len(), "prompt", engine.USMCAVersion, "fast_path", cfg.USMCAFastPath)
	}
	var pharmaEng *engine.Engine
	if cfg.PharmaEngine {
		ids, err := corpus.LoadIDList(cfg.PharmaRulings)
		if err != nil {
			return httpapi.Deps{}, err
		}
		var ppool []domain.Ruling
		for _, id := range ids {
			if r, ok := pool.Get(id); ok {
				ppool = append(ppool, r)
			}
		}
		pharmaEng = engine.New(engineModel, retrieval.NewBM25(ppool, engine.PrecedentText), engine.Options{Model: cfg.EngineModel, K: cfg.PrecedentK,
			MaxOutputTokens: cfg.MaxOutputTokens, System: engine.PharmaSystem, Version: engine.PharmaVersion})
		logger.Info("pharma engine on", "precedents", len(ppool), "prompt", engine.PharmaVersion, "fast_path", cfg.PharmaFastPath)
	}
	dutyTable, err := duty.Load()
	if err != nil {
		return httpapi.Deps{}, err
	}
	// Section 301 and forced-labor duties on top of live MFN rates (D90).
	dutyAdd, err := duty.LoadAdditional()
	if err != nil {
		return httpapi.Deps{}, err
	}
	var liveMFN *duty.Live
	if cfg.DutyLiveMFN {
		liveMFN = duty.NewLive(cross.UserAgent, cfg.DutyLiveMFNTTL, cfg.DutyLiveMFNTimeout)
		logger.Info("live MFN duty rates on", "source", duty.DefaultHTSBaseURL, "ttl", cfg.DutyLiveMFNTTL, "timeout", cfg.DutyLiveMFNTimeout)
	}
	tmpls, err := planner.LoadTemplates()
	if err != nil {
		return httpapi.Deps{}, err
	}
	var engineBlobs, verdictBlobs blob.Store
	switch cfg.VerifierCacheStore {
	case config.VerdictsFirestore:
		fs, ok := cases.(*store.Firestore)
		if !ok { // cases elsewhere: open a client just for the verdicts
			if fs, err = store.NewFirestore(context.Background(), cfg.GCPProject, cfg.FirestoreDatabase); err != nil {
				return httpapi.Deps{}, err
			}
		}
		ver = verifier.NewCacheIn(ver, fs.Verdicts(), cfg.VerifierModel)
		engineBlobs, verdictBlobs = fs.Blobs(store.EngineCollection), fs.Verdicts()
		logger.Info("verifier verdicts and engine predictions cached in Firestore", "collections", []string{store.VerdictCollection, store.EngineCollection})
	case config.VerdictsFile:
		vc, err := verifier.NewCache(ver, cfg.VerifierCacheDir, cfg.VerifierModel)
		if err != nil {
			return httpapi.Deps{}, err
		}
		ver = vc
		verdictBlobs = verifier.DirBlobs{Dir: cfg.VerifierCacheDir}
		dir := filepath.Join(filepath.Dir(filepath.Clean(cfg.VerifierCacheDir)), "engine-cache")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return httpapi.Deps{}, fmt.Errorf("engine cache dir: %w", err)
		}
		engineBlobs = blob.Dir{Dir: dir}
		logger.Info("verifier verdicts and engine predictions cached", "dirs", []string{cfg.VerifierCacheDir, dir})
	}
	// Engine predictions are cached in the same store as verdicts (D54).
	var predictor httpapi.Predictor = eng
	var textile, usmca, pharma httpapi.Predictor
	if textileEng != nil {
		textile = textileEng
	}
	if usmcaEng != nil {
		usmca = usmcaEng
	}
	if pharmaEng != nil {
		pharma = pharmaEng
	}
	if engineBlobs != nil {
		predictor = engine.NewCache(eng, engineBlobs)
		if textileEng != nil {
			textile = engine.NewCache(textileEng, engineBlobs)
		}
		if usmcaEng != nil {
			usmca = engine.NewCache(usmcaEng, engineBlobs)
		}
		if pharmaEng != nil { // the key includes the prompt version, so no clash with the other engines
			pharma = engine.NewCache(pharmaEng, engineBlobs)
		}
	}
	planOpts := planner.Options{PlanConfidence: cfg.PlanConfidence, Concurrency: cfg.PlannerConcurrency, MaxMoves: cfg.PlanMaxMoves,
		Panel: cfg.VerifierPanel == "on", MaxInFlight: cfg.LLMMaxInFlight}
	if cfg.NearMissAdvice {
		// Advice on the closest moves just under the plan bar (D81), cached
		// beside the verdicts (keys are prefixed, so no clash).
		var adv nearmiss.Adviser = nearmiss.New(model, cfg.VerifierModel, searcher, 1500)
		if verdictBlobs != nil {
			adv = nearmiss.NewCache(adv, verdictBlobs, cfg.VerifierModel)
		}
		planOpts.NearMiss = adv
		logger.Info("near-miss advice on", "prompt", nearmiss.PromptVersion, "floor", planner.DefaultNearMissFloor, "max_per_search", planner.DefaultNearMissMax)
		if cfg.FollowAdvice {
			planOpts.FollowAdvice, planOpts.Mapper = true, nearmiss.NewMapper(model, cfg.VerifierModel)
			logger.Info("following near-miss advice", "moves_per_search", 1, "map_prompt", nearmiss.MapPromptVersion)
		}
	}
	ds, err := demos.Load()
	if err != nil {
		return httpapi.Deps{}, err
	}
	var familyBars *triage.FamilyBars
	if cfg.FamilyBars() {
		b, err := triage.LoadFamilyBars()
		if err != nil {
			return httpapi.Deps{}, err
		}
		familyBars = &b
		logger.Info("auto-accept bars by subfamily", "bars", b.Bars, "global", cfg.FastPathConfidence)
	}
	var htsSuggester httpapi.HTSSuggester
	if cfg.HTSSuggest {
		x, err := hts.LoadIndex(cfg.HTSIndex)
		if err != nil {
			return httpapi.Deps{}, err
		}
		s, err := hts.New(model, cfg.RouterModel, x, cfg.FactsDir, func(code string) bool { _, ok := dutyTable.Line(code); return ok })
		if err != nil {
			return httpapi.Deps{}, err
		}
		n, subs := s.Coverage()
		logger.Info("hts suggest on", "rulings", n, "subheadings", subs, "index", cfg.HTSIndex)
		htsSuggester = s
	}
	// Teams (D68): workspaces live next to the cases (Firestore on Cloud Run).
	var users teams.Verifier
	var workspaces teams.Store
	if cfg.Teams {
		if users, err = teams.NewGoogle(cfg.TeamsAudience); err != nil {
			return httpapi.Deps{}, err
		}
		workspaces = teams.NewMemory()
		if fs, ok := cases.(*store.Firestore); ok {
			workspaces = fs.Workspaces()
		}
		logger.Info("teams on: Google sign-in and workspaces", "audience", cfg.TeamsAudience, "workspaces", fmt.Sprintf("%T", workspaces))
	}
	// API keys for brokers (D69), in Firestore when cases are there.
	var keys *apikey.Keys
	if cfg.APIKeys {
		var ks apikey.Store = apikey.NewMemory()
		if fs, ok := cases.(*store.Firestore); ok {
			ks = fs.APIKeys()
		}
		keys = apikey.New(ks)
		logger.Info("API keys on", "store", fmt.Sprintf("%T", ks))
	}
	// Rate limits shared by all instances (D70); local keeps each
	// instance's own buckets.
	var rateStore ratelimit.Store
	switch cfg.RateLimitStore {
	case config.RateFirestore:
		fc, err := store.NewFirestoreCounters(context.Background(), cfg.GCPProject, cfg.FirestoreDatabase)
		if err != nil {
			return httpapi.Deps{}, err
		}
		rateStore = fc
		logger.Info("rate limits shared in Firestore", "collection", store.RateLimitCollection)
	case config.RateMemory:
		rateStore = ratelimit.NewMemory()
		logger.Info("rate limits counted in fixed windows in memory")
	}
	return httpapi.Deps{
		Demos:                 ds,
		Engine:                predictor,
		Router:                router.New(model, cfg.RouterModel, 2000),
		Family:                familycheck.New(model, cfg.RouterModel, tmpls),
		Agent:                 caseagent.New(model, cfg.EngineModel, searcher, predictor),
		Extractor:             extract.New(model, cfg.EngineModel),
		Clarifier:             clarify.New(model, cfg.EngineModel, 3000),
		Precedents:            precedents,
		Planner:               planner.New(tmpls, predictor, ver, planOpts),
		Rulings:               crossClient,
		Cases:                 cases,
		Templates:             tmpls,
		Summary:               evalsummary.Load,
		FastPathConf:          cfg.FastPathConfidence,
		FallbackFastPath:      cfg.FallbackFastPath,
		PlanConfidence:        cfg.PlanConfidence,
		Provider:              model.Provider(),
		EngineModel:           cfg.EngineModel,
		RouterModel:           cfg.RouterModel,
		VerifierModel:         cfg.VerifierModel,
		VerifierPromptVersion: map[string]string{"v1": verifier.PromptVersion, "v2": verifier.PromptVersion2, "v3": verifier.PromptVersion3}[cfg.VerifierPrompt],
		Logger:                logger,
		Limits:                httpapi.Limits{LightPerMin: cfg.RateLightPerMin, HeavyPerMin: cfg.RateHeavyPerMin, GlobalPerMin: cfg.RateGlobalPerMin, LookupPerMin: cfg.RateLookupPerMin},
		RateStore:             rateStore,
		CasesListLimit:        cfg.CasesListLimit,
		CaseOwnership:         cfg.CaseOwnership,
		BOMVersions:           cfg.BOMVersions == "on",
		Teams:                 cfg.Teams,
		Users:                 users,
		Workspaces:            workspaces,
		APIKeys:               keys,
		StreamPredict:         cfg.StreamPredictOn(),
		Spend:                 meter,
		AdminToken:            cfg.AdminToken,
		Duty:                  dutyTable,
		LiveMFN:               liveMFN,
		DutyAdd:               dutyAdd,
		Textile:               textile,
		TextileFastPath:       cfg.TextileFastPath,
		USMCA:                 usmca,
		USMCAFastPath:         cfg.USMCAFastPath,
		Pharma:                pharma,
		PharmaFastPath:        cfg.PharmaFastPath,
		FamilyBars:            familyBars,
		HTS:                   htsSuggester,
		ExportCFR177:          cfg.ExportCFR177,
		Checks:                []httpapi.HealthCheck{httpapi.StoreCheck(cases), httpapi.ModelCheck("model", model, cfg.RouterModel)},
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
