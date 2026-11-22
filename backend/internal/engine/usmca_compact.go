package engine

import "strings"

// usmcaHead is the part of USMCASystem before its answer format: the 102.11
// hierarchy and the 102.17 list.
var usmcaHead = USMCASystem[:strings.Index(USMCASystem, "\n\nReturn ONLY")]

// usmcaCompactRules are the D590 checklist instructions, written tersely
// for the compact checklists (D600).
const usmcaCompactRules = `Use the tariff codes the facts give, as given; read a code yourself only when the facts give none. A foreign material (one not of the country where the good is produced) fails only when its own code is one the 102.20 rule's words exclude. Assembling, joining or fastening parts is not a reason a material fails; simple assembly matters only where the rule's own words name it. Go to (b) only when a foreign material fails, and then weigh only the materials that fail. De minimis (102.13): when a foreign material fails and its value share is not given, answer "value not given" and decide as if de minimis does not apply.`

// USMCAVersion4 is the Part 102 prompt with a compact checklist (D600,
// variant Vc): the good's code, one short entry per foreign material (code,
// origin, whether it makes the required change), the de minimis answer and
// the deciding rule, as fixed short fields, and a short answer. Eval only
// until D600's pass rule adopts it.
const USMCAVersion4 = "usmca-v4"

// USMCASystem4 is USMCASystem with the D590 instructions and the compact
// checklist answer (CompactChecklistSchema).
var USMCASystem4 = usmcaHead + "\n\n" + usmcaCompactRules + `

Fill the short fields first. Write no prose outside the fields and keep every field within its word limit.

Return ONLY a JSON object:
{"good_hts": "<the good's code, as the facts give it, else your reading>",
 "foreign_materials": [{"part": "<at most 3 words>", "hts": "<its code>", "origin": "<country>", "shift": "<yes|no|unknown: whether it makes the change the 102.20 rule requires for the good's code>"}],
 "de_minimis": "<not needed|applies|does not apply|value not given>",
 "rule_step": "<(a)(1)|(a)(2)|(a)(3)|(b)|(c)|(d)>",
 "origin_country": "<one country name in English>",
 "essential_character": "<the 102.11 rule that decides, at most 12 words, e.g. (a)(3) tariff shift to 8509.80>",
 "decisive_step": "<the processing step and country that confer origin, at most 15 words>",
 "flip": "<the smallest plausible change to the facts that would change the origin, at most 15 words, or null>",
 "confidence": <0.0-1.0>,
 "reasoning": "<at most 40 words>"}`

// USMCAVersion5 is the Part 102 prompt with the smallest checklist (D600,
// variant Vd): the good's code, only the foreign materials that fail, the de
// minimis answer and the deciding rule. The production Part 102 engine since
// D602 (USMCAProduction).
const USMCAVersion5 = "usmca-v5"

// USMCASystem5 is USMCASystem4 with the per-material list replaced by the
// failing materials' names (FailingChecklistSchema).
var USMCASystem5 = usmcaHead + "\n\n" + usmcaCompactRules + `

Fill the short fields first. Write no prose outside the fields and keep every field within its word limit.

Return ONLY a JSON object:
{"good_hts": "<the good's code, as the facts give it, else your reading>",
 "failing_materials": ["<each foreign material that does not make the change the 102.20 rule requires for the good's code, at most 3 words; [] if none>"],
 "de_minimis": "<not needed|applies|does not apply|value not given>",
 "rule_step": "<(a)(1)|(a)(2)|(a)(3)|(b)|(c)|(d)>",
 "origin_country": "<one country name in English>",
 "essential_character": "<the 102.11 rule that decides, at most 12 words, e.g. (a)(3) tariff shift to 8509.80>",
 "decisive_step": "<the processing step and country that confer origin, at most 15 words>",
 "flip": "<the smallest plausible change to the facts that would change the origin, at most 15 words, or null>",
 "confidence": <0.0-1.0>,
 "reasoning": "<at most 40 words>"}`

// compactTail are the fields both compact checklists share, after their
// material field.
var compactTail = map[string]any{
	"good_hts":            map[string]any{"type": "string"},
	"de_minimis":          map[string]any{"type": "string", "enum": []string{"not needed", "applies", "does not apply", "value not given"}},
	"rule_step":           map[string]any{"type": "string", "enum": []string{"(a)(1)", "(a)(2)", "(a)(3)", "(b)", "(c)", "(d)"}},
	"origin_country":      map[string]any{"type": "string"},
	"essential_character": map[string]any{"type": "string"},
	"decisive_step":       map[string]any{"type": "string"},
	"flip":                map[string]any{"type": []string{"string", "null"}},
	"confidence":          map[string]any{"type": "number"},
	"reasoning":           map[string]any{"type": "string"},
}

func compactSchema(field string, spec map[string]any) map[string]any {
	props := map[string]any{field: spec}
	for k, v := range compactTail {
		props[k] = v
	}
	return map[string]any{
		"type":       "object",
		"properties": props,
		"required": []string{"good_hts", field, "de_minimis", "rule_step",
			"origin_country", "essential_character", "decisive_step", "flip", "confidence", "reasoning"},
		"additionalProperties": false,
	}
}

// CompactChecklistSchema constrains the usmca-v4 answer.
var CompactChecklistSchema = compactSchema("foreign_materials", map[string]any{"type": "array", "items": map[string]any{
	"type": "object",
	"properties": map[string]any{
		"part":   map[string]any{"type": "string"},
		"hts":    map[string]any{"type": "string"},
		"origin": map[string]any{"type": "string"},
		"shift":  map[string]any{"type": "string", "enum": []string{"yes", "no", "unknown"}},
	},
	"required":             []string{"part", "hts", "origin", "shift"},
	"additionalProperties": false,
}})

// FailingChecklistSchema constrains the usmca-v5 answer.
var FailingChecklistSchema = compactSchema("failing_materials", map[string]any{"type": "array", "items": map[string]any{"type": "string"}})

// USMCAProduction is the production Part 102 engine's prompt, version and
// answer schema: usmca-v5 since D602 (selected on development under D600
// and D601, then scored once on the two held-out windows). usmca-v1 stays
// selectable for evaluation.
func USMCAProduction() Options {
	return Options{System: USMCASystem5, Version: USMCAVersion5, Schema: FailingChecklistSchema}
}
