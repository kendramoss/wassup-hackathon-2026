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
	}
	return "EARLIER CBP RULINGS (precedent; may or may not be on point):\n\n" + strings.Join(blocks, "\n\n") +
		"\n\n---\n" + user + "\n\nIf you rely on a precedent, name its ruling number in reasoning."
}
