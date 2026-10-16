package config

import (
	"strings"
	"testing"
	"time"
)

func getenv(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func vertexEnv() map[string]string {
	return map[string]string{"GOOGLE_CLOUD_PROJECT": "wassup-hackathon-2026"}
}

func TestDefaults(t *testing.T) {
	c, err := FromEnv(getenv(vertexEnv()))
	if err != nil {
		t.Fatal(err)
	}
	if c.LLMProvider != ProviderVertex || c.EngineModel != "gemini-4-argon" || c.RouterModel != "gemini-3.8-flash" ||
		c.PrecedentK != 5 || c.FastPathConfidence != 0.92 || c.PlanConfidence != 0.92 || c.VerifierMaxToolCalls != 6 ||
		c.LLMTimeout != 90*time.Second || c.GCPLocation != "global" || c.Port != "8080" {
		t.Errorf("unexpected defaults: %+v", c)
	}
	if c.RateLightPerMin != 20 || c.RateHeavyPerMin != 4 || c.RateGlobalPerMin != 120 || c.AdminToken != "" {
		t.Errorf("rate/admin defaults = %d %d %d %q", c.RateLightPerMin, c.RateHeavyPerMin, c.RateGlobalPerMin, c.AdminToken)
	}
	if c.CasesDir != "../data/cases" || c.VerifierCacheDir != "../data/verifier-cache" || c.PlannerConcurrency != 8 || c.VerifierVotes != 1 {
		t.Errorf("cases/cache/concurrency/votes defaults = %q %q %d %d", c.CasesDir, c.VerifierCacheDir, c.PlannerConcurrency, c.VerifierVotes)
	}
	env := vertexEnv()
	env["CASES_DIR"], env["VERIFIER_CACHE_DIR"] = "off", "OFF"
	if off, err := FromEnv(getenv(env)); err != nil || off.CasesDir != "" || off.VerifierCacheDir != "" {
		t.Errorf("off not honoured: %q %q %v", off.CasesDir, off.VerifierCacheDir, err)
	} else if off.VerifierCacheStore != VerdictsOff {
		t.Errorf("cache store with dir off = %q", off.VerifierCacheStore)
	}
	if c.VerifierCacheStore != VerdictsFile {
		t.Errorf("local verdict cache = %q, want file", c.VerifierCacheStore)
	}
	run := map[string]string{"GOOGLE_CLOUD_PROJECT": "p", "K_SERVICE": "backend", "VERIFIER_CACHE_DIR": "/tmp/verifier-cache"}
	if cr, err := FromEnv(getenv(run)); err != nil || cr.VerifierCacheStore != VerdictsFirestore {
		t.Errorf("Cloud Run verdict cache = %q %v, want firestore (shared between instances)", cr.VerifierCacheStore, err)
	}
	run["VERIFIER_CACHE_STORE"] = "file"
	if cr, err := FromEnv(getenv(run)); err != nil || cr.VerifierCacheStore != VerdictsFile {
		t.Errorf("explicit file = %q %v", cr.VerifierCacheStore, err)
	}
	run["VERIFIER_CACHE_STORE"] = "redis"
	if _, err := FromEnv(getenv(run)); err == nil || !strings.Contains(err.Error(), "VERIFIER_CACHE_STORE") {
		t.Errorf("bad store accepted: %v", err)
	}
}

func TestValidation(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string // fragment of the error; empty means valid
	}{
		{"vertex by default", vertexEnv(), ""},
		{"litellm removed", merge(vertexEnv(), map[string]string{"LLM_PROVIDER": "litellm"}), "LiteLLM proxy was removed"},
		{"vertex ok on Cloud Run", map[string]string{"LLM_PROVIDER": "vertex", "GOOGLE_CLOUD_PROJECT": "p", "K_SERVICE": "backend"}, ""},
		{"vertex needs project", map[string]string{"LLM_PROVIDER": "Vertex"}, "GOOGLE_CLOUD_PROJECT is required"},
		{"fake ok locally", map[string]string{"LLM_PROVIDER": "fake"}, ""},
		{"fake refused on Cloud Run", map[string]string{"LLM_PROVIDER": "fake", "K_SERVICE": "x"}, "fake cannot run"},
		{"unknown provider", map[string]string{"LLM_PROVIDER": "openai"}, `got "openai"`},
		{"bad int", merge(vertexEnv(), map[string]string{"PRECEDENT_K": "five"}), `PRECEDENT_K="five" is not an integer`},
		{"zero k", merge(vertexEnv(), map[string]string{"PRECEDENT_K": "0"}), "PRECEDENT_K must be positive"},
		{"threshold above 1", merge(vertexEnv(), map[string]string{"FAST_PATH_CONFIDENCE": "1.5"}), "FAST_PATH_CONFIDENCE must be in (0, 1]"},
		{"bad duration", merge(vertexEnv(), map[string]string{"LLM_TIMEOUT": "90"}), "not a duration"},
		{"firestore needs project", map[string]string{"LLM_PROVIDER": "fake", "CASES_STORE": "firestore"}, "GOOGLE_CLOUD_PROJECT is required when CASES_STORE=firestore"},
		{"unknown cases store", merge(vertexEnv(), map[string]string{"CASES_STORE": "redis"}), "CASES_STORE must be firestore, file or memory"},
		{"bad verifier search", merge(vertexEnv(), map[string]string{"VERIFIER_SEARCH": "google"}), "VERIFIER_SEARCH must be local or cross"},
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
		})
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
	local, err := FromEnv(getenv(vertexEnv()))
	if err != nil || local.CasesStore != CasesFile || local.FirestoreDatabase != "(default)" {
		t.Errorf("local default = %q %q %v", local.CasesStore, local.FirestoreDatabase, err)
	}
	run, err := FromEnv(getenv(map[string]string{"LLM_PROVIDER": "vertex", "GOOGLE_CLOUD_PROJECT": "p", "K_SERVICE": "backend"}))
	if err != nil || run.CasesStore != CasesFirestore {
		t.Errorf("Cloud Run default = %q %v", run.CasesStore, err)
	}
}

func TestTextileConfig(t *testing.T) {
	c, err := FromEnv(getenv(vertexEnv()))
	if err != nil || c.TextileEngine || c.TextileFastPath != 0 || c.TextileRulings != "data/textile_rulings.json" {
		t.Fatalf("textile defaults = %v %v %q %v", c.TextileEngine, c.TextileFastPath, c.TextileRulings, err)
	}
	on, err := FromEnv(getenv(merge(vertexEnv(), map[string]string{"TEXTILE_ENGINE": "on", "TEXTILE_FAST_PATH": "0.95"})))
	if err != nil || !on.TextileEngine || on.TextileFastPath != 0.95 {
		t.Errorf("textile on = %+v %v", on.TextileEngine, err)
	}
	for _, bad := range []map[string]string{{"TEXTILE_ENGINE": "maybe"}, {"TEXTILE_FAST_PATH": "1.5"}} {
		if _, err := FromEnv(getenv(merge(vertexEnv(), bad))); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
}
