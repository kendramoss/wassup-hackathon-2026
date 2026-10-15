package config

import (
	"strings"
	"testing"
	"time"
)

const fakeKey = "sk-test-not-a-real-key"

func getenv(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func litellmEnv() map[string]string {
	return map[string]string{"LITELLM_BASE_URL": "https://proxy.example/", "LITELLM_API_KEY": fakeKey}
}

func TestDefaults(t *testing.T) {
	c, err := FromEnv(getenv(litellmEnv()))
	if err != nil {
		t.Fatal(err)
	}
	if c.LLMProvider != ProviderLiteLLM || c.EngineModel != "gemini-4-argon" || c.RouterModel != "gemini-3.8-flash" ||
		c.PrecedentK != 5 || c.FastPathConfidence != 0.92 || c.PlanConfidence != 0.92 || c.VerifierMaxToolCalls != 6 ||
		c.LLMTimeout != 90*time.Second || c.GCPLocation != "global" || c.Port != "8080" {
		t.Errorf("unexpected defaults: %+v", c)
	}
	if c.LiteLLMBaseURL != "https://proxy.example" {
		t.Errorf("trailing slash not trimmed: %q", c.LiteLLMBaseURL)
	}
	if c.RateLightPerMin != 20 || c.RateHeavyPerMin != 4 || c.RateGlobalPerMin != 120 || c.AdminToken != "" {
		t.Errorf("rate/admin defaults = %d %d %d %q", c.RateLightPerMin, c.RateHeavyPerMin, c.RateGlobalPerMin, c.AdminToken)
	}
	if c.CasesDir != "../data/cases" || c.VerifierCacheDir != "../data/verifier-cache" || c.PlannerConcurrency != 8 || c.VerifierVotes != 1 {
		t.Errorf("cases/cache/concurrency/votes defaults = %q %q %d %d", c.CasesDir, c.VerifierCacheDir, c.PlannerConcurrency, c.VerifierVotes)
	}
	env := litellmEnv()
	env["CASES_DIR"], env["VERIFIER_CACHE_DIR"] = "off", "OFF"
	if off, err := FromEnv(getenv(env)); err != nil || off.CasesDir != "" || off.VerifierCacheDir != "" {
		t.Errorf("off not honoured: %q %q %v", off.CasesDir, off.VerifierCacheDir, err)
	}
}

func TestValidation(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string // fragment of the error; empty means valid
	}{
		{"litellm ok", litellmEnv(), ""},
		{"litellm refused on Cloud Run", merge(litellmEnv(), map[string]string{"K_SERVICE": "backend"}), "cannot run on Cloud Run"},
		{"litellm missing key", map[string]string{"LITELLM_BASE_URL": "https://p.example"}, "LITELLM_API_KEY is required"},
		{"litellm relative url", map[string]string{"LITELLM_BASE_URL": "proxy", "LITELLM_API_KEY": "k"}, "absolute URL"},
		{"vertex ok on Cloud Run", map[string]string{"LLM_PROVIDER": "vertex", "GOOGLE_CLOUD_PROJECT": "p", "K_SERVICE": "backend"}, ""},
		{"vertex needs project", map[string]string{"LLM_PROVIDER": "Vertex"}, "GOOGLE_CLOUD_PROJECT is required"},
		{"fake ok locally", map[string]string{"LLM_PROVIDER": "fake"}, ""},
		{"fake refused on Cloud Run", map[string]string{"LLM_PROVIDER": "fake", "K_SERVICE": "x"}, "fake cannot run"},
		{"unknown provider", map[string]string{"LLM_PROVIDER": "openai"}, `got "openai"`},
		{"bad int", merge(litellmEnv(), map[string]string{"PRECEDENT_K": "five"}), `PRECEDENT_K="five" is not an integer`},
		{"zero k", merge(litellmEnv(), map[string]string{"PRECEDENT_K": "0"}), "PRECEDENT_K must be positive"},
		{"threshold above 1", merge(litellmEnv(), map[string]string{"FAST_PATH_CONFIDENCE": "1.5"}), "FAST_PATH_CONFIDENCE must be in (0, 1]"},
		{"bad duration", merge(litellmEnv(), map[string]string{"LLM_TIMEOUT": "90"}), "not a duration"},
		{"firestore needs project", merge(litellmEnv(), map[string]string{"CASES_STORE": "firestore"}), "GOOGLE_CLOUD_PROJECT is required when CASES_STORE=firestore"},
		{"unknown cases store", merge(litellmEnv(), map[string]string{"CASES_STORE": "redis"}), "CASES_STORE must be firestore, file or memory"},
		{"bad verifier search", merge(litellmEnv(), map[string]string{"VERIFIER_SEARCH": "google"}), "VERIFIER_SEARCH must be local or cross"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := FromEnv(getenv(tt.env))
			switch {
			case tt.want == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
				t.Fatalf("error %v, want it to contain %q", err, tt.want)
			}
			if err != nil && strings.Contains(err.Error(), fakeKey) {
				t.Fatalf("error leaks the API key: %v", err)
			}
		})
	}
}

func TestStringOmitsSecrets(t *testing.T) {
	c, err := FromEnv(getenv(litellmEnv()))
	if err != nil {
		t.Fatal(err)
	}
	s := c.String()
	if strings.Contains(s, fakeKey) || strings.Contains(s, "proxy.example") {
		t.Fatalf("String leaks secrets: %s", s)
	}
}

func merge(a, b map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func TestCasesStoreDefaults(t *testing.T) {
	local, err := FromEnv(getenv(litellmEnv()))
	if err != nil || local.CasesStore != CasesFile || local.FirestoreDatabase != "(default)" {
		t.Errorf("local default = %q %q %v", local.CasesStore, local.FirestoreDatabase, err)
	}
	run, err := FromEnv(getenv(map[string]string{"LLM_PROVIDER": "vertex", "GOOGLE_CLOUD_PROJECT": "p", "K_SERVICE": "backend"}))
	if err != nil || run.CasesStore != CasesFirestore {
		t.Errorf("Cloud Run default = %q %v", run.CasesStore, err)
	}
}
