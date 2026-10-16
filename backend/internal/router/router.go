// Package router labels a case's product family and origin regime with a fast
// model (Gemini Flash). It never decides origin and never overrides the
// engine: its labels drive the scope check in triage and the UI badge.
package router

import (
	"context"
	"fmt"
	"slices"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// PromptVersion is recorded with every route.
const PromptVersion = "router-v2"

// routerMD is skill/specialists/ROUTER.md from
// hackathon-research/experiments/origin-proof, verbatim.
const routerMD = "# Router\n\n" +
	"Read the case facts. Do not decide origin. Return ONLY JSON:\n" +
	"`{\"route\": \"set_kit|commodity|assembly|scenario\", \"subfamily\": \"<one value>\", \"reason\": \"<one line>\"}`\n\n" +
	"Check these in order and stop at the first match:\n\n" +
	"1. **scenario**: the request sets out alternative production paths (\"Scenario 1/2\", \"Option A/B\", \"alternatively\", several sourcing variants), OR reconsiders an earlier decision (revocation, modification, reconsideration, protest, further review). Choose this even when the product is a set or a commodity.\n" +
	"2. **set_kit**: two or more different non-textile articles packed together for retail sale (tool set in a case, kit, gift set, multi-pack with accessories), or a composite good where one component must be picked (lock with knob, watch with strap).\n" +
	"3. **commodity**: a natural or processed material with no assembly: seafood, meat, produce, tea, oils, chemicals, minerals, metal powders, resins, drugs, vitamin tablets, supplements, cosmetics, soap.\n" +
	"4. **assembly**: anything else built from parts. This includes food appliances, filters and filter cartridges, batteries, and articles cut and sewn from textiles.\n\n" +
	"Tie-breaks:\n" +
	"- Textile goods, including bedding sets, go to assembly / apparel_textile_goods.\n" +
	"- A single article with spare parts or a manual is still assembly.\n" +
	"- If unsure between commodity and assembly, ask: are discrete parts joined? If yes, choose assembly.\n\n" +
	"Subfamily (set \"other\" unless route is assembly): electronics (PCBA, sensors, chargers, devices), motorised (motors, pumps, compressors, appliances, HVAC, vehicles), metal_tools_parts (hand tools, blades, forgings, castings, auto parts), battery_solar, filters, furniture_fixtures (furniture, blinds, faucets, locks, lighting), medical, apparel_textile_goods (bags, footwear, plush toys, sewn goods), other.\n"

// regimeAddendum is new (not in the research prompt): the regime label used
// by the scope guard. router-v1 marked rulings that merely mention Canada,
// Mexico or USMCA as usmca_part102 (3 of the first 35 dev cases, all decided
// by CBP under substantial transformation); router-v2 narrows that label.
// Tuned and validated on the dev window only.
const regimeAddendum = `
Also return "regime": the origin rule CBP will apply to the question asked.
- textile_102_21: the product is a textile or apparel good (yarn, fabric, knit or woven garments, home textiles and bedding, sewn textile articles), whose origin follows the textile rules of 19 CFR 102.21.
- usmca_part102: the request asks whether the good qualifies for USMCA or NAFTA preference, or asks for marking of a good of Canada or Mexico under the Part 102 tariff-shift rules. Do NOT choose this just because a component, plant or the requester is in Canada, Mexico or the United States, or because USMCA is mentioned in passing; if the question is origin for Section 301 duties, marking of a non-USMCA good, government procurement (Trade Agreements Act) or substantial transformation, choose substantial_transformation.
- pharma_api: a drug, vitamin or supplement where the question turns on where the active pharmaceutical ingredient is made or processed.
- substantial_transformation: anything else.
- unclear: you cannot tell from the facts.
Final JSON: {"route": ..., "subfamily": ..., "regime": ..., "reason": ...}`

// System is the full router prompt.
const System = routerMD + regimeAddendum

// Allowed values.
var (
	Routes      = []string{"set_kit", "commodity", "assembly", "scenario"}
	Subfamilies = []string{"electronics", "motorised", "metal_tools_parts", "battery_solar", "filters", "furniture_fixtures", "medical", "apparel_textile_goods", "other"}
	Regimes     = []string{"substantial_transformation", RegimeTextile, "usmca_part102", "pharma_api", "unclear"}
)

// Schema constrains the router's answer to the allowed values.
var Schema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"route":     map[string]any{"type": "string", "enum": Routes},
		"subfamily": map[string]any{"type": "string", "enum": Subfamilies},
		"regime":    map[string]any{"type": "string", "enum": Regimes},
		"reason":    map[string]any{"type": "string"},
	},
	"required":             []string{"route", "subfamily", "regime", "reason"},
	"additionalProperties": false,
}

// Label is the router's output.
type Label struct {
	Route         string `json:"route"`
	Subfamily     string `json:"subfamily"`
	Regime        string `json:"regime"`
	Reason        string `json:"reason"`
	ModelID       string `json:"model_id"`
	Provider      string `json:"llm_provider"`
	PromptVersion string `json:"prompt_version"`
	InputTokens   int    `json:"input_tokens"`
	OutputTokens  int    `json:"output_tokens"`
	LatencyMS     int64  `json:"latency_ms"`
}

// RegimeTextile is the label for textile and apparel goods (19 CFR 102.21).
const RegimeTextile = "textile_102_21"

// OutOfScopeRegimes are routed to review: origin prediction covers the
// substantial-transformation regime only.
var outOfScope = map[string]string{
	"textile_102_21": "textile goods follow the 19 CFR 102.21 rules, which this tool does not cover",
	"usmca_part102":  "USMCA / Part 102 tariff-shift rules are not covered by this tool",
	"pharma_api":     "pharmaceutical active-ingredient cases are not covered by this tool",
}

// Scope reports whether the label is in the tool's scope, and why not.
func (l Label) Scope() (inScope bool, reason string) {
	if r, ok := outOfScope[l.Regime]; ok {
		return false, r
	}
	return true, ""
}

// Router labels cases.
type Router struct {
	model     llm.Model
	modelID   string
	maxTokens int
}

// New returns a router using the given model ID (e.g. gemini-3.8-flash).
func New(model llm.Model, modelID string, maxTokens int) *Router {
	return &Router{model: model, modelID: modelID, maxTokens: maxTokens}
}

// RenderUser is the user message Classify sends.
func RenderUser(facts string) string { return "CASE FACTS:\n" + facts }

// Classify labels one case from its facts.
func (r *Router) Classify(ctx context.Context, facts string) (Label, error) {
	resp, err := r.model.Generate(ctx, llm.Request{
		Model:           r.modelID,
		System:          System,
		Messages:        []llm.Message{{Role: llm.RoleUser, Content: RenderUser(facts)}},
		JSONSchema:      Schema,
		SchemaName:      "route",
		MaxOutputTokens: r.maxTokens,
	})
	if err != nil {
		return Label{}, fmt.Errorf("router: classify: %w", err)
	}
	var l Label
	if err := resp.DecodeJSON(&l); err != nil {
		return Label{}, fmt.Errorf("router: classify: %w", err)
	}
	if !slices.Contains(Routes, l.Route) || !slices.Contains(Subfamilies, l.Subfamily) || !slices.Contains(Regimes, l.Regime) {
		return Label{}, fmt.Errorf("router: classify: %w: route=%q subfamily=%q regime=%q", llm.ErrBadResponse, l.Route, l.Subfamily, l.Regime)
	}
	l.ModelID, l.Provider, l.PromptVersion = resp.ModelID, resp.Provider, PromptVersion
	l.InputTokens, l.OutputTokens, l.LatencyMS = resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Latency.Milliseconds()
	return l, nil
}
