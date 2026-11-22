"use client";

import { useRef, useState, useSyncExternalStore, type KeyboardEvent } from "react";
import { Fold } from "@/components/op/ui";
import type { CaseResponse } from "@/lib/api/client";
import { filterTools, rates, toolGroups, tools, type ToolGroup } from "./catalog";

const geminiAdd = "gemini mcp add --transport http unde https://undelabs.dev/mcp";
const endpoint = "https://undelabs.dev/api/v1/cases/predict";
const auth = "X-API-Key: op_...";
const payload = JSON.stringify({
  date: "2026-06-01",
  facts: "A heat gun is assembled in Thailand from a motor and control board made in China.",
  jurisdiction: "us",
});
const predictCommand = [
  `curl -X POST ${endpoint}`,
  `  -H "${auth}"`,
  '  -H "Content-Type: application/json"',
  `  -d '${payload}'`,
].join(" \\\n");
const anonymousCommand = [
  `curl -c jar -b jar -X POST ${endpoint}`,
  '  -H "Content-Type: application/json"',
  `  -d '${payload}'`,
].join(" \\\n");
const response = JSON.stringify({
  id: "case_0123456789ab",
  jurisdiction: "us",
  prediction: {
    origin_country: "Thailand",
    confidence: 0.86,
    cited_rulings: ["N328954"],
  },
  triage: { route: "needs_review" },
  disclaimer: "Not a ruling. Confirm with a licensed customs broker or a CBP binding ruling.",
} satisfies Pick<CaseResponse, "id" | "jurisdiction" | "prediction" | "triage" | "disclaimer">, null, 2);

const tabs = [
  { key: "mcp", label: "MCP server", audience: "AI agents" },
  { key: "api", label: "REST API", audience: "Customs brokers" },
] as const;
type Mode = (typeof tabs)[number]["key"];

const docLink = "text-ok underline underline-offset-3 hover:text-ink";
const panel = "min-w-0 rounded-sm border border-edge bg-sheet shadow-[3px_3px_0_var(--shadow)]";
const heading = "text-xl font-medium tracking-[-0.04em] sm:text-2xl";
const lab = "block text-xs font-medium text-dim";
const field = "mt-1.5 min-h-11 w-full rounded-sm border border-edge bg-sheet px-3 text-sm text-ink";
const groups = Object.entries(toolGroups) as [ToolGroup, string][];

function modeFromURL(): Mode {
  const hash = window.location.hash;
  return hash === "#api" || hash === "#keys" || hash === "#limits" ? "api" : "mcp";
}

function onHashChange(listener: () => void) {
  window.addEventListener("hashchange", listener);
  return () => window.removeEventListener("hashchange", listener);
}

function defaultMode(): Mode {
  return "mcp";
}

function selectMode(mode: Mode) {
  window.history.replaceState(window.history.state, "", `#${mode}`);
  window.dispatchEvent(new Event("hashchange"));
}

export function DevelopersPortal({ repo }: { repo: string }) {
  const mode = useSyncExternalStore(onHashChange, modeFromURL, defaultMode);

  function moveTab(event: KeyboardEvent<HTMLButtonElement>, current: Mode) {
    const choices: Record<string, Mode> = {
      ArrowRight: current === "mcp" ? "api" : "mcp",
      ArrowLeft: current === "mcp" ? "api" : "mcp",
      Home: "mcp",
      End: "api",
    };
    const next = choices[event.key];
    if (!next) return;
    event.preventDefault();
    selectMode(next);
    document.getElementById(`tab-${next}`)?.focus();
  }

  return (
    <>
      <div role="tablist" aria-label="Integration mode" className="grid grid-cols-2 divide-x divide-line rounded-sm border border-edge bg-sheet">
        {tabs.map(({ key, label, audience }) => {
          const selected = mode === key;
          return (
            <button
              key={key}
              id={`tab-${key}`}
              type="button"
              role="tab"
              aria-selected={selected}
              aria-controls={key}
              tabIndex={selected ? 0 : -1}
              onClick={() => selectMode(key)}
              onKeyDown={(event) => moveTab(event, key)}
              className={`min-w-0 border-b-2 px-3 py-3 text-left hover:bg-wash sm:px-5 sm:py-4 ${selected ? "border-ok bg-ok-wash text-ok" : "border-transparent text-ink"}`}
            >
              <span className="block text-base font-semibold sm:text-lg">{label}</span>
              <span className="mt-0.5 block text-xs text-dim">{audience}</span>
            </button>
          );
        })}
      </div>
      <div id="mcp" role="tabpanel" aria-labelledby="tab-mcp" tabIndex={0} hidden={mode !== "mcp"}>
        <McpMode repo={repo} />
      </div>
      <div id="api" role="tabpanel" aria-labelledby="tab-api" tabIndex={0} hidden={mode !== "api"}>
        <RestMode repo={repo} />
      </div>
    </>
  );
}

function CopyCode({ label, value, language }: { label: string; value: string; language?: "json" }) {
  const code = useRef<HTMLElement>(null);
  const [feedback, setFeedback] = useState("");
  const [copied, setCopied] = useState(false);

  async function copyCode() {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      setFeedback(`${label} copied.`);
    } catch {
      const selection = window.getSelection();
      if (!selection || !code.current) {
        setFeedback("Copy unavailable.");
        return;
      }
      const range = document.createRange();
      range.selectNodeContents(code.current);
      selection.removeAllRanges();
      selection.addRange(range);
      setFeedback("Text selected. Use your device’s copy command.");
    }
  }

  return (
    <div className={panel}>
      <div className="flex min-h-11 items-center justify-between gap-3 border-b border-line bg-wash px-3 sm:px-4">
        <p className="min-w-0 text-xs font-medium text-ink [overflow-wrap:anywhere]">{label}</p>
        <button
          type="button"
          aria-label={`Copy ${label}`}
          onClick={copyCode}
          className="my-1.5 min-h-8 shrink-0 rounded-sm border border-edge bg-sheet px-2.5 text-xs font-medium text-ok hover:bg-ok-wash"
        >
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
      <pre className="min-w-0 px-3 py-3 text-[12px] leading-6 whitespace-pre-wrap [overflow-wrap:anywhere] sm:px-4 sm:text-[13px]">
        <code ref={code}>{language === "json" ? <HighlightedJSON value={value} /> : value}</code>
      </pre>
      <p role="status" className={copied || !feedback ? "sr-only" : "px-3 pb-3 text-xs text-dim"}>
        {feedback}
      </p>
    </div>
  );
}

function HighlightedJSON({ value }: { value: string }) {
  const tokens = value.split(/("(?:[^"\\]|\\.)*"|\b\d+(?:\.\d+)?\b)/g);
  return tokens.map((token, index) => {
    if (index % 2 === 0) return token;
    const color = token.startsWith('"') ? (tokens[index + 1]?.trimStart().startsWith(":") ? "text-ok" : "text-body") : "text-dimension";
    return (
      <span key={index} className={color}>
        {token}
      </span>
    );
  });
}

function McpMode({ repo }: { repo: string }) {
  const [query, setQuery] = useState("");
  const [group, setGroup] = useState<ToolGroup | "">("");
  const filtered = filterTools(query, group);

  return (
    <div className="flex min-w-0 flex-col gap-7 sm:gap-9">
      <section aria-labelledby="mcp-setup" className="grid items-start gap-4 lg:grid-cols-[minmax(0,0.7fr)_minmax(0,1.3fr)] lg:gap-8">
        <div>
          <h2 id="mcp-setup" className={heading}>Connect Gemini CLI</h2>
          <p className="duo mt-2 text-sm text-dim">No key needed to start. Ask where CBP would say your product is from.</p>
          <div className="mt-2.5 flex flex-wrap gap-x-4 gap-y-1 text-xs">
            <a className={docLink} href={`${repo}/blob/main/docs/MCP.md`}>MCP setup guide ↗</a>
            <a className={docLink} href={`${repo}/tree/main/examples/adk-agent`}>Google ADK example ↗</a>
          </div>
        </div>
        <CopyCode label="Gemini CLI command" value={geminiAdd} />
      </section>

      <section aria-labelledby="tool-heading" className="border-t border-line pt-5 sm:pt-6">
        <div className="grid items-end gap-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,230px)_minmax(0,245px)]">
          <h2 id="tool-heading" className={heading}>
            {filtered.length === tools.length ? tools.length : `${filtered.length} of ${tools.length}`} MCP tools
          </h2>
          <label className={lab}>
            Search tools
            <input type="search" className={field} value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Name or task" />
          </label>
          <label className={lab}>
            Category
            <select className={field} value={group} onChange={(event) => setGroup(event.target.value as ToolGroup | "")}>
              <option value="">All categories</option>
              {groups.map(([key, name]) => <option key={key} value={key}>{name}</option>)}
            </select>
          </label>
        </div>
        <p role="status" className="sr-only">{filtered.length} tools match</p>
        {filtered.length ? (
          <div className="mt-4 grid items-start gap-3 sm:gap-4 min-[700px]:grid-cols-2 min-[1050px]:grid-cols-4">
            {groups.map(([key, name]) => {
              const members = filtered.filter((tool) => tool[1] === key);
              if (!members.length) return null;
              return (
                <section key={key} aria-labelledby={`catalog-${key}`} className="min-w-0 rounded-sm border border-line bg-sheet">
                  <h3 id={`catalog-${key}`} className="border-b border-line bg-wash px-3 py-2.5 text-sm font-semibold sm:px-4">{name}</h3>
                  {key === "tariff" && <p className="duo border-b border-line px-3 py-2 text-xs text-dim sm:px-4">U.S. predictions can include the 102.20 check when you provide tariff codes.</p>}
                  <ul className="divide-y divide-line">
                    {members.map(([id, , , input, output]) => (
                      <li key={id} className="px-3 py-2.5 sm:px-4">
                        <code className="text-[12px] font-medium text-ok [overflow-wrap:anywhere] sm:text-[13px]">{id}</code>
                        <p className="duo mt-0.5 text-[13px] text-dim">{input} <span aria-hidden="true">→</span><span className="sr-only">returns</span> {output}</p>
                      </li>
                    ))}
                  </ul>
                </section>
              );
            })}
          </div>
        ) : (
          <p className="duo mt-4 rounded-sm border border-line bg-sheet p-4 text-sm text-dim">No tools match. Try another search or category.</p>
        )}
        <p className="duo mt-4 text-sm text-dim">Without an API key, keep the private <code>case_access_key</code> to reopen your case.</p>
        <div className="mt-3">
          <Fold title="Tool access and rule checks">
            <div className="duo flex max-w-[72ch] flex-col gap-2 text-sm text-dim">
              <p>With a key, send <code>Authorization: Bearer op_...</code>. No tool deletes anything.</p>
              <p>Use <code>GET /v1/jurisdictions</code> to see supported markets. Plan search and country comparison work on U.S. cases only.</p>
              <p>When you enter tariff codes for the finished good and its parts, <code>predict_origin</code> and <code>get_case</code> can return the 19 CFR 102.20 rule check. Suggested codes need review. The Canada check is rules only, with no accuracy figure.</p>
              <p>Each tool uses the limit of its REST operation. Live ruling and duty lookups can also use the ruling-lookup bucket.</p>
            </div>
          </Fold>
        </div>
      </section>
    </div>
  );
}

const limitRows = [
  ["Read-only and other basic calls", rates.basic],
  ["Light model calls", rates.light],
  ["Plan search and comparison", rates.heavy],
  ["Ruling lookups", rates.lookup],
  ["Concurrent model calls", "4"],
];

const statuses = [
  { code: "401", detail: "unauthorized", meaning: "Key invalid or disabled", color: "border-fail/40 bg-fail-wash text-fail" },
  { code: "429", detail: "rate_limited / quota_exceeded", meaning: "Slow down or wait for quota reset", color: "border-ok/40 bg-ok-wash text-ok" },
  { code: "503", detail: "budget_reached", meaning: "Daily model budget reached", color: "border-ask-line bg-ask-wash text-ask" },
];

function RestMode({ repo }: { repo: string }) {
  return (
    <div className="flex min-w-0 flex-col gap-7 sm:gap-9">
      <section aria-labelledby="rest-predict">
        <h2 id="rest-predict" className={heading}>Predict origin</h2>
        <div className="mt-3 grid gap-4 md:grid-cols-2">
          <div id="keys" className="flex min-w-0 flex-col gap-3">
            <div className="rounded-sm border border-line bg-sheet px-3 py-3 sm:px-4">
              <h3 className="text-sm font-semibold">Authentication</h3>
              <p className="duo mt-1 text-sm text-dim">Call from your server. Request a key from Unde Labs and send <code className="whitespace-nowrap">{auth}</code>.</p>
            </div>
            <CopyCode label="POST /v1/cases/predict" value={predictCommand} />
            <Fold title="Try without a key">
              <p className="duo mb-2 text-sm text-dim">Reuse the cookie jar on every call to keep access to your case.</p>
              <CopyCode label="Anonymous cURL request" value={anonymousCommand} />
            </Fold>
          </div>
          <div className="min-w-0">
            <CopyCode label="201 response excerpt · illustrative" value={response} language="json" />
          </div>
        </div>
      </section>
      <section id="limits" aria-labelledby="limit-heading" className="border-t border-line pt-5 sm:pt-6">
        <h2 id="limit-heading" className={heading}>Rate limits</h2>
        <div className="mt-3 grid gap-4 md:grid-cols-2">
          <div className="rounded-sm border border-line bg-sheet">
            <table className="w-full border-collapse text-left text-sm">
              <thead>
                <tr className="border-b border-line bg-wash">
                  <th scope="col" className="px-3 py-2.5 font-medium sm:px-4">Request class</th>
                  <th scope="col" className="px-3 py-2.5 font-medium whitespace-nowrap sm:px-4">Per client</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line">
                {limitRows.map(([name, value]) => (
                  <tr key={name}>
                    <th scope="row" className="duo px-3 py-2.5 font-normal sm:px-4">{name}</th>
                    <td className="px-3 py-2.5 font-medium whitespace-nowrap sm:px-4">{value}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <ul aria-label="HTTP error responses" className="grid content-start gap-2">
            {statuses.map(({ code, detail, meaning, color }) => (
              <li key={code} className="rounded-sm border border-line bg-sheet px-3 py-2.5 sm:px-4">
                <code className={`inline-block max-w-full rounded-sm border px-2 py-1 text-[11px] leading-normal whitespace-normal ${color}`}>{code} {detail}</code>
                <p className="duo mt-1 text-[13px] text-dim">{meaning}</p>
              </li>
            ))}
          </ul>
        </div>
        <p className="duo mt-3 text-sm text-dim">A 429 includes <code>Retry-After</code>. Daily quotas and budgets reset at 00:00 UTC.</p>
        <div className="mt-3">
          <Fold title="Quotas, keys and deployment">
            <div className="duo flex max-w-[72ch] flex-col gap-2 text-sm text-dim">
              <p>Other 429 codes include <code>too_many_in_flight</code>, <code>client_budget_reached</code> and <code>auth_rate_limited</code>. Without a key, limits apply per address.</p>
              <p>The REST API takes <code>X-API-Key</code> and serves server-to-server requests. MCP uses a bearer key.</p>
              <p>API keys, private cases and a model spend cap are on. The audit trail and required sign-in are available to operators. <a className={docLink} href={`${repo}/blob/main/docs/api/SECURITY.md#as-built-vs-as-deployed-21-nov-2026`}>Deployment settings ↗</a></p>
            </div>
          </Fold>
        </div>
      </section>
    </div>
  );
}
