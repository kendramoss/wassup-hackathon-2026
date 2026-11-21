package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testKey = "sk-test-secret"

func TestLiteLLMRequestAndResponse(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer "+testKey {
			t.Errorf("unexpected request %s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		w.Header().Set("x-litellm-response-cost", "0.0125")
		_, _ = io.WriteString(w, `{"model":"gemini-4-carbon","choices":[{"message":{"role":"assistant","content":"{\"origin\":\"Vietnam\"}"},
			"finish_reason":"stop"}],"usage":{"prompt_tokens":120,"completion_tokens":7,"total_tokens":127}}`)
	}))
	defer srv.Close()

	m := NewLiteLLM(srv.URL+"/", testKey, 5*time.Second)
	tally := &CostTally{}
	ctx := ContextWithCostTally(context.Background(), tally)
	resp, err := m.Generate(ctx, Request{
		Model:           "gemini-4-carbon",
		System:          "sys",
		Messages:        []Message{{Role: RoleUser, Content: "facts"}},
		JSONSchema:      map[string]any{"type": "object"},
		SchemaName:      "prediction",
		MaxOutputTokens: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["temperature"] != 0.0 || got["seed"] != float64(Seed) || got["model"] != "gemini-4-carbon" || got["max_tokens"] != 100.0 {
		t.Errorf("request fields wrong: %v", got)
	}
	msgs := got["messages"].([]any)
	if len(msgs) != 2 || msgs[0].(map[string]any)["role"] != "system" || msgs[1].(map[string]any)["content"] != "facts" {
		t.Errorf("messages = %v", msgs)
	}
	rf := got["response_format"].(map[string]any)
	js := rf["json_schema"].(map[string]any)
	if rf["type"] != "json_schema" || js["name"] != "prediction" || js["strict"] != true {
		t.Errorf("response_format = %v", rf)
	}
	if resp.Text != `{"origin":"Vietnam"}` || resp.Provider != "litellm" || resp.ModelID != "gemini-4-carbon" ||
		resp.RequestedModel != "gemini-4-carbon" || resp.Usage.InputTokens != 120 || resp.Usage.OutputTokens != 7 || resp.FinishReason != "stop" {
		t.Errorf("resp = %+v", resp)
	}
	if tally.USD() != 0.0125 {
		t.Errorf("tally = %v, want 0.0125", tally.USD())
	}
}

func TestLiteLLMToolCallsRoundTrip(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		_, _ = io.WriteString(w, `{"model":"m","choices":[{"message":{"role":"assistant","content":null,
			"tool_calls":[{"id":"call_1__thought__abc","type":"function","function":{"name":"search","arguments":"{\"q\":\"x\"}"}}]},
			"finish_reason":"tool_calls"}],"usage":{}}`)
	}))
	defer srv.Close()
	resp, err := NewLiteLLM(srv.URL, testKey, time.Second).Generate(context.Background(), Request{
		Model: "m",
		Messages: []Message{
			{Role: RoleUser, Content: "go"},
			{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "call_0", Name: "search", Args: json.RawMessage(`{"q":"y"}`)}}},
			{Role: RoleTool, ToolCallID: "call_0", Name: "search", Content: "[]"},
		},
		Tools: []Tool{{Name: "search", Description: "d", Parameters: map[string]any{"type": "object"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	asst := got["messages"].([]any)[1].(map[string]any)
	if asst["content"] != nil {
		t.Errorf("assistant tool-call turn should send null content, got %v", asst["content"])
	}
	if tc := asst["tool_calls"].([]any)[0].(map[string]any); tc["id"] != "call_0" || tc["function"].(map[string]any)["arguments"] != `{"q":"y"}` {
		t.Errorf("tool call = %v", tc)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].ID != "call_1__thought__abc" || string(resp.ToolCalls[0].Args) != `{"q":"x"}` {
		t.Errorf("tool calls = %+v", resp.ToolCalls)
	}
}

func TestLiteLLMErrorsHideKeyAndAddress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, "rate limited for key "+testKey)
	}))
	defer srv.Close()
	_, err := NewLiteLLM(srv.URL, testKey, time.Second).Generate(context.Background(), Request{Model: "m", Messages: []Message{{Role: RoleUser, Content: "x"}}})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 429 || !apiErr.Retryable() {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), testKey) {
		t.Errorf("key leaked: %v", err)
	}

	srv.Close()
	_, err = NewLiteLLM(srv.URL, testKey, time.Second).Generate(context.Background(), Request{Model: "m", Messages: []Message{{Role: RoleUser, Content: "x"}}})
	if !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), srv.URL) {
		t.Errorf("transport error = %v", err)
	}
}

func TestLiteLLMSendsMediaPartsAsDataURLs(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = io.WriteString(w, `{"model":"m","choices":[{"message":{"role":"assistant","content":"{}"},"finish_reason":"stop"}],"usage":{}}`)
	}))
	defer srv.Close()
	_, err := NewLiteLLM(srv.URL, testKey, time.Second).Generate(context.Background(), Request{
		Model: "m", Messages: []Message{{Role: RoleUser, Content: "read", Parts: []Part{
			{MIMEType: "image/png", Data: []byte{1, 2}},
			{MIMEType: "application/pdf", Data: []byte{3}},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	msgs := got["messages"].([]any)
	parts := msgs[0].(map[string]any)["content"].([]any)
	if len(parts) != 3 {
		t.Fatalf("parts = %v", parts)
	}
	if p := parts[0].(map[string]any); p["type"] != "text" || p["text"] != "read" {
		t.Errorf("text part = %v", p)
	}
	if p := parts[1].(map[string]any); p["type"] != "image_url" || p["image_url"].(map[string]any)["url"] != "data:image/png;base64,AQI=" {
		t.Errorf("image part = %v", p)
	}
	if p := parts[2].(map[string]any); p["type"] != "file" || p["file"].(map[string]any)["file_data"] != "data:application/pdf;base64,Aw==" {
		t.Errorf("pdf part = %v", p)
	}
}

func TestLiteLLMRefusesBadMediaParts(t *testing.T) {
	for _, m := range []Message{
		{Role: RoleUser, Content: "x", Parts: []Part{{MIMEType: "image/png"}}},
		{Role: RoleAssistant, Content: "x", Parts: []Part{{MIMEType: "image/png", Data: []byte{1}}}},
	} {
		_, err := NewLiteLLM("http://unused", testKey, time.Second).Generate(context.Background(), Request{Model: "m", Messages: []Message{m}})
		if err == nil {
			t.Errorf("%v: no error", m.Role)
		}
	}
}

func TestScrubGatewayHost(t *testing.T) {
	got := ScrubGatewayHost(`dial tcp: lookup gw.example.net: no such host; Post "https://gw.example.net/v1"`, "https://gw.example.net")
	if strings.Contains(got, "gw.example.net") || !strings.Contains(got, GatewayHostPlaceholder) {
		t.Errorf("got %q", got)
	}
	if got := ScrubGatewayHost("plain", ""); got != "plain" {
		t.Errorf("no base URL: %q", got)
	}
}

func TestLiteLLMSkipCacheSendsNoCache(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		bodies = append(bodies, got)
		_, _ = io.WriteString(w, `{"model":"m","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()

	m := NewLiteLLM(srv.URL, testKey, 5*time.Second)
	req := Request{Model: "m", Messages: []Message{{Role: RoleUser, Content: "x"}}}
	if _, err := m.Generate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	m.SkipCache(true)
	if _, err := m.Generate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if _, ok := bodies[0]["cache"]; ok {
		t.Errorf("default request has a cache field: %v", bodies[0]["cache"])
	}
	c, _ := bodies[1]["cache"].(map[string]any)
	if c["no-cache"] != true {
		t.Errorf("SkipCache request cache = %v, want no-cache true", bodies[1]["cache"])
	}
}
