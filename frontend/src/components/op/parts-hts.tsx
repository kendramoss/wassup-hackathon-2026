"use client";

import { useEffect, useState } from "react";
import { copy } from "@/lib/copy";
import { ApiError, api, type Component, type PartHtsCandidate, type Product } from "@/lib/api/client";
import { Button, Chip, Spinner, cx } from "./ui";

// Per-part HTS suggestions (D380). The 19 CFR 102.20 rule check needs a code
// for the product and every part; these are offered next to each part and
// filled in only when the user accepts one. The route exists unless the
// server turns it off, so an empty request is a 400 when on and a 404 when
// off; it runs no model call. Asked once per page load.
let available: Promise<boolean> | null = null;
function partsSuggestAvailable(): Promise<boolean> {
  available ??= api.partsHtsSuggest({ product: { name: "", plants: [], components: [], operations: [] } }).then(
    () => true,
    (e) => !(e instanceof ApiError && e.status === 404),
  );
  return available;
}

const MAX_PARTS = 20;
const rulingUrl = (id: string) => `https://rulings.cbp.gov/ruling/${encodeURIComponent(id)}`;

// What came back, keyed by the part's position in the table. A suggestion is
// shown only while that row still has the same name, so editing or removing
// parts never moves a code onto the wrong part.
export type PartsSuggestions = {
  parts: Map<number, { name: string; candidates: PartHtsCandidate[] }>;
  good: PartHtsCandidate[] | null;
  reasonsDown: boolean;
  capped: boolean;
};

export function suggestionFor(s: PartsSuggestions | null, i: number, c: Component): PartHtsCandidate[] | null {
  const hit = s?.parts.get(i);
  return hit && hit.name === c.name.trim() ? hit.candidates : null;
}

// withSuggested puts an accepted code on a part, with the part's other
// suggested codes as hts_alternatives: the rule check then says whether its
// answer is the same under each (D430).
export function withSuggested(c: Component, code: string, cands: PartHtsCandidate[]): Component {
  const others = cands.map((x) => x.hts ?? "").filter((h) => h && h !== code).slice(0, 2);
  return { ...c, hts: code, hts_alternatives: others.length ? others : undefined };
}

// acceptAll fills the top suggestion into every part that has none yet;
// codes the user typed are kept. A code the server marks "check this code"
// (D471) is passed over for the next one; a part whose every code is marked
// is left for the user.
export function acceptAll(product: Product, s: PartsSuggestions): { product: Product; filled: number } {
  let filled = 0;
  const components = product.components.map((c, i) => {
    const cands = suggestionFor(s, i, c);
    const top = cands?.find((x) => !x.check);
    if (!top?.hts || c.hts?.trim()) return c;
    filled++;
    return withSuggested(c, top.hts, cands ?? []);
  });
  return { product: { ...product, components }, filled };
}

export function PartsHtsBar({
  product,
  name,
  goodHts,
  onGoodHts,
  onChange,
  value,
  setValue,
}: {
  product: Product;
  name: string;
  goodHts: string;
  onGoodHts?: (hts: string) => void;
  onChange: (p: Product) => void;
  value: PartsSuggestions | null;
  setValue: (s: PartsSuggestions | null) => void;
}) {
  const P = copy.partsHts;
  const [on, setOn] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [status, setStatus] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    partsSuggestAvailable().then((ok) => live && setOn(ok));
    return () => {
      live = false;
    };
  }, []);

  const named = product.components.map((c, i) => ({ c, i })).filter(({ c }) => c.name.trim());
  const missing = named.filter(({ c }) => !c.hts?.trim());
  if (!on || named.length === 0) return null;

  const run = async () => {
    const ask = missing.slice(0, MAX_PARTS);
    if (ask.length === 0) {
      setStatus(P.noneMissing);
      return;
    }
    setBusy(true);
    setError(null);
    setStatus(null);
    try {
      const res = await api.partsHtsSuggest({
        product: {
          name: name.trim() || product.name,
          family: product.family || undefined, // the plausibility check reads it (D471)
          description: product.description,
          hts: goodHts.trim() || undefined,
          plants: [],
          operations: [],
          components: ask.map(({ c }) => ({ name: c.name.trim(), made_in: c.made_in })),
        },
      });
      const parts: PartsSuggestions["parts"] = new Map();
      for (const p of res.parts ?? []) {
        const at = ask[p.index ?? -1];
        if (at) parts.set(at.i, { name: at.c.name.trim(), candidates: p.candidates ?? [] });
      }
      setValue({ parts, good: res.good?.candidates ?? null, reasonsDown: !!res.reasons_unavailable, capped: missing.length > MAX_PARTS });
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) setOn(false);
      setError(e instanceof ApiError && e.status === 429 ? P.busyLimit : P.failed);
    } finally {
      setBusy(false);
    }
  };

  const goodTop = value?.good?.[0];
  const any = !!value && ([...value.parts.values()].some((p) => p.candidates.length > 0) || !!goodTop);
  const takeAll = () => {
    if (!value) return;
    const r = acceptAll(product, value);
    let n = r.filled;
    if (goodTop?.hts && !goodHts.trim() && onGoodHts) {
      onGoodHts(goodTop.hts);
      n++;
    }
    onChange(r.product);
    setStatus(P.accepted(n));
  };

  return (
    <div className="mb-3" data-parts-hts>
      <div className="flex flex-wrap items-center gap-2.5">
        <Button variant="ghost" onClick={busy ? undefined : run} disabled={busy}>
          {value ? P.again : P.suggest}
        </Button>
        {any && (
          <Button variant="ghost" onClick={takeAll}>
            {P.acceptAll}
          </Button>
        )}
        {!value && <span className="text-[12.5px] text-dim">{P.why}</span>}
      </div>
      {busy && (
        <div className="mt-3">
          <Spinner label={P.busy} />
        </div>
      )}
      {error && (
        <p role="alert" className="mt-2 text-xs text-fail">
          {error}
        </p>
      )}
      <p aria-live="polite" className="mt-2 text-[12.5px] text-body empty:hidden">
        {status}
      </p>
      {value && !busy && (
        <div className="mt-2 flex flex-col gap-1.5">
          <p className="text-xs text-ask">{P.unverified}</p>
          {value.capped && <p className="text-xs text-dim">{P.tooMany(MAX_PARTS)}</p>}
          {value.reasonsDown && <p className="text-xs text-dim">{P.reasonsDown}</p>}
          {value.good && onGoodHts && (
            <div className="border border-line bg-sheet px-3 py-2">
              <p className="text-[12.5px] text-body">{P.product}: {name.trim() || product.name}</p>
              <Candidates cands={value.good} value={goodHts} onUse={onGoodHts} label={name.trim() || product.name} />
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// The suggestions for one part, as a table row under it.
export function PartSuggestionRow({ cands, value, onUse, label, colSpan }: { cands: PartHtsCandidate[]; value: string; onUse: (hts: string) => void; label: string; colSpan: number }) {
  return (
    <tr data-part-suggestions>
      <td colSpan={colSpan} className="pb-2.5 pl-3 pr-2">
        <div className="border-l-2 border-line pl-3">
          <Candidates cands={cands} value={value} onUse={onUse} label={label} />
        </div>
      </td>
    </tr>
  );
}

function Candidates({ cands, value, onUse, label }: { cands: PartHtsCandidate[]; value: string; onUse: (hts: string) => void; label: string }) {
  const P = copy.partsHts;
  if (cands.length === 0) return <p className="text-[12.5px] text-dim">{P.none}</p>;
  return (
    <ul className="flex flex-col gap-1.5" aria-label={`Suggested codes for ${label}`}>
      {cands.map((c) => {
        const using = value.trim() === c.hts;
        const rulings = c.source === "rulings";
        return (
          <li key={c.hts} className="flex flex-wrap items-baseline gap-x-2.5 gap-y-1 text-[12.5px]">
            <button
              type="button"
              aria-pressed={using}
              aria-label={`${using ? P.using : P.use} ${c.hts} for ${label}`}
              onClick={() => onUse(c.hts ?? "")}
              className={cx(
                "min-h-8 shrink-0 rounded-[2px] border px-2 tabular-nums",
                using ? "border-ok bg-ok-wash text-ok" : "border-line bg-white text-ink hover:border-ok",
              )}
            >
              {using ? "✓ " : ""}
              {c.hts}
            </button>
            <Chip tone={rulings ? "ok" : "ask"}>{rulings ? P.fromRulings : P.fromModel}</Chip>
            {c.check && <Chip tone="fail">{P.checkCode}</Chip>}
            <span className="duo min-w-0 basis-60 flex-1 text-body">
              {c.reason}
              {rulings && c.supporting_rulings?.length ? (
                <span className="text-dim">
                  {" "}
                  (
                  {c.supporting_rulings.map((r, i) => (
                    <span key={r.id}>
                      {i > 0 && ", "}
                      <a href={r.url || rulingUrl(r.id ?? "")} target="_blank" rel="noreferrer" className="text-ok underline underline-offset-2" title={r.sentence ?? r.subject}>
                        {r.id}
                      </a>
                    </span>
                  ))}
                  )
                </span>
              ) : null}
              {c.check && <span className="block text-fail">{c.check}</span>}
            </span>
          </li>
        );
      })}
    </ul>
  );
}
