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
		LLMTimeout:           e.duration("LLM_TIMEOUT", 90*time.Second),
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
	}
	defStore := CasesFile
	if c.OnCloudRun {
		defStore = CasesFirestore // local disk is lost on restart there
	}
	c.CasesStore = strings.ToLower(e.str("CASES_STORE", defStore))
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
