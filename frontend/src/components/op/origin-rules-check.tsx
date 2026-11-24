"use client";

import Link from "next/link";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { api, errorText, type CaseResponse, type OriginMarket, type OriginRules, type SuppliedOriginFacts } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { formatDate } from "@/lib/format";
import { blankForm, body as intakeBody, classify, formErrors, hasInput, suppliedCount, type IntakeForm } from "@/lib/origin-intake";
import { caseReadOnly, isStatus, writeGate } from "@/lib/roles";
import { ErrorBox } from "./chrome";
import { Button, Chip, Country, Fold, cx } from "./ui";

// The EU and UK non-preferential origin rules (D760-D766), run from the
// case's product codes. A rules check only: the label stays visible with
// every answer; the rule text, source, version and hash sit under Details
// (rule 16). Missing information is asked (#770, D800-D802): each question
// names the document that settles it, and answers re-run the check on the
// committed tables. A rule that is not modelled gets no control.

function sentence(s: string): string {
  const t = s.trim();
  return t ? t[0].toUpperCase() + t.slice(1) + (/[.!?]$/.test(t) ? "" : ".") : t;
}

// firstSentence: a judgement's reason up to its first full stop before a
// capital letter ("reg. 9" and "Art. 60" are not ends); the rest is under
// Details (rule 16).
export function firstSentence(s: string): string {
  const m = s.match(/^([\s\S]*?[.)])\s+(?=[A-Z])/);
  return m ? m[1].replace(/\.$/, "") : s;
}

// canCheckOrigin: the product gives a code for the good and at least one part.
export function canCheckOrigin(c: CaseResponse): boolean {
  const p = c.product;
  return Boolean(p?.hts?.trim() && p.components?.some((x) => x.hts?.trim()));
}

type Need = NonNullable<OriginRules["result"]["needs"]>[number];

function needText(n: Need, parts?: string[]): string {
  if (n.material === undefined) return n.label;
  const name = parts?.[n.material]?.trim();
  return name && !n.label.includes(name) ? `${n.label} (${name})` : n.label;
}

export function OriginRulesView({ v, parts, intake }: { v: OriginRules; parts?: string[]; intake?: ReactNode }) {
  const C = copy.originRules;
  const r = v.result;
  const market = v.jurisdiction;
  const head = r.status === "decided" ? C.decided[market] : r.status === "needs_judgement" ? C.judgement : C.undetermined;
  const needs = r.needs ?? [];
  return (
    <section aria-label={C.title[market]} data-origin-rules={r.status} data-market={market} className="max-w-[75ch] border border-line bg-sheet px-4 py-3">
      <h2 className="eyebrow mb-1">{C.title[market]}</h2>
      <p data-origin-question className="duo mb-0.5 text-[13px] text-body">
        {C.question[market]}
      </p>
      <p data-origin-label className="duo mb-1.5 text-[12.5px] text-dim">
        {v.label}
      </p>
      {r.status === "undetermined" ? (
        <p className="mb-1">
          <Chip tone="ask">{C.missing}</Chip>
        </p>
      ) : r.status === "needs_judgement" ? (
        <p className="mb-1">
          <Chip>{C.notModelled}</Chip>
        </p>
      ) : null}
      <p data-origin-outcome tabIndex={-1} aria-live="polite" className="read text-[15px] outline-none">
        {r.status === "decided" && r.origin ? (
          <>
            <Country name={r.origin} />{" "}
          </>
        ) : null}
        {head}
      </p>
      {r.reason ? <p className="duo mt-1.5 text-[13px] text-body">{sentence(r.status === "needs_judgement" ? firstSentence(r.reason) : r.reason)}</p> : null}
      {r.status === "needs_judgement" ? <p className="duo mt-1.5 text-[13px] text-body">{C.judgementNext}</p> : null}
      {intake && needs.length ? (
        intake
      ) : needs.length ? (
        <div data-origin-needs className="duo mt-2 text-[13px] text-body">
          <p>{C.toDecide}</p>
          <ul className="mt-0.5 list-disc pl-4">
            {needs.map((n, i) => (
              <li key={i}>
                {needText(n, parts)}
                {n.options?.length ? (
                  <ul className="mt-0.5 list-[circle] pl-4 text-dim">
                    {n.options.map((o) => (
                      <li key={o.key}>{o.key === "none" ? C.none : `${o.code}: ${o.description}`}</li>
                    ))}
                  </ul>
                ) : null}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      <div className="mt-2">
        <Fold title={C.details}>
          <div className="duo flex flex-col gap-2 text-[12.5px] text-dim">
            {r.status === "needs_judgement" && r.reason && firstSentence(r.reason) !== r.reason ? <p className="text-body">{sentence(r.reason)}</p> : null}
            {r.rule ? (
              <div>
                <p className="text-body">
                  {C.rule}: {r.rule.code} · {r.rule.description}
                </p>
                <p className="whitespace-pre-line">{r.rule.text}</p>
                {r.rule.alternatives.length > 1 ? (
                  <ul className="mt-0.5 list-disc pl-4">
                    {r.rule.alternatives.map((a) => (
                      <li key={a.key}>
                        {a.text.split("\n")[0]} · {C.outcome[a.outcome] ?? a.outcome}
                      </li>
                    ))}
                  </ul>
                ) : null}
              </div>
            ) : null}
            {r.basis ? <p>{r.basis}</p> : null}
            {r.assumes?.length ? (
              <div>
                <p>{C.assumes}:</p>
                <ul className="list-disc pl-4">
                  {r.assumes.map((a, i) => (
                    <li key={i}>{sentence(a)}</li>
                  ))}
                </ul>
              </div>
            ) : null}
            <p>
              {C.who}: {v.administration}
            </p>
            <dl data-origin-source className="grid gap-0.5">
              <dt className="text-body">{C.source}</dt>
              <dd>{v.source.title}</dd>
              <dd>
                {C.version}: {v.source.version}
              </dd>
              <dd>
                {C.retrieved}: {v.source.retrieved}
              </dd>
              <dd className="break-all">
                {C.hash}: {v.source.sha256}
              </dd>
              <dd>
                {C.licence}: {v.source.licence}
              </dd>
            </dl>
            <p>{v.note}</p>
          </div>
        </Fold>
      </div>
    </section>
  );
}

// OriginRulesCheck runs the market's check on load for a case with codes,
// with the answers saved on the case; without codes it says what to add.
// onSaved runs after answers are saved (the comparison view re-reads its
// report). bare shows only the questions and the saved answers: the
// comparison view's card already shows the outcome.
export function OriginRulesCheck({ c, market, onSaved, bare }: { c: CaseResponse; market: OriginMarket; onSaved?: () => void; bare?: boolean }) {
  const C = copy.originRules;
  const [v, setV] = useState<OriginRules | null>(null);
  const [err, setErr] = useState("");
  // The answers as saved; the PUT answer replaces them.
  const [saved, setSaved] = useState<SuppliedOriginFacts | undefined>(() => c.origin_facts?.[market]);
  const [first] = useState(saved);
  // After Re-check the questions are new: focus the next one, else the
  // outcome, so a keyboard or screen-reader user is not left on nothing.
  const box = useRef<HTMLDivElement>(null);
  const focusNext = useRef(false);
  useEffect(() => {
    if (!focusNext.current) return;
    focusNext.current = false;
    const el = box.current?.querySelector<HTMLElement>("[data-origin-intake] input:not([disabled])") ?? box.current?.querySelector<HTMLElement>("[data-origin-outcome]") ?? document.querySelector<HTMLElement>(`[data-check="${market}_origin"] [data-outcome]`);
    el?.focus();
  }, [v, market]);
  const ok = canCheckOrigin(c);
  const product = c.product;
  useEffect(() => {
    if (!ok || !product) return;
    let live = true;
    const ac = new AbortController();
    api
      .originRules(market, { product, ...(first ? { supplied: first } : {}) }, ac.signal)
      .then((r) => live && setV(r))
      .catch((e) => live && setErr(errorText(e instanceof Error ? e.message : String(e))));
    return () => {
      live = false;
      ac.abort();
    };
  }, [ok, product, market, first]);
  if (!ok) {
    return (
      <div data-origin-rules="codes" className="duo max-w-[75ch] text-[13px] text-dim">
        <p className="text-body">{C.title[market]}</p>
        <p className="mt-0.5">{C.addCodes}</p>
        <ul className="mt-0.5 list-disc pl-4">
          {!c.product?.hts?.trim() ? <li>{C.needGood}</li> : null}
          {!c.product?.components?.some((x) => x.hts?.trim()) ? <li>{C.needPart}</li> : null}
        </ul>
        <Link href={`/cases/new?from=${encodeURIComponent(c.id ?? "")}#field-good-hts`} className="mt-1 inline-flex min-h-11 items-center text-ok underline underline-offset-2">
          {copy.ruleCheck.addCodesLink}
        </Link>
      </div>
    );
  }
  if (err)
    return (
      <p role="alert" className="duo text-[13px] text-ask">
        {err}
      </p>
    );
  if (!v)
    return (
      <p role="status" className="duo text-[13px] text-dim">
        {C.running}
      </p>
    );
  const parts = c.product?.components?.map((x) => x.name ?? "");
  // Answers saved before the parts list was replaced are not read (D806):
  // they are not resent either, so the questions start fresh.
  const staleAll = Boolean(saved?.stale || v.supplied_stale);
  const stalePart = Boolean(v.stale_supplied?.length || v.supplied_dropped?.length);
  const save = async (facts: Parameters<typeof api.putOriginFacts>[2]) => {
    // The answers as this page read them: 409 if someone saved since.
    const r = await api.putOriginFacts(c.id!, market, { ...facts, expected_updated_at: saved?.updated_at ?? "" });
    focusNext.current = true;
    setV(r.check);
    setSaved(r.origin_facts[market]);
    onSaved?.();
  };
  const intake = c.pinned ? null : <OriginIntake key={v.result.status + JSON.stringify(v.result.needs ?? [])} c={c} v={v} saved={staleAll ? undefined : saved} save={save} />;
  const note =
    saved && (staleAll || stalePart) ? (
      <p data-origin-stale role="status" className="duo text-[13px] text-ask">
        {staleAll ? C.staleAll : C.stale}
      </p>
    ) : null;
  const yours = saved ? <Supplied c={c} saved={saved} stale={staleAll || stalePart} clear={() => save({})} /> : null;
  if (bare)
    return (
      <div ref={box} className="flex max-w-[75ch] flex-col gap-2">
        {note}
        {v.result.status === "undetermined" ? intake : null}
        {yours}
      </div>
    );
  return (
    <div ref={box} className="flex max-w-[75ch] flex-col gap-2">
      {note}
      <OriginRulesView v={v} parts={parts} intake={intake} />
      {c.pinned && v.result.status === "undetermined" ? <p className="duo text-[12.5px] text-dim">{C.demo}</p> : null}
      {yours}
    </div>
  );
}

// partOf names the part a need is about.
function partOf(c: CaseResponse, i: number | undefined): string {
  return i === undefined ? "" : (c.product?.components?.[i]?.name ?? "").trim();
}

// OriginIntake asks for the missing information the check named: one control
// per need a person can supply, each with the document that settles it.
// Needs that belong on the product link there; needs only a person can read
// are listed with no control.
export function OriginIntake({
  c,
  v,
  saved,
  save,
}: {
  c: CaseResponse;
  v: OriginRules;
  saved?: SuppliedOriginFacts;
  save: (facts: Parameters<typeof api.putOriginFacts>[2]) => Promise<void>;
}) {
  const C = copy.originRules;
  const [f, setF] = useState<IntakeForm>(blankForm);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const needs = v.result.needs ?? [];
  const ask = needs.filter((n) => classify(n) === "ask");
  const product = needs.filter((n) => classify(n) === "product");
  const person = needs.filter((n) => classify(n) === "person");
  const ro = caseReadOnly(c);
  const bad = formErrors(f);
  const parts = c.product?.components?.map((x) => x.name ?? "");
  const submit = async () => {
    setBusy(true);
    setError(null);
    try {
      await save(intakeBody(c, saved, f));
    } catch (e) {
      setError(e);
      setBusy(false);
    }
  };
  const set = (next: Partial<IntakeForm>) => setF((x) => ({ ...x, ...next }));
  const field = "min-h-10 rounded-[2px] border border-line bg-white px-3 py-2 text-sm placeholder:text-faint disabled:text-dim";
  return (
    <div data-origin-intake className="mt-3 flex flex-col gap-3">
      {ask.length ? (
        <form
          className="flex flex-col gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (hasInput(f) && !bad.length && !ro) void submit();
          }}
        >
          <h3 className="text-[13px] font-medium">{C.ask}</h3>
          {ask.map((n, i) => {
            const id = `oi-${n.kind}-${n.key}-${n.material ?? "g"}`.replace(/[^a-zA-Z0-9-]/g, "-");
            const label = needText(n, parts);
            const from = n.evidence ? (
              <p className="duo mt-0.5 text-[12.5px] text-dim">
                {C.from}: {n.evidence}
              </p>
            ) : null;
            if (n.kind === "row" || n.kind === "fact") {
              const opts =
                n.kind === "row"
                  ? (n.options ?? []).map((o) => ({ value: o.key, text: o.key === "none" ? C.none : `${o.code}: ${o.description}` }))
                  : [
                      { value: "yes", text: C.yes },
                      { value: "no", text: C.no },
                    ];
              const cur = n.kind === "row" ? f.row : f.answers[n.key] === undefined ? undefined : f.answers[n.key] ? "yes" : "no";
              return (
                <fieldset key={i} data-need={n.kind} className="duo text-[13px]">
                  <legend className="text-body">{label}</legend>
                  {from}
                  <div className={cx("mt-1 flex gap-x-4 gap-y-1", n.kind === "row" ? "flex-col" : "flex-row flex-wrap")}>
                    {opts.map((o) => (
                      <label key={o.value} className="inline-flex min-h-10 items-center gap-2">
                        <input
                          type="radio"
                          name={id}
                          value={o.value}
                          checked={cur === o.value}
                          disabled={ro}
                          onChange={() => (n.kind === "row" ? set({ row: o.value }) : set({ answers: { ...f.answers, [n.key]: o.value === "yes" } }))}
                          className="size-4 shrink-0"
                        />
                        <span>{o.text}</span>
                      </label>
                    ))}
                  </div>
                </fieldset>
              );
            }
            const idx = n.material;
            const kind = n.kind === "share" ? (n.key === "weight_share" ? "weight" : "value") : n.key === "good_code" ? "good" : n.key === "material_origin" ? "origin" : "hts";
            const val =
              kind === "good" ? (f.good ?? "") : kind === "value" ? (f.value[idx!] ?? "") : kind === "weight" ? (f.weight[idx!] ?? "") : kind === "origin" ? (f.origin[idx!] ?? "") : (f.hts[idx!] ?? "");
            const onChange = (x: string) =>
              kind === "good"
                ? set({ good: x })
                : set({ [kind]: { ...(f[kind] as Record<number, string>), [idx!]: x } } as Partial<IntakeForm>);
            const wrong = bad.includes(kind === "good" ? "good" : `${kind}:${idx}`);
            return (
              <div key={i} data-need={n.kind} className="duo text-[13px]">
                <label htmlFor={id} className="text-body">
                  {label}
                </label>
                {from}
                <div className="mt-1 flex items-center gap-2">
                  <input
                    id={id}
                    inputMode={kind === "value" || kind === "weight" ? "decimal" : kind === "origin" ? "text" : "numeric"}
                    className={cx(field, "w-40", wrong && "border-fail")}
                    value={val}
                    disabled={ro}
                    aria-invalid={wrong || undefined}
                    aria-describedby={wrong ? `${id}-err` : undefined}
                    placeholder={kind === "value" || kind === "weight" ? "e.g. 70" : kind === "origin" ? "e.g. China" : "e.g. 8529.90"}
                    onChange={(e) => onChange(e.target.value)}
                  />
                  {kind === "value" || kind === "weight" ? <span className="text-dim">{C.percent}</span> : null}
                </div>
                {wrong ? (
                  <p id={`${id}-err`} className="mt-0.5 text-[12.5px] text-fail">
                    {kind === "value" || kind === "weight" ? C.badShare : C.badCode}
                  </p>
                ) : null}
              </div>
            );
          })}
          <div className="flex flex-wrap items-center gap-3">
            <Button type="submit" variant="ghost" disabled={!hasInput(f) || bad.length > 0 || busy} {...writeGate(ro)}>
              {busy ? C.rechecking : C.recheck}
            </Button>
            {bad.length ? (
              <span role="alert" className="text-[12.5px] text-fail">
                {C.fixFields}
              </span>
            ) : null}
          </div>
          {error ? <ErrorBox error={error} onRetry={isStatus(error, 409) ? undefined : submit} /> : null}
        </form>
      ) : null}
      {product.length ? (
        <div className="duo text-[13px]">
          <p className="text-body">{C.onProduct}</p>
          <ul className="mt-0.5 list-disc pl-4">
            {product.map((n, i) => (
              <li key={i}>
                {needText(n, parts)}
                {n.evidence ? (
                  <span className="text-dim">
                    {" "}
                    · {C.from}: {n.evidence}
                  </span>
                ) : null}
              </li>
            ))}
          </ul>
          <Link href={`/cases/new?from=${encodeURIComponent(c.id ?? "")}`} className="inline-flex min-h-11 items-center text-ok underline underline-offset-2">
            {C.editProduct}
          </Link>
        </div>
      ) : null}
      {person.length ? (
        <div data-origin-person className="duo text-[13px]">
          <p className="text-body">{C.person}</p>
          <ul className="mt-0.5 list-disc pl-4">
            {person.map((n, i) => (
              <li key={i}>
                {needText(n, parts)}
                {n.detail ? <span className="block whitespace-pre-line text-[12.5px] text-dim">{n.detail}</span> : null}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </div>
  );
}

// Supplied shows the saved answers: how many, that the requester supplied
// them and when, and a way to clear them.
function Supplied({ c, saved, stale, clear }: { c: CaseResponse; saved: SuppliedOriginFacts; stale: boolean; clear: () => Promise<void> }) {
  const C = copy.originRules;
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const n = suppliedCount(saved);
  const go = async () => {
    setBusy(true);
    setError(null);
    try {
      await clear();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  return (
    <div data-origin-supplied>
      <Fold title={C.yours} meta={C.yoursMeta(n, formatDate(saved.updated_at))}>
        <div className="duo flex flex-col gap-1.5 text-[12.5px] text-dim">
          <p>{C.yoursNote}</p>
          {stale ? <p className="text-ask">{C.stale}</p> : null}
          <ul className="list-disc pl-4 text-body">
            {saved.row ? <li>{saved.row === "none" ? C.none : saved.row}</li> : null}
            {saved.good_hts ? <li>{saved.good_hts}</li> : null}
            {Object.entries(saved.answers ?? {}).map(([k, b]) => (
              <li key={k}>
                {k}: {b ? C.yes : C.no}
              </li>
            ))}
            {(saved.materials ?? []).map((m) => (
              <li key={m.index}>
                {partOf(c, m.index) || m.name}:{" "}
                {[m.hts, m.origin, m.value_share !== undefined ? `${Math.round(m.value_share * 10000) / 100}% value` : "", m.weight_share !== undefined ? `${Math.round(m.weight_share * 10000) / 100}% weight` : ""]
                  .filter(Boolean)
                  .join(", ")}
              </li>
            ))}
          </ul>
          <div>
            <Button variant="ghost" onClick={go} disabled={busy} {...writeGate(caseReadOnly(c) || Boolean(c.pinned))}>
              {C.clear}
            </Button>
          </div>
          {error ? <ErrorBox error={error} onRetry={go} /> : null}
        </div>
      </Fold>
    </div>
  );
}
