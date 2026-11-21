// Package gateway is the eval-only OpenAI-compatible LiteLLM client. It is
// constructed only inside cmd/readiness-eval, is never registered as a server
// provider, and is meant to be wrapped by budget.Guard. One Generate call is
// exactly one HTTP request: no retry layer, no HTTP/2 transparent replay, no
// redirects, no proxy from the environment.
package gateway

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/readiness/budget"
)

// ErrRedirect means the gateway answered with a redirect. It is refused, never
// followed, and counts as a failed attempt.
var ErrRedirect = errors.New("gateway: redirect refused")

// Config for one client.
type Config struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
	// AllowLoopbackHTTP permits http:// to localhost, for tests with
	// httptest only. The CLI never sets it.
	AllowLoopbackHTTP bool
}

// Client implements llm.Model.
type Client struct {
	endpoint string
	host     string
	key      string
	hc       *http.Client
}

// permitTransport is the only route to the network. Every RoundTrip needs a
// fresh one-shot permit from the budget guard, the exact allowlisted host over
// https (or loopback http in tests), no userinfo, no query and the one fixed
// path. A redirect hop would be a second RoundTrip with a used permit.
type permitTransport struct {
	base          http.RoundTripper
	host          string
	path          string
	allowHTTPLoop bool
}

func (t permitTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	// Validate the target first: a refusal here leaves the permit untouched,
	// so the guard can see that nothing was sent.
	u := r.URL
	okScheme := u.Scheme == "https" || (u.Scheme == "http" && t.allowHTTPLoop && isLoopback(u.Hostname()))
	if !okScheme || u.Host != t.host || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != t.path {
		return nil, fmt.Errorf("%w: request target is not the allowlisted endpoint", budget.ErrNotSent)
	}
	p := budget.PermitFrom(r.Context())
	if err := p.Take(); err != nil {
		if errors.Is(err, budget.ErrNoPermit) {
			return nil, fmt.Errorf("%w: %w", budget.ErrNotSent, err) // no permit was ever taken
		}
		return nil, err // ErrPermitUsed: an earlier request may have been sent; no unsent claim
	}
	return t.base.RoundTrip(r)
}

var proxyVars = []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"}

// checkProxyEnv refuses to run when a proxy is configured for the gateway
// host. Clearing it silently would bypass the administrator's network
// policy, and using it would route the key and prompts through an unverified
// hop; neither is the tool's decision. NO_PROXY coverage (or loopback, which
// Go never proxies) lets the direct connection proceed.
func checkProxyEnv(host string, getenv func(string) string) error {
	set := false
	for _, v := range proxyVars {
		if getenv(v) != "" {
			set = true
		}
	}
	if !set || isLoopback(host) {
		return nil
	}
	for _, v := range []string{"NO_PROXY", "no_proxy"} {
		for _, p := range strings.Split(getenv(v), ",") {
			p = strings.TrimSpace(p)
			p = strings.TrimPrefix(p, ".")
			if p == "*" || (p != "" && (host == p || strings.HasSuffix(host, "."+p))) {
				return nil
			}
		}
	}
	return errors.New("gateway: refusing to run with a proxy configured in the environment that does not exclude the gateway host (not clearing it; an administrator must confirm the route)")
}

// FromEnv reads only LITELLM_BASE_URL and LITELLM_API_KEY from the process
// environment. It reads no files and echoes neither value.
func FromEnv(timeout time.Duration) (*Client, error) {
	base, key := os.Getenv("LITELLM_BASE_URL"), os.Getenv("LITELLM_API_KEY")
	if base == "" || key == "" {
		return nil, errors.New("gateway: LITELLM_BASE_URL and LITELLM_API_KEY must be set in the environment")
	}
	return New(Config{BaseURL: base, APIKey: key, Timeout: timeout})
}

// New validates the configuration. Error text never contains the key.
func New(c Config) (*Client, error) {
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("gateway: base URL must be an absolute URL without credentials, query or fragment")
	}
	loop := isLoopback(u.Hostname())
	switch {
	case u.Scheme == "https":
	case u.Scheme == "http" && c.AllowLoopbackHTTP && loop:
	default:
		return nil, errors.New("gateway: base URL must use https")
	}
	if c.APIKey == "" || strings.ContainsAny(c.APIKey, "\r\n\x00 ") {
		return nil, errors.New("gateway: API key is empty or contains control characters")
	}
	if c.Timeout <= 0 {
		c.Timeout = 120 * time.Second
	}
	if err := checkProxyEnv(u.Hostname(), os.Getenv); err != nil {
		return nil, err
	}
	tr := &http.Transport{
		Proxy:                 nil, // direct only; env proxies are refused above, never cleared
		DisableKeepAlives:     true,
		ForceAttemptHTTP2:     false,
		TLSNextProto:          map[string]func(string, *tls.Conn) http.RoundTripper{}, // HTTP/1.1 only: no h2 replay
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		ResponseHeaderTimeout: c.Timeout,
		TLSHandshakeTimeout:   15 * time.Second,
	}
	ep := strings.TrimRight(u.String(), "/") + "/v1/chat/completions"
	eu, _ := url.Parse(ep)
	cl := &Client{
		endpoint: ep,
		host:     u.Host,
		key:      c.APIKey,
		hc: &http.Client{
			Timeout:       c.Timeout,
			Transport:     permitTransport{base: tr, host: u.Host, path: eu.Path, allowHTTPLoop: c.AllowLoopbackHTTP},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
	return cl, nil
}

func isLoopback(h string) bool {
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// Provider implements llm.Model.
func (c *Client) Provider() string { return "litellm-eval" }

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

type oaTool struct {
	Type     string   `json:"type"`
	Function llm.Tool `json:"function"`
}

type oaRequest struct {
	Model          string      `json:"model"`
	Messages       []oaMessage `json:"messages"`
	Temperature    float64     `json:"temperature"`
	MaxTokens      int         `json:"max_tokens"`
	Seed           int         `json:"seed"`
	Tools          []oaTool    `json:"tools,omitempty"`
	ResponseFormat any         `json:"response_format,omitempty"`
}

type oaResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message      oaMessage `json:"message"`
		FinishReason string    `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		TotalTokens         int `json:"total_tokens"`
		CompletionTokensDet struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
	} `json:"usage"`
}

// Generate implements llm.Model with exactly one HTTP request.
func (c *Client) Generate(ctx context.Context, req llm.Request) (llm.Response, error) {
	if req.MaxOutputTokens <= 0 {
		return llm.Response{}, fmt.Errorf("%w: refusing a request without max output tokens", budget.ErrNotSent)
	}
	for _, m := range req.Messages {
		if len(m.Parts) > 0 {
			// The eval gateway sends text only; dropping media silently would
			// score a request the model never saw.
			return llm.Response{}, fmt.Errorf("%w: media parts are not supported by this gateway", budget.ErrNotSent)
		}
	}
	body, err := json.Marshal(build(req))
	if err != nil {
		return llm.Response{}, fmt.Errorf("%w: encode request: %v", budget.ErrNotSent, err)
	}
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return llm.Response{}, fmt.Errorf("%w: build request failed", budget.ErrNotSent)
	}
	hr.GetBody = nil // a body that cannot be replayed cannot be silently retried
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("Authorization", "Bearer "+c.key)
	start := time.Now()
	resp, err := c.hc.Do(hr)
	if err != nil {
		if errors.Is(err, budget.ErrPermitUsed) {
			return llm.Response{}, fmt.Errorf("gateway: %w", budget.ErrPermitUsed)
		}
		// Refused inside RoundTrip before any byte left the process.
		if errors.Is(err, budget.ErrNotSent) {
			if errors.Is(err, budget.ErrNoPermit) {
				return llm.Response{}, fmt.Errorf("gateway: %w: %w", budget.ErrNotSent, budget.ErrNoPermit)
			}
			return llm.Response{}, fmt.Errorf("gateway: %w", budget.ErrNotSent)
		}
		if ctx.Err() != nil {
			return llm.Response{}, ctx.Err()
		}
		return llm.Response{}, fmt.Errorf("gateway: %w: request failed", llm.ErrUnavailable)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return llm.Response{}, fmt.Errorf("%w: HTTP %d: %w", ErrRedirect, resp.StatusCode, &llm.APIError{Provider: "litellm-eval", StatusCode: resp.StatusCode})
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return llm.Response{}, fmt.Errorf("gateway: %w: read response", llm.ErrUnavailable)
	}
	if resp.StatusCode/100 != 2 {
		return llm.Response{}, &llm.APIError{Provider: "litellm-eval", StatusCode: resp.StatusCode, Body: c.scrub(trunc(string(raw), 200))}
	}
	var out oaResponse
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Choices) == 0 {
		return llm.Response{}, fmt.Errorf("gateway: %w: undecodable answer", llm.ErrBadResponse)
	}
	ch := out.Choices[0]
	r := llm.Response{ModelID: out.Model, RequestedModel: req.Model, Provider: "litellm-eval",
		Latency: time.Since(start), FinishReason: ch.FinishReason,
		Usage: llm.Usage{InputTokens: out.Usage.PromptTokens, OutputTokens: BilledOutput(out.Usage.PromptTokens, out.Usage.CompletionTokens, out.Usage.TotalTokens, out.Usage.CompletionTokensDet.ReasoningTokens), TotalTokens: out.Usage.TotalTokens}}
	// ModelID stays empty when the gateway does not report it: the guard
	// treats an unverifiable resolved model as a halt.
	if ch.Message.Content != nil {
		r.Text = *ch.Message.Content
	}
	for _, tc := range ch.Message.ToolCalls {
		args := json.RawMessage(tc.Function.Arguments)
		if !json.Valid(args) {
			return llm.Response{}, fmt.Errorf("gateway: %w: invalid tool arguments", llm.ErrBadResponse)
		}
		r.ToolCalls = append(r.ToolCalls, llm.ToolCall{ID: tc.ID, Name: tc.Function.Name, Args: args})
	}
	return r, nil
}

// BilledOutput is the largest output figure any usage field implies, so
// hidden reasoning tokens that a gateway reports only through total_tokens or
// completion_tokens_details are still priced as output.
func BilledOutput(prompt, completion, total, reasoning int) int {
	o := completion
	if d := total - prompt; d > o {
		o = d
	}
	if reasoning > o {
		o = reasoning
	}
	return o
}

func build(req llm.Request) oaRequest {
	out := oaRequest{Model: req.Model, Temperature: 0, MaxTokens: req.MaxOutputTokens, Seed: llm.Seed}
	sp := func(s string) *string { return &s }
	if req.System != "" {
		out.Messages = append(out.Messages, oaMessage{Role: "system", Content: sp(req.System)})
	}
	for _, m := range req.Messages {
		om := oaMessage{Role: string(m.Role), ToolCallID: m.ToolCallID, Name: m.Name}
		if m.Content != "" || len(m.ToolCalls) == 0 {
			om.Content = sp(m.Content)
		}
		for _, tc := range m.ToolCalls {
			var o oaToolCall
			o.ID, o.Type = tc.ID, "function"
			o.Function.Name, o.Function.Arguments = tc.Name, string(tc.Args)
			om.ToolCalls = append(om.ToolCalls, o)
		}
		out.Messages = append(out.Messages, om)
	}
	for _, t := range req.Tools {
		out.Tools = append(out.Tools, oaTool{Type: "function", Function: t})
	}
	if req.JSONSchema != nil {
		n := req.SchemaName
		if n == "" {
			n = "answer"
		}
		out.ResponseFormat = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": n, "schema": req.JSONSchema, "strict": true}}
	}
	return out
}

func (c *Client) scrub(s string) string { return strings.ReplaceAll(s, c.key, "[redacted]") }

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

var _ llm.Model = (*Client)(nil)
