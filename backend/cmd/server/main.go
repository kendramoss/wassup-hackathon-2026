// Command server is the Origin Proof API: origin prediction, plan search and
// precedent verification for CBP substantial-transformation cases.
//
// Configuration comes from the environment (see internal/config). Flags:
//
//	-smoke  make one live call to the engine model and print provider, model,
//	        tokens and latency as JSON, then exit
//
// Endpoints so far:
//
//	GET /health  liveness check (not /healthz: Cloud Run reserves paths ending in "z")
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/httpapi"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

func main() {
	smoke := flag.Bool("smoke", false, "make one live engine-model call and exit")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger, *smoke); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger, smoke bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger.Info("config", "summary", cfg.String())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	model, err := newModel(ctx, cfg, logger)
	if err != nil {
		return err
	}
	if smoke {
		return runSmoke(ctx, model, cfg)
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           httpapi.NewHandler(logger, httpapi.Info{Provider: model.Provider(), Version: os.Getenv("K_REVISION")}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	logger.Info("listening", "port", cfg.Port, "llm_provider", model.Provider())
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server: %w", err)
	}
	return nil
}

// newModel builds the configured provider wrapped with retry and logging.
func newModel(ctx context.Context, cfg config.Config, logger *slog.Logger) (llm.Model, error) {
	var base llm.Model
	switch cfg.LLMProvider {
	case config.ProviderLiteLLM:
		logger.Warn("using the LiteLLM proxy: development only; results are labelled proxy, development only")
		base = llm.NewLiteLLM(cfg.LiteLLMBaseURL, cfg.LiteLLMAPIKey, cfg.LLMTimeout)
	case config.ProviderVertex:
		v, err := llm.NewVertex(ctx, llm.VertexOptions{Project: cfg.GCPProject, Location: cfg.GCPLocation, Timeout: cfg.LLMTimeout})
		if err != nil {
			return nil, err
		}
		base = v
	case config.ProviderFake:
		base = llm.FakeText(`{"origin_country":"China","confidence":0.5}`)
	default:
		return nil, fmt.Errorf("unknown LLM provider %q", cfg.LLMProvider)
	}
	return llm.WithLogging(llm.WithRetry(base, cfg.LLMMaxRetries, time.Second), logger), nil
}

// smokeSchema is a minimal origin answer, enough to prove JSON-schema output
// works end to end.
var smokeSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"origin_country": map[string]any{"type": "string"},
		"confidence":     map[string]any{"type": "number"},
	},
	"required":             []string{"origin_country", "confidence"},
	"additionalProperties": false,
}

func runSmoke(ctx context.Context, model llm.Model, cfg config.Config) error {
	ctx, cancel := context.WithTimeout(ctx, cfg.LLMTimeout)
	defer cancel()
	resp, err := model.Generate(ctx, llm.Request{
		Model:  cfg.EngineModel,
		System: "You are a US customs origin analyst. Answer with JSON only.",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "A heat gun is assembled in Thailand from a Thai housing and heating element " +
			"and a Chinese motor, fan and circuit board. Which country of origin would CBP most likely assign?"}},
		JSONSchema:      smokeSchema,
		SchemaName:      "smoke",
		MaxOutputTokens: cfg.MaxOutputTokens,
	})
	if err != nil {
		return fmt.Errorf("smoke: %w", err)
	}
	var answer map[string]any
	if err := resp.DecodeJSON(&answer); err != nil {
		return fmt.Errorf("smoke: %w", err)
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"llm_provider": resp.Provider, "model_id": resp.ModelID, "usage": resp.Usage,
		"latency_ms": resp.Latency.Milliseconds(), "finish_reason": resp.FinishReason, "answer": answer,
	})
}
