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
	VerifierVotes      int
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
	// CasesListLimit caps GET /v1/cases: pinned cases plus the newest others (D58).
	CasesListLimit int
	// CaseOwnership (on|off, default off) limits visitors to the pinned demos
	// and their own cases; turn on once the frontend sends X-Visitor-ID (D58).
	CaseOwnership bool
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
}

// Load reads the configuration from the process environment.
func Load() (Config, error) { return FromEnv(os.Getenv) }

// FromEnv reads the configuration through getenv, so tests can supply a map.
func FromEnv(getenv func(string) string) (Config, error) {
	e := env{get: getenv}
	c := Config{
		Port:                 e.str("PORT", "8080"),
		LLMProvider:          strings.ToLower(e.str("LLM_PROVIDER", ProviderVertex)),
		GCPProject:           e.str("GOOGLE_CLOUD_PROJECT", ""),
		GCPLocation:          e.str("GOOGLE_CLOUD_LOCATION", "global"),
		OnCloudRun:           e.str("K_SERVICE", "") != "",
		EngineModel:          e.str("ENGINE_MODEL", "gemini-4-argon"),
		RouterModel:          e.str("ROUTER_MODEL", "gemini-3.8-flash"),
		VerifierModel:        e.str("VERIFIER_MODEL", "gemini-4-argon"),
		LLMTimeout:           e.duration("LLM_TIMEOUT", 30*time.Second), // per attempt; 4 attempts fit Cloud Run's 300 s (D54)
		LLMMaxRetries:        e.integer("LLM_MAX_RETRIES", 3),
		MaxOutputTokens:      e.integer("LLM_MAX_OUTPUT_TOKENS", 8000),
		PrecedentK:           e.integer("PRECEDENT_K", 5),
		FastPathConfidence:   e.float("FAST_PATH_CONFIDENCE", DefaultFastPath),
		PlanConfidence:       e.float("PLAN_CONFIDENCE", DefaultPlanBar),
		VerifierMaxToolCalls: e.integer("VERIFIER_MAX_TOOL_CALLS", 6),
		VerifierVotes:        e.integer("VERIFIER_VOTES", 1),
		PlannerConcurrency:   e.integer("PLANNER_CONCURRENCY", 8),
		// Defaults resolve both from backend/ (local) and from / in the image.
		LabelsPath:        e.str("LABELS_PATH", "data/labels.jsonl"),
		FactsDir:          e.str("CORPUS_FACTS_DIR", "data/facts"),
		RawDir:            e.str("CORPUS_RAW_DIR", "../data/corpus/raw"),
		VerifierSearch:    strings.ToLower(e.str("VERIFIER_SEARCH", "cross")),
		CasesDir:          e.dir("CASES_DIR", "../data/cases"),
		FirestoreDatabase: e.str("FIRESTORE_DATABASE", "(default)"),
		VerifierCacheDir:  e.dir("VERIFIER_CACHE_DIR", "../data/verifier-cache"),
		RateLightPerMin:   e.integer("RATE_LIGHT_PER_MIN", 20),
		RateHeavyPerMin:   e.integer("RATE_HEAVY_PER_MIN", 4),
		RateGlobalPerMin:  e.integer("RATE_GLOBAL_PER_MIN", 120),
		RateLookupPerMin:  e.integer("RATE_LOOKUP_PER_MIN", 30),
		CasesListLimit:    e.integer("CASES_LIST_LIMIT", 200),
		DailySpendCapUSD:  e.float("DAILY_SPEND_CAP_USD", 0),
		AdminToken:        e.str("ADMIN_TOKEN", ""),
		TextileRulings:    e.str("TEXTILE_RULINGS", "data/textile_rulings.json"),
		TextileFastPath:   e.float("TEXTILE_FAST_PATH", 0),
		TextilePrompt:     strings.ToLower(e.str("TEXTILE_PROMPT", "v1")),
		TextileReasoning:  e.str("TEXTILE_REASONING", "data/textile_reasoning.json"),
	}
	switch v := strings.ToLower(e.str("CASE_OWNERSHIP", "off")); v {
	case "on", "true", "1":
		c.CaseOwnership = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("CASE_OWNERSHIP must be on or off, got %q", v))
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
	if err := errors.Join(append(e.errs, c.validate())...); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	return c, nil
}

func (c Config) validate() error {
	var errs []error
	bad := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
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
	if c.TextilePrompt != "v1" && c.TextilePrompt != "v2" {
		bad("TEXTILE_PROMPT must be v1 or v2, got %q", c.TextilePrompt)
	}
	if c.TextileFastPath < 0 || c.TextileFastPath > 1 {
		bad("TEXTILE_FAST_PATH must be 0 (always review) or in (0, 1], got %v", c.TextileFastPath)
	}
	for name, v := range map[string]int{"RATE_LIGHT_PER_MIN": c.RateLightPerMin, "RATE_HEAVY_PER_MIN": c.RateHeavyPerMin, "RATE_GLOBAL_PER_MIN": c.RateGlobalPerMin, "RATE_LOOKUP_PER_MIN": c.RateLookupPerMin} {
		if v < 0 {
			bad("%s must be 0 (off) or positive, got %d", name, v)
		}
	}
	if c.RateGlobalPerMin > 0 && c.RateGlobalPerMin < 20 { // one batch request takes 20 (httpapi.MaxBatch)
		bad("RATE_GLOBAL_PER_MIN must be 0 (off) or at least 20, got %d", c.RateGlobalPerMin)
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
	if c.LLMTimeout <= 0 {
		bad("LLM_TIMEOUT must be positive, got %s", c.LLMTimeout)
	}
	return errors.Join(errs...)
}

// String renders the config for startup logs; it holds no secrets.
func (c Config) String() string {
	return fmt.Sprintf("provider=%s engine=%s router=%s verifier=%s k=%d fast_path=%.2f plan_conf=%.2f max_tool_calls=%d timeout=%s cloud_run=%t verifier_search=%s facts=%s cases=%s",
		c.LLMProvider, c.EngineModel, c.RouterModel, c.VerifierModel, c.PrecedentK,
		c.FastPathConfidence, c.PlanConfidence, c.VerifierMaxToolCalls, c.LLMTimeout, c.OnCloudRun, c.VerifierSearch, c.FactsDir, c.CasesStore)
}

// env parses typed values and collects parse errors.
type env struct {
	get  func(string) string
	errs []error
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
