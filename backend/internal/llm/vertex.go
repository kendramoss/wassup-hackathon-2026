package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"cloud.google.com/go/auth"
	"google.golang.org/genai"
)

// Vertex calls Gemini on Vertex AI through google.golang.org/genai with
// Application Default Credentials. It is the only provider allowed in
// deployment.
type Vertex struct {
	client *genai.Client
}

// VertexOptions configures NewVertex. BaseURL and Credentials are for tests;
// leave them empty to use the real endpoint and ADC.
type VertexOptions struct {
	Project     string
	Location    string
	Timeout     time.Duration
	BaseURL     string
	Credentials *auth.Credentials
	HTTPClient  *http.Client
}

// NewVertex creates a Vertex AI client.
func NewVertex(ctx context.Context, o VertexOptions) (*Vertex, error) {
	cfg := &genai.ClientConfig{
		Backend:     genai.BackendVertexAI,
		Project:     o.Project,
		Location:    o.Location,
		Credentials: o.Credentials,
		HTTPClient:  o.HTTPClient,
		HTTPOptions: genai.HTTPOptions{BaseURL: o.BaseURL},
	}
	if o.Timeout > 0 {
		cfg.HTTPOptions.Timeout = &o.Timeout
	}
	c, err := genai.NewClient(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("vertex: new client (project %q, location %q): %w", o.Project, o.Location, err)
	}
	return &Vertex{client: c}, nil
}

// Provider implements Model.
func (v *Vertex) Provider() string { return "vertex" }

// Generate implements Model.
func (v *Vertex) Generate(ctx context.Context, req Request) (Response, error) {
	contents, err := toGenaiContents(req.Messages)
	if err != nil {
		return Response{}, fmt.Errorf("vertex: %w", err)
	}
	start := time.Now()
	out, err := v.client.Models.GenerateContent(ctx, req.Model, contents, toGenaiConfig(req))
	if err != nil {
		return Response{}, mapVertexError(ctx, err)
	}
	return fromGenaiResponse(out, req.Model, time.Since(start))
}

func toGenaiConfig(req Request) *genai.GenerateContentConfig {
	zero := float32(0)
	cfg := &genai.GenerateContentConfig{Temperature: &zero, MaxOutputTokens: int32(req.MaxOutputTokens)}
	if req.System != "" {
		cfg.SystemInstruction = genai.NewContentFromText(req.System, genai.RoleUser)
	}
	if len(req.Tools) > 0 {
		decls := make([]*genai.FunctionDeclaration, len(req.Tools))
		for i, t := range req.Tools {
			decls[i] = &genai.FunctionDeclaration{Name: t.Name, Description: t.Description, ParametersJsonSchema: t.Parameters}
		}
		cfg.Tools = []*genai.Tool{{FunctionDeclarations: decls}}
	}
	if req.JSONSchema != nil {
		cfg.ResponseMIMEType = "application/json"
		cfg.ResponseJsonSchema = req.JSONSchema
	}
	return cfg
}

func toGenaiContents(msgs []Message) ([]*genai.Content, error) {
	var out []*genai.Content
	for i, m := range msgs {
		switch m.Role {
		case RoleUser:
			out = append(out, genai.NewContentFromText(m.Content, genai.RoleUser))
		case RoleAssistant:
			c := &genai.Content{Role: genai.RoleModel}
			if m.Content != "" {
				c.Parts = append(c.Parts, genai.NewPartFromText(m.Content))
			}
			for _, tc := range m.ToolCalls {
				var args map[string]any
				if len(tc.Args) > 0 {
					if err := json.Unmarshal(tc.Args, &args); err != nil {
						return nil, fmt.Errorf("message %d: tool call %s args: %w", i, tc.Name, err)
					}
				}
				c.Parts = append(c.Parts, &genai.Part{
					FunctionCall:     &genai.FunctionCall{ID: tc.ID, Name: tc.Name, Args: args},
					ThoughtSignature: tc.Signature,
				})
			}
			out = append(out, c)
		case RoleTool:
			part := &genai.Part{FunctionResponse: &genai.FunctionResponse{
				ID: m.ToolCallID, Name: m.Name, Response: toolResult(m.Content),
			}}
			// Consecutive tool results answer one model turn: keep them in one content.
			if n := len(out); n > 0 && out[n-1].Role == genai.RoleUser && isFunctionResponses(out[n-1]) {
				out[n-1].Parts = append(out[n-1].Parts, part)
			} else {
				out = append(out, &genai.Content{Role: genai.RoleUser, Parts: []*genai.Part{part}})
			}
		default:
			return nil, fmt.Errorf("message %d: unknown role %q", i, m.Role)
		}
	}
	return out, nil
}

// toolResult wraps a tool's output as the object Gemini expects: a JSON
// object is passed through, anything else goes under "result".
func toolResult(content string) map[string]any {
	var obj map[string]any
	if json.Unmarshal([]byte(content), &obj) == nil && obj != nil {
		return obj
	}
	return map[string]any{"result": content}
}

func isFunctionResponses(c *genai.Content) bool {
	for _, p := range c.Parts {
		if p.FunctionResponse == nil {
			return false
		}
	}
	return len(c.Parts) > 0
}

func fromGenaiResponse(out *genai.GenerateContentResponse, requested string, latency time.Duration) (Response, error) {
	resp := Response{Provider: "vertex", ModelID: out.ModelVersion, Latency: latency}
	if resp.ModelID == "" {
		resp.ModelID = requested
	}
	if u := out.UsageMetadata; u != nil {
		resp.Usage = Usage{
			InputTokens: int(u.PromptTokenCount),
			// Thinking tokens are billed as output.
			OutputTokens: int(u.CandidatesTokenCount + u.ThoughtsTokenCount),
			TotalTokens:  int(u.TotalTokenCount),
		}
	}
	if len(out.Candidates) == 0 || out.Candidates[0].Content == nil {
		reason := "no candidates"
		if out.PromptFeedback != nil && out.PromptFeedback.BlockReason != "" {
			reason = "prompt blocked: " + string(out.PromptFeedback.BlockReason)
		}
		return Response{}, fmt.Errorf("vertex: %w: %s", ErrBadResponse, reason)
	}
	cand := out.Candidates[0]
	resp.FinishReason = string(cand.FinishReason)
	for i, p := range cand.Content.Parts {
		switch {
		case p.Thought:
			// Thought summaries are not part of the answer.
		case p.FunctionCall != nil:
			args, err := json.Marshal(p.FunctionCall.Args)
			if err != nil {
				return Response{}, fmt.Errorf("vertex: %w: encode args of %s: %v", ErrBadResponse, p.FunctionCall.Name, err)
			}
			id := p.FunctionCall.ID
			if id == "" {
				id = fmt.Sprintf("call_%d", i)
			}
			resp.ToolCalls = append(resp.ToolCalls, ToolCall{ID: id, Name: p.FunctionCall.Name, Args: args, Signature: p.ThoughtSignature})
		case p.Text != "":
			resp.Text += p.Text
		}
	}
	return resp, nil
}

func mapVertexError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("vertex: %w", ctx.Err())
	}
	var apiErr genai.APIError
	if errors.As(err, &apiErr) {
		return &APIError{Provider: "vertex", StatusCode: apiErr.Code, Body: truncate(apiErr.Message, 300)}
	}
	return fmt.Errorf("vertex: %w: %v", ErrUnavailable, err)
}

var _ Model = (*Vertex)(nil)
