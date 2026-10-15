package engine

import (
	"fmt"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// PromptVersion is recorded in every prediction and eval row. engine-v1 is
// the frozen research prompt, unchanged.
const PromptVersion = "engine-v1"

// System is copied verbatim from SYSTEM in
// hackathon-research/experiments/origin-proof/predict.py (frozen since the
// pilot; produced the 89.1% held-out result).
const System = `You are a US customs origin analyst. You will be given the facts of a product submitted to U.S. Customs and Border Protection (CBP): its components, where each is made, and the processing performed in each country.

Determine the country of origin CBP would assign for non-preferential purposes (Section 301, marking) under the substantial-transformation test: the last country in which the article underwent a substantial transformation into a new and different article of commerce with a distinct name, character or use. Consider which component(s) impart the essential character, the complexity and value of the processing in each country, and whether later operations are mere assembly or finishing.

Return ONLY a JSON object:
{"origin_country": "<one country name in English>",
 "essential_character": "<component or operation that drives the result>",
 "decisive_step": "<the processing step and country that confers origin>",
 "flip": "<the smallest plausible change to the facts that would change the origin, or null>",
 "confidence": <0.0-1.0>,
 "reasoning": "<=80 words"}`

// Text limits from predict.py, in characters.
const (
	FactsRunes          = 12000
	PrecedentFactsRunes = 1500
)

// Schema constrains the answer to the shape System asks for.
var Schema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"origin_country":      map[string]any{"type": "string"},
		"essential_character": map[string]any{"type": "string"},
		"decisive_step":       map[string]any{"type": "string"},
		"flip":                map[string]any{"type": []string{"string", "null"}},
		"confidence":          map[string]any{"type": "number"},
		"reasoning":           map[string]any{"type": "string"},
	},
	"required":             []string{"origin_country", "essential_character", "decisive_step", "flip", "confidence", "reasoning"},
	"additionalProperties": false,
}

// RenderUser builds the user message exactly as predict.py does for the
// "prec" condition: precedent blocks (facts truncated to 1,500 characters,
// holding to 600), then the case facts truncated to 12,000 characters.
func RenderUser(facts string, precedents []domain.Ruling) string {
	return renderUser(facts, precedents, false)
}

// ReasoningVersion names the precedent-reasoning variant of the user message.
const ReasoningVersion = "reasoning-v1"

func renderUser(facts string, precedents []domain.Ruling, withReasoning bool) string {
	user := "FACTS (ruling request, analysis removed):\n" + corpus.Truncate(facts, FactsRunes)
	if len(precedents) == 0 {
		return user
	}
	blocks := make([]string, len(precedents))
	for i, p := range precedents {
		blocks[i] = fmt.Sprintf("[%s | %s] FACTS: %s\nCBP HOLDING: %s",
			p.ID, p.Date.Format("2006-01-02"),
			corpus.Truncate(p.Facts, PrecedentFactsRunes),
			corpus.Truncate(p.Holding, corpus.PoolHoldingRunes))
		if withReasoning && p.Reasoning != "" {
			blocks[i] += "\nCBP REASONING: " + p.Reasoning
		}
	}
	return "EARLIER CBP RULINGS (precedent; may or may not be on point):\n\n" + strings.Join(blocks, "\n\n") +
		"\n\n---\n" + user + "\n\nIf you rely on a precedent, name its ruling number in reasoning."
}

// RulesVersion names Rules; a prediction made with them records
// PromptVersion+"+"+RulesVersion.
const RulesVersion = "rules-v1"

// Rules are short statements of CBP's substantial-transformation practice for
// the three judgments the engine gets wrong at high confidence (dev error
// review, D27). Eval-only until adopted.
const Rules = `CBP practice on three recurring questions:
1. Chemical and mineral processing. A chemical reaction that creates a different compound (a new chemical name, formula or CAS number, including forming a different hydrate or salt) is generally a substantial transformation. Mixing, diluting, purifying, drying, crystallising or packaging without a new compound generally is not. Crushing, milling or sizing generally is not, unless the facts show it makes the material fit for a specific use it could not serve before.
2. Blanks and preforms. If the imported article is a blank or preform that already has the approximate shape, dimensions and predetermined use of the finished article and is dedicated to it, later finishing (machining, flow forming, heat treatment, drilling, coating, sharpening) generally does not substantially transform it. If the input is a generic material (bar, sheet, coil, wire, resin, powder) that is formed into the article's shape, the forming does.
3. Essential character in assemblies. The essential character usually comes from the component that performs the article's primary function for the user (the functional base), not from the most decorative, most visible or most numerous parts. Weigh what the article does over what it looks like.`
