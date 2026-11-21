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

	"cloud.google.com/go/auth"
)

type staticToken struct{}

func (staticToken) Token(context.Context) (*auth.Token, error) {
	return &auth.Token{Value: "test-token", Type: "Bearer"}, nil
}

func newTestVertex(t *testing.T, handler http.HandlerFunc) *Vertex {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	v, err := NewVertex(context.Background(), VertexOptions{
		Project: "test-project", Location: "global", Timeout: 5 * time.Second,
		BaseURL: srv.URL, Credentials: auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: staticToken{}}),
	})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestVertexRequestAndResponse(t *testing.T) {
	var path string
	var got map[string]any
	v := newTestVertex(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		_, _ = io.WriteString(w, `{"modelVersion":"gemini-4-argon-001","candidates":[{"finishReason":"STOP","content":{"role":"model","parts":[
			{"text":"thinking...","thought":true},
			{"functionCall":{"name":"read_ruling","args":{"ruling":"N351460"}},"thoughtSignature":"c2ln"},
			{"text":"{\"ok\":true}"}]}}],
			"usageMetadata":{"promptTokenCount":200,"candidatesTokenCount":20,"thoughtsTokenCount":30,"totalTokenCount":250}}`)
	})
	resp, err := v.Generate(context.Background(), Request{
		Model:  "gemini-4-argon",
		System: "be precise",
		Messages: []Message{
			{Role: RoleUser, Content: "q"},
			{Role: RoleAssistant, ToolCalls: []ToolCall{
				{ID: "a", Name: "search_rulings", Args: json.RawMessage(`{"query":"x"}`), Signature: []byte("sig")},
				{ID: "b", Name: "search_rulings", Args: json.RawMessage(`{"query":"y"}`)},
			}},
			{Role: RoleTool, ToolCallID: "a", Name: "search_rulings", Content: `[{"id":"N1"}]`},
			{Role: RoleTool, ToolCallID: "b", Name: "search_rulings", Content: `{"hits":[]}`},
		},
		Tools:           []Tool{{Name: "read_ruling", Description: "d", Parameters: map[string]any{"type": "object"}}},
		JSONSchema:      map[string]any{"type": "object"},
		MaxOutputTokens: 500,
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(path, "projects/test-project/locations/global/publishers/google/models/gemini-4-argon:generateContent") {
		t.Errorf("path = %s", path)
	}
	gc := got["generationConfig"].(map[string]any)
	if gc["temperature"] != 0.0 || gc["responseMimeType"] != "application/json" || gc["maxOutputTokens"] != 500.0 || gc["responseJsonSchema"] == nil || gc["seed"] != float64(Seed) {
		t.Errorf("generationConfig = %v", gc)
	}
	if got["systemInstruction"] == nil || got["tools"] == nil {
		t.Errorf("missing systemInstruction or tools: %v", got)
	}
	contents := got["contents"].([]any)
	if len(contents) != 3 {
		t.Fatalf("contents = %d, want 3 (user, model, merged tool results): %v", len(contents), contents)
	}
	modelParts := contents[1].(map[string]any)["parts"].([]any)
	if modelParts[0].(map[string]any)["thoughtSignature"] != "c2ln" {
		t.Errorf("thought signature not sent back: %v", modelParts[0])
	}
	toolParts := contents[2].(map[string]any)["parts"].([]any)
	first := toolParts[0].(map[string]any)["functionResponse"].(map[string]any)["response"].(map[string]any)
	if len(toolParts) != 2 || first["result"] != `[{"id":"N1"}]` {
		t.Errorf("tool results = %v", toolParts)
	}

	if resp.Text != `{"ok":true}` || resp.ModelID != "gemini-4-argon-001" || resp.FinishReason != "STOP" {
		t.Errorf("response = %+v", resp)
	}
	if resp.Usage != (Usage{InputTokens: 200, OutputTokens: 50, TotalTokens: 250}) {
		t.Errorf("usage = %+v", resp.Usage)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "read_ruling" || string(resp.ToolCalls[0].Signature) != "sig" ||
		string(resp.ToolCalls[0].Args) != `{"ruling":"N351460"}` || resp.ToolCalls[0].ID == "" {
		t.Errorf("tool calls = %+v", resp.ToolCalls)
	}
}

func TestVertexErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		check  func(error) bool
	}{
		{"429 retryable", 429, `{"error":{"code":429,"message":"quota","status":"RESOURCE_EXHAUSTED"}}`,
			func(e error) bool {
				var a *APIError
				return errors.As(e, &a) && a.Retryable() && a.Provider == "vertex"
			}},
		{"404 model not found", 404, `{"error":{"code":404,"message":"not found","status":"NOT_FOUND"}}`,
			func(e error) bool { var a *APIError; return errors.As(e, &a) && !a.Retryable() }},
		{"blocked prompt", 200, `{"promptFeedback":{"blockReason":"SAFETY"}}`,
			func(e error) bool { return errors.Is(e, ErrBadResponse) && strings.Contains(e.Error(), "SAFETY") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := newTestVertex(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			})
			_, err := v.Generate(context.Background(), Request{Model: "gemini-4-argon", Messages: []Message{{Role: RoleUser, Content: "q"}}})
			if err == nil || !tt.check(err) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestVertexRejectsUnknownRole(t *testing.T) {
	v := newTestVertex(t, func(http.ResponseWriter, *http.Request) { t.Error("should not call the API") })
	if _, err := v.Generate(context.Background(), Request{Model: "m", Messages: []Message{{Role: "system", Content: "x"}}}); err == nil {
		t.Fatal("want error for unknown role")
	}
}

func TestVertexSendsInlineMediaParts(t *testing.T) {
	var got map[string]any
	v := newTestVertex(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		_, _ = io.WriteString(w, `{"candidates":[{"finishReason":"STOP","content":{"role":"model","parts":[{"text":"{}"}]}}]}`)
	})
	png := []byte("\x89PNG\r\n\x1a\nrest")
	if _, err := v.Generate(context.Background(), Request{Model: "m", Messages: []Message{
		{Role: RoleUser, Content: "read this", Parts: []Part{{MIMEType: "image/png", Data: png}}},
	}}); err != nil {
		t.Fatal(err)
	}
	contents := got["contents"].([]any)
	parts := contents[0].(map[string]any)["parts"].([]any)
	if len(parts) != 2 || parts[0].(map[string]any)["text"] != "read this" {
		t.Fatalf("parts = %v", parts)
	}
	inline := parts[1].(map[string]any)["inlineData"].(map[string]any)
	if inline["mimeType"] != "image/png" || inline["data"] == "" {
		t.Fatalf("inline = %v", inline)
	}
}

func TestVertexTextOnlyUserTurnUnchanged(t *testing.T) {
	plain, err := toGenaiContents([]Message{{Role: RoleUser, Content: "q"}})
	if err != nil {
		t.Fatal(err)
	}
	withNil, err := toGenaiContents([]Message{{Role: RoleUser, Content: "q", Parts: nil}})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(plain)
	b, _ := json.Marshal(withNil)
	if string(a) != string(b) || len(plain[0].Parts) != 1 || plain[0].Parts[0].Text != "q" {
		t.Fatalf("text-only turn changed: %s vs %s", a, b)
	}
}

func TestVertexRejectsMediaOutsideUserTurns(t *testing.T) {
	for _, m := range []Message{
		{Role: RoleAssistant, Parts: []Part{{MIMEType: "image/png", Data: []byte{1}}}},
		{Role: RoleTool, Name: "x", Parts: []Part{{MIMEType: "image/png", Data: []byte{1}}}},
		{Role: RoleUser, Parts: []Part{{MIMEType: "image/png"}}},
	} {
		if _, err := toGenaiContents([]Message{m}); err == nil {
			t.Errorf("%s turn with %v: want an error", m.Role, m.Parts)
		}
	}
}

func TestMessagePartsAreNeverSerialised(t *testing.T) {
	b, err := json.Marshal(Message{Role: RoleUser, Content: "q", Parts: []Part{{MIMEType: "image/png", Data: []byte("SECRETBYTES")}}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "SECRET") || strings.Contains(string(b), "U0VDUkVU") || strings.Contains(string(b), "image/png") {
		t.Fatalf("parts leaked into JSON: %s", b)
	}
}
