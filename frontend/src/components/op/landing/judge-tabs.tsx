"use client";

import Link from "next/link";
import { useId, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { RulingLink } from "../ui";
import "./judge-tabs.css";

type TabItem = {
  label: string;
  subtitle?: string;
  content: ReactNode;
  actions?: ReactNode;
};

export function JudgeTabs({
  label,
  items,
  className,
  orientation = "horizontal",
}: {
  label: string;
  items: readonly TabItem[];
  className?: string;
  orientation?: "horizontal" | "vertical";
}) {
  const id = useId();
  const [selected, setSelected] = useState(0);
  const vertical = orientation === "vertical";

  function moveFocus(event: KeyboardEvent<HTMLButtonElement>, current: number) {
    let next: number;
    switch (event.key) {
      case vertical ? "ArrowUp" : "ArrowLeft":
        next = (current + items.length - 1) % items.length;
        break;
      case vertical ? "ArrowDown" : "ArrowRight":
        next = (current + 1) % items.length;
        break;
      case "Home":
        next = 0;
        break;
      case "End":
        next = items.length - 1;
        break;
      default:
        return;
    }
    event.preventDefault();
    setSelected(next);
    event.currentTarget.parentElement?.querySelectorAll<HTMLButtonElement>("button[role=tab]")[next]?.focus();
  }

  const tabs = (
    <div className="judgeTabList" role="tablist" aria-label={label} aria-orientation={orientation}>
      {items.map((item, index) => (
        <button
          key={item.label}
          className="judgeTab"
          id={`${id}-tab-${index}`}
          type="button"
          role="tab"
          aria-controls={`${id}-panel-${index}`}
          aria-selected={selected === index}
          tabIndex={selected === index ? 0 : -1}
          onClick={() => setSelected(index)}
          onKeyDown={(event) => moveFocus(event, index)}
        >
          <span>{item.label}</span>
          {item.subtitle ? <span className="judgeTabModel">{item.subtitle}</span> : null}
        </button>
      ))}
    </div>
  );

  return (
    <div className={["judgeTabs", vertical && "judgeTabsVertical", className].filter(Boolean).join(" ")}>
      {vertical ? (
        <div className="judgeTabSidebar">
          {tabs}
          {items[selected]?.actions}
        </div>
      ) : tabs}
      {items.map((item, index) => (
        <div
          key={item.label}
          className="judgeTabPanel"
          id={`${id}-panel-${index}`}
          role="tabpanel"
          aria-labelledby={`${id}-tab-${index}`}
          tabIndex={selected === index ? 0 : -1}
          hidden={selected !== index}
        >
          {!vertical && item.subtitle ? <p className="judgePanelModel">{item.subtitle}</p> : null}
          {item.content}
        </div>
      ))}
    </div>
  );
}

const geminiCommand = "gemini mcp add --transport http unde https://undelabs.dev/mcp";

function GeminiTerminal() {
  const code = useRef<HTMLElement>(null);
  const [status, setStatus] = useState<"copied" | "manual" | "unavailable">();

  async function copyCommand() {
    try {
      await navigator.clipboard.writeText(geminiCommand);
      setStatus("copied");
    } catch {
      const selection = window.getSelection();
      if (!selection || !code.current) {
        setStatus("unavailable");
        return;
      }
      const range = document.createRange();
      range.selectNodeContents(code.current);
      selection.removeAllRanges();
      selection.addRange(range);
      setStatus("manual");
    }
  }

  return (
    <article className="judgePersonaPreview">
      <header className="judgePersonaPreviewHead">
        <h3>Gemini CLI</h3>
        <span>Hosted MCP</span>
      </header>
      <div className="judgePersonaTerminal">
        <div className="judgePersonaTerminalTop">
          <p>Connect the server</p>
          <button type="button" aria-label="Copy Gemini CLI setup command" onClick={copyCommand}>
            {status === "copied" ? "Copied" : "Copy"}
          </button>
        </div>
        <pre className="judgePersonaCommand" aria-label="Gemini CLI setup command">
          <code ref={code}>{geminiCommand}</code>
        </pre>
        <p role="status" className={status === "manual" || status === "unavailable" ? "judgePersonaCopyFeedback" : "sr-only"}>
          {status === "copied" && "Gemini CLI setup command copied."}
          {status === "manual" && "Command selected. Use your device’s copy command."}
          {status === "unavailable" && "Select the command to copy it manually."}
        </p>
      </div>
      <div className="judgePersonaPreviewFoot">
        <Link href="/developers#tool-heading">Explore MCP tools →</Link>
        <Link href="/developers#api">REST API →</Link>
      </div>
    </article>
  );
}

export function JudgePersonas({
  demoHref = "/cases",
  compareHref = "/cases",
}: {
  demoHref?: string;
  compareHref?: string;
}) {
  return (
    <section className="judgePersonas" id="can" aria-labelledby="judge-personas-heading">
      <h2 id="judge-personas-heading">What Unde does</h2>
      <JudgeTabs
        label="Choose your role"
        orientation="vertical"
        items={[
          {
            label: "Trade Compliance Leads",
            actions: (
              <div className="judgePersonaActions">
                <p>Compare an origin estimate with the published ruling.</p>
                <Link href="/cases/new">Start a case →</Link>
              </div>
            ),
            content: (
              <article className="judgePersonaPreview">
                <header className="judgePersonaPreviewHead">
                  <h3>Origin evidence</h3>
                  <span className="judgePersonaTag">Needs more facts</span>
                </header>
                <p className="judgePersonaSubject">Kobalt heat gun</p>
                <dl className="judgePersonaEvidence">
                  <div><dt>Unde prediction</dt><dd>China</dd></div>
                  <div><dt>CBP’s ruling</dt><dd>China <RulingLink id="N328954" className="lp-ruling-pill" /></dd></div>
                </dl>
                <div className="judgePersonaPreviewFoot">
                  <span>Not a ruling.</span>
                  <Link href={demoHref}>Open the origin result →</Link>
                </div>
              </article>
            ),
          },
          {
            label: "Supply Chain & Execs",
            actions: (
              <div className="judgePersonaActions">
                <p>Compare how a production move changes duty and manufacturing cost.</p>
                <a href="#impact">See the savings calculation ↓</a>
              </div>
            ),
            content: (
              <article className="judgePersonaPreview">
                <header className="judgePersonaPreviewHead">
                  <h3>Heat-gun duty comparison</h3>
                  <span className="judgePersonaTag">Illustrative</span>
                </header>
                <div className="judgePersonaCompare">
                  <div>
                    <span>China origin</span>
                    <strong>41.7%</strong>
                  </div>
                  <span aria-hidden="true">→</span>
                  <div>
                    <span>Thailand origin</span>
                    <strong>16.7%</strong>
                  </div>
                </div>
                <div className="judgePersonaPreviewFoot">
                  <Link href={compareHref}>Compare countries →</Link>
                </div>
              </article>
            ),
          },
          {
            label: "Developers (MCP & API)",
            actions: (
              <div className="judgePersonaActions">
                <p>Connect your agent to Unde with MCP or call the REST API.</p>
                <Link href="/developers#mcp">View integration docs →</Link>
              </div>
            ),
            content: <GeminiTerminal />,
          },
        ]}
      />
    </section>
  );
}
