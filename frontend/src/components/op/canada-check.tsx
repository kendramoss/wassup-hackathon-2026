"use client";

import Link from "next/link";
import { useEffect, useId, useRef, useState } from "react";
import { api, errorText, type CanadaBody, type CanadaMarking, type CaseResponse, type Product } from "@/lib/api/client";
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

// The three yes/no facts the request can carry. Not sure is a missing key,
// never false.
export const FACT_KEYS = ["simple_assembly", "gri_2a", "bending_to_shape"] as const;
export type FactKey = (typeof FACT_KEYS)[number];
export type Answers = Partial<Record<FactKey, boolean>>;
type Held = { key: string; a: Answers; s: Shares };
type Guidance = NonNullable<CanadaMarking["guidance"]>;
type Item = Guidance["missing"][number];

// buildAnswers: the optional request booleans from the answers held here.
// Only true and false are sent; anything else is left out.
export function buildAnswers(a: Answers): Answers {
  const out: Answers = {};
  for (const k of FACT_KEYS) if (typeof a[k] === "boolean") out[k] = a[k];
  return out;
}

// setAnswer: Yes is true, No is false, Not sure (null) removes the key.
export function setAnswer(a: Answers, key: FactKey, value: boolean | null): Answers {
  const { [key]: _drop, ...rest } = a;
  void _drop;
  return value === null ? rest : { ...rest, [key]: value };
}

// factItems: the questions to ask, one per key.
export function factItems(g: Guidance | undefined): Item[] {
  const seen = new Set<string>();
  return (g?.missing ?? []).filter((m) => {
    if (m.kind !== "fact" || !m.answerable || !(FACT_KEYS as readonly string[]).includes(m.key) || seen.has(m.key)) return false;
    seen.add(m.key);
    return true;
  });
}

// itemText: what to add, with the part's name when the item is about a part
// (index 0 is a real part).
export function itemText(m: Item, parts?: string[]): string {
  if (m.material === undefined) return m.label;
  const name = parts?.[m.material]?.trim() || copy.canadaCheck.part(m.material + 1);
  return `${m.label} (${name})`;
}

// Shares: typed as a percent (0 to 100), sent as 0 to 1. Empty is unknown (the
// key is left out); 0 is a real 0.
export type Shares = Record<string, number>;
export const shareKey = (key: string, material: number) => `${key}@${material}`;

// parsePercent: null for empty, undefined for a bad entry, else the share.
export function parsePercent(text: string): number | null | undefined {
  const t = text.trim().replace(",", ".");
  if (t === "") return null;
  if (!/^\d+(\.\d+)?$/.test(t)) return undefined;
  const n = Number(t);
  // Not rounded: 7.00004% must stay above the 7% limit.
  return Number.isFinite(n) && n <= 100 ? n / 100 : undefined;
}

export function setShare(s: Shares, k: string, value: number | null): Shares {
  const { [k]: _drop, ...rest } = s;
  void _drop;
  return value === null ? rest : { ...rest, [k]: value };
}

// buildBody: the request for the answers held. With shares it is sent as the
// good's code, country and materials (a product has nowhere to carry a share);
// without, as the product. The case is only read.
export function buildBody(product: Product, a: Answers, s: Shares, country?: string): CanadaBody {
  const facts = buildAnswers(a);
  if (!Object.keys(s).length) return { product, ...facts };
  return {
    good_hts: product.hts,
    country,
    materials: (product.components ?? []).map((m, i) => {
      const out: NonNullable<CanadaBody["materials"]>[number] = { name: m.name, hts: m.hts, origin: m.made_in };
      const v = s[shareKey("value_share", i)];
      const w = s[shareKey("weight_share", i)];
      if (v !== undefined) out.value_share = v;
      if (w !== undefined) out.weight_share = w;
      return out;
    }),
    ...facts,
  };
}

const pct = (v: number | undefined) => (v === undefined ? "" : String(Number((v * 100).toPrecision(12))));

function ShareInput({ item, value, onCommit }: { item: Item; value: number | undefined; onCommit: (v: number | null) => void }) {
  const C = copy.canadaCheck;
  const id = useId();
  const [text, setText] = useState(pct(value));
  const [bad, setBad] = useState(false);
  // What was last sent, so Enter then blur is one request. The box is not
  // remounted on a commit, so focus stays.
  const last = useRef<number | null>(value ?? null);
  const commit = () => {
    const r = parsePercent(text);
    setBad(r === undefined);
    if (r === undefined || r === last.current) return;
    last.current = r;
    onCommit(r);
  };
  return (
    <div data-canada-share={item.key}>
      <label htmlFor={id} className="duo mb-1 block text-[13px] text-ink">
        {item.label}
      </label>
      <div className="flex items-center gap-1.5">
        <input
          id={id}
          inputMode="decimal"
          autoComplete="off"
          value={text}
          aria-invalid={bad}
          onChange={(e) => setText(e.target.value)}
          onBlur={commit}
          onKeyDown={(e) => {
            if (e.key === "Enter") commit();
          }}
          className="duo min-h-11 w-24 rounded-[2px] border border-line bg-sheet px-3 text-[14px] text-ink aria-[invalid=true]:border-ask"
        />
        <span className="duo text-[13px] text-dim">%</span>
      </div>
      <p role={bad ? "alert" : undefined} className="duo min-h-4 text-[12.5px] text-ask">
        {bad ? C.shareBad : ""}
      </p>
    </div>
  );
}

function FactChoice({ item, value, onChange }: { item: Item; value: boolean | undefined; onChange: (v: boolean | null) => void }) {
  const C = copy.canadaCheck;
  const name = useId();
  const opts: [string, boolean | null][] = [
    [C.yes, true],
    [C.no, false],
    [C.notSure, null],
  ];
  const cur = value === undefined ? null : value;
  return (
    <fieldset data-canada-fact={item.key} className="min-w-0 border-0 p-0">
      <legend className="duo mb-1 text-[13px] text-ink">{item.label}</legend>
      <div className="flex flex-wrap gap-1.5">
        {opts.map(([text, v]) => (
          <label
            key={text}
            className="duo inline-flex min-h-11 min-w-16 cursor-pointer items-center justify-center rounded-[2px] border border-line bg-sheet px-3 text-[13px] text-ink has-checked:border-ink has-checked:bg-ink has-checked:text-paper has-focus-visible:outline-2 has-focus-visible:outline-offset-2"
          >
            <input type="radio" name={name} className="sr-only" checked={cur === v} onChange={() => onChange(v)} />
            {text}
          </label>
        ))}
      </div>
    </fieldset>
  );
}

export function CanadaCheckView({
  v,
  parts,
  caseId,
  answers = {},
  shares = {},
  onAnswer,
  onShare,
  onClear,
  epoch = 0,
  busy = false,
}: {
  v: CanadaMarking;
  parts?: string[];
  caseId?: string;
  answers?: Answers;
  shares?: Shares;
  onAnswer?: (key: FactKey, value: boolean | null) => void;
  onShare?: (key: string, material: number, value: number | null) => void;
  onClear?: () => void;
  // Bumped when the answers are reset from outside the boxes.
  epoch?: number;
  busy?: boolean;
}) {
  const C = copy.canadaCheck;
  const r = v.result;
  const head = r.status === "decided" ? C.decided : r.status === "out_of_scope" ? C.outOfScope : C.undetermined;
  const g = v.guidance;
  const missing = g?.missing ?? [];
  const facts = onAnswer ? factItems(g) : [];
  const isShare = (m: Item) => m.kind === "share" && m.material !== undefined && Boolean(onShare);
  const shareItems = missing.filter(isShare);
  const shown = new Set<string>(facts.map((f) => f.key));
  const shownShares = new Set(shareItems.map((m) => shareKey(m.key, m.material!)));
  // Answers with no control now (the result is decided, or the question went
  // away): kept visible and clearable.
  const gone: string[] = [
    ...FACT_KEYS.filter((k) => typeof answers[k] === "boolean" && !shown.has(k)).map((k) => `${C.answerLabels[k]} ${answers[k] ? C.yes.toLowerCase() : C.no.toLowerCase()}`),
    ...Object.keys(shares)
      .filter((k) => !shownShares.has(k))
      .map((k) => {
        const [key, i] = k.split("@");
        return `${C.shareLabels[key] ?? key} (${parts?.[Number(i)]?.trim() || C.part(Number(i) + 1)}) ${pct(shares[k])}%`;
      }),
  ];
  const adds = missing.filter((m) => (m.kind === "code" || m.kind === "share") && !isShare(m));
  const review = missing.some((m) => m.kind === "review");
  const why = [...missing.filter((m) => m.detail && m.kind !== "fact"), ...facts.filter((m) => m.detail)];
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
      {facts.length || shareItems.length || adds.length || review || gone.length || onAnswer ? (
        <div data-canada-guidance={g?.state ?? "answers"} aria-busy={busy} className="mt-2 flex flex-col gap-2">
          {facts.map((f) => (
            <FactChoice key={f.key} item={f} value={answers[f.key as FactKey]} onChange={(val) => onAnswer?.(f.key as FactKey, val)} />
          ))}
          {shareItems.map((m) => {
            const k = shareKey(m.key, m.material!);
            return <ShareInput key={`${k}:${epoch}`} item={m} value={shares[k]} onCommit={(val) => onShare?.(m.key, m.material!, val)} />;
          })}
          {adds.length ? (
            <div data-canada-add className="duo text-[13px] text-body">
              <p>{C.toAdd}</p>
              <ul className="mt-0.5 list-disc pl-4">
                {adds.map((m, i) => (
                  <li key={i}>{itemText(m, parts)}</li>
                ))}
              </ul>
              {caseId ? (
                <Link href={`/cases/new?from=${encodeURIComponent(caseId)}#field-good-hts`} className="inline-flex min-h-11 items-center text-ok underline underline-offset-2">
                  {copy.ruleCheck.addCodesLink}
                </Link>
              ) : null}
            </div>
          ) : null}
          {review ? (
            <p data-canada-review className="duo text-[13px] text-body">
              {C.review}
            </p>
          ) : null}
          {gone.length ? (
            <p data-canada-answers className="duo flex flex-wrap items-center gap-x-2 text-[13px] text-dim">
              <span>{C.yourAnswers}:</span>
              {gone.map((t) => (
                <span key={t}>{t}</span>
              ))}
              <button type="button" onClick={onClear} className="inline-flex min-h-11 items-center text-ok underline underline-offset-2">
                {C.clearAll}
              </button>
            </p>
          ) : null}
          {onAnswer ? (
            <p role="status" className="duo min-h-4 text-[12.5px] text-dim">
              {busy ? C.running : ""}
            </p>
          ) : null}
        </div>
      ) : null}
      <div className="mt-2">
        {why.length || g?.review ? (
          <Fold title={C.why}>
            <ul className="duo flex list-disc flex-col gap-1 pl-4 text-[12.5px] text-dim">
              {why.map((m, i) => (
                <li key={i}>
                  {m.label}: {sentence(m.detail ?? "")}
                </li>
              ))}
              {g?.review ? <li>{sentence(g.review.reason)}</li> : null}
            </ul>
          </Fold>
        ) : null}
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
  const [epoch, setEpoch] = useState(0);
  const ok = Boolean(c.product && canCheckCanada(c));
  // Answers (facts and shares) live here only: they are sent with the request
  // and never stored on the case. They belong to the product's content.
  const pkey = JSON.stringify(c.product ?? null);
  const [held, setHeld] = useState<Held>({ key: pkey, a: {}, s: {} });
  const cur: Held = held.key === pkey ? held : { key: pkey, a: {}, s: {} };
  const curRef = useRef(cur);
  const vRef = useRef(v);
  useEffect(() => {
    curRef.current = cur;
    vRef.current = v;
  });
  const applied = useRef<Held>({ key: pkey, a: {}, s: {} });
  const seq = useRef(0);
  const ctl = useRef<AbortController | null>(null);
  const run = async (h: Held) => {
    ctl.current?.abort();
    const mine = ++seq.current;
    const ac = (ctl.current = new AbortController());
    setBusy(true);
    setErr("");
    try {
      const r = await api.canadaMarking(buildBody(c.product!, h.a, h.s, vRef.current?.country), ac.signal);
      if (mine !== seq.current) return;
      applied.current = h;
      setV(r);
    } catch (e) {
      if (mine !== seq.current) return;
      setErr(errorText(e instanceof Error ? e.message : String(e)));
      // The answers shown are the ones the last result was run for.
      setHeld(applied.current);
      setEpoch((n) => n + 1);
    } finally {
      if (mine === seq.current) setBusy(false);
    }
  };
  const change = (next: Held) => {
    setHeld(next);
    void run(next);
  };
  const answer = (key: FactKey, value: boolean | null) => change({ ...cur, a: setAnswer(cur.a, key, value) });
  const answerShare = (key: string, material: number, value: number | null) => change({ ...cur, s: setShare(cur.s, shareKey(key, material), value) });
  // One request for all of them.
  const clearAll = () => {
    setEpoch((n) => n + 1);
    change({ key: pkey, a: {}, s: {} });
  };
  // A different product (by content) drops what was running.
  useEffect(
    () => () => {
      ctl.current?.abort();
      seq.current++;
      setBusy(false);
      // The old product's result must not seed the new one (its country).
      setV(null);
      setEpoch((n) => n + 1);
    },
    [pkey],
  );
  // On a Canadian case the check runs on load; state is set only after the
  // answer, never synchronously in the effect.
  const product = c.product;
  useEffect(() => {
    if (!auto || !ok || !product) return;
    let live = true;
    const mine = ++seq.current;
    const ac = new AbortController();
    ctl.current?.abort();
    api
      .canadaMarking(buildBody(product, curRef.current.a, curRef.current.s, vRef.current?.country), ac.signal)
      .then((r) => {
        if (!live || mine !== seq.current) return;
        applied.current = curRef.current;
        setV(r);
      })
      .catch((e) => live && mine === seq.current && setErr(errorText(e instanceof Error ? e.message : String(e))));
    return () => {
      live = false;
      ac.abort();
      setBusy(false);
    };
  }, [auto, ok, product]);
  if (!ok) {
    if (!auto) return null;
    return (
      <div data-canada-check="codes" className="duo max-w-[75ch] text-[13px] text-dim">
        <p className="text-body">{C.title}</p>
        <p className="mt-0.5">{C.addCodes}</p>
        <ul data-canada-needs className="mt-0.5 list-disc pl-4">
          {!c.product?.hts?.trim() ? <li>{C.needGood}</li> : null}
          {!c.product?.components?.some((x) => x.hts?.trim()) ? <li>{C.needPart}</li> : null}
        </ul>
        <Link href={`/cases/new?from=${encodeURIComponent(c.id ?? "")}#field-good-hts`} className="mt-1 inline-flex min-h-11 items-center text-ok underline underline-offset-2">
          {copy.ruleCheck.addCodesLink}
        </Link>
      </div>
    );
  }
  if (v)
    return (
      <>
        <CanadaCheckView
          v={v}
          parts={c.product?.components?.map((x) => x.name ?? "")}
          caseId={c.id}
          answers={cur.a}
          shares={cur.s}
          onAnswer={answer}
          onShare={answerShare}
          onClear={clearAll}
          epoch={epoch}
          busy={busy}
        />
        {err ? (
          <p role="alert" className="duo mt-1 text-[13px] text-ask">
            {err}
          </p>
        ) : null}
      </>
    );
  return (
    <div data-canada-check="ask" className="flex flex-wrap items-center gap-3">
      <Button variant="ghost" onClick={() => void run(cur)} disabled={busy || (auto && !err)}>
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
