# Google ADK agent running a full Unde case over MCP (runs of 22 Nov 2026)

Status: run end to end twice on 22 Nov 2026 against the production endpoint `https://undelabs.dev/mcp`, anonymously (no Unde API key). This page shows the second run (D422), made after the server joined duplicate plan searches (D402) and the agent's MCP read timeout was raised to 900 s. The first run (D391) is summarised at the end. The agent is [`examples/adk-agent`](../../examples/adk-agent/): a Google ADK `LlmAgent` with an `McpToolset` over `StreamableHTTPConnectionParams`, filtered to the tools a case needs. Its model was Gemini 3.8 Flash through an OpenAI-compatible LiteLLM gateway to Gemini (ADK's `LiteLlm` wrapper), not a Google API key. Gateway-reported model cost for the agent: $0.0446 over 7 model calls. The tools' own model calls ran on Unde's side, under its usual limits, and are not in that figure. It created one case.

## What we ran

Versions: Python 3.13.9, google-adk 2.14.1, mcp 2.3.0, litellm 1.110.2 ([requirements.txt](../../examples/adk-agent/requirements.txt)).

```bash
cd examples/adk-agent
python3 -m venv .venv && ./.venv/bin/pip install -r requirements.txt
export LITELLM_BASE_URL=...   # an OpenAI-compatible LiteLLM gateway to Gemini (not in this repository)
export LITELLM_API_KEY=...
export ADK_MODEL=gemini-3.8-flash
./.venv/bin/python run_case.py
```

Changes to the agent since the first run: the instruction asks for an `idempotency_key` on `plan_production_moves` ("<case_id>-plan-<target>") and to retry a timed-out call with the same input and key; the client-side `drop_root_composition` callback is gone, because the live server no longer has a root `anyOf` (D390), so every tool schema went to the gateway as the server lists it.

The product is the same heat gun as the first run: motor, fan, PCBA and heating element from China, housing, nozzle and cord from Vietnam, assembled in Vietnam, with a plant in Thailand that could take the motor-fan, PCBA, heating-element and final-assembly work. The target country is Thailand.

## What the agent did

1. `extract_product` drafted 7 parts, 3 plants and 3 operations, with a warning that the text does not say whether the Thai plant is used for this product.
2. `predict_origin` predicted China (calibrated confidence 90%, model stated 95%), triage `auto`, citing N357771 and N358092, and created case `case_f735ea3da7eb`. The agent passed the returned `case_access_key` to every later call.
3. `suggest_part_hts` suggested codes for the product and each part, each marked as from an earlier ruling or the model's own reading. The agent reported them as unverified and did not put them on the product.
4. `plan_production_moves` to Thailand, with `idempotency_key` `case_f735ea3da7eb-plan-Thailand`. The first call came back after 300 s as an error from the server ("Server returned an error response"), not as the client's read timeout, which is now 900 s. The agent called again with the same input and key, and that call answered 29 s later: 2 precedent-supported plans (move the motor-fan assembly, the PCBA and the heating element, check confidence 96%; or the motor-fan assembly and the heating element, 92%; both predicted Thailand, supported by N357771) and 6 rejected moves.
5. `export_ruling_request_draft` for the first plan returned a 19 CFR Part 177 request draft with placeholders for the requester.

The final answer ends with "Not a ruling — confirm with a licensed customs broker." It is the model's text, kept as it came. Its reasoning paragraphs paraphrase the tool results in the model's words and were not checked line by line; the origin, confidence, plans and codes it gives match the tool results above. It also names N358925 beside the plan, which the text summaries below do not show; the agent also saw each result's `structuredContent`, which this transcript leaves out.

## One plan search, two calls

The search ran once. The second call carried the same idempotency key and answered 29 s after it was sent, while the search had already been running for more than 300 s when the first call failed. That fits D402: the retry joined the search still running, and no second search was started. We infer this from the timings; we did not read the server's logs.

The call is still repeated, because the first one failed at 300 s on the server's side. The public `/mcp` endpoint is served through the website's Cloud Run service, which proxies to the API. The deploy sets a 900 s request timeout on the API service only, so the website service most likely keeps Cloud Run's default of 300 s and ends the request there. Raising it is a deploy change and is not made here; until then an MCP plan call that runs past 300 s fails once and the retry joins the running search.

A one-line-per-event replay with timings is in [adk-full-flow.replay.txt](adk-full-flow.replay.txt).

## Transcript

Printed by `run_case.py`. Redacted: the `case_access_key` is replaced with `<redacted key>` by the script; each tool result shows its text content only (the `structuredContent` JSON copy is left out), and the draft is cut at 2,500 characters. The client also logged one error line on stderr for the failed call (`MCP tool execution failed with McpError: Server returned an error response`); it is not repeated here. Nothing else is changed.

```text
[+  2.7s] MCP server https://undelabs.dev/mcp; tools offered to the model: export_ruling_request_draft, extract_product, get_ruling, plan_production_moves, predict_origin, suggest_part_hts
[+  2.7s] User
Product: a 1,800 W handheld electric heat gun for paint stripping. Parts: universal motor and fan made in China; control PCBA made in China; heating element (resistance wire on a mica former) made in China; plastic housing and nozzle moulded in Vietnam; power cord made in Vietnam. The motor-fan unit, PCBA and heating element are assembled into the housing and tested in Vietnam. We also run a plant in Thailand that can do motor-fan assembly, PCBA assembly, heating-element production and final assembly. Family: motorised_tools_small_appliances. Target country for production moves: Thailand. Predict the U.S. origin, plan the moves to make it Thai, and draft the broker request.

[+  7.4s] Tool call: extract_product
{"family": "motorised_tools_small_appliances", "name": "1,800 W handheld electric heat gun", "facts": "Product: a 1,800 W handheld electric heat gun for paint stripping. Parts: universal motor and fan made in China; control PCBA made in China; heating element (resistance wire on a mica former) made in China; plastic housing and nozzle moulded in Vietnam; power cord made in Vietnam. The motor-fan unit, PCBA and heating element are assembled into the housing and tested in Vietnam. We also run a plant in Thailand that can do motor-fan assembly, PCBA assembly, heating-element production and final assembly."}

[+ 15.9s] Tool result: extract_product
Draft product "1,800 W handheld electric heat gun": 7 parts, 3 plants, 3 operations.
Warning: The text mentions a plant in Thailand capable of motor-fan assembly, PCBA assembly, heating-element production, and final assembly, but does not state whether it is used for the subject merchandise as an alternative scenario.
3 follow-up questions (see questions).
Review it, then pass it as product to predict_origin.

Not a ruling. Confirm with a licensed customs broker or a CBP binding ruling.

[+ 24.1s] Tool call: predict_origin
{"family": "motorised_tools_small_appliances", "product": {"plants": [{"name": "China plant", "capabilities": ["heating_element", "motor_fan_assembly", "pcba"], "country": "China", "id": "china_plant"}, {"capabilities": ["cord_knobs_covers", "final_assembly", "housing_moulding"], "id": "vietnam_plant", "name": "Vietnam plant", "country": "Vietnam"}, {"capabilities": ["motor_fan_assembly", "pcba", "heating_element", "final_assembly"], "name": "plant in Thailand", "id": "plant_in_thailand", "country": "Thailand"}], "name": "1,800 W handheld electric heat gun", "family": "motorised_tools_small_appliances", "operations": [{"name": "mould plastic housing and nozzle", "group": "housing_moulding", "plant": "vietnam_plant"}, {"name": "assemble motor-fan unit, PCBA and heating element into the housing and test", "group": "final_assembly", "plant": "vietnam_plant"}], "components": [{"group": "motor_fan_assembly", "name": "universal motor", "plant": "china_plant", "made_in": "China"}, {"plant": "china_plant", "group": "motor_fan_assembly", "name": "fan", "made_in": "China"}, {"name": "control PCBA", "group": "pcba", "made_in": "China", "plant": "china_plant"}, {"plant": "china_plant", "name": "heating element (resistance wire on a mica former)", "made_in": "China", "group": "heating_element"}, {"plant": "vietnam_plant", "group": "housing_moulding", "name": "plastic housing", "made_in": "Vietnam"}, {"name": "nozzle", "group": "housing_moulding", "made_in": "Vietnam", "plant": "vietnam_plant"}, {"made_in": "Vietnam", "plant": "vietnam_plant", "name": "power cord", "group": "cord_knobs_covers"}]}, "facts": "Product: a 1,800 W handheld electric heat gun for paint stripping. Parts: universal motor and fan made in China; control PCBA made in China; heating element (resistance wire on a mica former) made in China; plastic housing and nozzle moulded in Vietnam; power cord made in Vietnam. The motor-fan unit, PCBA and heating element are assembled into the housing and tested in Vietnam. We also run a plant in Thailand that can do motor-fan assembly, PCBA assembly, heating-element production and final assembly."}

[+ 28.2s] Tool result: predict_origin
Likely origin: China (calibrated confidence 90%; the model stated 95%). Decisive step: Manufacturing of the heating element, motor-fan unit, and PCBA in China.
Triage: auto. calibrated confidence 0.90 ≥ 0.90 (cal-v2; model stated 0.95) and in scope
Cited CBP rulings: N357771, N358092.
Case id: case_f735ea3da7eb.
case_access_key returned: pass it with the case_id to later calls on this case. Keep it private; anyone holding it can read the case.

Not a ruling. Confirm with a licensed customs broker or a CBP binding ruling.

[+ 32.1s] Tool call: suggest_part_hts
{"product": {"name": "1,800 W handheld electric heat gun", "description": "1,800 W handheld electric heat gun for paint stripping", "family": "motorised_tools_small_appliances", "components": [{"group": "motor_fan_assembly", "made_in": "China", "plant": "china_plant", "name": "universal motor"}, {"name": "fan", "plant": "china_plant", "group": "motor_fan_assembly", "made_in": "China"}, {"made_in": "China", "name": "control PCBA", "plant": "china_plant", "group": "pcba"}, {"group": "heating_element", "made_in": "China", "plant": "china_plant", "name": "heating element (resistance wire on a mica former)"}, {"made_in": "Vietnam", "name": "plastic housing", "group": "housing_moulding", "plant": "vietnam_plant"}, {"made_in": "Vietnam", "group": "housing_moulding", "plant": "vietnam_plant", "name": "nozzle"}, {"plant": "vietnam_plant", "name": "power cord", "group": "cord_knobs_covers", "made_in": "Vietnam"}]}}

[+ 44.7s] Tool result: suggest_part_hts
Suggested HTS codes (unverified; confirm with a licensed customs broker):
The product: 8467.29.00 (rulings: H319727)
universal motor: 8501 (rulings: N348465, N315710, H303140), 8501.20 (model), 8501.10.60 (rulings: H314638)
fan: 8414.59.65 (rulings: N309707), 8414.59 (model)
control PCBA: 8537.10 (rulings: N325337), 8467.99 (model)
heating element (resistance wire on a mica former): 8516.80 (model)
plastic housing: 8467.99 (model), 3926.90 (model)
nozzle: 8467.99 (model), 7326.90 (model)
power cord: 8544.42 (model)
Put a code on a part (components[].hts) only when the user accepts it.

Not a ruling. Confirm with a licensed customs broker or a CBP binding ruling.

[+ 48.1s] Tool call: plan_production_moves
{"target_country": "Thailand", "case_id": "case_f735ea3da7eb", "case_access_key": "<redacted key>", "idempotency_key": "case_f735ea3da7eb-plan-Thailand"}

[+348.4s] Tool result: plan_production_moves
{"error": "MCP tool execution failed: Server returned an error response"}

[+355.2s] Tool call: plan_production_moves
{"target_country": "Thailand", "case_access_key": "<redacted key>", "case_id": "case_f735ea3da7eb", "idempotency_key": "case_f735ea3da7eb-plan-Thailand"}

[+384.6s] Tool result: plan_production_moves
Target Thailand: 2 precedent-supported plans, 6 rejected moves.
1. Move the motor-fan assembly, the PCBA (SMT) and the heating element to Thailand; predicted origin Thailand; supported by N357771 (check confidence 96%)
2. Move the motor-fan assembly and the heating element to Thailand; predicted origin Thailand; supported by N357771 (check confidence 92%)

Not a ruling. Confirm with a licensed customs broker or a CBP binding ruling.

[+389.7s] Tool call: export_ruling_request_draft
{"case_id": "case_f735ea3da7eb", "case_access_key": "<redacted key>", "plan_index": 0}

[+390.1s] Tool result: export_ruling_request_draft
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

Final assembly in Vietnam: assemble motor-fan unit, PCBA and heating element into the housing and test.

## Issue

What is the country of origin of 1,800 W handheld electric heat gun, produced as described above, under the substantial-transformation test?

## Requester's position

The requester submits that the merchandise is a product of Thailand, based on these key factors: heating element, universal motor/fan unit, and control PCBA. [REQUESTER TO CONFIRM AND EXPAND WITH COUNSEL.]

## Earlier CBP rulings considered

- CBP ruling N357771 (29 January 2026, a heat gun; CBP held Thailand) — a comparable change where CBP found the origin moved ([rulings.cbp.gov
[... 908 more characters cut from this transcript]

[+399.9s] Agent
### 1. Current Origin Prediction

* **Predicted Country of Origin:** **China**
* **Confidence:** Calibrated confidence **90%** (model stated confidence: 95%)
* **Decisive Step:** Manufacturing of the core functional heating element, electric motor-fan unit, and control PCBA in China.
* **CBP Rulings Cited:** **N357771**, **N358092**
* **Reasoning:** Under CBP precedent for heat guns and forced-air heaters, the essential character is imparted by the core electro-mechanical components that generate and propel hot air (heating element, motor-fan unit, and control PCBA). Because all three are produced in China, simple final assembly and moulding of the housing/cord in Vietnam does not effect a substantial transformation.

---

### 2. Tariff Classification Suggestions (Unverified)

The following HTS codes are suggested for reference only (confirm with a licensed customs broker before adding to any entry or BOM):

* **Finished Product (Heat Gun):** `8467.29.00` (supported by ruling H319727)
* **Universal Motor:** `8501` (rulings: N348465, N315710, H303140), `8501.20` (model), `8501.10.60` (ruling: H314638)
* **Fan:** `8414.59.65` (ruling: N309707), `8414.59` (model)
* **Control PCBA:** `8537.10` (ruling: N325337), `8467.99` (model)
* **Heating Element:** `8516.80` (model)
* **Plastic Housing:** `8467.99` (model), `3926.90` (model)
* **Nozzle:** `8467.99` (model), `7326.90` (model)
* **Power Cord:** `8544.42` (model)

---

### 3. Production Move Plan to Achieve Thai Origin

* **Best Supported Move:** Move the **motor-fan assembly**, the **control PCBA (SMT)**, and the **heating element** to the Thailand plant.
* **Predicted Origin After Move:** **Thailand** (check confidence: **96%**)
* **Supporting Precedents:** **N357771** (also supported by **N358925**, **N358092**)
* **Legal Rationale:** In ruling **N357771**, CBP determined that the heating element, electric motor, PCBA, and fan collectively impart the essential character and functional use of a heat gun (delivering directed hot air). Shifting the complex manufacturing of these subassemblies (stamping/winding the motor, SMT placement/reflow of the PCBA, and winding/insulating the heating element) to Thailand establishes Thailand as the country of origin, as downstream assembly into the housing in Vietnam remains a simple assembly that does not substantially transform those Thai core components.

*(Note: Moving only the motor-fan assembly and heating element to Thailand was also precedent-supported under N357771 with 92% confidence, while moving fewer components remained origin China).*

---

### 4. Binding Ruling Request Draft (Preview)

Below are the opening lines of the generated ruling request letter for review by a customs broker:

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

*This is an automated preliminary draft prepared for a licensed customs broker or trade counsel to review, complete, and verify before submission to CBP.*

Not a ruling — confirm with a licensed customs broker.

[+399.9s] Gateway-reported model cost: $0.0446 over 7 model calls
```

## The first run (D391)

Same product and model, before D402 and with ADK's default 300 s read timeout and the client-side schema callback. It predicted China (calibrated 90%), citing N357771 and N358092, created case `case_b27b37913fd7` and found 1 precedent-supported plan to Thailand with 7 rejected moves. Its first plan call timed out on the client at 300 s and the agent called again; the second call answered after 103 s, and the server may have run the plan search twice. Gateway-reported model cost $0.0465 over 7 model calls. Its transcript is in this file's history (`git log -p docs/mcp-transcripts/adk-full-flow.md`).
