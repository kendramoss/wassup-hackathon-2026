// Package config reads every tunable from the environment, applies documented
// defaults and validates the result once at startup.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// DefaultFastPath is the auto-accept confidence bar: the lowest confidence at
// which cumulative accuracy stays at or above 95% on dev (95.7%, 81% of cases)
// and held-out (96.7%, 80%) through the proxy (D32).
const DefaultFastPath = 0.92

// DefaultPlanBar is the verifier confidence a precedent-supported plan needs
// to be shown; it matches the auto-accept bar (D35).
const DefaultPlanBar = 0.92

// Provider names for LLM_PROVIDER.
const (
	ProviderVertex = "vertex"
	ProviderFake   = "fake"
)

// Case store names for CASES_STORE.
const (
	CasesFirestore = "firestore"
	CasesFile      = "file"
	CasesMemory    = "memory"
)

// Verdict cache stores for VERIFIER_CACHE_STORE.
const (
	VerdictsFirestore = "firestore"
	VerdictsFile      = "file"
	VerdictsOff       = "off"
)

// Rate limiter stores for RATE_LIMIT_STORE (D70).
const (
	RateLocal     = "local"
	RateMemory    = "memory"
	RateFirestore = "firestore"
)

// Config is the validated runtime configuration. Secret fields are excluded
// from String so the config can be logged.
type Config struct {
	Port string

	// LLM provider: vertex (Gemini on Vertex AI with Application Default
	// Credentials), or fake for offline tests and UI work. The LiteLLM proxy
	// used before Vertex access was removed (D49).
	LLMProvider string
	GCPProject  string
	GCPLocation string
	OnCloudRun  bool

	// Models (IDs depend on the provider; the defaults are served by both).
	EngineModel   string
	RouterModel   string
	VerifierModel string
	// FallbackEngineModel is called once when the engine's primary model
	// ends in a 429, 503, 504 or deadline (D73). The default is the base
	// model gemini-3.8-flash. FALLBACK_ENGINE_MODEL=off disables it.
	// The fallback client uses FallbackEngineLocation (global, where
	// gemini-3.8-flash is served); the primary client stays on GCPLocation.
	FallbackEngineModel    string
	FallbackEngineLocation string
	// EngineAttempts is how many times the primary engine model is tried
	// before that fallback. 0 means 2 when a fallback is configured; the
	// shared client used by the router, verifier, clarify and agent keeps
	// LLMMaxRetries.
	EngineAttempts int
	// FallbackFastPath is the auto-accept bar for a prediction the fallback
	// model made. 0 (the default) always sends it to review.
	FallbackFastPath float64

	// Model call behaviour.
	LLMTimeout      time.Duration
	LLMMaxRetries   int
	MaxOutputTokens int

	// Engine, triage and planner tunables.
	PrecedentK           int
	FastPathConfidence   float64
	PlanConfidence       float64
	VerifierMaxToolCalls int
	// VerifierVotes runs the precedent check this many times and keeps the
	// majority; 1 (default) disables voting: 3 votes doubled plan-search time
	// without making verdicts repeatable (D33).
	VerifierVotes int
	// VerifierPrompt is v1 or v2 (D74): v2 adds a counter-ruling search and a
	// confidence scale tied to the rulings read.
	VerifierPrompt     string
	PlannerConcurrency int

	// Corpus. The engine's precedent pool is labels + facts files; the
	// verifier searches the local corpus or live CROSS.
	LabelsPath     string
	FactsDir       string
	RawDir         string
	VerifierSearch string // local | cross
	// CasesStore is where cases live: firestore (default on Cloud Run, D25),
	// file (default locally) or memory.
	CasesStore string
	// FirestoreDatabase is the Firestore database ID for CASES_STORE=firestore.
	FirestoreDatabase string
	// CasesDir persists cases as JSON files there; CASES_DIR=off keeps them
	// in memory.
	CasesDir string
	// VerifierCacheDir, when set, stores verifier verdicts so repeated plan
	// searches give the same verdicts (D31); VERIFIER_CACHE_DIR=off disables it.
	VerifierCacheDir string
	// VerifierCacheStore is where verdicts are cached: firestore (default
	// when cases are in Firestore, so all instances share them, D50), file
	// (VerifierCacheDir) or off.
	VerifierCacheStore string
	// RateLightPerMin, RateHeavyPerMin and RateGlobalPerMin cap the
	// model-calling endpoints (D45); 0 disables that limit.
	RateLightPerMin, RateHeavyPerMin, RateGlobalPerMin int
	// RateLookupPerMin caps ruling fetches from CROSS per client (D58).
	RateLookupPerMin int
	// RateLimitStore is where the limits are counted (D70): local (default,
	// each instance its own buckets), firestore (fixed-window counters shared
	// by all instances; falls back to local on a store error) or memory (the
	// shared limiter in one process, for trying it locally).
	RateLimitStore string
	// CasesListLimit caps GET /v1/cases: pinned cases plus the newest others (D58).
	CasesListLimit int
	// CaseOwnership (on|off, default off) limits visitors to the pinned demos
	// and their own cases; turn on once the frontend sends X-Visitor-ID (D58).
	CaseOwnership bool
	// BOMVersions is on or off (default): on serves POST
	// /v1/cases/{id}/bom-version, a newer BOM diffed against the case's
	// product and re-predicted (D67).
	BOMVersions string
	// Teams (on|off, default off) verifies the Google ID token the frontend
	// proxy forwards as X-User-Token and serves team workspaces (D68).
	// TeamsAudience is the OAuth client ID the tokens must be issued to;
	// required when Teams is on.
	Teams         bool
	TeamsAudience string
	// APIKeys (on|off, default off) accepts X-API-Key from brokers: the
	// admin issues keys with a daily request quota, each key gets its own
	// rate-limit buckets and owns its cases (D69). Keys are kept in Firestore
	// when cases are, else in memory.
	APIKeys bool
	// DailySpendCapUSD stops new model-calling requests once the UTC day's
	// estimated model spend reaches it (D59); 0 disables the cap.
	DailySpendCapUSD float64
	// AdminToken lets the demo seed bypass the limits, pin demo cases and
	// delete pinned ones. Empty: nobody is admin.
	AdminToken string
	// TextileEngine (on|off, default off) predicts textile cases under
	// 19 CFR 102.21 instead of leaving them out of scope (D51), with
	// precedents from TextileRulings. TextileFastPath is its auto-accept bar;
	// 0 (default) sends every textile case to review.
	TextileEngine   bool
	TextileRulings  string
	TextileFastPath float64
	// TextilePrompt is v1 or v2 (textile-v2: CBP's reasoning shown with each
	// precedent, from TextileReasoning, plus filled made-up guidance; D53).
	TextilePrompt    string
	TextileReasoning string
	// USMCAEngine (on|off, default off) predicts cases the router labels
	// usmca_part102 under the Part 102 rules (19 CFR 102.11 / 102.20,
	// usmca-v1) with precedents from USMCALabels (a copy of
	// eval/data/labels_usmca.jsonl; facts in FactsDir). USMCAFastPath is its
	// auto-accept bar; 0 (default) sends every USMCA case to review (D61).
	USMCAEngine   bool
	USMCALabels   string
	USMCAFastPath float64
	// PharmaEngine (on|off, default off) predicts pharmaceutical cases (router
	// regime pharma_api) with the pharma-v1 prompt, where origin follows the
	// active ingredient, instead of leaving them out of scope (D62), with
	// precedents from PharmaRulings. PharmaFastPath is its auto-accept bar;
	// 0 (default) sends every pharmaceutical case to review.
	PharmaEngine   bool
	PharmaRulings  string
	PharmaFastPath float64
	// FastPathByFamily (on|off, default off) replaces the global auto-accept
	// bar with the per-subfamily bars fitted on dev by `eval calibrate`
	// (embedded in internal/triage); subfamilies without a fitted bar keep
	// FastPathConfidence (D63).
	FastPathByFamily string
	// HTSSuggest (on|off, default off) serves POST /v1/products/hts-suggest:
	// candidate HTS subheadings from rulings that state one, listed in
	// HTSIndex (written by "corpus hts-index"; D64). Off: the endpoint is a 404.
	HTSSuggest bool
	HTSIndex   string
	// DutyLiveMFN (on|off, default off) looks up the MFN (column 1 general)
	// rate on the USITC HTS for subheadings the sourced duty table does not
	// cover, caching each answer for DutyLiveMFNTTL; each fetch waits at most
	// DutyLiveMFNTimeout (D65).
	DutyLiveMFN        bool
	DutyLiveMFNTTL     time.Duration
	DutyLiveMFNTimeout time.Duration
	// ExportCFR177 (on|off, default off) lets POST /v1/cases/{id}/export
	// take "format": "cfr177", a ruling-request letter laid out on
	// 19 CFR 177.2 (D66). Off: only the Markdown draft.
	ExportCFR177 bool
	exportCFR177 string // as given, checked by validate
	// StreamPredict (on|off, default off) lets POST /v1/cases/predict answer
	// as server-sent stage events when asked for text/event-stream (D71).
	StreamPredict string
	// EvalSlice (on|off, default off) lets `eval slice` spend model calls on
	// the 30-case regression slice (D72); the comparison against the
	// committed baseline (-in FILE) needs no model and works either way.
	// EvalSliceMaxDrop is how many fewer correct cases than the baseline
	// still pass (default 2); EvalSliceBandMin is the accuracy the cases at
	// confidence >= 0.95 must keep (default 0.95).
	EvalSlice        bool
	EvalSliceMaxDrop int
	EvalSliceBandMin float64
}

// StreamPredictOn reports whether STREAM_PREDICT is on.
func (c Config) StreamPredictOn() bool { return c.StreamPredict == "on" }

// Load reads the configuration from the process environment.
func Load() (Config, error) { return FromEnv(os.Getenv) }

// FromEnv reads the configuration through getenv, so tests can supply a map.
func FromEnv(getenv func(string) string) (Config, error) {
	e := env{get: getenv}
	c := Config{
		Port:                   e.str("PORT", "8080"),
		LLMProvider:            strings.ToLower(e.str("LLM_PROVIDER", ProviderVertex)),
		GCPProject:             e.str("GOOGLE_CLOUD_PROJECT", ""),
		GCPLocation:            e.str("GOOGLE_CLOUD_LOCATION", "global"),
		OnCloudRun:             e.str("K_SERVICE", "") != "",
		EngineModel:            e.str("ENGINE_MODEL", "gemini-4-argon"),
		RouterModel:            e.str("ROUTER_MODEL", "gemini-3.8-flash"),
		VerifierModel:          e.str("VERIFIER_MODEL", "gemini-4-argon"),
		FallbackEngineModel:    fallbackModel(e.str("FALLBACK_ENGINE_MODEL", "gemini-3.8-flash")),
		FallbackEngineLocation: e.str("FALLBACK_ENGINE_LOCATION", "global"),
		EngineAttempts:         e.integer("ENGINE_ATTEMPTS", 0),
		FallbackFastPath:       e.float("FALLBACK_FAST_PATH", 0),
		LLMTimeout:             e.duration("LLM_TIMEOUT", 30*time.Second), // per attempt; 4 attempts fit Cloud Run's 300 s (D54)
		LLMMaxRetries:          e.integer("LLM_MAX_RETRIES", 3),
		MaxOutputTokens:        e.integer("LLM_MAX_OUTPUT_TOKENS", 8000),
		PrecedentK:             e.integer("PRECEDENT_K", 5),
		FastPathConfidence:     e.float("FAST_PATH_CONFIDENCE", DefaultFastPath),
		PlanConfidence:         e.float("PLAN_CONFIDENCE", DefaultPlanBar),
		VerifierMaxToolCalls:   e.integer("VERIFIER_MAX_TOOL_CALLS", 6),
		VerifierVotes:          e.integer("VERIFIER_VOTES", 1),
		VerifierPrompt:         strings.ToLower(e.str("VERIFIER_PROMPT", "v2")),
		PlannerConcurrency:     e.integer("PLANNER_CONCURRENCY", 8),
		// Defaults resolve both from backend/ (local) and from / in the image.
		LabelsPath:         e.str("LABELS_PATH", "data/labels.jsonl"),
		FactsDir:           e.str("CORPUS_FACTS_DIR", "data/facts"),
		RawDir:             e.str("CORPUS_RAW_DIR", "../data/corpus/raw"),
		VerifierSearch:     strings.ToLower(e.str("VERIFIER_SEARCH", "cross")),
		CasesDir:           e.dir("CASES_DIR", "../data/cases"),
		FirestoreDatabase:  e.str("FIRESTORE_DATABASE", "(default)"),
		VerifierCacheDir:   e.dir("VERIFIER_CACHE_DIR", "../data/verifier-cache"),
		RateLightPerMin:    e.integer("RATE_LIGHT_PER_MIN", 20),
		RateHeavyPerMin:    e.integer("RATE_HEAVY_PER_MIN", 4),
		RateGlobalPerMin:   e.integer("RATE_GLOBAL_PER_MIN", 120),
		RateLookupPerMin:   e.integer("RATE_LOOKUP_PER_MIN", 30),
		RateLimitStore:     strings.ToLower(e.str("RATE_LIMIT_STORE", RateLocal)),
		CasesListLimit:     e.integer("CASES_LIST_LIMIT", 200),
		BOMVersions:        strings.ToLower(e.str("BOM_VERSIONS", "off")),
		DailySpendCapUSD:   e.float("DAILY_SPEND_CAP_USD", 0),
		AdminToken:         e.str("ADMIN_TOKEN", ""),
		TeamsAudience:      e.str("TEAMS_AUDIENCE", ""),
		TextileRulings:     e.str("TEXTILE_RULINGS", "data/textile_rulings.json"),
		TextileFastPath:    e.float("TEXTILE_FAST_PATH", 0),
		TextilePrompt:      strings.ToLower(e.str("TEXTILE_PROMPT", "v1")),
		TextileReasoning:   e.str("TEXTILE_REASONING", "data/textile_reasoning.json"),
		USMCALabels:        e.str("USMCA_LABELS", "data/labels_usmca.jsonl"),
		USMCAFastPath:      e.float("USMCA_FAST_PATH", 0),
		PharmaRulings:      e.str("PHARMA_RULINGS", "data/pharma_rulings.json"),
		PharmaFastPath:     e.float("PHARMA_FAST_PATH", 0),
		FastPathByFamily:   strings.ToLower(e.str("FAST_PATH_BY_FAMILY", "off")),
		HTSIndex:           e.str("HTS_INDEX", "data/hts_index.json"),
		DutyLiveMFNTTL:     e.duration("DUTY_LIVE_MFN_TTL", 24*time.Hour),
		DutyLiveMFNTimeout: e.duration("DUTY_LIVE_MFN_TIMEOUT", 5*time.Second),
		EvalSliceMaxDrop:   e.integer("EVAL_SLICE_MAX_DROP", 2),
		EvalSliceBandMin:   e.float("EVAL_SLICE_BAND_MIN", 0.95),
	}
	switch v := strings.ToLower(e.str("CASE_OWNERSHIP", "off")); v {
	case "on", "true", "1":
		c.CaseOwnership = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("CASE_OWNERSHIP must be on or off, got %q", v))
	}
	switch v := strings.ToLower(e.str("TEAMS", "off")); v {
	case "on", "true", "1":
		c.Teams = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("TEAMS must be on or off, got %q", v))
	}
	switch v := strings.ToLower(e.str("API_KEYS", "off")); v {
	case "on", "true", "1":
		c.APIKeys = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("API_KEYS must be on or off, got %q", v))
	}
	switch v := strings.ToLower(e.str("TEXTILE_ENGINE", "off")); v {
	case "on", "true", "1":
		c.TextileEngine = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("TEXTILE_ENGINE must be on or off, got %q", v))
	}
	switch v := strings.ToLower(e.str("USMCA_ENGINE", "off")); v {
	case "on", "true", "1":
		c.USMCAEngine = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("USMCA_ENGINE must be on or off, got %q", v))
	}
	switch v := strings.ToLower(e.str("PHARMA_ENGINE", "off")); v {
	case "on", "true", "1":
		c.PharmaEngine = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("PHARMA_ENGINE must be on or off, got %q", v))
	}
	switch v := strings.ToLower(e.str("HTS_SUGGEST", "off")); v {
	case "on", "true", "1":
		c.HTSSuggest = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("HTS_SUGGEST must be on or off, got %q", v))
	}
	switch v := strings.ToLower(e.str("DUTY_LIVE_MFN", "off")); v {
	case "on", "true", "1":
		c.DutyLiveMFN = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("DUTY_LIVE_MFN must be on or off, got %q", v))
	}
	c.exportCFR177 = strings.ToLower(e.str("EXPORT_CFR177", "off"))
	c.ExportCFR177 = c.exportCFR177 == "on" || c.exportCFR177 == "true" || c.exportCFR177 == "1"
	switch v := strings.ToLower(e.str("STREAM_PREDICT", "off")); v {
	case "on", "true", "1":
		c.StreamPredict = "on"
	case "off", "false", "0":
		c.StreamPredict = "off"
	default:
		c.StreamPredict = v // refused by validate
	}
	switch v := strings.ToLower(e.str("EVAL_SLICE", "off")); v {
	case "on", "true", "1":
		c.EvalSlice = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("EVAL_SLICE must be on or off, got %q", v))
	}
	defStore := CasesFile
	if c.OnCloudRun {
		defStore = CasesFirestore // local disk is lost on restart there
	}
	c.CasesStore = strings.ToLower(e.str("CASES_STORE", defStore))
	defCache := VerdictsFile
	switch {
	case c.VerifierCacheDir == "": // VERIFIER_CACHE_DIR=off
		defCache = VerdictsOff
	case c.CasesStore == CasesFirestore:
		defCache = VerdictsFirestore
	}
	c.VerifierCacheStore = strings.ToLower(e.str("VERIFIER_CACHE_STORE", defCache))
	// Two attempts, then the fallback: 4 attempts × 30 s plus backoff can
	// take about two minutes before any other model is asked (D56).
	if c.FallbackEngineModel != "" && c.EngineAttempts == 0 {
		c.EngineAttempts = 2
	}
	if err := errors.Join(append(e.errs, c.validate())...); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	return c, nil
}

func (c Config) validate() error {
	var errs []error
	bad := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	if c.VerifierPrompt != "v1" && c.VerifierPrompt != "v2" {
		bad("VERIFIER_PROMPT must be v1 or v2, got %q", c.VerifierPrompt)
	}
	switch c.LLMProvider {
	case ProviderVertex:
		if c.GCPProject == "" {
			bad("GOOGLE_CLOUD_PROJECT is required when LLM_PROVIDER=vertex")
		}
	case ProviderFake:
		if c.OnCloudRun {
			bad("LLM_PROVIDER=fake cannot run on Cloud Run")
		}
	default:
		bad("LLM_PROVIDER must be vertex or fake, got %q (the LiteLLM proxy was removed, D49)", c.LLMProvider)
	}
	for name, v := range map[string]string{"ENGINE_MODEL": c.EngineModel, "ROUTER_MODEL": c.RouterModel, "VERIFIER_MODEL": c.VerifierModel} {
		if v == "" {
			bad("%s must not be empty", name)
		}
	}
	for name, v := range map[string]float64{"FAST_PATH_CONFIDENCE": c.FastPathConfidence, "PLAN_CONFIDENCE": c.PlanConfidence} {
		if v <= 0 || v > 1 {
			bad("%s must be in (0, 1], got %v", name, v)
		}
	}
	positive := map[string]int{"PRECEDENT_K": c.PrecedentK, "VERIFIER_MAX_TOOL_CALLS": c.VerifierMaxToolCalls, "VERIFIER_VOTES": c.VerifierVotes,
		"PLANNER_CONCURRENCY": c.PlannerConcurrency, "LLM_MAX_OUTPUT_TOKENS": c.MaxOutputTokens}
	for name, v := range positive {
		if v <= 0 {
			bad("%s must be positive, got %d", name, v)
		}
	}
	if c.DailySpendCapUSD < 0 {
		bad("DAILY_SPEND_CAP_USD must be 0 (no cap) or positive, got %v", c.DailySpendCapUSD)
	}
	if c.CasesListLimit < 1 {
		bad("CASES_LIST_LIMIT must be positive, got %d", c.CasesListLimit)
	}
	switch c.exportCFR177 {
	case "on", "true", "1", "off", "false", "0":
	default:
		bad("EXPORT_CFR177 must be on or off, got %q", c.exportCFR177)
	}
	if c.BOMVersions != "on" && c.BOMVersions != "off" {
		bad("BOM_VERSIONS must be on or off, got %q", c.BOMVersions)
	}
	if c.Teams && !strings.HasSuffix(c.TeamsAudience, ".apps.googleusercontent.com") {
		bad("TEAMS=on needs TEAMS_AUDIENCE, the Google OAuth client ID (…apps.googleusercontent.com), got %q", c.TeamsAudience)
	}
	if c.TextilePrompt != "v1" && c.TextilePrompt != "v2" {
		bad("TEXTILE_PROMPT must be v1 or v2, got %q", c.TextilePrompt)
	}
	if c.FastPathByFamily != "on" && c.FastPathByFamily != "off" {
		bad("FAST_PATH_BY_FAMILY must be on or off, got %q", c.FastPathByFamily)
	}
	if c.HTSSuggest && !strings.HasSuffix(c.HTSIndex, ".json") {
		bad("HTS_INDEX must be a .json file written by \"corpus hts-index\", got %q", c.HTSIndex)
	}
	if c.StreamPredict != "on" && c.StreamPredict != "off" {
		bad("STREAM_PREDICT must be on or off, got %q", c.StreamPredict)
	}
	if c.EvalSliceMaxDrop < 0 {
		bad("EVAL_SLICE_MAX_DROP must be 0 or more correct cases, got %d", c.EvalSliceMaxDrop)
	}
	if c.EvalSliceBandMin <= 0 || c.EvalSliceBandMin > 1 {
		bad("EVAL_SLICE_BAND_MIN must be in (0, 1], got %v", c.EvalSliceBandMin)
	}
	if c.TextileFastPath < 0 || c.TextileFastPath > 1 {
		bad("TEXTILE_FAST_PATH must be 0 (always review) or in (0, 1], got %v", c.TextileFastPath)
	}
	if c.USMCAFastPath < 0 || c.USMCAFastPath > 1 {
		bad("USMCA_FAST_PATH must be 0 (always review) or in (0, 1], got %v", c.USMCAFastPath)
	}
	if c.PharmaFastPath < 0 || c.PharmaFastPath > 1 {
		bad("PHARMA_FAST_PATH must be 0 (always review) or in (0, 1], got %v", c.PharmaFastPath)
	}
	for name, v := range map[string]int{"RATE_LIGHT_PER_MIN": c.RateLightPerMin, "RATE_HEAVY_PER_MIN": c.RateHeavyPerMin, "RATE_GLOBAL_PER_MIN": c.RateGlobalPerMin, "RATE_LOOKUP_PER_MIN": c.RateLookupPerMin} {
		if v < 0 {
			bad("%s must be 0 (off) or positive, got %d", name, v)
		}
	}
	if c.RateGlobalPerMin > 0 && c.RateGlobalPerMin < 20 { // one batch request takes 20 (httpapi.MaxBatch)
		bad("RATE_GLOBAL_PER_MIN must be 0 (off) or at least 20, got %d", c.RateGlobalPerMin)
	}
	switch c.RateLimitStore {
	case RateFirestore:
		if c.GCPProject == "" {
			bad("GOOGLE_CLOUD_PROJECT is required when RATE_LIMIT_STORE=firestore")
		}
	case RateLocal, RateMemory:
	default:
		bad("RATE_LIMIT_STORE must be local, firestore or memory, got %q", c.RateLimitStore)
	}
	if c.AdminToken != "" && len(c.AdminToken) < 16 && c.OnCloudRun {
		bad("ADMIN_TOKEN must be at least 16 characters on Cloud Run")
	}
	if c.APIKeys {
		if c.AdminToken == "" {
			bad("API_KEYS=on needs ADMIN_TOKEN (keys are issued with it)")
		}
		if c.OnCloudRun && c.CasesStore != CasesFirestore {
			bad("API_KEYS=on on Cloud Run needs CASES_STORE=firestore (keys in memory are lost on restart and not shared)")
		}
	}
	switch c.VerifierCacheStore {
	case VerdictsFirestore:
		if c.GCPProject == "" {
			bad("GOOGLE_CLOUD_PROJECT is required when VERIFIER_CACHE_STORE=firestore")
		}
	case VerdictsFile:
		if c.VerifierCacheDir == "" {
			bad("VERIFIER_CACHE_STORE=file needs VERIFIER_CACHE_DIR")
		}
	case VerdictsOff:
	default:
		bad("VERIFIER_CACHE_STORE must be firestore, file or off, got %q", c.VerifierCacheStore)
	}
	switch c.CasesStore {
	case CasesFirestore:
		if c.GCPProject == "" {
			bad("GOOGLE_CLOUD_PROJECT is required when CASES_STORE=firestore")
		}
	case CasesFile, CasesMemory:
	default:
		bad("CASES_STORE must be firestore, file or memory, got %q", c.CasesStore)
	}
	if c.VerifierSearch != "local" && c.VerifierSearch != "cross" {
		bad("VERIFIER_SEARCH must be local or cross, got %q", c.VerifierSearch)
	}
	if c.LLMMaxRetries < 0 {
		bad("LLM_MAX_RETRIES must not be negative, got %d", c.LLMMaxRetries)
	}
	if c.DutyLiveMFNTTL <= 0 {
		bad("DUTY_LIVE_MFN_TTL must be positive, got %s", c.DutyLiveMFNTTL)
	}
	if c.DutyLiveMFNTimeout <= 0 || c.DutyLiveMFNTimeout > 30*time.Second {
		bad("DUTY_LIVE_MFN_TIMEOUT must be positive and at most 30s (it delays a plan search), got %s", c.DutyLiveMFNTimeout)
	}
	if c.LLMTimeout <= 0 {
		bad("LLM_TIMEOUT must be positive, got %s", c.LLMTimeout)
	}
	if c.EngineAttempts < 0 {
		bad("ENGINE_ATTEMPTS must be 0 (default) or positive, got %d", c.EngineAttempts)
	}
	if c.FallbackEngineModel != "" && c.EngineAttempts < 1 {
		bad("ENGINE_ATTEMPTS must be at least 1 when FALLBACK_ENGINE_MODEL is set, got %d", c.EngineAttempts)
	}
	if c.FallbackEngineModel != "" && c.FallbackEngineLocation == "" {
		bad("FALLBACK_ENGINE_LOCATION must not be empty when FALLBACK_ENGINE_MODEL is set")
	}
	if c.FallbackFastPath < 0 || c.FallbackFastPath > 1 {
		bad("FALLBACK_FAST_PATH must be 0 (always review) or in (0, 1], got %v", c.FallbackFastPath)
	}
	return errors.Join(errs...)
}

// FamilyBars reports whether triage uses the per-subfamily bars (D63).
func (c Config) FamilyBars() bool { return c.FastPathByFamily == "on" }

// String renders the config for startup logs; it holds no secrets.
func (c Config) String() string {
	fallback := "off"
	if c.FallbackEngineModel != "" {
		fallback = fmt.Sprintf("%s@%s attempts=%d fast_path=%.2f", c.FallbackEngineModel, c.FallbackEngineLocation, c.EngineAttempts, c.FallbackFastPath)
	}
	return fmt.Sprintf("provider=%s engine=%s router=%s verifier=%s fallback=%s k=%d fast_path=%.2f plan_conf=%.2f max_tool_calls=%d timeout=%s cloud_run=%t verifier_search=%s facts=%s cases=%s",
		c.LLMProvider, c.EngineModel, c.RouterModel, c.VerifierModel, fallback, c.PrecedentK,
		c.FastPathConfidence, c.PlanConfidence, c.VerifierMaxToolCalls, c.LLMTimeout, c.OnCloudRun, c.VerifierSearch, c.FactsDir, c.CasesStore)
}

// env parses typed values and collects parse errors.
type env struct {
	get  func(string) string
	errs []error
}

// fallbackModel treats "off" as disabled. An empty value is not reached:
// str already substitutes the default.
func fallbackModel(v string) string {
	if strings.EqualFold(v, "off") {
		return ""
	}
	return v
}

func (e *env) str(key, def string) string {
	if v := strings.TrimSpace(e.get(key)); v != "" {
		return v
	}
	return def
}

// dir is a directory setting: unset or empty means def, "off" disables it
// (returns "").
func (e *env) dir(key, def string) string {
	if v := e.str(key, def); !strings.EqualFold(v, "off") {
		return v
	}
	return ""
}

func (e *env) integer(key string, def int) int {
	v := e.str(key, "")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		e.errs = append(e.errs, fmt.Errorf("%s=%q is not an integer", key, v))
	}
	return n
}

func (e *env) float(key string, def float64) float64 {
	v := e.str(key, "")
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		e.errs = append(e.errs, fmt.Errorf("%s=%q is not a number", key, v))
	}
	return f
}

func (e *env) duration(key string, def time.Duration) time.Duration {
	v := e.str(key, "")
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		e.errs = append(e.errs, fmt.Errorf("%s=%q is not a duration (e.g. 90s)", key, v))
	}
	return d
}
