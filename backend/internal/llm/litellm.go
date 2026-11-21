package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LiteLLM calls an OpenAI-compatible LiteLLM gateway to Gemini:
// POST {BaseURL}/v1/chat/completions. It exists for offline evaluation only
// (cmd/eval, D151): the server's config still accepts only vertex and fake
// (D49), so nothing deployed can use it. It also carries inline media parts
// (images, PDFs) for the offline image-import check, cmd/mediacheck (D167).
// Every call sends temperature 0 and the fixed Seed, the same as Vertex.
type LiteLLM struct {
	baseURL string
	apiKey  string
	client  *http.Client
	noCache bool
}

// SkipCache makes every call bypass the gateway's response cache, so a
// repeat run is a fresh model call and not a replay of an earlier answer
// (repeatability runs, D190). Off by default.
func (l *LiteLLM) SkipCache(on bool) { l.noCache = on }

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

// CostTally sums the cost the gateway reports for each call made with a
// context that carries it (ContextWithCostTally). Vertex reports no cost, so
// a tally stays zero there and callers price tokens instead.
type CostTally struct {
	mu  sync.Mutex
	usd float64
}

// USD returns the total so far.
func (t *CostTally) USD() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.usd
}

func (t *CostTally) add(v float64) {
	t.mu.Lock()
	t.usd += v
	t.mu.Unlock()
}

type costTallyKey struct{}

// ContextWithCostTally returns a context whose calls add their reported cost
// to t.
func ContextWithCostTally(ctx context.Context, t *CostTally) context.Context {
	return context.WithValue(ctx, costTallyKey{}, t)
}

func tallyFrom(ctx context.Context) *CostTally {
	t, _ := ctx.Value(costTallyKey{}).(*CostTally)
	return t
}

type oaMessage struct {
	Role       string       `json:"role"`
	Content    *string      `json:"content"`
	ToolCalls  []oaToolCall `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
	Name       string       `json:"name,omitempty"`
}

// oaReqMessage is a request turn. Content is a string, or for a user turn
// with media parts a list of OpenAI content parts (text, then each image or
// PDF as a base64 data URL).
type oaReqMessage struct {
	Role       string       `json:"role"`
	Content    any          `json:"content"`
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
	Model          string         `json:"model"`
	Messages       []oaReqMessage `json:"messages"`
	Temperature    float64        `json:"temperature"`
	Seed           int            `json:"seed"`
	MaxTokens      int            `json:"max_tokens,omitempty"`
	Tools          []oaTool       `json:"tools,omitempty"`
	ResponseFormat any            `json:"response_format,omitempty"`
	// Cache asks LiteLLM not to answer from its cache ({"no-cache": true}).
	Cache map[string]bool `json:"cache,omitempty"`
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

// costHeader is the per-request cost LiteLLM reports, in USD.
const costHeader = "X-Litellm-Response-Cost"

// Generate implements Model.
func (l *LiteLLM) Generate(ctx context.Context, req Request) (Response, error) {
	oreq, err := l.buildRequest(req)
	if err != nil {
		return Response{}, fmt.Errorf("litellm: %w", err)
	}
	body, err := json.Marshal(oreq)
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
		// Transport errors name the URL, never the key; scrub both anyway.
		return Response{}, fmt.Errorf("litellm: %w: %s", ErrUnavailable, l.scrub(err.Error()))
	}
	defer httpResp.Body.Close()
	if t := tallyFrom(ctx); t != nil {
		if v, err := strconv.ParseFloat(httpResp.Header.Get(costHeader), 64); err == nil && v > 0 {
			t.add(v)
		}
	}
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
	resp := Response{
		ModelID:        out.Model,
		RequestedModel: req.Model,
		Provider:       "litellm",
		Latency:        time.Since(start),
		Usage: Usage{
			InputTokens:  out.Usage.PromptTokens,
			OutputTokens: out.Usage.CompletionTokens,
			TotalTokens:  out.Usage.TotalTokens,
		},
	}
	if resp.ModelID == "" {
		resp.ModelID = req.Model
	}
	if len(out.Choices) == 0 {
		return resp, fmt.Errorf("litellm: %w: no choices", ErrBadResponse)
	}
	choice := out.Choices[0]
	resp.FinishReason = choice.FinishReason
	if choice.Message.Content != nil {
		resp.Text = *choice.Message.Content
	}
	for _, tc := range choice.Message.ToolCalls {
		args := json.RawMessage(tc.Function.Arguments)
		if !json.Valid(args) {
			return resp, fmt.Errorf("litellm: %w: tool call %s has invalid JSON arguments", ErrBadResponse, tc.Function.Name)
		}
		// LiteLLM encodes Gemini's thought signature into the call ID, so
		// sending the ID back unchanged preserves it.
		resp.ToolCalls = append(resp.ToolCalls, ToolCall{ID: tc.ID, Name: tc.Function.Name, Args: args})
	}
	if strings.TrimSpace(resp.Text) == "" && len(resp.ToolCalls) == 0 {
		// Name the finish reason (e.g. "length" when thinking used the whole
		// output budget) so an eval row says why there was no answer.
		return resp, fmt.Errorf("litellm: %w: empty answer (finish_reason %q, %d output tokens)", ErrBadResponse, resp.FinishReason, resp.Usage.OutputTokens)
	}
	return resp, nil
}

// mediaContent turns a user turn with media parts into OpenAI content parts:
// the text first, then each part as a base64 data URL (image_url for
// images, file for a PDF), the order Vertex sends them in.
func mediaContent(i int, m Message) ([]map[string]any, error) {
	if m.Role != RoleUser {
		return nil, fmt.Errorf("message %d: media parts are only allowed on user turns", i)
	}
	var parts []map[string]any
	if m.Content != "" {
		parts = append(parts, map[string]any{"type": "text", "text": m.Content})
	}
	for k, p := range m.Parts {
		if p.MIMEType == "" || len(p.Data) == 0 {
			return nil, fmt.Errorf("message %d: part %d: empty media part", i, k)
		}
		url := "data:" + p.MIMEType + ";base64," + base64.StdEncoding.EncodeToString(p.Data)
		if p.MIMEType == "application/pdf" {
			parts = append(parts, map[string]any{"type": "file", "file": map[string]any{"file_data": url}})
		} else {
			parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": url}})
		}
	}
	return parts, nil
}

func (l *LiteLLM) buildRequest(req Request) (oaRequest, error) {
	out := oaRequest{Model: req.Model, Temperature: 0, Seed: Seed, MaxTokens: req.MaxOutputTokens}
	if l.noCache {
		out.Cache = map[string]bool{"no-cache": true}
	}
	if req.System != "" {
		out.Messages = append(out.Messages, oaReqMessage{Role: "system", Content: req.System})
	}
	for i, m := range req.Messages {
		om := oaReqMessage{Role: string(m.Role), ToolCallID: m.ToolCallID, Name: m.Name}
		switch {
		case len(m.Parts) > 0:
			parts, err := mediaContent(i, m)
			if err != nil {
				return oaRequest{}, err
			}
			om.Content = parts
		case m.Content != "" || len(m.ToolCalls) == 0:
			om.Content = m.Content
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
	return out, nil
}

// scrub removes the API key and the gateway address from text that may be
// returned or logged.
func (l *LiteLLM) scrub(s string) string {
	if l.apiKey != "" {
		s = strings.ReplaceAll(s, l.apiKey, "[redacted]")
	}
	if l.baseURL != "" {
		s = strings.ReplaceAll(s, l.baseURL, "[gateway]")
	}
	return ScrubGatewayHost(s, l.baseURL)
}

// GatewayHostPlaceholder replaces the gateway's host name in any text that
// may be written to a committed result file or a log.
const GatewayHostPlaceholder = "litellm-gateway.invalid"

// ScrubGatewayHost replaces the host of baseURL (a DNS lookup or dial error
// names it without the scheme) with GatewayHostPlaceholder.
func ScrubGatewayHost(s, baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil || u.Hostname() == "" {
		return s
	}
	return strings.ReplaceAll(s, u.Hostname(), GatewayHostPlaceholder)
}

var _ Model = (*LiteLLM)(nil)
