package main

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpengine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jurisdiction"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/spend"
)

// The Japan authority is on by default but fails closed: without a usable
// model it stays unavailable, the fake provider exercises the same route
// offline, and a half-set or invalid dedicated gateway errors instead of
// falling back to the engine model.
func TestJapanAuthorityWiring(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meter := spend.New(spend.NewMemory(), 100)

	t.Run("gateway settings without the enable gate stay unavailable", func(t *testing.T) {
		t.Setenv("JAPAN_LITELLM_BASE_URL", "https://gateway.example/v1")
		t.Setenv("JAPAN_LITELLM_API_KEY", "secret")
		for _, provider := range []string{config.ProviderVertex, config.ProviderFake} {
			got, err := japanAuthority(config.Config{LLMProvider: provider}, nil, meter, logger)
			if err != nil || got.Engine != nil || got.Market != "" {
				t.Fatalf("%v without JAPAN_ENGINE: authority %+v, err %v; want none", provider, got, err)
			}
		}
		// A malformed gateway is also ignored while the gate is off.
		t.Setenv("JAPAN_LITELLM_BASE_URL", "http://gateway.example")
		if got, err := japanAuthority(config.Config{LLMProvider: config.ProviderVertex}, nil, meter, logger); err != nil || got.Engine != nil {
			t.Fatalf("gate off must ignore gateway settings: %+v, %v", got, err)
		}
	})

	t.Run("vertex without a model fails closed", func(t *testing.T) {
		t.Setenv("JAPAN_LITELLM_BASE_URL", "")
		t.Setenv("JAPAN_LITELLM_API_KEY", "")
		got, err := japanAuthority(config.Config{LLMProvider: config.ProviderVertex, JapanEngine: true}, nil, meter, logger)
		if err != nil {
			t.Fatal(err)
		}
		if got.Engine != nil || got.Market != "" {
			t.Fatalf("Japan without a usable model must not register an authority, got %+v", got)
		}
	})

	t.Run("fake provider", func(t *testing.T) {
		t.Setenv("JAPAN_LITELLM_BASE_URL", "")
		t.Setenv("JAPAN_LITELLM_API_KEY", "")
		got, err := japanAuthority(config.Config{LLMProvider: config.ProviderFake, JapanEngine: true}, nil, meter, logger)
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

	t.Run("vertex reuses the production engine model", func(t *testing.T) {
		t.Setenv("JAPAN_LITELLM_BASE_URL", "")
		t.Setenv("JAPAN_LITELLM_API_KEY", "")
		var gotModel string
		model := &llm.Fake{Respond: func(req llm.Request) (llm.Response, error) {
			gotModel = req.Model
			return llm.Response{Text: `{"outcome":"qualifies","criterion":"CTH","origin_country":"Viet Nam","deciding_rule":"Change of heading","confidence":0.7,"reasoning":"Illustrative.","cited_rulings":[]}`}, nil
		}}
		cfg := config.Config{LLMProvider: config.ProviderVertex, JapanEngine: true, EngineModel: "engine-model-x"}
		got, err := japanAuthority(cfg, model, meter, logger)
		if err != nil {
			t.Fatal(err)
		}
		if got.Market != jurisdiction.JP {
			t.Fatalf("market = %q, want %q", got.Market, jurisdiction.JP)
		}
		eng, ok := got.Engine.(*jpengine.Engine)
		if !ok {
			t.Fatalf("Japan engine = %T, want *jpengine.Engine", got.Engine)
		}
		c := domain.Case{
			Date:    time.Date(2025, 10, 1, 14, 0, 0, 0, time.UTC),
			Japan:   &domain.JapanInput{Agreement: "RCEP", HS: "1704.90-010"},
			Facts:   "Sugar confectionery manufactured and packaged in Vietnam, imported into Japan under the RCEP.",
			Product: &domain.Product{Components: []domain.Component{{Name: "chocolate", MadeIn: "Other", HTS: "1806.20"}}},
		}
		// The reply content is irrelevant here; only the model id used matters.
		_, _ = eng.Predict(context.Background(), c)
		if gotModel != "engine-model-x" || len(model.Calls()) == 0 {
			t.Fatalf("model id = %q (calls %d), want cfg.EngineModel via the supplied model", gotModel, len(model.Calls()))
		}
	})

	t.Run("dedicated gateway wins over the engine model", func(t *testing.T) {
		t.Setenv("JAPAN_LITELLM_BASE_URL", "https://gateway.example/v1")
		t.Setenv("JAPAN_LITELLM_API_KEY", "secret")
		model := llm.FakeText(`{}`)
		cfg := config.Config{LLMProvider: config.ProviderVertex, JapanEngine: true, EngineModel: "engine-model-x"}
		got, err := japanAuthority(cfg, model, meter, logger)
		if err != nil || got.Engine == nil {
			t.Fatalf("gateway Japan: %+v, %v", got, err)
		}
		t.Setenv("JAPAN_LITELLM_API_KEY", "")
		if _, err := japanAuthority(cfg, model, meter, logger); err == nil {
			t.Fatal("half-set gateway must fail closed, not fall back to the engine model")
		}
	})

	t.Run("invalid gateway fails closed", func(t *testing.T) {
		t.Setenv("JAPAN_LITELLM_BASE_URL", "http://gateway.example")
		t.Setenv("JAPAN_LITELLM_API_KEY", "secret")
		_, err := japanAuthority(config.Config{LLMProvider: config.ProviderVertex, JapanEngine: true}, nil, meter, logger)
		if err == nil || !strings.Contains(err.Error(), "HTTPS URL") {
			t.Fatalf("invalid gateway error = %v, want an HTTPS URL error", err)
		}
	})

	t.Run("gateway registered without Vertex", func(t *testing.T) {
		t.Setenv("JAPAN_LITELLM_BASE_URL", "https://gateway.example/v1")
		t.Setenv("JAPAN_LITELLM_API_KEY", "secret")
		got, err := japanAuthority(config.Config{LLMProvider: config.ProviderVertex, JapanEngine: true}, nil, meter, logger)
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

// Through the production wiring, Japan is on by default and an explicit
// JAPAN_ENGINE=off makes it unavailable, even with gateway settings present.
func TestJapanFollowsTheEnableGate(t *testing.T) {
	t.Setenv("JAPAN_LITELLM_BASE_URL", "https://gateway.example/v1")
	t.Setenv("JAPAN_LITELLM_API_KEY", "secret")
	meter := spend.New(spend.NewMemory(), 100)
	base := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) { return llm.Response{Text: "{}"}, nil }}
	t.Setenv("JAPAN_ENGINE", "off")
	off := engineEnv(t, "", base, meter)
	if off.Jurisdictions.Supported(jurisdiction.JP) {
		t.Fatal("Japan registered with JAPAN_ENGINE=off")
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
