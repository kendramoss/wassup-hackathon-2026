"use client";

import Link from "next/link";
import { Fragment, useEffect, useRef, useState, useSyncExternalStore, type ReactNode } from "react";
import ReactMarkdown from "react-markdown";
import { api, type CaseResponse, type ChatTurn, type PrecedentRef, type WhatIf } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { BAR, conf, formatDate } from "@/lib/format";
import { Elapsed, ErrorBox } from "./chrome";
import { Button, Country, Drawer, Flag, RulingLink, Ruler } from "./ui";
import { planTarget } from "./plan-target";

type Step = { tool?: string; arg?: string };
type Extra = { whatIfs?: WhatIf[]; steps?: Step[]; calls?: number };
type Turn = ChatTurn & Extra;

// The conversation lives on the case (#110), including each turn's what-ifs
// and steps (#112). A question still being answered, and ruling names the
// case has not loaded yet, live here for this visit.
const inflight = new Map<string, { q: string; since: number }>();
const listeners = new Set<() => void>();
const emit = () => listeners.forEach((l) => l());
const subscribe = (l: () => void) => {
  listeners.add(l);
  return () => {
    listeners.delete(l);
  };
};
const extras = new Map<string, Extra>();
const details = new Map<string, PrecedentRef>();
const turnKey = (caseId: string, t: ChatTurn) => `${caseId}|${t.at ?? ""}|${t.question ?? ""}`;

// "Ask about this case" (DESIGN.md, #46): a right-hand drawer on the case
// screens. Earlier turns collapse to one line; the newest shows in full.
export function AskDrawer({ c, onClose, onChat }: { c: CaseResponse; onClose: () => void; onChat: (chat: ChatTurn[]) => void }) {
  const id = c.id!;
  const chat = c.chat ?? [];
  const turns: Turn[] = chat.map((t) => {
    const extra = extras.get(turnKey(id, t));
    return { ...t, ...extra, whatIfs: extra?.whatIfs ?? t.what_ifs, steps: extra?.steps ?? t.steps };
  });
  for (const r of [...(c.precedents_detail ?? []), ...(c.cited_detail ?? [])]) if (r.id && !details.has(r.id)) details.set(r.id, r);
  const pending = useSyncExternalStore(
    subscribe,
    () => inflight.get(id) ?? null,
    () => null,
  );
  const [question, setQuestion] = useState("");
  const [error, setError] = useState<unknown>(null);
  const [open, setOpen] = useState<number | null>(null);
  const [confirming, setConfirming] = useState(false);
  const [announced, setAnnounced] = useState("");
  const input = useRef<HTMLInputElement>(null);
  const log = useRef<HTMLDivElement>(null);

  useEffect(() => {
    input.current?.focus();
  }, []);

  // Bring the newest question to the top of the log, so a short answer
  // isn't scrolled out of view under empty space (#87).
  useEffect(() => {
    const box = log.current;
    const latest = box?.querySelectorAll<HTMLElement>("[data-latest]");
    const el = latest?.[latest.length - 1];
    if (box && el) box.scrollTop += el.getBoundingClientRect().top - box.getBoundingClientRect().top - 16;
  }, [turns.length, pending]);

  // Not aborted when the drawer closes: the answer is saved either way, and
  // it shows when the drawer opens again (#110).
  const ask = async (q: string) => {
    q = q.trim();
    if (!q || inflight.has(id)) return;
    setError(null);
    setConfirming(false);
    setQuestion("");
    inflight.set(id, { q, since: Date.now() });
    emit();
    try {
      const r = await api.ask(id, q);
      const reply = r.reply ?? {};
      for (const x of r.cited_detail ?? []) if (x.id) details.set(x.id, x);
      const last = r.chat?.at(-1);
      if (last) extras.set(turnKey(id, last), { whatIfs: reply.what_ifs, steps: reply.steps, calls: reply.model_calls });
      setOpen(null);
      setAnnounced(`Answer: ${(reply.answer ?? "").slice(0, 300)}`);
      inflight.delete(id);
      emit();
      onChat(r.chat ?? []);
    } catch (e) {
      inflight.delete(id);
      emit();
      setError(e);
      setQuestion(q);
    }
  };

  const reset = async () => {
    setConfirming(false);
    try {
      await api.resetChat(id);
      onChat([]);
    } catch (e) {
      setError(e);
    }
  };

  // Starter questions: never one already asked, and none while an answer is
  // pending or an error waits for a retry (#117).
  const asked = new Set(turns.map((t) => (t.question ?? "").trim().toLowerCase()));
  const suggestions = pending || error ? [] : starterQuestions(c).filter((s) => !asked.has(s.toLowerCase()));

  return (
    <Drawer
      label={copy.result.ask}
      onClose={onClose}
      header={
        <>
          <b className="font-medium text-ink">◆ {copy.result.ask}</b>
          {/* Clearing is final, so it asks first, and waits for a pending answer (#116). */}
          {turns.length > 0 && !pending && (
            <span className="ml-auto flex items-center gap-3 text-xs normal-case tracking-normal">
              {confirming ? (
                <>
                  <span className="text-dim">
                    Clear all {turns.length} {turns.length === 1 ? "question" : "questions"}?
                  </span>
                  <button type="button" className="text-fail hover:underline" onClick={reset}>
                    Clear
                  </button>
                  <button type="button" className="text-dim hover:underline" onClick={() => setConfirming(false)}>
                    Keep
                  </button>
                </>
              ) : (
                <button type="button" className="text-ok hover:underline" onClick={() => setConfirming(true)}>
                  Clear conversation
                </button>
              )}
            </span>
          )}
        </>
      }
    >
      <div className="flex h-full flex-col">
        {/* Screen readers hear only the newest answer, not the whole log (#101). */}
        <p className="sr-only" aria-live="polite">
          {pending ? "Reading the case and earlier rulings…" : announced}
        </p>
        <div ref={log} className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto px-5 py-4">
          {turns.length === 0 && !pending && (
            <p className="duo text-[13px] text-dim">
              Ask why the engine landed where it did, or try a change (&ldquo;what if …&rdquo;). The agent reads this case and earlier CBP rulings dated before{" "}
              {formatDate(c.date)}; a what-if re-runs the origin engine on the changed facts.
            </p>
          )}
          {turns.map((t, i) =>
            i < turns.length - 1 && open !== i ? (
              <button key={i} type="button" onClick={() => setOpen(i)} className="duo flex gap-2 border-b border-line pb-3 text-left text-xs text-dim hover:text-ink">
                <span aria-hidden="true">▸</span>
                <span>
                  <b className="font-medium text-ink">Earlier:</b> {t.question} · <span className="text-ok">{citedLabel(t.cited_rulings?.length ?? 0)} · show answer</span>
                </span>
              </button>
            ) : (
              <div key={i} data-latest={i === turns.length - 1 ? "" : undefined}>
                <TurnView t={t} caseId={id} />
              </div>
            ),
          )}
          {pending && (
            <div data-latest="">
              <Q>{pending.q}</Q>
              <div className="mt-2.5 border border-dashed border-guide px-3.5 py-3 text-[12.5px] text-dim" role="status">
                <span className="float-right">
                  <Elapsed since={pending.since} />
                </span>
                <b className="font-medium text-ink">Reading the case and earlier rulings…</b>
                <br />
                usually 15–40 s · you can close this and come back; the answer is kept
                <div className="relative mt-2.5 h-[3px] overflow-hidden bg-wash">
                  <i className="absolute inset-y-0 w-1/3 animate-[slide_1.6s_ease-in-out_infinite] bg-ok" />
                </div>
              </div>
            </div>
          )}
          {error ? <ErrorBox error={error} onRetry={() => ask(question)} /> : null}
        </div>

        {suggestions.length > 0 && (
          <div className="flex flex-wrap gap-2 px-5 pb-3">
            {suggestions.map((s) => (
              <button key={s} type="button" onClick={() => ask(s)} className="border border-line bg-paper px-2 py-1 text-[11.5px] text-dim hover:border-edge hover:text-ink">
                {s}
              </button>
            ))}
          </div>
        )}
        <form
          className="flex gap-2.5 border-t border-dashed border-guide px-5 py-3"
          onSubmit={(e) => {
            e.preventDefault();
            ask(question);
          }}
        >
          <input
            ref={input}
            aria-label="Your question"
            maxLength={2000}
            className="min-w-0 flex-1 rounded-[2px] border border-line bg-white px-3 py-2 text-[13px] placeholder:text-faint disabled:bg-paper"
            placeholder={pending ? "Wait for the answer…" : "Ask about this case, or “what if …”"}
            value={question}
            disabled={Boolean(pending)}
            onChange={(e) => setQuestion(e.target.value)}
          />
          <Button type="submit" disabled={Boolean(pending) || !question.trim()}>
            Ask
          </Button>
        </form>
      </div>
    </Drawer>
  );
}

function citedLabel(n: number): string {
  return n === 0 ? "no rulings cited" : n === 1 ? "1 ruling cited" : `${n} rulings cited`;
}

// Starter questions come from the case: the plan target, and the first
// part made in the predicted origin (the one a move would shift).
function starterQuestions(c: CaseResponse): string[] {
  const origin = c.prediction?.origin_country;
  const target = planTarget(c);
  const part = c.product?.components?.find((p) => p.made_in && p.made_in === origin);
  const name = part?.name;
  // "Motor" reads as "the motor"; keep "PCBA" as is.
  const the = name && /^[A-Z][a-z]/.test(name) ? name[0].toLowerCase() + name.slice(1) : name;
  const qs: string[] = [];
  if (target) qs.push(`Why not ${target}?`);
  else if (origin) qs.push(`Why ${origin}?`);
  if (target && the) qs.push(`What if the ${the} were made in ${target}?`);
  qs.push("Which ruling matters most?");
  return qs;
}

function Q({ children }: { children: ReactNode }) {
  return (
    <p className="duo max-w-[90%] border border-line bg-paper px-3 py-2 text-[13px] font-medium">
      <span className="font-mono text-ok">Q </span>
      {children}
    </p>
  );
}

// The agent's steps in plain words, folded (#119).
function stepWords(s: Step): string {
  switch (s.tool) {
    case "get_case":
      return "Read the case";
    case "search_rulings":
      return `Searched earlier rulings for “${s.arg ?? ""}”`;
    case "read_ruling":
      return `Read ruling ${s.arg ?? ""}`;
    case "what_if":
      return `Re-ran the origin engine: ${s.arg ?? ""}`;
    default:
      return s.tool ?? "";
  }
}

function TurnView({ t, caseId }: { t: Turn; caseId: string }) {
  const cited = t.cited_rulings ?? [];
  return (
    <div>
      <Q>{t.question}</Q>
      {t.whatIfs?.map((w, i) => (
        <div key={i} className="mt-3 grid gap-3.5 border border-edge bg-white px-3.5 py-3 shadow-[3px_3px_0_var(--shadow)] sm:grid-cols-[minmax(0,1fr)_150px]">
          <div>
            <p className="text-[11px] uppercase tracking-[0.06em] text-ok">What if · {w.change}</p>
            <p className="mt-1 flex items-center gap-2 text-2xl tracking-[-0.05em]">
              <Flag country={w.origin} size={20} />
              {w.origin}
            </p>
            {w.decisive_step && <p className="duo clamp-2 mt-1 text-xs text-dim">{w.decisive_step}</p>}
          </div>
          <div className="grid content-between gap-2">
            <Ruler value={w.confidence} label={`${BAR} auto-accept`} width={150} caption={`confidence ${conf(w.confidence)}`} />
            {w.change && (
              <Link
                href={`/cases/${encodeURIComponent(caseId)}/scenarios?add=${encodeURIComponent(w.change)}`}
                className="text-xs text-ok hover:underline"
              >
                Add as a scenario →
              </Link>
            )}
          </div>
        </div>
      ))}
      <Answer text={t.answer ?? ""} cited={cited} />
      {cited.length > 0 && (
        <div className="mt-2.5 text-xs">
          <p className="mb-1 text-[11px] uppercase tracking-[0.06em] text-dim">Cited · rulings the agent read</p>
          {cited.map((rid) => {
            const r = details.get(rid);
            return (
              <div key={rid} className="flex justify-between gap-2.5 border-t border-line py-1.5">
                <span className="min-w-0">
                  {r?.subject ? r.subject.replace(/^the (tariff classification and )?country of origin (determination )?(of|for) (an? |the )?/i, "").replace(/\.$/, "") : <span className="text-dim">earlier CBP ruling</span>}
                  {r?.date ? (
                    <span className="text-dim">
                      {" "}
                      · {formatDate(r.date)}
                      {r.origin ? (
                        <>
                          {" "}
                          · held <Country name={r.origin} />
                        </>
                      ) : null}
                    </span>
                  ) : null}
                </span>
                <RulingLink id={rid} />
              </div>
            );
          })}
        </div>
      )}
      {t.steps?.length ? (
        <details className="mt-2 text-xs text-dim">
          <summary className="inline-flex min-h-6 items-center hover:text-ink">
            How it answered · {t.steps.length} {t.steps.length === 1 ? "step" : "steps"}
          </summary>
          <ol className="duo mt-1 list-decimal pl-5">
            {t.steps.map((s, i) => (
              <li key={i}>{stepWords(s)}</li>
            ))}
          </ol>
        </details>
      ) : null}
    </div>
  );
}

const RULING = /\b([NH]\d{6})\b/;

// The answer as light Markdown: bold, italics and lists render; headings,
// links and raw HTML don't (#114). Cited ruling numbers are blue; a number
// the agent never read is marked as not verified (#113).
function Answer({ text, cited }: { text: string; cited: string[] }) {
  const unverified = [...new Set(text.match(new RegExp(RULING, "g")) ?? [])].filter((id) => !cited.includes(id));
  const mark = (node: ReactNode): ReactNode =>
    typeof node === "string" ? (
      node.split(RULING).map((part, i) =>
        i % 2 === 0 ? (
          part
        ) : cited.includes(part) ? (
          <span key={i} className="font-mono text-xs text-ok">
            {part}
          </span>
        ) : (
          <span key={i} className="font-mono text-xs text-ask underline decoration-dotted underline-offset-2" title="Not verified: the agent didn't read this ruling">
            {part}
            <span className="sr-only"> (not verified)</span>
          </span>
        ),
      )
    ) : Array.isArray(node) ? (
      node.map((n, i) => <Fragment key={i}>{mark(n)}</Fragment>)
    ) : (
      node
    );
  return (
    <div className="duo mt-2.5 flex flex-col gap-2 text-[13px] leading-relaxed text-body">
      <ReactMarkdown
        allowedElements={["p", "ul", "ol", "li", "strong", "em", "code", "br"]}
        unwrapDisallowed
        components={{
          p: ({ children }) => <p>{mark(children)}</p>,
          li: ({ children }) => <li>{mark(children)}</li>,
          strong: ({ children }) => <strong className="font-medium text-ink">{mark(children)}</strong>,
          em: ({ children }) => <em>{mark(children)}</em>,
          ul: ({ children }) => <ul className="flex list-disc flex-col gap-1 pl-5">{children}</ul>,
          ol: ({ children }) => <ol className="flex list-decimal flex-col gap-1 pl-5">{children}</ol>,
          code: ({ children }) => <span className="font-mono text-xs">{children}</span>,
        }}
      >
        {text}
      </ReactMarkdown>
      {unverified.length > 0 && (
        <p role="note" className="border border-ask-line bg-ask-wash px-3 py-2 text-xs text-ask">
          Not verified: {unverified.join(", ")} {unverified.length === 1 ? "isn't a ruling" : "aren't rulings"} the agent read for this answer, so{" "}
          {unverified.length === 1 ? "it" : "they"} may be wrong. Check before relying on {unverified.length === 1 ? "it" : "them"}.
        </p>
      )}
    </div>
  );
}
