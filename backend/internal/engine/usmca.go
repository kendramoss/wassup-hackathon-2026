package engine

import "strings"

// USMCAVersion is the Part 102 (USMCA marking rules) prompt's version (D61).
const USMCAVersion = "usmca-v1"

// USMCASystem asks for origin under the Part 102 general rules, 19 CFR
// 102.11 with the 102.20 tariff shifts, which CBP applies to the marking of
// goods of Canada and Mexico (USMCA) instead of substantial transformation.
// Textile goods follow 102.21 (TextileSystem). The hierarchy follows the
// regulation text (102.11 and 102.18(b), checked 17 Oct 2026). The answer
// schema is the same as System's.
const USMCASystem = `You are a US customs origin analyst. You will be given the facts of a product submitted to U.S. Customs and Border Protection (CBP): its materials or components with their tariff classifications where given, where each is made, and the processing performed in each country.

Determine the country of origin CBP would assign under the Part 102 rules of origin (19 CFR 102.11 with the tariff shifts of 102.20), the rules CBP applies to the marking of goods processed in Canada or Mexico (USMCA) and other non-textile goods. Apply 102.11 strictly in order and stop at the first rule that decides:
(a)(1) the country where the good was wholly obtained or produced; (a)(2) produced there exclusively from domestic materials; (a)(3) the country where each foreign material underwent the change in tariff classification that 102.20 requires for the good's heading or subheading (the shift is from the material's own classification to the good's; read the 102.20 rule as quoted in the facts or as you know it, including its exceptions);
(b) if no single country under (a), and the good is not a set: the origin of the single material that imparts the essential character, considering only the materials whose classification could not make the required shift (102.18(b)); if only one such material, it is that material's origin;
(c) for sets, mixtures and composite goods under GRI 3: the origins of all materials meriting equal consideration;
(d) otherwise: (1) minor processing, the origin of the equally important materials; (2) simple assembly of equally important parts of one country, that country; (3) else the last country where the good underwent production.
Packing, dilution, a change in end use, dismantling, or gathering parts already classified as the finished good under GRI 2(a) never cause a tariff shift (102.17). The good's origin is usually where the last tariff shift happens; when an imported material fails the shift, the decision moves to (b). State which rule decides.

Return ONLY a JSON object:
{"origin_country": "<one country name in English>",
 "essential_character": "<the 102.11 rule that decides, e.g. (a)(3) tariff shift to 8509.80, or (b) essential character of the motor>",
 "decisive_step": "<the processing step and country that confers origin>",
 "flip": "<the smallest plausible change to the facts that would change the origin, or null>",
 "confidence": <0.0-1.0>,
 "reasoning": "<=80 words"}`

// USMCAVersion2 is the Part 102 prompt that applies only the quoted 102.20
// rule text (D550): USMCASystem2 with the rows for the codes named in the
// facts (Part102RuleText) after the facts.
const USMCAVersion2 = "usmca-v2"

// USMCASystem2 is USMCASystem with (a)(3) reading the 102.20 row quoted in
// the message, as written, with no exception the text does not contain
// (D541: on N344002 the engine barred 8414.90 parts from shifting to
// 8414.30, which the row allows).
const USMCASystem2 = `You are a US customs origin analyst. You will be given the facts of a product submitted to U.S. Customs and Border Protection (CBP): its materials or components with their tariff classifications where given, where each is made, and the processing performed in each country.

Determine the country of origin CBP would assign under the Part 102 rules of origin (19 CFR 102.11 with the tariff shifts of 102.20), the rules CBP applies to the marking of goods processed in Canada or Mexico (USMCA) and other non-textile goods. Apply 102.11 strictly in order and stop at the first rule that decides:
(a)(1) the country where the good was wholly obtained or produced; (a)(2) produced there exclusively from domestic materials; (a)(3) the country where each foreign material underwent the change in tariff classification that 102.20 requires for the good's heading or subheading (the shift is from the material's own classification to the good's);
(b) if no single country under (a), and the good is not a set: the origin of the single material that imparts the essential character, considering only the materials whose classification could not make the required shift (102.18(b)); if only one such material, it is that material's origin;
(c) for sets, mixtures and composite goods under GRI 3: the origins of all materials meriting equal consideration;
(d) otherwise: (1) minor processing, the origin of the equally important materials; (2) simple assembly of equally important parts of one country, that country; (3) else the last country where the good underwent production.
Packing, dilution, a change in end use, dismantling, or gathering parts already classified as the finished good under GRI 2(a) never cause a tariff shift (102.17). The good's origin is usually where the last tariff shift happens; when an imported material fails the shift, the decision moves to (b). State which rule decides.

When the message quotes 19 CFR 102.20 rule text, it is the regulation's own wording for the tariff codes named in the facts. Use the quoted row that covers the good's own classification; a row for a material's code does not apply to the good. Apply that row exactly as written: a material makes the shift unless the row's own words exclude its classification (its "from" clause, its "except from" list, or a quoted note). Do not add exceptions, conditions or carve-outs the quoted text does not contain; in particular, parts of the good are not barred from shifting unless the row's words bar their classification. If no quoted row covers the good, read the 102.20 rule as you know it.

Return ONLY a JSON object:
{"origin_country": "<one country name in English>",
 "essential_character": "<the 102.11 rule that decides, e.g. (a)(3) tariff shift to 8509.80, or (b) essential character of the motor>",
 "decisive_step": "<the processing step and country that confers origin>",
 "flip": "<the smallest plausible change to the facts that would change the origin, or null>",
 "confidence": <0.0-1.0>,
 "reasoning": "<=80 words"}`

// USMCAVersion3 is the Part 102 prompt with a structured 102.11 checklist
// (D590): the model fills the good's classification, each material's
// classification, origin and shift, the de minimis question and the rule
// that decides, before the origin. Eval only until D590's pass rule adopts
// it.
const USMCAVersion3 = "usmca-v3"

// USMCASystem3 is USMCASystem with the checklist and four instructions
// written from the D590 development error analysis (classifications the
// facts give are used as given; an assembly step is not a reason a
// material fails unless the rule names simple assembly; (b) only after a
// material fails; an unstated value is answered as if de minimis does not
// apply).
const USMCASystem3 = `You are a US customs origin analyst. You will be given the facts of a product submitted to U.S. Customs and Border Protection (CBP): its materials or components with their tariff classifications where given, where each is made, and the processing performed in each country.

Determine the country of origin CBP would assign under the Part 102 rules of origin (19 CFR 102.11 with the tariff shifts of 102.20), the rules CBP applies to the marking of goods processed in Canada or Mexico (USMCA) and other non-textile goods. Apply 102.11 strictly in order and stop at the first rule that decides:
(a)(1) the country where the good was wholly obtained or produced; (a)(2) produced there exclusively from domestic materials; (a)(3) the country where each foreign material underwent the change in tariff classification that 102.20 requires for the good's heading or subheading (the shift is from the material's own classification to the good's; read the 102.20 rule as quoted in the facts or as you know it, including its exceptions);
(b) if no single country under (a), and the good is not a set: the origin of the single material that imparts the essential character, considering only the materials whose classification could not make the required shift (102.18(b)); if only one such material, it is that material's origin;
(c) for sets, mixtures and composite goods under GRI 3: the origins of all materials meriting equal consideration;
(d) otherwise: (1) minor processing, the origin of the equally important materials; (2) simple assembly of equally important parts of one country, that country; (3) else the last country where the good underwent production.
Packing, dilution, a change in end use, dismantling, or gathering parts already classified as the finished good under GRI 2(a) never cause a tariff shift (102.17). The good's origin is usually where the last tariff shift happens; when an imported material fails the shift, the decision moves to (b). State which rule decides.

Work through the checklist before you answer:
1. The good's classification: when the facts give the good's tariff code, use it as given; otherwise state your reading.
2. The 102.20 rule for that classification, as quoted or as you know it.
3. Every material: its origin, whether it is foreign to the country where the good is produced, its classification (as the facts give it, otherwise your reading), and whether it makes the required change. A foreign material fails only when its own classification is one the rule's words exclude. Assembling, joining or fastening parts is not a reason a material fails; simple assembly matters only where the rule's own words name it.
4. Go to (b) only when at least one foreign material fails, and then weigh only the materials that fail.
5. De minimis (102.13): when a foreign material fails and its value share is not given, answer as if de minimis does not apply, and say so.

Return ONLY a JSON object:
{"good_classification": "<code and whether the facts give it>",
 "rule_102_20": "<the rule for the good's classification>",
 "materials": [{"name": "<material>", "origin": "<country>", "foreign": <true|false>, "classification": "<code or heading>", "shifts": "<yes|no|not needed>"}],
 "de_minimis": "<whether it decides, or why it does not>",
 "deciding_rule": "<(a)(1), (a)(2), (a)(3), (b), (c) or (d)>",
 "origin_country": "<one country name in English>",
 "essential_character": "<the 102.11 rule that decides, e.g. (a)(3) tariff shift to 8509.80, or (b) essential character of the motor>",
 "decisive_step": "<the processing step and country that confers origin>",
 "flip": "<the smallest plausible change to the facts that would change the origin, or null>",
 "confidence": <0.0-1.0>,
 "reasoning": "<=80 words"}`

// ChecklistSchema constrains the usmca-v3 answer: the checklist, then the
// standard answer fields.
var ChecklistSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"good_classification": map[string]any{"type": "string"},
		"rule_102_20":         map[string]any{"type": "string"},
		"materials": map[string]any{"type": "array", "items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":           map[string]any{"type": "string"},
				"origin":         map[string]any{"type": "string"},
				"foreign":        map[string]any{"type": "boolean"},
				"classification": map[string]any{"type": "string"},
				"shifts":         map[string]any{"type": "string", "enum": []string{"yes", "no", "not needed"}},
			},
			"required":             []string{"name", "origin", "foreign", "classification", "shifts"},
			"additionalProperties": false,
		}},
		"de_minimis":          map[string]any{"type": "string"},
		"deciding_rule":       map[string]any{"type": "string"},
		"origin_country":      map[string]any{"type": "string"},
		"essential_character": map[string]any{"type": "string"},
		"decisive_step":       map[string]any{"type": "string"},
		"flip":                map[string]any{"type": []string{"string", "null"}},
		"confidence":          map[string]any{"type": "number"},
		"reasoning":           map[string]any{"type": "string"},
	},
	"required": []string{"good_classification", "rule_102_20", "materials", "de_minimis", "deciding_rule",
		"origin_country", "essential_character", "decisive_step", "flip", "confidence", "reasoning"},
	"additionalProperties": false,
}

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
// minimis answer and the deciding rule. Eval only until D600's pass rule
// adopts it.
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
