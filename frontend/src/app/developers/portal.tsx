"use client";

import { useRef, useState, useSyncExternalStore, type KeyboardEvent } from "react";
import { filterTools, rates, tools, type ToolGroup, type ToolName } from "./catalog";
import { endpoints, keyHeader, mcpEndpoint, setupExamples, toolExamples } from "./examples";

type Mode = "mcp" | "api";
type Client = keyof typeof setupExamples;
type Endpoint = keyof typeof endpoints;
type Tab<Key extends string> = { key: Key; label: string; subtext?: string };

const modes: Tab<Mode>[] = [
  { key: "mcp", label: "MCP server", subtext: "For AI agents" },
  { key: "api", label: "REST API", subtext: "Customs & ERP software" },
];
const clients: Tab<Client>[] = [
  { key: "gemini", label: "Gemini CLI" },
  { key: "claude", label: "Claude Desktop" },
  { key: "adk", label: "ADK for Go" },
];
const categories: [ToolGroup | "", string][] = [
  ["", `All ${tools.length}`],
  ["origin", "Origin"],
  ["planning", "Plan"],
  ["tariff", "Tariff codes"],
  ["broker", "Draft"],
];
const docLink = "text-ok underline underline-offset-3 hover:text-ink";
const panel = "min-w-0 rounded-sm border border-edge bg-sheet";
const heading = "text-xl font-medium tracking-[-0.04em] sm:text-2xl";
const field = "min-h-10 w-full rounded-sm border border-edge bg-sheet px-3 text-sm text-ink";
const json = (value: unknown) => JSON.stringify(value, null, 2);

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

function Tabs<Key extends string>({
  label, prefix, items, active, select, compact = false,
}: {
  label: string;
  prefix: string;
  items: Tab<Key>[];
  active: Key;
  select: (key: Key) => void;
  compact?: boolean;
}) {
  function moveTab(event: KeyboardEvent<HTMLButtonElement>, index: number) {
    const last = items.length - 1;
    const next = {
      ArrowLeft: index === 0 ? last : index - 1,
      ArrowRight: index === last ? 0 : index + 1,
      Home: 0,
      End: last,
    }[event.key];
    if (next === undefined) return;
    event.preventDefault();
    const key = items[next].key;
    select(key);
    document.getElementById(`${prefix}${key}-tab`)?.focus();
  }

  return (
    <div
      role="tablist"
      aria-label={label}
      className={`grid gap-px rounded-sm border border-edge bg-edge ${compact ? "grid-cols-3" : "grid-cols-2"}`}
    >
      {items.map(({ key, label, subtext }, index) => {
        const selected = active === key;
        return (
          <button
            key={key}
            type="button"
            id={`${prefix}${key}-tab`}
            role="tab"
            aria-selected={selected}
            aria-controls={`${prefix}${key}`}
            tabIndex={selected ? 0 : -1}
            onClick={() => select(key)}
            onKeyDown={(event) => moveTab(event, index)}
            className={`relative min-w-0 border-b-2 px-2.5 py-2.5 text-left font-semibold hover:bg-wash ${compact ? "text-xs sm:px-3 sm:text-sm" : "text-base sm:px-5 sm:py-3.5 sm:text-lg"} ${selected ? "border-ok bg-ok-wash text-ok" : "border-transparent bg-sheet text-ink"}`}
          >
            {label}
            {subtext && <span className="mt-0.5 block text-xs font-normal text-dim">{subtext}</span>}
          </button>
        );
      })}
    </div>
  );
}

export function DevelopersPortal({ repo }: { repo: string }) {
  const mode = useSyncExternalStore(onHashChange, modeFromURL, defaultMode);
  return (
    <>
      <Tabs label="Integration mode" prefix="" items={modes} active={mode} select={selectMode} />
      <div id="mcp" role="tabpanel" aria-labelledby="mcp-tab" tabIndex={0} hidden={mode !== "mcp"}>
        <div className="flex min-w-0 flex-col gap-6 sm:gap-8">
          <Quickstart repo={repo} />
          <ToolExplorer repo={repo} />
        </div>
      </div>
      <div id="api" role="tabpanel" aria-labelledby="api-tab" tabIndex={0} hidden={mode !== "api"}>
        <RestConsole repo={repo} />
      </div>
    </>
  );
}

function CopyCode({ label, value, language, dark = false }: {
  label: string;
  value: string;
  language?: "json";
  dark?: boolean;
}) {
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
    <div className={`min-w-0 rounded-sm border ${dark ? "border-ink bg-ink text-paper shadow-[3px_3px_0_var(--shadow)]" : "border-edge bg-sheet"}`}>
      <div className={`flex min-h-10 items-center justify-between gap-2 border-b px-3 sm:px-4 ${dark ? "border-dim bg-body" : "border-line bg-wash"}`}>
        <p className="min-w-0 text-xs font-medium [overflow-wrap:anywhere]">{label}</p>
        <button
          type="button"
          aria-label={`Copy ${label}`}
          onClick={copyCode}
          className="my-1 min-h-10 shrink-0 sm:min-h-8 rounded-sm border border-edge bg-sheet px-2.5 text-xs font-medium text-ok hover:bg-ok-wash"
        >
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
      <pre className="min-w-0 px-3 py-3 text-[12px] leading-5 whitespace-pre-wrap [overflow-wrap:anywhere] sm:px-4 sm:text-[13px]">
        <code ref={code}>{language === "json" ? <HighlightedJSON value={value} /> : value}</code>
      </pre>
      <p role="status" className={copied || !feedback ? "sr-only" : "px-3 pb-3 text-xs"}>
        {feedback}
      </p>
    </div>
  );
}

function HighlightedJSON({ value }: { value: string }) {
  const tokens = value.split(/("(?:[^"\\]|\\.)*"|\b(?:\d+(?:\.\d+)?|true|false|null)\b)/g);
  return tokens.map((token, index) => {
    if (index % 2 === 0) return token;
    const isString = token.startsWith('"');
    const color = !isString ? "text-dimension" : tokens[index + 1]?.trimStart().startsWith(":") ? "text-ok" : "text-body";
    return <span key={index} className={color}>{token}</span>;
  });
}

function Quickstart({ repo }: { repo: string }) {
  const [client, setClient] = useState<Client>("gemini");
  return (
    <section aria-labelledby="quickstart-heading" className="grid items-start gap-3 lg:grid-cols-[minmax(0,0.55fr)_minmax(0,1.45fr)] lg:gap-6">
      <div className="flex flex-wrap items-baseline justify-between gap-2 lg:block">
        <h2 id="quickstart-heading" className={heading}>Connect a client</h2>
        <a className={`${docLink} inline-flex min-h-11 items-center text-xs lg:mt-2 lg:min-h-0`} href={`${repo}/blob/main/docs/MCP.md`}>Setup guide ↗</a>
      </div>
      <div className="min-w-0">
        <Tabs label="MCP clients" prefix="setup-" items={clients} active={client} select={setClient} compact />
        {clients.map(({ key }) => {
          const setup = setupExamples[key];
          return (
            <div id={`setup-${key}`} key={key} role="tabpanel" aria-labelledby={`setup-${key}-tab`} hidden={client !== key}>
              <div className="mt-2">
                <CopyCode label={setup.label} value={setup.code} dark />
              </div>
              {setup.note && <p className="duo mt-2 text-xs text-dim">{setup.note}</p>}
              {key === "claude" && <p className="mt-1 text-xs"><code>{mcpEndpoint}</code></p>}
            </div>
          );
        })}
      </div>
    </section>
  );
}

function ToolExplorer({ repo }: { repo: string }) {
  const [query, setQuery] = useState("");
  const [group, setGroup] = useState<ToolGroup | "">("");
  const [active, setActive] = useState<ToolName>("predict_origin");
  const filtered = filterTools(query, group);
  const selected = filtered.find(([name]) => name === active) ?? filtered[0];

  return (
    <section aria-labelledby="tool-heading" className="border-t border-line pt-5 sm:pt-6">
      <div className="mb-3 flex flex-wrap items-baseline justify-between gap-2">
        <h2 id="tool-heading" className={heading}>Pick a tool</h2>
        <a className={`${docLink} inline-flex min-h-11 items-center text-xs sm:min-h-0`} href={`${repo}/blob/main/docs/MCP.md#tools`}>Tool reference ↗</a>
      </div>
      <div className="grid items-start gap-4 lg:grid-cols-[minmax(260px,0.72fr)_minmax(0,1.28fr)]">
        <div className={panel}>
          <div className="space-y-2 border-b border-line bg-wash p-3">
            <input
              type="search"
              aria-label="Search tools"
              className={field}
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Search tools"
            />
            <div role="group" aria-label="Tool category" className="flex flex-wrap gap-1.5">
              {categories.map(([key, text]) => (
                <button
                  type="button"
                  key={key}
                  aria-pressed={group === key}
                  onClick={() => setGroup(key)}
                  className={`min-h-11 rounded-full border border-edge px-3 sm:min-h-9 sm:px-2.5 text-xs hover:bg-ok-wash ${group === key ? "bg-ok-wash text-ok" : "bg-sheet text-ink"}`}
                >
                  {text}
                </button>
              ))}
            </div>
          </div>
          <div role="region" aria-label="Matching tools" tabIndex={0} className="max-h-52 divide-y divide-line overflow-y-auto lg:max-h-[27rem]">
            {filtered.map(([name]) => (
              <button
                type="button"
                key={name}
                aria-pressed={selected?.[0] === name}
                aria-controls="tool-inspector"
                onClick={() => setActive(name)}
                className={`block min-h-10 w-full border-l-2 px-3 py-2 text-left text-xs [overflow-wrap:anywhere] hover:bg-wash ${selected?.[0] === name ? "border-ok bg-ok-wash font-medium text-ok" : "border-transparent text-ink"}`}
              >
                {name}
              </button>
            ))}
            {!filtered.length && <p className="duo px-3 py-3 text-sm text-dim">No matching tools.</p>}
          </div>
        </div>

        <div id="tool-inspector" className="min-w-0">
          <p className="sr-only" role="status">{filtered.length} tools match{selected ? `, showing ${selected[0]}` : ""}</p>
          {selected && <ToolInspector key={selected[0]} tool={selected} />}
        </div>
      </div>
    </section>
  );
}

function ToolInspector({ tool: [name, group, rate, input, output] }: { tool: (typeof tools)[number] }) {
  const example = toolExamples[name];
  return (
    <div className={`${panel} space-y-3 p-3 sm:p-4`}>
      <div className="flex flex-wrap items-center justify-between gap-1.5">
        <h3 className="min-w-0 text-base font-semibold text-ink [overflow-wrap:anywhere] sm:text-lg">{name}</h3>
        <span className="rounded-sm border border-line bg-wash px-2 py-1 text-xs text-dim">{rates[rate]}</span>
      </div>
      <p className="duo text-sm text-dim">{input} <span aria-hidden="true">→</span><span className="sr-only"> returns </span> {output}</p>
      <div className="grid items-start gap-3 xl:grid-cols-2">
        <CopyCode label="JSON input" value={json(example.input)} language="json" />
        <CopyCode label="Structured output · example" value={json(example.output)} language="json" />
      </div>
      {example.note && <p className="duo text-xs text-dim">{example.note}</p>}
      {group === "tariff" && <p className="duo text-xs text-dim">For U.S. Part 102, supply tariff codes to <code>predict_origin</code>.</p>}
    </div>
  );
}

const statuses = [
  { label: "401 Unauthorized", color: "border-fail/40 bg-fail-wash text-fail" },
  { label: "429 Rate limit", color: "border-ok/40 bg-ok-wash text-ok" },
  { label: "503 Daily cap", color: "border-ask-line border-dashed bg-ask-wash text-ask" },
];

function RestConsole({ repo }: { repo: string }) {
  const [endpoint, setEndpoint] = useState<Endpoint>("predict");
  const example = endpoints[endpoint];

  return (
    <section aria-labelledby="rest-heading">
      <h2 id="rest-heading" className={`${heading} mb-3`}>REST examples</h2>
      <div className="grid items-start gap-4 md:grid-cols-2">
        <div id="keys" className="flex min-w-0 flex-col gap-2">
          <label className="block text-xs font-medium text-dim">
            Endpoint
            <select className={`${field} mt-1`} value={endpoint} onChange={(event) => setEndpoint(event.target.value as Endpoint)}>
              {Object.entries(endpoints).map(([key, entry]) => <option key={key} value={key}>{entry.label}</option>)}
            </select>
          </label>
          <code className="self-start rounded-sm border border-edge bg-sheet px-2 py-1 text-xs">{keyHeader}</code>
          <CopyCode key={endpoint} label="cURL request" value={example.command} />
          {example.note && <p className="duo text-xs text-dim">{example.note}</p>}
        </div>
        <CopyCode key={endpoint} label={`${example.status} response · example`} value={json(example.output)} language="json" />
      </div>
      <div id="limits" role="group" aria-label="Rate limits" className="mt-5 flex flex-wrap items-center gap-2 border-y border-line py-3 text-xs">
        <span className="rounded-sm border border-line bg-sheet px-2 py-1.5"><strong>{rates.light}</strong> predict</span>
        <span className="rounded-sm border border-line bg-sheet px-2 py-1.5"><strong>{rates.heavy}</strong> plan search</span>
        {statuses.map(({ label, color }) => <span key={label} className={`rounded-sm border px-2 py-1.5 ${color}`}>{label}</span>)}
        <a className={docLink} href={`${repo}/blob/main/docs/api/SECURITY.md`}>Limits and errors ↗</a>
      </div>
    </section>
  );
}
