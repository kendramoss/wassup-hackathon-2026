package main

import (
	"fmt"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpcorpus"
)

// PromptVersion names the Japan baseline prompt (D711): engine-v1's general
// prompt (engine.System) adapted to "Japan, preference under <agreement>".
// Eval only; nothing in the server uses it.
const PromptVersion = "jp-baseline-v1"

// system adapts engine.System: same role, same JSON-only answer with
// confidence and short reasoning, same precedent block; the question becomes
// Japanese preferential origin under the claimed agreement, and the answer
// names the origin criterion.
func system(agreement string) string {
	name := jpcorpus.AgreementNames[agreement]
	q := fmt.Sprintf("Determine whether the good qualifies as an originating good for preferential tariff treatment under %s on import into Japan, and the origin criterion it meets. Apply the product-specific rule for the good's tariff number in that agreement and the agreement's general origin rules.", name)
	if agreement == "GSP" {
		q = "Determine whether the good qualifies for Japan's GSP preferential rate as originating in the beneficiary country, and the origin criterion it meets under the GSP origin rules (Temporary Tariff Measures Act enforcement rules)."
	}
	if agreement == "Non-preferential" {
		q = "Determine the country of origin under Japan's non-preferential rules of origin, and the criterion that confers it."
	}
	return `You are a Japan customs origin analyst. You will be given the facts of a good imported into Japan, as submitted to Japan Customs for an advance ruling on origin: its tariff number, its materials (with their tariff chapter, heading or subheading, where they were made, and whether they are originating) and the production process.

` + q + `

Criteria: WO = wholly obtained or produced entirely in one party; PE = produced entirely from originating materials; CC = change in tariff classification at chapter level; CTH = change of heading; CTSH = change of subheading; RVC = a value test (regional or qualifying value content, or a maximum share of non-originating materials); SP = a specific manufacturing or processing operation required by the rule (for example knitting to shape, or cutting and sewing). If the rule offers alternatives, name the one the facts most clearly meet.

Return ONLY a JSON object:
{"qualifies": <true or false>,
 "criterion": "<WO, PE, CC, CTH, CTSH, RVC, SP, or NONE if it does not qualify>",
 "origin_country": "<one country name in English, or EU for the European Union>",
 "deciding_rule": "<the rule the good meets or fails, in a few words>",
 "confidence": <0.0-1.0>,
 "reasoning": "<=80 words"}`
}

var schema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"qualifies":      map[string]any{"type": "boolean"},
		"criterion":      map[string]any{"type": "string", "enum": []string{"WO", "PE", "CC", "CTH", "CTSH", "RVC", "SP", "NONE"}},
		"origin_country": map[string]any{"type": "string"},
		"deciding_rule":  map[string]any{"type": "string"},
		"confidence":     map[string]any{"type": "number"},
		"reasoning":      map[string]any{"type": "string"},
	},
	"required":             []string{"qualifies", "criterion", "origin_country", "deciding_rule", "confidence", "reasoning"},
	"additionalProperties": false,
}

// user mirrors engine.RenderUser: precedent blocks, then the case facts.
// Precedent facts keep engine.PrecedentFactsRunes; the holding is the
// answer plus the end of the reasoning (jpcorpus.Precedent), where Japan
// Customs names the rule met.
func user(facts string, precedents []domain.Ruling) string {
	u := "FACTS (advance ruling request, reasoning and answer removed):\n" + corpus.Truncate(facts, engine.FactsRunes)
	if len(precedents) == 0 {
		return u
	}
	blocks := make([]string, len(precedents))
	for i, p := range precedents {
		blocks[i] = fmt.Sprintf("[%s | %s] FACTS: %s\nJAPAN CUSTOMS ANSWER: %s",
			p.ID, p.Date.Format("2006-01-02"),
			corpus.Truncate(p.Facts, engine.PrecedentFactsRunes),
			p.Holding)
	}
	return "EARLIER JAPAN CUSTOMS ADVANCE RULINGS (precedent; may or may not be on point):\n\n" + strings.Join(blocks, "\n\n") +
		"\n\n---\n" + u + "\n\nIf you rely on a precedent, name its ruling number in reasoning."
}
