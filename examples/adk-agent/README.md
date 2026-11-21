# Unde as tools for a Google ADK agent

Status: built, and run end to end once against `https://undelabs.dev/mcp` on 22 Nov 2026 ([transcript](../../docs/mcp-transcripts/adk-full-flow.md)).

A [Google Agent Development Kit (ADK)](https://adk.dev) agent that works one U.S. country-of-origin case through Unde's MCP server. It reads a product description, then calls:

1. `extract_product`: turns the description into parts, plants and operations.
2. `predict_origin`: the origin CBP would likely give, with the earlier CBP rulings it cites. This creates a case.
3. `suggest_part_hts`, when the server lists it: unverified HTS codes for the parts.
4. `plan_production_moves`: production moves to a target country that earlier rulings support.
5. `export_ruling_request_draft`: a ruling request draft for a licensed customs broker to review.

The agent is offered only these tools (plus `get_ruling`), through `McpToolset(tool_filter=...)`. Every answer ends with "Not a ruling — confirm with a licensed customs broker."

Files:

- [`unde_agent/agent.py`](unde_agent/agent.py): the agent (`root_agent`), its instruction, the tool filter and the model choice.
- [`run_case.py`](run_case.py): runs one case and prints each tool call and result, with timings, as a transcript.
- [`requirements.txt`](requirements.txt): pinned versions of the committed run.

## Install

Python 3.10 or later.

```bash
cd examples/adk-agent
python3 -m venv .venv
./.venv/bin/pip install -r requirements.txt
```

ADK's MCP tools need the `mcp` package; without it the import fails.

## Run with Gemini (what a real user runs)

With a Gemini API key from Google AI Studio:

```bash
export GOOGLE_API_KEY=...
./.venv/bin/python run_case.py
```

Or on Vertex AI, with Application Default Credentials:

```bash
export GOOGLE_GENAI_USE_VERTEXAI=TRUE
export GOOGLE_CLOUD_PROJECT=your-project
export GOOGLE_CLOUD_LOCATION=us-central1
./.venv/bin/python run_case.py
```

ADK talks to Gemini directly here. The model defaults to `gemini-flash-latest`; set `ADK_MODEL` to change it. We have not run this path; the committed run used the gateway below.

ADK's own runners work too, since `unde_agent` exports `root_agent`: `./.venv/bin/adk run unde_agent` or `./.venv/bin/adk web`.

## Run through an OpenAI-compatible gateway (what we ran)

The committed transcript reached Gemini 3.8 Flash through an OpenAI-compatible LiteLLM gateway to Gemini, with ADK's `LiteLlm` wrapper. When `LITELLM_BASE_URL` is set, the agent uses that path:

```bash
export LITELLM_BASE_URL=...   # your gateway; not in this repository
export LITELLM_API_KEY=...
export ADK_MODEL=gemini-3.8-flash
./.venv/bin/python run_case.py
```

It prints the model cost the gateway reports at the end.

## Options

| Variable | Default | What it does |
|---|---|---|
| `UNDE_MCP_URL` | `https://undelabs.dev/mcp` | The MCP endpoint, e.g. `http://localhost:8080/mcp` for a local server |
| `UNDE_API_KEY` | none | An Unde API key (`op_...`), sent as a bearer token. Without one the agent works anonymously |
| `ADK_MODEL` | `gemini-flash-latest` (`gemini-3.8-flash` on the gateway) | The model name |
| `MAX_RESULT_CHARS` | 2500 | How much of each tool result the transcript prints |

Pass your own case as one argument: `./.venv/bin/python run_case.py "A cordless drill: motor from China, ... Target country: Vietnam."`

Without an API key, each run creates one anonymous case. `predict_origin` returns a `case_access_key`; the agent passes it to the later calls, and the transcript prints `<redacted key>` in its place.

## Limits

- The agent and its answers are a demo, not advice. The origin and the plans are predictions; the draft is for a licensed customs broker to review.
- `plan_production_moves` can only move work to a plant in the target country. The instruction tells the agent to add one when the description names none.
- An OpenAI-compatible gateway refused Unde's full tool list until `predict_origin`'s schema lost its root `anyOf` (D390). `drop_root_composition` in `agent.py` removes such keywords on the client too, so the agent also works with a server that still has them.
