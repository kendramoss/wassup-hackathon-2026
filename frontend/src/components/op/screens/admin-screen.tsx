"use client";

import Link from "next/link";
import { useCallback, useEffect, useId, useState, type FormEvent } from "react";
import { ApiError, api, errorText, type AdminOverview, type ApiKey, type KeyRole, type WorkspaceRole, type AuditEvent, type RetentionPolicy, type WorkspaceUsage, type WorkspacesMine } from "@/lib/api/client";
import {
  ACTIONS,
  ACTORS,
  OUTCOMES,
  MAX_ACTIVE_KEYS,
  WEBHOOK_EVENT_LABEL,
  actionLabel,
  activeKeys,
  actorLabel,
  auditError,
  auditQuery,
  budgetProblem,
  daysLabel,
  KEY_ROLES,
  LAST_ADMIN_NOTE,
  isLastAdmin,
  keyError,
  keyRoleHelp,
  keyRoleLabel,
  verifiedDomains,
  withRole,
  keyIssued,
  keyRevoked,
  keysLoaded,
  nameValid,
  noFilters,
  normalizeName,
  overviewGate,
  retentionBody,
  retentionProblem,
  secretDismissed,
  sectionState,
  text,
  usd,
  webhookError,
  type AuditFilters,
  type Gate,
  type KeyOnce,
  type WebhookEndpoint,
} from "@/lib/admin";
import { formatDate } from "@/lib/format";
import { ROLES, memberMessage, roleLabel } from "@/lib/roles";
import { SignIn } from "../sign-in";
import { ScimPart, SsoPart } from "./admin-signin";
import { Button, Chip, Panel, PanelHead, Spinner } from "../ui";

const field = "h-10 min-w-0 rounded-[2px] border border-edge bg-white px-3 text-[13.5px] focus:border-ok";
const select = "h-10 rounded-[2px] border border-edge bg-white px-2 text-[13px]";
const th = "py-1.5 pr-4 text-left text-[11px] font-medium uppercase tracking-[0.08em] text-dim";
const td = "py-1.5 pr-4 align-top text-[13px]";

type Ov = {
  workspace: { id: string; name: string; created_at?: string; member_count?: number };
  members: { email: string; role: string }[];
  invited: string[];
  features: Record<string, boolean>;
  sso?: { connections: { id: string; protocol: string; enabled: boolean; enforced: boolean; domains: { domain: string; verified: boolean }[] }[] };
  scim?: { configured: boolean; hint?: string; domains?: string[]; default_role?: string; allow_admin?: boolean };
  retention?: { case_days: number; audit_days: number };
};

// Plain elements, not components, so the text is part of the section's own tree.
const off = <p className="text-[13px] text-dim">{text.off}</p>;

const failed = (onRetry: () => void) => (
  <div className="flex flex-wrap items-center gap-2">
    <p className="text-[13px] text-fail">{text.error}</p>
    <Button variant="ghost" onClick={onRetry}>
      {text.retry}
    </Button>
  </div>
);

export function NameSection({ id, name, onRenamed }: { id: string; name: string; onRenamed: (n: string) => void }) {
  const [value, setValue] = useState(name);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const changed = normalizeName(value) !== name;
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (!nameValid(value) || !changed || busy) return;
    setBusy(true);
    setError(null);
    setSaved(false);
    try {
      const w = await api.renameWorkspace(id, normalizeName(value));
      onRenamed(w.name);
      setValue(w.name);
      setSaved(true);
    } catch (err) {
      setError(err instanceof ApiError && err.status === 403 ? "Only workspace admins can do this." : errorText(err instanceof Error ? err.message : ""));
    } finally {
      setBusy(false);
    }
  };
  return (
    <Panel>
      <PanelHead>Name</PanelHead>
      <form onSubmit={submit} className="flex flex-wrap items-end gap-2.5">
        <label className="flex min-w-[240px] flex-1 flex-col gap-1.5 text-[12.5px]">
          Workspace name
          <input className={field} required maxLength={100} value={value} onChange={(e) => setValue(e.target.value)} />
        </label>
        <Button type="submit" disabled={busy || !changed || !nameValid(value)}>
          Rename
        </Button>
      </form>
      {saved && <p className="mt-2 text-[13px] text-ok">Saved.</p>}
      {error && (
        <p role="alert" className="mt-2 text-[13px] text-fail">
          {error}
        </p>
      )}
    </Panel>
  );
}

export function MembersSection({ ws, members, invited }: { ws: string; members: Ov["members"]; invited: string[] }) {
  const [rows, setRows] = useState(members);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState<string | null>(null);
  const change = async (email: string, role: WorkspaceRole) => {
    if (busy || rows.find((m) => m.email === email)?.role === role) return;
    setBusy(email);
    setError(null);
    setSaved(null);
    try {
      const w = await api.setMemberRole(ws, email, role);
      setRows((cur) => withRole(cur, email, role, w.member_roles));
      setSaved(`${email} is now ${role === "viewer" ? "a viewer" : `an ${role}`}.`);
    } catch (err) {
      setError(memberMessage(err, "role"));
    } finally {
      setBusy(null);
    }
  };
  return (
    <Panel>
      <PanelHead>{text.members}</PanelHead>
      <table className="w-full">
        <thead>
          <tr>
            <th className={th}>Email</th>
            <th className={th}>Role</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((m) => {
            const last = isLastAdmin(rows, m.email);
            return (
              <tr key={m.email} className="border-t border-line">
                <td className={td}>{m.email}</td>
                <td className={td}>
                  <label className="flex flex-wrap items-center gap-2">
                    <span className="sr-only">Role for {m.email}</span>
                    <select
                      className={select}
                      value={m.role}
                      disabled={busy !== null || last}
                      title={last ? LAST_ADMIN_NOTE : undefined}
                      onChange={(e) => change(m.email, e.target.value as WorkspaceRole)}
                    >
                      {ROLES.map((r) => (
                        <option key={r} value={r}>
                          {roleLabel(r)}
                        </option>
                      ))}
                    </select>
                    {last && <span className="text-[12px] text-dim">{LAST_ADMIN_NOTE}</span>}
                  </label>
                </td>
              </tr>
            );
          })}
          {invited.map((e) => (
            <tr key={e} className="border-t border-line text-dim">
              <td className={td}>{e}</td>
              <td className={td}>
                <Chip tone="ask">invited</Chip>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {saved && <p className="mt-2 text-[13px] text-ok">{saved}</p>}
      {error && (
        <p role="alert" className="mt-2 text-[13px] text-fail">
          {error}
        </p>
      )}
    </Panel>
  );
}

export function SignInSection({ ws, sso, scim }: { ws: string; sso?: Ov["sso"]; scim?: Ov["scim"] }) {
  const [domains, setDomains] = useState<string[]>(() => verifiedDomains(sso?.connections ?? []));
  return (
    <Panel>
      <PanelHead>{text.signInTitle}</PanelHead>
      {sso ? (
        <SsoPart ws={ws} onDomains={setDomains} />
      ) : (
        <div>
          <h3 className="mb-1 text-[12.5px] font-medium">SSO</h3>
          {off}
        </div>
      )}
      {scim ? (
        <ScimPart ws={ws} domains={domains} />
      ) : (
        <div className="mt-4">
          <h3 className="mb-1 text-[12.5px] font-medium">SCIM</h3>
          {off}
        </div>
      )}
    </Panel>
  );
}

export function KeysSection({ ws }: { ws: string }) {
  const ids = useId();
  const [state, setState] = useState<KeyOnce | null>(null);
  const [mode, setMode] = useState<"loading" | "ok" | "off" | "error">("loading");
  const [name, setName] = useState("");
  const [role, setRole] = useState<KeyRole>("viewer");
  const [busy, setBusy] = useState(false);
  const [copied, setCopied] = useState(false);
  const [confirm, setConfirm] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const load = useCallback(() => {
    api.listWorkspaceKeys(ws).then(
      (r) => {
        setState((s) => (s ? { ...s, keys: r.api_keys } : keysLoaded(r.api_keys)));
        setMode("ok");
      },
      (e) => setMode(sectionState(e)),
    );
  }, [ws]);
  useEffect(load, [load]);

  const create = async (e: FormEvent) => {
    e.preventDefault();
    if (!name.trim() || busy) return;
    setBusy(true);
    setError(null);
    try {
      const out = await api.createWorkspaceKey(ws, name.trim(), role);
      setState((s) => keyIssued(s ?? keysLoaded([]), out));
      setCopied(false);
      setName("");
    } catch (err) {
      setError(keyError(err));
    } finally {
      setBusy(false);
    }
  };
  const revoke = async (k: ApiKey) => {
    setBusy(true);
    setError(null);
    setConfirm(null);
    try {
      await api.revokeWorkspaceKey(ws, k.id);
      setState((s) => (s ? keyRevoked(s, k.id) : s));
    } catch (err) {
      setError(keyError(err));
    } finally {
      setBusy(false);
    }
  };
  const copySecret = async () => {
    if (!state?.secret) return;
    try {
      await navigator.clipboard.writeText(state.secret);
      setCopied(true);
    } catch {
      setError("Copy did not work. Select the key and copy it.");
    }
  };

  const keys = state?.keys ?? [];
  const active = activeKeys(keys).length;
  return (
    <Panel aria-labelledby={`${ids}-h`}>
      <PanelHead right={mode === "ok" ? `${active} of ${MAX_ACTIVE_KEYS}` : undefined}>{text.keys}</PanelHead>
      {mode === "loading" && <Spinner label="Loading…" />}
      {mode === "off" && off}
      {mode === "error" && failed(() => (setMode("loading"), load()))}
      {mode === "ok" && (
        <>
          {state?.secret && (
            <div className="mb-4 border border-ok bg-ok-wash p-3" data-testid="key-secret">
              <p className="text-[12.5px] font-medium text-ok">Copy this key now. It is not shown again.</p>
              <div className="mt-2 flex flex-wrap items-center gap-2">
                <code className="min-w-0 flex-1 break-all text-xs">{state.secret}</code>
                <Button variant="ghost" onClick={copySecret}>
                  {copied ? "Copied" : "Copy"}
                </Button>
                <Button variant="ghost" onClick={() => setState((s) => (s ? secretDismissed(s) : s))}>
                  Done
                </Button>
              </div>
            </div>
          )}
          <form onSubmit={create} className="flex flex-wrap items-end gap-2.5">
            <label className="flex min-w-[240px] flex-1 flex-col gap-1.5 text-[12.5px]">
              Key name
              <input className={`${field} w-full`} required maxLength={100} value={name} onChange={(e) => setName(e.target.value)} placeholder="Claude Desktop" />
            </label>
            <label className="flex flex-col gap-1.5 text-[12.5px]">
              Access
              <select className={select} value={role} onChange={(e) => setRole(e.target.value as KeyRole)}>
                {KEY_ROLES.map((r) => (
                  <option key={r} value={r}>
                    {keyRoleLabel(r)}
                  </option>
                ))}
              </select>
            </label>
            <Button type="submit" disabled={busy || !name.trim() || active >= MAX_ACTIVE_KEYS}>
              Create key
            </Button>
          </form>
          <p className="mt-2 text-[12.5px] text-dim">{keyRoleHelp[role]} Never an admin.</p>
          {error && (
            <p role="alert" className="mt-2 text-[13px] text-fail">
              {error}
            </p>
          )}
          {keys.length === 0 ? (
            <p className="mt-4 text-[13px] text-dim">No keys yet.</p>
          ) : (
            <table className="mt-4 w-full">
              <thead>
                <tr>
                  <th className={th}>Name</th>
                  <th className={th}>Access</th>
                  <th className={th}>Created</th>
                  <th className={th}>Per day</th>
                  <th className={th} />
                </tr>
              </thead>
              <tbody>
                {keys.map((k) => (
                  <tr key={k.id} className="border-t border-line">
                    <td className={td}>{k.name}</td>
                    <td className={td}>{keyRoleLabel(k.role)}</td>
                    <td className={td}>{formatDate(k.created_at)}</td>
                    <td className={td}>{k.daily_quota}</td>
                    <td className={td}>
                      {k.disabled ? (
                        <Chip tone="fail">revoked</Chip>
                      ) : confirm === k.id ? (
                        <span className="flex flex-wrap gap-2">
                          <Button variant="ghost" disabled={busy} onClick={() => revoke(k)}>
                            Revoke for good
                          </Button>
                          <Button variant="ghost" onClick={() => setConfirm(null)}>
                            Cancel
                          </Button>
                        </span>
                      ) : (
                        <Button variant="ghost" disabled={busy} onClick={() => setConfirm(k.id)}>
                          Revoke
                        </Button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </>
      )}
    </Panel>
  );
}

export function RetentionSection({ ws, initial }: { ws: string; initial: { case_days: number; audit_days: number } }) {
  const [policy, setPolicy] = useState<RetentionPolicy | null>(null);
  const [caseDays, setCaseDays] = useState(String(initial.case_days));
  const [auditDays, setAuditDays] = useState(String(initial.audit_days));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  useEffect(() => {
    let live = true;
    api.getRetention(ws).then((p) => live && setPolicy(p), () => {});
    return () => {
      live = false;
    };
  }, [ws]);
  const problem = retentionProblem(caseDays, auditDays, policy?.limits);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (problem || busy) return;
    setBusy(true);
    setError(null);
    setSaved(false);
    try {
      const p = await api.setRetention(ws, retentionBody(caseDays, auditDays));
      setPolicy(p);
      setCaseDays(String(p.case_days));
      setAuditDays(String(p.audit_days));
      setSaved(true);
    } catch (err) {
      setError(err instanceof ApiError && err.status === 400 ? errorText(err.message) : sectionState(err) === "off" ? "Not allowed." : "That did not work. Try again.");
    } finally {
      setBusy(false);
    }
  };
  return (
    <Panel>
      <PanelHead>{text.retention}</PanelHead>
      <form onSubmit={submit} className="flex flex-wrap items-end gap-2.5">
        <label className="flex flex-col gap-1.5 text-[12.5px]">
          Delete cases after (days)
          <input className={`${field} w-40`} inputMode="numeric" value={caseDays} onChange={(e) => setCaseDays(e.target.value)} />
        </label>
        <label className="flex flex-col gap-1.5 text-[12.5px]">
          Delete audit events after (days)
          <input className={`${field} w-40`} inputMode="numeric" value={auditDays} onChange={(e) => setAuditDays(e.target.value)} />
        </label>
        <Button type="submit" disabled={busy || !!problem}>
          Save
        </Button>
      </form>
      <p className="mt-2 text-[12.5px] text-dim">
        Now: cases {daysLabel(policy?.case_days ?? initial.case_days, "kept until deleted")} · audit {daysLabel(policy?.audit_days ?? initial.audit_days, "default")}. Saving deletes nothing at once; old cases are removed by the next retention run.
      </p>
      {problem && <p className="mt-1 text-[12.5px] text-ask">{problem}</p>}
      {saved && <p className="mt-1 text-[13px] text-ok">Saved.</p>}
      {error && (
        <p role="alert" className="mt-1 text-[13px] text-fail">
          {error}
        </p>
      )}
    </Panel>
  );
}

export function UsageSection({ ws }: { ws: string }) {
  const [usage, setUsage] = useState<WorkspaceUsage | null>(null);
  const [mode, setMode] = useState<"loading" | "ok" | "off" | "error">("loading");
  const [amount, setAmount] = useState("");
  const [alert, setAlert] = useState("80");
  const [hard, setHard] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const load = useCallback(() => {
    api.workspaceUsage(ws).then(
      (r) => {
        setUsage(r.usage);
        if (r.usage.budget) {
          setAmount(String(r.usage.budget.monthly_usd));
          setAlert(String(r.usage.budget.alert_percent));
          setHard(r.usage.budget.hard_cap);
        }
        setMode("ok");
      },
      (e) => setMode(sectionState(e)),
    );
  }, [ws]);
  useEffect(load, [load]);
  const problem = amount === "" ? null : budgetProblem(amount, alert);
  const save = async (e: FormEvent) => {
    e.preventDefault();
    const p = budgetProblem(amount, alert);
    if (p || busy) return;
    setBusy(true);
    setError(null);
    try {
      await api.setBudget(ws, { monthly_usd: Number(amount), hard_cap: hard, alert_percent: alert.trim() === "" ? 0 : Number(alert) });
      load();
    } catch {
      setError("That did not work. Try again.");
    } finally {
      setBusy(false);
    }
  };
  const remove = async () => {
    setBusy(true);
    setError(null);
    try {
      await api.deleteBudget(ws);
      setAmount("");
      load();
    } catch {
      setError("That did not work. Try again.");
    } finally {
      setBusy(false);
    }
  };
  return (
    <Panel>
      <PanelHead right={usage?.totals.month}>{text.usage}</PanelHead>
      {mode === "loading" && <Spinner label="Loading…" />}
      {mode === "off" && off}
      {mode === "error" && failed(() => (setMode("loading"), load()))}
      {mode === "ok" && usage && (
        <>
          <dl className="grid grid-cols-3 gap-4 text-[13px]">
            <div>
              <dt className="text-[11px] uppercase tracking-[0.08em] text-dim">Predictions</dt>
              <dd className="text-xl">{usage.totals.predictions}</dd>
            </div>
            <div>
              <dt className="text-[11px] uppercase tracking-[0.08em] text-dim">Plan searches</dt>
              <dd className="text-xl">{usage.totals.plan_searches}</dd>
            </div>
            <div>
              <dt className="text-[11px] uppercase tracking-[0.08em] text-dim">Model spend</dt>
              <dd className="text-xl">{usd(usage.totals.model_usd)}</dd>
            </div>
          </dl>
          {usage.blocked && <p className="mt-3 text-[13px] text-fail">Budget reached. Model requests are refused until next month.</p>}
          {!usage.blocked && usage.alert && <p className="mt-3 text-[13px] text-ask">Spend passed the alert level.</p>}
          <form onSubmit={save} className="mt-4 flex flex-wrap items-end gap-2.5">
            <label className="flex flex-col gap-1.5 text-[12.5px]">
              Monthly budget (USD)
              <input className={`${field} w-40`} inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} />
            </label>
            <label className="flex flex-col gap-1.5 text-[12.5px]">
              Alert at (%)
              <input className={`${field} w-28`} inputMode="numeric" value={alert} onChange={(e) => setAlert(e.target.value)} />
            </label>
            <label className="flex h-10 items-center gap-2 text-[13px]">
              <input type="checkbox" checked={hard} onChange={(e) => setHard(e.target.checked)} />
              Stop at the budget
            </label>
            <Button type="submit" disabled={busy || amount === "" || !!problem}>
              Save budget
            </Button>
            {usage.budget && (
              <Button variant="ghost" disabled={busy} onClick={remove}>
                Remove budget
              </Button>
            )}
          </form>
          {usage.budget && <p className="mt-2 text-[12.5px] text-dim">Left this month: {usd(usage.remaining_usd)}</p>}
          {problem && <p className="mt-1 text-[12.5px] text-ask">{problem}</p>}
          {error && (
            <p role="alert" className="mt-1 text-[13px] text-fail">
              {error}
            </p>
          )}
        </>
      )}
    </Panel>
  );
}

export function AuditSection({ ws }: { ws: string }) {
  const [f, setF] = useState<AuditFilters>(noFilters);
  const [events, setEvents] = useState<AuditEvent[] | null>(null);
  const [next, setNext] = useState<string | undefined>();
  const [mode, setMode] = useState<"loading" | "ok" | "off" | "error">("loading");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const apply = useCallback(
    (page: { events: AuditEvent[]; next_cursor?: string }, append: boolean) => {
      setEvents((prev) => (append && prev ? [...prev, ...page.events] : page.events));
      setNext(page.next_cursor);
      setMode("ok");
    },
    [],
  );
  const run = async (filters: AuditFilters, cursor?: string) => {
    setBusy(true);
    setError(null);
    try {
      apply(await api.workspaceAudit(ws, auditQuery(filters, cursor)), !!cursor);
    } catch (e) {
      if (events) setError(auditError(e));
      else setMode(sectionState(e));
    } finally {
      setBusy(false);
    }
  };
  // The first page loads once; filters apply on "Search".
  useEffect(() => {
    let live = true;
    api.workspaceAudit(ws, auditQuery(noFilters)).then(
      (p) => live && apply(p, false),
      (e) => live && setMode(sectionState(e)),
    );
    return () => {
      live = false;
    };
  }, [ws, apply]);
  const set = (k: keyof AuditFilters) => (e: { target: { value: string } }) => setF((p) => ({ ...p, [k]: e.target.value }));
  return (
    <Panel>
      <PanelHead>{text.audit}</PanelHead>
      {mode === "loading" && <Spinner label="Loading…" />}
      {mode === "off" && off}
      {mode === "error" && failed(() => (setMode("loading"), void run(f)))}
      {mode === "ok" && (
        <>
          <form
            className="flex flex-wrap items-end gap-2.5"
            onSubmit={(e) => {
              e.preventDefault();
              run(f);
            }}
          >
            <label className="flex flex-col gap-1.5 text-[12.5px]">
              Who
              <select className={select} value={f.actor_kind} onChange={set("actor_kind")}>
                <option value="">Anyone</option>
                {ACTORS.map((a) => (
                  <option key={a} value={a}>
                    {actorLabel(a)}
                  </option>
                ))}
              </select>
            </label>
            <label className="flex flex-col gap-1.5 text-[12.5px]">
              What
              <select className={select} value={f.action} onChange={set("action")}>
                <option value="">Anything</option>
                {ACTIONS.map((a) => (
                  <option key={a} value={a}>
                    {actionLabel(a)}
                  </option>
                ))}
              </select>
            </label>
            <label className="flex flex-col gap-1.5 text-[12.5px]">
              Result
              <select className={select} value={f.outcome} onChange={set("outcome")}>
                <option value="">Any</option>
                {OUTCOMES.map((o) => (
                  <option key={o} value={o}>
                    {o}
                  </option>
                ))}
              </select>
            </label>
            <label className="flex flex-col gap-1.5 text-[12.5px]">
              From
              <input type="date" className={select} value={f.from} onChange={set("from")} />
            </label>
            <label className="flex flex-col gap-1.5 text-[12.5px]">
              To
              <input type="date" className={select} value={f.to} onChange={set("to")} />
            </label>
            <label className="flex flex-col gap-1.5 text-[12.5px]">
              Order
              <select className={select} value={f.order} onChange={set("order")}>
                <option value="desc">Newest first</option>
                <option value="asc">Oldest first</option>
              </select>
            </label>
            <Button type="submit" disabled={busy}>
              Search
            </Button>
          </form>
          {error && (
            <p role="alert" className="mt-2 text-[13px] text-fail">
              {error}
            </p>
          )}
          {events && events.length === 0 ? (
            <p className="mt-4 text-[13px] text-dim">No events.</p>
          ) : (
            <div className="mt-4 overflow-x-auto">
              <table className="w-full min-w-[520px]">
                <thead>
                  <tr>
                    <th className={th}>Time</th>
                    <th className={th}>Who</th>
                    <th className={th}>What</th>
                    <th className={th}>Result</th>
                  </tr>
                </thead>
                <tbody>
                  {events?.map((ev) => (
                    <tr key={ev.seq} className="border-t border-line">
                      <td className={td}>{new Date(ev.time).toISOString().replace("T", " ").slice(0, 19)}</td>
                      <td className={td}>{actorLabel(ev.actor_kind)}</td>
                      <td className={td}>{actionLabel(ev.action)}</td>
                      <td className={td}>
                        <Chip tone={ev.outcome === "allowed" ? "ok" : "fail"}>{ev.outcome}</Chip>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          {next && (
            <div className="mt-3">
              <Button variant="ghost" disabled={busy} onClick={() => run(f, next)}>
                More
              </Button>
            </div>
          )}
        </>
      )}
    </Panel>
  );
}

// Shown only when the API answers; otherwise one quiet line.
export function WebhooksSection({ ws }: { ws: string }) {
  const [list, setList] = useState<{ endpoints: WebhookEndpoint[]; available: string[] } | null>(null);
  const [mode, setMode] = useState<"loading" | "ok" | "off">("loading");
  const [url, setUrl] = useState("");
  const [events, setEvents] = useState<string[]>([]);
  const [secret, setSecret] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const load = useCallback(() => {
    api.listWebhooks(ws).then(
      (r) => {
        setList({ endpoints: r.endpoints, available: r.available_events });
        setMode("ok");
      },
      () => setMode("off"),
    );
  }, [ws]);
  useEffect(load, [load]);
  const act = async (f: () => Promise<unknown>) => {
    setBusy(true);
    setError(null);
    try {
      await f();
      load();
    } catch (e) {
      setError(webhookError(e));
    } finally {
      setBusy(false);
    }
  };
  if (mode === "loading") return null;
  if (mode === "off")
    return (
      <Panel>
        <PanelHead>{text.webhooks}</PanelHead>
        {off}
      </Panel>
    );
  return (
    <Panel>
      <PanelHead>{text.webhooks}</PanelHead>
      {secret && (
        <div className="mb-4 border border-ok bg-ok-wash p-3" data-testid="webhook-secret">
          <p className="text-[12.5px] font-medium text-ok">Copy this secret now. It is not shown again.</p>
          <div className="mt-2 flex flex-wrap items-center gap-2">
            <code className="min-w-0 flex-1 break-all text-xs">{secret}</code>
            <Button variant="ghost" onClick={() => setSecret(null)}>
              Done
            </Button>
          </div>
        </div>
      )}
      <form
        className="flex flex-wrap items-end gap-2.5"
        onSubmit={(e) => {
          e.preventDefault();
          if (!url.trim() || events.length === 0) return;
          act(async () => {
            const out = await api.createWebhook(ws, { url: url.trim(), events });
            setSecret(out.endpoint.secret ?? null);
            setUrl("");
            setEvents([]);
          });
        }}
      >
        <label className="flex min-w-[260px] flex-1 flex-col gap-1.5 text-[12.5px]">
          Endpoint URL
          <input type="url" required className={`${field} w-full`} value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://hooks.example.com/unde" />
        </label>
        <fieldset className="flex flex-wrap gap-x-3 gap-y-1 text-[12.5px]">
          <legend className="mb-1.5">Events</legend>
          {list?.available.map((ev) => (
            <label key={ev} className="flex items-center gap-1.5">
              <input type="checkbox" checked={events.includes(ev)} onChange={(e) => setEvents((p) => (e.target.checked ? [...p, ev] : p.filter((x) => x !== ev)))} />
              {WEBHOOK_EVENT_LABEL[ev] ?? ev}
            </label>
          ))}
        </fieldset>
        <Button type="submit" disabled={busy || !url.trim() || events.length === 0}>
          Add endpoint
        </Button>
      </form>
      {error && (
        <p role="alert" className="mt-2 text-[13px] text-fail">
          {error}
        </p>
      )}
      {list && list.endpoints.length === 0 ? (
        <p className="mt-4 text-[13px] text-dim">No endpoints yet.</p>
      ) : (
        <table className="mt-4 w-full">
          <thead>
            <tr>
              <th className={th}>URL</th>
              <th className={th}>Events</th>
              <th className={th}>Secret</th>
              <th className={th} />
            </tr>
          </thead>
          <tbody>
            {list?.endpoints.map((w) => (
              <tr key={w.id} className="border-t border-line">
                <td className={`${td} break-all`}>{w.url}</td>
                <td className={td}>{w.events.map((e) => WEBHOOK_EVENT_LABEL[e] ?? e).join(", ")}</td>
                <td className={td}>…{w.secret_hint}</td>
                <td className={td}>
                  <span className="flex flex-wrap gap-2">
                    <Button variant="ghost" disabled={busy} onClick={() => act(() => api.setWebhookEnabled(ws, w.id, !w.enabled))}>
                      {w.enabled ? "Pause" : "Resume"}
                    </Button>
                    <Button variant="ghost" disabled={busy} onClick={() => act(() => api.deleteWebhook(ws, w.id))}>
                      Delete
                    </Button>
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </Panel>
  );
}

type Resolved = { gate: Gate; ov?: Ov };

// Session, then workspace, then the overview. Each refusal is a gate with its
// own plain sentence; the overview read is what finally decides (403).
export async function resolveAdmin(): Promise<Resolved> {
  try {
    const s = (await fetch("/api/session", { cache: "no-store" }).then((r) => r.json())) as { email: string | null };
    if (!s.email) return { gate: "signin" };
    let mine: WorkspacesMine;
    try {
      mine = await api.workspacesMine();
    } catch (e) {
      return { gate: e instanceof ApiError && e.status === 404 ? "no_teams" : overviewGate(e) === "signin" ? "signin" : "error" };
    }
    const w = mine.workspace;
    if (!w) return { gate: "no_workspace" };
    if (!w.admin) return { gate: "not_admin" };
    const raw = (await api.adminOverview(w.id)) as AdminOverview;
    const ov = raw as unknown as Ov;
    return { gate: "ok", ov: { ...ov, workspace: { ...ov.workspace, name: ov.workspace.name ?? w.name } } };
  } catch (e) {
    return { gate: overviewGate(e) };
  }
}

// The screen: who may see it is the server's answer. Only an admin gets past
// workspaces/mine (admin: true) and then the overview read (403 otherwise).
export function AdminScreen() {
  const [gate, setGate] = useState<Gate | "loading">("loading");
  const [ov, setOv] = useState<Ov | null>(null);
  const [name, setName] = useState("");
  const apply = useCallback((r: Resolved) => {
    setGate(r.gate);
    if (r.ov) {
      setOv(r.ov);
      setName(r.ov.workspace.name);
    }
  }, []);
  useEffect(() => {
    let live = true;
    resolveAdmin().then((r) => live && apply(r));
    return () => {
      live = false;
    };
  }, [apply]);
  const reload = () => {
    setGate("loading");
    resolveAdmin().then(apply);
  };

  if (gate === "loading") return <Spinner label="Loading…" />;
  if (gate === "signin")
    return (
      <Panel>
        <p className="duo text-sm">{text.signIn}</p>
        <div className="mt-3">
          <SignIn />
        </div>
      </Panel>
    );
  if (gate !== "ok" || !ov) {
    const msg = gate === "no_teams" ? text.noTeams : gate === "no_workspace" ? text.noWorkspace : gate === "not_admin" ? text.notAdmin : text.error;
    return (
      <Panel>
        <p className="duo text-sm" role={gate === "error" ? "alert" : undefined}>
          {msg}
        </p>
        <div className="mt-3 flex gap-2">
          {gate === "error" && (
            <Button variant="ghost" onClick={reload}>
              {text.retry}
            </Button>
          )}
          <Link href="/cases" className="inline-flex min-h-10 items-center text-sm text-ok underline underline-offset-2">
            Back to cases
          </Link>
        </div>
      </Panel>
    );
  }
  const ws = ov.workspace.id;
  const f = ov.features;
  return (
    <div className="flex flex-col gap-6" data-testid="admin-screen">
      <NameSection id={ws} name={name} onRenamed={setName} />
      <MembersSection ws={ws} members={ov.members} invited={ov.invited} />
      <SignInSection ws={ws} sso={ov.sso} scim={ov.scim} />
      {f.api_keys && <KeysSection ws={ws} />}
      {f.retention && ov.retention && <RetentionSection ws={ws} initial={ov.retention} />}
      {f.metering && <UsageSection ws={ws} />}
      {f.audit && <AuditSection ws={ws} />}
      <WebhooksSection ws={ws} />
    </div>
  );
}
