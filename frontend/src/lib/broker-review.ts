// Broker review links: the pure parts (labels, the link, comment checks, the
// timeline). The screens are components/op/broker-reviews.tsx (owner, on the
// Draft step) and components/op/screens/broker-review-page.tsx (/review#<token>).
// Nothing here touches the network, storage or the console, so the token is
// never logged or kept: it lives in the route param and one request header.
import { formatDate } from "./format";

export type ReviewStatus = "pending" | "viewed" | "approved" | "changes_requested" | "file_requested" | "revoked" | "expired";
export type DecisionKind = "approve" | "changes" | "file_request";
export type ReviewDecision = { decision: DecisionKind; comment?: string; at: string };

export type ReviewSummary = {
  id: string;
  status: ReviewStatus;
  subject: "setup" | "plan";
  draft_label: string;
  format?: "markdown" | "cfr177";
  created_at: string;
  expires_at: string;
  revoked_at?: string;
  first_viewed_at?: string;
  decisions?: ReviewDecision[];
};

export type CreateReviewBody = {
  plan_index?: number;
  move_key?: string;
  target?: string;
  format?: "markdown" | "cfr177";
  expires_in_days?: number;
};
export type CreateReviewResponse = { review: ReviewSummary; token: string; review_path: string };
export type ReviewList = { case_id: string; reviews: ReviewSummary[] };

// What the broker's page reads (GET /v1/review). The product is the domain
// JSON; only its name and the components' name and made_in are relied on.
export type BrokerPacket = {
  review: ReviewSummary;
  product_name: string;
  product?: { name?: string; components?: { name?: string; made_in?: string }[] } | null;
  prediction?: { origin_country?: string; confidence?: number; calibrated_confidence?: number; decisive_step?: string; key_factors?: string[] } | null;
  cited_rulings?: { id: string; url?: string; date?: string; subject?: string; origin?: string }[];
  plan?: { description?: string; predicted_origin?: string; supporting_ruling?: string; counter_ruling?: string; check_confidence?: number; below_bar?: boolean } | null;
  draft?: { format?: "markdown" | "cfr177"; content?: string; notes?: string[] } | null;
  shared_at?: string;
  disclaimer?: string;
};

export const DISCLAIMER = "Not a ruling. Confirm with a licensed customs broker or a CBP binding ruling.";
export const EXPIRY_DAYS = [7, 14, 30] as const;
export const DEFAULT_EXPIRY = 14;
export const COMMENT_MAX = 2000;
export const MAX_ACTIVE = 5;
export const COPY_NOTE = "Copy it now. Unde keeps only a hash of the link, so it can't show it again. Revoke it and make a new one if it's lost.";
export const FROZEN_NOTE = "The link shows this version of the draft.";

export type Tone = "ok" | "ask" | undefined;

const STATUS: Record<ReviewStatus, { label: string; tone: Tone }> = {
  pending: { label: "Not opened yet", tone: undefined },
  viewed: { label: "Opened", tone: undefined },
  approved: { label: "Approved", tone: "ok" },
  changes_requested: { label: "△ Changes requested", tone: "ask" },
  file_requested: { label: "Filing requested", tone: "ok" },
  revoked: { label: "Revoked", tone: undefined },
  expired: { label: "Expired", tone: undefined },
};

export function statusLabel(status: string | null | undefined): { label: string; tone: Tone } {
  return STATUS[status as ReviewStatus] ?? { label: "Unknown", tone: undefined };
}

const time = (iso: string | null | undefined) => {
  const t = iso ? Date.parse(iso) : NaN;
  return Number.isFinite(t) ? t : NaN;
};

// The stored status can lag the clock: a link past its expiry reads as
// expired here even before the server's next answer says so.
export function effectiveStatus(r: Pick<ReviewSummary, "status" | "expires_at" | "revoked_at">, now = Date.now()): ReviewStatus {
  if (r.status === "revoked" || r.revoked_at) return "revoked";
  if (r.status === "expired" || time(r.expires_at) <= now) return "expired";
  return r.status;
}

export const isActive = (r: Pick<ReviewSummary, "status" | "expires_at" | "revoked_at">, now = Date.now()) => {
  const s = effectiveStatus(r, now);
  return s !== "revoked" && s !== "expired";
};

// The case-wide chip: the newest active link, else the newest link. Lists
// come newest first. null when the case has no links.
export function caseReviewChip(reviews: ReviewSummary[] | null | undefined, now = Date.now()): { label: string; tone: Tone; active: number } | null {
  if (!reviews?.length) return null;
  const active = reviews.filter((r) => isActive(r, now));
  const pick = active[0] ?? reviews[0];
  return { ...statusLabel(effectiveStatus(pick, now)), active: active.length };
}

// The full link to hand the broker. Only "/review#<token>" is accepted, so a
// malformed answer never turns into a link to somewhere else.
// The token is the URL fragment (/review#<token>): browsers never send a
// fragment to a server, so it reaches no request log and no Referer header.
// A path that puts anything after /review other than "#token" is refused.
export function reviewLink(origin: string, path: string | null | undefined): string | null {
  if (!path || !/^\/review#[A-Za-z0-9._~-]+$/.test(path)) return null;
  return origin.replace(/\/+$/, "") + path;
}

// tokenFromHash reads the token from location.hash ("#ubr_..."), or null.
export function tokenFromHash(hash: string | null | undefined): string | null {
  const t = (hash ?? "").replace(/^#/, "");
  return /^[A-Za-z0-9._~-]{1,256}$/.test(t) ? t : null;
}

// A token as it may travel in a header: printable ASCII, no spaces.
export const plausibleToken = (t: string | null | undefined): t is string => typeof t === "string" && /^[\x21-\x7e]{1,256}$/.test(t);

// The draft the link freezes: the same choice the Draft step exports.
export function createBody(sel: { move?: string | null; target?: string | null; plan_index?: number | null; format: "markdown" | "cfr177" }, days: number): CreateReviewBody {
  const body: CreateReviewBody = { format: sel.format, expires_in_days: days };
  if (sel.move) {
    body.move_key = sel.move;
    if (sel.target) body.target = sel.target;
  } else if (typeof sel.plan_index === "number") body.plan_index = sel.plan_index;
  return body;
}

// Characters as the backend counts them (code points, not UTF-16 units).
export const commentLength = (s: string) => [...s].length;

export function counter(s: string): { text: string; over: boolean } {
  const n = commentLength(s.trim());
  return { text: `${n} / ${COMMENT_MAX}`, over: n > COMMENT_MAX };
}

export type DecisionCheck = { ok: true; body: { decision: DecisionKind; comment?: string } } | { ok: false; error: string };

export function checkDecision(decision: DecisionKind, comment: string): DecisionCheck {
  const c = comment.trim();
  const n = commentLength(c);
  if (decision === "changes" && n === 0) return { ok: false, error: "Say what should change. A comment is needed to request changes." };
  if (n > COMMENT_MAX) return { ok: false, error: `The comment is ${n} characters. The limit is ${COMMENT_MAX}.` };
  return { ok: true, body: n ? { decision, comment: c } : { decision } };
}

export const decisionLabel: Record<DecisionKind, string> = {
  approve: "Approved",
  changes: "Requested changes",
  file_request: "Asked to file as a binding ruling request",
};

export type TimelineEntry = { at: string; kind: "created" | "viewed" | "decision" | "revoked" | "expired"; label: string; comment?: string; tone: Tone };

// Created, first opened, each decision with its comment, then revoked or
// expired, oldest first. Same-time entries keep that order.
export function timeline(r: ReviewSummary, now = Date.now()): TimelineEntry[] {
  const out: TimelineEntry[] = [{ at: r.created_at, kind: "created", label: `Link made for ${r.draft_label || "the draft"}`, tone: undefined }];
  if (r.first_viewed_at) out.push({ at: r.first_viewed_at, kind: "viewed", label: "First opened by the broker", tone: undefined });
  for (const d of r.decisions ?? []) {
    out.push({
      at: d.at,
      kind: "decision",
      label: decisionLabel[d.decision] ?? d.decision,
      comment: d.comment?.trim() ? d.comment : undefined,
      tone: d.decision === "changes" ? "ask" : "ok",
    });
  }
  const s = effectiveStatus(r, now);
  if (s === "revoked") out.push({ at: r.revoked_at ?? "", kind: "revoked", label: "Link revoked", tone: undefined });
  else if (s === "expired") out.push({ at: r.expires_at, kind: "expired", label: "Link expired", tone: undefined });
  const order = out.map((e, i) => ({ e, i, t: time(e.at) }));
  order.sort((a, b) => (Number.isFinite(a.t) && Number.isFinite(b.t) ? a.t - b.t : 0) || a.i - b.i);
  return order.map((x) => x.e);
}

// The broker page's states for a failed request.
export type BrokerFailure = "not_found" | "expired" | "revoked" | "rate_limited" | "limit" | "error";

export function brokerFailure(e: { status?: number; code?: string } | null | undefined): BrokerFailure {
  if (!e) return "error";
  if (e.code === "link_expired") return "expired";
  if (e.code === "link_revoked") return "revoked";
  if (e.status === 429 || e.code === "rate_limited" || e.code === "auth_rate_limited") return "rate_limited";
  if (e.status === 404 || e.code === "not_found" || e.code === "review_not_found") return "not_found";
  if (e.status === 410) return "expired";
  if (e.status === 409) return "limit";
  return "error";
}

// What the owner reads when making or revoking a link fails, or null to show
// the shared error box (rate limits, sign-in, network).
export function ownerFailure(e: { status?: number; code?: string; message?: string } | null | undefined, action: "create" | "revoke"): string | null {
  if (!e) return null;
  if (e.status === 409) return `This case already has ${MAX_ACTIVE} active links. Revoke one to make another.`;
  if (e.status === 403) return "You can't change this case, so you can't make or revoke review links.";
  if (action === "revoke" && e.status === 404) return "That link no longer exists. The list is up to date now.";
  if (e.status === 404) return "This case was not found. It may have been deleted.";
  if (e.code === "audit_unavailable") return "The audit trail is unavailable, so nothing was changed. Try again in a moment.";
  if (e.status === 400) return e.message ? e.message[0].toUpperCase() + e.message.slice(1) + (/[.!?]$/.test(e.message) ? "" : ".") : "That request wasn't valid.";
  return null;
}

export const dateLabel = (iso: string | null | undefined) => formatDate(iso);

// The links a case response carries (GET /v1/cases/{id}: broker_reviews,
// newest first). The generated CaseResponse type does not list it yet.
export function caseReviews(c: unknown): ReviewSummary[] {
  const r = (c as { broker_reviews?: unknown } | null | undefined)?.broker_reviews;
  return Array.isArray(r) ? (r as ReviewSummary[]) : [];
}

export const formatName = (f: string | null | undefined) => (f === "cfr177" ? "Part 177 letter" : "Short draft");
