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
		_, _ = io.WriteString(w, `{"model":"gemini-4-argon","choices":[{"message":{"role":"assistant","content":null,
			"tool_calls":[{"id":"call_1__thought__abc","type":"function","function":{"name":"search_rulings","arguments":"{\"query\":\"heat gun\"}"}}]},
			"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":120,"completion_tokens":7,"total_tokens":127}}`)
	}))
	defer srv.Close()

	m := NewLiteLLM(srv.URL+"/", testKey, 5*time.Second)
	resp, err := m.Generate(context.Background(), Request{
		Model:  "gemini-4-argon",
		System: "sys",
		Messages: []Message{
			{Role: RoleUser, Content: "find rulings"},
			{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "call_0", Name: "search_rulings", Args: json.RawMessage(`{"query":"x"}`)}}},
			{Role: RoleTool, ToolCallID: "call_0", Name: "search_rulings", Content: "[]"},
		},
		Tools:           []Tool{{Name: "search_rulings", Description: "d", Parameters: map[string]any{"type": "object"}}},
		JSONSchema:      map[string]any{"type": "object"},
		SchemaName:      "verdict",
		MaxOutputTokens: 100,
	})
	if err != nil {
		t.Fatal(err)
	}

	if got["temperature"] != 0.0 || got["model"] != "gemini-4-argon" || got["max_tokens"] != 100.0 {
		t.Errorf("request fields wrong: %v", got)
	}
	msgs := got["messages"].([]any)
	if len(msgs) != 4 || msgs[0].(map[string]any)["role"] != "system" {
		t.Errorf("messages = %v", msgs)
	}
	asst := msgs[2].(map[string]any)
	if tc := asst["tool_calls"].([]any)[0].(map[string]any); tc["id"] != "call_0" ||
		tc["function"].(map[string]any)["arguments"] != `{"query":"x"}` {
		t.Errorf("assistant tool call = %v", tc)
	}
	if msgs[3].(map[string]any)["tool_call_id"] != "call_0" {
		t.Errorf("tool message = %v", msgs[3])
	}
	rf := got["response_format"].(map[string]any)
	if rf["type"] != "json_schema" || rf["json_schema"].(map[string]any)["name"] != "verdict" {
		t.Errorf("response_format = %v", rf)
	}

	if resp.ModelID != "gemini-4-argon" || resp.Provider != "litellm" || resp.FinishReason != "tool_calls" {
		t.Errorf("response meta = %+v", resp)
	}
	if resp.Usage != (Usage{InputTokens: 120, OutputTokens: 7, TotalTokens: 127}) {
		t.Errorf("usage = %+v", resp.Usage)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].ID != "call_1__thought__abc" || string(resp.ToolCalls[0].Args) != `{"query":"heat gun"}` {
		t.Errorf("tool calls = %+v", resp.ToolCalls)
	}
}

func TestLiteLLMErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		check   func(error) bool
		retries bool
	}{
		{"429 is retryable", 429, `{"error":"rate limit"}`, func(e error) bool { var a *APIError; return errors.As(e, &a) && a.Retryable() }, true},
		{"401 is not", 401, `{"error":"bad key ` + testKey + `"}`, func(e error) bool { var a *APIError; return errors.As(e, &a) && !a.Retryable() }, false},
		{"garbage body", 200, `not json`, func(e error) bool { return errors.Is(e, ErrBadResponse) }, false},
		{"no choices", 200, `{"choices":[]}`, func(e error) bool { return errors.Is(e, ErrBadResponse) }, false},
		{"bad tool args", 200, `{"choices":[{"message":{"tool_calls":[{"id":"1","function":{"name":"f","arguments":"{oops"}}]}}]}`,
			func(e error) bool { return errors.Is(e, ErrBadResponse) }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			defer srv.Close()
			_, err := NewLiteLLM(srv.URL, testKey, time.Second).Generate(context.Background(), Request{Model: "m"})
			if err == nil || !tt.check(err) {
				t.Fatalf("error = %v", err)
			}
			if strings.Contains(err.Error(), testKey) {
				t.Fatalf("error leaks the API key: %v", err)
			}
		})
	}
}

func TestLiteLLMUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { time.Sleep(200 * time.Millisecond) }))
	defer srv.Close()
	_, err := NewLiteLLM(srv.URL, testKey, 20*time.Millisecond).Generate(context.Background(), Request{Model: "m"})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}
