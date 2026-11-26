"use client";

import { useEffect, useState } from "react";
import { api } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { defaultSupported, marketOf, markets, type MarketCode } from "@/lib/market";
import { Flag, cx } from "./ui";

// The market a new case is for (D680). A native select, so it works with a
// keyboard and the phone's own picker; the chosen market's flag sits inside
// the box. Markets with no origin engine yet are listed but disabled.
export function MarketPicker({ id, value, onChange, className }: { id: string; value: MarketCode; onChange: (m: MarketCode) => void; className?: string }) {
  const supported = useSupportedMarkets();
  const m = marketOf(value);
  return (
    <div className={className}>
      <div className="relative">
        <span className="pointer-events-none absolute left-3.5 top-1/2 -translate-y-1/2">
          {m.flag ? <Flag country={m.flag} size={12} /> : <span aria-hidden className="inline-block h-3 w-4 rounded-[2px] border border-line" />}
        </span>
        <select
          id={id}
          data-field="market"
          value={value}
          onChange={(e) => onChange(e.target.value as MarketCode)}
          className="h-11 w-full appearance-none rounded-[2px] border border-edge bg-white pl-10 pr-8 text-[13.5px] text-ink focus:border-ok focus:shadow-[3px_3px_0_var(--shadow)] focus:outline-2 focus:outline-offset-2 focus:outline-accent"
        >
          {markets
            .filter((x) => supported.includes(x.code))
            .map((x) => (
              <option key={x.code} value={x.code}>
                {x.name}
              </option>
            ))}
        </select>
        <span aria-hidden className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-[11px] text-dim">
          ▾
        </span>
      </div>
      <ul data-market-summary className="duo mt-1.5 flex flex-wrap gap-x-3 gap-y-0.5 text-[12.5px] text-dim">
        {copy.market.summary
          .filter((line) => !line.startsWith("Japan:") || supported.includes("jp"))
          .map((line) => (
            <li key={line}>{line}</li>
          ))}
      </ul>
    </div>
  );
}

// The supported markets, from the API; the United States until it answers.
export function useSupportedMarkets(): MarketCode[] {
  const [codes, setCodes] = useState<MarketCode[]>(defaultSupported);
  useEffect(() => {
    let live = true;
    api.jurisdictions().then(
      (r) => {
        const on = (r.jurisdictions ?? []).filter((j) => j.supported && j.code).map((j) => j.code as MarketCode);
        if (live && on.length) setCodes(on);
      },
      () => {},
    );
    return () => {
      live = false;
    };
  }, []);
  return codes;
}

// MarketTag names a case's market in a few words: a flag and a short name.
export function MarketTag({ code, className }: { code: string | null | undefined; className?: string }) {
  const m = marketOf(code);
  return (
    <span className={cx("inline-flex items-center gap-1.5", className)}>
      {m.flag ? <Flag country={m.flag} size={10} /> : null}
      {m.short}
    </span>
  );
}
