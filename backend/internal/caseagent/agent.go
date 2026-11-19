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

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/verifier"
)

// PromptVersion is recorded with every answer.
const PromptVersion = "case-agent-v3" // v3: regime context (not every case is substantial transformation); what_if reports the applied regime, review route and provisional state (#319)

// Limits per question.
const (
	MaxToolCalls = 6
	MaxWhatIfs   = 2
	MaxHistory   = 6 // earlier Q&A turns given as context
)

// System is the agent's instruction.
const System = `You answer questions from a trade-compliance lead about ONE product's likely U.S. country of origin. Unde has already predicted the origin from the facts and earlier CBP rulings.

Which legal test a prediction used depends on the regime the case was routed to. Most goods use CBP's substantial-transformation test. Textile and apparel goods use the textile rules of origin (19 CFR 102.21). USMCA preference or Canada/Mexico marking questions use the Part 102 tariff-shift rules (19 CFR 102.11). Pharmaceutical active ingredients use the pharma API engine. Do not assume substantial transformation applies: say which regime a result used, and do not describe the Part 102 or textile result as a substantial-transformation finding.

Tools:
- get_case: the facts, the prediction (origin, confidence, decisive step, reasoning, earlier rulings used), the review route, any plans and scenarios. Call it first. It may not name the regime; if it does not, do not guess one.
- search_rulings / read_ruling: earlier CBP rulings (dated before the case). Use them to support or test an answer.
- what_if: re-run the origin prediction on revised facts. Use it when the user asks what would happen if something changed; pass the full revised facts, changing only what the user asked about. Its result states the regime that answered (or "unknown"), the review route and whether the result is provisional.

Rules:
- Answer only about this case and CBP origin practice. Ground every claim in the case or a ruling you read; cite ruling numbers like N351460.
- Never invent a ruling number or a fact. If the facts don't say, say so.
- If a what_if result says PROVISIONAL, the regime is unknown, or the route is needs_review, say the legal regime was not confirmed and an expert should review; do not present that origin as settled.
- Precedent support is not a ruling: for anything the user will rely on, say to confirm with a licensed customs broker or a CBP binding ruling.
- Plain language, at most 150 words, no headings.`

// Predictor is the origin engine. A Predictor that is also a Classifier is
// used through Classify; a bare Predictor's what-if result is reported as
// unclassified (regime unknown, no review route), never as regime-correct.
type Predictor interface {
	Predict(ctx context.Context, c domain.Case) (engine.Result, error)
}

// Classification is one dispatched, triaged run of the origin engine, as the
// API would produce it for a saved scenario. The values are copied from the
// dispatch seam by an adapter (this package cannot import httpapi).
type Classification struct {
	Result engine.Result
	// Regime is the regime whose engine actually produced Result, from the
	// prediction's own trace. "" is unknown: it is never replaced by the
	// router label or by general.
	Regime string
	// Selected is the regime the router chose ("" for general or none).
	Selected string
	// Scope is no_router, configured_failed, confirmed or label_missing.
	Scope string
	// Specialist is "", ok, failed or unconfigured.
	Specialist string
	Route      domain.Route
	Reason     string
}

// Classifier runs the same dispatch and triage as a saved scenario.
type Classifier interface {
	Classify(ctx context.Context, c domain.Case) (Classification, error)
}

// Provisional says why a result must not be presented as regime-confirmed,
// or "" when it can be.
func (c Classification) Provisional() string {
	switch {
	case c.Specialist == "failed":
		return "the " + c.Selected + " engine failed, so this is a provisional general prediction"
	case c.Specialist == "unconfigured":
		return "the " + c.Selected + " engine is not enabled, so this is a general prediction"
	case c.Scope == "configured_failed":
		return "the scope router failed, so the legal regime is not confirmed"
	case c.Scope == "label_missing":
		return "the legal regime is not confirmed"
	case c.Regime == "":
		return "the regime that produced this prediction is unknown"
	}
	return ""
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

// WhatIf is one re-run of the engine on revised facts. Facts is that
// revision, so it can be saved with the turn and reused as a scenario (#115).
type WhatIf struct {
	Change     string  `json:"change"`
	Origin     string  `json:"origin"`
	Confidence float64 `json:"confidence"`
	Decisive   string  `json:"decisive_step"`
	Facts      string  `json:"facts,omitempty"`
	// Regime is the regime that answered ("unknown" when the trace does not
	// say; "unclassified" when the predictor had no dispatch). Route and
	// Reason are the review triage; Provisional is set, with its reason,
	// when the result is not regime-confirmed. Empty on an unclassified run.
	Regime      string       `json:"regime,omitempty"`
	Specialist  string       `json:"specialist,omitempty"`
	Scope       string       `json:"scope,omitempty"`
	Route       domain.Route `json:"route,omitempty"`
	Reason      string       `json:"triage_reason,omitempty"`
	Provisional string       `json:"provisional,omitempty"`
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

type factsArgs struct {
	// Offset is required in the tool schema (no omitempty); 0 means the start.
	Offset int `json:"offset" jsonschema:"rune offset to start at; 0 to the total"`
	Limit  int `json:"limit,omitempty" jsonschema:"runes to return; default and maximum 4000"`
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
	factsTool, err := functiontool.New(functiontool.Config{Name: "get_case_facts",
		Description: "Read this case's facts by rune offset and limit (at most 4000 runes per call). get_case shows only the first 6000 runes; use this for the rest."},
		func(_ agent.ToolContext, x factsArgs) (out, error) {
			text := factsPage(in.Facts, x.Offset, x.Limit)
			note(Step{"get_case_facts", fmt.Sprintf("offset=%d limit=%d", x.Offset, x.Limit)}, text)
			return out{text}, nil
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
		Description: "Re-run the origin prediction on revised facts. Returns origin, confidence, the decisive step, the regime that answered, the review route and whether the result is provisional."},
		func(_ agent.ToolContext, x whatIfArgs) (out, error) {
			mu.Lock()
			n := len(ans.WhatIfs)
			mu.Unlock()
			if n >= MaxWhatIfs {
				return out{"what-if budget used; answer with what you have"}, nil
			}
			facts, runes, ok := domain.CanonicalFacts(x.Facts)
			if !ok {
				if runes == 0 {
					return out{"facts are required"}, nil
				}
				// Never predict a prefix: the card, the saved turn and
				// Add-as-scenario must all hold what was predicted.
				msg := fmt.Sprintf("these revised facts are %d characters; the limit is %d. Nothing was predicted. Shorten the full revised facts to at most %d characters (keep the facts that decide origin) and call what_if again.", runes, domain.MaxFactsRunes, domain.MaxFactsRunes)
				note(Step{"what_if", x.Change}, msg)
				return out{msg}, nil
			}
			wc := domain.Case{ID: "what-if", Date: in.Date, Facts: facts, Exclude: in.Exclude}
			w, text, err := a.runWhatIf(ctx, wc, x.Change)
			if err != nil {
				return out{}, err
			}
			mu.Lock()
			ans.WhatIfs = append(ans.WhatIfs, w)
			mu.Unlock()
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
		Tools:                 []tool.Tool{caseTool, factsTool, searchTool, readTool, whatIfTool},
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
	r, err := runner.New(runner.Config{AppName: "unde-case-agent", Agent: ag, SessionService: session.InMemoryService(), AutoCreateSession: true})
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

// runWhatIf runs the revised facts through the same dispatch and triage as a
// saved scenario when the predictor can classify, and says so when it cannot.
func (a *Agent) runWhatIf(ctx context.Context, c domain.Case, change string) (WhatIf, string, error) {
	if cl, ok := a.engine.(Classifier); ok {
		k, err := cl.Classify(ctx, c)
		if err != nil {
			return WhatIf{}, "", err
		}
		p := k.Result.Prediction
		w := WhatIf{Change: change, Origin: p.Origin, Confidence: p.Confidence, Decisive: p.DecisiveStep, Facts: c.Facts,
			Regime: k.Regime, Specialist: k.Specialist, Scope: k.Scope, Route: k.Route, Reason: k.Reason, Provisional: k.Provisional()}
		if w.Regime == "" {
			w.Regime = "unknown"
		}
		if w.Provisional != "" && w.Route == domain.RouteAuto {
			// An unconfirmed regime is never auto; policy stays in the
			// seam, this only stops the tool from showing auto next to it.
			w.Route, w.Reason = domain.RouteNeedsReview, "regime not confirmed ("+w.Provisional+"); seam said: "+w.Reason
			k.Route, k.Reason = w.Route, w.Reason
		}
		text := fmt.Sprintf("origin %s, confidence %.2f, decisive step: %s. Reasoning: %s. Earlier rulings used: %s. Regime that answered: %s. Review route: %s (%s).",
			p.Origin, p.Confidence, p.DecisiveStep, p.Reasoning, strings.Join(p.Precedents, ", "), w.Regime, k.Route, k.Reason)
		if w.Provisional != "" {
			text += " PROVISIONAL: " + w.Provisional + "; an expert reviews. Do not present this origin as settled."
		}
		return w, text, nil
	}
	if a.engine == nil {
		return WhatIf{}, "", fmt.Errorf("caseagent: no predictor")
	}
	res, err := a.engine.Predict(ctx, c)
	if err != nil {
		return WhatIf{}, "", err
	}
	p := res.Prediction
	w := WhatIf{Change: change, Origin: p.Origin, Confidence: p.Confidence, Decisive: p.DecisiveStep, Facts: c.Facts, Regime: "unclassified",
		Provisional: "this run had no regime dispatch or review route"}
	text := fmt.Sprintf("origin %s, confidence %.2f, decisive step: %s. Reasoning: %s. Earlier rulings used: %s. Regime that answered: unclassified. PROVISIONAL: %s; an expert reviews. Do not present this origin as settled or regime-correct.",
		p.Origin, p.Confidence, p.DecisiveStep, p.Reasoning, strings.Join(p.Precedents, ", "), w.Provisional)
	return w, text, nil
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
