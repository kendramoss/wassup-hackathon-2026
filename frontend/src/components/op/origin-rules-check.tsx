"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { api, errorText, type CaseResponse, type OriginMarket, type OriginRules } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { Country, Fold } from "./ui";

// The EU and UK non-preferential origin rules (D760-D766), run from the
// case's product codes. A rules check only: the label stays visible with
// every answer; the rule text, source, version and hash sit under Details
// (rule 16). The full question-and-answer intake is later work (#770): here
// the missing facts are listed, not asked.

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

export function OriginRulesView({ v, parts }: { v: OriginRules; parts?: string[] }) {
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
      <p className="read text-[15px]">
        {r.status === "decided" && r.origin ? (
          <>
            <Country name={r.origin} />{" "}
          </>
        ) : null}
        {head}
      </p>
      {r.reason ? <p className="duo mt-1.5 text-[13px] text-body">{sentence(r.status === "needs_judgement" ? firstSentence(r.reason) : r.reason)}</p> : null}
      {r.status === "needs_judgement" ? <p className="duo mt-1.5 text-[13px] text-body">{C.judgementNext}</p> : null}
      {needs.length ? (
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

// OriginRulesCheck runs the market's check on load for a case with codes;
// without codes it says what to add.
export function OriginRulesCheck({ c, market }: { c: CaseResponse; market: OriginMarket }) {
  const C = copy.originRules;
  const [v, setV] = useState<OriginRules | null>(null);
  const [err, setErr] = useState("");
  const ok = canCheckOrigin(c);
  const product = c.product;
  useEffect(() => {
    if (!ok || !product) return;
    let live = true;
    const ac = new AbortController();
    api
      .originRules(market, { product }, ac.signal)
      .then((r) => live && setV(r))
      .catch((e) => live && setErr(errorText(e instanceof Error ? e.message : String(e))));
    return () => {
      live = false;
      ac.abort();
    };
  }, [ok, product, market]);
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
  return <OriginRulesView v={v} parts={c.product?.components?.map((x) => x.name ?? "")} />;
}
