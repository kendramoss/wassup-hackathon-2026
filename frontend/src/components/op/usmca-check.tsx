"use client";

import { useEffect, useRef, useState } from "react";
import { api, type CaseResponse, type UsmcaCheckResponse } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { MAX_MATERIALS, blankForm, body, check, factList, exactNote, hasErrors, keyOf, needsValue, runner, type OriginAnswer, type UsmcaForm } from "@/lib/usmca-check";
import { Button, Chip, Fold, cx } from "./ui";

// The USMCA origin-rule check (D740): does the selected USMCA rule for this
// good hold on the facts given? Deterministic rules on the server, no model
// call, nothing saved. It is a separate question from the origin above, so it
// sits in its own fold and says "selected origin rule only".

const C = copy.usmcaCheck;
const TONE = { met: "ok", not_met: "fail", needs_information: "ask", not_supported: "ask" } as const;

export function UsmcaResult({ res, stale }: { res: UsmcaCheckResponse; stale?: boolean }) {
  // A result for older facts is not shown at all: only the request to check again.
  if (stale)
    return (
      <p data-usmca-stale role="status" className="duo mt-3 text-[13px] text-ask">
        {C.stale}
      </p>
    );
  const needs = factList(res.missing_facts);
  const rvc = res.rvc;
  const row = res.source.row;
  return (
    <div data-usmca={res.status} className="mt-3 flex flex-col gap-2">
      <p className="read flex flex-wrap items-center gap-2 text-[15px]">
        <Chip tone={TONE[res.status]}>{C.status[res.status]}</Chip>
        <span className="duo text-[13px] text-dim">{C.label}</span>
      </p>
      {res.status === "not_met" ? <p className="duo text-[13px] text-body">{C.notMet}</p> : null}
      {res.status === "met" ? <p className="duo text-[13px] text-body">{C.met}</p> : null}
      {res.status === "not_supported" ? (
        <>
          <p className="duo text-[13px] text-body">{C.notCovered}</p>
          {res.unsupported_reasons?.length ? (
            <ul className="duo flex list-disc flex-col gap-0.5 pl-4 text-[13px] text-dim">
              {res.unsupported_reasons.map((r) => (
                <li key={r}>{r}</li>
              ))}
            </ul>
          ) : null}
        </>
      ) : null}
      {rvc && rvc.status === "computed" ? (
        <p data-usmca-calc className="duo text-[13px] text-body">
          {C.calc} ({rvc.method === "net_cost" ? C.methodNc : C.methodTv}): <span className="tabular-nums text-ink">{rvc.display}%</span>
          {exactNote(rvc) ? (
            <span className="text-dim">
              {" "}
              · {C.exact} <span className="tabular-nums">{exactNote(rvc)}%</span>
            </span>
          ) : null}
          {rvc.display_note ? <span className="block text-[12px] text-dim">{rvc.display_note}</span> : null}
        </p>
      ) : null}
      {rvc && rvc.status === "undefined_denominator" ? (
        <p data-usmca-calc className="duo text-[13px] text-body">
          {C.calc}: {C.undefinedCalc}
        </p>
      ) : null}
      {needs.length ? (
        <div>
          <p className="text-[13px] text-ink">{C.needs}</p>
          <ul data-usmca-needs className="duo mt-1 flex list-disc flex-col gap-0.5 pl-4 text-[13px] text-body">
            {needs.map((n) => (
              <li key={n}>{n}</li>
            ))}
          </ul>
        </div>
      ) : null}
      <Fold title={C.details} meta={row?.code}>
        <div className="duo flex max-w-[75ch] flex-col gap-2 text-[13px] text-body">
          {row ? (
            <p>
              <span className="text-dim">{C.rule}: </span>
              {row.code} · {row.description}
              <span className="block text-dim">{row.source?.citation}</span>
              <span className="block text-dim">{row.source?.excerpt}</span>
            </p>
          ) : null}
          {res.alternatives?.length ? (
            <div>
              <p className="text-ink">{C.how}</p>
              <ul className="mt-1 flex list-disc flex-col gap-0.5 pl-4">
                {res.alternatives.map((a) => (
                  <li key={a.id}>
                    {a.label}: {C.outcome[a.result]}
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
          <p className="text-dim">
            {C.snapshot}: {res.source.snapshot.source} · {res.source.snapshot.revision} · {res.source.snapshot.retrieved_on}
            <span className="block">
              {C.version}: {res.source.data_version}
            </span>
          </p>
          <div>
            <p className="text-ink">{C.scope}</p>
            <ul className="mt-1 flex list-disc flex-col gap-0.5 pl-4 text-dim">
              {res.limits.map((l) => (
                <li key={l}>{l}</li>
              ))}
            </ul>
          </div>
        </div>
      </Fold>
    </div>
  );
}

const field = "h-11 rounded-[2px] border border-edge bg-white px-2 text-sm focus:border-ok";
const bad = "border-fail";

function Problem({ id, children }: { id: string; children?: string }) {
  return children ? (
    <span id={id} className="duo text-[12px] text-fail">
      {children}
    </span>
  ) : null;
}

export function UsmcaCheck({ c }: { c: CaseResponse }) {
  const [f, setF] = useState<UsmcaForm>(() => blankForm(c.product?.hts?.trim() ?? ""));
  const [out, setOut] = useState<{ key: string; res: UsmcaCheckResponse } | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [tried, setTried] = useState(false);
  const [focusAsk, setFocusAsk] = useState(0);
  const job = useRef(runner<UsmcaCheckResponse>());
  useEffect(() => {
    const j = job.current;
    return () => j.retire();
  }, []);

  const errors = check(f);
  useEffect(() => {
    if (!focusAsk || typeof document === "undefined") return;
    const e = check(f);
    const m = e.materials.findIndex(Boolean);
    const first = e.hs ? "usmca-hs" : m >= 0 ? `usmca-m${m}` : e.amount ? "usmca-amount" : "usmca-vnm";
    document.getElementById(first)?.focus();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [focusAsk]);
  const shown = tried ? errors : { materials: [] as (string | undefined)[] };
  const key = keyOf(f);
  const stale = Boolean(out && out.key !== key);

  // Every edit cancels a check in flight, so an old answer can never land on
  // newer facts.
  function edit(next: UsmcaForm) {
    job.current.retire();
    setBusy(false);
    setErr(null);
    setF(next);
  }

  function run() {
    setTried(true);
    if (hasErrors(errors)) {
      // Focus waits for the render that shows the messages, so the first
      // invalid field is announced with its message.
      setFocusAsk((n) => n + 1);
      return;
    }
    const sent = body(f);
    const k = JSON.stringify(sent);
    setErr(null);
    void job.current.run((signal) => api.usmcaCheck(sent, signal), {
      busy: setBusy,
      done: (res) => setOut({ key: k, res }),
      fail: (e) => {
        setOut(null);
        setErr(e instanceof Error && e.message ? e.message : C.failed);
      },
    });
  }

  const value = needsValue(f, out?.res);
  const set = <K extends keyof UsmcaForm>(k: K, v: UsmcaForm[K]) => edit({ ...f, [k]: v });
  const setRow = (i: number, patch: Partial<UsmcaForm["materials"][number]>) => set("materials", f.materials.map((m, j) => (j === i ? { ...m, ...patch } : m)));

  return (
    <Fold title={C.title} meta={C.meta}>
      <form
        data-usmca-form
        noValidate
        className="flex max-w-[75ch] flex-col gap-3 text-[13px]"
        onSubmit={(e) => {
          e.preventDefault();
          run();
        }}
      >
        <p className="duo text-body">{C.lead}</p>
        <div className="flex flex-col gap-1">
          <label className="flex flex-col gap-1">
            <span className="text-dim">{C.good}</span>
            <input
              id="usmca-hs"
              className={cx(field, "w-44", shown.hs && bad)}
              value={f.hs}
              inputMode="decimal"
              autoComplete="off"
              aria-invalid={shown.hs ? true : undefined}
              aria-describedby={shown.hs ? "usmca-hs-err" : undefined}
              onChange={(e) => set("hs", e.target.value)}
            />
          </label>
          <Problem id="usmca-hs-err">{shown.hs}</Problem>
        </div>

        <fieldset className="flex flex-col">
          <legend className="text-dim">{C.materials}</legend>
          <ul className="flex flex-col">
            {f.materials.map((m, i) => (
              <li key={i} data-usmca-material className="flex flex-wrap items-end gap-x-3 gap-y-1 border-t border-line py-2">
                <label className="flex flex-col gap-1">
                  <span className="sr-only">{C.materialCode(i + 1)}</span>
                  <input
                    id={`usmca-m${i}`}
                    className={cx(field, "w-36", shown.materials[i] && bad)}
                    placeholder={C.hsPlaceholder}
                    value={m.hs}
                    inputMode="decimal"
                    autoComplete="off"
                    aria-label={C.materialCode(i + 1)}
                    aria-invalid={shown.materials[i] ? true : undefined}
                    aria-describedby={shown.materials[i] ? `usmca-m${i}-err` : undefined}
                    onChange={(e) => setRow(i, { hs: e.target.value })}
                  />
                </label>
                <label className="flex items-center gap-2">
                  <span className="text-dim">{i === 0 ? C.originColumn : <span className="sr-only">{C.originColumn}</span>}</span>
                  <select className={cx(field, "w-28")} aria-label={C.materialOrigin(i + 1)} value={m.origin} onChange={(e) => setRow(i, { origin: e.target.value as OriginAnswer })}>
                    <option value="unknown">{C.originUnknown}</option>
                    <option value="originating">{C.originYes}</option>
                    <option value="non_originating">{C.originNo}</option>
                  </select>
                </label>
                <button
                  type="button"
                  className="inline-flex min-h-11 min-w-11 items-center justify-center text-dim hover:text-fail"
                  aria-label={C.removeMaterial(i + 1)}
                  onClick={() => set("materials", f.materials.filter((_, j) => j !== i))}
                >
                  {C.removeMark}
                </button>
                <Problem id={`usmca-m${i}-err`}>{shown.materials[i]}</Problem>
              </li>
            ))}
          </ul>
          {f.materials.length < MAX_MATERIALS ? (
            <div>
              <Button type="button" variant="ghost" onClick={() => set("materials", [...f.materials, { hs: "", origin: "unknown" }])}>
                {C.addMaterial}
              </Button>
            </div>
          ) : null}
          <label className="mt-2 flex min-h-11 cursor-pointer items-center gap-2">
            <input type="checkbox" className="size-5 accent-[var(--ok)]" checked={f.complete} onChange={(e) => set("complete", e.target.checked)} />
            <span className="text-body">{C.complete}</span>
          </label>
        </fieldset>

        {value ? (
          <div data-usmca-value className="flex flex-wrap items-end gap-3">
            <label className="flex flex-col gap-1">
              <span className="text-dim">{C.value}</span>
              <select className={cx(field, "w-48")} value={f.method} onChange={(e) => set("method", e.target.value as UsmcaForm["method"])}>
                <option value="">{C.valueNone}</option>
                <option value="transaction_value">{C.methodTv}</option>
                <option value="net_cost">{C.methodNc}</option>
              </select>
            </label>
            {f.method ? (
              <>
                <div className="flex flex-col gap-1">
                  <label className="flex flex-col gap-1">
                    <span className="text-dim">{f.method === "net_cost" ? C.amountNc : C.amountTv}</span>
                    <input
                      id="usmca-amount"
                      key={f.method}
                      className={cx(field, "w-32 text-right tabular-nums", shown.amount && bad)}
                      inputMode="decimal"
                      autoComplete="off"
                      value={f.method === "net_cost" ? f.nc : f.tv}
                      aria-invalid={shown.amount ? true : undefined}
                      aria-describedby={shown.amount ? "usmca-amount-err" : undefined}
                      onChange={(e) => set(f.method === "net_cost" ? "nc" : "tv", e.target.value)}
                    />
                  </label>
                  <Problem id="usmca-amount-err">{shown.amount}</Problem>
                </div>
                <div className="flex flex-col gap-1">
                  <label className="flex flex-col gap-1">
                    <span className="text-dim">{C.vnm}</span>
                    <input
                      id="usmca-vnm"
                      className={cx(field, "w-32 text-right tabular-nums", shown.vnm && bad)}
                      inputMode="decimal"
                      autoComplete="off"
                      value={f.vnm}
                      aria-invalid={shown.vnm ? true : undefined}
                      aria-describedby={shown.vnm ? "usmca-vnm-err" : undefined}
                      onChange={(e) => set("vnm", e.target.value)}
                    />
                  </label>
                  <Problem id="usmca-vnm-err">{shown.vnm}</Problem>
                </div>
                <span className="duo basis-full text-[12px] text-dim">{C.sameUnit}</span>
              </>
            ) : null}
          </div>
        ) : null}

        <label className="flex min-h-11 cursor-pointer items-start gap-2">
          <input type="checkbox" className="mt-2.5 size-5 shrink-0 accent-[var(--ok)]" checked={f.simple} onChange={(e) => set("simple", e.target.checked)} />
          <span className="py-2.5 text-body">{C.simple}</span>
        </label>

        <div className="flex flex-wrap items-center gap-3">
          <Button type="submit" disabled={busy}>
            {busy ? C.checking : C.check}
          </Button>
          {tried && hasErrors(errors) ? (
            <span role="alert" className="duo text-fail">
              {C.fixInputs}
            </span>
          ) : null}
        </div>
        {err ? (
          <p role="alert" className="duo text-fail">
            {err}
          </p>
        ) : null}
        {out ? <UsmcaResult res={out.res} stale={stale} /> : null}
      </form>
    </Fold>
  );
}
