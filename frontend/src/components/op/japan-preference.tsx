"use client";

import Link from "next/link";
import { useId, useState } from "react";
import { api, type CaseResponse } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { japanAgreements } from "@/lib/japan-preference";
import { ErrorBox, Main } from "./chrome";
import { Button, Country, Flag, Fold, Panel } from "./ui";

const criteria: Record<string, string> = {
  WO: "Wholly obtained",
  PE: "Made entirely from originating materials",
  CC: "Change of tariff chapter",
  CTH: "Change of tariff heading",
  CTSH: "Change of tariff subheading",
  RVC: "Regional value content",
  SP: "Product-specific requirement",
};

function SourceList({ title, sources }: { title: string; sources: { citation: string; url: string }[] }) {
  const distinct = [...new Map(sources.map((source) => [source.url, source])).values()];
  return (
    <div className="mt-3">
      <h2 className="text-[13px] font-medium">{title}</h2>
      <ul className="mt-1 flex flex-col gap-1 text-[13px]">
        {distinct.map(({ citation, url }) => (
          <li key={url}>
            {/^https:\/\//i.test(url) ? (
              <a href={url} target="_blank" rel="noreferrer" className="text-ok underline underline-offset-2">{citation}</a>
            ) : citation}
          </li>
        ))}
      </ul>
    </div>
  );
}

type Question = { id: string; text: string; options?: string[] };

// headingFor names an outcome the way the result heading does.
export function headingFor(outcome: string | undefined): string {
  const C = copy.japan;
  return outcome === "qualifies" ? C.qualifies : outcome === "does_not_qualify" ? C.doesNotQualify : C.undetermined;
}

// answerPayload keeps the answers that add a fact: a chosen option other than
// "Not sure", or typed text. The API drops "Not sure" anyway and refuses a
// request with no fact.
export function answerPayload(qs: Question[], answers: Record<string, string>): { question: string; answer: string }[] {
  return qs
    .map((q) => ({ question: q.text, answer: (answers[q.id] ?? "").trim() }))
    .filter((a) => a.answer !== "" && a.answer !== copy.japan.notSure);
}

function Choice({ q, value, onChange }: { q: Question; value: string; onChange: (v: string) => void }) {
  const name = useId();
  return (
    <fieldset data-japan-question={q.id} className="min-w-0 border-0 p-0">
      <legend className="duo mb-1 text-[13px] text-ink">{q.text}</legend>
      <div className="flex flex-wrap gap-1.5">
        {q.options!.map((o) => (
          <label
            key={o}
            className="duo inline-flex min-h-11 min-w-16 cursor-pointer items-center justify-center rounded-[2px] border border-line bg-sheet px-3 text-[13px] text-ink has-checked:border-ink has-checked:bg-ink has-checked:text-paper has-focus-visible:outline-2 has-focus-visible:outline-offset-2"
          >
            <input type="radio" name={name} className="sr-only" checked={value === o} onChange={() => onChange(o)} />
            {o}
          </label>
        ))}
      </div>
    </fieldset>
  );
}

function Typed({ q, value, onChange }: { q: Question; value: string; onChange: (v: string) => void }) {
  const id = useId();
  return (
    <div data-japan-question={q.id} className="min-w-0">
      <label htmlFor={id} className="duo mb-1 block text-[13px] text-ink">{q.text}</label>
      <input
        id={id}
        value={value}
        maxLength={300}
        onChange={(e) => onChange(e.target.value)}
        className="h-11 w-full max-w-[52ch] rounded-[2px] border border-edge bg-white px-3 text-[13.5px] text-ink focus:border-ok focus:outline-2 focus:outline-offset-2 focus:outline-accent"
      />
    </div>
  );
}

// JapanQuestions (D917): the facts that settle an undetermined result, asked
// in place. "Check again" sends them and shows the new result in the page.
function JapanQuestions({ c, questions, onCase }: { c: CaseResponse; questions: Question[]; onCase: (c: CaseResponse) => void }) {
  const C = copy.japan;
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const payload = answerPayload(questions, answers);
  const set = (id: string, v: string) => setAnswers((a) => ({ ...a, [id]: v }));
  const submit = async () => {
    if (!payload.length || busy) return;
    setBusy(true);
    setError(null);
    try {
      onCase(await api.answers(c.id!, payload));
      setAnswers({});
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Panel tone="ask" data-japan-questions aria-busy={busy} className="w-full max-w-[75ch]">
      <h2 className="text-base font-medium tracking-[-0.02em]">{C.ask}</h2>
      <div className="mt-3 flex flex-col gap-4">
        {questions.map((q) =>
          q.options?.length ? (
            <Choice key={q.id} q={q} value={answers[q.id] ?? ""} onChange={(v) => set(q.id, v)} />
          ) : (
            <Typed key={q.id} q={q} value={answers[q.id] ?? ""} onChange={(v) => set(q.id, v)} />
          ),
        )}
      </div>
      <div className="mt-4 flex flex-wrap items-center gap-3">
        <Button onClick={submit} aria-disabled={!payload.length || busy}>
          {C.checkAgain}
        </Button>
        <p role="status" className="duo text-[12.5px] text-dim">
          {busy ? C.checking : !payload.length ? C.pickOne : ""}
        </p>
      </div>
      {error ? (
        <div className="mt-3">
          <ErrorBox error={error} onRetry={submit} />
        </div>
      ) : null}
    </Panel>
  );
}

export function JapanPreference({ c, setCase }: { c: CaseResponse; setCase?: (c: CaseResponse) => void }) {
  // The outcome before the last answers, for the "Was" line after a re-check.
  const [was, setWas] = useState<string | null>(null);
  const C = copy.japan;
  const prediction = c.prediction;
  const result = prediction?.japan_preference;
  const origin = result?.outcome === "qualifies" ? prediction?.origin_country : undefined;
  const heading = headingFor(result?.outcome);
  // The questions replace the review reason when the page can ask them.
  const asking = !!(result?.outcome === "undetermined" && result.questions?.length && setCase && c.id && !c.pinned);
  const agreement = result?.agreement ? japanAgreements[result.agreement] ?? result.agreement : undefined;
  return (
    <>
      <section aria-label={C.scope} className="border-b border-dashed border-guide">
        <div className="column mx-auto max-w-[1240px] px-4 pb-8 pt-10 sm:px-10">
          <p className="eyebrow">{C.scope}</p>
          <h1 className="mt-2 flex items-center gap-3 text-[32px] font-normal leading-none tracking-[-0.05em] sm:text-[52px]">
            <Flag country="Japan" size={36} />
            {heading}
          </h1>
          {was && was !== heading && (
            <p data-japan-change className="duo mt-3 text-sm text-dim">
              {C.was} <span className="line-through">{was}</span>
            </p>
          )}
          {agreement && <p className="mt-3 text-sm text-dim">{agreement}</p>}
          {origin && <p className="mt-2 text-sm">{C.origin} <Country name={origin} /></p>}
          {c.llm_provider === "fake" && <p className="mt-3 text-sm text-dim">{C.simulated}</p>}
          <p className="duo mt-4 max-w-[70ch] text-sm text-body">{C.review}</p>
          {result?.review_reason && !asking && <p className="duo mt-1 max-w-[70ch] text-sm text-dim [overflow-wrap:anywhere]">{result.review_reason}</p>}
          {!result && <p className="duo mt-1 max-w-[70ch] text-sm text-dim">{C.noResult}</p>}
        </div>
      </section>
      <Main className="flex flex-col items-start gap-4">
        {asking ? (
          <JapanQuestions
            c={c}
            questions={result!.questions!}
            onCase={(next) => {
              setWas(heading);
              setCase(next);
            }}
          />
        ) : null}
        <p className="duo text-[13px] text-dim">{C.approvalsOnly}</p>
        {c.id && (
          <Link href={`/cases/new?from=${encodeURIComponent(c.id)}`} className="inline-flex min-h-10 items-center text-sm text-ok underline underline-offset-2">
            {C.edit}
          </Link>
        )}
        {result && (
          <div className="w-full max-w-[75ch] [overflow-wrap:anywhere]">
            <Fold title={C.details}>
              {result.hs && <p className="text-[13px]">{C.tariffCode} {result.hs}</p>}
              {result.criterion && <p className="mt-1 text-[13px]">{criteria[result.criterion] ?? result.criterion}</p>}
              {result.deciding_rule && <p className="duo mt-2 whitespace-pre-line text-[13px] text-body">{result.deciding_rule}</p>}
              {prediction.reasoning && <p className="duo mt-2 whitespace-pre-line text-[13px] text-body">{prediction.reasoning}</p>}
              {result.sources?.length ? <SourceList title={C.rule} sources={result.sources} /> : null}
              {result.precedent_sources?.length ? <SourceList title={C.precedents} sources={result.precedent_sources} /> : null}
            </Fold>
          </div>
        )}
      </Main>
    </>
  );
}
