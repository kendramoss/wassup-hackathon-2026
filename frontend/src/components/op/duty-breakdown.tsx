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
            <Country name={origin} /> {pc(total)} <span className="text-dim">= MFN {pc(mfn)}</span>
            {list.map((x) => (
              // The heading (9903.05.31) is on hover; the line stays short (18 Oct).
              <span key={`${x.heading}-${x.name}`} title={[x.name, x.heading].filter(Boolean).join(" · ")} className="text-dim">
                {" "}
                + {shortName(x.name ?? "Additional duty")} {pc(x.rate ?? 0)}
              </span>
            ))}
          </li>
        );
      })}
    </ul>
  );
}

// The duty note, short (18 Oct): one line on where the rates come from and
// what they leave out, and the full text behind "Sources and caveats".
const DUTY_SOURCES = "docs/DUTY-SOURCES.md";
function NoteText({ text }: { text: string }) {
  // The note names the repo file it comes from; link it rather than show a
  // bare path (#154).
  const [before, ...rest] = text.split(DUTY_SOURCES);
  if (!rest.length) return <>{text}</>;
  return (
    <>
      {before}
      <a href={`https://github.com/kendramoss/wassup-hackathon-2026/blob/main/${DUTY_SOURCES}`} target="_blank" rel="noreferrer" className="text-ok underline underline-offset-2">
        duty sources
      </a>
      {rest.join(DUTY_SOURCES)}
    </>
  );
}

export function DutySources({ note, className }: { note: string | null | undefined; className?: string }) {
  if (!note) return null;
  const live = /live HTS MFN rate for ([\d.]+)/.exec(note)?.[1];
  const summary = live
    ? `Live U.S. tariff rates for HTS ${live}: MFN${/Section 301/.test(note) ? " + Section 301" : ""}.`
    : /sourced|DUTY-SOURCES/i.test(note)
      ? "Sourced duty rates for this HTS number."
      : "Duty rates filled for you.";
  const caveat = /Section 232/.test(note) ? " Section 232 isn't included." : "";
  return (
    <div className={cx("text-xs text-dim", className)}>
      <p className="duo">
        {summary}
        {caveat}
      </p>
      <details className="mt-1">
        <summary className="inline-flex min-h-6 cursor-pointer items-center text-ok">Sources and caveats</summary>
        <p className="duo mt-1">
          <NoteText text={note} />
        </p>
      </details>
    </div>
  );
}
