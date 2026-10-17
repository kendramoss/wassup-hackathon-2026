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

// PromptVersion2 is verifier-v2 (D74): verifier-v1 plus an explicit search
// for a counter-ruling and a confidence scale tied to the evidence read.
// Developed on the pre-2026 flip pairs only.
const PromptVersion2 = "verifier-v2"

// Evidence2 is added to the v1 method for verifier-v2. v1 left confidence
// unanchored: on the dev flip pairs every precedent-supported verdict was
// right (62/62) yet most were scored 0.75–0.90, so confidence said little.
const Evidence2 = `
6. COUNTER-SEARCH: before answering, run at least one search aimed at a counter-ruling: the same kind of component or operation moving, with CBP holding that origin did NOT change. Read the closest result if it looks on point.
7. CONFIDENCE comes from the evidence you READ, not from how plausible the answer feels:
   - 0.95–0.98: you read an earlier ruling on the same kind of product where CBP held that this same component or operation moving decided origin; the facts match on the point CBP relied on; and your counter-search found nothing on point.
   - 0.90–0.94: the same, but the anchor is a closely analogous product (same function and the same kind of deciding component), or a counter-ruling exists but differs on a fact CBP itself said mattered.
   - 0.80–0.89: the anchor shares the deciding component but differs in something CBP might weigh (for example more or fewer other operations stay in the old country).
   - 0.60–0.79: only distant analogues or general doctrine.
   - below 0.60: no relevant ruling found.
   The same scale applies when your answer is that origin does not change.
`

// PromptVersion3 is verifier-v3 (D75): verifier-v2 plus a fixed first
// search. v2's verdict on the same move swung between runs (heat gun: 0.92
// locally, 0.75 on production) because each run searched its own way and
// anchored on different rulings. v3 runs one deterministic search for the
// change itself before the agent starts and hands it the results, so every
// run starts from the same evidence.
const PromptVersion3 = "verifier-v3"

// Start3 is added for verifier-v3.
const Start3 = `
A first search for the change has already been run for you; its results are in the task. Start from them: read the closest one or two before searching more. Further searches are for the counter-search (step 6) or when none of the first results is on the same kind of product.
`

// System3 renders the verifier-v3 instruction.
func System3(maxToolCalls int) string {
	return Principles + Evidence2 + Start3 + fmt.Sprintf("\nUse the tools (at most %d calls). When done, reply with ONLY the final JSON (no tool call).\n", maxToolCalls) + FlipFinal
}

// changeQuery is the fixed first search for verifier-v3: the words of
// variant B that variant A lacks (what moved and where to), then B's opening
// words (what the product is). It depends only on the facts, so it is the
// same on every run.
func changeQuery(a, b string) string {
	inA := map[string]bool{}
	for _, w := range strings.Fields(strings.ToLower(a)) {
		inA[strings.Trim(w, ".,;:()\"'")] = true
	}
	var diff []string
	seen := map[string]bool{}
	for _, w := range strings.Fields(strings.ToLower(b)) {
		w = strings.Trim(w, ".,;:()\"'")
		if len(w) > 3 && !inA[w] && !seen[w] {
			seen[w] = true
			diff = append(diff, w)
		}
	}
	head := strings.Fields(b)
	if len(head) > 25 {
		head = head[:25]
	}
	if len(diff) > 25 {
		diff = diff[:25]
	}
	return strings.Join(append(diff, head...), " ")
}

// System2 renders the verifier-v2 instruction.
func System2(maxToolCalls int) string {
	return Principles + Evidence2 + fmt.Sprintf("\nUse the tools (at most %d calls). When done, reply with ONLY the final JSON (no tool call).\n", maxToolCalls) + FlipFinal
}

// Views are second opinions for the panel (D83). Each keeps verifier-v2's
// method and confidence scale, so confidences stay comparable, and changes
// only where it looks first.
const (
	// ViewSkeptic argues as the CBP import specialist who would deny the
	// change, and searches hardest for rulings that called it minor.
	ViewSkeptic = "skeptic"
	// ViewProduct searches by what the product is rather than by the part
	// that moved, to find same-product rulings a component search misses.
	ViewProduct = "product"
)

// viewText is the instruction each view adds to verifier-v2.
var viewText = map[string]string{
	ViewSkeptic: `
VIEW: You are the CBP import specialist reviewing this request, and your job is to test whether the change is enough. Search first and hardest for earlier rulings where the same kind of component or operation moved and CBP held it minor assembly, simple processing, or otherwise not a substantial transformation. Read the strongest such ruling. Only then look for support. Hold the change to the same burden of proof and confidence scale as above: do not lower confidence for a doubt the rulings you read do not support.
`,
	ViewProduct: `
VIEW: Start from what the product is, not from the part that moved. Search first for earlier rulings on the same kind of finished product (its name, its function), then read how CBP decided origin for those products and which component or operation it held decisive. Only then search by the moved component. Use the same confidence scale as above.
`,
}

// SystemView renders verifier-v2 with a view's instruction.
func SystemView(maxToolCalls int, view string) string {
	return Principles + Evidence2 + viewText[view] + fmt.Sprintf("\nUse the tools (at most %d calls). When done, reply with ONLY the final JSON (no tool call).\n", maxToolCalls) + FlipFinal
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

// promptVersion is the version recorded for a task under base version v.
func (t Task) promptVersion(v string) string {
	if t.KnownOriginA != "" {
		return v + "+known-a"
	}
	return v
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
	// Prompt is "v1" (default) or "v2" (D74).
	Prompt string
	// View, with Prompt v2, is a panel view (ViewSkeptic, ViewProduct; D83).
	View string
}

// Version is the verifier's prompt version (cache keys and traces use it).
func (v *Verifier) Version() string {
	switch v.opts.Prompt {
	case "v2":
		if v.opts.View != "" {
			return PromptVersion2 + "-" + v.opts.View
		}
		return PromptVersion2
	case "v3":
		return PromptVersion3
	}
	return PromptVersion
}

// Verifier runs the precedent-first agent.
type Verifier struct {
	model  llm.Model
	search Searcher
	opts   Options
}

// NewWithPanel returns the production checker: opts' verifier, with the
// skeptic and product views as a panel near the bar when panel is set
// (D83). The views need prompt v2.
func NewWithPanel(model llm.Model, search Searcher, opts Options, panel bool, low, high float64) Checker {
	first := New(model, search, opts)
	if !panel {
		return first
	}
	var views []Checker
	for _, view := range []string{ViewSkeptic, ViewProduct} {
		o := opts
		o.View = view
		views = append(views, New(model, search, o))
	}
	return NewPanel(first, views, low, high)
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
	var trail []domain.TrailStep
	toolErrors := 0
	failed := func() {
		mu.Lock()
		toolErrors++
		mu.Unlock()
	}
	seen := map[string]bool{} // rulings that appeared in any tool result
	note := func(s Step, out string, found []string) {
		mu.Lock()
		defer mu.Unlock()
		steps = append(steps, s)
		for _, id := range rulingRef.FindAllString(out, -1) {
			seen[id] = true
		}
		tool := "search"
		if s.Tool == "read_ruling" {
			tool = "read"
		}
		trail = append(trail, domain.TrailStep{Tool: tool, Query: s.Arg, Rulings: found})
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
		note(Step{"search_rulings", a.Query}, out, hitIDs(hits))
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
		var found []string
		if id := strings.TrimSpace(a.Ruling); out != notFound && domain.ValidRulingID(id) {
			found = []string{id}
		}
		note(Step{"read_ruling", a.Ruling}, out, found)
		return toolOut{out}, nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("verifier: read tool: %w", err)
	}

	m := &adkModel{m: v.model, id: v.opts.Model, maxTokens: v.opts.MaxOutputTokens}
	var toolCalls, modelCalls int
	system := System(v.opts.MaxToolCalls)
	switch v.opts.Prompt {
	case "v2":
		system = System2(v.opts.MaxToolCalls)
		if _, ok := viewText[v.opts.View]; ok {
			system = SystemView(v.opts.MaxToolCalls, v.opts.View)
		}
	case "v3":
		system = System3(v.opts.MaxToolCalls)
	}
	message := t.UserMessage()
	if v.opts.Prompt == "v3" {
		q := changeQuery(t.FactsA, t.FactsB)
		hits, err := v.search.Search(ctx, q, t.Before, exclude)
		if err != nil {
			failed()
		} else {
			out := formatHits(hits)
			note(Step{"first_search", q}, out, hitIDs(hits))
			message += "\n\nFIRST SEARCH (run for you, for the change between A and B):\n" + out
		}
	}
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
	for ev, err := range r.Run(ctx, "planner", uuid.NewString(), genai.NewContentFromText(message, genai.RoleUser), agent.RunConfig{}) {
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
		Trail:            trail,
		Trace: domain.Trace{Provider: m.provider, ModelID: m.modelID, PromptVersion: t.promptVersion(v.Version()),
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
