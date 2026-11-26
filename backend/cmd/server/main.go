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
//	POST /v1/products/parts-hts-suggest  candidate codes per part (on unless PARTS_HTS_SUGGEST=off, D380)
//	POST|GET /v1/admin/api-keys, DELETE /v1/admin/api-keys/{id}
//	                             broker API keys (API_KEYS=on, admin token)
package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/calibrate"
	"log"
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
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/costest"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/cross"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/demos"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/duty"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/envelope"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/euorigin"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalsummary"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/extract"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/familycheck"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/hts"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/httpapi"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jurisdiction"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/logsafe"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/metering"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/nearmiss"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/nporigin"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/placement"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/ratelimit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/spend"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/sso"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/triage"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/ukorigin"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/verifier"
)

func main() {
	smoke := flag.Bool("smoke", false, "make one live engine-model call and exit")
	flag.Parse()

	// The config is loaded first because the log policy is built from it (the
	// configured model ids, the startup values) and must be complete before the
	// one handler is made. A config error is logged through the same safe
	// logger, with a zero config (every model id masked).
	cfg, cfgErr := config.Load()
	logger, errorLog, err := newLogger(os.Stdout, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fatal: the log handler could not be built; refusing to log unprotected")
		os.Exit(1)
	}
	slog.SetDefault(logger) // so a stray slog.Default() or log.Print is also behind the allowlist
	if cfgErr == nil {
		cfgErr = run(logger, errorLog, cfg, *smoke)
	}
	if cfgErr != nil {
		// Only the error CLASS is logged: the text of a startup error can name
		// a path or echo a provider or store message. See the residual-risk list
		// in the #285 wiring report.
		logger.Error("fatal", "err", logsafe.Err(cfgErr))
		if hint := credentialHint(cfgErr); hint != "" {
			fmt.Fprintln(os.Stderr, hint)
		}
		os.Exit(1)
	}
}

// credentialHint is the first-run fix for the most common Vertex failure
// (#146, #163): signed in to gcloud, but without the separate application-
// default credentials. Returns "" for unrelated errors. Kept pure so the hint
// can be unit-tested without starting a server.
func credentialHint(err error) string {
	if err == nil || !strings.Contains(err.Error(), "could not find default credentials") {
		return ""
	}
	return "\nVertex AI needs application-default credentials: run `gcloud auth application-default login`.\nFor offline UI work with no model calls, set LLM_PROVIDER=fake."
}

func run(logger *slog.Logger, errorLog *log.Logger, cfg config.Config, smoke bool) error {
	logger.Info("config", "summary", cfg.String())
	// #420: a missing auth/audit flag must not be invisible just because the
	// service starts. The allowlisted logger logs the message only.
	for _, w := range cfg.Warnings {
		logger.Warn("production config", "warning", w)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	base, err := newBase(ctx, cfg)
	if err != nil {
		return err
	}
	// The smoke check stays unmetered, as before. Otherwise the spend meter is
	// built first so it can sit on each raw client (see buildModels).
	var meter *spend.Meter
	var cases store.Cases
	if !smoke {
		if cases, err = newCases(context.Background(), cfg, logger); err != nil {
			return err
		}
		meter = newMeter(cfg, cases, logger)
	}
	model, engineModel, err := buildModels(ctx, cfg, base, meter, logger, smoke)
	if err != nil {
		return err
	}
	if smoke {
		return runSmoke(ctx, model, cfg)
	}

	deps, err := newDeps(cfg, model, engineModel, cases, meter, logger)
	if err != nil {
		return err
	}
	if deps.Audit, err = newAudit(ctx, cfg, cases, logger); err != nil {
		return err
	}
	if cfg.UsageMetering {
		deps.Metering = newMetering(cases, meter, logger)
	}
	deps.AuditExport = cfg.AuditExport && deps.Audit != nil
	if deps.Retention, err = newRetention(ctx, cfg, cases, logger); err != nil {
		return err
	}
	srv := newServer(":"+cfg.Port, httpapi.NewHandler(logger, httpapi.Info{Provider: model.Provider(), Version: os.Getenv("K_REVISION")}, deps), errorLog)
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

// Server timeouts (D137). ReadTimeout bounds reading a whole request,
// headers and body (the largest body is a 5 MB upload); IdleTimeout closes
// idle keep-alive connections. There is no WriteTimeout: predict and plan
// stream server-sent events for longer than any fixed write deadline, so
// handlers bound their own work instead (model call timeouts, and a
// deadline on the cheap routes).
const (
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	idleTimeout       = 120 * time.Second
	maxHeaderBytes    = 64 << 10
)

func newServer(addr string, h http.Handler, errorLog *log.Logger) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
		// net/http's own error lines (a handler panic with its value and stack,
		// TLS and connection errors) become one constant record of class
		// stdlog. The bridge never recovers: http.ErrAbortHandler and a
		// committed stream behave exactly as net/http defines.
		ErrorLog: errorLog,
	}
}

// rateCounterPrune is how often each instance deletes ended rate-limit
// windows, rateCounterKeep how long an ended window is kept, and
// rateCounterBatch how many one pass deletes at most (D137).
const (
	rateCounterPrune = 10 * time.Minute
	rateCounterKeep  = 5 * time.Minute
	rateCounterBatch = 500
)

// pruneRateCounters keeps the rate_limits collection bounded when no
// Firestore TTL policy deletes ended windows: every rateCounterPrune it
// deletes windows that ended more than rateCounterKeep ago, a batch at a
// time, up to ten batches. Failures are logged and retried next time.
func pruneRateCounters(fc *store.FirestoreCounters, logger *slog.Logger) {
	t := time.NewTicker(rateCounterPrune)
	defer t.Stop()
	for range t.C {
		for range 10 {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			n, err := fc.Prune(ctx, time.Now().Add(-rateCounterKeep), rateCounterBatch)
			cancel()
			if err != nil {
				logger.Warn("rate counters not pruned", "err", logsafe.Err(err))
				break
			}
			if n < rateCounterBatch {
				break
			}
		}
	}
}

// buildModels wraps the raw provider client into the shared model and the
// engine model. The spend meter (D59) is installed exactly once per raw
// client, directly over it and below pace, retry and fallback: every provider
// attempt is charged once at its own model's price, including attempts that
// fail or return an unusable answer after reporting usage. Nothing above this
// layer meters again. A nil meter leaves the clients unmetered.
func buildModels(ctx context.Context, cfg config.Config, base llm.Model, meter *spend.Meter, logger *slog.Logger, smoke bool) (model, engineModel llm.Model, err error) {
	// One gate for every client, including the engine's fallback Vertex
	// client, so a plan search's moves do not call the model together (D78).
	gate := llm.NewGate(cfg.LLMMaxInFlight, cfg.LLMCoolDown)
	paced := llm.WithPace(meter.Attempts(base), cfg.LLMCallGap, gate)
	if cfg.LLMCallGap > 0 || cfg.LLMMaxInFlight > 0 {
		logger.Info("model calls paced", "max_in_flight", cfg.LLMMaxInFlight, "gap_after_overload", cfg.LLMCallGap.String(), "cool_down", cfg.LLMCoolDown.String())
	}
	model = llm.WithLogging(llm.WithRetry(paced, cfg.LLMMaxRetries, time.Second), logger)
	if smoke {
		return model, nil, nil
	}
	engineModel, err = newEngineModel(ctx, cfg, paced, model, gate, meter, logger)
	return model, engineModel, err
}

// newMeter builds the daily spend meter, on Firestore when cases are there so
// all instances share one total.
func newMeter(cfg config.Config, cases store.Cases, logger *slog.Logger) *spend.Meter {
	var spendStore spend.Store = spend.NewMemory()
	if fs, ok := cases.(*store.Firestore); ok {
		spendStore = fs.Spend()
	}
	if cfg.DailySpendCapUSD > 0 {
		logger.Info("daily model spend cap", "usd", cfg.DailySpendCapUSD)
	}
	return spend.New(spendStore, cfg.DailySpendCapUSD).WithClientShare(cfg.DailySpendClientShare)
}

// newMetering builds per-workspace monthly metering (#886) on the cases'
// database, and has the daily spend meter tell it every model charge.
func newMetering(cases store.Cases, meter *spend.Meter, logger *slog.Logger) *metering.Meter {
	var ms metering.Store = metering.NewMemory()
	if fs, ok := cases.(*store.Firestore); ok {
		ms = fs.Metering()
	}
	m := metering.New(ms)
	if meter != nil {
		meter.Observe(m.Charge)
	}
	logger.Info("workspace usage metering on")
	return m
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
			// Import from an image or scan: a fixed, clearly labelled draft
			// (the fake model does not read the file).
			if req.SchemaName == extract.MediaSchemaName {
				return llm.Response{Text: extract.SimulatedMediaAnswer, Usage: llm.Usage{InputTokens: 1000, OutputTokens: 100, TotalTokens: 1100}}, nil
			}
			// Per-part HTS codes (D380): each part's first ruling code, with a
			// reason that says the fake model did not judge it.
			if req.SchemaName == hts.PartsSchemaName && len(req.Messages) > 0 {
				return llm.Response{Text: hts.SimulatedPartsAnswer(req.Messages[0].Content), Usage: llm.Usage{InputTokens: 1000, OutputTokens: 100, TotalTokens: 1100}}, nil
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
func newEngineModel(ctx context.Context, cfg config.Config, base, shared llm.Model, gate *llm.Gate, meter *spend.Meter, logger *slog.Logger) (llm.Model, error) {
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
		fb = llm.WithPace(meter.Attempts(v), cfg.LLMCallGap, gate)
	default:
		fb = base // already metered, so it is not metered again
	}
	logger.Info("engine fallback on", "model", cfg.FallbackEngineModel, "location", cfg.FallbackEngineLocation, "primary_attempts", cfg.EngineAttempts, "fast_path", cfg.FallbackFastPath)
	return llm.WithFallback(primary, llm.WithLogging(fb, logger), cfg.FallbackEngineModel), nil
}

// newDeps loads the corpus and builds engine, router, verifier and planner.
// model is the shared client, already metered at the raw client (see
// buildModels). engineModel is the engine's client, which may
// fall back to another model.
func newDeps(cfg config.Config, model, engineModel llm.Model, cases store.Cases, meter *spend.Meter, logger *slog.Logger) (httpapi.Deps, error) {
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
	generalIdx := evidenceBM25(pool.Rulings()) // also serves GET /v1/rulings?q= (search_rulings over MCP)
	eng := engine.New(engineModel, generalIdx,
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
		textileEng = engine.New(engineModel, evidenceBM25(tpool), opts)
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
		// The production Part 102 prompt: usmca-v5, a short 102.11 checklist (D602).
		uopts := engine.USMCAProduction()
		uopts.Model, uopts.K, uopts.MaxOutputTokens = cfg.EngineModel, cfg.PrecedentK, cfg.MaxOutputTokens
		usmcaEng = engine.New(engineModel, evidenceBM25(upool.Rulings()), uopts)
		// Most USMCA precedents (2015-2022) are not in labels.jsonl; the API
		// looks them up here too so their details are not lost.
		precedents = httpapi.PrecedentStores{pool, upool}
		logger.Info("USMCA engine on", "precedents", upool.Len(), "prompt", uopts.Version, "fast_path", cfg.USMCAFastPath)
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
		pharmaEng = engine.New(engineModel, evidenceBM25(ppool), engine.Options{Model: cfg.EngineModel, K: cfg.PrecedentK,
			MaxOutputTokens: cfg.MaxOutputTokens, System: engine.PharmaSystem, Version: engine.PharmaVersion})
		logger.Info("pharma engine on", "precedents", len(ppool), "prompt", engine.PharmaVersion, "fast_path", cfg.PharmaFastPath)
	}
	// The 19 CFR 102.20 rule check (D363): display only, on by default.
	var ruleCheck *tariffshift.Rules
	if cfg.RuleCheck {
		if ruleCheck, err = tariffshift.Default(); err != nil {
			return httpapi.Deps{}, err
		}
	}
	// Canada's marking rules (SOR/94-23, D720): a rules check on its own
	// route, on with the rule check.
	var canadaMarking *tariffshift.Rules
	markets := jurisdiction.Default()
	if cfg.RuleCheck {
		if canadaMarking, err = tariffshift.CanadaRules(); err != nil {
			return httpapi.Deps{}, err
		}
		// Canada is a rules-only market (D725): its cases get the SOR/94-23
		// check from the product's codes, always reviewed.
		if err = markets.Register(httpapi.CanadaAuthority(canadaMarking)); err != nil {
			return httpapi.Deps{}, err
		}
	}
	// EU and UK non-preferential origin list rules (D760-D766): rules checks
	// on their own routes, on with the rule check. Both markets are
	// rules-only: their cases get the check from the product's codes,
	// always reviewed, with no follow-ups.
	var euOrigin, ukOrigin *nporigin.Rules
	if cfg.RuleCheck {
		if euOrigin, err = euorigin.Rules(); err != nil {
			return httpapi.Deps{}, err
		}
		if ukOrigin, err = ukorigin.Rules(); err != nil {
			return httpapi.Deps{}, err
		}
		for _, a := range []jurisdiction.Authority{httpapi.OriginRulesAuthority(jurisdiction.EU, euOrigin), httpapi.OriginRulesAuthority(jurisdiction.UK, ukOrigin)} {
			if err = markets.Register(a); err != nil {
				return httpapi.Deps{}, err
			}
		}
	}
	// Mexico is a rules-only market (D914): its cases get the
	// non-preferential origin check of the Acuerdo DOF 30-08-1994 from the
	// product's codes, always reviewed.
	if cfg.RuleCheck {
		mexicoOrigin, err := tariffshift.MexicoRules()
		if err != nil {
			return httpapi.Deps{}, err
		}
		if err = markets.Register(httpapi.MexicoAuthority(mexicoOrigin)); err != nil {
			return httpapi.Deps{}, err
		}
	}
	// South Korea and Australia are RCEP Parties: their cases get the RCEP
	// rules check from the product's codes, always reviewed.
	if cfg.RuleCheck {
		for _, a := range []jurisdiction.Authority{httpapi.RCEPAuthority(jurisdiction.KR, "Korea Customs Service"), httpapi.RCEPAuthority(jurisdiction.AU, "Australian Border Force")} {
			if err = markets.Register(a); err != nil {
				return httpapi.Deps{}, err
			}
		}
	}
	japan, err := japanAuthority(cfg, engineModel, meter, logger)
	if err != nil {
		return httpapi.Deps{}, err
	}
	if japan.Engine != nil {
		if err := markets.Register(japan); err != nil {
			return httpapi.Deps{}, err
		}
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
	infra, err := newInfra(context.Background(), os.Getenv, logger)
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
		verdictBlobs = envelope.Wrap(fs.Verdicts(), infra.blobKey)
		ver = verifier.NewCacheIn(ver, verdictBlobs, cfg.VerifierModel)
		engineBlobs = envelope.Wrap(fs.Blobs(store.EngineCollection), infra.blobKey)
		logger.Info("verifier verdicts and engine predictions cached in Firestore", "collections", []string{store.VerdictCollection, store.EngineCollection})
	case config.VerdictsFile:
		if err := os.MkdirAll(cfg.VerifierCacheDir, 0o755); err != nil {
			return httpapi.Deps{}, fmt.Errorf("verifier: cache dir: %w", err)
		}
		verdictBlobs = envelope.Wrap(verifier.DirBlobs{Dir: cfg.VerifierCacheDir}, infra.blobKey)
		ver = verifier.NewCacheIn(ver, verdictBlobs, cfg.VerifierModel)
		dir := filepath.Join(filepath.Dir(filepath.Clean(cfg.VerifierCacheDir)), "engine-cache")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return httpapi.Deps{}, fmt.Errorf("engine cache dir: %w", err)
		}
		engineBlobs = envelope.Wrap(blob.Dir{Dir: dir}, infra.blobKey)
		logger.Info("verifier verdicts and engine predictions cached", "store", config.CasesFile)
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
	if usmca != nil {
		// Plan searches on Part 102 cases predict with the Part 102 engine
		// (D540), from facts that state the product's codes (D602).
		planOpts.Engines = map[string]planner.Predictor{router.RegimeUSMCA: usmca}
		planOpts.CodesInFacts = map[string]bool{router.RegimeUSMCA: true}
	}
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
		logger.Info("auto-accept bars by subfamily", "subfamilies", len(b.Bars), "global", cfg.FastPathConfidence)
	}
	var calib *calibrate.Map
	if cfg.Calibrated() {
		m, err := calibrate.Load()
		if err != nil {
			return httpapi.Deps{}, err
		}
		calib = &m
		logger.Info("calibrated confidence on", "model", m.Model, "prompt", m.EnginePrompt, "fast_path", cfg.CalibratedFastPath)
	}
	var htsSuggester httpapi.HTSSuggester
	var partsSuggester httpapi.PartsHTSSuggester
	var current hts.Current
	var products *hts.Suggester
	if cfg.HTSSuggest || cfg.PartsHTSSuggest {
		// Subheadings are kept only when in today's HTS (D92).
		mfn := liveMFN
		if mfn == nil {
			mfn = duty.NewLive(cross.UserAgent, cfg.DutyLiveMFNTTL, cfg.DutyLiveMFNTimeout)
		}
		current = func(ctx context.Context, sub string) bool {
			_, err := mfn.MFN(ctx, sub)
			return err == nil || !errors.Is(err, duty.ErrNoRate) // keep it when the HTS is unreachable
		}
		s, err := newProductSuggester(cfg, model, dutyTable)
		switch {
		case err != nil && cfg.HTSSuggest:
			return httpapi.Deps{}, err
		case err != nil:
			logger.Warn("parts hts suggest: no product index, so no code for the good", "index", cfg.HTSIndex)
		default:
			// Candidates from a live CROSS search, kept to subheadings in
			// today's HTS (D92); the local index is the fallback.
			products = s.WithCROSS(crossClient, current)
		}
	}
	if cfg.HTSSuggest {
		n, subs := products.Coverage()
		logger.Info("hts suggest on", "source", "cross", "fallback_rulings", n, "fallback_subheadings", subs, "index", cfg.HTSIndex)
		htsSuggester = products
	}
	// Per-part codes for the 19 CFR 102.20 rule check (D380): on unless
	// PARTS_HTS_SUGGEST=off. Rate-limited as light and spend-capped like the
	// other model routes. Unset, a missing index turns it off with a warning.
	if cfg.PartsHTSSuggest {
		x, err := hts.LoadPartIndex(cfg.PartHTSIndex)
		switch {
		case err != nil && cfg.PartsHTSSuggestSet:
			return httpapi.Deps{}, err
		case err != nil:
			logger.Warn("parts hts suggest off: index not loaded", "index", cfg.PartHTSIndex)
		default:
			ps := hts.NewParts(model, cfg.RouterModel, x, products, current)
			_, rulings := ps.Coverage()
			logger.Info("parts hts suggest on", "fallback_rulings", rulings, "index", cfg.PartHTSIndex)
			partsSuggester = ps
		}
	}
	// Teams (D68): workspaces live next to the cases (Firestore on Cloud Run).
	var users teams.Verifier
	var workspaces teams.Store
	if cfg.Teams {
		if users, err = teams.NewGoogle(cfg.TeamsAudience); err != nil {
			return httpapi.Deps{}, err
		}
		workspaces = teams.NewMemory()
		kind := config.CasesMemory
		if fs, ok := cases.(*store.Firestore); ok {
			workspaces, kind = fs.Workspaces(), config.CasesFirestore
		}
		logger.Info("teams on: Google sign-in and workspaces", "audience", cfg.TeamsAudience, "workspaces", kind)
	}
	// Single sign-on (#868): OIDC and SAML for workspaces with a verified
	// email domain. Its session tokens are accepted beside Google's.
	var ssoSvc *sso.Service
	if cfg.SSO {
		keys, err := sso.NewKeys([]byte(cfg.SSOSessionKey))
		if err != nil {
			return httpapi.Deps{}, err
		}
		var conns sso.Store = sso.NewMemoryStore()
		var state sso.OneTime = sso.NewMemoryOneTime(nil)
		kind := config.CasesMemory
		if fs, ok := cases.(*store.Firestore); ok {
			conns, state, kind = fs.SSOConnections(), fs.SSOOneTime(), config.CasesFirestore
		}
		if ssoSvc, err = sso.New(sso.Config{SiteURL: cfg.SSOSiteURL, SessionTTL: cfg.SSOSessionTTL}, keys, conns, state, workspaces); err != nil {
			return httpapi.Deps{}, err
		}
		users = ssoSvc.Verifier(users)
		logger.Info("sso on: OIDC and SAML sign-in", "store", kind, "session_ttl", cfg.SSOSessionTTL.String())
	}
	if cfg.StrictAuth && !cfg.Teams && !cfg.APIKeys {
		logger.Warn("STRICT_AUTH is on but neither TEAMS nor API_KEYS is: only the admin token can reach private case routes")
	}
	// API keys for brokers (D69), in Firestore when cases are there.
	var keys *apikey.Keys
	if cfg.APIKeys {
		var ks apikey.Store = apikey.NewMemory()
		kind := config.CasesMemory
		if fs, ok := cases.(*store.Firestore); ok {
			ks, kind = fs.APIKeys(), config.CasesFirestore
		}
		keys = apikey.New(ks)
		logger.Info("API keys on", "store", kind)
	}
	// Rate limits shared by all instances (D70); local keeps each
	// instance's own buckets.
	var rateStore ratelimit.Store
	switch cfg.RateLimitStore {
	case config.RateFirestore:
		fc, err := store.NewFirestoreCounters(context.Background(), cfg.GCPProject, cfg.FirestoreDatabase)
		switch {
		case err != nil && cfg.RateLimitStoreDefaulted:
			// Chosen by the Cloud Run default, not asked for: start with
			// this instance's limits rather than not at all (D137).
			logger.Warn("shared rate limiter unavailable; using this instance's limits", "err", logsafe.Err(err))
		case err != nil:
			return httpapi.Deps{}, err
		default:
			rateStore = fc
			go pruneRateCounters(fc, logger)
			logger.Info("rate limits shared in Firestore", "collection", store.RateLimitCollection)
		}
	case config.RateMemory:
		rateStore = ratelimit.NewMemory()
		logger.Info("rate limits counted in fixed windows in memory")
	}
	d := httpapi.Deps{
		Demos:                 ds,
		Engine:                predictor,
		Router:                router.New(model, cfg.RouterModel, 2000),
		Family:                familycheck.New(model, cfg.RouterModel, tmpls),
		CostEstimator:         costest.New(model, cfg.RouterModel),
		Extractor:             extract.New(model, cfg.EngineModel),
		MediaExtractor:        extract.New(model, cfg.EngineModel),
		Clarifier:             clarify.New(model, cfg.EngineModel, 3000),
		Precedents:            precedents,
		RulingSearch:          generalIdx,
		Planner:               planner.New(tmpls, predictor, ver, planOpts),
		Rulings:               crossClient,
		Cases:                 placement.Wrap(cases, infra.placement),
		Templates:             tmpls,
		Summary:               evalsummary.Load,
		FastPathConf:          cfg.FastPathConfidence,
		Calibration:           calib,
		CalibratedFastPath:    cfg.CalibratedFastPath,
		FallbackFastPath:      cfg.FallbackFastPath,
		PlanConfidence:        cfg.PlanConfidence,
		Provider:              model.Provider(),
		EngineModel:           cfg.EngineModel,
		RouterModel:           cfg.RouterModel,
		VerifierModel:         cfg.VerifierModel,
		VerifierPromptVersion: map[string]string{"v1": verifier.PromptVersion, "v2": verifier.PromptVersion2, "v3": verifier.PromptVersion3}[cfg.VerifierPrompt],
		Logger:                logger,
		Limits:                httpapi.Limits{BasicPerMin: cfg.RateBasicPerMin, LightPerMin: cfg.RateLightPerMin, HeavyPerMin: cfg.RateHeavyPerMin, GlobalPerMin: cfg.RateGlobalPerMin, LookupPerMin: cfg.RateLookupPerMin, MaxInFlight: cfg.RateMaxInFlight, AuthFailuresPerMin: cfg.RateAuthFailuresPerMin, ReviewPerMin: cfg.RateReviewPerMin},
		RequireBearer:         cfg.RequireBearer,
		TrustedProxyCIDRs:     cfg.TrustedProxyCIDRs,
		RateStore:             rateStore,
		CasesListLimit:        cfg.CasesListLimit,
		CaseOwnership:         cfg.CaseOwnership,
		StrictAuth:            cfg.StrictAuth,
		BOMVersions:           cfg.BOMVersions == "on",
		ERPImport:             cfg.ERPImport,
		BrokerWorkflow:        cfg.BrokerWorkflow == "on",
		Teams:                 cfg.Teams,
		Users:                 users,
		Workspaces:            workspaces,
		SSO:                   ssoSvc,
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
		RuleCheck:             ruleCheck,
		CanadaMarking:         canadaMarking,
		EUOrigin:              euOrigin,
		UKOrigin:              ukOrigin,
		Jurisdictions:         markets,
		Pharma:                pharma,
		PharmaFastPath:        cfg.PharmaFastPath,
		FamilyBars:            familyBars,
		HTS:                   htsSuggester,
		PartsHTS:              partsSuggester,
		ExportCFR177:          cfg.ExportCFR177,
		ATIGACheck:            cfg.ATIGACheck,
		Checks:                []httpapi.HealthCheck{httpapi.StoreCheck(cases), httpapi.ModelCheck("model", model, cfg.RouterModel)},
	}
	if infra.statusPage {
		d.Status = &httpapi.StatusPage{Checks: d.Checks, SLOs: httpapi.DefaultSLOs}
	}
	// The agent's what_if uses the same dispatch and triage as a saved
	// scenario (#319). It is built from d BEFORE d.Agent is set: the predictor
	// copies d (engines, router, bars; the cache-wrapped engines above), and
	// the agent needs the predictor while the HTTP handler needs the agent.
	return withAgent(d, model, cmp.Or(cfg.CaseAgentModel, cfg.EngineModel), searcher), nil
}

// withAgent returns d with the case agent attached. The predictor is built
// from d as it is here, before Agent is set (see the comment in newDeps).
func withAgent(d httpapi.Deps, model llm.Model, modelID string, s verifier.Searcher) httpapi.Deps {
	d.Agent = caseagent.New(model, modelID, s, newAskPredictor(d))
	return d
}

// askPredictor adapts httpapi.CasePredictor to caseagent.Classifier. The agent
// cannot import httpapi (httpapi imports caseagent), so the values are copied
// here. The embedded Predict satisfies caseagent.Predictor.
type askPredictor struct{ *httpapi.CasePredictor }

func newAskPredictor(d httpapi.Deps) askPredictor { return askPredictor{httpapi.NewCasePredictor(d)} }

// Classify runs the saved-scenario dispatch and triage. Regime is the one the
// result's own trace names; an unrecognised trace stays "" (unknown) and is
// never replaced by the router label or by general.
func (p askPredictor) Classify(ctx context.Context, c domain.Case) (caseagent.Classification, error) {
	k, err := p.CasePredictor.Classify(ctx, c)
	if err != nil {
		return caseagent.Classification{}, err
	}
	return caseagent.Classification{Result: k.Result, Regime: httpapi.PredictionRegime(k.Result.Prediction),
		Selected: k.Selected, Scope: string(k.Scope), Specialist: string(k.Specialist),
		Route: k.Triage.Route, Reason: k.Triage.Reason}, nil
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
		logger.Info("cases persisted", "store", config.CasesFile)
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

// evidenceBM25 indexes exactly rs and stamps the index with the revision of
// that same slice (corpus.RevisionOf), computed once at startup. The slice
// must be the final one: textile after WithReasoning, USMCA and pharma as the
// subsets they index, never the Load-time Store.Revision of a wider pool. The
// engine prediction cache keys on this revision and bypasses itself for a
// retriever without one (#316).
func evidenceBM25(rs []domain.Ruling) *retrieval.BM25 {
	return retrieval.NewBM25(rs, engine.PrecedentText).WithRevision(corpus.RevisionOf(rs))
}

// newProductSuggester loads the product-level HTS suggester (D64) over the
// shipped index and facts.
func newProductSuggester(cfg config.Config, model llm.Model, dutyTable *duty.Table) (*hts.Suggester, error) {
	x, err := hts.LoadIndex(cfg.HTSIndex)
	if err != nil {
		return nil, err
	}
	return hts.New(model, cfg.RouterModel, x, cfg.FactsDir, func(code string) bool { _, ok := dutyTable.Line(code); return ok })
}
