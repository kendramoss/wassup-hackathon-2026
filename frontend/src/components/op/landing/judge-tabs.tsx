"use client";

import Link from "next/link";
import { useId, useState, type KeyboardEvent, type ReactNode } from "react";
import "./judge-tabs.css";

type TabItem = {
  label: string;
  subtitle?: string;
  content: ReactNode;
};

export function JudgeTabs({
  label,
  items,
  className,
}: {
  label: string;
  items: readonly TabItem[];
  className?: string;
}) {
  const id = useId();
  const [selected, setSelected] = useState(0);

  function moveFocus(event: KeyboardEvent<HTMLButtonElement>, current: number) {
    let next: number;
    switch (event.key) {
      case "ArrowLeft":
        next = (current + items.length - 1) % items.length;
        break;
      case "ArrowRight":
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

  return (
    <div className={className ? `judgeTabs ${className}` : "judgeTabs"}>
      <div className="judgeTabList" role="tablist" aria-label={label} aria-orientation="horizontal">
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
          {item.subtitle ? <p className="judgePanelModel">{item.subtitle}</p> : null}
          {item.content}
        </div>
      ))}
    </div>
  );
}

const geminiCommand = "gemini mcp add --transport http unde https://undelabs.dev/mcp";

export function JudgePersonas({
  demoHref = "/cases",
  compareHref = "/cases",
}: {
  demoHref?: string;
  compareHref?: string;
}) {
  return (
    <section className="judgePersonas" id="can" aria-labelledby="judge-personas-heading">
      <h2 id="judge-personas-heading">What you can do</h2>
      <JudgeTabs
        label="Choose your role"
        items={[
          {
            label: "Trade Compliance Leads",
            content: (
              <div className="judgePersonaContent">
                <p>Import a parts list, trace likely origin to CBP rulings, and send a draft to your broker.</p>
                <div className="judgePersonaActions">
                  <Link href="/cases/new">Start a case <span aria-hidden="true">→</span></Link>
                  <Link href={demoHref}>See an origin result <span aria-hidden="true">→</span></Link>
                </div>
              </div>
            ),
          },
          {
            label: "Supply Chain & Execs",
            content: (
              <div className="judgePersonaContent">
                <p>Compare production moves by likely origin, duty, and added manufacturing cost.</p>
                <div className="judgePersonaActions">
                  <Link href={compareHref}>Compare countries <span aria-hidden="true">→</span></Link>
                  <a href="#impact">See the illustrative savings <span aria-hidden="true">↓</span></a>
                </div>
              </div>
            ),
          },
          {
            label: "Developers (MCP & API)",
            content: (
              <div className="judgePersonaContent">
                <p>Connect Gemini CLI or call the REST API from your own tools.</p>
                <pre className="judgePersonaCommand" aria-label="Gemini CLI setup command"><code>{geminiCommand}</code></pre>
                <div className="judgePersonaActions">
                  <Link href="/developers#mcp">MCP setup <span aria-hidden="true">→</span></Link>
                  <Link href="/developers#api">REST API <span aria-hidden="true">→</span></Link>
                </div>
              </div>
            ),
          },
        ]}
      />
    </section>
  );
}
