package verifier

import "fmt"

const PromptVersion4 = "verifier-v4"

const functionalContrast4 = `
8. FUNCTIONAL CONTRAST: identify what work gives this particular article its working features, not just its outline or a generic component name. Distinguish raw stock, a dedicated unfinished blank, and the complete functional article. Explain whether those working features already exist before the later forming, machining or finishing operation. A close ruling about a finished part is not interchangeable with one about an unfinished blank merely because both involve the same material.
9. Check the reverse direction too. If the decisive manufacturing moves back, do not assume the known origin of A survives. Determine B from its own manufacturing facts, using earlier rulings on the same functional distinction. Neither forging nor machining is a categorical winner: compare the actual work and stage reached in each country.
10. Production volume, ordinary packing, labelling, transit and storage alone do not supply a missing manufacturing change. If the decisive operations or locations are missing, withhold support instead of importing those facts from an earlier ruling. Preserve the evidence-based confidence scale: a plausible functional explanation without a close earlier ruling stays below the shown-plan bar.
`

const functionalFinal4 = `Final answer schema: {"origin_a": "<country>", "origin_b": "<country>", "changes_origin": true|false, "supporting_ruling": "<earlier ruling where CBP treated the material manufacturing distinction as decisive, or null>", "counter_ruling": "<earlier ruling where a materially similar change did NOT alter origin, or null>", "confidence": 0.0-1.0, "reasoning": "<=100 words"}
Burden of proof: answer changes_origin=true ONLY if you found an earlier ruling supporting why the material manufacturing distinction changes origin on these facts. Otherwise answer false. Do not treat the name of a process, the destination country, or a generic doctrine as a substitute for the close ruling. A counter-ruling controls only when the facts that mattered to CBP are materially the same.`

func System4(maxToolCalls int, view string) string {
	return Principles + Evidence2 + functionalContrast4 + viewText[view] + fmt.Sprintf("\nUse the tools (at most %d calls). When done, reply with ONLY the final JSON (no tool call).\n", maxToolCalls) + functionalFinal4
}
