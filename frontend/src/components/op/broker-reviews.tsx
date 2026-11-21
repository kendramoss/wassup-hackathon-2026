"use client";

import Link from "next/link";
import { useEffect, useId, useRef, useState } from "react";
import { api, ApiError, type CaseResponse } from "@/lib/api/client";
import {
  COPY_NOTE,
  DEFAULT_EXPIRY,
  EXPIRY_DAYS,
  FROZEN_NOTE,
  caseReviewChip,
  caseReviews,
  createBody,
  dateLabel,
  effectiveStatus,
  formatName,
  isActive,
  ownerFailure,
  reviewLink,
  statusLabel,
  timeline,
  type ReviewSummary,
} from "@/lib/broker-review";
import type { CaseCtx } from "./case-shell";
import { ErrorBox } from "./chrome";
import { Button, Chip, Panel, PanelHead, cx } from "./ui";

// Date and time for the timeline, in the reader's own time zone.
export function when(iso: string | null | undefined): string {
  const t = iso ? new Date(iso) : null;
  if (!t || Number.isNaN(t.getTime())) return "";
  return t.toLocaleString(undefined, { day: "numeric", month: "short", year: "numeric", hour: "2-digit", minute: "2-digit" });
}

// A pinned demo is read-only unless the server says otherwise; any explicit
// can_write: false (a viewer) is too. The server still decides (403).
function writeBlock(c: CaseResponse): string | null {
  if (c.can_write === true) return null;
  if (c.pinned) return "This is a pinned demo, so review links can't be made or revoked here. Start a new case from it to send one.";
  if (c.can_write === false) return "Your role in this workspace is viewer, so you can see review links but not make or revoke them.";
  return null;
}

type Props = Pick<CaseCtx, "id" | "c" | "setCase"> & {
  // The draft the Draft step shows and exports: the link freezes this one.
  selection: { move: string | null; target: string | null; format: "markdown" | "cfr177" };
  draftLabel: string;
  ready: boolean;
};

// "Send to broker for review" on the Draft step: make a link, copy it once,
// see each link's status and timeline, revoke a link.
export function BrokerReviews({ id, c, setCase, selection, draftLabel, ready }: Props) {
  const [reviews, setReviews] = useState<ReviewSummary[]>(() => caseReviews(c));
  const [days, setDays] = useState<number>(DEFAULT_EXPIRY);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [action, setAction] = useState<"create" | "revoke">("create");
  const [created, setCreated] = useState<{ link: string; expires: string } | null>(null);
  const blocked = writeBlock(c);
  const uid = useId();

  const apply = (list: ReviewSummary[]) => {
    setReviews(list);
    setCase((d) => (d && d.id === id ? ({ ...d, broker_reviews: list } as CaseResponse) : d));
  };

  // The case already carries its links; read the list once more for fresh
  // statuses. A failed read keeps what the case said.
  useEffect(() => {
    let live = true;
    api.listReviews(id).then(
      (r) => {
        if (live && Array.isArray(r?.reviews)) {
          setReviews(r.reviews);
          setCase((d) => (d && d.id === id ? ({ ...d, broker_reviews: r.reviews } as CaseResponse) : d));
        }
      },
      () => {},
    );
    return () => {
      live = false;
    };
  }, [id, setCase]);

  const create = async () => {
    if (blocked || busy) return;
    setBusy(true);
    setError(null);
    setAction("create");
    setCreated(null);
    try {
      const r = await api.createReview(id, createBody(selection, days));
      const link = reviewLink(window.location.origin, r.review_path);
      if (!link) throw new ApiError("The link came back in a form Unde can't show. Revoke it and make a new one.", 0, "internal", false);
      setCreated({ link, expires: r.review.expires_at });
      apply([r.review, ...reviews.filter((x) => x.id !== r.review.id)]);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const revoke = async (rid: string) => {
    setError(null);
    setAction("revoke");
    try {
      const r = await api.revokeReview(id, rid);
      apply(reviews.map((x) => (x.id === rid ? r.review : x)));
    } catch (e) {
      setError(e);
      if (e instanceof ApiError && e.status === 404) api.listReviews(id).then((r) => Array.isArray(r?.reviews) && apply(r.reviews), () => {});
    }
  };

  const failure = error instanceof ApiError ? ownerFailure(error, action) : null;
  const active = reviews.filter((r) => isActive(r)).length;

  return (
    <Panel id="broker-review" aria-labelledby={`${uid}-head`} className="scroll-mt-40">
      <div id={`${uid}-head`}>
        <PanelHead>Broker review</PanelHead>
      </div>
      <p className="duo text-[13px] text-body">
        Send {draftLabel} to your customs broker as a read-only page. They can approve it, ask for changes or ask you to file it. They don&apos;t need an account.
      </p>
      <p className="duo mt-2 text-xs text-dim">{FROZEN_NOTE}</p>

      {blocked ? (
        <p role="note" className="duo mt-3 border border-dashed border-guide bg-paper px-3 py-2 text-[12.5px] text-dim">
          {blocked}
        </p>
      ) : (
        <div className="mt-4 flex flex-col gap-3">
          <fieldset>
            <legend className="eyebrow">Link expires after</legend>
            <div className="mt-1.5 flex gap-1.5">
              {EXPIRY_DAYS.map((d) => (
                <label
                  key={d}
                  className={cx(
                    "flex min-h-10 flex-1 cursor-pointer items-center justify-center gap-2 border px-2 text-sm",
                    days === d ? "border-ink bg-white" : "border-line bg-white hover:border-edge",
                  )}
                >
                  <input type="radio" name={`${uid}-days`} checked={days === d} onChange={() => setDays(d)} className="accent-[var(--ok)]" />
                  {d} days
                </label>
              ))}
            </div>
          </fieldset>
          <Button onClick={create} disabled={!ready || busy} aria-describedby={`${uid}-draft`}>
            {busy ? "Making the link…" : "Send to broker for review"}
          </Button>
          <p id={`${uid}-draft`} className="text-xs text-dim">
            Shares {draftLabel}, {formatName(selection.format).toLowerCase()}. {active} of 5 active links in use.
          </p>
        </div>
      )}

      {error ? (
        <div className="mt-3">
          {failure ? (
            <p role="alert" className="duo border border-fail bg-fail-wash px-3 py-2 text-[13px] text-fail">
              {failure}
            </p>
          ) : (
            <ErrorBox error={error} onRetry={() => setError(null)} />
          )}
        </div>
      ) : null}

      {created && <CreatedLink link={created.link} expires={created.expires} />}

      {reviews.length > 0 && (
        <div className="mt-5">
          <h3 className="eyebrow">Review links · newest first</h3>
          <ul className="mt-2 flex flex-col">
            {reviews.map((r, i) => (
              <ReviewRow key={r.id} r={r} open={i === 0} canRevoke={!blocked} onRevoke={() => revoke(r.id)} />
            ))}
          </ul>
        </div>
      )}
    </Panel>
  );
}

function CreatedLink({ link, expires }: { link: string; expires: string }) {
  const input = useRef<HTMLInputElement>(null);
  const [said, setSaid] = useState("");
  const uid = useId();
  const copyLink = () => {
    const fallback = () => {
      input.current?.focus();
      input.current?.select();
      setSaid("Couldn't copy. The link is selected: copy it with your keyboard.");
    };
    if (navigator.clipboard?.writeText) navigator.clipboard.writeText(link).then(() => setSaid("Copied."), fallback);
    else fallback();
  };
  return (
    <div className="mt-4 border border-ok bg-ok-wash px-3 py-3">
      <label htmlFor={`${uid}-link`} className="eyebrow block text-ok">
        Review link
      </label>
      <div className="mt-1.5 flex gap-1.5">
        <input
          id={`${uid}-link`}
          ref={input}
          readOnly
          value={link}
          onFocus={(e) => e.currentTarget.select()}
          className="min-w-0 flex-1 rounded-[2px] border border-line bg-white px-2 py-2 font-mono text-[12px]"
        />
        <Button variant="ghost" onClick={copyLink} className="shrink-0 px-3">
          Copy
        </Button>
      </div>
      <p aria-live="polite" className="mt-1 min-h-4 text-xs text-ok">
        {said}
      </p>
      <p className="text-xs">Expires {dateLabel(expires)}.</p>
      <p className="duo mt-1.5 text-xs text-body">{COPY_NOTE}</p>
    </div>
  );
}

function ReviewRow({ r, open, canRevoke, onRevoke }: { r: ReviewSummary; open: boolean; canRevoke: boolean; onRevoke: () => Promise<void> }) {
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const status = effectiveStatus(r);
  const s = statusLabel(status);
  const live = isActive(r);
  const events = timeline(r);
  return (
    <li className="border-b border-line py-3 first:border-t">
      <div className="flex flex-wrap items-center gap-2">
        <Chip tone={s.tone}>{s.label}</Chip>
        <span className="min-w-0 break-words text-[13px] font-medium">{r.draft_label || "Draft"}</span>
      </div>
      <p className="mt-1 text-xs text-dim">
        {formatName(r.format)} · made {dateLabel(r.created_at)} · {live ? "expires" : status === "revoked" ? "was to expire" : "expired"} {dateLabel(r.expires_at)}
      </p>
      <details open={open} className="group mt-2">
        <summary className="flex min-h-8 cursor-pointer list-none items-center gap-2 text-[12.5px] text-ink hover:text-ok [&::-webkit-details-marker]:hidden">
          <span aria-hidden="true" className="inline-block w-3 text-dim transition-transform group-open:rotate-90">
            ▸
          </span>
          Timeline · {events.length} {events.length === 1 ? "event" : "events"}
        </summary>
        <ol className="mt-1 flex flex-col gap-2 border-l border-line pl-3">
          {events.map((e, i) => (
            <li key={i} className="text-[12.5px]">
              <span className="block font-mono text-[11px] text-dim">{when(e.at)}</span>
              <span className={cx(e.tone === "ok" && "text-ok", e.tone === "ask" && "font-medium")}>{e.label}</span>
              {e.comment && <p className="duo mt-1 whitespace-pre-wrap break-words border border-line bg-white px-2.5 py-1.5 text-[12.5px] text-body">{e.comment}</p>}
            </li>
          ))}
        </ol>
      </details>
      {live && canRevoke && (
        <div className="mt-2">
          {confirming ? (
            <div role="group" aria-label="Confirm revoking this link" className="flex flex-wrap items-center gap-2 border border-dashed border-ask-line px-3 py-2">
              <span className="text-[12.5px]">Revoke this link? The broker can no longer open it.</span>
              <Button
                variant="ghost"
                className="min-h-8 px-3 py-1 text-[12.5px]"
                disabled={busy}
                onClick={async () => {
                  setBusy(true);
                  await onRevoke();
                  setBusy(false);
                  setConfirming(false);
                }}
              >
                {busy ? "Revoking…" : "Revoke"}
              </Button>
              <Button variant="ghost" className="min-h-8 px-3 py-1 text-[12.5px]" disabled={busy} onClick={() => setConfirming(false)}>
                Keep it
              </Button>
            </div>
          ) : (
            <Button variant="ghost" className="min-h-8 px-3 py-1 text-[12.5px]" onClick={() => setConfirming(true)}>
              Revoke link
            </Button>
          )}
        </div>
      )}
    </li>
  );
}

// The small status line under the case tabs on every step, when the case has
// a review link: the newest active link's status, linking to the Draft step.
export function BrokerReviewStatus({ id, c }: { id: string; c: CaseResponse }) {
  const chip = caseReviewChip(caseReviews(c));
  if (!chip) return null;
  return (
    <p className="no-print mx-auto flex w-full max-w-[1240px] flex-wrap items-center gap-2 px-5 pt-3 text-[13px] text-dim">
      <span>Broker review</span>
      <Chip tone={chip.tone}>{chip.label}</Chip>
      <Link href={`/cases/${encodeURIComponent(id)}/draft#broker-review`} className="text-ok underline underline-offset-2">
        See the timeline
      </Link>
    </p>
  );
}
