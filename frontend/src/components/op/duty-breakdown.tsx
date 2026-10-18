import type { Economics } from "@/lib/api/client";
import { Country, cx } from "./ui";

type Extra = { name?: string; heading?: string; rate?: number };

// 0.125 → "12.5%", 0.25 → "25%".
const pc = (v: number) => `${Math.round(v * 1000) / 10}%`;

// "Section 301 (2026, 91 FR 47318)" → "Section 301 2026";
// "Section 301 China, List 3" → "Section 301 List 3". Full name on hover.
function shortName(n: string): string {
  return n.replace(/^Section 301 \((\d{4}),[^)]*\)$/, "Section 301 $1").replace(/^Section 301 China, /, "Section 301 ");
}

// The Chapter 99 duties in an origin's live rate (D90): the rate is the
// total, so MFN is what's left after the extras. One line per origin; the
// headings in dim text. Nothing shows without duty_additional (the sourced
// table's products, like the heat gun, don't carry it).
export function DutyBreakdown({ e, className }: { e: Economics | null | undefined; className?: string }) {
  const extras = Object.entries(e?.duty_additional ?? {}).filter(([o, xs]) => xs?.length && e?.duty_rates?.[o] !== undefined);
  if (!extras.length) return null;
  return (
    <ul className={cx("flex flex-col gap-1 text-xs", className)} aria-label="Duty rate breakdown">
      {extras.map(([origin, xs]) => {
        const list = [...(xs as Extra[])].sort((a, b) => (b.rate ?? 0) - (a.rate ?? 0));
        const total = e!.duty_rates![origin];
        const mfn = Math.max(0, total - list.reduce((s, x) => s + (x.rate ?? 0), 0));
        return (
          <li key={origin} className="duo tabular-nums">
            <Country name={origin} /> {pc(total)} = MFN {pc(mfn)}
            {list.map((x) => (
              <span key={`${x.heading}-${x.name}`} title={x.name}>
                {" "}
                + {shortName(x.name ?? "Additional duty")} {pc(x.rate ?? 0)}
                {x.heading && <span className="text-dim"> ({x.heading})</span>}
              </span>
            ))}
          </li>
        );
      })}
    </ul>
  );
}
