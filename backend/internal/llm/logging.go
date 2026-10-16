package llm

import (
	"context"
	"log/slog"
	"time"
)

// WithLogging logs one line per call: provider, model, tokens, latency and
// tool-call names. Prompts and answers are logged only at debug level, since
// they can contain a user's BOM.
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
		l.log.ErrorContext(ctx, "llm call failed", append(attrs, "err", err)...)
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
	l.log.DebugContext(ctx, "llm call content", "system", req.System, "messages", req.Messages, "text", resp.Text)
	return resp, nil
}
