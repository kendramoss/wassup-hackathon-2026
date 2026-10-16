package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// LiteLLM calls an OpenAI-compatible LiteLLM proxy:
// POST {BaseURL}/v1/chat/completions. It is for local development only;
// config refuses it on Cloud Run, and numbers it produces are labelled
// "proxy, development only".
type LiteLLM struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

// NewLiteLLM returns a LiteLLM provider. The key is sent only in the
// Authorization header and is scrubbed from any error text.
func NewLiteLLM(baseURL, apiKey string, timeout time.Duration) *LiteLLM {
	return &LiteLLM{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		client:  &http.Client{Timeout: timeout},
	}
}

// Provider implements Model.
func (l *LiteLLM) Provider() string { return "litellm" }

type oaMessage struct {
	Role       string       `json:"role"`
	Content    *string      `json:"content"`
	ToolCalls  []oaToolCall `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
	Name       string       `json:"name,omitempty"`
}

type oaToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type oaRequest struct {
	Model          string      `json:"model"`
	Messages       []oaMessage `json:"messages"`
	Temperature    float64     `json:"temperature"`
	MaxTokens      int         `json:"max_tokens,omitempty"`
	Tools          []oaTool    `json:"tools,omitempty"`
	ResponseFormat any         `json:"response_format,omitempty"`
}

type oaTool struct {
	Type     string `json:"type"`
	Function Tool   `json:"function"`
}

type oaResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message      oaMessage `json:"message"`
		FinishReason string    `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// Generate implements Model.
func (l *LiteLLM) Generate(ctx context.Context, req Request) (Response, error) {
	body, err := json.Marshal(l.buildRequest(req))
	if err != nil {
		return Response{}, fmt.Errorf("litellm: encode request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, l.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("litellm: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+l.apiKey)

	start := time.Now()
	httpResp, err := l.client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return Response{}, fmt.Errorf("litellm: %w", ctx.Err())
		}
		// The transport error text contains the URL, never the key, but scrub anyway.
		return Response{}, fmt.Errorf("litellm: %w: %s", ErrUnavailable, l.scrub(err.Error()))
	}
	defer httpResp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(httpResp.Body, 8<<20))
	if err != nil {
		return Response{}, fmt.Errorf("litellm: read response: %w", ErrUnavailable)
	}
	if httpResp.StatusCode/100 != 2 {
		return Response{}, &APIError{Provider: "litellm", StatusCode: httpResp.StatusCode, Body: l.scrub(truncate(string(raw), 300))}
	}
	var out oaResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return Response{}, fmt.Errorf("litellm: %w: decode: %v", ErrBadResponse, err)
	}
	if len(out.Choices) == 0 {
		return Response{}, fmt.Errorf("litellm: %w: no choices", ErrBadResponse)
	}
	choice := out.Choices[0]
	resp := Response{
		ModelID:      out.Model,
		Provider:     "litellm",
		Latency:      time.Since(start),
		FinishReason: choice.FinishReason,
		Usage: Usage{
			InputTokens:  out.Usage.PromptTokens,
			OutputTokens: out.Usage.CompletionTokens,
			TotalTokens:  out.Usage.TotalTokens,
		},
	}
	if resp.ModelID == "" {
		resp.ModelID = req.Model
	}
	if choice.Message.Content != nil {
		resp.Text = *choice.Message.Content
	}
	for _, tc := range choice.Message.ToolCalls {
		args := json.RawMessage(tc.Function.Arguments)
		if !json.Valid(args) {
			return Response{}, fmt.Errorf("litellm: %w: tool call %s has invalid JSON arguments", ErrBadResponse, tc.Function.Name)
		}
		// LiteLLM encodes Gemini's thought signature into the call ID, so
		// sending the ID back unchanged preserves it.
		resp.ToolCalls = append(resp.ToolCalls, ToolCall{ID: tc.ID, Name: tc.Function.Name, Args: args})
	}
	return resp, nil
}

func (l *LiteLLM) buildRequest(req Request) oaRequest {
	out := oaRequest{Model: req.Model, Temperature: 0, MaxTokens: req.MaxOutputTokens}
	if req.System != "" {
		out.Messages = append(out.Messages, oaMessage{Role: "system", Content: ptr(req.System)})
	}
	for _, m := range req.Messages {
		om := oaMessage{Role: string(m.Role), ToolCallID: m.ToolCallID, Name: m.Name}
		if m.Content != "" || len(m.ToolCalls) == 0 {
			om.Content = ptr(m.Content)
		}
		for _, tc := range m.ToolCalls {
			var otc oaToolCall
			otc.ID, otc.Type = tc.ID, "function"
			otc.Function.Name, otc.Function.Arguments = tc.Name, string(tc.Args)
			om.ToolCalls = append(om.ToolCalls, otc)
		}
		out.Messages = append(out.Messages, om)
	}
	for _, t := range req.Tools {
		out.Tools = append(out.Tools, oaTool{Type: "function", Function: t})
	}
	if req.JSONSchema != nil {
		name := req.SchemaName
		if name == "" {
			name = "answer"
		}
		out.ResponseFormat = map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": name, "schema": req.JSONSchema, "strict": true},
		}
	}
	return out
}

// scrub removes the API key from text that may be returned or logged.
func (l *LiteLLM) scrub(s string) string {
	if l.apiKey == "" {
		return s
	}
	return strings.ReplaceAll(s, l.apiKey, "[redacted]")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func ptr[T any](v T) *T { return &v }

var _ Model = (*LiteLLM)(nil)
