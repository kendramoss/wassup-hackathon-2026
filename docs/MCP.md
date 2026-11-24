# Use Unde from Gemini CLI, ADK agents or Claude (MCP)

Status: built. The Unde API is also a [Model Context Protocol](https://modelcontextprotocol.io) server, so an agent in Gemini CLI, Google ADK or Claude (or any other MCP client) can call it as tools.

- Hosted endpoint (Streamable HTTP): `https://undelabs.dev/mcp`
- Local stdio relay for clients that prefer a local process: `mcp/stdio/` (`undelabs-mcp`)

Every answer is a prediction, not a ruling. Confirm it with a licensed customs broker or a CBP binding ruling. Each tool result says so.

## Set it up

The Gemini CLI and ADK formats below follow their documentation as read on 21 Nov 2026 ([Gemini CLI MCP servers](https://github.com/google-gemini/gemini-cli/blob/main/docs/tools/mcp-server.md), [ADK MCP tools](https://adk.dev/tools-custom/mcp-tools/)). We ran both end to end once on 21 Nov 2026, with the model reached through an OpenAI-compatible LiteLLM gateway to Gemini rather than a Google API key, and the read-only tools `search_rulings` and `get_ruling`: [ADK transcript](mcp-transcripts/adk-search-rulings-2026-11-21.md), [Gemini CLI transcript](mcp-transcripts/gemini-cli-search-rulings-2026-11-21.md). On 22 Nov 2026 an ADK agent ran a whole case through the same endpoint, anonymously: extract, predict, part HTS suggestions, plan and ruling request draft ([full-flow transcript](mcp-transcripts/adk-full-flow.md), D391). The same agent was then scored on 12 products from development-split rulings, once each, with rules written first: 10 of 12 passed every check, no answer named a ruling the tools had not returned, and every answer kept the "Not a ruling" line ([EVALUATION.md §12](EVALUATION.md#12-agent-evaluation-the-adk-agent-on-12-products-22-nov-2026-d450-d451), D450, D451). After the D470 changes it was run again with stricter checks written first: 9 of 12 passed, all 8 runs the live server's anonymous budget did not cut short passed, and the second anonymous run was blocked by production sign-in after the budget reset (D510, D511, D731). Repeatability is unmeasured. The same cases are in ADK's evaluation format (`eval/agent/adk/unde_agent.evalset.json`, run with `adk eval`). They reach the same Streamable HTTP endpoint as Claude Code.

### Gemini CLI

```bash
gemini mcp add --transport http unde https://undelabs.dev/mcp
```

Gemini CLI turns MCP servers off in folders it does not trust; trust the folder first (it asks, or see the transcript). With an API key: `gemini mcp add --transport http --header "Authorization: Bearer op_..." unde https://undelabs.dev/mcp`. Or in `~/.gemini/settings.json` (user) or `.gemini/settings.json` (project):

```json
{
  "mcpServers": {
    "unde": {
      "httpUrl": "https://undelabs.dev/mcp",
      "headers": { "Authorization": "Bearer op_..." }
    }
  }
}
```

Leave out `headers` for the anonymous tier. Gemini CLI's per-server `timeout` defaults to 600,000 ms (10 minutes), which covers a plan search; to match the ADK setting below, add `"timeout": 900000` beside `httpUrl`.

### Google ADK agent (Python)

Install `google-adk` and `mcp` (`pip install google-adk mcp`): ADK's MCP tools import nothing without the `mcp` package. The smallest agent is below. A full trade-compliance agent is [`examples/adk-agent`](../examples/adk-agent/): it filters the toolset to `extract_product`, `predict_origin`, `suggest_part_hts`, `plan_production_moves`, `export_ruling_request_draft` and `get_ruling`, works one case from a product description to a broker draft, and runs on Gemini with a Google API key or Vertex AI, or through an OpenAI-compatible gateway with ADK's `LiteLlm` wrapper (the path we ran). Its README has the exact commands; the run is in [mcp-transcripts/adk-full-flow.md](mcp-transcripts/adk-full-flow.md). The earlier two-tool script is [`mcp-transcripts/adk_unde_agent.py`](mcp-transcripts/adk_unde_agent.py).

```python
from google.adk.agents import LlmAgent
from google.adk.tools.mcp_tool import McpToolset
from google.adk.tools.mcp_tool.mcp_session_manager import StreamableHTTPConnectionParams

root_agent = LlmAgent(
    model="gemini-flash-latest",
    name="origin_planner",
    instruction="Use the Unde tools to find the likely US origin and the production moves that would change it.",
    tools=[
        McpToolset(
            connection_params=StreamableHTTPConnectionParams(
                url="https://undelabs.dev/mcp",
                # With an API key: headers={"Authorization": "Bearer op_..."},
            ),
            # Optional: offer only the tools the task needs.
            # tool_filter=["extract_product", "predict_origin", "plan_production_moves", "export_ruling_request_draft"],
        )
    ],
)
```

Plan search often takes one to several minutes: the recorded heat-gun search took 33 s, the six D661 live checks a median of 49 s and up to 101 s, and the ADK run's first plan call was cut off at 300 s (D391). ADK's default MCP read timeout is 300 s; pass `sse_read_timeout=900` to `StreamableHTTPConnectionParams` so a slow plan call is not cut off and retried (D391). If a client does time out and call again with the same input, the server joins the search still running rather than starting (and paying for) a second one; pass the same `idempotency_key` on the retry to also get a search that already finished (kept 15 minutes, D402). Joining works within one server instance. On the hosted endpoint a plan call that runs past 300 s can still fail once on the server's side; call again with the same input and key and it joins the running search (D422).

### Claude Code

With an API key:

```bash
claude mcp add --transport http unde https://undelabs.dev/mcp --header "Authorization: Bearer op_..."
```

Anonymous (per-address limits, see below):

```bash
claude mcp add --transport http unde https://undelabs.dev/mcp
```

Check it with `claude mcp get unde`, or `/mcp` inside Claude Code.

### Claude.ai and Claude Desktop (custom connector)

Add a custom connector from the connector settings ("Add custom connector"), with the URL `https://undelabs.dev/mcp` and no sign-in. Unde has no OAuth: without a key the connector uses the anonymous tier. If your connector settings offer request headers, you can add `Authorization: Bearer op_...` there to use an API key. Menus differ by plan and change over time; Anthropic's help center has the current steps.

### Claude Desktop with the stdio relay

The relay is published as [`@undelabs/mcp`](https://www.npmjs.com/package/@undelabs/mcp). Run it with `npx -y @undelabs/mcp` — no install and no dependencies beyond Node 20 or later. You can also run it from a clone of this repository with `node mcp/stdio/index.js`.

```json
{
  "mcpServers": {
    "unde": {
      "command": "npx",
      "args": ["-y", "@undelabs/mcp"],
      "env": { "UNDE_API_KEY": "op_..." }
    }
  }
}
```

Or, for Claude Code: `claude mcp add --env UNDE_API_KEY=op_... --transport stdio unde -- npx -y @undelabs/mcp`.

Environment: `UNDE_API_KEY` (optional, `op_...`, sent as a bearer token) and `UNDE_BASE_URL` (default `https://undelabs.dev`; the relay calls `<base>/mcp`). The relay forwards each JSON-RPC message to the hosted endpoint and writes the answers back. It holds no logic of its own, so it always lists the hosted tools.

## API keys

The bearer form (`Authorization: Bearer op_...`) works only on `/mcp`. The REST API at `https://undelabs.dev/api/v1/...` takes the key only as `X-API-Key: op_...`; a bearer header there is dropped ([api/openapi.yaml](api/openapi.yaml)).


Keys (`op_...`) are issued by the Unde admin, not self-service. An admin issues one with `go run ./cmd/apikey issue -name "..." -quota N` (see `backend/cmd/apikey`) or `POST /v1/admin/api-keys` with the admin token. Ask the team for one. A key:

- has its own rate-limit buckets and a daily request quota (each tool call is one request);
- owns the cases it creates: only that key (and the admin) can see them;
- still counts toward the deployment's daily model spend cap.

The admin token is never accepted on `/mcp` (403).

## Tools

Each tool runs exactly one REST operation in-process, so it has that operation's rate-limit class, quota, spend cap, case isolation and audit row. Classes, per client (code defaults; the deployed values are in [api/SECURITY.md](api/SECURITY.md)): basic 120 a minute (no model call), light 20 a minute, heavy 4 a minute, ruling lookup 30 a minute (ruling searches, and rulings fetched from CBP). No tool is unlimited. Tool names follow the OpenAPI `operationId`s in [api/openapi.yaml](api/openapi.yaml).

| Tool | REST operation (operationId) | Limit class | Annotations |
|---|---|---|---|
| `predict_origin` | `POST /v1/cases/predict` (`predictCase`) | light | writes a new case |
| `extract_product` | `POST /v1/products/extract` (`extractProduct`) | light | read-only |
| `extract_product_from_media` | `POST /v1/products/extract-media` (`extractProductFromMedia`) | light | read-only |
| `plan_production_moves` | `POST /v1/cases/{id}/plan` (`planCase`) | heavy | writes the case's plan |
| `compare_target_countries` | `POST /v1/cases/{id}/compare-targets` (`compareTargets`) | heavy | writes the case's comparison |
| `search_rulings` | `GET /v1/rulings?q=` (`searchRulings`) | ruling lookup | read-only |
| `get_ruling` | `GET /v1/rulings/{id}` (`getRuling`) | basic; a fetch from CBP also takes the ruling-lookup bucket | read-only, open world |
| `get_duty_rates` | `GET /v1/duty-rates` (`getDutyRates`) | basic; a live fetch also takes the ruling-lookup bucket | read-only, open world |
| `get_case` | `GET /v1/cases/{id}` (`getCase`) | basic | read-only |
| `list_cases` | `GET /v1/cases` (`listCases`) | basic | read-only |
| `export_ruling_request_draft` | `POST /v1/cases/{id}/export` with `preview: true` (`exportCase`) | basic | read-only (a preview records nothing) |
| `create_broker_review_link` | `POST /v1/cases/{id}/reviews` (`createBrokerReviewLink`) | basic | writes a link on the case (D200) |
| `get_broker_review_status` | `GET /v1/cases/{id}/reviews` (`listBrokerReviewLinks`) | basic | read-only |
| `suggest_part_hts` | `POST /v1/products/parts-hts-suggest` (`suggestPartsHts`) | light | read-only, open world (listed when the server serves the route; on by default, D380) |
| `check_rcep_preference` | `POST /v1/rcep/check` (`checkRcepPreference`) | basic | read-only (no model call, nothing saved; D690) |
| `check_canada_marking` | `POST /v1/rules/canada-marking` (`checkCanadaMarking`) | basic | read-only (listed when the server serves the route; on with the rule check, D720) |
| `check_usmca_origin_rule` | `POST /v1/usmca/check` (`checkUsmcaOriginRule`) | basic | read-only (no model call, nothing saved; D740) |
| `check_eu_origin` | `POST /v1/rules/eu-origin` (`checkEUOrigin`) | basic | read-only (listed when the server serves the route; on with the rule check, D760) |
| `check_uk_origin` | `POST /v1/rules/uk-origin` (`checkUKOrigin`) | basic | read-only (listed when the server serves the route; on with the rule check, D760) |
| `get_evidence_report` | `GET /v1/cases/{id}/evidence-report` (`getEvidenceReport`) | basic | read-only (no model call; the U.S. answer is the stored one, D803) |

`extract_product_from_media` takes 1 to 4 files, 8 MB together (PNG, JPEG, WebP or scanned PDF), each as `{data, mime_type}` with base64 data, as an MCP image content block (`{type: "image", data, mimeType}`), or as an MCP embedded resource (`{type: "resource", resource: {mimeType, blob}}`). The tool decodes them and sends one multipart upload to the REST route, which checks the bytes and its other limits again. As on the website, the files are not stored and the draft must be reviewed before `predict_origin`.

When the product gives an HTS code for the good (`product.hts`) and for at least one part (`components[].hts`), `predict_origin` and `get_case` add one line for the 19 CFR 102.20 rule check (D363), and the structured result carries `rule_check`. The line says when the check differs from the model and that it changes neither the prediction nor the triage.

When CBP has revoked, modified or proposed to change a ruling the case relies on (D480), `get_case` adds a "Changed rulings:" line and the structured result carries `ruling_alerts`; `list_cases` adds a count to that case's line. Each change says whether it bears on the case's answer: the origin the changed ruling held and the one the changing ruling holds, from the shipped labels, against the answer; not known when a holding is not labelled (D531). The saved answer is never changed; a re-check is the ruling watch (`POST /v1/cases/{id}/watch`).

`suggest_part_hts` fills the gap when parts have no codes. Give it the product (send the parts still without a code, at most 20); it returns up to 3 codes per part, and for the product itself when `product.hts` is empty, each labelled `rulings` (an earlier CBP ruling states that code for a similar part, listed with links) or `model` (the model's own reading, no supporting ruling). They are unverified: show them to the user, put a code on a part only when the user accepts it (with the part's other suggested codes in `components[].hts_alternatives`), then call `predict_origin` with the product and its codes. The rule check then also says whether its answer is the same under every suggested code, or which part's code it depends on (`rule_check.robustness`, D430). On the Part 102 dev split the top suggestion matched the code CBP states for the part in 20 of 27 parts (EVALUATION.md §10).

`check_rcep_preference` checks RCEP eligibility between participating markets. It applies the official Annex 3A rules (HS 2022) without a model call (D690). Give the good's HS code, where it is made and its materials (HS code, value, country, and `status`: `originating` only with the supplier's declaration or proof of origin). It returns `yes`, `no` or `undetermined` (missing facts are undetermined, never no), the rule used, RVC both ways, the failing materials and the reasons. The response identifies this check as `regime: rcep_preferential`.

`check_usmca_origin_rule` checks one USMCA product-specific origin rule (HTSUS General Note 11(o)) for a good entering the United States, without a model call (D740). Pilot goods only: 8202.31, 8207.13 and 9503.00; any other code returns `not_supported`. Give `destination: "US"`, `regime: "usmca"`, the good's HS code, the regional value content method you elect (`transaction_value` or `net_cost`, never chosen for you), the matching amount and the value of non-originating materials as decimal strings (JSON numbers are rejected), the materials (HS code and your `originating` claim) and the `profile` confirmations that the simple-goods assumptions hold. It returns `met`, `not_met`, `needs_information` or `not_supported` for the selected rule only, the exact RVC calculation, the missing facts and the source snapshot. An omitted fact is unknown, not zero or originating. It is not a certification of origin, a duty rate or a preferential entitlement, and a `not_met` result is not a legal finding.

`predict_origin` takes an optional `jurisdiction`: the market the goods enter (`us`, `eu`, `uk`, `ca`, `jp`, `kr`, `au`, `mx` or `other`). With none, a `japan` input selects Japan. Otherwise the server reads the market from the facts or defaults to the United States. `jurisdiction_detail` says whether the deployment supports that market. When Japan is available, pass `japan: {"agreement": "RCEP", "hs": "4412.31"}` with the product and its manufacturing facts. `prediction.japan_preference` gives the eligibility outcome, applicable rule and source links. Treat every Japan result as preliminary and confirm it with a licensed Japanese customs specialist. Published test rulings cover only goods that qualified. Production moves and ruling-request drafts are U.S. features.


`check_canada_marking` applies Canada's marking rules for goods imported from the United States or Mexico (SOR/94-23) to a good's HS code and its materials' codes and origins: the Schedule III tariff-shift row, then the general rules (D722). Send a `product` as for `predict_origin`, or `good_hts`, `country` and `materials`. It answers like the U.S. rule check: status, origin, the row applied, the materials that do not shift and plain reasons. It is a rules check only, with no accuracy figure: Canada publishes too few origin rulings to test it against (D725). No model call; nothing is saved.

When the answer is `undetermined`, `structuredContent.guidance` says what would resolve it: yes/no facts the same call can take (`simple_assembly`, `gri_2a`, `bending_to_shape`; leave a field out when unknown, `false` is an answer), codes or shares to add (with the 0-based `material` index when it is about one material), and rules written by goods description that need human review and cannot be answered by a flag. The text adds one short line for each. The tool takes `bending_to_shape`, as the REST route does. Guidance is absent when the answer is decided or out of scope. On the committed Canadian table today, guidance is codes, shares and human review; yes/no facts appear only for a rule that carries such a clause, and none does yet.

`check_eu_origin` and `check_uk_origin` apply the EU's and the UK's non-preferential origin list rules to a good's HS code and its materials' codes and origins, without a model call (D760-D766). EU: Union Customs Code Art. 60 and Delegated Regulation (EU) 2015/2446 Art. 31-36 with the list rules of Annex 22-01. UK: Taxation (Cross-border Trade) Act 2018 s. 17, SI 2020/1433 and the product-specific rules in Part Four of HMRC's reference document v1.2. Both lists have rules for some goods in 37 HS chapters only. Send a `product` as for `predict_origin`, or `good_hts`, `country` and `materials`. The answer is `decided` (with the row or residual rule that decided it), `undetermined` (with `result.needs`: a row to pick, yes/no facts to send back in `answers` under the key the need names, codes or shares to add, or review by a person), or `needs_judgement`: no list rule covers the good, so its origin is where it last underwent substantial, economically justified processing, which a person decides. Do not answer a `needs_judgement` result yourself. The EU rules are common to the EU; each Member State's customs authority applies them and issues Binding Origin Information, which the tool does not read. HMRC publishes no origin rulings. Both are rules checks only, with no accuracy figure. No model call; nothing is saved.

`get_evidence_report` returns a case's evidence report (D803-D805): every rules check the server serves, run separately on the case's product as stored (the U.S. 19 CFR 102.20 tariff shift, Canada's marking rules, EU and UK non-preferential origin, RCEP preference, USMCA preference for the pilot goods), beside the case's stored U.S. answer, which is reused and never re-run. Each check is its own section: the legal question it answers, its outcome, missing information with the document that would settle it, what is not modelled, its sources, rule or data version and limits. The checks answer different legal questions; never combine them into one verdict, score or "overall" answer. The text is the report as Markdown, with the case's facts as stored and any facts the user supplied to the EU or UK check (`PUT /v1/cases/{id}/origin-facts/{market}`, on the website) listed apart. In `check_eu_origin` and `check_uk_origin`, each need now names what would settle it ("settled by: the supplier's bill of materials with values ..."). Nothing is saved. Answers saved before the parts list was replaced, or that no longer fit a part, are listed as not used.

`plan_production_moves` lists each shown plan as `- plan_id <id> (plan_index <n>): ...`. The `plan_id` is the plan's move key (each moved group and its plant, `group@plant`, joined by `+`), returned on every plan and stable across reads; pass it to `export_ruling_request_draft` or `create_broker_review_link`. `plan_index` still works and counts from 0: 0 is the first shown plan. There is no 1-based numbering (D470: an agent drafted "plan 1" as `plan_index` 1 when the list was numbered from 1). When the search shows no plan, the text says "No move is supported by an earlier ruling; do not claim a move changes origin." Call `export_ruling_request_draft` with no `plan_id`, `plan_index` or `move_key` to draft the current setup.

`suggest_part_hts` marks a code whose chapter looks wrong for the product with a `check` note ending "check this code" (D471): a chapter far from the product's family (food, clothing, footwear, jewellery and the like for a part of a machine, appliance, vehicle or device), or a chapter of another material than the part's name states. The code stays in the list; nothing is removed. A deterministic check, not a classification: on the held-out Part 102 suggestion rows it marked 5 of 424 codes, all in one ruling ("nuts" as confectionery and fruit, a temperature transmitter as a food preparation), none on a part with a gold code (EVALUATION.md §12).

`plan_production_moves` adds the rule check of each move (D441) after the move's line, when the case is Part 102-routed or every part has a code: for example "102.20 rule check: Shifts under 102.20 (8543.70)", and the flag's own note in brackets when the move is flagged for a broker: "(an earlier ruling supports this move, but the 102.20 rule gives China: worth a broker's look)" when precedent and the rule disagree (D441), or, on a Part 102 case, "(the engine predicts China for the current setup, but the 102.20 rule gives Vietnam: worth a broker's look)" when the engine and the rule disagree on a row no precedent check ran on (D591, D621). Each plan's structured result carries `rule_check`. It never changes which plans are shown or their order.

No tool deletes anything (`destructiveHint: false` on all). Inputs have closed JSON schemas: capped string lengths (facts up to 12,000 characters, as the API), patterns for dates, ruling numbers and case ids, an enum of the product families the server has templates for, 2 to 4 distinct target countries. A malformed call is refused before any API call. No input schema has `anyOf`, `oneOf`, `allOf` or `not` at its root, because an OpenAI-compatible gateway to Gemini refused the whole tool list over one (D390); `predict_origin`'s "facts or product" rule is checked by the tool instead, still before any API call. Countries are free text with a length and character check, because the API has no country enum.

Results carry a short text summary (with the disclaimer) and `structuredContent`: the API's JSON answer, without bulky internals (verifier search trails, model traces, cost stats, case history and chat; `GET /v1/cases/{id}` over REST has them). An API error becomes a tool error with the status, code and message, and the retry time for a rate limit.

`create_broker_review_link` returns `review_path` (`/review#<token>` on the Unde web app: the token is the URL fragment, which browsers never send to a server) and the token once: Unde keeps only a hash. Whoever holds the link can read that one draft until it expires or is revoked, so share it with the broker only. The broker answers on the web page; `get_broker_review_status` reads the status and comments and never returns a token. Revoking is on the web app or `DELETE /v1/cases/{id}/reviews/{review}`; there is no MCP tool for it.

Resources: `unde://demos` (the demo products) and `unde://demos/{id}` (one demo with ready request bodies). Prompt: `plan_production_move` (product text and a target country; walks extract, predict, plan, draft).

## Anonymous use and the case access key

The hosted server is stateless (several instances serve it), so two MCP requests share no session. Without an API key, `predict_origin` returns a `case_access_key` beside the `case_id`. Pass it to `get_case`, `get_evidence_report`, `plan_production_moves`, `compare_target_countries`, `export_ruling_request_draft`, `create_broker_review_link`, `get_broker_review_status` and `list_cases` to reach that case. It works like the website's visitor cookie: whoever holds it can read and change the cases made with it, so keep it private. Pass the same key to later `predict_origin` calls to keep several cases under one key. With an API key, the key owns the cases and no access key is used.

## Limits

The same as the REST API ([api/openapi.yaml](api/openapi.yaml), top of the file): per-client buckets (by API key, else by the caller's address), light and heavy classes, a separate ruling-lookup bucket, a global cap and the daily spend cap. Over a limit, the tool result is an error with the API's status, code and message, and when to retry: `rate_limited` (429, the class's per-minute limit), `too_many_in_flight` (429, the client already has 4 model calls running), `quota_exceeded` (429, the API key's daily quota, until the next UTC day), `client_budget_reached` (429, this key or address has spent its tenth of the day's model budget, until 00:00 UTC), `auth_rate_limited` (429, too many invalid API keys came from this address; retry when another key may be tried) or `budget_reached` (503, the day's model budget is spent, until 00:00 UTC). Tools without a model call (`get_case`, `list_cases`, `search_rulings`, the review links and the draft preview) keep working past the budget. One MCP request body is capped at 12 MB (room for 8 MB of files in base64). Plan search and target comparison take as long as on the website, often one to several minutes (see the ADK timeout note above).

## Privacy

What you send through a tool is what the REST API receives: product facts, parts and countries are stored on the case, under your API key or access key, as described on the website's privacy page (`/privacy`). Request bodies and queries are never logged; each tool call appears in the server log as an ordinary request line (route, status, latency). The MCP endpoint forwards only your API key and your address; it never forwards cookies or other headers. The MCP client (Gemini CLI, an ADK agent, Claude or another) sees the tool results; what its provider does with them is governed by your terms with that provider, not by Unde.

## How it is built

- `backend/internal/mcpserver`: the MCP server (official Go SDK `github.com/modelcontextprotocol/go-sdk`, Streamable HTTP, stateless). Tools build one REST request each and dispatch it to the API's own handler chain in-process (`httpapi.(*api).mcp`), with only the caller's API key, client address and access key. The admin and user tokens never reach the API from there: the MCP guard drops their headers before the handler, and the API's `withUser` middleware also skips `/mcp`, so a signed-in identity cannot reach a tool call through the request context either. An MCP caller is an API key or anonymous, even if it sends a valid `X-User-Token` (#385). As defence in depth, each tool call's `/v1` request is also built on a context that keeps the MCP request's cancellation but none of its values, so no identity, key or audit state the API's middleware put on `POST /mcp` can reach a tool call (D280). A broker review token sent to `/mcp` is dropped.
- `POST /mcp` on the backend (registered in `httpapi/server.go`).
- `frontend/src/app/mcp/route.ts` and `frontend/src/lib/mcp-proxy.ts`: the public route. The backend is private on Cloud Run, so the frontend proxies `/mcp` as it proxies `/api/v1`. It forwards the MCP headers, maps `Authorization: Bearer op_...` to `X-API-Key`, sets `X-Client-IP`, refuses the admin token, and streams the response through (an MCP answer can be a `text/event-stream`).
- `mcp/stdio/index.js`: the stdio relay.

Why in the Go backend rather than in Next.js: tools then share the API's rate limits, quotas, spend cap, isolation and audit natively, instead of re-implementing them, and the proxy stays a thin pass-through.

Adding a tool is one `add(...)` call and one handler in `internal/mcpserver` (`extract_product_from_media` in `media.go` is an example). A tool is listed only when its REST route is served.
