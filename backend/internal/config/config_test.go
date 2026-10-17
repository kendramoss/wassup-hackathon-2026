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
	if c.FallbackEngineModel != "gemini-3.8-flash" || c.EngineAttempts != 2 || c.FallbackFastPath != 0 || c.FallbackEngineLocation != "global" {
		t.Errorf("fallback defaults = %q attempts %d fast %v loc %q", c.FallbackEngineModel, c.EngineAttempts, c.FallbackFastPath, c.FallbackEngineLocation)
	}
	off := merge(vertexEnv(), map[string]string{"FALLBACK_ENGINE_MODEL": "off"})
	if disabled, err := FromEnv(getenv(off)); err != nil || disabled.FallbackEngineModel != "" || disabled.EngineAttempts != 0 {
		t.Errorf("fallback off = %q attempts %d err %v", disabled.FallbackEngineModel, disabled.EngineAttempts, err)
	}
	fb := merge(vertexEnv(), map[string]string{"FALLBACK_ENGINE_MODEL": "tuned-flash"})
	if on, err := FromEnv(getenv(fb)); err != nil || on.EngineAttempts != 2 || on.FallbackFastPath != 0 {
		t.Errorf("fallback on = attempts %d fast %v err %v", on.EngineAttempts, on.FallbackFastPath, err)
	}
	fb["ENGINE_ATTEMPTS"] = "1"
	fb["FALLBACK_FAST_PATH"] = "0.97"
	if on, err := FromEnv(getenv(fb)); err != nil || on.EngineAttempts != 1 || on.FallbackFastPath != 0.97 {
		t.Errorf("explicit fallback = attempts %d fast %v err %v", on.EngineAttempts, on.FallbackFastPath, err)
	}
	if c.LLMProvider != ProviderVertex || c.EngineModel != "gemini-4-argon" || c.RouterModel != "gemini-3.8-flash" ||
		c.PrecedentK != 5 || c.FastPathConfidence != 0.92 || c.PlanConfidence != 0.92 || c.VerifierMaxToolCalls != 6 ||
		c.LLMTimeout != 30*time.Second || c.LLMCallGap != 0 || c.GCPLocation != "global" || c.Port != "8080" {
		t.Errorf("unexpected defaults: %+v", c)
	}
	if g, err := FromEnv(getenv(merge(vertexEnv(), map[string]string{"LLM_CALL_GAP": "8s"}))); err != nil || g.LLMCallGap != 8*time.Second {
		t.Errorf("LLM_CALL_GAP = %s %v, want 8s", g.LLMCallGap, err)
	}
	if _, err := FromEnv(getenv(merge(vertexEnv(), map[string]string{"LLM_CALL_GAP": "-1s"}))); err == nil {
		t.Error("negative LLM_CALL_GAP accepted")
	}
	if c.VerifierPrompt != "v2" {
		t.Errorf("verifier prompt default = %q, want v2 (D74)", c.VerifierPrompt)
	}
	if _, err := FromEnv(getenv(merge(vertexEnv(), map[string]string{"VERIFIER_PROMPT": "v4"}))); err == nil {
		t.Error("VERIFIER_PROMPT=v4 accepted")
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

func TestRateLimitStore(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string // the store; empty means a config error
	}{
		{"local by default", vertexEnv(), RateLocal},
		{"local by default on Cloud Run", map[string]string{"GOOGLE_CLOUD_PROJECT": "p", "K_SERVICE": "backend"}, RateLocal},
		{"firestore", merge(vertexEnv(), map[string]string{"RATE_LIMIT_STORE": "Firestore"}), RateFirestore},
		{"memory", merge(vertexEnv(), map[string]string{"RATE_LIMIT_STORE": "memory"}), RateMemory},
		{"firestore needs a project", map[string]string{"LLM_PROVIDER": "fake", "RATE_LIMIT_STORE": "firestore"}, ""},
		{"unknown", merge(vertexEnv(), map[string]string{"RATE_LIMIT_STORE": "redis"}), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := FromEnv(getenv(tt.env))
			switch {
			case tt.want == "" && (err == nil || !strings.Contains(err.Error(), "RATE_LIMIT_STORE")):
				t.Errorf("error %v, want one naming RATE_LIMIT_STORE", err)
			case tt.want != "" && (err != nil || c.RateLimitStore != tt.want):
				t.Errorf("store = %q %v, want %q", c.RateLimitStore, err, tt.want)
			}
		})
	}
}

func TestStreamPredictConfig(t *testing.T) {
	tests := []struct {
		value string
		on    bool
		want  string // fragment of the error; empty means valid
	}{
		{"", false, ""},
		{"off", false, ""},
		{"on", true, ""},
		{"ON", true, ""},
		{"true", true, ""},
		{"0", false, ""},
		{"stream", false, `STREAM_PREDICT must be on or off, got "stream"`},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			env := vertexEnv()
			if tt.value != "" {
				env["STREAM_PREDICT"] = tt.value
			}
			c, err := FromEnv(getenv(env))
			switch {
			case tt.want == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
				t.Fatalf("error %v, want it to contain %q", err, tt.want)
			case tt.want == "" && c.StreamPredictOn() != tt.on:
				t.Errorf("StreamPredictOn = %v, want %v", c.StreamPredictOn(), tt.on)
			}
		})
	}
	if err := (Config{LLMProvider: ProviderFake, StreamPredict: "maybe"}).validate(); err == nil || !strings.Contains(err.Error(), "STREAM_PREDICT") {
		t.Errorf("validate accepted STREAM_PREDICT=maybe: %v", err)
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
	if on.TextilePrompt != "v1" {
		t.Errorf("textile prompt default = %q", on.TextilePrompt)
	}
	for _, bad := range []map[string]string{{"TEXTILE_ENGINE": "maybe"}, {"TEXTILE_FAST_PATH": "1.5"}, {"TEXTILE_PROMPT": "v3"}} {
		if _, err := FromEnv(getenv(merge(vertexEnv(), bad))); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
}

func TestUSMCAConfig(t *testing.T) {
	c, err := FromEnv(getenv(vertexEnv()))
	if err != nil || c.USMCAEngine || c.USMCAFastPath != 0 || c.USMCALabels != "data/labels_usmca.jsonl" {
		t.Fatalf("usmca defaults = %v %v %q %v", c.USMCAEngine, c.USMCAFastPath, c.USMCALabels, err)
	}
	on, err := FromEnv(getenv(merge(vertexEnv(), map[string]string{"USMCA_ENGINE": "on", "USMCA_FAST_PATH": "0.9"})))
	if err != nil || !on.USMCAEngine || on.USMCAFastPath != 0.9 {
		t.Errorf("usmca on = %v %v %v", on.USMCAEngine, on.USMCAFastPath, err)
	}
	for _, bad := range []map[string]string{{"USMCA_ENGINE": "maybe"}, {"USMCA_FAST_PATH": "1.5"}, {"USMCA_FAST_PATH": "-0.1"}} {
		if _, err := FromEnv(getenv(merge(vertexEnv(), bad))); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
}

func TestPharmaConfig(t *testing.T) {
	c, err := FromEnv(getenv(vertexEnv()))
	if err != nil || c.PharmaEngine || c.PharmaFastPath != 0 || c.PharmaRulings != "data/pharma_rulings.json" {
		t.Fatalf("pharma defaults = %v %v %q %v", c.PharmaEngine, c.PharmaFastPath, c.PharmaRulings, err)
	}
	tests := []struct {
		env    map[string]string
		on     bool
		fast   float64
		wantOK bool
	}{
		{map[string]string{"PHARMA_ENGINE": "on"}, true, 0, true},
		{map[string]string{"PHARMA_ENGINE": "ON", "PHARMA_FAST_PATH": "0.97"}, true, 0.97, true},
		{map[string]string{"PHARMA_ENGINE": "off", "PHARMA_FAST_PATH": "1"}, false, 1, true},
		{map[string]string{"PHARMA_ENGINE": "maybe"}, false, 0, false},
		{map[string]string{"PHARMA_FAST_PATH": "1.5"}, false, 0, false},
		{map[string]string{"PHARMA_FAST_PATH": "-0.1"}, false, 0, false},
		{map[string]string{"PHARMA_FAST_PATH": "high"}, false, 0, false},
	}
	for _, tt := range tests {
		got, err := FromEnv(getenv(merge(vertexEnv(), tt.env)))
		if (err == nil) != tt.wantOK {
			t.Errorf("%v: err = %v, want ok %v", tt.env, err, tt.wantOK)
			continue
		}
		if tt.wantOK && (got.PharmaEngine != tt.on || got.PharmaFastPath != tt.fast) {
			t.Errorf("%v = engine %v fast %v", tt.env, got.PharmaEngine, got.PharmaFastPath)
		}
	}
}

func TestFastPathByFamily(t *testing.T) {
	c, err := FromEnv(getenv(vertexEnv()))
	if err != nil || c.FastPathByFamily != "off" || c.FamilyBars() {
		t.Fatalf("default = %q %v %v", c.FastPathByFamily, c.FamilyBars(), err)
	}
	for _, tt := range []struct {
		v    string
		want bool
		bad  bool
	}{{"on", true, false}, {"ON", true, false}, {"off", false, false}, {"true", false, true}, {"family", false, true}} {
		c, err := FromEnv(getenv(merge(vertexEnv(), map[string]string{"FAST_PATH_BY_FAMILY": tt.v})))
		switch {
		case tt.bad && (err == nil || !strings.Contains(err.Error(), "FAST_PATH_BY_FAMILY must be on or off")):
			t.Errorf("%q accepted: %v", tt.v, err)
		case !tt.bad && (err != nil || c.FamilyBars() != tt.want):
			t.Errorf("%q = %v %v, want %v", tt.v, c.FamilyBars(), err, tt.want)
		}
	}
}

func TestHTSSuggestConfig(t *testing.T) {
	c, err := FromEnv(getenv(vertexEnv()))
	if err != nil || c.HTSSuggest || c.HTSIndex != "data/hts_index.json" {
		t.Fatalf("hts defaults = %v %q %v", c.HTSSuggest, c.HTSIndex, err)
	}
	on, err := FromEnv(getenv(merge(vertexEnv(), map[string]string{"HTS_SUGGEST": "ON", "HTS_INDEX": "/data/x.json"})))
	if err != nil || !on.HTSSuggest || on.HTSIndex != "/data/x.json" {
		t.Errorf("hts on = %v %q %v", on.HTSSuggest, on.HTSIndex, err)
	}
	tests := []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"HTS_SUGGEST": "maybe"}, "HTS_SUGGEST must be on or off"},
		{map[string]string{"HTS_SUGGEST": "on", "HTS_INDEX": "data/facts"}, "HTS_INDEX must be a .json file"},
	}
	for _, tt := range tests {
		if _, err := FromEnv(getenv(merge(vertexEnv(), tt.env))); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%v: error %v, want %q", tt.env, err, tt.want)
		}
	}
	if _, err := FromEnv(getenv(merge(vertexEnv(), map[string]string{"HTS_INDEX": "data/facts"}))); err != nil {
		t.Errorf("HTS_INDEX is not checked while off: %v", err)
	}
}

func TestDutyLiveMFNConfig(t *testing.T) {
	c, err := FromEnv(getenv(vertexEnv()))
	if err != nil || c.DutyLiveMFN || c.DutyLiveMFNTTL != 24*time.Hour || c.DutyLiveMFNTimeout != 5*time.Second {
		t.Fatalf("live MFN defaults = %v %s %s %v", c.DutyLiveMFN, c.DutyLiveMFNTTL, c.DutyLiveMFNTimeout, err)
	}
	on, err := FromEnv(getenv(merge(vertexEnv(), map[string]string{"DUTY_LIVE_MFN": "on", "DUTY_LIVE_MFN_TTL": "6h", "DUTY_LIVE_MFN_TIMEOUT": "3s"})))
	if err != nil || !on.DutyLiveMFN || on.DutyLiveMFNTTL != 6*time.Hour || on.DutyLiveMFNTimeout != 3*time.Second {
		t.Errorf("live MFN on = %v %s %s %v", on.DutyLiveMFN, on.DutyLiveMFNTTL, on.DutyLiveMFNTimeout, err)
	}
	for _, bad := range []map[string]string{{"DUTY_LIVE_MFN": "yes please"}, {"DUTY_LIVE_MFN_TTL": "0s"}, {"DUTY_LIVE_MFN_TTL": "soon"},
		{"DUTY_LIVE_MFN_TIMEOUT": "-1s"}, {"DUTY_LIVE_MFN_TIMEOUT": "2m"}} {
		if _, err := FromEnv(getenv(merge(vertexEnv(), bad))); err == nil || !strings.Contains(err.Error(), "DUTY_LIVE_MFN") {
			t.Errorf("%v accepted: %v", bad, err)
		}
	}
}

func TestExportCFR177Config(t *testing.T) {
	c, err := FromEnv(getenv(vertexEnv()))
	if err != nil || c.ExportCFR177 {
		t.Fatalf("EXPORT_CFR177 default = %v %v, want off", c.ExportCFR177, err)
	}
	for v, want := range map[string]bool{"on": true, "ON": true, "1": true, "off": false, "0": false} {
		c, err := FromEnv(getenv(merge(vertexEnv(), map[string]string{"EXPORT_CFR177": v})))
		if err != nil || c.ExportCFR177 != want {
			t.Errorf("EXPORT_CFR177=%s = %v %v, want %v", v, c.ExportCFR177, err, want)
		}
	}
	if _, err := FromEnv(getenv(merge(vertexEnv(), map[string]string{"EXPORT_CFR177": "maybe"}))); err == nil || !strings.Contains(err.Error(), "EXPORT_CFR177") {
		t.Errorf("EXPORT_CFR177=maybe: %v", err)
	}
}

func TestBOMVersionsConfig(t *testing.T) {
	for _, tt := range []struct {
		value, want string
		ok          bool
	}{
		{"", "off", true}, {"off", "off", true}, {"ON", "on", true}, {"true", "", false}, {"maybe", "", false},
	} {
		c, err := FromEnv(getenv(merge(vertexEnv(), map[string]string{"BOM_VERSIONS": tt.value})))
		switch {
		case tt.ok && (err != nil || c.BOMVersions != tt.want):
			t.Errorf("BOM_VERSIONS=%q: got %q, %v; want %q", tt.value, c.BOMVersions, err, tt.want)
		case !tt.ok && (err == nil || !strings.Contains(err.Error(), "BOM_VERSIONS must be on or off")):
			t.Errorf("BOM_VERSIONS=%q accepted: %v", tt.value, err)
		}
	}
}

func TestTeamsConfig(t *testing.T) {
	c, err := FromEnv(getenv(vertexEnv()))
	if err != nil || c.Teams || c.TeamsAudience != "" {
		t.Fatalf("teams defaults = %v %q %v", c.Teams, c.TeamsAudience, err)
	}
	const aud = "1234-abc.apps.googleusercontent.com"
	on, err := FromEnv(getenv(merge(vertexEnv(), map[string]string{"TEAMS": "on", "TEAMS_AUDIENCE": aud})))
	if err != nil || !on.Teams || on.TeamsAudience != aud {
		t.Errorf("teams on = %v %q %v", on.Teams, on.TeamsAudience, err)
	}
	// An audience alone does nothing: teams stays off.
	if off, err := FromEnv(getenv(merge(vertexEnv(), map[string]string{"TEAMS_AUDIENCE": aud}))); err != nil || off.Teams {
		t.Errorf("audience without TEAMS = %v %v", off.Teams, err)
	}
	for _, bad := range []map[string]string{
		{"TEAMS": "maybe", "TEAMS_AUDIENCE": aud},
		{"TEAMS": "on"}, // no audience: any client's token would pass
		{"TEAMS": "on", "TEAMS_AUDIENCE": "my-backend-url"}, // not an OAuth client ID
	} {
		if _, err := FromEnv(getenv(merge(vertexEnv(), bad))); err == nil || !strings.Contains(err.Error(), "TEAMS") {
			t.Errorf("%v accepted: %v", bad, err)
		}
	}
}

func TestAPIKeysConfig(t *testing.T) {
	c, err := FromEnv(getenv(vertexEnv()))
	if err != nil || c.APIKeys {
		t.Fatalf("API keys default = %v %v, want off", c.APIKeys, err)
	}
	tests := []struct {
		name string
		env  map[string]string
		on   bool
		want string // fragment of the error; empty means valid
	}{
		{"on with admin token", map[string]string{"API_KEYS": "on", "ADMIN_TOKEN": "local-admin"}, true, ""},
		{"off explicitly", map[string]string{"API_KEYS": "off"}, false, ""},
		{"bad value", map[string]string{"API_KEYS": "maybe"}, false, "API_KEYS must be on or off"},
		{"needs admin token", map[string]string{"API_KEYS": "on"}, false, "API_KEYS=on needs ADMIN_TOKEN"},
		{"Cloud Run needs firestore", map[string]string{"API_KEYS": "on", "ADMIN_TOKEN": "a-long-admin-token-123", "K_SERVICE": "backend", "CASES_STORE": "memory"}, false, "CASES_STORE=firestore"},
		{"Cloud Run with firestore", map[string]string{"API_KEYS": "on", "ADMIN_TOKEN": "a-long-admin-token-123", "K_SERVICE": "backend"}, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := FromEnv(getenv(merge(vertexEnv(), tt.env)))
			switch {
			case tt.want == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
				t.Fatalf("error %v, want it to contain %q", err, tt.want)
			case tt.want == "" && c.APIKeys != tt.on:
				t.Errorf("APIKeys = %v, want %v", c.APIKeys, tt.on)
			}
		})
	}
}

func TestEvalSliceConfig(t *testing.T) {
	c, err := FromEnv(getenv(vertexEnv()))
	if err != nil || c.EvalSlice || c.EvalSliceMaxDrop != 2 || c.EvalSliceBandMin != 0.95 {
		t.Fatalf("eval slice defaults = %v %d %v %v", c.EvalSlice, c.EvalSliceMaxDrop, c.EvalSliceBandMin, err)
	}
	on, err := FromEnv(getenv(merge(vertexEnv(), map[string]string{"EVAL_SLICE": "ON", "EVAL_SLICE_MAX_DROP": "0", "EVAL_SLICE_BAND_MIN": "1"})))
	if err != nil || !on.EvalSlice || on.EvalSliceMaxDrop != 0 || on.EvalSliceBandMin != 1 {
		t.Errorf("eval slice on = %v %d %v %v", on.EvalSlice, on.EvalSliceMaxDrop, on.EvalSliceBandMin, err)
	}
	tests := []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"EVAL_SLICE": "sometimes"}, "EVAL_SLICE must be on or off"},
		{map[string]string{"EVAL_SLICE_MAX_DROP": "-1"}, "EVAL_SLICE_MAX_DROP must be 0 or more"},
		{map[string]string{"EVAL_SLICE_MAX_DROP": "two"}, "not an integer"},
		{map[string]string{"EVAL_SLICE_BAND_MIN": "0"}, "EVAL_SLICE_BAND_MIN must be in (0, 1]"},
		{map[string]string{"EVAL_SLICE_BAND_MIN": "95"}, "EVAL_SLICE_BAND_MIN must be in (0, 1]"},
	}
	for _, tt := range tests {
		if _, err := FromEnv(getenv(merge(vertexEnv(), tt.env))); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%v: error %v, want %q", tt.env, err, tt.want)
		}
	}
}
