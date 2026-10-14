// Package config reads every tunable from the environment, applies documented
// defaults and validates the result once at startup.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Provider names for LLM_PROVIDER.
const (
	ProviderLiteLLM = "litellm"
	ProviderVertex  = "vertex"
	ProviderFake    = "fake"
)

// Config is the validated runtime configuration. Secret fields are excluded
// from String so the config can be logged.
type Config struct {
	Port string

	// LLM provider. litellm is for local development only and refuses to run
	// on Cloud Run (K_SERVICE set); vertex is required in deployment.
	LLMProvider    string
	LiteLLMBaseURL string
	LiteLLMAPIKey  string // secret
	GCPProject     string
	GCPLocation    string
	OnCloudRun     bool

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
	PlannerConcurrency   int

	// Corpus. The engine's precedent pool is labels + facts files; the
	// verifier searches the local corpus or live CROSS.
	LabelsPath     string
	FactsDir       string
	RawDir         string
	VerifierSearch string // local | cross
}

// Load reads the configuration from the process environment.
func Load() (Config, error) { return FromEnv(os.Getenv) }

// FromEnv reads the configuration through getenv, so tests can supply a map.
func FromEnv(getenv func(string) string) (Config, error) {
	e := env{get: getenv}
	c := Config{
		Port:                 e.str("PORT", "8080"),
		LLMProvider:          strings.ToLower(e.str("LLM_PROVIDER", ProviderLiteLLM)),
		LiteLLMBaseURL:       strings.TrimRight(e.str("LITELLM_BASE_URL", ""), "/"),
		LiteLLMAPIKey:        e.str("LITELLM_API_KEY", ""),
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
		FastPathConfidence:   e.float("FAST_PATH_CONFIDENCE", 0.95),
		PlanConfidence:       e.float("PLAN_CONFIDENCE", 0.95),
		VerifierMaxToolCalls: e.integer("VERIFIER_MAX_TOOL_CALLS", 6),
		PlannerConcurrency:   e.integer("PLANNER_CONCURRENCY", 4),
		// Defaults resolve both from backend/ (local) and from / in the image.
		LabelsPath:     e.str("LABELS_PATH", "data/labels.jsonl"),
		FactsDir:       e.str("CORPUS_FACTS_DIR", "data/facts"),
		RawDir:         e.str("CORPUS_RAW_DIR", "../data/corpus/raw"),
		VerifierSearch: strings.ToLower(e.str("VERIFIER_SEARCH", "cross")),
	}
	if err := errors.Join(append(e.errs, c.validate())...); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	return c, nil
}

func (c Config) validate() error {
	var errs []error
	bad := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	switch c.LLMProvider {
	case ProviderLiteLLM:
		if c.OnCloudRun {
			bad("LLM_PROVIDER=litellm is for local development only and cannot run on Cloud Run; use vertex")
		}
		if u, err := url.Parse(c.LiteLLMBaseURL); c.LiteLLMBaseURL == "" || err != nil || u.Host == "" {
			bad("LITELLM_BASE_URL must be an absolute URL when LLM_PROVIDER=litellm")
		}
		if c.LiteLLMAPIKey == "" {
			bad("LITELLM_API_KEY is required when LLM_PROVIDER=litellm")
		}
	case ProviderVertex:
		if c.GCPProject == "" {
			bad("GOOGLE_CLOUD_PROJECT is required when LLM_PROVIDER=vertex")
		}
	case ProviderFake:
		if c.OnCloudRun {
			bad("LLM_PROVIDER=fake cannot run on Cloud Run")
		}
	default:
		bad("LLM_PROVIDER must be litellm, vertex or fake, got %q", c.LLMProvider)
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
	positive := map[string]int{"PRECEDENT_K": c.PrecedentK, "VERIFIER_MAX_TOOL_CALLS": c.VerifierMaxToolCalls,
		"PLANNER_CONCURRENCY": c.PlannerConcurrency, "LLM_MAX_OUTPUT_TOKENS": c.MaxOutputTokens}
	for name, v := range positive {
		if v <= 0 {
			bad("%s must be positive, got %d", name, v)
		}
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

// String renders the config for startup logs. The LiteLLM key and base URL are
// never included.
func (c Config) String() string {
	return fmt.Sprintf("provider=%s engine=%s router=%s verifier=%s k=%d fast_path=%.2f plan_conf=%.2f max_tool_calls=%d timeout=%s cloud_run=%t verifier_search=%s facts=%s",
		c.LLMProvider, c.EngineModel, c.RouterModel, c.VerifierModel, c.PrecedentK,
		c.FastPathConfidence, c.PlanConfidence, c.VerifierMaxToolCalls, c.LLMTimeout, c.OnCloudRun, c.VerifierSearch, c.FactsDir)
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
