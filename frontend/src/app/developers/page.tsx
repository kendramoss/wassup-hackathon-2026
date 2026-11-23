import type { Metadata } from "next";
import type { ReactNode } from "react";
import { AppBar, Main } from "@/components/op/chrome";
import { Fold } from "@/components/op/ui";
import { copy } from "@/lib/copy";

export const metadata: Metadata = { title: "API and MCP" };

const spec = `${copy.landing.repo}/blob/main/docs/api/openapi.yaml`;
const mcpDoc = `${copy.landing.repo}/blob/main/docs/MCP.md`;
const relay = `${copy.landing.repo}/tree/main/mcp/stdio`;
const security = `${copy.landing.repo}/blob/main/docs/api/SECURITY.md#as-built-vs-as-deployed-21-nov-2026`;

// Config formats as Gemini CLI's MCP guide (docs/tools/mcp-server.md, read
// 21 Nov 2026) and ADK's MCP tools page (adk.dev/tools-custom/mcp-tools) give
// them. Only formats read in those docs are shown (judge panel round 4).
const geminiDocs = "https://github.com/google-gemini/gemini-cli/blob/main/docs/tools/mcp-server.md";
const adkDocs = "https://adk.dev/tools-custom/mcp-tools/";
const geminiConfig = `{
  "mcpServers": {
    "unde": { "httpUrl": "https://undelabs.dev/mcp" }
  }
}`;
const geminiAdd = `gemini mcp add --transport http unde https://undelabs.dev/mcp`;
const geminiSettings = `{
  "mcpServers": {
    "unde": {
      "httpUrl": "https://undelabs.dev/mcp",
      "headers": { "Authorization": "Bearer op_…" }
    }
  }
}`;
// The toolset and model lines of examples/adk-agent/unde_agent/agent.py,
// which ran whole cases against the live server (D391, D422).
const adkAgent = `from google.adk.agents import LlmAgent
from google.adk.tools.mcp_tool import McpToolset
from google.adk.tools.mcp_tool.mcp_session_manager import StreamableHTTPConnectionParams

root_agent = LlmAgent(
    model="gemini-flash-latest",  # Google API key or Vertex AI
    name="unde_trade_compliance",
    instruction=INSTRUCTION,  # extract, predict, plan, draft; "Not a ruling"
    tools=[
        McpToolset(
            connection_params=StreamableHTTPConnectionParams(
                url="https://undelabs.dev/mcp",
                sse_read_timeout=900,  # plan search can take minutes
                # With an API key: headers={"Authorization": "Bearer op_…"},
            ),
            tool_filter=["extract_product", "predict_origin", "suggest_part_hts",
                         "plan_production_moves", "export_ruling_request_draft", "get_ruling"],
        )
    ],
)`;
const adkExample = `${copy.landing.repo}/tree/main/examples/adk-agent`;
const adkTranscript = `${copy.landing.repo}/blob/main/docs/mcp-transcripts/adk-full-flow.md`;
// docs/EVALUATION.md §12, eval/agent/summary.json (D450, D451).
const adkEval = `${copy.landing.repo}/blob/main/docs/EVALUATION.md#12-agent-evaluation-the-adk-agent-on-12-products-22-nov-2026-d450-d451`;
const desktopRelay = `{
  "mcpServers": {
    "unde": {
      "command": "npx",
      "args": ["-y", "@undelabs/mcp"],
      "env": { "UNDE_API_KEY": "op_…" }
    }
  }
}`;
const mcpAdd = `claude mcp add --transport http unde https://undelabs.dev/mcp`;
const mcpAddKey = `claude mcp add --transport http unde https://undelabs.dev/mcp \\
  --header "Authorization: Bearer op_…"`;

// Per-client rate classes (docs/MCP.md "Classes", openapi.yaml top): basic
// has no model call; lookup is the ruling-lookup bucket.
const RATE = { basic: "120 a minute", light: "20 a minute", heavy: "4 a minute", lookup: "30 a minute" } as const;

// The MCP tools, in the order docs/MCP.md lists them, with that table's limit
// class; each runs one REST operation with that operation's limits
// (internal/mcpserver). scripts/developers-copy.test.mjs checks names and
// classes against docs/MCP.md, which api_docs_test checks against server.go.
const tools: [string, string, keyof typeof RATE][] = [
  ["predict_origin", "Predict where CBP would say a product is from. Starts a case.", "light"],
  ["extract_product", "Turn a description into a product to check.", "light"],
  ["extract_product_from_media", "Read a photo or scan of a parts list.", "light"],
  ["plan_production_moves", "Find production moves that change the origin, each checked against earlier rulings.", "heavy"],
  ["compare_target_countries", "Plan for 2 to 4 countries and rank them.", "heavy"],
  ["search_rulings", "Search the CBP rulings Unde reads.", "lookup"],
  ["get_ruling", "Fetch one CBP ruling.", "basic"],
  ["get_duty_rates", "Duty rates for a tariff code.", "basic"],
  ["get_case", "Read a case.", "basic"],
  ["list_cases", "List your cases and the demos.", "basic"],
  ["export_ruling_request_draft", "Preview the ruling request for a broker.", "basic"],
  ["create_broker_review_link", "Make a link your broker opens to approve or comment.", "basic"],
  ["get_broker_review_status", "Read the broker's answer.", "basic"],
  ["suggest_part_hts", "Suggest a tariff code for each part.", "light"],
  ["check_rcep_preference", "Check if a good qualifies for the RCEP tariff. Rules only.", "basic"],
  ["check_canada_marking", "Apply Canada's marking rules to goods from the U.S. or Mexico. Rules only.", "basic"],
];

// The limits a caller can hit, with the error each returns (openapi.yaml top,
// docs/MCP.md "Limits").
const limits: [string, string, ReactNode][] = [
  ["Reads and other calls with no model", RATE.basic, <C key="r">429 rate_limited</C>],
  ["Model calls (predict, extract, …)", RATE.light, <C key="r">429 rate_limited</C>],
  ["Plan search, compare countries", RATE.heavy, <C key="r">429 rate_limited</C>],
  ["Model calls running at once", "4", <C key="f">429 too_many_in_flight</C>],
  ["Requests per key per UTC day", "set per key", <C key="q">429 quota_exceeded</C>],
  ["One key's share of the day's model budget", "a tenth", <C key="c">429 client_budget_reached</C>],
  ["The day's model budget", "shared", <C key="b">503 budget_reached</C>],
];

// The MCP one-liners wrap on a phone so the whole command shows.
const wrap = " whitespace-pre-wrap [overflow-wrap:anywhere]";
const pre = "overflow-x-auto border border-edge bg-white px-4 py-3 font-mono text-[12.5px] leading-relaxed shadow-[3px_3px_0_var(--shadow)]";

// A code sample that can scroll sideways on a phone is a named, focusable
// region, so a keyboard user can tab to it and scroll it with the arrow
// keys; the global :focus-visible outline shows where focus is (#517). A
// one-liner that wraps never scrolls and stays out of the tab order.
function Code({ label, wraps, children }: { label: string; wraps?: boolean; children: string }) {
  if (wraps) {
    return (
      <pre className={pre + wrap}>
        <code>{children}</code>
      </pre>
    );
  }
  return (
    <pre role="region" aria-label={label} tabIndex={0} className={pre}>
      <code>{children}</code>
    </pre>
  );
}

const example = `curl -X POST https://undelabs.dev/api/v1/cases/predict \\
  -H "X-API-Key: op_…" \\
  -H "Content-Type: application/json" \\
  -d '{"date": "2026-06-01", "facts": "Each heat gun is assembled in Thailand from 24 components…"}'`;

// Without a key the proxy keys cases to the op_visitor cookie it sets on the
// first response (lib/visitor.ts, lib/backend.ts); a caller's own
// X-Visitor-ID is not forwarded. A cookie jar keeps the case reachable.
const keyless = `curl -c jar -b jar -X POST https://undelabs.dev/api/v1/cases/predict \\
  -H "Content-Type: application/json" \\
  -d '{"date": "2026-06-01", "facts": "Each heat gun is assembled in Thailand from 24 components…"}'`;
const keylessRead = `curl -c jar -b jar https://undelabs.dev/api/v1/cases/<case_id>`;

const link = "text-ok underline underline-offset-2";

function C({ children }: { children: ReactNode }) {
  return <code className="font-mono text-[12.5px]">{children}</code>;
}

function Details({ title = "Details", children }: { title?: string; children: ReactNode }) {
  return (
    <div className="mt-1">
      <Fold title={title}>
        <div className="read flex max-w-[72ch] flex-col gap-2.5 text-[12.5px] leading-snug text-dim [&_li]:leading-snug">{children}</div>
      </Fold>
    </div>
  );
}

function Step({ n, title, children }: { n: number; title: string; children: ReactNode }) {
  return (
    <li className="grid grid-cols-[28px_minmax(0,1fr)] gap-x-3 gap-y-1.5">
      <span aria-hidden="true" className="flex h-6 w-6 items-center justify-center border border-edge font-mono text-[12px]">
        {n}
      </span>
      <h3 className="self-center text-[14px] font-medium">{title}</h3>
      <div className="col-start-2 flex min-w-0 flex-col gap-1.5">{children}</div>
    </li>
  );
}

const row = "grid gap-x-4 gap-y-0.5 border-b border-line py-1.5";

// How a broker calls the API with a key (D69). Keys are issued by the admin;
// the browser never manages them, so this page only documents their use.
export default function Page() {
  return (
    <>
      <AppBar active="api" />
      <Main className="flex max-w-[860px] flex-col gap-8">
        <div>
          <p className="eyebrow">API · MCP</p>
          <h1 className="mt-1 text-[28px] tracking-[-0.05em]">Use Unde from your own tools</h1>
          <p className="duo mt-2 max-w-[72ch] text-[13.5px]">Connect an agent over MCP, or call the REST API. No key needed to start. Every answer is a prediction, not a ruling.</p>
        </div>

        <section aria-labelledby="start-h" className="flex flex-col gap-4 border border-edge bg-sheet px-5 py-4 shadow-[4px_4px_0_var(--shadow)]">
          <h2 id="start-h" className="eyebrow">
            Start in 2 minutes
          </h2>
          <ol className="flex flex-col gap-5">
            <Step n={1} title="Add the MCP server">
              <p className="duo text-[13px] text-dim">
                In <C>~/.gemini/settings.json</C>, or your client&apos;s MCP config:
              </p>
              <Code label="MCP config for Gemini CLI">{geminiConfig}</Code>
            </Step>
            <Step n={2} title="Ask your agent">
              <p className="duo text-[13.5px]">&ldquo;Where would CBP say this heat gun is from, and what move would make it Thai?&rdquo;</p>
            </Step>
            <Step n={3} title="Or call the API">
              <Code label="REST request without a key">{keyless}</Code>
              <p className="duo text-[13px] text-dim">
                The cookie jar keeps your case reachable.{" "}
                <a href="#keys" className={link}>
                  With a key
                </a>
              </p>
            </Step>
          </ol>
        </section>

        <section id="mcp" aria-labelledby="mcp-h" className="flex scroll-mt-20 flex-col gap-3">
          <h2 id="mcp-h" className="text-xl tracking-[-0.04em]">
            MCP
          </h2>
          <p className="duo max-w-[72ch] text-[13.5px]">
            Server: <C>https://undelabs.dev/mcp</C> (Streamable HTTP).
          </p>
          <div className="border-t border-line text-[13px]">
            <div className={row + " sm:grid-cols-[160px_minmax(0,1fr)]"}>
              <h3 className="font-medium">Gemini CLI</h3>
              <Code label="Gemini CLI command" wraps>
                {geminiAdd}
              </Code>
            </div>
            <div className={row + " sm:grid-cols-[160px_minmax(0,1fr)]"}>
              <h3 className="font-medium">Claude Code</h3>
              <Code label="Claude Code command" wraps>
                {mcpAdd}
              </Code>
            </div>
            <div className={row + " sm:grid-cols-[160px_minmax(0,1fr)]"}>
              <h3 className="font-medium">Claude.ai, Claude Desktop</h3>
              <p className="duo">Add a custom connector with the server URL and no sign-in.</p>
            </div>
            <div className={row + " sm:grid-cols-[160px_minmax(0,1fr)]"}>
              <h3 className="font-medium">Google ADK</h3>
              <p className="duo">
                A sample Python agent works a whole case: parts, origin, plans and a broker draft.{" "}
                <a href={adkExample} target="_blank" rel="noreferrer" className={link}>
                  The agent
                </a>
                {" · "}
                <a href={adkTranscript} target="_blank" rel="noreferrer" className={link}>
                  A run
                </a>
                {" · "}
                <a href={adkEval} target="_blank" rel="noreferrer" className={link}>
                  How it scored
                </a>
              </p>
            </div>
          </div>
          <p className="duo text-[13px] text-dim">
            With an API key, send <C>Authorization: Bearer op_…</C>.
          </p>
          <Details title="Key setup, ADK code, local relay">
            <p>Gemini CLI with a key:</p>
            <Code label="Gemini CLI settings.json">{geminiSettings}</Code>
            <p>
              Gemini CLI also reads a project&apos;s <C>.gemini/settings.json</C>. Leave the <C>headers</C> line out to use the anonymous tier.
            </p>
            <p>
              Claude Code with a key. Check the connection with <C>/mcp</C> inside Claude Code.
            </p>
            <Code label="Claude Code command with an API key" wraps>
              {mcpAddKey}
            </Code>
            <p>
              Claude.ai and Claude Desktop menus differ by plan; Anthropic&apos;s help center has the current steps. Use the published{" "}
              <a href="https://www.npmjs.com/package/@undelabs/mcp" target="_blank" rel="noreferrer" className={link}>
                stdio relay (<code className="font-mono text-[12.5px]">npx -y @undelabs/mcp</code>)
              </a>{" "}
              (Node 20+), or the{" "}
              <a href={relay} target="_blank" rel="noreferrer" className={link}>
                repo copy
              </a>{" "}
              from a clone. Example Claude Desktop config (published package):
            </p>
            <Code label="Claude Desktop config (published)">{desktopRelay}</Code>
            <p className="duo text-[13px]">
              From a clone, use <C>command</C> <C>node</C> with <C>args</C> set to the path of <C>mcp/stdio/index.js</C> (see the{" "}
              <a href={mcpDoc} target="_blank" rel="noreferrer" className={link}>
                MCP guide
              </a>
              ).
            </p>
            <p>
              The Google ADK agent runs on Gemini with a Google API key or Vertex AI. Each answer ends with &ldquo;Not a ruling&rdquo; and a pointer to a licensed customs broker.
            </p>
            <Code label="Google ADK agent in Python">{adkAgent}</Code>
            <p>
              Run on 22 Nov 2026 against undelabs.dev, anonymously, with Gemini 3.8 Flash through a LiteLLM gateway to Gemini: one heat-gun case, predicted China,
              precedent-supported plans to Thailand, a draft for the broker. The first plan call failed on the server&apos;s side at 300 s; the retry, with the same
              idempotency key, joined the search still running, so the search ran once.
            </p>
            <p>
              Then scored on 12 products from earlier CBP rulings, one run each, with the checks written first: 10 of 12 passed every check, and no answer named a
              ruling the tools had not returned. One miss retried a call the tool had refused; the other drafted the current setup when no plan was found. After fixes, a
              second evaluation with stricter checks passed all 8 runs the live server&apos;s anonymous budget did not touch; the budget cut the other 4 short, and 9 of 12
              pass as scored. Repeatability is not measured yet.
            </p>
            <p>
              The Gemini CLI and ADK formats follow their documentation (
              <a href={geminiDocs} target="_blank" rel="noreferrer" className={link}>
                Gemini CLI MCP servers
              </a>
              ,{" "}
              <a href={adkDocs} target="_blank" rel="noreferrer" className={link}>
                ADK MCP tools
              </a>
              , read 21 Nov 2026). We first ran both on 21 Nov 2026 with the read-only tools (search_rulings, get_ruling), the model reached through a LiteLLM gateway to
              Gemini; transcripts are in docs/mcp-transcripts/. The server they reach is the same one Claude Code uses.
            </p>
          </Details>
        </section>

        <section aria-labelledby="tools-h" className="flex flex-col gap-3">
          <h2 id="tools-h" className="text-xl tracking-[-0.04em]">
            The {tools.length} tools
          </h2>
          <table className="w-full border-collapse text-left text-[13px]">
            <thead className="sr-only sm:not-sr-only">
              <tr className="border-b border-line text-[12px] text-dim">
                <th scope="col" className="py-1.5 pr-4 font-normal">
                  Tool
                </th>
                <th scope="col" className="py-1.5 pr-4 font-normal">
                  What it does
                </th>
                <th scope="col" className="py-1.5 font-normal whitespace-nowrap">
                  Limit
                </th>
              </tr>
            </thead>
            <tbody>
              {tools.map(([name, what, cls]) => (
                <tr key={name} className="grid border-b border-line py-1.5 sm:table-row">
                  <th scope="row" className="py-0.5 pr-4 text-left align-top font-mono text-[12.5px] font-normal [overflow-wrap:anywhere] sm:py-1.5">
                    {name}
                  </th>
                  <td className="duo py-0.5 pr-4 align-top sm:py-1.5">{what}</td>
                  <td className="py-0.5 align-top text-dim whitespace-nowrap sm:py-1.5">{RATE[cls]}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <ul className="duo flex flex-col gap-1 text-[13px]">
            <li>
              Only U.S. imports get an origin prediction. Other markets go to review.
            </li>
            <li>
              Without a key, keep the <C>case_access_key</C> that <C>predict_origin</C> returns. It opens that case in the other tools; keep it private. If you use your own API key (<C>X-API-Key</C> or an <C>Authorization: Bearer</C> that owns the case), you do not need it.
            </li>
            <li>No tool deletes anything.</li>
          </ul>
          <Details title="Anonymous MCP workflow example">
            <p>Example <C>predict_origin</C> result (synthetic):</p>
            <Code label="predict_origin result (synthetic)">{`{
  "id": "case_abc123",
  "case_access_key": "ca_example_synthetic_key_do_not_use",
  "prediction": { "origin_country": "…" }
}`}</Code>
            <p>Then call other tools for that case with the same key, for example <C>get_case</C>:</p>
            <Code label="get_case (synthetic)">{`{
  "id": "case_abc123",
  "case_access_key": "ca_example_synthetic_key_do_not_use"
}`}</Code>
            <p>Or <C>plan_production_moves</C> (pass <C>case_id</C> and <C>case_access_key</C> when you are not the API-key owner):</p>
            <Code label="plan_production_moves (synthetic)">{`{
  "case_id": "case_abc123",
  "case_access_key": "ca_example_synthetic_key_do_not_use"
}`}</Code>
          </Details>
          <Details>
            <p>
              <C>predict_origin</C> takes an optional <C>jurisdiction</C>: the market the goods enter (<C>us</C>, <C>eu</C>, <C>uk</C>, <C>ca</C>, <C>jp</C>, <C>kr</C>,{" "}
              <C>au</C>, <C>mx</C> or <C>other</C>). Left out, the server reads it from the facts and falls back to <C>us</C>, with no model call.{" "}
              <C>GET /v1/jurisdictions</C> lists the markets. Plan search and country comparison work on U.S. cases only.
            </p>
            <p>
              Each plan has a <C>plan_id</C>; pass it to <C>export_ruling_request_draft</C>, or send none to draft the current setup. On a Part 102 case the Part 102
              engine predicts each move and the 19 CFR 102.20 check orders the shown plans.
            </p>
            <p>
              <C>suggest_part_hts</C> marks each code as from an earlier ruling or the model&apos;s reading, so the tariff-shift rule check can run. A code whose chapter
              looks wrong for the product is marked to check.
            </p>
            <p>
              <C>check_rcep_preference</C> and <C>check_canada_marking</C> make no model call and save nothing. Both are separate from the origin answer. The Canada check
              has no accuracy figure: Canada publishes too few origin rulings to test it against.
            </p>
            <p>
              Each tool runs one REST call, with the same limits as the API. <C>get_ruling</C> and <C>get_duty_rates</C> also take from the ruling-lookup limit when they
              fetch live.
            </p>
          </Details>
          <p className="text-[13.5px]">
            <a href={mcpDoc} target="_blank" rel="noreferrer" className={link}>
              Full MCP guide →
            </a>
          </p>
        </section>

        <section id="api" aria-labelledby="api-h" className="flex scroll-mt-20 flex-col gap-3">
          <h2 id="api-h" className="text-xl tracking-[-0.04em]">
            REST API
          </h2>
          <p className="duo max-w-[72ch] text-[13.5px]">
            Base URL <C>https://undelabs.dev/api</C>. Same JSON the site uses.{" "}
            <a href={spec} target="_blank" rel="noreferrer" className={link}>
              Every endpoint (OpenAPI) →
            </a>
          </p>
          <div id="keys" className="scroll-mt-20 border-t border-line text-[13px]">
            <div className={row + " sm:grid-cols-[160px_minmax(0,1fr)]"}>
              <h3 className="font-medium">With a key</h3>
              <p className="duo">
                Send <C>X-API-Key: op_…</C> on every request. The key sees the demos and its own cases. The Unde Labs team issues keys.
              </p>
            </div>
            <div className={row + " sm:grid-cols-[160px_minmax(0,1fr)]"}>
              <h3 className="font-medium">Without a key</h3>
              <p className="duo">Keep the cookie from the first reply, as in step 3. Lower limits.</p>
            </div>
          </div>
          <Code label="REST request with an API key">{example}</Code>
          <p className="duo text-[13px] text-dim">Call from your server. There are no CORS headers, so browsers on other sites are refused.</p>
          <Details title="Keys and cookies">
            <p>
              A key is <C>op_</C> plus 43 characters. The REST API does not take <C>Authorization: Bearer</C>; that form works only on <C>/mcp</C>. There is no key
              management in the browser. Every origin answer and plan carries the model ids, prompt versions and the disclaimer.
            </p>
            <p>
              Without a key, the site&apos;s proxy keys your cases to the <C>op_visitor</C> cookie it sets on your first request (HttpOnly, one year). Send it back on
              every call, as <C>-c jar -b jar</C> does, or a later request cannot see the case and gets 404. An <C>X-Visitor-ID</C> header you send yourself is not passed
              on. Whoever holds the cookie can read and change those cases. Read the case back with the <C>case_id</C> from the answer:
            </p>
            <Code label="REST read without a key, with a cookie jar">{keylessRead}</Code>
          </Details>
        </section>

        <section aria-labelledby="limits-h" className="flex flex-col gap-3">
          <h2 id="limits-h" className="text-xl tracking-[-0.04em]">
            Limits
          </h2>
          <table className="w-full border-collapse text-left text-[13px]">
            <thead>
              <tr className="border-b border-line text-[12px] text-dim">
                <th scope="col" className="py-1.5 pr-3 font-normal">
                  Per key
                </th>
                <th scope="col" className="py-1.5 pr-3 font-normal">
                  Limit
                </th>
                <th scope="col" className="py-1.5 font-normal">
                  Past it
                </th>
              </tr>
            </thead>
            <tbody>
              {limits.map(([what, n, err]) => (
                <tr key={what} className="border-b border-line">
                  <th scope="row" className="duo py-1.5 pr-3 text-left align-top font-normal">
                    {what}
                  </th>
                  <td className="py-1.5 pr-3 align-top">{n}</td>
                  <td className="py-1.5 align-top [overflow-wrap:anywhere]">{err}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <p className="duo text-[13px] text-dim">
            A 429 carries <C>Retry-After</C>. Without a key, limits apply per address.
          </p>
          <Details>
            <p>
              Light model calls: predict, answers, ask, extract from text or files, import, money prefill, code suggestions for the good and its parts, cost estimate,
              family check, scenarios, ruling watch, BOM version. Plan estimate and the audit reads take from the same bucket without a model call. Plan searches and
              target comparisons are heavy; a batch triage takes the whole heavy bucket.
            </p>
            <p>
              Ruling searches, and rulings fetched from CBP, have their own limit of 30 a minute. The daily limits reset at 00:00 UTC.
            </p>
            <p>
              Every limited response carries <C>RateLimit-Limit</C>, <C>RateLimit-Remaining</C> and <C>RateLimit-Reset</C>. An unknown or disabled key gets 401{" "}
              <C>unauthorized</C>; after 10 in a minute from one address, 429 <C>auth_rate_limited</C>.
            </p>
          </Details>
        </section>

        {/* Built vs switched on (judge panel round 4): the deployed values are
            in docs/api/SECURITY.md, "As built vs as deployed". */}
        <section aria-labelledby="on-h" className="flex flex-col gap-2">
          <h2 id="on-h" className="text-xl tracking-[-0.04em]">
            What is switched on
          </h2>
          <ul className="duo flex list-disc flex-col gap-1 pl-5 text-[13.5px]">
            <li>On here: API keys, the limits above, private cases and a daily cap on model spend.</li>
            <li>Built, off here: the audit trail and required sign-in. Turning either on is an owner step, not a code change.</li>
          </ul>
          <p className="text-[13px]">
            <a href={security} target="_blank" rel="noreferrer" className={link}>
              Every switch, as built vs as deployed →
            </a>
          </p>
        </section>
      </Main>
    </>
  );
}
