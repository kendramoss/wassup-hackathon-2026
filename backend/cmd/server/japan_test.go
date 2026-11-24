package main

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpengine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jurisdiction"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/spend"
)

// The Japan authority is opt-in: unconfigured deployments stay unavailable,
// the fake provider exercises the same route offline, and a misconfigured
// gateway fails closed rather than falling back to the Vertex engines.
func TestJapanAuthorityWiring(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := spend.New(spend.NewMemory(), 100)

	t.Run("gateway settings without the enable gate stay unavailable", func(t *testing.T) {
		t.Setenv("JAPAN_LITELLM_BASE_URL", "https://gateway.example/v1")
		t.Setenv("JAPAN_LITELLM_API_KEY", "secret")
		for _, provider := range []string{config.ProviderVertex, config.ProviderFake} {
			got, err := japanAuthority(config.Config{LLMProvider: provider}, meter, logger)
			if err != nil || got.Engine != nil || got.Market != "" {
				t.Fatalf("%v without JAPAN_ENGINE: authority %+v, err %v; want none", provider, got, err)
			}
		}
		// A malformed gateway is also ignored while the gate is off.
		t.Setenv("JAPAN_LITELLM_BASE_URL", "http://gateway.example")
		if got, err := japanAuthority(config.Config{LLMProvider: config.ProviderVertex}, meter, logger); err != nil || got.Engine != nil {
			t.Fatalf("gate off must ignore gateway settings: %+v, %v", got, err)
		}
	})

	t.Run("unconfigured", func(t *testing.T) {
		t.Setenv("JAPAN_LITELLM_BASE_URL", "")
		t.Setenv("JAPAN_LITELLM_API_KEY", "")
		got, err := japanAuthority(config.Config{LLMProvider: config.ProviderVertex, JapanEngine: true}, meter, logger)
		if err != nil {
			t.Fatal(err)
		}
		if got.Engine != nil || got.Market != "" {
			t.Fatalf("unconfigured Japan must not register an authority, got %+v", got)
		}
	})

	t.Run("fake provider", func(t *testing.T) {
		t.Setenv("JAPAN_LITELLM_BASE_URL", "")
		t.Setenv("JAPAN_LITELLM_API_KEY", "")
		got, err := japanAuthority(config.Config{LLMProvider: config.ProviderFake, JapanEngine: true}, meter, logger)
		if err != nil {
			t.Fatal(err)
		}
		if got.Market != jurisdiction.JP {
			t.Errorf("market = %q, want %q", got.Market, jurisdiction.JP)
		}
		if _, ok := got.Engine.(*jpengine.Engine); !ok {
			t.Fatalf("fake Japan engine = %T, want *jpengine.Engine", got.Engine)
		}
	})

	t.Run("invalid gateway fails closed", func(t *testing.T) {
		t.Setenv("JAPAN_LITELLM_BASE_URL", "http://gateway.example")
		t.Setenv("JAPAN_LITELLM_API_KEY", "secret")
		_, err := japanAuthority(config.Config{LLMProvider: config.ProviderVertex, JapanEngine: true}, meter, logger)
		if err == nil || !strings.Contains(err.Error(), "HTTPS URL") {
			t.Fatalf("invalid gateway error = %v, want an HTTPS URL error", err)
		}
	})

	t.Run("gateway registered without Vertex", func(t *testing.T) {
		t.Setenv("JAPAN_LITELLM_BASE_URL", "https://gateway.example/v1")
		t.Setenv("JAPAN_LITELLM_API_KEY", "secret")
		got, err := japanAuthority(config.Config{LLMProvider: config.ProviderVertex, JapanEngine: true}, meter, logger)
		if err != nil {
			t.Fatal(err)
		}
		if got.Market != jurisdiction.JP {
			t.Errorf("market = %q, want %q", got.Market, jurisdiction.JP)
		}
		if _, ok := got.Engine.(*jpengine.Engine); !ok {
			t.Fatalf("gateway Japan engine = %T, want *jpengine.Engine", got.Engine)
		}
	})
}

// Through the production wiring, Japan is not a supported market unless
// JAPAN_ENGINE is on, even with gateway settings present.
func TestJapanIsNotPublicWithoutTheEnableGate(t *testing.T) {
	t.Setenv("JAPAN_LITELLM_BASE_URL", "https://gateway.example/v1")
	t.Setenv("JAPAN_LITELLM_API_KEY", "secret")
	meter := spend.New(spend.NewMemory(), 100)
	base := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) { return llm.Response{Text: "{}"}, nil }}
	off := engineEnv(t, "", base, meter)
	if off.Jurisdictions.Supported(jurisdiction.JP) {
		t.Fatal("Japan registered without JAPAN_ENGINE=on")
	}
	for _, m := range off.Jurisdictions.List() {
		if m["code"] == jurisdiction.JP && m["supported"] == true {
			t.Fatalf("GET /v1/jurisdictions would list Japan as supported: %v", m)
		}
	}
	t.Setenv("JAPAN_ENGINE", "on")
	if on := engineEnv(t, "", base, meter); !on.Jurisdictions.Supported(jurisdiction.JP) {
		t.Fatal("Japan not registered with JAPAN_ENGINE=on")
	}
}
