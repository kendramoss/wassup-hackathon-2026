// Workspace admin screen (#870): the data handling, kept apart from the
// component so it can be tested. The server decides who is an admin and which
// features are on; nothing here grants access. A 403 or 404 only changes what
// the screen says.
import { ApiError, type ApiKey } from "./api/client";

export type Gate = "ok" | "signin" | "no_teams" | "no_workspace" | "not_admin" | "error";

// What a failed first read of the overview means for the whole screen.
export function overviewGate(e: unknown): Gate {
  if (!(e instanceof ApiError)) return "error";
  if (e.status === 401) return "signin";
  if (e.status === 403) return "not_admin";
  if (e.status === 404) return "no_workspace";
  return "error";
}

// What a failed read of one section means. 404 and 403 are "not on here": the
// feature is off, or this user may not see it. Anything else is a real error.
export type Section = "off" | "error";
export function sectionState(e: unknown): Section {
  return e instanceof ApiError && (e.status === 404 || e.status === 403) ? "off" : "error";
}

export const text = {
  title: "Workspace admin",
  signIn: "Sign in to manage your workspace.",
  noTeams: "Team workspaces are not on here.",
  noWorkspace: "You are not in a workspace.",
  notAdmin: "Only workspace admins can open this page.",
  error: "This page could not load. Nothing was changed.",
  retry: "Try again",
  off: "Not enabled",
  members: "Members",
  keys: "API keys",
  retention: "Retention",
  usage: "Model use",
  audit: "Audit trail",
  webhooks: "Webhooks",
  signInTitle: "Sign-in",
};

// ---- API keys: the secret exists only in the answer that creates the key ----
export type KeyOnce = { secret: string | null; keys: ApiKey[] };

export function keysLoaded(keys: ApiKey[]): KeyOnce {
  return { secret: null, keys };
}
export function keyIssued(s: KeyOnce, out: { key: string; api_key: ApiKey }): KeyOnce {
  return { secret: out.key, keys: [out.api_key, ...s.keys] };
}
export function keyRevoked(s: KeyOnce, id: string): KeyOnce {
  return { ...s, keys: s.keys.map((k) => (k.id === id ? { ...k, disabled: true } : k)) };
}
export function secretDismissed(s: KeyOnce): KeyOnce {
  return { ...s, secret: null };
}
export const MAX_ACTIVE_KEYS = 20;
export const activeKeys = (keys: ApiKey[]) => keys.filter((k) => !k.disabled);

export function keyError(e: unknown): string {
  if (e instanceof ApiError) {
    if (e.status === 409) return `A workspace can have ${MAX_ACTIVE_KEYS} active keys. Revoke one first.`;
    if (e.status === 403) return "Only workspace admins can do this.";
    if (e.status === 404) return "API keys are not enabled.";
    if (e.status === 400) return "Use a name of 1 to 100 characters.";
  }
  return "That did not work. Try again.";
}

// ---- name ----
export const normalizeName = (s: string) => s.replace(/\s+/g, " ").trim();
export const nameValid = (s: string) => {
  const n = normalizeName(s);
  return n.length >= 1 && [...n].length <= 100;
};

// ---- retention ----
export type Limits = { min_case_days?: number; max_case_days?: number; min_audit_days?: number; max_audit_days?: number };
// 0 means "keep for the default"; otherwise a whole number inside the limits.
export function retentionProblem(caseDays: string, auditDays: string, lim: Limits | undefined): string | null {
  const parse = (v: string) => (v.trim() === "" ? NaN : Number(v));
  const c = parse(caseDays);
  const a = parse(auditDays);
  if (!Number.isInteger(c) || !Number.isInteger(a) || c < 0 || a < 0) return "Use whole numbers. 0 keeps the default.";
  const cmin = lim?.min_case_days ?? 7;
  const cmax = lim?.max_case_days ?? 3650;
  if (c !== 0 && (c < cmin || c > cmax)) return `Case days: 0, or ${cmin} to ${cmax}.`;
  const amin = lim?.min_audit_days ?? 7;
  if (a !== 0 && a < amin) return `Audit days: 0, or ${amin} or more.`;
  if (a !== 0 && lim?.max_audit_days !== undefined && a > lim.max_audit_days) return `Audit days: 0, or ${amin} to ${lim.max_audit_days}.`;
  return null;
}
export const retentionBody = (caseDays: string, auditDays: string) => ({ case_days: Number(caseDays), audit_days: Number(auditDays) });
export const daysLabel = (n: number | undefined, zero: string) => (n === undefined ? "–" : n === 0 ? zero : `${n} days`);

// ---- budget ----
export function budgetProblem(amount: string, alert: string): string | null {
  const m = Number(amount);
  if (amount.trim() === "" || !Number.isFinite(m) || m <= 0 || m > 1_000_000) return "Monthly budget: more than 0, up to 1,000,000.";
  const a = alert.trim() === "" ? 0 : Number(alert);
  if (!Number.isInteger(a) || a < 0 || a > 100) return "Alert at: a whole number, 0 to 100.";
  return null;
}
export const usd = (n: number | null | undefined) => (n === null || n === undefined ? "–" : `$${n.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`);

// ---- audit ----
export type AuditFilters = { actor_kind: string; action: string; outcome: string; from: string; to: string; order: string };
export const noFilters: AuditFilters = { actor_kind: "", action: "", outcome: "", from: "", to: "", order: "desc" };

// A query string from the filters that are set. Dates come from date inputs
// (YYYY-MM-DD) and go out as the start and end of that UTC day.
export function auditQuery(f: AuditFilters, cursor?: string, limit = 50): string {
  const q = new URLSearchParams();
  q.set("limit", String(limit));
  if (f.order === "asc") q.set("order", "asc");
  for (const k of ["actor_kind", "action", "outcome"] as const) if (f[k]) q.set(k, f[k]);
  if (/^\d{4}-\d{2}-\d{2}$/.test(f.from)) q.set("from", `${f.from}T00:00:00Z`);
  if (/^\d{4}-\d{2}-\d{2}$/.test(f.to)) q.set("to", `${f.to}T23:59:59Z`);
  if (cursor) q.set("cursor", cursor);
  return `?${q}`;
}

export const ACTORS = ["user", "apikey", "admin", "anonymous", "system"] as const;
export const OUTCOMES = ["allowed", "denied", "failed"] as const;
const ACTOR_LABEL: Record<string, string> = { user: "Member", apikey: "API key", admin: "Operator", anonymous: "Anonymous", system: "System" };
export const actorLabel = (k: string) => ACTOR_LABEL[k] ?? k;

// The closed set of audit actions in plain words. An unknown one is shown as
// the server sent it.
export const ACTION_LABEL: Record<string, string> = {
  "case.create": "Case created",
  "case.update": "Case changed",
  "case.export": "Case exported",
  "case.delete": "Case deleted",
  "case.pin": "Case pinned",
  "case.read": "Case read",
  "workspace.create": "Workspace created",
  "workspace.invite": "Member invited",
  "workspace.join": "Member joined",
  "workspace.role_change": "Role changed",
  "workspace.member_remove": "Member removed",
  "workspace.leave": "Member left",
  "workspace.retention": "Retention changed",
  "workspace.rename": "Workspace renamed",
  "workspace.budget": "Budget changed",
  "audit.purge": "Old events cleared",
  "apikey.issue": "Key created",
  "apikey.list": "Keys listed",
  "apikey.disable": "Key revoked",
  "audit.read": "Trail read",
  "audit.verify": "Trail verified",
  "audit.export": "Trail exported",
  "auth.denied": "Sign-in refused",
  "review.create": "Review link created",
  "review.revoke": "Review link revoked",
  "review.view": "Review link opened",
  "review.decision": "Broker decided",
  "webhook.create": "Webhook added",
  "webhook.update": "Webhook changed",
  "webhook.delete": "Webhook deleted",
  "webhook.rotate_secret": "Webhook secret replaced",
  "webhook.test": "Webhook tested",
  "webhook.read": "Webhooks read",
  "admin.watch_run": "Ruling watch run",
  "admin.webhook_sweep": "Webhook sweep",
};
export const actionLabel = (a: string) => ACTION_LABEL[a] ?? a;
export const ACTIONS = Object.keys(ACTION_LABEL);

export function auditError(e: unknown): string {
  if (e instanceof ApiError && e.status === 400) return "Those filters are not valid.";
  return "The trail could not load. Try again.";
}

// ---- webhooks ----
export type WebhookEndpoint = { id: string; url: string; events: string[]; description: string; enabled: boolean; secret_hint: string; secret?: string };
export const WEBHOOK_EVENT_LABEL: Record<string, string> = {
  "ruling_watch.hit": "Ruling watch change",
  "review.decision": "Broker decision",
  "filing.status": "Filing status",
  "budget.alert": "Budget alert",
  "batch.finished": "Batch finished",
  "eval.drift": "Accuracy drift",
};
export function webhookError(e: unknown): string {
  if (e instanceof ApiError) {
    if (e.status === 400) return "That URL is not allowed. Use a public https address.";
    if (e.status === 409) return "The workspace has the most endpoints it can have.";
    if (e.status === 403) return "Only workspace admins can do this.";
  }
  return "That did not work. Try again.";
}
