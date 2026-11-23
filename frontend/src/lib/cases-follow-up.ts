// #715: the cases-list review reading and the one "Needs follow-up" filter.
// Display only: it reads fields the list row already carries, stores nothing
// and changes no case.
import { statusLabel, type Tone } from "./broker-review";
import type { CaseSummary } from "./api/client";

// A link still waiting on the broker counts as needing follow-up when it
// lapses within this many hours.
export const FOLLOW_UP_HOURS = 72;

type Row = Pick<CaseSummary, "review_status" | "review_expires_at" | "ruling_alerts">;

const time = (iso: string | null | undefined) => (iso ? Date.parse(iso) : NaN);

// Changes requested; or a pending or viewed link that ends in the next
// FOLLOW_UP_HOURS (a lapsed one does not); or a final ruling alert. Approved
// and filing-requested links lead to no follow-up here. A proposal alone is
// information, as on the badge.
export function needsFollowUp(c: Row, now = Date.now()): boolean {
  if (c.review_status === "changes_requested") return true;
  if (c.review_status === "pending" || c.review_status === "viewed") {
    const end = time(c.review_expires_at);
    if (Number.isFinite(end) && end > now && end - now <= FOLLOW_UP_HOURS * 3600e3) return true;
  }
  return Boolean(c.ruling_alerts?.final);
}

export const followUpRows = <T extends Row>(rows: T[], now = Date.now()): T[] => rows.filter((c) => needsFollowUp(c, now));

// The row's status words; the expiry only while a link is pending or viewed.
export function reviewRowReading(c: Row): { label: string; tone: Tone; expires: string | undefined } | null {
  if (!c.review_status) return null;
  const waiting = c.review_status === "pending" || c.review_status === "viewed";
  return { ...statusLabel(c.review_status), expires: waiting ? c.review_expires_at : undefined };
}
