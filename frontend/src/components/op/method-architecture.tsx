"use client";

import { useRef, useState, type KeyboardEvent } from "react";
import { blob, decisionHref } from "@/lib/method";
import { kobaltSources, methodExperiments, methodStages } from "@/lib/method-inspector";
import { Term } from "./term";

// Plain names first; the API field name is one hover or tap away (#685).
function Schema({ title, fields }: { title: string; fields: ReadonlyArray<readonly [string, string, string]> }) {
  return (
    <div className="mi-schema">
      <h4>{title}</h4>
      <dl>
        {fields.map(([name, kind, field]) => (
          <div key={name}>
            <dt><Term explain={`API field: ${field}`}>{name}</Term></dt>
            <dd>{kind}</dd>
          </div>
        ))}
      </dl>
    </div>
  );
}

export function MethodArchitecture() {
  const [active, setActive] = useState<number>(0);
  const nodes = useRef<Array<HTMLButtonElement | null>>([]);
  const inspector = useRef<HTMLDivElement | null>(null);
  const stage = methodStages[active];

  function moveFocus(event: KeyboardEvent<HTMLButtonElement>, index: number) {
    const last = methodStages.length - 1;
    let next: number;
    switch (event.key) {
      case "ArrowDown": next = index === last ? 0 : index + 1; break;
      case "ArrowUp": next = index === 0 ? last : index - 1; break;
      case "Home": next = 0; break;
      case "End": next = last; break;
      default: return;
    }
    event.preventDefault();
    setActive(next);
    nodes.current[next]?.focus();
  }

  return (
    <section id="how-its-built" className="mi-section" aria-labelledby="mi-architecture">
      <header className="mi-section-title">
        <h2 id="mi-architecture">System architecture</h2>
        <span>U.S. origin and production moves</span>
      </header>
      <div className="mi-architecture">
        <div className="mi-node-rail" role="tablist" aria-label="Architecture stages" aria-orientation="vertical">
          {methodStages.map((item, index) => (
            <button
              key={item.id}
              ref={(element) => { nodes.current[index] = element; }}
              type="button"
              role="tab"
              id={"mi-stage-" + item.id}
              aria-selected={active === index}
              aria-controls="architecture-detail"
              tabIndex={active === index ? 0 : -1}
              data-tone={item.tone}
              className="mi-node"
              onKeyDown={(event) => moveFocus(event, index)}
              onClick={(event) => {
                setActive(index);
                if (event.detail > 0 && window.matchMedia("(max-width: 767px)").matches) {
                  inspector.current?.scrollIntoView({ block: "nearest" });
                }
              }}
            >
              <span aria-hidden="true" className="mi-node-index">{String(index + 1).padStart(2, "0")}</span>
              <span className="mi-node-text">
                <span className="mi-node-title">{item.title}</span>
                <span className="mi-node-label">{item.label}</span>
              </span>
              <span aria-hidden="true" className="mi-node-arrow">→</span>
            </button>
          ))}
        </div>
        <div
          ref={inspector}
          id="architecture-detail"
          role="tabpanel"
          aria-labelledby={"mi-stage-" + stage.id}
          tabIndex={0}
          className="mi-inspector"
          data-tone={stage.tone}
        >
          <div className="mi-inspector-heading">
            <h3>{stage.title}</h3>
            <span className="mi-badge">{stage.label}</span>
          </div>
          <div className="mi-schema-pair">
            <Schema title="Input" fields={stage.input} />
            <Schema title="Output" fields={stage.output} />
          </div>
          <section className="mi-enforced" aria-labelledby="mi-enforced">
            <h4 id="mi-enforced">Guardrail</h4>
            <p className="read">{stage.guard}</p>
          </section>
          <section className="mi-trace" aria-labelledby="mi-trace">
            <div className="mi-trace-heading">
              <h4 id="mi-trace">Recorded run: Kobalt heat gun</h4>
              <a className="mi-link" href={blob(kobaltSources[stage.source])}>{stage.recording} <span aria-hidden="true">↗</span></a>
            </div>
            <dl className="mi-trace-values">
              {stage.trace.map(([label, value]) => (
                <div key={label}>
                  <dt>{label}</dt>
                  <dd>{value}</dd>
                </div>
              ))}
            </dl>
          </section>
          {stage.id === "ask" && (
            <section className="mi-enforced" aria-labelledby="mi-adk-eval">
              <h4 id="mi-adk-eval">Agent test (built with Google’s ADK)</h4>
              <p className="read">Run 1 passed the tool-use checks on 9 of 12 products. Production sign-in blocked the second anonymous run.</p>
              <a className="mi-link" href={blob("docs/EVALUATION.md#12-agent-evaluation-the-adk-agent-on-12-products-22-nov-2026-d450-d451")}>
                ADK evaluation details <span aria-hidden="true">↗</span>
              </a>
            </section>
          )}
        </div>
      </div>
    </section>
  );
}

function ExperimentCard({ record }: { record: (typeof methodExperiments)[number] }) {
  const [fixed, setFixed] = useState(false);
  return (
    <article className="mi-card">
      <h3>{record.title}</h3>
      <div className="mi-switch" role="group" aria-label={record.title + " comparison"}>
        <button type="button" aria-pressed={!fixed} onClick={() => setFixed(false)}>Failed approach</button>
        <button type="button" aria-pressed={fixed} onClick={() => setFixed(true)}>Production fix</button>
      </div>
      <div aria-live="polite" className="mi-experiment-result" data-tone={fixed ? "evidence" : undefined}>
        <p className="read">{fixed ? record.fix : record.failed}</p>
        <a className="mi-link" href={decisionHref(fixed ? record.fixDecision : record.failedDecision)}>Decision <span aria-hidden="true">↗</span></a>
      </div>
    </article>
  );
}

export function MethodExperiments() {
  return (
    <section id="negative" className="mi-section" aria-labelledby="mi-experiments">
      <header className="mi-section-title"><h2 id="mi-experiments">What failed and how we fixed it</h2></header>
      <div className="mi-triple-grid">
        {methodExperiments.map((record) => <ExperimentCard key={record.title} record={record} />)}
      </div>
    </section>
  );
}
