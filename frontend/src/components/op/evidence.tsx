// Evidence shared by the Result and Scenarios screens.
import type { PrecedentRef } from "@/lib/api/client";
import { formatDate, shortSubject } from "@/lib/format";
import { Country, RulingLink, cx } from "./ui";

// Earlier rulings from precedents_detail, cited ones first and marked.
export function RulingsList({ rulings, cited = [] }: { rulings: PrecedentRef[]; cited?: string[] }) {
  const isCited = new Set(cited);
  const ordered = [...rulings].sort((a, b) => Number(isCited.has(b.id)) - Number(isCited.has(a.id)));
  return (
    <ul className="text-[13px]">
      {ordered.map((r) => (
        <li key={r.id} className="flex justify-between gap-3 border-t border-line py-2.5 last:border-b">
          <div className="min-w-0">
            {r.subject ? (
              <span className="duo block" title={r.subject}>
                {shortSubject(r.subject)}
              </span>
            ) : null}
            {r.date ? (
              <span className="text-xs text-dim">
                {formatDate(r.date)}
                {r.origin ? (
                  <>
                    {" "}
                    · held <Country name={r.origin} />
                  </>
                ) : null}
              </span>
            ) : null}
          </div>
          <span className="shrink-0 text-right">
            <RulingLink id={r.id} className={cx(isCited.has(r.id) ? "" : "text-ink")} />
            {isCited.has(r.id) && <em className="block text-[10.5px] not-italic uppercase tracking-[0.06em] text-ok">cited</em>}
          </span>
        </li>
      ))}
    </ul>
  );
}

export function Audit({ children }: { children: React.ReactNode }) {
  return <p className="duo text-xs text-dim">{children}</p>;
}
