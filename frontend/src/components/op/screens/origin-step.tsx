"use client";

import { useState } from "react";
import { api, type CaseResponse } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { BAR, conf, formatDate, formatMonth, isProxy, shortSubject } from "@/lib/format";
import { HistoryDrawer } from "../history-drawer";
import { CaseShell, currentOrigin, type CaseCtx } from "../case-shell";
import { ErrorBox, Footer, Main, Next, NextSteps, StepEyebrow } from "../chrome";
import { Audit, RulingsList } from "../evidence";
import { Button, ButtonLink, Country, Dimension, Flag, Fold, Panel, RulingLink, Ruler, StatusChip, cx } from "../ui";
import { planTarget } from "../plan-target";

export function OriginStep() {
  return <CaseShell tab="origin">{(ctx) => <Result {...ctx} />}</CaseShell>;
}

function Result({ id, c, setCase }: CaseCtx) {
  const [history, setHistory] = useState(false);
  const p = c.prediction ?? {};
  const target = planTarget(c);
  const route = c.triage?.route;
  const qs = c.clarify?.questions?.length ?? 0;
  const cited = p.cited_rulings?.length ?? 0;
  const enc = encodeURIComponent(id);

  return (
    <>
      <VerdictBand c={c} />
      {/* Top to bottom: the verdict, then why (left) beside what to do next
          (right); details people may want fold into accordions (17 Oct). */}
      <Main className="grid gap-10 lg:grid-cols-[minmax(0,1fr)_320px]">
        <div className="flex min-w-0 flex-col gap-6">
          {qs ? <Questions c={c} setCase={setCase} /> : null}
          {p.reasoning ? (
            <section aria-label="Why">
              <h2 className="eyebrow mb-1.5">{route === "auto" ? `Why · auto-accepted, ${conf(p.confidence)} is at or above the ${BAR} bar` : "Why"}</h2>
              <p className="duo max-w-[75ch] text-sm leading-relaxed">{p.reasoning}</p>
            </section>
          ) : null}
          {p.key_factors?.length ? (
            <section aria-label={copy.result.keyFactors}>
              <h2 className="eyebrow mb-1.5">{copy.result.keyFactors}</h2>
              <p className="duo max-w-[75ch] text-sm">{p.key_factors.join("; ")}</p>
            </section>
          ) : null}
          {c.watch?.status === "changed" ? <WatchLine c={c} setCase={setCase} /> : null}
          <div>
            {c.precedents_detail?.length ? (
              <Fold title={copy.result.rulings} meta={`${c.precedents_detail.length} read${cited ? `, ${cited} cited` : ""}`}>
                <RulingsSummary c={c} />
              </Fold>
            ) : null}
            {c.clarify?.conflict ? <Conflict c={c} /> : null}
            {c.answers?.length ? (
              <Fold title="Your answers" meta={c.answers.length}>
                <ul className="duo flex flex-col gap-2 text-[13px]">
                  {c.answers.map((a, i) => (
                    <li key={i}>
                      <span className="text-dim">{a.question}</span>
                      <br />
                      {a.answer}
                    </li>
                  ))}
                </ul>
              </Fold>
            ) : null}
            {c.watch?.status !== "changed" ? (
              <Fold title="Ruling watch" meta={c.watch ? `unchanged, ${formatDate(c.watch.checked_at)}` : "not re-checked"}>
                <WatchLine c={c} setCase={setCase} />
              </Fold>
            ) : null}
            {/* Plain words first; model and prompt ids sit here (#93). */}
            <Fold title="Run details" meta={c.history?.length ? `history ${c.history.length}` : undefined}>
              <Audit>
                Read {p.precedents?.length ?? 0} CBP rulings dated before {formatDate(c.date)}
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
              <p className="duo mt-1 text-xs text-dim">
                Model {c.model_ids?.engine} · prompt {c.prompt_versions?.engine}
                {c.prompt_versions?.clarify && c.clarify ? ` · questions ${c.prompt_versions.clarify}` : ""}
                {c.triage?.route ? ` · route ${c.triage.route}` : ""}
              </p>
            </Fold>
          </div>
        </div>
        <NextSteps
          steps={[{ label: route === "auto" ? "Draft a ruling request" : "Have a broker review: draft a ruling request", href: `/cases/${enc}/draft` }]}
          suggestions={[
            ...(qs ? [{ label: `Answer ${qs === 1 ? "the question" : `the ${qs} questions`} to sharpen this`, onClick: () => document.getElementById("q0")?.focus() }] : []),
            { label: "Try a what-if scenario", href: `/cases/${enc}/scenarios` },
          ]}
        />
      </Main>
      <Footer>
        <Next>
          {!c.product?.plants?.length
            ? "To plan a move, turn the description into parts and plants."
            : c.clarify?.questions?.length
              ? `Answer a question below to sharpen this, or plan a move to ${target ?? "another country"}.`
              : route === "auto"
                ? `Plan a move to ${target ?? "another country"}, or try a what-if.`
                : `Plan a move to ${target ?? "another country"}, or have a broker review this origin.`}
        </Next>
        <div className="ml-auto flex flex-wrap items-center gap-2.5">
          {c.product?.plants?.length ? (
            <ButtonLink href={`/cases/${encodeURIComponent(id)}/plan${target ? `?target=${encodeURIComponent(target)}` : ""}`}>
              Plan a move{target ? <> to <Flag country={target} size={12} /> {target}</> : null} →
            </ButtonLink>
          ) : (
            <ButtonLink href={`/cases/new?from=${encodeURIComponent(id)}`}>Add parts and plants →</ButtonLink>
          )}
        </div>
      </Footer>
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
        <div>
          <StepEyebrow tab="origin">{copy.result.eyebrow}</StepEyebrow>
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

// Ruling watch as one line in the evidence column (was a full-width band).
// A changed result is the exception: it gets the violet "look again" box.
function WatchLine({ c, setCase }: { c: CaseResponse; setCase: (c: CaseResponse) => void }) {
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
    <section aria-label="Ruling watch" className={cx("duo text-[13px]", changed && "border border-ask-line bg-ask-wash p-3")}>
      {changed && <h3 className="eyebrow mb-1">Ruling watch</h3>}
      <p>
        {!w && <>Not re-checked against rulings after {formatDate(c.date)}.</>}
        {w && !changed && <>Unchanged with today&apos;s rulings ({formatDate(w.checked_at)}).</>}
        {w && changed && (
          <>
            <b className="font-medium text-ask">Changed:</b> with today&apos;s rulings it&apos;s <Country name={w.prediction?.origin_country} /> ({conf(w.prediction?.confidence)}).{" "}
            {w.newer_rulings?.map((r) => (
              <span key={r.id}>
                <RulingLink id={r.id} /> ({formatMonth(r.date)}){" "}
              </span>
            ))}
          </>
        )}{" "}
        <button type="button" className="text-ok underline underline-offset-2 disabled:text-dim" onClick={run} disabled={busy}>
          {busy ? "Re-checking…" : w ? "Re-check again" : "Re-check now"}
        </button>
      </p>
      {error ? <p className="mt-1 text-fail">Re-check failed: {(error as Error).message}</p> : null}
    </section>
  );
}

// Earlier rulings, summarised: the cited ones show, the rest fold behind
// "Show all N" (cited first, as before).
function RulingsSummary({ c }: { c: CaseResponse }) {
  const all = c.precedents_detail ?? [];
  const cited = c.prediction?.cited_rulings ?? [];
  const shown = all.filter((r) => cited.includes(r.id));
  const [open, setOpen] = useState(shown.length === 0);
  return (
    <section aria-label={copy.result.rulings}>
      <RulingsList rulings={open ? all : shown} cited={cited} />
      {all.length > shown.length && (
        <button type="button" className="mt-1.5 text-[13px] text-ok hover:underline" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
          {open ? "Show cited only" : `Show all ${all.length}`}
        </button>
      )}
    </section>
  );
}

// Earlier rulings disagree (locked 16 Oct, p-res-conflict.html): the two
// rulings as halves; the one named by conflict.leans is tinted blue with
// "this case follows". Folded by default under the questions: it explains
// the low confidence, it isn't the next step.
function Conflict({ c }: { c: CaseResponse }) {
  const k = c.clarify!.conflict!;
  const detail = new Map((c.precedents_detail ?? []).map((r) => [r.id, r]));
  const halves = [
    { side: "a", id: k.ruling_a, holding: k.holding_a },
    { side: "b", id: k.ruling_b, holding: k.holding_b },
  ];
  return (
    <Fold title={<span className="text-ask">{copy.result.conflict}</span>} meta="why the confidence is lower">
      <div className="grid gap-4 sm:grid-cols-2">
        {halves.map(({ side, id, holding }) => {
          const r = id ? detail.get(id) : undefined;
          const follows = k.leans === side;
          return (
            <div key={side} className={cx("border p-3 text-[13px]", follows ? "border-ok bg-ok-wash" : "border-line bg-white")}>
              {follows && <p className="mb-1.5 text-[11px] font-medium uppercase tracking-[0.06em] text-ok">This case follows</p>}
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
    </Fold>
  );
}

// The one panel with focus on this screen: the first question shows with its
// answer field; the rest fold behind "N more questions".
function Questions({ c, setCase }: { c: CaseResponse; setCase: (c: CaseResponse) => void }) {
  const qs = c.clarify!.questions!;
  const [answers, setAnswers] = useState<string[]>(() => qs.map(() => ""));
  const [unknown, setUnknown] = useState<boolean[]>(() => qs.map(() => false));
  const [more, setMore] = useState(false);
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
  const visible = more ? qs : qs.slice(0, 1);
  return (
    <Panel tone="ask">
      <h2 className="text-base font-medium tracking-[-0.02em]">
        {copy.result.questions} <span className="text-[13px] font-normal text-dim">· optional, sharpens the answer</span>
      </h2>
      <p className="duo mt-1 text-[13px] text-dim">
        {c.clarify?.closest_ruling ? (
          <>
            Closest earlier ruling: <RulingLink id={c.clarify.closest_ruling} />.{" "}
          </>
        ) : null}
        {copy.result.questionsWhy}
      </p>
      {visible.map((q, i) => (
        <div key={i} className="mt-4">
          <label htmlFor={`q${i}`} className="duo block text-sm font-medium">
            {i + 1}. {q}
          </label>
          <div className="mt-1.5 flex gap-2">
            <input
              id={`q${i}`}
              className="min-w-0 flex-1 rounded-[2px] border border-line bg-white px-3 py-2 text-sm placeholder:text-faint disabled:text-dim"
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
      {qs.length > 1 && (
        <button type="button" className="mt-3 text-[13px] text-ok hover:underline" aria-expanded={more} onClick={() => setMore((m) => !m)}>
          {more ? "Show one question" : `${qs.length - 1} more ${qs.length - 1 === 1 ? "question" : "questions"}`}
        </button>
      )}
      <div className="mt-4 flex flex-wrap items-center gap-3.5">
        <Button variant="ghost" onClick={submit} disabled={!addsFact || busy}>
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
