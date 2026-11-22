package engine

import "strings"

// stHead is the part of System before its answer format: the task and the
// substantial-transformation test, unchanged from engine-v1.
var stHead = System[:strings.Index(System, "\n\nReturn ONLY")]

// stChecklistRules ask for the component checklist (D650). They add no rule
// about which way a case goes; they make the engine write down, per major
// component, the facts the test turns on before it answers.
const stChecklistRules = `Before you answer, fill a short checklist. List only the major components or sub-assemblies (those that give the article its function or much of its value), at most 5, one entry each: what it is, where it is made, what is done to it or with it in the country of final processing or assembly, whether that processing turns it into a new article with a different name, character or use (yes), leaves its identity (no: assembly, finishing, testing or packing), or the facts do not say (unclear), and which earlier rulings above support that answer. Then answer from the checklist. Write no prose outside the fields and keep every field within its word limit.`

// stChecklistTail are the answer fields after the checklist, engine-v1's
// fields with word limits.
const stChecklistTail = ` "origin_country": "<one country name in English>",
 "essential_character": "<component or operation that drives the result, at most 12 words>",
 "decisive_step": "<the processing step and country that confer origin, at most 15 words>",
 "flip": "<the smallest plausible change to the facts that would change the origin, at most 15 words, or null>",
 "confidence": <0.0-1.0>,
 "reasoning": "<at most 50 words; name any ruling you rely on>"}`

const stChecklistHead = `{"assembly_country": "<the country of final processing or assembly>",
 "components": [{"part": "<at most 4 words>", "origin": "<country where it is made>", "essential": <true if it gives the article its essential character>, "processing": "<what is done to it or with it in the assembly country, at most 10 words>", "transforms": "<yes|no|unclear>", "precedents": ["<ruling numbers from above; [] if none>"]}],
`

// STChecklistVersion1 is the substantial-transformation prompt with a
// compact component checklist (D650, variant Va). Eval only unless D650's
// rule adopts it.
const STChecklistVersion1 = "st-checklist-v1"

// STChecklistSystem1 is engine-v1's task and test, the checklist
// instructions and the checklist answer (STChecklistSchema1).
var STChecklistSystem1 = stHead + "\n\n" + stChecklistRules + `

Return ONLY a JSON object:
` + stChecklistHead + stChecklistTail

// STChecklistVersion2 is STChecklistVersion1 with the two closest contrasting
// precedents named (D650, variant Vb).
const STChecklistVersion2 = "st-checklist-v2"

// stPrecedentRule is the Vb instruction: compare with the closest earlier
// ruling that held a component's country although assembly was elsewhere,
// and with the closest that held the assembling country.
const stPrecedentRule = `Then compare with the earlier rulings above: name the closest one in which CBP held the country of a component or sub-assembly although the article was assembled elsewhere, and the closest one in which CBP held the assembling country (null if there is none), say how each differs from these facts, and follow the one whose split of work between countries is closer to these facts.`

// STChecklistSystem2 is STChecklistSystem1 with the precedent comparison
// (STChecklistSchema2).
var STChecklistSystem2 = stHead + "\n\n" + stChecklistRules + "\n\n" + stPrecedentRule + `

Return ONLY a JSON object:
` + stChecklistHead + ` "component_precedent": {"ruling": "<ruling number or null>", "differs": "<the difference from these facts that matters, at most 12 words, or none>"},
 "assembly_precedent": {"ruling": "<ruling number or null>", "differs": "<the difference from these facts that matters, at most 12 words, or none>"},
 "closer": "<component|assembly|neither>",
` + stChecklistTail

var stComponent = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"part":       map[string]any{"type": "string"},
		"origin":     map[string]any{"type": "string"},
		"essential":  map[string]any{"type": "boolean"},
		"processing": map[string]any{"type": "string"},
		"transforms": map[string]any{"type": "string", "enum": []string{"yes", "no", "unclear"}},
		"precedents": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	},
	"required":             []string{"part", "origin", "essential", "processing", "transforms", "precedents"},
	"additionalProperties": false,
}

var stPrecedent = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"ruling":  map[string]any{"type": []string{"string", "null"}},
		"differs": map[string]any{"type": "string"},
	},
	"required":             []string{"ruling", "differs"},
	"additionalProperties": false,
}

func stChecklistSchema(extra map[string]any, extraOrder []string) map[string]any {
	props := map[string]any{
		"assembly_country": map[string]any{"type": "string"},
		"components":       map[string]any{"type": "array", "items": stComponent},
	}
	for k, v := range extra {
		props[k] = v
	}
	for k, v := range Schema["properties"].(map[string]any) {
		props[k] = v
	}
	req := append([]string{"assembly_country", "components"}, extraOrder...)
	req = append(req, Schema["required"].([]string)...)
	return map[string]any{"type": "object", "properties": props, "required": req, "additionalProperties": false}
}

// STChecklistSchema1 constrains the st-checklist-v1 answer.
var STChecklistSchema1 = stChecklistSchema(nil, nil)

// STChecklistSchema2 constrains the st-checklist-v2 answer.
var STChecklistSchema2 = stChecklistSchema(map[string]any{
	"component_precedent": stPrecedent,
	"assembly_precedent":  stPrecedent,
	"closer":              map[string]any{"type": "string", "enum": []string{"component", "assembly", "neither"}},
}, []string{"component_precedent", "assembly_precedent", "closer"})

// STChecklistOptions returns the prompt, version and schema of a checklist
// variant: "ck1" (st-checklist-v1) or "ck2" (st-checklist-v2); ok is false
// for any other name.
func STChecklistOptions(v string) (Options, bool) {
	switch v {
	case "ck1":
		return Options{System: STChecklistSystem1, Version: STChecklistVersion1, Schema: STChecklistSchema1}, true
	case "ck2":
		return Options{System: STChecklistSystem2, Version: STChecklistVersion2, Schema: STChecklistSchema2}, true
	}
	return Options{}, false
}
