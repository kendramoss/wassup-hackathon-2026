"use client";

import Link from "next/link";
import { useState } from "react";
import { api, type CaseResponse } from "@/lib/api/client";
import { formatDate } from "@/lib/format";
import { VIEWER_REASON, caseReadOnly } from "@/lib/roles";
import { alertTone, changeWords, recheckWords, relied, roleWords, type RulingAlerts } from "@/lib/ruling-alerts";
import { cx } from "./ui";

function CbpLink({ id }: { id: string }) {
  return (
    <a href={`https://rulings.cbp.gov/ruling/${encodeURIComponent(id)}`} target="_blank" rel="noreferrer" className="font-mono text-ok underline decoration-1 underline-offset-2">
      {id}
    </a>
  );
}

// CBP changed, or proposed to change, a ruling this case relies on (D480).
// Shown above every step of the case. The saved answer is never changed:
// Re-check now runs the ruling watch, which predicts again with today's
// rulings and leaves out revoked ones. recorded draws the same box on the
// /examples/revocation-alert page: no page padding, and Re-check now is shown
// disabled, since a recorded example runs nothing.
export function RulingAlertsBanner({ c, setCase, recorded }: { c: CaseResponse; setCase: (c: CaseResponse) => void; recorded?: boolean }) {
  const ra = c.ruling_alerts as RulingAlerts | undefined;
  const tone = alertTone(ra);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  if (!ra || !tone) return null;
  const finals = ra.items.filter((a) => relied(a) && a.status === "final");
  const proposed = ra.items.filter((a) => relied(a) && a.status === "proposed");
  const readOnly = ra.items.filter((a) => !relied(a) && a.status === "final");
  const demo = Boolean(c.pinned);
  const blocked = caseReadOnly(c) || demo;
  const run = async () => {
    setBusy(true);
    setError(null);
    try {
      setCase(await api.watch(c.id!));
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className={recorded ? "" : "no-print mx-auto w-full max-w-[1240px] px-5 pt-3"}>
      <section
        aria-label="Changed rulings"
        className={cx("duo flex flex-col gap-1.5 border p-3 text-[13px]", tone === "attention" ? "border-ask-line bg-ask-wash text-ask" : "border-line bg-sheet text-body")}
      >
        <p className="eyebrow">{tone === "attention" ? "A ruling this case relies on has changed" : "CBP rulings changed since this case"}</p>
        {finals.map((a) => (
          <p key={a.ruling} className="max-w-[90ch]">
            Ruling <CbpLink id={a.ruling} /> {roleWords(a.roles)} {changeWords(a, formatDate)}
            {a.bulletin ? ` (${a.bulletin})` : ""}.{" "}
            {a.before_case ? "That is before this case's date: this answer was saved before Unde stopped citing such rulings. " : ""}
            {a.relevance_note ? <span data-relevance={a.relevance}>{a.relevance_note} </span> : null}
            <b className="font-medium">{recheckWords(a)}</b>
          </p>
        ))}
        {proposed.map((a) => (
          <p key={a.ruling} className="max-w-[90ch] text-dim">
            Ruling <CbpLink id={a.ruling} /> {roleWords(a.roles)} {changeWords(a, formatDate)} ({a.bulletin}). Proposed, not final: it is shown, not applied.
          </p>
        ))}
        {readOnly.length ? (
          <p className="max-w-[90ch] text-dim">
            Read but not cited:{" "}
            {readOnly.map((a, i) => (
              <span key={a.ruling}>
                {i > 0 ? "; " : ""}
                <CbpLink id={a.ruling} /> {changeWords(a, formatDate)}
              </span>
            ))}
            .
          </p>
        ) : null}
        {finals.length ? (
          <p className="text-dim">
            Unde never changes a saved answer on its own.{" "}
            {ra.rechecked_at ? (
              <>
                Re-checked on {formatDate(ra.rechecked_at)}: see the ruling watch on the{" "}
                <Link href={`/cases/${encodeURIComponent(c.id!)}/origin`} className="text-ok underline underline-offset-2">
                  Origin step
                </Link>
                .{" "}
              </>
            ) : null}
            {recorded ? (
              <>
                <button type="button" disabled aria-describedby="recheck-recorded" className="inline-flex min-h-6 cursor-not-allowed items-center border border-line px-1.5 text-dim">
                  Re-check now
                </button>{" "}
                <span id="recheck-recorded">(recorded example: nothing is run here)</span>
              </>
            ) : demo ? (
              "This demo can't be re-checked; start a new case from the demo to run one."
            ) : (
              <button
                type="button"
                className="inline-flex min-h-6 items-center text-ok underline underline-offset-2 disabled:text-dim"
                onClick={run}
                disabled={busy || blocked}
                title={blocked ? VIEWER_REASON : undefined}
              >
                {busy ? "Re-checking…" : ra.rechecked_at ? "Re-check again" : "Re-check now"}
              </button>
            )}
          </p>
        ) : null}
        {error ? <p className="text-fail">Re-check failed: {(error as Error).message}</p> : null}
      </section>
    </div>
  );
}
