package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/spend"
)

func TestCredentialHint(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"adc missing", errors.New("google: could not find default credentials"), true},
		{"other error", errors.New("port already in use"), false},
		{"nil", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := credentialHint(tc.err)
			if (got != "") != tc.want {
				t.Fatalf("credentialHint(%v) = %q, want hint=%v", tc.err, got, tc.want)
			}
			if tc.want && (!strings.Contains(got, "gcloud auth application-default login") || !strings.Contains(got, "LLM_PROVIDER=fake")) {
				t.Errorf("hint is missing a recovery step: %q", got)
			}
		})
	}
}

// Wiring (#312): the meter sits once on each raw client, so the shared model
// and an engine model with a fallback each charge every attempt exactly once.
func TestBuildModelsMetersEachRawAttemptOnce(t *testing.T) {
	usage := llm.Usage{InputTokens: 10000, OutputTokens: 1000}
	newBaseFake := func(calls *[]string) llm.Model {
		return &llm.Fake{Respond: func(r llm.Request) (llm.Response, error) {
			*calls = append(*calls, r.Model)
			if r.Model == "gemini-4-argon" {
				return llm.Response{Usage: usage}, &llm.APIError{Provider: "vertex", StatusCode: 429}
			}
			return llm.Response{Text: "{}", Usage: usage}, nil
		}}
	}
	argon := evalkit.Cost(evalkit.PromoPrices, "gemini-4-argon", 10000, 1000)
	flash := evalkit.Cost(evalkit.PromoPrices, "gemini-3.8-flash", 10000, 1000)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	total := func(m *spend.Meter) float64 { _, v := m.Over(context.Background()); return v }

	t.Run("engine with fallback", func(t *testing.T) {
		var calls []string
		meter := spend.New(spend.NewMemory(), 100)
		cfg := config.Config{FallbackEngineModel: "gemini-3.8-flash", EngineAttempts: 2, LLMProvider: config.ProviderFake}
		_, engine, err := buildModels(context.Background(), cfg, newBaseFake(&calls), meter, logger, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := engine.Generate(context.Background(), llm.Request{Model: "gemini-4-argon"}); err != nil {
			t.Fatal(err)
		}
		// 2 primary attempts at the primary price, 1 fallback at its own.
		if want := 2*argon + flash; math.Abs(total(meter)-want) > 1e-9 || len(calls) != 3 {
			t.Fatalf("total %v want %v, calls %v", total(meter), want, calls)
		}
	})
	t.Run("shared model and fallback off", func(t *testing.T) {
		var calls []string
		meter := spend.New(spend.NewMemory(), 100)
		cfg := config.Config{LLMProvider: config.ProviderFake}
		model, engine, err := buildModels(context.Background(), cfg, newBaseFake(&calls), meter, logger, false)
		if err != nil {
			t.Fatal(err)
		}
		if engine != model {
			t.Fatal("with the fallback off the engine uses the shared model")
		}
		if _, err := model.Generate(context.Background(), llm.Request{Model: "gemini-3.8-flash"}); err != nil {
			t.Fatal(err)
		}
		if math.Abs(total(meter)-flash) > 1e-9 {
			t.Fatalf("total %v want %v (one charge)", total(meter), flash)
		}
	})
	t.Run("smoke is unmetered", func(t *testing.T) {
		var calls []string
		model, _, err := buildModels(context.Background(), config.Config{}, newBaseFake(&calls), nil, logger, true)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := model.Generate(context.Background(), llm.Request{Model: "gemini-3.8-flash"}); err != nil {
			t.Fatal(err)
		}
	})
}
