package verifier

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"strings"
	"sync"

	"google.golang.org/adk/model"
	"google.golang.org/genai"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// adkModel adapts llm.Model to ADK's model.LLM, so the ADK agent runs on
// whichever provider is configured (Vertex, or Fake in tests) and every call
// goes through the same retry/logging wrappers.
// It also totals token usage for cost reporting.
type adkModel struct {
	m         llm.Model
	id        string
	maxTokens int

	mu       sync.Mutex
	usage    llm.Usage
	calls    int
	provider string
	modelID  string
}

func (a *adkModel) Name() string { return a.id }

func (a *adkModel) GenerateContent(ctx context.Context, req *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		r, err := toLLMRequest(req, a.id, a.maxTokens)
		if err != nil {
			yield(nil, err)
			return
		}
		resp, err := a.m.Generate(ctx, r)
		if err != nil {
			yield(nil, err)
			return
		}
		a.mu.Lock()
		a.calls++
		a.usage.InputTokens += resp.Usage.InputTokens
		a.usage.OutputTokens += resp.Usage.OutputTokens
		a.usage.TotalTokens += resp.Usage.TotalTokens
		a.provider, a.modelID = resp.Provider, resp.ModelID
		a.mu.Unlock()
		yield(fromLLMResponse(resp), nil)
	}
}

func toLLMRequest(req *model.LLMRequest, modelID string, maxTokens int) (llm.Request, error) {
	out := llm.Request{Model: modelID, MaxOutputTokens: maxTokens}
	if cfg := req.Config; cfg != nil {
		if cfg.SystemInstruction != nil {
			var parts []string
			for _, p := range cfg.SystemInstruction.Parts {
				if p != nil && p.Text != "" {
					parts = append(parts, p.Text)
				}
			}
			out.System = strings.Join(parts, "\n\n")
		}
		for _, t := range cfg.Tools {
			if t == nil {
				continue
			}
			for _, d := range t.FunctionDeclarations {
				params := map[string]any{"type": "object"}
				if d.ParametersJsonSchema != nil {
					b, err := json.Marshal(d.ParametersJsonSchema)
					if err != nil {
						return out, fmt.Errorf("verifier: tool %s schema: %w", d.Name, err)
					}
					if err := json.Unmarshal(b, &params); err != nil {
						return out, fmt.Errorf("verifier: tool %s schema: %w", d.Name, err)
					}
				}
				out.Tools = append(out.Tools, llm.Tool{Name: d.Name, Description: d.Description, Parameters: params})
			}
		}
	}
	for _, c := range req.Contents {
		if c == nil {
			continue
		}
		if c.Role == genai.RoleModel {
			msg := llm.Message{Role: llm.RoleAssistant}
			for _, p := range c.Parts {
				switch {
				case p == nil || p.Thought:
				case p.FunctionCall != nil:
					args, err := json.Marshal(p.FunctionCall.Args)
					if err != nil {
						return out, fmt.Errorf("verifier: encode call args: %w", err)
					}
					msg.ToolCalls = append(msg.ToolCalls, llm.ToolCall{ID: p.FunctionCall.ID, Name: p.FunctionCall.Name, Args: args, Signature: p.ThoughtSignature})
				case p.Text != "":
					msg.Content += p.Text
				}
			}
			if msg.Content != "" || len(msg.ToolCalls) > 0 {
				out.Messages = append(out.Messages, msg)
			}
			continue
		}
		var text []string
		for _, p := range c.Parts {
			switch {
			case p == nil:
			case p.FunctionResponse != nil:
				b, err := json.Marshal(p.FunctionResponse.Response)
				if err != nil {
					return out, fmt.Errorf("verifier: encode tool result: %w", err)
				}
				out.Messages = append(out.Messages, llm.Message{Role: llm.RoleTool, ToolCallID: p.FunctionResponse.ID, Name: p.FunctionResponse.Name, Content: string(b)})
			case p.Text != "":
				text = append(text, p.Text)
			}
		}
		if len(text) > 0 {
			out.Messages = append(out.Messages, llm.Message{Role: llm.RoleUser, Content: strings.Join(text, "\n")})
		}
	}
	return out, nil
}

func fromLLMResponse(r llm.Response) *model.LLMResponse {
	c := &genai.Content{Role: genai.RoleModel}
	if r.Text != "" {
		c.Parts = append(c.Parts, genai.NewPartFromText(r.Text))
	}
	for _, tc := range r.ToolCalls {
		var args map[string]any
		_ = json.Unmarshal(tc.Args, &args)
		c.Parts = append(c.Parts, &genai.Part{
			FunctionCall:     &genai.FunctionCall{ID: tc.ID, Name: tc.Name, Args: args},
			ThoughtSignature: tc.Signature,
		})
	}
	if len(c.Parts) == 0 {
		c.Parts = append(c.Parts, genai.NewPartFromText(""))
	}
	return &model.LLMResponse{
		Content:      c,
		ModelVersion: r.ModelID,
		TurnComplete: true,
		FinishReason: genai.FinishReasonStop,
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     int32(r.Usage.InputTokens),
			CandidatesTokenCount: int32(r.Usage.OutputTokens),
			TotalTokenCount:      int32(r.Usage.TotalTokens),
		},
	}
}

var _ model.LLM = (*adkModel)(nil)

// ADKModel is the ADK adapter over llm.Model, for other agents (case agent).
type ADKModel = adkModel

// NewADKModel adapts m for an ADK agent; maxTokens caps each response.
func NewADKModel(m llm.Model, modelID string, maxTokens int) *ADKModel {
	return &adkModel{m: m, id: modelID, maxTokens: maxTokens}
}

// Stats returns the model calls made and the tokens used so far.
func (a *adkModel) Stats() (calls int, usage llm.Usage, provider, modelID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls, a.usage, a.provider, a.modelID
}
