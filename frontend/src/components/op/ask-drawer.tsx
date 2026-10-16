"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { api, type CaseResponse, type ChatTurn, type PrecedentRef, type WhatIf } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { BAR, conf, formatDate } from "@/lib/format";
import { Elapsed, ErrorBox } from "./chrome";
import { Button, Country, Drawer, Flag, RulingLink, Ruler, cx } from "./ui";
import { planTarget } from "./plan-target";

type Turn = ChatTurn & { whatIfs?: WhatIf[]; steps?: { tool?: string; arg?: string }[]; calls?: number };

// "Ask about this case" (DESIGN.md, #46): a right-hand drawer on the Result
// screen. Earlier turns collapse to one line; the newest shows in full.
export function AskDrawer({ c, onClose }: { c: CaseResponse; onClose: () => void; onCase?: (c: CaseResponse) => void }) {
  const stored = (c as CaseResponse & { chat?: ChatTurn[] }).chat ?? [];
  const [turns, setTurns] = useState<Turn[]>(stored);
  const [detail, setDetail] = useState<Record<string, PrecedentRef>>(() =>
    Object.fromEntries((c.precedents_detail ?? []).map((r) => [r.id, r])),
  );
  const [question, setQuestion] = useState("");
  const [pending, setPending] = useState<{ q: string; since: number } | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [open, setOpen] = useState<number | null>(null);
  const abort = useRef<AbortController | null>(null);
  const input = useRef<HTMLInputElement>(null);
  const log = useRef<HTMLDivElement>(null);

  useEffect(() => {
    input.current?.focus();

    return () => {
      abort.current?.abort();
    };
  }, []);

  useEffect(() => {
    log.current?.scrollTo({ top: log.current.scrollHeight });
  }, [turns, pending]);

  const ask = async (q: string) => {
    q = q.trim();
    if (!q || pending) return;
    setError(null);
    setPending({ q, since: Date.now() });
    setQuestion("");
    abort.current = new AbortController();
    try {
      const r = await api.ask(c.id!, q, abort.current.signal);
      setDetail((d) => ({ ...d, ...Object.fromEntries((r.cited_detail ?? []).map((x) => [x.id, x])) }));
      const reply = r.reply ?? {};
      setTurns((t) => [
        ...t,
        { question: q, answer: reply.answer ?? "", cited_rulings: reply.cited_rulings ?? [], whatIfs: reply.what_ifs, steps: reply.steps, calls: reply.model_calls },
      ]);
      setOpen(null);
    } catch (e) {
      if (!(e instanceof DOMException && e.name === "AbortError")) {
        setError(e);
        setQuestion(q);
      }
    } finally {
      setPending(null);
    }
  };

  const reset = async () => {
    try {
      await api.resetChat(c.id!);
      setTurns([]);
    } catch (e) {
      setError(e);
    }
  };

  const suggestions = starterQuestions(c);

  return (
    <Drawer
      label={copy.result.ask}
      onClose={onClose}
      header={
        <>
          <b className="font-medium text-ink">◆ {copy.result.ask}</b>
          {turns.length > 0 && (
            <button type="button" className="ml-auto text-xs normal-case tracking-normal text-ok hover:underline" onClick={reset}>
              Clear conversation
            </button>
          )}
        </>
      }
    >
      <div ref={log} className="flex h-full flex-col gap-5 overflow-y-auto px-5 py-4" aria-live="polite">
        {turns.length === 0 && !pending && (
          <p className="duo text-[13px] text-dim">
            Ask why the engine landed where it did, or try a change (&ldquo;what if …&rdquo;). The agent reads this case and earlier CBP rulings dated before{" "}
            {formatDate(c.date)}; a what-if re-runs the origin engine on the changed facts.
          </p>
        )}
        {turns.map((t, i) =>
          i < turns.length - 1 && open !== i ? (
            <button key={i} type="button" onClick={() => setOpen(i)} className="duo border-b border-line pb-3 text-left text-xs text-dim">
              <b className="font-medium text-ink">Earlier:</b> {t.question} · <span className="text-ok">{t.cited_rulings?.length ?? 0} rulings cited · show answer</span>
            </button>
          ) : (
            <TurnView key={i} t={t} detail={detail} caseId={c.id!} />
          ),
        )}
        {pending && (
          <div>
            <Q>{pending.q}</Q>
            <div className="mt-2.5 border border-dashed border-guide px-3.5 py-3 text-[12.5px] text-dim" role="status">
              <span className="float-right">
                <Elapsed since={pending.since} />
              </span>
              <b className="font-medium text-ink">Reading the case and earlier rulings…</b>
              <br />
              usually 15–40 s · a what-if re-runs the origin engine on changed facts
              <div className="relative mt-2.5 h-[3px] overflow-hidden bg-wash">
                <i className="absolute inset-y-0 w-1/3 animate-[slide_1.6s_ease-in-out_infinite] bg-ok" />
              </div>
            </div>
          </div>
        )}
        {error ? <ErrorBox error={error} onRetry={() => ask(question)} /> : null}
      </div>

      {!pending && (
        <div className="flex flex-wrap gap-2 px-5 pb-3">
          {suggestions.map((s) => (
            <button key={s} type="button" onClick={() => ask(s)} className="border border-line bg-paper px-2 py-1 text-[11.5px] text-dim hover:text-ink">
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
    </Drawer>
  );
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

function Q({ children }: { children: React.ReactNode }) {
  return (
    <p className="duo max-w-[90%] border border-line bg-paper px-3 py-2 text-[13px] font-medium">
      <span className="font-mono text-ok">Q </span>
      {children}
    </p>
  );
}

function TurnView({ t, detail, caseId }: { t: Turn; detail: Record<string, PrecedentRef>; caseId: string }) {
  const cited = t.cited_rulings ?? [];
  return (
    <div>
      <Q>{t.question}</Q>
      {t.whatIfs?.map((w, i) => (
        <div key={i} className="mt-3 grid gap-3.5 border border-edge bg-white px-3.5 py-3 shadow-[3px_3px_0_var(--shadow)] sm:grid-cols-[minmax(0,1fr)_150px]">
          <div>
            <p className="text-[10.5px] uppercase tracking-[0.06em] text-ok">What if · {w.change}</p>
            <p className="mt-1 flex items-center gap-2 text-2xl tracking-[-0.05em]">
              <Flag country={w.origin} size={20} />
              {w.origin}
            </p>
            {w.decisive_step && <p className="duo clamp-2 mt-1 text-xs text-dim">{w.decisive_step}</p>}
          </div>
          <div className="grid content-between gap-2">
            <Ruler value={w.confidence} label={String(BAR)} width={150} caption={`confidence ${conf(w.confidence)}`} />
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
      <Answer text={t.answer ?? ""} ids={cited} />
      {cited.length > 0 && (
        <div className="mt-2.5 text-xs">
          <p className="mb-1 text-[10.5px] uppercase tracking-[0.06em] text-dim">Cited · rulings the agent read</p>
          {cited.map((id) => {
            const r = detail[id];
            return (
              <div key={id} className="flex justify-between gap-2.5 border-t border-line py-1.5">
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
                <RulingLink id={id} />
              </div>
            );
          })}
        </div>
      )}
      {t.steps?.length ? (
        <p className="mt-2 font-mono text-[11px] text-dim">
          {t.steps.map((s) => s.tool + (s.arg ? `("${s.arg}")` : "")).join(" → ")}
          {t.calls ? ` · ${t.calls} model calls` : ""}
        </p>
      ) : null}
    </div>
  );
}

// Answer text as returned, paragraphs kept, cited ruling numbers in blue.
function Answer({ text, ids }: { text: string; ids: string[] }) {
  const re = ids.length ? new RegExp(`(${ids.map((i) => i.replace(/[^A-Z0-9]/g, "")).join("|")})`) : null;
  return (
    <div className="duo mt-2.5 flex flex-col gap-2 text-[13px] leading-relaxed text-body">
      {text
        .split(/\n\s*\n/)
        .filter((p) => p.trim())
        .map((para, i) => (
          <p key={i}>{re ? para.split(re).map((part, j) => (ids.includes(part) ? <span key={j} className={cx("font-mono text-xs text-ok")}>{part}</span> : part.replace(/\*([^*]+)\*/g, "$1"))) : para}</p>
        ))}
    </div>
  );
}
