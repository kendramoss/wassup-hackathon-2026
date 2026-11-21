# Google ADK agent running a full Unde case over MCP (run 22 Nov 2026)

Status: run end to end once, on 22 Nov 2026, against the production endpoint `https://undelabs.dev/mcp`, anonymously (no Unde API key). The agent is [`examples/adk-agent`](../../examples/adk-agent/): a Google ADK `LlmAgent` with an `McpToolset` over `StreamableHTTPConnectionParams`, filtered to the tools a case needs. Its model was Gemini 3.8 Flash through an OpenAI-compatible LiteLLM gateway to Gemini (ADK's `LiteLlm` wrapper), not a Google API key. Gateway-reported model cost for the agent: $0.0465 over 7 model calls. The tools' own model calls ran on Unde's side, under its usual limits, and are not in that figure. It created one case.

## What we ran

Versions: Python 3.13.9, google-adk 2.14.1, mcp 2.3.0, litellm 1.110.2 ([requirements.txt](../../examples/adk-agent/requirements.txt)).

```bash
cd examples/adk-agent
python3 -m venv .venv && ./.venv/bin/pip install -r requirements.txt
export LITELLM_BASE_URL=...   # an OpenAI-compatible LiteLLM gateway to Gemini (not in this repository)
export LITELLM_API_KEY=...
./.venv/bin/python run_case.py
```

The product is a heat gun like the demo's, built new for this run: motor, fan, PCBA and heating element from China, housing, nozzle and cord from Vietnam, assembled in Vietnam, with a plant in Thailand that could take the motor-fan, PCBA, heating-element and final-assembly work. The target country is Thailand.

## What the agent did

1. `extract_product` drafted 7 parts, 3 plants and 2 operations, including the Thai plant from the description.
2. `predict_origin` predicted China (calibrated confidence 90%, model stated 95%), triage `auto`, citing N357771 and N358092, and created case `case_b27b37913fd7`. The agent passed the returned `case_access_key` to every later call.
3. `suggest_part_hts` (listed by the live server on this date) suggested codes for each part, each marked as from an earlier ruling or the model's own reading. The agent reported them as unverified and did not put them on the product.
4. `plan_production_moves` to Thailand: the first call passed ADK's default 300 s MCP read timeout and the client gave up; the agent called it again, and the second call answered after 103 s with 1 precedent-supported plan (move the motor-fan assembly, the PCBA and the heating element to Thailand; predicted Thailand, supported by N357771) and 7 rejected moves. The server may have run the plan search twice. `examples/adk-agent` now sets a 900 s read timeout (D391); we did not run it again.
5. `export_ruling_request_draft` for that plan returned a 19 CFR Part 177 request draft with placeholders for the requester.

The final answer ends with "Not a ruling — confirm with a licensed customs broker." It is the model's text, kept as it came. Its reasoning paragraphs paraphrase the tool results in the model's words and were not checked line by line; the origin, confidence, plan and codes it gives match the tool results above. It also names N358925 beside the plan, which the text summaries below do not show; the agent also saw each result's `structuredContent`, which this transcript leaves out.

A one-line-per-event replay with timings is in [adk-full-flow.replay.txt](adk-full-flow.replay.txt).

## Transcript

Printed by `run_case.py`. Redacted: the `case_access_key` is replaced with `<redacted key>` by the script; each tool result shows its text content only (the `structuredContent` JSON copy is left out), and the draft is cut at 2,500 characters. The client also logged three error lines on stderr for the timed-out call (`McpError: Request 'tools/call' timed out` and two `TaskGroup` lines when the session closed); they are not repeated here. Nothing else is changed.

```text
[+  3.3s] MCP server https://undelabs.dev/mcp; tools offered to the model: export_ruling_request_draft, extract_product, get_ruling, plan_production_moves, predict_origin, suggest_part_hts
[+  3.3s] User
Product: a 1,800 W handheld electric heat gun for paint stripping. Parts: universal motor and fan made in China; control PCBA made in China; heating element (resistance wire on a mica former) made in China; plastic housing and nozzle moulded in Vietnam; power cord made in Vietnam. The motor-fan unit, PCBA and heating element are assembled into the housing and tested in Vietnam. We also run a plant in Thailand that can do motor-fan assembly, PCBA assembly, heating-element production and final assembly. Family: motorised_tools_small_appliances. Target country for production moves: Thailand. Predict the U.S. origin, plan the moves to make it Thai, and draft the broker request.

[+  6.8s] Tool call: extract_product
{"name": "1,800 W handheld electric heat gun", "facts": "Product: a 1,800 W handheld electric heat gun for paint stripping. Parts: universal motor and fan made in China; control PCBA made in China; heating element (resistance wire on a mica former) made in China; plastic housing and nozzle moulded in Vietnam; power cord made in Vietnam. The motor-fan unit, PCBA and heating element are assembled into the housing and tested in Vietnam. We also run a plant in Thailand that can do motor-fan assembly, PCBA assembly, heating-element production and final assembly.", "family": "motorised_tools_small_appliances"}

[+ 13.0s] Tool result: extract_product
Draft product "1,800 W handheld electric heat gun": 7 parts, 3 plants, 2 operations.
3 follow-up questions (see questions).
Review it, then pass it as product to predict_origin.

Not a ruling. Confirm with a licensed customs broker or a CBP binding ruling.

[+ 21.3s] Tool call: predict_origin
{"product": {"family": "motorised_tools_small_appliances", "plants": [{"id": "plant_in_thailand", "capabilities": ["motor_fan_assembly", "pcba", "heating_element", "final_assembly"], "country": "Thailand", "name": "plant in Thailand"}, {"name": "China plant", "id": "china_plant", "capabilities": ["heating_element", "motor_fan_assembly", "pcba"], "country": "China"}, {"country": "Vietnam", "capabilities": ["cord_knobs_covers", "final_assembly", "housing_moulding"], "id": "vietnam_plant", "name": "Vietnam plant"}], "name": "1,800 W handheld electric heat gun", "components": [{"group": "motor_fan_assembly", "plant": "china_plant", "made_in": "China", "name": "universal motor"}, {"name": "fan", "plant": "china_plant", "group": "motor_fan_assembly", "made_in": "China"}, {"made_in": "China", "group": "pcba", "name": "control PCBA", "plant": "china_plant"}, {"group": "heating_element", "plant": "china_plant", "made_in": "China", "name": "heating element (resistance wire on a mica former)"}, {"name": "plastic housing", "made_in": "Vietnam", "plant": "vietnam_plant", "group": "housing_moulding"}, {"group": "housing_moulding", "name": "nozzle", "plant": "vietnam_plant", "made_in": "Vietnam"}, {"plant": "vietnam_plant", "group": "cord_knobs_covers", "name": "power cord", "made_in": "Vietnam"}], "operations": [{"name": "plastic housing and nozzle moulded", "group": "housing_moulding", "plant": "vietnam_plant"}, {"name": "The motor-fan unit, PCBA and heating element are assembled into the housing and tested", "plant": "vietnam_plant", "group": "final_assembly"}]}, "family": "motorised_tools_small_appliances", "facts": "Product: a 1,800 W handheld electric heat gun for paint stripping. Parts: universal motor and fan made in China; control PCBA made in China; heating element (resistance wire on a mica former) made in China; plastic housing and nozzle moulded in Vietnam; power cord made in Vietnam. The motor-fan unit, PCBA and heating element are assembled into the housing and tested in Vietnam. We also run a plant in Thailand that can do motor-fan assembly, PCBA assembly, heating-element production and final assembly."}

[+ 28.1s] Tool result: predict_origin
Likely origin: China (calibrated confidence 90%; the model stated 95%). Decisive step: Manufacturing of the heating element, motor-fan unit, and PCBA in China.
Triage: auto. calibrated confidence 0.90 ≥ 0.90 (cal-v2; model stated 0.95) and in scope
Cited CBP rulings: N357771, N358092.
Case id: case_b27b37913fd7.
case_access_key returned: pass it with the case_id to later calls on this case. Keep it private; anyone holding it can read the case.

Not a ruling. Confirm with a licensed customs broker or a CBP binding ruling.

[+ 33.9s] Tool call: suggest_part_hts
{"product": {"family": "motorised_tools_small_appliances", "name": "1,800 W handheld electric heat gun", "components": [{"made_in": "China", "group": "motor_fan_assembly", "plant": "china_plant", "name": "universal motor"}, {"name": "fan", "made_in": "China", "plant": "china_plant", "group": "motor_fan_assembly"}, {"made_in": "China", "name": "control PCBA", "plant": "china_plant", "group": "pcba"}, {"group": "heating_element", "plant": "china_plant", "made_in": "China", "name": "heating element (resistance wire on a mica former)"}, {"group": "housing_moulding", "plant": "vietnam_plant", "name": "plastic housing", "made_in": "Vietnam"}, {"plant": "vietnam_plant", "group": "housing_moulding", "made_in": "Vietnam", "name": "nozzle"}, {"name": "power cord", "group": "cord_knobs_covers", "plant": "vietnam_plant", "made_in": "Vietnam"}], "plants": [{"name": "plant in Thailand", "country": "Thailand", "id": "plant_in_thailand", "capabilities": ["motor_fan_assembly", "pcba", "heating_element", "final_assembly"]}, {"id": "china_plant", "name": "China plant", "capabilities": ["heating_element", "motor_fan_assembly", "pcba"], "country": "China"}, {"name": "Vietnam plant", "id": "vietnam_plant", "country": "Vietnam", "capabilities": ["cord_knobs_covers", "final_assembly", "housing_moulding"]}], "operations": [{"group": "housing_moulding", "plant": "vietnam_plant", "name": "plastic housing and nozzle moulded"}, {"name": "The motor-fan unit, PCBA and heating element are assembled into the housing and tested", "plant": "vietnam_plant", "group": "final_assembly"}]}}

[+ 46.6s] Tool result: suggest_part_hts
Suggested HTS codes (unverified; confirm with a licensed customs broker):
The product: 8467.29.00 (rulings: H319727)
universal motor: 8501 (rulings: N348465, N315710, H303140), 8501.10.60 (rulings: H314638)
fan: 8414.59.65 (rulings: N309707)
control PCBA: 8537.10 (rulings: N325337)
heating element (resistance wire on a mica former): 8516.80 (model)
plastic housing: 8467.99 (model), 3926.90 (model)
nozzle: 8467.99 (model), 7326.90 (model)
power cord: 8544.42 (model)
Put a code on a part (components[].hts) only when the user accepts it.

Not a ruling. Confirm with a licensed customs broker or a CBP binding ruling.

[+ 51.7s] Tool call: plan_production_moves
{"case_id": "case_b27b37913fd7", "case_access_key": "<redacted key>", "target_country": "Thailand"}

[+351.7s] Tool result: plan_production_moves
{"error": "MCP tool execution failed: Request 'tools/call' timed out"}

[+359.0s] Tool call: plan_production_moves
{"case_access_key": "<redacted key>", "target_country": "Thailand", "case_id": "case_b27b37913fd7"}

[+462.1s] Tool result: plan_production_moves
Target Thailand: 1 precedent-supported plans, 7 rejected moves.
1. Move the motor-fan assembly, the PCBA (SMT) and the heating element to Thailand; predicted origin Thailand; supported by N357771 (check confidence 96%)

Not a ruling. Confirm with a licensed customs broker or a CBP binding ruling.

[+467.6s] Tool call: export_ruling_request_draft
{"plan_index": 0, "case_access_key": "<redacted key>", "case_id": "case_b27b37913fd7"}

[+468.0s] Tool result: export_ruling_request_draft
U.S. Customs and Border Protection  
National Commodity Specialist Division  
New York, NY

**RE: Request for a binding country of origin ruling — 1,800 W handheld electric heat gun**

Dear Sir or Madam:

Pursuant to 19 C.F.R. Part 177, [REQUESTER NAME] respectfully requests a binding ruling on the country of origin of 1,800 W handheld electric heat gun for purposes of marking and the application of trade remedies, including Section 301 duties.

This request concerns a proposed production setup: move the motor-fan assembly, the PCBA (SMT) and the heating element to Thailand.

## Facts

The merchandise under consideration is a 1,800 W handheld electric heat gun. The 1,800 W handheld electric heat gun is assembled in Vietnam using components sourced from Thailand and Vietnam.

The universal motor, fan, control PCBA and heating element (resistance wire on a mica former) are produced in Thailand. The plastic housing, nozzle and power cord are produced in Vietnam.

In Thailand, the universal motor and fan will be produced in-house: stamp rotor laminations from silicon steel; stack laminations and wind the rotor; build the stator; assemble the motor; press-fit the fan to the motor spindle (assumed from the product-family template; no operations were supplied for this group).

In Thailand, the control PCBA will be produced in-house: SMT placement and reflow of the printed circuit board assembly; test the PCBA (assumed from the product-family template; no operations were supplied for this group).

In Thailand, the heating element (resistance wire on a mica former) will be produced in-house: wind resistance wire around a core fitted with ceramic pipes; insert the core into a mica-insulated metal tube; solder the safety fuse (assumed from the product-family template; no operations were supplied for this group).

Final assembly in Vietnam: The motor-fan unit, PCBA and heating element are assembled into the housing and tested.

## Issue

What is the country of origin of 1,800 W handheld electric heat gun, produced as described above, under the substantial-transformation test?

## Requester's position

The requester submits that the merchandise is a product of Thailand, based on these key factors: the heating element, universal motor-fan assembly, and control PCBA. [REQUESTER TO CONFIRM AND EXPAND WITH COUNSEL.]

## Earlier CBP rulings considered

- CBP ruling N357771 (29 January 2026, a heat gun; CBP held Thailand) — a comparable change where CBP found the origin move
[... 927 more characters cut from this transcript]

[+477.4s] Agent
### 1. Predicted Country of Origin (Current Setup)

* **Predicted Origin:** China
* **Confidence:** 90% calibrated confidence (model-stated: 95%)
* **Decisive Step:** Manufacturing of the core functional subassemblies—the heating element, motor-fan unit, and control PCBA—in China.
* **Cited CBP Rulings:** N357771, N358092
* **Reasoning:** In rulings concerning handheld heat guns and electric fan heaters (such as N357771 and N358092), CBP consistently holds that the essential character is imparted by the functional components that generate and propel heat: the heating element, electric motor-fan unit, and control PCBA. Because all three are manufactured in China, assembling them into Vietnamese-moulded plastic housings with a power cord in Vietnam is considered simple assembly that does not effect a substantial transformation.

---

### 2. Suggested Tariff Classifications (Unverified Suggestions)

*(Note: These are preliminary model suggestions and must be reviewed and verified by a licensed customs broker; they have not been assigned to the product profile.)*

* **Finished Heat Gun:** 8467.29.00 (supported by ruling H319727)
* **Universal Motor:** 8501 (rulings N348465, N315710, H303140) / 8501.10.60 (ruling H314638)
* **Fan:** 8414.59.65 (ruling N309707)
* **Control PCBA:** 8537.10 (ruling N325337)
* **Heating Element (resistance wire on mica former):** 8516.80
* **Plastic Housing:** 8467.99 / 3926.90
* **Nozzle:** 8467.99 / 7326.90
* **Power Cord:** 8544.42

---

### 3. Production Move Plan (Target: Thailand)

* **Best Precedent-Supported Plan:** Move production of the **motor-fan assembly**, **control PCBA (SMT)**, and **heating element** to Thailand.
* **Predicted Origin After Move:** Thailand
* **Check Confidence:** 96%
* **Precedent Support:** CBP ruling **N357771** (supported by **N358092** and **N358925**).
* **Rationale:** In N357771, CBP determined that a handheld heat gun's origin is governed by where its core functional components (heating element, motor, fan, and PCBA) are fabricated from raw or basic materials, rather than where final enclosure assembly occurs. By relocating the full fabrication of the motor-fan unit, PCBA, and heating element to Thailand, the primary origin-conferring components originate in Thailand. The subsequent housing attachment and final testing in Vietnam remain simple assembly, preserving Thai origin.

---

### 4. Broker Ruling Request Draft (Preview)

Below are the opening lines of the draft CBP binding ruling request letter prepared for this proposed plan:

```markdown
U.S. Customs and Border Protection  
National Commodity Specialist Division  
New York, NY

RE: Request for a binding country of origin ruling — 1,800 W handheld electric heat gun

Dear Sir or Madam:

Pursuant to 19 C.F.R. Part 177, [REQUESTER NAME] respectfully requests a binding ruling on the country of origin of 1,800 W handheld electric heat gun for purposes of marking and the application of trade remedies, including Section 301 duties.

This request concerns a proposed production setup: move the motor-fan assembly, the PCBA (SMT) and the heating element to Thailand.

## Facts

The merchandise under consideration is a 1,800 W handheld electric heat gun. The 1,800 W handheld electric heat gun is assembled in Vietnam using components sourced from Thailand and Vietnam...
```

*Note: This letter is an initial draft generated from the factual submission and must be reviewed, completed, and confirmed by a licensed customs broker or legal counsel prior to formal submission to CBP.*

Not a ruling — confirm with a licensed customs broker.

[+477.4s] Gateway-reported model cost: $0.0465 over 7 model calls
```
