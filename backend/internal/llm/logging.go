package llm

import (
	"context"
	"log/slog"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/logsafe"
)

// WithLogging logs one line per call: provider, model, tokens, latency and
// tool-call names. Prompts, messages and answers are NEVER logged, at any
// level, because they can contain a user's BOM (#285): the DEBUG line carries
// sizes only. Errors are logged as a class and a status number, never their
// text (a provider body can echo the request). To inspect a prompt use the
// prompt snapshots and the eval artifacts, which hold public or synthetic
// input only.
func WithLogging(m Model, logger *slog.Logger) Model {
	return &logging{next: m, log: logger}
}

type logging struct {
	next Model
	log  *slog.Logger
}

func (l *logging) Provider() string { return l.next.Provider() }

func (l *logging) Generate(ctx context.Context, req Request) (Response, error) {
	start := time.Now()
	resp, err := l.next.Generate(ctx, req)
	attrs := []any{
		"provider", l.next.Provider(), "model", req.Model,
		"latency_ms", time.Since(start).Milliseconds(),
	}
	if err != nil {
		l.log.ErrorContext(ctx, "llm call failed", append(attrs, "err", logsafe.Err(err))...)
		return resp, err
	}
	names := make([]string, len(resp.ToolCalls))
	for i, tc := range resp.ToolCalls {
		names[i] = tc.Name
	}
	l.log.InfoContext(ctx, "llm call", append(attrs,
		"model_id", resp.ModelID, "input_tokens", resp.Usage.InputTokens,
		"output_tokens", resp.Usage.OutputTokens, "tool_calls", names,
		"finish_reason", resp.FinishReason)...)
	l.log.DebugContext(ctx, "llm call sizes", "system_chars", len(req.System), "messages", len(req.Messages), "text_chars", len(resp.Text))
	return resp, nil
}
