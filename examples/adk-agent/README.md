# Unde as tools for a Google ADK agent

Status: built. Run end to end against `https://undelabs.dev/mcp` on 22 Nov 2026 ([transcript](../../docs/mcp-transcripts/adk-full-flow.md)), then scored on 12 products, once each: 10 of 12 passed every check; after fixes, 9 of 12 under stricter checks (see [Evaluation](#evaluation)).

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
- [`run_eval.py`](run_eval.py): runs one product of the evaluation set and appends one row for the scorer (see Evaluation).
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

## Evaluation

Built and run once per product on 22 Nov 2026 (D450, D451; [EVALUATION.md §12](../../docs/EVALUATION.md#12-agent-evaluation-the-adk-agent-on-12-products-22-nov-2026-d450-d451)). The set is [`eval/agent/products.jsonl`](../../eval/agent/products.jsonl): 14 products from development-split CBP rulings, each with a description, a family, a target country and CBP's origin. [`eval/agent/score.py`](../../eval/agent/score.py) checks tool order, arguments, that the answer states the origin `predict_origin` returned, that it cites only rulings the tools returned, and the "Not a ruling" line. Scores are in [`eval/agent/SUMMARY.md`](../../eval/agent/SUMMARY.md).

On 12 products against `undelabs.dev`, Gemini 3.8 Flash through the gateway: 10 of 12 passed every check, no answer named a ruling the tools had not returned, $0.0394 of agent model cost a run on average, median 313 s a run. The two misses and what reading the answers found are in that section.

After the D470 fixes (plan_id, the no-plan line, one draft), a second evaluation with stricter checks written first (D510) ran against the live server: 9 of 12 passed. The 8 runs the server's anonymous per-client budget did not cut short all passed; a09 to a12 were cut by it (429), and the second run per product, for repeatability, has not run yet (D511).

The [D732 authenticated follow-up](../../docs/DECISIONS.md#d732--pre-registration-for-an-authenticated-live-adk-evaluation-683-23-nov-2026) preserves the historical anonymous rows. After the preregistration is pushed and an approved scoped key is injected into the process as `UNDE_API_KEY`, use the gateway variables above and run each scheduled fixture once. From `examples/adk-agent`, the single-fixture command is:

```bash
./.venv/bin/python run_eval.py --require-bearer --id a01 \
  --label live-v2-authenticated-run1 --out ../../eval/agent/results/live-v2-authenticated-run1.jsonl
```

See D732 for batch size, spend checks and the separate result metadata.

Plumbing, with no model and no cost: start a local server in fake mode (`LLM_PROVIDER=fake make run`), then

```bash
UNDE_MCP_URL=http://localhost:8080/mcp ./.venv/bin/python run_eval.py --id a01 --scripted --out /tmp/plumbing.jsonl
```

ADK's own evaluator can run the same 12 cases (D510). The eval set and config are written by `eval/agent/score.py`. `adk eval` needs ADK's eval extras, which also move `litellm` to the version they pin:

```bash
./.venv/bin/pip install "google-adk[eval]==2.14.1"
# from the repository root, with the model variables above set
examples/adk-agent/.venv/bin/adk eval examples/adk-agent/unde_agent \
  eval/agent/adk/unde_agent.evalset.json \
  --config_file_path eval/agent/adk/test_config.json \
  --eval_storage_uri file:///tmp/unde-adk-eval --print_detailed_results
```

It scores `tool_trajectory_avg_score`: the four required tools in order, other tools allowed between, arguments not compared (case ids and keys change every run), two runs per case. Each case makes one anonymous case on the server and may run one plan search, so 24 in all. We have loaded it in `adk eval` but not run it live (D511). `eval/agent/adk/replay_trajectory.py` applies the same metric to the recorded rows offline, with no model.

A scored run uses the gateway variables above and `--out ../../eval/agent/results/<run>.jsonl`; then `python3 -I -B eval/agent/score.py .` from the repository root. Each run makes one case and one plan search.

## Limits

- The agent and its answers are a demo, not advice. The origin and the plans are predictions; the draft is for a licensed customs broker to review.
- `plan_production_moves` can only move work to a plant in the target country. The instruction tells the agent to add one when the description names none.
- An OpenAI-compatible gateway refused Unde's full tool list until `predict_origin`'s schema lost its root `anyOf` (D390). The live server has that fix, so the agent no longer strips such keywords on the client (D422). Against an older server built before D390, the gateway path fails with a 400.
