"use client";

import { useState } from "react";
import { api, type CaseResponse } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { BAR, conf, formatDate, formatMonth, isProxy, shortSubject } from "@/lib/format";
import { AskDrawer } from "../ask-drawer";
import { HistoryDrawer } from "../history-drawer";
import { CaseShell, currentOrigin, type CaseCtx } from "../case-shell";
import { ErrorBox, Footer, Main } from "../chrome";
import { Audit, RulingsList } from "../evidence";
import { Button, ButtonLink, Country, Dimension, Flag, Panel, RulingLink, Ruler, StatusChip, cx } from "../ui";
import { planTarget } from "../plan-target";

export function OriginStep() {
  return <CaseShell step={2}>{(ctx) => <Result {...ctx} />}</CaseShell>;
}

function Result({ id, c, setCase }: CaseCtx) {
  const [asking, setAsking] = useState(false);
  const [history, setHistory] = useState(false);
  const p = c.prediction ?? {};
  const target = planTarget(c);
  const route = c.triage?.route;

  return (
    <>
      <VerdictBand c={c} />
      <WatchStrip c={c} setCase={setCase} />
      <Main className={cx("grid gap-10 lg:grid-cols-[minmax(0,1fr)_400px]", asking && "lg:opacity-60")}>
        <div className="flex min-w-0 flex-col gap-8">
          {c.clarify?.conflict ? <Conflict c={c} /> : null}
          {c.clarify?.questions?.length ? <Questions c={c} setCase={setCase} /> : null}
          {!c.clarify?.questions?.length && p.reasoning ? (
            <Panel>
              <h2 className="eyebrow mb-2">Why</h2>
              <p className="duo text-sm leading-relaxed">{p.reasoning}</p>
            </Panel>
          ) : null}
          {route === "auto" && <p className="duo text-sm text-dim">Auto-accepted: confidence {conf(p.confidence)} is at or above the {BAR} bar.</p>}
          {c.answers?.length ? (
            <Panel>
              <h2 className="eyebrow mb-2">Your answers</h2>
              <ul className="duo flex flex-col gap-2 text-[13px]">
                {c.answers.map((a, i) => (
                  <li key={i}>
                    <span className="text-dim">{a.question}</span>
                    <br />
                    {a.answer}
                  </li>
                ))}
              </ul>
            </Panel>
          ) : null}
        </div>
        <aside className="flex min-w-0 flex-col gap-6">
          {p.key_factors?.length ? (
            <div>
              <h2 className="eyebrow mb-2">{copy.result.keyFactors}</h2>
              <ul className="text-sm">
                {p.key_factors.map((k) => (
                  <li key={k} className="duo border-y border-line py-2">
                    {k}
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
          {c.precedents_detail?.length ? (
            <div>
              <h2 className="eyebrow mb-2">{copy.result.rulings}</h2>
              <RulingsList rulings={c.precedents_detail} cited={p.cited_rulings} />
            </div>
          ) : null}
          <Audit>
            {c.model_ids?.engine} · {c.prompt_versions?.engine}
            {c.prompt_versions?.clarify && c.clarify ? ` · ${c.prompt_versions.clarify}` : ""} · read {p.precedents?.length ?? 0} CBP rulings dated before {formatDate(c.date)}
            {isProxy(c.llm_provider) ? ` · ${copy.proxy}` : ""}
            {c.history?.length ? (
              <>
                {" · "}
                <button type="button" className="text-ok underline underline-offset-2" onClick={() => setHistory(true)}>
                  History ({c.history.length})
                </button>
              </>
            ) : null}
          </Audit>
        </aside>
      </Main>
      <Footer>
        <ButtonLink variant="ghost" href={`/cases/${encodeURIComponent(id)}/product`}>
          ← Product
        </ButtonLink>
        <ButtonLink variant="ghost" href={`/cases/${encodeURIComponent(id)}/scenarios`}>
          {copy.result.scenarios}
        </ButtonLink>
        <div className="ml-auto flex flex-wrap items-center gap-2.5">
          <Button variant="ghost" onClick={() => setAsking(true)} aria-expanded={asking}>
            {copy.result.ask}
          </Button>
          {c.clarify?.questions?.length ? <span className="hidden text-xs text-dim sm:inline">{copy.result.optional}</span> : null}
          {c.product?.plants?.length ? (
            <ButtonLink href={`/cases/${encodeURIComponent(id)}/plan${target ? `?target=${encodeURIComponent(target)}` : ""}`}>
              Plan a move{target ? <> to <Flag country={target} size={12} /> {target}</> : null} →
            </ButtonLink>
          ) : (
            <span className="text-xs text-dim">Plans need parts and plants (step 1)</span>
          )}
        </div>
      </Footer>
      {asking && <AskDrawer c={c} onClose={() => setAsking(false)} onCase={setCase} />}
      {history && <HistoryDrawer c={c} onClose={() => setHistory(false)} />}
    </>
  );
}

export function VerdictBand({ c }: { c: CaseResponse }) {
  const p = c.prediction ?? {};
  const cur = currentOrigin(c);
  const route = c.triage?.route;
  return (
    <section aria-label="Likely origin" className="relative border-b border-dashed border-guide">
      <div className="column relative mx-auto grid max-w-[1240px] items-end gap-8 px-4 pb-8 pt-10 sm:px-10 lg:grid-cols-[auto_minmax(0,1fr)_260px] lg:gap-11">
        <Dimension className="absolute left-4 top-3 sm:left-10">fig.1 — likely origin</Dimension>
        {route && <Dimension className="absolute right-4 top-3 sm:right-10">route: {route}</Dimension>}
        <div>
          <p className="eyebrow">{copy.result.eyebrow}</p>
          <h1 className="mt-1 flex items-center gap-4 text-[56px] font-normal leading-none tracking-[-0.06em] sm:text-[84px]">
            <Flag country={p.origin_country} size={58} />
            {p.origin_country || "—"}
          </h1>
        </div>
        <div className="min-w-0 pb-3.5">
          {p.decisive_step && (
            <p className="duo clamp-2 text-[17px] leading-snug" title={p.decisive_step}>
              Decisive step: {p.decisive_step}
            </p>
          )}
          {cur?.country && (
            <p className={cx("duo mt-1.5 text-[13px]", cur.country === p.origin_country ? "text-ok" : "text-ask")}>
              {cur.country === p.origin_country ? "✓ Same as" : "△ Differs from"} CBP&apos;s ruling on this setup: <Country name={cur.country} />
              {cur.ruling && (
                <>
                  , <RulingLink id={cur.ruling} />
                </>
              )}
            </p>
          )}
        </div>
        <div className="flex flex-col items-start gap-1 pb-3.5">
          <StatusChip route={route} />
          <Ruler value={p.confidence} label={`${BAR} auto-accept`} caption={`confidence ${conf(p.confidence)}`} />
        </div>
      </div>
    </section>
  );
}

function WatchStrip({ c, setCase }: { c: CaseResponse; setCase: (c: CaseResponse) => void }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const w = c.watch;
  const run = async () => {
    setBusy(true);
    setError(null);
    try {
      setCase(await api.watch(c.id!));
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  const changed = w?.status === "changed";
  return (
    <section aria-label="Ruling watch" className="column mx-auto w-full max-w-[1240px] px-4 pt-6 sm:px-10">
      <div
        className={cx(
          "duo grid items-center gap-3 px-4 py-3 text-[13px] sm:grid-cols-[auto_minmax(0,1fr)_auto] sm:gap-5",
          changed ? "border border-ask-line bg-white shadow-[4px_4px_0_var(--ask-shadow)]" : "border border-dashed border-guide bg-sheet",
        )}
      >
        <span className="whitespace-nowrap text-[11px] uppercase tracking-[0.08em] text-dim">
          <span className={changed ? "text-ask" : "text-ok"}>◆</span> Ruling watch
        </span>
        <span>
          {!w && <>Not re-checked yet. CBP may have published rulings since this case&apos;s date ({formatDate(c.date)}).</>}
          {w && !changed && (
            <>
              Unchanged with every ruling available today (checked {formatDate(w.checked_at)}).{" "}
              {w.newer_rulings?.length ? <>Newer rulings read: {w.newer_rulings.map((r) => r.id).join(", ")}.</> : "No newer ruling was on point."}
            </>
          )}
          {w && changed && (
            <>
              <b className="font-medium text-ask">Changed:</b> with today&apos;s rulings the likely origin is <Country name={w.prediction?.origin_country} /> (
              {conf(w.prediction?.confidence)}).{" "}
              {w.newer_rulings?.length ? (
                <>
                  Newer:{" "}
                  {w.newer_rulings.map((r) => (
                    <span key={r.id}>
                      <RulingLink id={r.id} /> ({formatMonth(r.date)}){" "}
                    </span>
                  ))}
                </>
              ) : null}
            </>
          )}
          {error ? <span className="text-fail"> Re-check failed: {(error as Error).message}</span> : null}
        </span>
        <Button variant="ghost" className="min-h-8 px-3 py-1 text-[12.5px]" onClick={run} disabled={busy}>
          {busy ? "Re-checking…" : w ? "Re-check again" : "Re-check now"}
        </Button>
      </div>
    </section>
  );
}

// Earlier rulings disagree (locked 16 Oct, p-res-conflict.html): the two
// rulings as halves; the one named by conflict.leans is tinted blue with
// "this case follows". Only endpoint text.
function Conflict({ c }: { c: CaseResponse }) {
  const k = c.clarify!.conflict!;
  const detail = new Map((c.precedents_detail ?? []).map((r) => [r.id, r]));
  const halves = [
    { side: "a", id: k.ruling_a, holding: k.holding_a },
    { side: "b", id: k.ruling_b, holding: k.holding_b },
  ];
  return (
    <Panel tone="ask">
      <h2 className="eyebrow text-ask">{copy.result.conflict}</h2>
      <div className="mt-3 grid gap-4 sm:grid-cols-2">
        {halves.map(({ side, id, holding }) => {
          const r = id ? detail.get(id) : undefined;
          const follows = k.leans === side;
          return (
            <div key={side} className={cx("border p-3 text-[13px]", follows ? "border-ok bg-ok-wash" : "border-line bg-white")}>
              {follows && <p className="mb-1.5 text-[10.5px] font-medium uppercase tracking-[0.06em] text-ok">This case follows</p>}
              {r?.subject && <p className="font-medium">{shortSubject(r.subject)}</p>}
              <p className="mt-0.5 text-xs text-dim">
                {id && <RulingLink id={id} />}
                {r?.origin && (
                  <>
                    {" "}
                    · held <Country name={r.origin} />
                  </>
                )}
              </p>
              <p className="duo mt-2">{holding}</p>
            </div>
          );
        })}
      </div>
      {k.deciding_difference && (
        <p className="duo mt-3 text-[13px]">
          <span className="text-dim">What CBP relied on: </span>
          {k.deciding_difference}
        </p>
      )}
    </Panel>
  );
}

function Questions({ c, setCase }: { c: CaseResponse; setCase: (c: CaseResponse) => void }) {
  const qs = c.clarify!.questions!;
  const [answers, setAnswers] = useState<string[]>(() => qs.map(() => ""));
  const [unknown, setUnknown] = useState<boolean[]>(() => qs.map(() => false));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  // "Unknown" answers are kept but add no fact; the button stays disabled
  // until one real answer is typed (the API returns 400 otherwise).
  const addsFact = answers.some((a, i) => !unknown[i] && a.trim() !== "");
  const submit = async () => {
    setBusy(true);
    setError(null);
    try {
      setCase(
        await api.answers(
          c.id!,
          qs.map((q, i) => ({ question: q, answer: unknown[i] ? "I don't know." : answers[i].trim() })).filter((a) => a.answer),
        ),
      );
      setAnswers(qs.map(() => ""));
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Panel tone="ask">
      <h2 className="text-lg tracking-[-0.02em]">{copy.result.questions}</h2>
      <p className="duo mt-1 text-sm text-dim">
        {c.clarify?.closest_ruling ? (
          <>
            Closest earlier ruling: <RulingLink id={c.clarify.closest_ruling} />.{" "}
          </>
        ) : null}
        {copy.result.questionsWhy}
      </p>
      {qs.map((q, i) => (
        <div key={i} className="mt-4">
          <label htmlFor={`q${i}`} className="duo block text-sm font-medium">
            {i + 1}. {q}
          </label>
          <div className="mt-1.5 flex gap-2">
            <input
              id={`q${i}`}
              className="min-w-0 flex-1 rounded-[2px] border border-line bg-paper px-3 py-2 text-sm placeholder:text-faint disabled:text-dim"
              placeholder="Type your answer"
              value={unknown[i] ? "" : answers[i]}
              disabled={unknown[i]}
              onChange={(e) => setAnswers(answers.map((a, j) => (j === i ? e.target.value : a)))}
            />
            <button
              type="button"
              aria-pressed={unknown[i]}
              onClick={() => setUnknown(unknown.map((u, j) => (j === i ? !u : u)))}
              className={cx("whitespace-nowrap rounded-[2px] border px-3 text-[13px]", unknown[i] ? "border-ask-line bg-ask-wash text-ask" : "border-line text-dim hover:bg-wash")}
            >
              {copy.result.idk}
            </button>
          </div>
        </div>
      ))}
      <div className="mt-5 flex flex-wrap items-center gap-3.5">
        <Button onClick={submit} disabled={!addsFact || busy}>
          {busy ? "Re-running…" : copy.result.rerun}
        </Button>
        {!addsFact && <span className="text-xs text-dim">{copy.result.rerunHint}</span>}
      </div>
      {error ? (
        <div className="mt-3">
          <ErrorBox error={error} onRetry={submit} />
        </div>
      ) : null}
    </Panel>
  );
}
