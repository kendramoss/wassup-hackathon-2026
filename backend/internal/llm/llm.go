package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Model generates one model turn.
type Model interface {
	Generate(ctx context.Context, req Request) (Response, error)
	// Provider is the provider name recorded in every output ("vertex",
	// "fake"; runs recorded before D39 say "litellm").
	Provider() string
}

// Role of a message in a conversation.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is one conversation turn. Assistant turns may carry ToolCalls;
// tool turns answer one call (ToolCallID, Name) with Content.
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// ToolCall is a function call requested by the model. Signature is opaque
// provider data (Gemini thought signatures) that must be sent back unchanged
// with the call on the next turn.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Args      json.RawMessage `json:"args"`
	Signature []byte          `json:"signature,omitempty"`
}

// Tool declares a function the model may call. Parameters is a JSON schema.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// Request is one model call. Temperature is always 0: implementations must
// not expose a way to change it.
type Request struct {
	Model    string
	System   string
	Messages []Message
	Tools    []Tool
	// JSONSchema, when set, constrains the answer to JSON matching it.
	// SchemaName names it for providers that require a name.
	JSONSchema      map[string]any
	SchemaName      string
	MaxOutputTokens int
}

// Usage is token accounting for one call.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// Response is one model turn.
type Response struct {
	Text      string
	ToolCalls []ToolCall
	Usage     Usage
	// ModelID is the model that answered, as reported by the provider when
	// available, else the requested model.
	ModelID      string
	Provider     string
	Latency      time.Duration
	FinishReason string
}

// DecodeJSON unmarshals the response text into v. Models occasionally wrap
// JSON in prose or code fences, so the outermost {...} is used.
func (r Response) DecodeJSON(v any) error {
	s := r.Text
	start, end := -1, -1
	for i := 0; i < len(s); i++ {
		if s[i] == '{' {
			start = i
			break
		}
	}
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '}' {
			end = i
			break
		}
	}
	if start < 0 || end < start {
		return fmt.Errorf("llm: %w: no JSON object in response (%d chars)", ErrBadResponse, len(s))
	}
	if err := json.Unmarshal([]byte(s[start:end+1]), v); err != nil {
		return fmt.Errorf("llm: %w: %v", ErrBadResponse, err)
	}
	return nil
}

// Sentinel errors for callers and the HTTP layer.
var (
	// ErrBadResponse means the provider answered but the answer was unusable.
	ErrBadResponse = errors.New("unusable model response")
	// ErrUnavailable means the provider could not be reached or kept failing
	// with retryable errors (429, 5xx, timeouts).
	ErrUnavailable = errors.New("model provider unavailable")
)

// APIError is a non-2xx answer from a provider. Body is truncated and never
// contains request headers.
type APIError struct {
	Provider   string
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s: HTTP %d: %s", e.Provider, e.StatusCode, e.Body)
}

// Retryable reports whether the call may succeed if repeated.
func (e *APIError) Retryable() bool {
	return e.StatusCode == 429 || e.StatusCode >= 500
}
