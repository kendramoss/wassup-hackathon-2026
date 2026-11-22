"use client";

import { useState } from "react";
import { blob, decisionHref } from "@/lib/method";

const stages = [
  {
    id: "router",
    title: "Router",
    model: "Gemini 3.8 Flash",
    summary: "Reads the product facts and selects the U.S. origin test.",
    flow: "Product facts → origin regime",
    detail: "The main path uses substantial transformation. Textile, pharmaceutical and Part 102 marking cases have specialist routes.",
    source: "Routing design",
    href: decisionHref("D233"),
  },
  {
    id: "origin",
    title: "Origin engine",
    model: "Gemini 4 Argon + BM25",
    summary: "Retrieves the five most similar eligible CBP rulings and predicts the likely origin.",
    flow: "Facts + earlier rulings → prediction with reasons",
    detail: "Each retrieved ruling must predate the case. The engine sees earlier facts and holdings. The test case supplies its facts alone.",
    source: "Retrieval and date filter",
    href: blob("docs/ARCHITECTURE.md"),
  },
  {
    id: "triage",
    title: "Confidence triage",
    model: "Confidence + scope",
    summary: "Accepts in-scope predictions above the bar. Lower-confidence cases go to review, with questions where more facts would help.",
    flow: "Prediction → accept, ask for facts, or review",
    detail: "The benchmark used stated confidence ≥ 0.92. The main engine now uses calibrated confidence ≥ 0.90, fitted on development rulings. It selected the same cases on that benchmark.",
    source: "Calibration decision",
    href: decisionHref("D161"),
    fold: "How the bar was set",
  },
  {
    id: "planner",
    title: "Operation planner",
    model: "Go search",
    summary: "Moves groups of parts and operations between listed plants, then predicts origin again for each candidate.",
    flow: "Plant capabilities + target → candidate moves",
    detail: "Templates define which work moves together. Part 102 cases use their specialist engine for each move. Other plan searches use the general engine.",
    source: "Plan search and checks",
    href: "#plans",
  },
  {
    id: "verifier",
    title: "Dual-sided verifier",
    model: "Gemini 4 Argon",
    summary: "Searches earlier CBP rulings for both support and counterexamples.",
    flow: "Candidate move → show or set aside",
    detail: "A shown move needs a supporting ruling and a verifier score of at least 0.92. An on-point counter-ruling can keep a move below that bar.",
    source: "Evidence gate",
    href: decisionHref("D74"),
  },
  {
    id: "ask",
    title: "Ask agent",
    model: "Gemini · ADK for Go",
    summary: "Answers questions about a saved case, inspects rulings and re-runs the origin prediction on changed facts.",
    flow: "Question or what-if → answer grounded in case evidence",
    detail: "The case agent calls tools for facts, earlier rulings and re-prediction. Explore any scenario before saving it to the case.",
    source: "Case agent tools",
    href: blob("backend/internal/caseagent/agent.go"),
  },
];

export function MethodArchitecture() {
  const [selected, setSelected] = useState(0);
  const stage = stages[selected];

  return (
    <section id="how-its-built" aria-labelledby="architecture-heading" className="mt-10 scroll-mt-6 sm:mt-14">
      <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1 border-b border-edge pb-3">
        <h2 id="architecture-heading" className="text-[24px] font-medium leading-tight tracking-[-0.04em] sm:text-[28px]">
          System architecture
        </h2>
        <p className="text-[12px] text-dim">U.S. origin and production moves</p>
      </div>
      <div className="mt-4 grid items-stretch gap-4 md:grid-cols-[minmax(0,2fr)_minmax(0,3fr)] md:gap-5">
        <ol aria-label="Architecture stages" className="grid min-w-0 grid-cols-2 gap-2 min-[500px]:grid-cols-3 md:grid-cols-1 md:gap-1.5">
          {stages.map((item, index) => (
            <li key={item.id} className="min-w-0">
              <button
                type="button"
                aria-pressed={selected === index}
                aria-controls="architecture-detail"
                onClick={() => setSelected(index)}
                className="group flex h-full min-h-16 w-full min-w-0 cursor-pointer items-start gap-2 rounded-[2px] border border-edge bg-sheet px-2.5 py-2.5 text-left hover:bg-wash aria-pressed:border-ok aria-pressed:bg-ok-wash sm:gap-3 sm:px-3 md:min-h-13 md:items-center"
              >
                <span aria-hidden="true" className="mt-0.5 grid size-6 shrink-0 place-items-center rounded-[2px] border border-edge text-[11px] tabular-nums text-dim group-aria-pressed:border-ok group-aria-pressed:bg-ok group-aria-pressed:text-white md:mt-0">
                  {index + 1}
                </span>
                <span className="min-w-0">
                  <span className="block text-[13px] font-semibold leading-tight text-ink sm:text-[14px]">{item.title}</span>
                  <span className="mt-1 block text-[11px] leading-tight text-dim">{item.model}</span>
                </span>
                <span aria-hidden="true" className="ml-auto hidden text-ok group-aria-pressed:md:block">→</span>
              </button>
            </li>
          ))}
        </ol>
        <div
          id="architecture-detail"
          aria-live="polite"
          className="flex min-h-60 min-w-0 flex-col rounded-[2px] border border-edge bg-sheet p-4 shadow-[4px_4px_0_var(--shadow)] sm:p-6"
        >
          <div key={stage.id} className="flex flex-1 flex-col items-start">
            <p className="text-[12px] font-medium text-ok">Stage {selected + 1} of {stages.length}</p>
            <h3 className="mt-2 text-[23px] font-semibold leading-tight tracking-[-0.045em] sm:text-[30px]">{stage.title}</h3>
            <p className="read mt-2 max-w-[56ch] text-[15px] leading-snug text-body sm:text-[17px]">{stage.summary}</p>
            <p className="mt-4 rounded-[2px] border border-line bg-paper px-2.5 py-2 text-[12px] leading-snug text-ink">{stage.flow}</p>
            {stage.fold ? (
              <details className="mt-4 w-full border-t border-line">
                <summary className="flex min-h-11 cursor-pointer items-center text-[13px] font-medium text-ok">{stage.fold}</summary>
                <p className="read pb-3 text-[13px] leading-snug text-body">{stage.detail}</p>
              </details>
            ) : (
              <p className="read mt-4 max-w-[63ch] text-[13px] leading-snug text-dim">{stage.detail}</p>
            )}
            <div className="mt-auto pt-3">
              <a href={stage.href} className="inline-flex min-h-11 items-center text-[12px] text-ok underline underline-offset-4">{stage.source} <span aria-hidden="true" className="ml-1">→</span></a>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
