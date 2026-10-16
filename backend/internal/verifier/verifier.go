package verifier

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
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// PromptVersion is recorded in every verdict.
const PromptVersion = "verifier-v1"

// Principles is PRINCIPLES from
// hackathon-research/experiments/origin-proof/agent_search.py, verbatim.
const Principles = `How CBP actually decides: it compares the product to its OWN earlier rulings on similar products and processes. Your job is to find those rulings and reason from them.

Method (follow in order):
1. SEARCH for earlier CBP rulings on the same product type and the same kind of processing split (use 2-4 searches with different wording: product name, key component, key operation, countries).
2. READ the 2-3 closest rulings. For each, note: what was made where, and what CBP held and why.
3. ANCHOR: pick the single closest ruling. List the material differences between it and this case.
4. DECIDE: follow the anchor's outcome unless a difference is one that CBP itself treated as decisive in another ruling you read. If you depart, cite that ruling.
5. General doctrine (name/character/use; essential character; simple vs complex assembly) is only a tie-breaker when no close ruling exists. Say so if that happens, and lower your confidence.
`

// FlipFinal is FLIP_FINAL from agent_search.py, verbatim, including the
// burden-of-proof sentence. Disclosure: that sentence was written after the
// research team saw the held-out 2026 flip failures, so those items cannot
// test this verifier; its results are development-only.
const FlipFinal = `Final answer schema: {"origin_a": "<country>", "origin_b": "<country>", "changes_origin": true|false, "supporting_ruling": "<ruling where CBP treated this kind of change as decisive, or null>", "counter_ruling": "<ruling where a similar change did NOT alter origin, or null>", "confidence": 0.0-1.0, "reasoning": "<=100 words>"}
Burden of proof: answer changes_origin=true ONLY if you found an earlier ruling where CBP held that the same kind of change (the same component/operation moving) was decisive. Otherwise answer false. Many moves (final assembly, a few circuit boards out of several, a motor in an appliance whose other work stays, forging vs machining) were held NOT to change origin.`

// System renders the agent instruction for a tool-call budget, as
// run_agent() in agent_search.py does.
func System(maxToolCalls int) string {
	return Principles + fmt.Sprintf("\nUse the tools (at most %d calls). When done, reply with ONLY the final JSON (no tool call).\n", maxToolCalls) + FlipFinal
}

// Task is one move to verify: two variants of the same product.
type Task struct {
	FactsA, FactsB string
	// Target is the origin the move aims for; a supported verdict requires
	// the agent's origin_b to match it. Empty means "any change".
	Target string
	// Before bounds the evidence: only rulings dated strictly earlier.
	Before time.Time
	// Exclude lists rulings the agent may not see (the case's own rulings).
	Exclude []string
	// KnownOriginA, when set, is CBP's holding for variant A (e.g. from the
	// user's own ruling). It is stated in the task, as in the research flip
	// setting "A's ruling known"; prompt version becomes verifier-v1+known-a.
	KnownOriginA string
	KnownRulingA string
}

// UserMessage renders the task as flip_case() in agent_search.py does.
func (t Task) UserMessage() string {
	msg := "Two variants of the same product differ in where some component is made or some operation is performed. " +
		"Determine CBP's origin for each and whether the change alters origin.\n\n" +
		"VARIANT A FACTS:\n" + corpus.Truncate(t.FactsA, 7000) + "\n\nVARIANT B FACTS:\n" + corpus.Truncate(t.FactsB, 7000)
	if t.KnownOriginA != "" {
		msg += "\n\nCBP has already ruled on variant A: origin " + t.KnownOriginA
		if t.KnownRulingA != "" {
			msg += " (" + t.KnownRulingA + "; that ruling is not available to you)"
		}
		msg += "."
	}
	return msg
}

// promptVersion is the version recorded for a task.
func (t Task) promptVersion() string {
	if t.KnownOriginA != "" {
		return PromptVersion + "+known-a"
	}
	return PromptVersion
}

// Step is one tool call the agent made.
type Step struct {
	Tool string `json:"tool"`
	Arg  string `json:"arg"`
}

// Result is a verdict plus the agent's full answer and trace.
type Result struct {
	Verdict       domain.Verdict
	OriginA       string
	OriginB       string
	ChangesOrigin bool
	Steps         []Step
	ModelCalls    int
	Usage         llm.Usage
	// Dropped lists rulings the agent named that it never saw in a tool
	// result; they are removed from the verdict.
	Dropped []string
	// Cached is true when the verdict came from Cache, not a fresh run.
	Cached bool
	// ToolErrors counts search/read calls that failed (e.g. CROSS down). A
	// verdict reached without that evidence is not cached (D58).
	ToolErrors int
}

// Options configures the verifier.
type Options struct {
	Model           string
	MaxToolCalls    int
	MaxOutputTokens int
}

// Verifier runs the precedent-first agent.
type Verifier struct {
	model  llm.Model
	search Searcher
	opts   Options
}

// New returns a verifier.
func New(m llm.Model, s Searcher, opts Options) *Verifier {
	return &Verifier{model: m, search: s, opts: opts}
}

type searchArgs struct {
	Query string `json:"query" jsonschema:"product type, key component, operation, countries"`
}

type readArgs struct {
	Ruling string `json:"ruling" jsonschema:"e.g. N351460 or H337371"`
}

type toolOut struct {
	Result string `json:"result"`
}

type answer struct {
	OriginA          string   `json:"origin_a"`
	OriginB          string   `json:"origin_b"`
	ChangesOrigin    bool     `json:"changes_origin"`
	SupportingRuling *string  `json:"supporting_ruling"`
	CounterRuling    *string  `json:"counter_ruling"`
	Confidence       *float64 `json:"confidence"`
	Reasoning        string   `json:"reasoning"`
}

// Verify runs the agent on one task. A fresh ADK agent and in-memory session
// are created per call, so tool budgets and evidence never leak between
// tasks.
func (v *Verifier) Verify(ctx context.Context, t Task) (Result, error) {
	exclude := map[string]bool{}
	for _, id := range t.Exclude {
		exclude[id] = true
	}
	var mu sync.Mutex
	var steps []Step
	toolErrors := 0
	failed := func() {
		mu.Lock()
		toolErrors++
		mu.Unlock()
	}
	seen := map[string]bool{} // rulings that appeared in any tool result
	note := func(s Step, out string) {
		mu.Lock()
		defer mu.Unlock()
		steps = append(steps, s)
		for _, id := range rulingRef.FindAllString(out, -1) {
			seen[id] = true
		}
	}

	searchTool, err := functiontool.New(functiontool.Config{
		Name:        "search_rulings",
		Description: "Search earlier CBP rulings (only rulings dated before this case are returned). Returns ruling number, date, subject and the origin CBP held.",
	}, func(tc agent.ToolContext, a searchArgs) (toolOut, error) {
		hits, err := v.search.Search(ctx, a.Query, t.Before, exclude)
		if err != nil {
			failed()
			return toolOut{}, err
		}
		out := formatHits(hits)
		note(Step{"search_rulings", a.Query}, out)
		return toolOut{out}, nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("verifier: search tool: %w", err)
	}
	readTool, err := functiontool.New(functiontool.Config{
		Name:        "read_ruling",
		Description: "Read the facts and CBP's analysis/holding of an earlier ruling.",
	}, func(tc agent.ToolContext, a readArgs) (toolOut, error) {
		out, err := v.search.Read(ctx, strings.TrimSpace(a.Ruling), t.Before, exclude)
		if err != nil {
			failed()
			return toolOut{}, err
		}
		note(Step{"read_ruling", a.Ruling}, out)
		return toolOut{out}, nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("verifier: read tool: %w", err)
	}

	m := &adkModel{m: v.model, id: v.opts.Model, maxTokens: v.opts.MaxOutputTokens}
	var toolCalls, modelCalls int
	system := System(v.opts.MaxToolCalls)
	a, err := llmagent.New(llmagent.Config{
		Name:        "move_verifier",
		Description: "Checks a proposed production move against earlier CBP rulings.",
		Model:       m,
		InstructionProvider: func(agent.ReadonlyContext) (string, error) {
			return system, nil
		},
		GenerateContentConfig: &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](0)},
		Tools:                 []tool.Tool{searchTool, readTool},
		BeforeToolCallbacks: []llmagent.BeforeToolCallback{
			func(_ agent.ToolContext, _ tool.Tool, _ map[string]any) (map[string]any, error) {
				mu.Lock()
				defer mu.Unlock()
				toolCalls++
				if toolCalls > v.opts.MaxToolCalls {
					return map[string]any{"result": "Tool budget exhausted. Reply now with ONLY the final JSON."}, nil
				}
				return nil, nil
			},
		},
		BeforeModelCallbacks: []llmagent.BeforeModelCallback{
			func(_ agent.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
				mu.Lock()
				defer mu.Unlock()
				modelCalls++
				// After the budget, withhold the tools so the model must answer
				// (agent_search.py does the same on its last turn).
				if toolCalls >= v.opts.MaxToolCalls && req.Config != nil {
					req.Config.Tools = nil
				}
				if modelCalls > v.opts.MaxToolCalls+3 {
					return nil, fmt.Errorf("verifier: no final answer after %d model calls", modelCalls-1)
				}
				return nil, nil
			},
		},
	})
	if err != nil {
		return Result{}, fmt.Errorf("verifier: agent: %w", err)
	}
	r, err := runner.New(runner.Config{AppName: "origin-proof-verifier", Agent: a, SessionService: session.InMemoryService(), AutoCreateSession: true})
	if err != nil {
		return Result{}, fmt.Errorf("verifier: runner: %w", err)
	}

	var final string
	for ev, err := range r.Run(ctx, "planner", uuid.NewString(), genai.NewContentFromText(t.UserMessage(), genai.RoleUser), agent.RunConfig{}) {
		if err != nil {
			return Result{}, fmt.Errorf("verifier: run: %w", err)
		}
		if ev.Content == nil || ev.Partial {
			continue
		}
		var text strings.Builder
		hasCall := false
		for _, p := range ev.Content.Parts {
			if p.FunctionCall != nil {
				hasCall = true
			}
			if p.Text != "" && !p.Thought {
				text.WriteString(p.Text)
			}
		}
		if !hasCall && text.Len() > 0 {
			final = text.String()
		}
	}

	res := Result{Steps: steps, ModelCalls: m.calls, Usage: m.usage, ToolErrors: toolErrors}
	var ans answer
	if err := (llm.Response{Text: final}).DecodeJSON(&ans); err != nil {
		return res, fmt.Errorf("verifier: final answer: %w", err)
	}
	res.OriginA, res.OriginB = domain.NormalizeCountry(ans.OriginA), domain.NormalizeCountry(ans.OriginB)
	res.ChangesOrigin = ans.ChangesOrigin

	keep := func(p *string) string {
		if p == nil {
			return ""
		}
		id := strings.TrimSpace(*p)
		if !domain.ValidRulingID(id) {
			return ""
		}
		if !seen[id] {
			res.Dropped = append(res.Dropped, id)
			return ""
		}
		return id
	}
	vd := domain.Verdict{
		SupportingRuling: keep(ans.SupportingRuling),
		CounterRuling:    keep(ans.CounterRuling),
		Reasoning:        ans.Reasoning,
		Trace: domain.Trace{Provider: m.provider, ModelID: m.modelID, PromptVersion: t.promptVersion(),
			InputTokens: m.usage.InputTokens, OutputTokens: m.usage.OutputTokens},
	}
	if ans.Confidence != nil {
		vd.Confidence = *ans.Confidence
	}
	reachesTarget := t.Target == "" || res.OriginB == domain.NormalizeCountry(t.Target)
	vd.PrecedentSupported = ans.ChangesOrigin && reachesTarget && vd.SupportingRuling != ""
	if err := vd.Validate(); err != nil {
		return res, fmt.Errorf("verifier: %w: %v", llm.ErrBadResponse, err)
	}
	res.Verdict = vd
	return res, nil
}
