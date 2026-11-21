# Google ADK agent calling Unde over MCP (run 21 Nov 2026)

Status: run end to end once, on 21 Nov 2026, against the production endpoint `https://undelabs.dev/mcp`. The agent was a minimal Google ADK `LlmAgent` with an `McpToolset` over `StreamableHTTPConnectionParams`. Its model was Gemini 3.8 Flash through an OpenAI-compatible LiteLLM gateway to Gemini (ADK's `LiteLlm` wrapper), not a Google API key. The agent was given the two read-only tools `search_rulings` and `get_ruling` only, so no case was created and no model call ran on the Unde side. Gateway-reported model cost: $0.0161.

## What we ran

Versions: Python 3.13.9, google-adk 2.14.1, mcp 2.3.0, litellm 1.110.2.

```bash
python3 -m venv venv
./venv/bin/pip install google-adk litellm mcp   # ADK's MCP tools need the mcp package; without it the import fails
export LITELLM_BASE_URL=...   # an OpenAI-compatible LiteLLM gateway to Gemini (not in this repository)
export LITELLM_API_KEY=...
./venv/bin/python -I docs/mcp-transcripts/adk_unde_agent.py
```

The script is [`adk_unde_agent.py`](adk_unde_agent.py). It lists the server's tools with one toolset, then runs the agent with a second toolset filtered to `search_rulings` and `get_ruling`, and prints every tool call and result. With all 13 tools offered, the gateway refused the request with a 400 before any tool ran; filtering to the tools the task needs avoided it. We did not find which tool schema it objected to.

With a Google API key instead of a gateway, use `model="gemini-flash-latest"` as in [docs/MCP.md](../MCP.md) and drop the `LiteLlm` wrapper.

## Transcript

Printed by the script. Redacted: the two `get_ruling` results are cut to their header lines (the full text is CBP's public ruling, with names and addresses that this record does not need); the search result's JSON copy (`structuredContent`) is shortened. Nothing else is changed.

```text
## MCP tools listed by https://undelabs.dev/mcp
- compare_target_countries
- create_broker_review_link
- export_ruling_request_draft
- extract_product
- extract_product_from_media
- get_broker_review_status
- get_case
- get_duty_rates
- get_ruling
- list_cases
- plan_production_moves
- predict_origin
- search_rulings

## Question
Which earlier CBP rulings cover a heat gun assembled in Thailand from a Chinese motor and PCBA? Search with the words: heat gun motor assembled Thailand.

## Tool call
search_rulings {"query": "heat gun motor assembled Thailand"}

## Tool result (search_rulings; the JSON copy in structuredContent is shortened here)
{"content": [{"type": "text", "text": "10 rulings found.\nN357771 (2026-01-29): The country of origin of a heat gun. Held origin: Thailand.\nN359646 (2026-03-26): The country of origin of a caulking gun. Held origin: Vietnam.\nN347556 (2025-05-06): The country of origin of a manual grease gun. Held origin: Vietnam.\nN347272 (2025-04-29): The country of origin of gas furnaces. Held origin: Thailand.\nN347271 (2025-04-29): The country of origin of gas furnaces. Held origin: Thailand.\nN347270 (2025-04-29): The country of origin of gas furnaces. Held origin: Thailand.\nN351922 (2025-08-27): The country of origin of a refrigerant recovery unit. Held origin: China.\nN361873 (2026-06-17): The country of origin of a sump pump. Held origin: Thailand.\nH337371 (2025-04-24): Country of Origin of an Automotive Fuel Pump; Section 301 Trade Remedy. Held origin: Thailand.\nN358925 (2026-03-03): The country of origin of electric heaters. Held origin: Thailand.\n\nNot a ruling. Confirm with a licensed customs broker or a CBP binding ruling."}], "structuredContent": {...same rulings as JSON...}}

## Tool call
get_ruling {"ruling_id": "N357771"}

## Tool result (get_ruling, header lines only)
{"content": [{"type": "text", "text": "N357771 (2026-01-29): The country of origin of a heat gun\nHeld origin: Thailand\nhttps://rulings.cbp.gov/ruling/N357771\n[rest of the ruling text removed from this transcript]"}], ...}

## Tool call
get_ruling {"ruling_id": "N328954"}

## Tool result (get_ruling, header lines only)
{"content": [{"type": "text", "text": "N328954 (2022-11-15): The country of origin of a heat gun.\nHeld origin: none stated\nhttps://rulings.cbp.gov/ruling/N328954\n[rest of the ruling text removed from this transcript]"}], ...}

## Final answer
In NY N328954, CBP determined that a heat gun assembled in Thailand using a Chinese-origin motor, PCBA, and fan was of Chinese origin because the Thai assembly operations were not sufficiently complex to effect a substantial transformation. NY N357771 subsequently addressed this product and distinguished that earlier determination, finding Thai origin only after the motor, PCBA, and other key components were manufactured in Thailand.

## Gateway-reported model cost
$0.0161 over 4 model calls
```

The final answer is the model's text, kept as it came. It reads N357771 correctly as a later ruling on the same heat gun with more Thai parts; it is not a ruling.
