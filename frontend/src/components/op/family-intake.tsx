"use client";

import { useId, useRef, useState } from "react";
import { api, type CaseResponse, type FamilyIntake as Intake, type SuppliedFamilyFacts } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { countryCodes } from "@/lib/countries";
import { asked, answerText, body, changed, formErrors, initialForm, type FamilyForm } from "@/lib/family-intake";
import { formatDate } from "@/lib/format";
import { caseReadOnly, isStatus, writeGate } from "@/lib/roles";
import { ErrorBox } from "./chrome";
import { Button, Chip, Fold, RulingLink, cx } from "./ui";

// The family questions (#770, D810-D813): for the deepened families, where
// the parts or steps the template's cited rulings turn on were made. Each
// question names the document that settles it. Missing information gets a
// control; "not modelled" (no template, a family the guard refuses, nothing
// fits) gets a reason and no control. Answers sit beside the case; the
// product, facts and stored origin are unchanged.

const COUNTRIES = Object.keys(countryCodes).sort();

export function FamilyIntakePanel({ c, setCase }: { c: CaseResponse; setCase: (c: CaseResponse) => void }) {
  const fi = c.family_intake;
  if (!fi) return null;
  if (fi.status === "not_modelled") return <NotModelled fi={fi} />;
  // A new form when the questions change (another family, or stale answers);
  // not after a save, so the saved note stays.
  return <Questions key={`${fi.family}:${fi.template_version}:${fi.stale ? 1 : 0}:${fi.questions.map((q) => q.id).join(",")}`} c={c} fi={fi} setCase={setCase} />;
}

function NotModelled({ fi }: { fi: Intake }) {
  const C = copy.family;
  return (
    <section aria-label={C.notModelled} data-family-intake="not_modelled" className="duo max-w-[75ch] border border-line bg-sheet px-4 py-3 text-[13px]">
      <p className="mb-1">
        <Chip>{C.notModelled}</Chip>
      </p>
      <p className="text-body">{fi.not_modelled?.reason}</p>
    </section>
  );
}

function Questions({ c, fi, setCase }: { c: CaseResponse; fi: Intake; setCase: (c: CaseResponse) => void }) {
  const C = copy.family;
  const [f, setF] = useState<FamilyForm>(() => initialForm(fi));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [done, setDone] = useState(false);
  const status = useRef<HTMLParagraphElement>(null);
  const base = useId().replace(/[^a-zA-Z0-9-]/g, "");
  const saved = c.family_facts ?? undefined;
  const qs = asked(fi);
  const known = fi.questions.filter((q) => q.state === "known");
  const ro = caseReadOnly(c) || Boolean(c.pinned);
  const bad = formErrors(fi, f);
  const open = fi.questions.filter((q) => q.state === "missing").length;

  const put = async (next: FamilyForm) => {
    setBusy(true);
    setError(null);
    try {
      const r = await api.putFamilyFacts(c.id!, body(fi, next, saved));
      setCase({ ...c, family_intake: r.family_intake, family_facts: r.family_facts ?? undefined });
      setDone(true);
      requestAnimationFrame(() => status.current?.focus());
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <section aria-labelledby={`${base}-h`} data-family-intake={fi.status} className="max-w-[75ch] border border-line bg-sheet px-4 py-3">
      <h2 id={`${base}-h`} className="mb-1 text-[13px] font-medium">
        {C.title}
      </h2>
      <p className="duo mb-1 text-[13px] text-body">
        {fi.family_name}
        {open ? (
          <>
            {" "}
            <Chip tone="ask">{open}</Chip>
          </>
        ) : null}
      </p>
      <p className="duo mb-2 text-[12.5px] text-dim">{C.lede}</p>
      {fi.stale ? (
        <p role="status" className="duo mb-2 text-[13px] text-ask">
          {C.stale}
        </p>
      ) : fi.dropped?.length ? (
        <p role="status" className="duo mb-2 text-[13px] text-ask">
          {C.dropped}
        </p>
      ) : null}
      <form
        className="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (!ro && !bad.length && changed(fi, f)) void put(f);
        }}
      >
        {qs.map((q) => {
          const id = `${base}-${q.id}`;
          const from = (
            <p id={`${id}-from`} className="duo mt-0.5 text-[12.5px] text-dim">
              {C.from}: {q.evidence}
            </p>
          );
          // The template's citations for the work asked about: where it
          // decided origin, and where it was not enough.
          const cite = (label: string, ids: string[]) =>
            ids.length ? (
              <p className="duo flex flex-wrap items-center gap-x-1.5 text-[12.5px] text-dim">
                <span>{label}:</span>
                {ids.slice(0, 4).map((r) => (
                  <RulingLink key={r} id={r} className="inline-flex min-h-11 items-center" />
                ))}
                {ids.length > 4 ? <span>+{ids.length - 4}</span> : null}
              </p>
            ) : null;
          const rulings = (
            <>
              {cite(C.decided, q.confers_origin)}
              {cite(C.notEnough, q.not_enough)}
            </>
          );
          if (q.kind === "yes_no") {
            return (
              <fieldset key={q.id} data-question={q.id} data-state={q.state} className="duo text-[13px]" aria-describedby={`${id}-from`}>
                <legend className="text-body">{q.question}</legend>
                {from}
                {rulings}
                <div className="mt-1 flex flex-wrap gap-x-4">
                  {(["yes", "no"] as const).map((v) => (
                    <label key={v} className="inline-flex min-h-11 items-center gap-2">
                      <input
                        type="radio"
                        name={id}
                        value={v}
                        checked={f[q.id] === v}
                        disabled={ro}
                        onChange={() => setF((x) => ({ ...x, [q.id]: v }))}
                        className="size-5 shrink-0"
                      />
                      <span>{v === "yes" ? C.yes : C.no}</span>
                    </label>
                  ))}
                </div>
              </fieldset>
            );
          }
          const wrong = bad.includes(q.id);
          return (
            <div key={q.id} data-question={q.id} data-state={q.state} className="duo text-[13px]">
              <label htmlFor={id} className="text-body">
                {q.question}
              </label>
              {from}
              {rulings}
              <input
                id={id}
                list={`${base}-countries`}
                autoComplete="country-name"
                className={cx("mt-1 min-h-11 w-56 max-w-full rounded-[2px] border border-line bg-white px-3 py-2 text-sm placeholder:text-faint disabled:text-dim", wrong && "border-fail")}
                value={f[q.id] ?? ""}
                disabled={ro}
                placeholder={C.country}
                aria-invalid={wrong || undefined}
                aria-describedby={wrong ? `${id}-err ${id}-from` : `${id}-from`}
                onChange={(e) => setF((x) => ({ ...x, [q.id]: e.target.value }))}
              />
              {wrong ? (
                <p id={`${id}-err`} className="mt-0.5 text-[12.5px] text-fail">
                  {C.badCountry}
                </p>
              ) : null}
            </div>
          );
        })}
        <datalist id={`${base}-countries`}>
          {COUNTRIES.map((n) => (
            <option key={n} value={n} />
          ))}
        </datalist>
        {qs.length ? (
          <div className="flex flex-wrap items-center gap-3">
            <Button type="submit" variant="ghost" className="min-h-11" disabled={busy || bad.length > 0 || !changed(fi, f)} {...writeGate(ro)}>
              {busy ? C.saving : C.save}
            </Button>
            {bad.length ? (
              <span role="alert" className="text-[12.5px] text-fail">
                {C.fixFields}
              </span>
            ) : null}
          </div>
        ) : null}
        <p ref={status} tabIndex={-1} aria-live="polite" data-family-status className="duo text-[13px] text-body outline-none">
          {done ? (open ? C.saved : `${C.complete} ${C.saved}`) : !qs.length ? C.complete : ""}
        </p>
        {c.pinned ? <p className="duo text-[12.5px] text-dim">{C.demo}</p> : null}
        {error ? <ErrorBox error={error} onRetry={isStatus(error, 409) ? undefined : () => void put(f)} /> : null}
      </form>
      {known.length ? (
        <div className="duo mt-2 text-[12.5px] text-dim">
          <p>{C.known}:</p>
          <ul className="list-disc pl-4">
            {known.map((q) => (
              <li key={q.id}>
                {q.question} {q.known}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      {saved ? <Yours c={c} saved={saved} clear={() => put({})} /> : null}
    </section>
  );
}

function Yours({ c, saved, clear }: { c: CaseResponse; saved: SuppliedFamilyFacts; clear: () => Promise<void> }) {
  const C = copy.family;
  const qs = new Map((c.family_intake?.questions ?? []).map((q) => [q.id, q.question]));
  const entries = Object.entries(saved.answers ?? {});
  return (
    <div data-family-supplied className="mt-2">
      <Fold title={C.yours} meta={C.yoursMeta(entries.length, formatDate(saved.updated_at))}>
        <div className="duo flex flex-col gap-1.5 text-[12.5px] text-dim">
          <p>{C.yoursNote}</p>
          {saved.stale ? <p className="text-ask">{C.stale}</p> : null}
          <ul className="list-disc pl-4 text-body">
            {entries.map(([k, a]) => (
              <li key={k}>
                {qs.get(k) ?? k} {answerText(a, C.yes, C.no)}
              </li>
            ))}
          </ul>
          <div>
            <Button variant="ghost" className="min-h-11" onClick={() => void clear()} {...writeGate(caseReadOnly(c) || Boolean(c.pinned))}>
              {C.clear}
            </Button>
          </div>
        </div>
      </Fold>
    </div>
  );
}
