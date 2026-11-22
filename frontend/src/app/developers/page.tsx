import type { Metadata } from "next";
import { AppBar, Main } from "@/components/op/chrome";
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
      "command": "node",
      "args": ["/path/to/wassup-hackathon-2026/mcp/stdio/index.js"],
      "env": { "UNDE_API_KEY": "op_…" }
    }
  }
}`;
const mcpAdd = `claude mcp add --transport http unde https://undelabs.dev/mcp`;
const mcpAddKey = `claude mcp add --transport http unde https://undelabs.dev/mcp \\
  --header "Authorization: Bearer op_…"`;

// The MCP tools, in the order docs/MCP.md lists them; each runs one REST
// operation with that operation's limits (internal/mcpserver).
const tools: [string, string][] = [
  ["predict_origin", "Predict the origin from facts or a structured product. Starts a case."],
  ["extract_product", "Turn a description into a structured product to check."],
  ["extract_product_from_media", "Read a photo or scan of a bill of materials into a draft product."],
  ["plan_production_moves", "Search production moves for a case and check each against earlier rulings. Each plan has a plan_id."],
  ["compare_target_countries", "Run one plan search per country (2 to 4) and rank them."],
  ["search_rulings", "Search the CBP rulings Unde reads."],
  ["get_ruling", "Fetch one CBP ruling."],
  ["get_duty_rates", "Look up the duty rates Unde uses for a tariff code."],
  ["get_case", "Read a case."],
  ["list_cases", "List your cases and the demos."],
  ["export_ruling_request_draft", "Preview the draft ruling request for a broker: a plan by its plan_id, or the current setup with none."],
  ["create_broker_review_link", "Make a link your broker opens to approve the draft or ask for changes."],
  ["get_broker_review_status", "Read the broker's answer and comments on each link."],
  ["suggest_part_hts", "Suggest HTS codes for each part, each marked as from an earlier ruling or the model's reading, so the tariff-shift rule check can run. A code whose chapter looks wrong for the product is marked to check."],
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

const example = `UNDE_API=https://undelabs.dev/api

curl -X POST "$UNDE_API/v1/cases/predict" \\
  -H "X-API-Key: op_…" \\
  -H "Content-Type: application/json" \\
  -d '{"date": "2026-06-01", "facts": "Each heat gun is assembled in Thailand from 24 components…"}'`;

// How a broker calls the API with a key (D69). Keys are issued by the admin;
// the browser never manages them, so this page only documents their use.
export default function Page() {
  return (
    <>
      <AppBar active="api" />
      <Main className="flex max-w-[860px] flex-col gap-6">
        <div>
          <p className="eyebrow">API · MCP</p>
          <h1 className="mt-1 text-[28px] tracking-[-0.05em]">Use Unde from your own tools</h1>
          <p className="duo mt-2 max-w-[72ch] text-[13.5px]">
            Two ways in, both built and running: an MCP server, so an agent in Gemini CLI, Google ADK or Claude can call Unde as tools, and a REST API for your own systems.
            Both work without a key, with lower limits. Every answer is a prediction, not a ruling.
          </p>
          <p className="duo mt-2 text-[13.5px]">
            <a href="#mcp" className="text-ok underline underline-offset-2">
              Use it from an agent (MCP)
            </a>
            {" · "}
            <a href="#api" className="text-ok underline underline-offset-2">
              Call the REST API
            </a>
          </p>
        </div>

        <section id="mcp" aria-labelledby="mcp-h" className="flex scroll-mt-20 flex-col gap-3 border border-edge bg-sheet px-5 py-4 shadow-[4px_4px_0_var(--shadow)]">
          <div>
            <h2 id="mcp-h" className="eyebrow">
              Use it from an agent (MCP)
            </h2>
            <p className="duo mt-1.5 max-w-[72ch] text-[13.5px]">
              MCP (Model Context Protocol) is an open protocol agents use to call other services. Gemini CLI, Google ADK agents and Claude all speak it. Unde&apos;s hosted MCP
              server is <code className="font-mono">https://undelabs.dev/mcp</code> (Streamable HTTP). Ask the agent something like &ldquo;Where would CBP say this heat gun is
              from, and what move would make it Thai?&rdquo; and it calls the tools below.
            </p>
          </div>
          <div className="flex flex-col gap-1.5">
            <h3 className="text-[13.5px] font-medium">Gemini CLI: one line</h3>
            <Code label="Gemini CLI command" wraps>{geminiAdd}</Code>
            <p className="duo text-[13px] text-dim">
              Or add it to <code className="font-mono">~/.gemini/settings.json</code> (or a project&apos;s <code className="font-mono">.gemini/settings.json</code>). The{" "}
              <code className="font-mono">headers</code> line is only for an API key; leave it out to use the anonymous tier.
            </p>
            <Code label="Gemini CLI settings.json">{geminiSettings}</Code>
          </div>
          <div className="flex flex-col gap-1.5">
            <h3 className="text-[13.5px] font-medium">Agents (Google ADK)</h3>
            <p className="duo max-w-[72ch] text-[13.5px]">
              A Python agent built with Google&apos;s Agent Development Kit works a whole case through the MCP server: from a product description it drafts the parts,
              predicts the origin with the CBP rulings it cites, plans production moves to a target country and drafts the ruling request for a broker. It runs on Gemini
              with a Google API key or Vertex AI. Each answer ends with &ldquo;Not a ruling&rdquo; and a pointer to a licensed customs broker.
            </p>
            <Code label="Google ADK agent in Python">{adkAgent}</Code>
            <p className="duo max-w-[72ch] text-[13px] text-dim">
              Run on 22 Nov 2026 against undelabs.dev, anonymously, with Gemini 3.8 Flash through a LiteLLM gateway to Gemini: one heat-gun case, predicted China,
              precedent-supported plans to Thailand, a draft for the broker. The first plan call failed on the server&apos;s side at 300 s; the retry, with the same
              idempotency key, joined the search still running, so the search ran once.{" "}
              <a href={adkExample} target="_blank" rel="noreferrer" className="text-ok underline underline-offset-2">
                The agent and exact commands
              </a>
              {" · "}
              <a href={adkTranscript} target="_blank" rel="noreferrer" className="text-ok underline underline-offset-2">
                The run&apos;s transcript
              </a>
            </p>
            <p className="duo max-w-[72ch] text-[13px] text-dim">
              Then scored on 12 products from earlier CBP rulings, one run each, with the checks written first: 10 of 12 passed every check, and no answer named a
              ruling the tools had not returned. One miss retried a call the tool had refused; the other drafted the current setup when no plan was found.{" "}
              <a href={adkEval} target="_blank" rel="noreferrer" className="text-ok underline underline-offset-2">
                The agent evaluation
              </a>
            </p>
            <p className="duo max-w-[72ch] text-[13px] text-dim">
              The Gemini CLI and ADK formats follow their documentation (
              <a href={geminiDocs} target="_blank" rel="noreferrer" className="text-ok underline underline-offset-2">
                Gemini CLI MCP servers
              </a>
              ,{" "}
              <a href={adkDocs} target="_blank" rel="noreferrer" className="text-ok underline underline-offset-2">
                ADK MCP tools
              </a>
              , read 21 Nov 2026). We first ran both on 21 Nov 2026 with the read-only tools (search_rulings, get_ruling), the model reached through a LiteLLM gateway to Gemini; transcripts are in docs/mcp-transcripts/. The server they reach is the same one Claude Code uses.
            </p>
          </div>
          <div className="flex flex-col gap-1.5">
            <h3 className="text-[13.5px] font-medium">Claude Code: one line</h3>
            <Code label="Claude Code command" wraps>{mcpAdd}</Code>
            <p className="duo text-[13px] text-dim">
              With an API key, add it as a header. Check the connection with <code className="font-mono">/mcp</code> inside Claude Code.
            </p>
            <Code label="Claude Code command with an API key" wraps>{mcpAddKey}</Code>
          </div>
          <div className="flex flex-col gap-1.5">
            <h3 className="text-[13.5px] font-medium">Claude.ai and Claude Desktop</h3>
            <p className="duo max-w-[72ch] text-[13.5px]">
              In connector settings, add a custom connector with the URL <code className="font-mono">https://undelabs.dev/mcp</code> and no sign-in. Menus differ by plan;
              Anthropic&apos;s help center has the current steps. A{" "}
              <a href={relay} target="_blank" rel="noreferrer" className="text-ok underline underline-offset-2">
                local stdio relay
              </a>{" "}
              is in the repository for clients that prefer a local process. In Claude Desktop&apos;s config file it runs from a clone of this repository with Node 20 or
              later:
            </p>
            <Code label="Claude Desktop config">{desktopRelay}</Code>
          </div>
          <div className="flex flex-col gap-1.5">
            <h3 className="text-[13.5px] font-medium">The 14 tools</h3>
            <dl className="border-t border-line text-[13px]">
              {tools.map(([name, what]) => (
                <div key={name} className="grid gap-x-4 gap-y-0.5 border-b border-line py-1.5 sm:grid-cols-[240px_minmax(0,1fr)]">
                  <dt className="font-mono text-[12.5px] [overflow-wrap:anywhere]">{name}</dt>
                  <dd className="duo">{what}</dd>
                </div>
              ))}
            </dl>
            <p className="duo max-w-[72ch] text-[13px] text-dim">
              Each tool runs one REST call, with the same limits as the API below. No tool deletes anything. Without a key, <code className="font-mono">predict_origin</code>{" "}
              returns a <code className="font-mono">case_access_key</code>; pass it to the other tools to reach that case, and keep it private.
            </p>
          </div>
          <p className="text-[13.5px]">
            <a href={mcpDoc} target="_blank" rel="noreferrer" className="text-ok underline underline-offset-2">
              Full MCP guide: setup, tools, limits and privacy →
            </a>
          </p>
        </section>

        <div id="api" className="scroll-mt-20">
          <h2 className="text-xl tracking-[-0.04em]">Call the REST API</h2>
          <p className="duo mt-2 max-w-[72ch] text-[13.5px]">
            Brokers can send cases to Unde from their own tools with an API key. The Unde Labs team issues keys; there&apos;s no key management in the browser. Requests without a key work as
            they do on this site.
          </p>
        </div>

        <section className="flex flex-col gap-2">
          <h2 className="eyebrow">Send the key with every request</h2>
          <Code label="REST request with an API key">{example}</Code>
          <p className="duo text-[13px] text-dim">
            A key is <code className="font-mono">op_</code> plus 43 characters, sent as the <code className="font-mono">X-API-Key</code> header. Responses are the same JSON this site uses, with the
            disclaimer on every one.
          </p>
        </section>

        <section className="flex flex-col gap-2">
          <h2 className="eyebrow">Limits</h2>
          <ul className="duo flex list-disc flex-col gap-1.5 pl-5 text-[13.5px]">
            <li>Each key has a number of requests per UTC day. Past it: 429 <code className="font-mono">quota_exceeded</code>, with Retry-After set to the next UTC day.</li>
            <li>
              Model calls are limited per key: 20 a minute for light requests (predict, answers, ask, import, scenarios, code suggestions) and 4 a minute for plan searches. Reads and
              other requests without a model call: 120 a minute. Over that: 429 <code className="font-mono">rate_limited</code> with Retry-After.
            </li>
            <li>
              Every limited response carries <code className="font-mono">RateLimit-Limit</code>, <code className="font-mono">RateLimit-Remaining</code> and{" "}
              <code className="font-mono">RateLimit-Reset</code>.
            </li>
            <li>
              At most 4 model calls may run at once per key (429 <code className="font-mono">too_many_in_flight</code>). One key may spend at most a tenth of
              the day&apos;s model budget (429 <code className="font-mono">client_budget_reached</code> until the next UTC day).
            </li>
            <li>
              An unknown or disabled key gets 401 <code className="font-mono">unauthorized</code>; after 10 in a minute from one address, 429{" "}
              <code className="font-mono">auth_rate_limited</code>. When the day&apos;s model budget is spent, model calls return 503{" "}
              <code className="font-mono">budget_reached</code>.
            </li>
            <li>Call the API from your server. It sends no CORS headers, so calls from a browser on another site are refused.</li>
          </ul>
        </section>

        <section className="flex flex-col gap-2">
          <h2 className="eyebrow">Your cases</h2>
          <p className="duo text-[13.5px]">A key sees the demo cases and the cases it created. Its cases are hidden from everyone else.</p>
        </section>

        {/* Built vs switched on (judge panel round 4): the deployed values are
            in docs/api/SECURITY.md, "As built vs as deployed". */}
        <section className="flex flex-col gap-2">
          <h2 className="eyebrow">Built, and what is switched on here</h2>
          <ul className="duo flex list-disc flex-col gap-1.5 pl-5 text-[13.5px]">
            <li>On at undelabs.dev: API keys, the limits above, cases visible only to the key or visitor that made them, and a daily cap on model spend.</li>
            <li>
              Built but not switched on here: the audit trail (it needs an audit key set in production) and required sign-in (anonymous use is on, so the demo opens
              without an account). Turning either on is an owner step in the deployment, not a code change.
            </li>
          </ul>
          <p className="duo text-[13px] text-dim">
            Every switch, its code default and its deployed value:{" "}
            <a href={security} target="_blank" rel="noreferrer" className="text-ok underline underline-offset-2">
              SECURITY.md, as built vs as deployed →
            </a>
          </p>
        </section>

        <p className="text-[13.5px]">
          <a href={spec} target="_blank" rel="noreferrer" className="text-ok underline underline-offset-2">
            Every endpoint, in the OpenAPI spec →
          </a>
        </p>
      </Main>
    </>
  );
}
