// Published CBSA rulings related to a Canada case (D923). Display only:
// they play no part in the rules-check answer.
import type { Prediction } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { formatDate } from "@/lib/format";
import { Fold } from "./ui";

type CanadaRuling = NonNullable<Prediction["canada_rulings"]>[number];

export function CbsaRulings({ rulings }: { rulings?: CanadaRuling[] | null }) {
  if (!rulings?.length) return null;
  const C = copy.cbsaRulings;
  return (
    <div className="max-w-[75ch]" data-cbsa-rulings={rulings.length}>
      <Fold title={C.title} meta={C.meta(rulings.length)} open>
        <ul className="text-[13px]">
          {rulings.map((r) => (
            <li key={r.id} className="flex justify-between gap-3 border-t border-line py-2.5 last:border-b">
              <div className="min-w-0">
                <span className="duo block">{r.holding}</span>
                <span className="text-xs text-dim">
                  {r.product} · {formatDate(r.date)}
                </span>
              </div>
              <a
                href={r.url}
                target="_blank"
                rel="noreferrer"
                className="shrink-0 self-start font-mono text-ok underline decoration-1 underline-offset-2"
              >
                {r.id}
              </a>
            </li>
          ))}
        </ul>
      </Fold>
    </div>
  );
}
