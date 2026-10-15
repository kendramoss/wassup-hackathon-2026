// Package caseagent answers questions about one case with an ADK agent that
// can read the case, search and read earlier CBP rulings, and re-run the
// origin engine on changed facts ("what if").
package caseagent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	"google.golang.org/genai"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/verifier"
)

// PromptVersion is recorded with every answer.
const PromptVersion = "case-agent-v1"

// Limits per question.
const (
	MaxToolCalls = 6
	MaxWhatIfs   = 2
	MaxHistory   = 6 // earlier Q&A turns given as context
)

// System is the agent's instruction.
const System = `You answer questions from a trade-compliance lead about ONE product's likely U.S. country of origin under CBP's substantial-transformation test. Origin Proof has already predicted the origin from the facts and earlier CBP rulings.

Tools:
- get_case: the facts, the prediction (origin, confidence, decisive step, reasoning, earlier rulings used), the review route, any plans and scenarios. Call it first.
- search_rulings / read_ruling: earlier CBP rulings (dated before the case). Use them to support or test an answer.
- what_if: re-run the origin prediction on revised facts. Use it when the user asks what would happen if something changed; pass the full revised facts, changing only what the user asked about.

Rules:
- Answer only about this case and CBP origin practice. Ground every claim in the case or a ruling you read; cite ruling numbers like N351460.
- Never invent a ruling number or a fact. If the facts don't say, say so.
- Precedent support is not a ruling: for anything the user will rely on, say to confirm with a licensed customs broker or a CBP binding ruling.
- Plain language, at most 150 words, no headings.`

// Predictor is the origin engine.
type Predictor interface {
	Predict(ctx context.Context, c domain.Case) (engine.Result, error)
}

// Turn is one earlier question and answer.
type Turn struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// Input is one question with what the agent may read.
type Input struct {
	Question string
	History  []Turn
	// CaseSummary is the text get_case returns.
	CaseSummary string
	// Facts, Date and Exclude bound what_if and the ruling tools.
	Facts   string
	Date    time.Time
	Exclude []string
}

// WhatIf is one re-run of the engine on revised facts.
type WhatIf struct {
	Change     string  `json:"change"`
	Origin     string  `json:"origin"`
	Confidence float64 `json:"confidence"`
	Decisive   string  `json:"decisive_step"`
}

// Step is one tool call.
type Step struct {
	Tool string `json:"tool"`
	Arg  string `json:"arg,omitempty"`
}

// Answer is the agent's reply.
type Answer struct {
	Answer string `json:"answer"`
	// Cited are ruling numbers in the answer that the agent saw in a tool
	// result or the case; others are listed in Dropped.
	Cited      []string     `json:"cited_rulings"`
	Dropped    []string     `json:"dropped_rulings,omitempty"`
	WhatIfs    []WhatIf     `json:"what_ifs"`
	Steps      []Step       `json:"steps"`
	ModelCalls int          `json:"model_calls"`
	Trace      domain.Trace `json:"trace"`
}

// Agent answers case questions.
type Agent struct {
	model  llm.Model
	id     string
	search verifier.Searcher
	engine Predictor
}

// New returns a case agent.
func New(m llm.Model, modelID string, s verifier.Searcher, e Predictor) *Agent {
	return &Agent{model: m, id: modelID, search: s, engine: e}
}

type searchArgs struct {
	Query string `json:"query" jsonschema:"product type, key component, operation, countries"`
}

type readArgs struct {
	Ruling string `json:"ruling" jsonschema:"e.g. N351460"`
}

type whatIfArgs struct {
	Change string `json:"change" jsonschema:"one line: what changed, e.g. motor made in Thailand"`
	Facts  string `json:"facts" jsonschema:"the full revised facts"`
}

type noArgs struct{}

type out struct {
	Result string `json:"result"`
}

// Ask answers one question. A fresh agent and session are made per call.
func (a *Agent) Ask(ctx context.Context, in Input) (Answer, error) {
	if strings.TrimSpace(in.Question) == "" {
		return Answer{}, fmt.Errorf("caseagent: %w: question is required", domain.ErrInvalid)
	}
	exclude := map[string]bool{}
	for _, id := range in.Exclude {
		exclude[id] = true
	}
	var mu sync.Mutex
	ans := Answer{WhatIfs: []WhatIf{}, Steps: []Step{}}
	seen := map[string]bool{}
	note := func(s Step, text string) {
		mu.Lock()
		defer mu.Unlock()
		ans.Steps = append(ans.Steps, s)
		for _, id := range verifier.RulingRef.FindAllString(text, -1) {
			seen[id] = true
		}
	}
	caseTool, err := functiontool.New(functiontool.Config{Name: "get_case", Description: "The case: facts, prediction, review route, plans and scenarios."},
		func(agent.ToolContext, noArgs) (out, error) {
			note(Step{Tool: "get_case"}, in.CaseSummary)
			return out{in.CaseSummary}, nil
		})
	if err != nil {
		return Answer{}, fmt.Errorf("caseagent: %w", err)
	}
	searchTool, err := functiontool.New(functiontool.Config{Name: "search_rulings",
		Description: "Search earlier CBP rulings (dated before the case). Returns ruling number, date, subject and the origin CBP held."},
		func(_ agent.ToolContext, x searchArgs) (out, error) {
			hits, err := a.search.Search(ctx, x.Query, in.Date, exclude)
			if err != nil {
				return out{}, err
			}
			text := verifier.FormatHits(hits)
			note(Step{"search_rulings", x.Query}, text)
			return out{text}, nil
		})
	if err != nil {
		return Answer{}, fmt.Errorf("caseagent: %w", err)
	}
	readTool, err := functiontool.New(functiontool.Config{Name: "read_ruling", Description: "Read the facts and CBP's analysis and holding of an earlier ruling."},
		func(_ agent.ToolContext, x readArgs) (out, error) {
			text, err := a.search.Read(ctx, strings.TrimSpace(x.Ruling), in.Date, exclude)
			if err != nil {
				return out{}, err
			}
			note(Step{"read_ruling", x.Ruling}, text)
			return out{text}, nil
		})
	if err != nil {
		return Answer{}, fmt.Errorf("caseagent: %w", err)
	}
	whatIfTool, err := functiontool.New(functiontool.Config{Name: "what_if",
		Description: "Re-run the origin prediction on revised facts. Returns origin, confidence and the decisive step."},
		func(_ agent.ToolContext, x whatIfArgs) (out, error) {
			mu.Lock()
			n := len(ans.WhatIfs)
			mu.Unlock()
			if n >= MaxWhatIfs {
				return out{"what-if budget used; answer with what you have"}, nil
			}
			facts := strings.TrimSpace(x.Facts)
			if facts == "" {
				return out{"facts are required"}, nil
			}
			res, err := a.engine.Predict(ctx, domain.Case{ID: "what-if", Date: in.Date, Facts: corpus.Truncate(facts, domain.MaxFactsRunes) + "\n", Exclude: in.Exclude})
			if err != nil {
				return out{}, err
			}
			p := res.Prediction
			w := WhatIf{Change: x.Change, Origin: p.Origin, Confidence: p.Confidence, Decisive: p.DecisiveStep}
			mu.Lock()
			ans.WhatIfs = append(ans.WhatIfs, w)
			mu.Unlock()
			text := fmt.Sprintf("origin %s, confidence %.2f, decisive step: %s. Reasoning: %s. Earlier rulings used: %s",
				p.Origin, p.Confidence, p.DecisiveStep, p.Reasoning, strings.Join(p.Precedents, ", "))
			note(Step{"what_if", x.Change}, text)
			return out{text}, nil
		})
	if err != nil {
		return Answer{}, fmt.Errorf("caseagent: %w", err)
	}

	m := verifier.NewADKModel(a.model, a.id, 2000)
	var toolCalls, modelCalls int
	ag, err := llmagent.New(llmagent.Config{
		Name:                  "case_agent",
		Description:           "Answers questions about one origin case.",
		Model:                 m,
		InstructionProvider:   func(agent.ReadonlyContext) (string, error) { return System, nil },
		GenerateContentConfig: &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](0)},
		Tools:                 []tool.Tool{caseTool, searchTool, readTool, whatIfTool},
		BeforeToolCallbacks: []llmagent.BeforeToolCallback{
			func(agent.ToolContext, tool.Tool, map[string]any) (map[string]any, error) {
				mu.Lock()
				defer mu.Unlock()
				toolCalls++
				if toolCalls > MaxToolCalls {
					return map[string]any{"result": "Tool budget used. Answer now."}, nil
				}
				return nil, nil
			},
		},
		BeforeModelCallbacks: []llmagent.BeforeModelCallback{
			func(_ agent.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
				mu.Lock()
				defer mu.Unlock()
				modelCalls++
				if toolCalls >= MaxToolCalls && req.Config != nil {
					req.Config.Tools = nil
				}
				if modelCalls > MaxToolCalls+3 {
					return nil, fmt.Errorf("caseagent: no answer after %d model calls", modelCalls-1)
				}
				return nil, nil
			},
		},
	})
	if err != nil {
		return Answer{}, fmt.Errorf("caseagent: %w", err)
	}
	r, err := runner.New(runner.Config{AppName: "origin-proof-case-agent", Agent: ag, SessionService: session.InMemoryService(), AutoCreateSession: true})
	if err != nil {
		return Answer{}, fmt.Errorf("caseagent: %w", err)
	}
	var final string
	for ev, err := range r.Run(ctx, "user", uuid.NewString(), genai.NewContentFromText(userMessage(in), genai.RoleUser), agent.RunConfig{}) {
		if err != nil {
			return Answer{}, fmt.Errorf("caseagent: run: %w", err)
		}
		if ev.Content == nil || ev.Partial {
			continue
		}
		var text strings.Builder
		call := false
		for _, p := range ev.Content.Parts {
			if p.FunctionCall != nil {
				call = true
			}
			if p.Text != "" && !p.Thought {
				text.WriteString(p.Text)
			}
		}
		if !call && text.Len() > 0 {
			final = text.String()
		}
	}
	if strings.TrimSpace(final) == "" {
		return Answer{}, fmt.Errorf("caseagent: %w: empty answer", llm.ErrBadResponse)
	}
	ans.Answer = strings.TrimSpace(final)
	for _, id := range verifier.RulingRef.FindAllString(in.CaseSummary, -1) {
		seen[id] = true
	}
	ans.Cited = []string{}
	done := map[string]bool{}
	for _, id := range verifier.RulingRef.FindAllString(ans.Answer, -1) {
		if done[id] {
			continue
		}
		done[id] = true
		if seen[id] {
			ans.Cited = append(ans.Cited, id)
		} else {
			ans.Dropped = append(ans.Dropped, id)
		}
	}
	calls, usage, provider, modelID := m.Stats()
	ans.ModelCalls = calls
	ans.Trace = domain.Trace{Provider: provider, ModelID: modelID, PromptVersion: PromptVersion, InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens}
	return ans, nil
}

func userMessage(in Input) string {
	var b strings.Builder
	h := in.History
	if len(h) > MaxHistory {
		h = h[len(h)-MaxHistory:]
	}
	if len(h) > 0 {
		b.WriteString("EARLIER IN THIS CONVERSATION:\n")
		for _, t := range h {
			fmt.Fprintf(&b, "Q: %s\nA: %s\n", t.Question, t.Answer)
		}
		b.WriteString("\n")
	}
	b.WriteString("QUESTION: " + strings.TrimSpace(in.Question))
	return b.String()
}
