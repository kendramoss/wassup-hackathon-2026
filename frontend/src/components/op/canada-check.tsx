"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { api, errorText, type CanadaMarking, type CaseResponse } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { Button, Country, Fold } from "./ui";

// Canada's marking rules (SOR/94-23, D720): run on request from the case's
// product codes. A rules check only: the label says there is no accuracy
// figure, and it stays visible with every answer (D725).

const statusWords: Record<string, string> = {
  shift: "shifts",
  no_shift: "does not shift",
  needs_fact: "needs a fact",
  missing_hts: "no HTS code",
};

function sentence(s: string): string {
  const t = s.trim();
  return t ? t[0].toUpperCase() + t.slice(1) + (/[.!?]$/.test(t) ? "" : ".") : t;
}

// canCheck: the product gives a code for the good and at least one part.
export function canCheckCanada(c: CaseResponse): boolean {
  const p = c.product;
  return Boolean(p?.hts?.trim() && p.components?.some((x) => x.hts?.trim()));
}

export function CanadaCheckView({ v }: { v: CanadaMarking }) {
  const C = copy.canadaCheck;
  const r = v.result;
  const head = r.status === "decided" ? C.decided : r.status === "out_of_scope" ? C.outOfScope : C.undetermined;
  return (
    <section aria-label={C.title} data-canada-check={r.status} className="max-w-[75ch] border border-line bg-sheet px-4 py-3">
      <h2 className="eyebrow mb-1">{C.title}</h2>
      <p data-canada-label className="duo mb-1.5 text-[12.5px] text-dim">
        {C.label}
      </p>
      <p className="read text-[15px]">
        {r.status === "decided" && r.origin ? (
          <>
            <Country name={r.origin} />{" "}
          </>
        ) : null}
        {head}
      </p>
      {r.reason ? <p className="duo mt-1.5 text-[13px] text-body">{sentence(r.reason)}</p> : null}
      <div className="mt-2">
        {r.materials?.length ? (
          <Fold title={copy.ruleCheck.parts} meta={`${r.materials.length}`}>
            <ul className="duo flex flex-col gap-1 text-[13px]">
              {r.materials.map((m, i) => (
                <li key={i}>
                  <span className="text-ink">{m.name}</span>
                  {m.hts ? <span className="text-dim"> · {m.hts}</span> : null}
                  {m.origin ? <span className="text-dim"> · {m.origin}</span> : null}
                  {" · "}
                  {statusWords[m.status] ?? m.status}
                </li>
              ))}
            </ul>
          </Fold>
        ) : null}
        <Fold title={C.details}>
          {r.rule ? <p className="duo whitespace-pre-line text-[13px] text-body">{r.rule.text}</p> : null}
          {r.basis ? <p className="duo mt-1 text-[12.5px] text-dim">{r.basis}</p> : null}
          {r.assumes?.length ? (
            <ul className="duo mt-1 flex list-disc flex-col gap-0.5 pl-4 text-[12.5px] text-dim">
              {r.assumes.map((a, i) => (
                <li key={i}>{sentence(a)}</li>
              ))}
            </ul>
          ) : null}
          <p className="duo mt-1 text-[12.5px] text-dim">{v.note}</p>
        </Fold>
      </div>
    </section>
  );
}

// CanadaCheck shows a button on a case with codes; the answer replaces it.
// On a Canadian case (auto) it runs on load, and without codes it says what
// to add.
export function CanadaCheck({ c, auto = false }: { c: CaseResponse; auto?: boolean }) {
  const C = copy.canadaCheck;
  const [v, setV] = useState<CanadaMarking | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const ok = Boolean(c.product && canCheckCanada(c));
  const run = async () => {
    setBusy(true);
    setErr("");
    try {
      setV(await api.canadaMarking({ product: c.product! }));
    } catch (e) {
      setErr(errorText(e instanceof Error ? e.message : String(e)));
    } finally {
      setBusy(false);
    }
  };
  // On a Canadian case the check runs on load; state is set only after the
  // answer, never synchronously in the effect.
  const product = c.product;
  useEffect(() => {
    if (!auto || !ok || !product) return;
    let live = true;
    api
      .canadaMarking({ product })
      .then((r) => live && setV(r))
      .catch((e) => live && setErr(errorText(e instanceof Error ? e.message : String(e))));
    return () => {
      live = false;
    };
  }, [auto, ok, product]);
  if (!ok) {
    if (!auto) return null;
    return (
      <p data-canada-check="codes" className="duo max-w-[75ch] text-[13px] text-dim">
        <span className="text-body">{C.title}:</span> {C.addCodes}{" "}
        <Link href={`/cases/new?from=${encodeURIComponent(c.id ?? "")}#field-good-hts`} className="text-ok underline underline-offset-2">
          {copy.ruleCheck.addCodesLink}
        </Link>
      </p>
    );
  }
  if (v) return <CanadaCheckView v={v} />;
  return (
    <div data-canada-check="ask" className="flex flex-wrap items-center gap-3">
      <Button variant="ghost" onClick={run} disabled={busy || (auto && !err)}>
        {busy || (auto && !err) ? C.running : C.run}
      </Button>
      {err ? (
        <p role="alert" className="duo text-[13px] text-ask">
          {err}
        </p>
      ) : null}
    </div>
  );
}
