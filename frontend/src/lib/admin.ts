// Workspace admin screen (#870): the data handling, kept apart from the
// component so it can be tested. The server decides who is an admin and which
// features are on; nothing here grants access. A 403 or 404 only changes what
// the screen says.
import { ApiError, type ApiKey, type KeyRole, type ScimStatus, type SsoConnection, type SsoRequest, type WorkspaceRole } from "./api/client";

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

// What a key may do with the workspace's cases (D961). The admin picks one when
// the key is made; there is no admin key. A key from before roles has none and
// reaches only its own cases.
export const KEY_ROLES: readonly KeyRole[] = ["viewer", "editor"];
export const keyRoleLabel = (r: string | undefined) => (r === "editor" ? "Editor" : r === "viewer" ? "Viewer" : "Own cases only");
export const keyRoleHelp: Record<KeyRole, string> = {
  viewer: "Reads the workspace's cases.",
  editor: "Reads, creates and changes the workspace's cases.",
};
export const activeKeys = (keys: ApiKey[]) => keys.filter((k) => !k.disabled);

export function keyError(e: unknown): string {
  if (e instanceof ApiError) {
    if (e.status === 409) return `A workspace can have ${MAX_ACTIVE_KEYS} active keys. Revoke one first.`;
    if (e.status === 403) return "Only workspace admins can do this.";
    if (e.status === 404) return "API keys are not enabled.";
    if (e.status === 400) return "Use a name of 1 to 100 characters and a role of viewer or editor.";
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

// ---- member roles ----
export type Member = { email: string; role: string };
export const countAdmins = (ms: Member[]) => ms.filter((m) => m.role === "admin").length;
// The only admin cannot be changed. The server refuses it too (409); this only
// saves the round trip and says why.
export const isLastAdmin = (ms: Member[], email: string) => countAdmins(ms) === 1 && ms.some((m) => m.email === email && m.role === "admin");
export const LAST_ADMIN_NOTE = "The only admin. Make someone else an admin first.";
export function withRole(ms: Member[], email: string, role: WorkspaceRole, fromServer?: Record<string, string>): Member[] {
  return ms.map((m) => ({ ...m, role: fromServer?.[m.email] ?? (m.email === email ? role : m.role) }));
}

// ---- SSO and SCIM setup (#868, #869) ----
// Only ever shown after a save: whether a secret exists, never the secret.
export const SECRET_NOTE = "Secret saved. Leave empty to keep it.";
export const SSO_ROLES = ["viewer", "editor"] as const;

export type SsoForm = {
  protocol: "oidc" | "saml";
  domains: string;
  default_role: string;
  oidc_issuer: string;
  oidc_client_id: string;
  oidc_client_secret: string;
  saml_sso_url: string;
  saml_issuer: string;
  saml_certs: string;
};
export const emptySsoForm: SsoForm = { protocol: "oidc", domains: "", default_role: "viewer", oidc_issuer: "", oidc_client_id: "", oidc_client_secret: "", saml_sso_url: "", saml_issuer: "", saml_certs: "" };

// Edit starts from what the server holds. The client secret is not among it.
export function ssoFormOf(c: SsoConnection): SsoForm {
  return {
    protocol: c.protocol,
    domains: c.domains.map((d) => d.domain).join(", "),
    default_role: c.default_role,
    oidc_issuer: c.oidc_issuer ?? "",
    oidc_client_id: c.oidc_client_id ?? "",
    oidc_client_secret: "",
    saml_sso_url: c.saml_sso_url ?? "",
    saml_issuer: c.saml_issuer ?? "",
    saml_certs: (c.saml_certs_pem ?? []).join("\n"),
  };
}

export function parseDomains(s: string): string[] {
  const out: string[] = [];
  for (const d of s.toLowerCase().split(/[\s,;]+/)) if (d && !out.includes(d)) out.push(d);
  return out;
}
export function splitCerts(s: string): string[] {
  return s.match(/-----BEGIN CERTIFICATE-----[\s\S]*?-----END CERTIFICATE-----/g)?.map((c) => c.trim()) ?? [];
}
const https = (v: string) => /^https:\/\/[^\s/]+/i.test(v.trim());

export function ssoProblem(f: SsoForm): string | null {
  const ds = parseDomains(f.domains);
  if (ds.length < 1 || ds.length > 20) return "Add 1 to 20 email domains.";
  if (f.protocol === "oidc") {
    if (!https(f.oidc_issuer)) return "The issuer is an https address.";
    if (!f.oidc_client_id.trim()) return "Add the client ID.";
  } else {
    if (!https(f.saml_sso_url)) return "The sign-in URL is an https address.";
    if (!f.saml_issuer.trim()) return "Add the identity provider's entity ID.";
    const n = splitCerts(f.saml_certs).length;
    if (n < 1 || n > 3) return "Add 1 to 3 signing certificates (PEM).";
  }
  return null;
}

// The request for a save. A blank client secret is left out, which keeps the
// stored one; enabled and enforced are never part of an edit (the server keeps
// them), and a new connection always starts off.
export function ssoBody(f: SsoForm): SsoRequest {
  const body: SsoRequest = { protocol: f.protocol, domains: parseDomains(f.domains), default_role: f.default_role as SsoRequest["default_role"] };
  if (f.protocol === "oidc") {
    body.oidc_issuer = f.oidc_issuer.trim();
    body.oidc_client_id = f.oidc_client_id.trim();
    if (f.oidc_client_secret !== "") body.oidc_client_secret = f.oidc_client_secret;
  } else {
    body.saml_sso_url = f.saml_sso_url.trim();
    body.saml_issuer = f.saml_issuer.trim();
    body.saml_certs_pem = splitCerts(f.saml_certs);
  }
  return body;
}

// A change of the enforced switch is a full replace with the stored settings.
export const ssoEnforceBody = (c: SsoConnection, enforced: boolean): SsoRequest => ({ ...ssoBody(ssoFormOf(c)), enforced });
export const ssoDisableBody = (c: SsoConnection): SsoRequest => ({ ...ssoBody(ssoFormOf(c)), enabled: false });

export const dnsLine = (d: { record_name: string; record_value: string }) => `${d.record_name}  TXT  ${d.record_value}`;
export const verifiedDomains = (cs: { enabled?: boolean; domains: { domain: string; verified: boolean }[] }[]) => [...new Set(cs.flatMap((c) => c.domains.filter((d) => d.verified).map((d) => d.domain)))].sort();

export function ssoError(e: unknown): string {
  if (e instanceof ApiError) {
    if (e.status === 409) {
      if (e.code === ("sso_domain_taken" as typeof e.code)) return "Another workspace has verified one of these domains.";
      if (e.code === ("sso_limit" as typeof e.code)) return "A workspace can have 5 connections.";
      if (e.code === ("sso_unverified" as typeof e.code)) return "Verify every domain before turning it on.";
    }
    if (e.status === 400) return e.message ? e.message[0].toUpperCase() + e.message.slice(1).replace(/\.?$/, ".") : "Check the settings and try again.";
    if (e.status === 403) return "Only workspace admins can do this.";
    if (e.status === 404) return "That connection is gone or SSO is not enabled.";
  }
  return "That did not work. Try again.";
}

export function scimError(e: unknown): string {
  if (e instanceof ApiError) {
    if (e.status === 409) return "Verify a domain under SSO first.";
    if (e.status === 400) return e.message ? e.message[0].toUpperCase() + e.message.slice(1).replace(/\.?$/, ".") : "Check the settings and try again.";
    if (e.status === 403) return "Only workspace admins can do this.";
    if (e.status === 404) return "SCIM is not enabled.";
  }
  return "That did not work. Try again.";
}

// A SCIM token is in the answer that issues it and nowhere else.
export type ScimOnce = { status: ScimStatus | null; token: string | null };
export const scimLoaded = (status: ScimStatus): ScimOnce => ({ status: { ...status, token: undefined }, token: null });
export const scimIssued = (status: ScimStatus): ScimOnce => ({ status: { ...status, token: undefined }, token: status.token ?? null });
export const scimDismissed = (s: ScimOnce): ScimOnce => ({ ...s, token: null });
