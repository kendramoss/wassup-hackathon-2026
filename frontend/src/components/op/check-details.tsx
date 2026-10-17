import type { Plan } from "@/lib/api/client";
import { RulingLink, cx } from "./ui";

type Verdict = NonNullable<Plan["verdict"]>;

// "2/3" → "3 views, 2 agree" (D83). The panel runs only when the first
// check lands near the bar, so most verdicts have no agreement.
export function agreementWords(a: string | undefined): string | null {
  const m = /^(\d+)\s*\/\s*(\d+)$/.exec(a ?? "");
  if (!m) return null;
  return `${m[2]} views, ${m[1]} agree`;
}

export function Agreement({ verdict, className }: { verdict: Verdict | undefined; className?: string }) {
  const words = agreementWords(verdict?.agreement);
  if (!words) return null;
  return (
    <span
      className={cx("text-xs text-dim", className)}
      title="The first check was close to the bar, so a skeptic view and a same-product view checked the move again."
    >
      {words}
    </span>
  );
}

// The precedent check's own search path (D80), in the order it ran: each
// search with the rulings it returned and each ruling read, as they
// interleave (read N345899, then search for a counter-ruling). Verdicts
// cached before D80 have no trail, and then nothing shows.
export function CheckTrail({ verdict }: { verdict: Verdict | undefined }) {
  const trail = verdict?.trail ?? [];
  if (!trail.length) return null;
  return (
    <details className="group mt-1.5">
      <summary className="inline-flex min-h-6 cursor-pointer list-none items-center gap-1.5 text-xs text-ok hover:underline [&::-webkit-details-marker]:hidden">
        <span aria-hidden="true" className="inline-block w-2.5 transition-transform group-open:rotate-90">
          ▸
        </span>
        How the check found this
      </summary>
      <ol className="mt-1.5 flex flex-col gap-1.5 border-l border-line pl-3 text-xs text-body">
        {trail.map((t, i) =>
          t.tool === "read" ? (
            <li key={i}>
              <span className="text-dim">Read</span> {t.rulings?.length ? <Rulings ids={t.rulings} /> : <span className="text-dim">{t.query} (not found)</span>}
            </li>
          ) : (
            <li key={i}>
              <span className="text-dim">Searched</span> “{t.query}”<span className="text-dim"> → </span>
              {t.rulings?.length ? <Rulings ids={t.rulings} /> : <span className="text-dim">no rulings</span>}
            </li>
          ),
        )}
      </ol>
    </details>
  );
}

function Rulings({ ids }: { ids: string[] }) {
  return (
    <>
      {ids.map((id, i) => (
        <span key={id}>
          {i > 0 && ", "}
          <RulingLink id={id} />
        </span>
      ))}
    </>
  );
}

// Near-miss advice (D81): for a move just under the bar, what differs from
// the ruling that nearly supports it, and what would match it. Advice only:
// it never changes the check, the bar or whether the move is shown.
export function NearMiss({ plan, className }: { plan: Plan; className?: string }) {
  const n = plan.near_miss;
  if (!n?.difference && !n?.would_match) return null;
  const ruling = n.ruling ?? plan.verdict?.supporting_ruling;
  return (
    <div className={cx("border border-dashed border-ask-line bg-ask-wash px-3.5 py-2.5 text-[13px] text-body", className)}>
      <p className="text-[11px] uppercase tracking-[0.06em] text-ask">Advice · doesn&apos;t change the check</p>
      {n.difference && (
        <p className="duo mt-1">
          <span className="text-ask">Why it fell short:</span> {n.difference}
        </p>
      )}
      {n.would_match && (
        <p className="duo mt-1">
          <span className="text-ask">Would match {ruling ? <RulingLink id={ruling} /> : "the ruling"} if:</span> {n.would_match}
        </p>
      )}
    </div>
  );
}
