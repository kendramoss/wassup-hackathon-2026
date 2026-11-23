package main

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpengine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jurisdiction"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/spend"
)

// The Japan authority is opt-in: unconfigured deployments stay unavailable,
// the fake provider exercises the same route offline, and a misconfigured
// gateway fails closed rather than falling back to the Vertex engines.
func TestJapanAuthorityWiring(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := spend.New(spend.NewMemory(), 100)

	t.Run("unconfigured", func(t *testing.T) {
		t.Setenv("JAPAN_LITELLM_BASE_URL", "")
		t.Setenv("JAPAN_LITELLM_API_KEY", "")
		got, err := japanAuthority(config.Config{LLMProvider: config.ProviderVertex}, meter, logger)
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
		got, err := japanAuthority(config.Config{LLMProvider: config.ProviderFake}, meter, logger)
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
		_, err := japanAuthority(config.Config{LLMProvider: config.ProviderVertex}, meter, logger)
		if err == nil || !strings.Contains(err.Error(), "HTTPS URL") {
			t.Fatalf("invalid gateway error = %v, want an HTTPS URL error", err)
		}
	})

	t.Run("gateway registered without Vertex", func(t *testing.T) {
		t.Setenv("JAPAN_LITELLM_BASE_URL", "https://gateway.example/v1")
		t.Setenv("JAPAN_LITELLM_API_KEY", "secret")
		got, err := japanAuthority(config.Config{LLMProvider: config.ProviderVertex}, meter, logger)
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
