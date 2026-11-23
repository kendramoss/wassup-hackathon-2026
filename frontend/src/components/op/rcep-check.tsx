"use client";

import { useState } from "react";
import { api, type CaseResponse, type RcepCheckRequest, type RcepCheckResponse } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { Button, Chip, Fold, cx } from "./ui";

// The RCEP preference check (D690): can this product claim the RCEP tariff
// when shipped between RCEP countries? Deterministic rules on the server, no
// model call, nothing saved. It answers a different question from the origin
// above, so it sits in its own fold and says so.

export type PartInput = { originating: boolean; value: string };
export type RcepForm = { madeIn: string; fob: string; parts: PartInput[] };

// madeIn is where the product is made: the rule check's country, else the
// plant of the last operation, else the only plant.
export function madeIn(c: CaseResponse): string {
  if (c.rule_check?.country?.trim()) return c.rule_check.country;
  const p = c.product;
  const plants = p?.plants ?? [];
  const last = p?.operations?.at(-1)?.plant;
  const byOp = plants.find((pl) => pl.id === last)?.country;
  if (byOp) return byOp;
  return plants.length === 1 ? (plants[0].country ?? "") : "";
}

export function blankForm(c: CaseResponse): RcepForm {
  return { madeIn: madeIn(c), fob: "", parts: (c.product?.components ?? []).map(() => ({ originating: false, value: "" })) };
}

function num(s: string): number | undefined {
  const n = Number(s.replace(/[$,\s]/g, ""));
  return s.trim() && Number.isFinite(n) && n >= 0 ? n : undefined;
}

// rcepBody builds the request from the case's parts list and the form. A
// part the user did not tick is "unknown", which the rules count as
// non-originating (Art. 3.5.4).
export function rcepBody(c: CaseResponse, f: RcepForm): RcepCheckRequest {
  const fob = num(f.fob);
  return {
    hs: c.product?.hts ?? "",
    produced_in: f.madeIn.trim(),
    ...(fob ? { fob } : {}),
    materials: (c.product?.components ?? []).map((p, i) => {
      const v = num(f.parts[i]?.value ?? "");
      return {
        name: p.name,
        hs: p.hts ?? "",
        country: p.made_in,
        status: f.parts[i]?.originating ? "originating" : "unknown",
        ...(v !== undefined ? { value: v } : {}),
      };
    }),
  };
}

const R = copy.rcep;

export function verdict(res: RcepCheckResponse): { text: string; tone: "ok" | "fail" | "ask" } {
  if (res.outcome === "yes") return { text: R.yes, tone: "ok" };
  if (res.outcome === "no") return { text: R.no, tone: "fail" };
  return { text: R.unsure, tone: "ask" };
}

export function RcepResult({ res }: { res: RcepCheckResponse }) {
  const v = verdict(res);
  const failing = res.failing ?? [];
  return (
    <div data-rcep={res.outcome} className="mt-3 flex flex-col gap-2">
      <p className="read text-[15px]">
        <Chip tone={v.tone}>{v.text}</Chip>
        {res.applied ? <span className="duo ml-2 text-[13px] text-dim">{res.applied}</span> : null}
      </p>
      {failing.length ? (
        <ul aria-label={R.failing} className="duo flex flex-col gap-1 text-[13px]">
          {failing.map((m) => (
            <li key={m.index}>
              <span className="text-ink">{m.name || R.part}</span>
              {m.hs ? <span className="text-dim"> · {m.hs}</span> : null}
              <span className="text-body"> · {m.reason}</span>
            </li>
          ))}
        </ul>
      ) : null}
      <Fold title={R.details} meta={res.rule?.text}>
        <ul className="duo flex flex-col gap-1 text-[13px] text-body">
          {(res.reasons ?? []).map((r, i) => (
            <li key={i}>{r}</li>
          ))}
        </ul>
        <p className="duo mt-2 text-[12px] text-dim">{res.note}</p>
      </Fold>
    </div>
  );
}

export function RcepCheck({ c }: { c: CaseResponse }) {
  const parts = c.product?.components ?? [];
  const [f, setF] = useState<RcepForm>(() => blankForm(c));
  const [res, setRes] = useState<RcepCheckResponse | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  if (!parts.length) return null;
  const noCode = !c.product?.hts?.trim();
  const field = "h-11 rounded-[2px] border border-edge bg-white px-2 text-sm focus:border-ok";

  async function run() {
    setBusy(true);
    setErr(null);
    try {
      setRes(await api.rcepCheck(rcepBody(c, f)));
    } catch (e) {
      setRes(null);
      setErr(e instanceof Error ? e.message : R.failed);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Fold title={R.title} meta={R.meta}>
      <div data-rcep-form className="flex max-w-[75ch] flex-col gap-3 text-[13px]">
        <p className="duo text-body">{R.lead}</p>
        <div>
          <p className="text-ink">{R.needsTitle}</p>
          <ul data-rcep-needs className="duo mt-1 flex list-disc flex-col gap-0.5 pl-4 text-body">
            {R.needs.map((n) => (
              <li key={n}>{n}</li>
            ))}
          </ul>
        </div>
        {noCode ? (
          <p className="duo text-dim">{R.needCode}</p>
        ) : (
          <>
            <div className="flex flex-wrap gap-3">
              <label className="flex flex-col gap-1">
                <span className="text-dim">{R.madeIn}</span>
                <input className={cx(field, "w-44")} value={f.madeIn} onChange={(e) => setF({ ...f, madeIn: e.target.value })} />
              </label>
              <label className="flex flex-col gap-1">
                <span className="text-dim">{R.price}</span>
                <input className={cx(field, "w-32 text-right tabular-nums")} inputMode="decimal" value={f.fob} onChange={(e) => setF({ ...f, fob: e.target.value })} />
              </label>
            </div>
            <p className="duo text-dim">{R.tick}</p>
            <ul className="flex flex-col">
              {parts.map((p, i) => (
                <li key={i} data-rcep-part className="flex flex-wrap items-center gap-x-4 gap-y-1 border-t border-line py-2">
                  <div className="min-w-0 basis-full sm:basis-52">
                    <span className="text-ink">{p.name}</span>
                    <span className="block text-dim">{[p.hts || R.noCode, p.made_in].filter(Boolean).join(" · ")}</span>
                  </div>
                  <label className="flex min-h-11 min-w-11 cursor-pointer items-center gap-2">
                    <input
                      type="checkbox"
                      className="size-5 accent-[var(--ok)]"
                      aria-label={R.declaredFor(p.name)}
                      checked={f.parts[i]?.originating ?? false}
                      onChange={(e) => setF({ ...f, parts: f.parts.map((x, j) => (j === i ? { ...x, originating: e.target.checked } : x)) })}
                    />
                    <span className="text-body">{R.declared}</span>
                  </label>
                  <label className="flex min-h-11 items-center gap-2">
                    <span className="text-dim">{R.value}</span>
                    <input
                      className={cx(field, "w-24 text-right tabular-nums")}
                      inputMode="decimal"
                      aria-label={R.valueFor(p.name)}
                      value={f.parts[i]?.value ?? ""}
                      onChange={(e) => setF({ ...f, parts: f.parts.map((x, j) => (j === i ? { ...x, value: e.target.value } : x)) })}
                    />
                  </label>
                </li>
              ))}
            </ul>
            <div>
              <Button onClick={run} disabled={busy || !f.madeIn.trim()}>
                {busy ? R.checking : R.check}
              </Button>
            </div>
            {err ? <p role="alert" className="duo text-fail">{err}</p> : null}
            {res ? <RcepResult res={res} /> : null}
          </>
        )}
      </div>
    </Fold>
  );
}
