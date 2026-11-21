// Package router labels a case's product family and origin regime with a fast
// model (Gemini Flash). It never decides origin and never overrides the
// engine: its labels drive the scope check in triage and the UI badge.
package router

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// PromptVersion is the production router prompt, recorded with every route.
// Other versions stay selectable for evaluation (NewVersion, eval route).
// router-v4 replaced router-v2 after passing the D230 rule on dev and the
// adoption check on the first held-out split (D232, D233).
const PromptVersion = PromptV4

// Router prompt versions.
const (
	PromptV2 = "router-v2"
	PromptV3 = "router-v3"
	PromptV4 = "router-v4"
)

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

// regimeAddendumV3 replaces regimeAddendum in router-v3 (D230). Error
// analysis on the dev router split: router-v2 sent 33 of 69 dev Part 102
// rulings (non-textile goods) to the general engine. Most are goods made or
// finished in Canada or Mexico, where CBP decides origin under the Part 102
// tariff-shift rules; router-v2 told the model not to pick usmca_part102 for
// that cue. It also told the model nothing about how the route and the regime
// relate, and scenario requests about drugs lost the pharma label. Tuned on
// the dev router split only.
const regimeAddendumV3 = `
Also return "regime": the origin rule CBP will apply to the question asked. Decide it separately from "route": a scenario, set or commodity can have any regime.
- textile_102_21: the product is a textile or apparel good (yarn, fabric, knit or woven garments, home textiles and bedding, sewn textile articles) classified in HTSUS chapters 50 to 63, whose origin follows the textile rules of 19 CFR 102.21. Goods classified outside those chapters (wipes, pads, diapers, footwear, furniture) are not textile goods even if they contain textile.
- usmca_part102: CBP applies the Part 102 tariff-shift rules (19 CFR 102.11 and 102.20) to a non-textile good when: (a) the request asks about USMCA or NAFTA preference, eligibility, "trade programs" or "trade program status"; or (b) the good is made, processed, finished or assembled in Canada or Mexico (its last production step named in the facts is there) and the request asks its country of origin or marking. Choose usmca_part102 in these cases even when foreign components are named and even when the facts sound like a substantial-transformation question.
- pharma_api: a drug, vitamin, supplement or medicinal preparation where the question turns on where the active pharmaceutical ingredient is made or processed. This holds for scenario requests too.
- substantial_transformation: anything else, including goods last processed in Canada or Mexico when the request asks only about Section 301 duties or government procurement (Trade Agreements Act), and goods last processed outside Canada and Mexico with no USMCA or trade-program question.
- unclear: you cannot tell from the facts.
"reason" is one line that names both the route and the regime evidence.
Final JSON: {"route": ..., "subfamily": ..., "regime": ..., "reason": ...}`

// regimeAddendumV4 is router-v3's addendum with the textile scope of 19 CFR
// 102.21 stated as the regulation states it (chapters 50-63 plus listed
// headings outside them, such as surgical gauze and sponges, textile bags,
// hats, umbrellas and bedding): router-v3 sent 3 dev textile rulings
// (surgical sponges of heading 3005) to the general engine (D230).
var regimeAddendumV4 = strings.Replace(regimeAddendumV3,
	"classified in HTSUS chapters 50 to 63, whose origin follows the textile rules of 19 CFR 102.21. Goods classified outside those chapters (wipes, pads, diapers, footwear, furniture) are not textile goods even if they contain textile.",
	"whose origin follows the textile rules of 19 CFR 102.21: goods of HTSUS chapters 50 to 63, and the textile goods 102.21 lists outside those chapters (surgical gauze, sponges and dressings of heading 3005, textile bags and cases of heading 4202, hats of headings 6501 to 6505, umbrellas, quilts and bedding of heading 9404). Other goods that merely contain textile (wipes, pet pads, footwear, furniture) are not textile goods.", 1)

// partCue matches facts that discuss what the Part 102 rules need: the
// tariff classification of the good or its materials, marking, or trade
// programs. On the router-v3 dev rows, 55 of the 65 Part 102 labels on Part
// 102 rulings had one (or named USMCA) and 5 of the 11 Part 102 labels on
// general-engine rulings did (D230).
var partCue = regexp.MustCompile(`(?i)classif|\bheading|subheading|HTSUS|\bHTS\b|chapter \d|\bmark(?:ing|ed)\b|trade program`)

// usmcaRuleV4 is router-v3's check plus a second one: a Part 102 label on
// facts that neither name USMCA or NAFTA nor discuss classification, marking
// or trade programs becomes substantial_transformation (a goods-made-in-
// Canada-or-Mexico label with nothing of the tariff-shift question in it).
func usmcaRuleV4(facts string, l Label) Label {
	l = usmcaRule(facts, l)
	if l.Regime == RegimeUSMCA && l.Rule == "" && !mentionsUSMCA.MatchString(facts) && !partCue.MatchString(facts) {
		l.Regime = "substantial_transformation"
		l.Rule = "no-part102-cue"
	}
	return l
}

// mentionsUSMCA matches a request that names the agreement. On the dev
// router split no general-engine ruling's facts name it (0 of 547) and 34 of
// 70 Part 102 rulings do (D230).
var mentionsUSMCA = regexp.MustCompile(`(?i)\bUSMCA\b|\bNAFTA\b|United States[- ]Mexico[- ]Canada`)

// usmcaRule is router-v3's deterministic check, applied after the model: a
// general (or unclear) label on facts that name USMCA or NAFTA becomes
// usmca_part102. Textile and pharmaceutical labels are kept.
func usmcaRule(facts string, l Label) Label {
	if (l.Regime == "substantial_transformation" || l.Regime == "unclear") && mentionsUSMCA.MatchString(facts) {
		l.Regime = RegimeUSMCA
		l.Rule = "usmca-named"
	}
	return l
}

// System is the full production router prompt.
var System = prompts[PromptVersion].system

// routerPrompt is one router version: its system prompt and the
// deterministic rules applied to the facts before the model is asked (nil:
// none).
type routerPrompt struct {
	system string
	// after adjusts the model's label from the facts (nil: none); ruleText
	// describes it for the prompt snapshot, so a change to the rule needs a
	// version bump like a change to the prompt.
	after    func(facts string, l Label) Label
	ruleText string
}

var prompts = map[string]routerPrompt{
	PromptV2: {system: routerMD + regimeAddendum},
	PromptV3: {system: routerMD + regimeAddendumV3, after: usmcaRule,
		ruleText: "after the model: regime substantial_transformation or unclear and facts match " + mentionsUSMCA.String() + " -> usmca_part102 (rule usmca-named)"},
	PromptV4: {system: routerMD + regimeAddendumV4, after: usmcaRuleV4,
		ruleText: "after the model: regime substantial_transformation or unclear and facts match " + mentionsUSMCA.String() + " -> usmca_part102 (rule usmca-named); regime usmca_part102 from the model and facts match neither that nor " + partCue.String() + " -> substantial_transformation (rule no-part102-cue)"},
}

// Versions lists the selectable router prompt versions, sorted.
func Versions() []string {
	var v []string
	for k := range prompts {
		v = append(v, k)
	}
	slices.Sort(v)
	return v
}

// RuleText describes a router version's deterministic check ("" if none).
func RuleText(version string) string { return prompts[version].ruleText }

// SystemFor returns the system prompt of a router version.
func SystemFor(version string) (string, bool) {
	p, ok := prompts[version]
	return p.system, ok
}

// Allowed values.
var (
	Routes      = []string{"set_kit", "commodity", "assembly", "scenario"}
	Subfamilies = []string{"electronics", "motorised", "metal_tools_parts", "battery_solar", "filters", "furniture_fixtures", "medical", "apparel_textile_goods", "other"}
	Regimes     = []string{"substantial_transformation", RegimeTextile, "usmca_part102", RegimePharma, "unclear"}
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
	// Rule names the deterministic check that changed the model's regime
	// (router-v3: "usmca-named"); empty when the model's label stands.
	Rule string `json:"rule,omitempty"`
}

// RegimeTextile is the label for textile and apparel goods (19 CFR 102.21).
const RegimeTextile = "textile_102_21"

// RegimeUSMCA is the label for USMCA preference or Canada/Mexico marking
// questions decided under the Part 102 tariff-shift rules (19 CFR 102.11).
const RegimeUSMCA = "usmca_part102"

// RegimePharma is the label for drugs whose origin turns on the active
// pharmaceutical ingredient (D62).
const RegimePharma = "pharma_api"

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
	version   string
	prompt    routerPrompt
}

// New returns a router with the production prompt using the given model ID
// (e.g. gemini-3.8-flash).
func New(model llm.Model, modelID string, maxTokens int) *Router {
	r, _ := NewVersion(model, modelID, maxTokens, PromptVersion)
	return r
}

// NewVersion returns a router with the named prompt version (Versions).
func NewVersion(model llm.Model, modelID string, maxTokens int, version string) (*Router, error) {
	p, ok := prompts[version]
	if !ok {
		return nil, fmt.Errorf("router: unknown prompt version %q (want one of %v)", version, Versions())
	}
	return &Router{model: model, modelID: modelID, maxTokens: maxTokens, version: version, prompt: p}, nil
}

// Version is the router's prompt version.
func (r *Router) Version() string { return r.version }

// RenderUser is the user message Classify sends.
func RenderUser(facts string) string { return "CASE FACTS:\n" + facts }

// Classify labels one case from its facts.
func (r *Router) Classify(ctx context.Context, facts string) (Label, error) {
	resp, err := r.model.Generate(ctx, llm.Request{
		Model:           r.modelID,
		System:          r.prompt.system,
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
	l.ModelID, l.Provider, l.PromptVersion = resp.ModelID, resp.Provider, r.version
	l.InputTokens, l.OutputTokens, l.LatencyMS = resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Latency.Milliseconds()
	if r.prompt.after != nil {
		l = r.prompt.after(facts, l)
	}
	return l, nil
}
