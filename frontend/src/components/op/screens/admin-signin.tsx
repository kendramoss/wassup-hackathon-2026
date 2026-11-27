"use client";

// Sign-in setup on the workspace admin screen (#870): SSO connections (OIDC and
// SAML), the DNS record that proves a domain, the switch that makes SSO the
// only way in, and the SCIM token. Every route is the backend's own and is for
// a workspace admin; a 403 or 404 only changes what is said here. A client
// secret or a SCIM token is never shown after the answer that sets it.
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api, type ScimStatus, type SsoConnection } from "@/lib/api/client";
import {
  SECRET_NOTE,
  SSO_ROLES,
  dnsLine,
  emptySsoForm,
  scimDismissed,
  scimError,
  scimIssued,
  scimLoaded,
  sectionState,
  ssoBody,
  ssoDisableBody,
  ssoEnforceBody,
  ssoError,
  ssoFormOf,
  ssoProblem,
  text,
  verifiedDomains,
  type ScimOnce,
  type SsoForm,
} from "@/lib/admin";
import { Button, Chip, Spinner } from "../ui";

const field = "h-10 min-w-0 rounded-[2px] border border-edge bg-white px-3 text-[13.5px] focus:border-ok";
const select = "h-10 rounded-[2px] border border-edge bg-white px-2 text-[13px]";
const label = "flex min-w-[240px] flex-1 flex-col gap-1.5 text-[12.5px]";

const off = <p className="text-[13px] text-dim">{text.off}</p>;
const failed = (onRetry: () => void) => (
  <div className="flex flex-wrap items-center gap-2">
    <p className="text-[13px] text-fail">{text.error}</p>
    <Button variant="ghost" onClick={onRetry}>
      {text.retry}
    </Button>
  </div>
);
const alertLine = (msg: string | null) =>
  msg && (
    <p role="alert" className="mt-2 text-[13px] text-fail">
      {msg}
    </p>
  );

// A value shown once, with Copy and Done.
export function Once({ title, value, onDone, testid }: { title: string; value: string; onDone: () => void; testid: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <div className="mb-4 border border-ok bg-ok-wash p-3" data-testid={testid}>
      <p className="text-[12.5px] font-medium text-ok">{title}</p>
      <div className="mt-2 flex flex-wrap items-center gap-2">
        <code className="min-w-0 flex-1 break-all text-xs">{value}</code>
        <Button
          variant="ghost"
          onClick={async () => {
            try {
              await navigator.clipboard.writeText(value);
              setCopied(true);
            } catch {}
          }}
        >
          {copied ? "Copied" : "Copy"}
        </Button>
        <Button variant="ghost" onClick={onDone}>
          Done
        </Button>
      </div>
    </div>
  );
}

export function SsoFormView({ initial, existing, busy, error, onSubmit, onCancel }: { initial: SsoForm; existing: SsoConnection | null; busy: boolean; error: string | null; onSubmit: (f: SsoForm) => void; onCancel: () => void }) {
  const [f, setF] = useState<SsoForm>(initial);
  const set = (k: keyof SsoForm) => (e: { target: { value: string } }) => setF((s) => ({ ...s, [k]: e.target.value }));
  const problem = ssoProblem(f);
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!problem && !busy) onSubmit(f);
  };
  const roles = existing?.default_role === "admin" ? ([...SSO_ROLES, "admin"] as string[]) : ([...SSO_ROLES] as string[]);
  return (
    <form onSubmit={submit} className="mt-3 flex flex-col gap-3 border border-line p-3">
      <div className="flex flex-wrap items-end gap-2.5">
        <label className="flex flex-col gap-1.5 text-[12.5px]">
          Type
          <select className={select} value={f.protocol} disabled={!!existing} onChange={(e) => setF((s) => ({ ...s, protocol: e.target.value as SsoForm["protocol"] }))}>
            <option value="oidc">OIDC</option>
            <option value="saml">SAML</option>
          </select>
        </label>
        <label className={label}>
          Email domains
          <input className={`${field} w-full`} value={f.domains} onChange={set("domains")} placeholder="acme.com, acme.co.uk" />
        </label>
        <label className="flex flex-col gap-1.5 text-[12.5px]">
          New members join as
          <select className={select} value={f.default_role} onChange={set("default_role")}>
            {roles.map((r) => (
              <option key={r} value={r}>
                {r}
              </option>
            ))}
          </select>
        </label>
      </div>
      {f.protocol === "oidc" ? (
        <div className="flex flex-wrap items-end gap-2.5">
          <label className={label}>
            Issuer URL
            <input className={`${field} w-full`} value={f.oidc_issuer} onChange={set("oidc_issuer")} placeholder="https://login.acme.com" />
          </label>
          <label className={label}>
            Client ID
            <input className={`${field} w-full`} value={f.oidc_client_id} onChange={set("oidc_client_id")} autoComplete="off" />
          </label>
          <label className={label}>
            Client secret
            <input
              className={`${field} w-full`}
              type="password"
              autoComplete="new-password"
              value={f.oidc_client_secret}
              onChange={set("oidc_client_secret")}
              placeholder={existing?.has_client_secret ? SECRET_NOTE : ""}
            />
          </label>
        </div>
      ) : (
        <div className="flex flex-col gap-2.5">
          <div className="flex flex-wrap items-end gap-2.5">
            <label className={label}>
              Sign-in URL
              <input className={`${field} w-full`} value={f.saml_sso_url} onChange={set("saml_sso_url")} placeholder="https://idp.acme.com/sso" />
            </label>
            <label className={label}>
              Entity ID
              <input className={`${field} w-full`} value={f.saml_issuer} onChange={set("saml_issuer")} />
            </label>
          </div>
          <label className="flex flex-col gap-1.5 text-[12.5px]">
            Signing certificates (PEM, up to 3)
            <textarea className="min-h-24 rounded-[2px] border border-edge bg-white p-3 font-mono text-xs focus:border-ok" value={f.saml_certs} onChange={set("saml_certs")} spellCheck={false} />
          </label>
        </div>
      )}
      <div className="flex flex-wrap items-center gap-2">
        <Button type="submit" disabled={busy || !!problem}>
          {existing ? "Save" : "Add connection"}
        </Button>
        <Button variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
        {problem && <span className="text-[12.5px] text-ask">{problem}</span>}
      </div>
      {alertLine(error)}
    </form>
  );
}

export function ConnectionCard({ ws, c, onChange, onRemoved, onEdit }: { ws: string; c: SsoConnection; onChange: (c: SsoConnection) => void; onRemoved: () => void; onEdit: () => void }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [confirm, setConfirm] = useState<"remove" | "enforce" | null>(null);
  const [problems, setProblems] = useState<string[]>([]);
  const run = async (fn: () => Promise<SsoConnection | void>) => {
    setBusy(true);
    setError(null);
    setConfirm(null);
    try {
      const out = await fn();
      if (out) {
        onChange(out);
        setProblems((out.checks ?? []).filter((k) => !k.verified).map((k) => `${k.domain}: ${k.problem || "no matching record yet"}`));
      }
    } catch (err) {
      setError(ssoError(err));
    } finally {
      setBusy(false);
    }
  };
  const pending = c.domains.filter((d) => !d.verified);
  return (
    <li className="border-t border-line py-3 first:border-t-0 first:pt-0" data-testid={`sso-${c.id}`}>
      <div className="flex flex-wrap items-center gap-2 text-[13px]">
        <b className="font-medium">{c.protocol.toUpperCase()}</b>
        <Chip tone={c.enabled ? "ok" : undefined}>{c.enabled ? "on" : "off"}</Chip>
        {c.enforced && <Chip tone="ask">required</Chip>}
        <span className="text-dim">joins as {c.default_role}</span>
        {c.protocol === "oidc" && <span className="text-dim">{c.has_client_secret ? "secret saved" : "no secret"}</span>}
      </div>
      <ul className="mt-2 flex flex-col gap-2 text-[13px]">
        {c.domains.map((d) => (
          <li key={d.domain}>
            <span className="mr-2">{d.domain}</span>
            <Chip tone={d.verified ? "ok" : "ask"}>{d.verified ? "verified" : "not verified"}</Chip>
            {!d.verified && (
              <div className="mt-1 text-[12.5px]">
                <p className="text-dim">Add this DNS TXT record, then check.</p>
                <code className="block break-all text-xs" data-testid={`dns-${d.domain}`}>
                  {dnsLine(d)}
                </code>
              </div>
            )}
          </li>
        ))}
      </ul>
      {problems.length > 0 && (
        <ul className="mt-2 text-[12.5px] text-ask">
          {problems.map((p) => (
            <li key={p}>{p}</li>
          ))}
        </ul>
      )}
      {c.service_provider && (
        <dl className="mt-2 text-[12.5px] text-dim">
          {c.service_provider.oidc_redirect_url && (
            <div>
              Redirect URL: <code className="break-all text-xs text-ink">{c.service_provider.oidc_redirect_url}</code>
            </div>
          )}
          {c.service_provider.saml_acs_url && (
            <div>
              ACS URL: <code className="break-all text-xs text-ink">{c.service_provider.saml_acs_url}</code>
            </div>
          )}
          {c.service_provider.saml_entity_id && (
            <div>
              Entity ID: <code className="break-all text-xs text-ink">{c.service_provider.saml_entity_id}</code>
            </div>
          )}
        </dl>
      )}
      <div className="mt-2 flex flex-wrap items-center gap-2">
        {pending.length > 0 && (
          <Button variant="ghost" disabled={busy} onClick={() => run(() => api.verifySsoConnection(ws, c.id, false))}>
            Check DNS
          </Button>
        )}
        {!c.enabled ? (
          <Button variant="ghost" disabled={busy} onClick={() => run(() => api.verifySsoConnection(ws, c.id, true))}>
            Turn on
          </Button>
        ) : (
          <Button variant="ghost" disabled={busy} onClick={() => run(() => api.updateSsoConnection(ws, c.id, ssoDisableBody(c)))}>
            Turn off
          </Button>
        )}
        {c.enabled &&
          (c.enforced ? (
            <Button variant="ghost" disabled={busy} onClick={() => run(() => api.updateSsoConnection(ws, c.id, ssoEnforceBody(c, false)))}>
              Allow Google sign-in
            </Button>
          ) : confirm === "enforce" ? (
            <span className="flex flex-wrap items-center gap-2 text-[12.5px]">
              Google sign-in stops for these domains.
              <Button variant="ghost" disabled={busy} onClick={() => run(() => api.updateSsoConnection(ws, c.id, ssoEnforceBody(c, true)))}>
                Require SSO
              </Button>
              <Button variant="ghost" onClick={() => setConfirm(null)}>
                Cancel
              </Button>
            </span>
          ) : (
            <Button variant="ghost" disabled={busy} onClick={() => setConfirm("enforce")}>
              Require SSO
            </Button>
          ))}
        <Button variant="ghost" disabled={busy} onClick={onEdit}>
          Edit
        </Button>
        {confirm === "remove" ? (
          <span className="flex flex-wrap items-center gap-2 text-[12.5px]">
            Its sessions stop at once.
            <Button
              variant="ghost"
              disabled={busy}
              onClick={() =>
                run(async () => {
                  await api.deleteSsoConnection(ws, c.id);
                  onRemoved();
                })
              }
            >
              Remove for good
            </Button>
            <Button variant="ghost" onClick={() => setConfirm(null)}>
              Cancel
            </Button>
          </span>
        ) : (
          <Button variant="ghost" disabled={busy} onClick={() => setConfirm("remove")}>
            Remove
          </Button>
        )}
      </div>
      {alertLine(error)}
    </li>
  );
}

export function SsoPart({ ws, onDomains }: { ws: string; onDomains?: (d: string[]) => void }) {
  const [conns, setConns] = useState<SsoConnection[] | null>(null);
  const [mode, setMode] = useState<"loading" | "ok" | "off" | "error">("loading");
  const [editing, setEditing] = useState<string | "new" | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const publish = useCallback((cs: SsoConnection[]) => {
    setConns(cs);
    onDomains?.(verifiedDomains(cs));
  }, [onDomains]);
  const load = useCallback(() => {
    api.listSsoConnections(ws).then(
      (r) => {
        publish(r.connections);
        setMode("ok");
      },
      (e) => setMode(sectionState(e)),
    );
  }, [ws, publish]);
  useEffect(load, [load]);

  const save = async (f: SsoForm) => {
    setBusy(true);
    setError(null);
    try {
      if (editing === "new") {
        const c = await api.createSsoConnection(ws, ssoBody(f));
        publish([...(conns ?? []), c]);
      } else if (editing) {
        const c = await api.updateSsoConnection(ws, editing, ssoBody(f));
        publish((conns ?? []).map((x) => (x.id === c.id ? c : x)));
      }
      setEditing(null);
    } catch (err) {
      setError(ssoError(err));
    } finally {
      setBusy(false);
    }
  };
  const replace = (c: SsoConnection) => publish((conns ?? []).map((x) => (x.id === c.id ? c : x)));
  const list = conns ?? [];
  const editedConn = editing && editing !== "new" ? list.find((c) => c.id === editing) ?? null : null;
  return (
    <div>
      <h3 className="mb-1 text-[12.5px] font-medium">SSO</h3>
      {mode === "loading" && <Spinner label="Loading…" />}
      {mode === "off" && off}
      {mode === "error" && failed(() => (setMode("loading"), load()))}
      {mode === "ok" && (
        <>
          {list.length === 0 && editing !== "new" && <p className="text-[13px] text-dim">No connection.</p>}
          <ul>
            {list.map((c) =>
              editing === c.id ? (
                <li key={c.id}>
                  <SsoFormView initial={ssoFormOf(c)} existing={c} busy={busy} error={error} onSubmit={save} onCancel={() => (setEditing(null), setError(null))} />
                </li>
              ) : (
                <ConnectionCard key={c.id} ws={ws} c={c} onChange={replace} onRemoved={() => publish(list.filter((x) => x.id !== c.id))} onEdit={() => (setEditing(c.id), setError(null))} />
              ),
            )}
          </ul>
          {editing === "new" ? (
            <SsoFormView initial={emptySsoForm} existing={null} busy={busy} error={error} onSubmit={save} onCancel={() => (setEditing(null), setError(null))} />
          ) : (
            !editedConn &&
            editing === null && (
              <div className="mt-2">
                <Button variant="ghost" onClick={() => (setEditing("new"), setError(null))}>
                  Add connection
                </Button>
              </div>
            )
          )}
        </>
      )}
    </div>
  );
}

export function ScimPart({ ws, domains }: { ws: string; domains: string[] }) {
  const [state, setState] = useState<ScimOnce | null>(null);
  const [mode, setMode] = useState<"loading" | "ok" | "off" | "error">("loading");
  const [pick, setPick] = useState<string[] | null>(null);
  const [role, setRole] = useState<"viewer" | "editor">("viewer");
  const [form, setForm] = useState(false);
  const [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const load = useCallback(() => {
    api.getScim(ws).then(
      (s: ScimStatus) => {
        setState((cur) => (cur?.token ? { ...cur, status: scimLoaded(s).status } : scimLoaded(s)));
        setMode("ok");
      },
      (e) => setMode(sectionState(e)),
    );
  }, [ws]);
  useEffect(load, [load]);

  const chosen = (pick ?? domains).filter((d) => domains.includes(d));
  const issue = async (e: FormEvent) => {
    e.preventDefault();
    if (!chosen.length || busy) return;
    setBusy(true);
    setError(null);
    try {
      const s = await api.issueScimToken(ws, { domains: chosen, default_role: role });
      setState(scimIssued(s));
      setForm(false);
    } catch (err) {
      setError(scimError(err));
    } finally {
      setBusy(false);
    }
  };
  const revoke = async () => {
    setBusy(true);
    setError(null);
    setConfirm(false);
    try {
      await api.revokeScimToken(ws);
      setState({ status: { configured: false, domains: [], allow_admin: false, base_url: state?.status?.base_url ?? "", token_header: state?.status?.token_header ?? "" }, token: null });
    } catch (err) {
      setError(scimError(err));
    } finally {
      setBusy(false);
    }
  };
  const st = state?.status;
  return (
    <div className="mt-4">
      <h3 className="mb-1 text-[12.5px] font-medium">SCIM</h3>
      {mode === "loading" && <Spinner label="Loading…" />}
      {mode === "off" && off}
      {mode === "error" && failed(() => (setMode("loading"), load()))}
      {mode === "ok" && st && (
        <>
          {state?.token && <Once testid="scim-token" title="Copy this token now. It is not shown again." value={state.token} onDone={() => setState((s) => (s ? scimDismissed(s) : s))} />}
          {st.configured ? (
            <div className="text-[13px]">
              <p>
                <Chip tone="ok">on</Chip> token ends …{st.hint} · {st.domains.join(", ")} · new users are {st.default_role}
              </p>
              <dl className="mt-2 text-[12.5px] text-dim">
                <div>
                  Base URL: <code className="break-all text-xs text-ink">{st.base_url}</code>
                </div>
                <div>
                  Header: <code className="text-xs text-ink">{st.token_header}</code>
                </div>
              </dl>
            </div>
          ) : (
            <p className="text-[13px] text-dim">Not set up.</p>
          )}
          {domains.length === 0 ? (
            <p className="mt-2 text-[12.5px] text-dim">Verify an SSO domain first.</p>
          ) : form ? (
            <form onSubmit={issue} className="mt-3 flex flex-col gap-3 border border-line p-3">
              <fieldset className="flex flex-wrap gap-3 text-[13px]">
                <legend className="mb-1 text-[12.5px]">Domains it manages</legend>
                {domains.map((d) => (
                  <label key={d} className="flex items-center gap-1.5">
                    <input
                      type="checkbox"
                      checked={chosen.includes(d)}
                      onChange={(e) => setPick(e.target.checked ? [...chosen, d] : chosen.filter((x) => x !== d))}
                    />
                    {d}
                  </label>
                ))}
              </fieldset>
              <label className="flex flex-col gap-1.5 text-[12.5px]">
                New users join as
                <select className={`${select} w-40`} value={role} onChange={(e) => setRole(e.target.value as "viewer" | "editor")}>
                  <option value="viewer">viewer</option>
                  <option value="editor">editor</option>
                </select>
              </label>
              {st.configured && <p className="text-[12.5px] text-ask">The current token stops working at once.</p>}
              <div className="flex gap-2">
                <Button type="submit" disabled={busy || chosen.length === 0}>
                  {st.configured ? "Replace token" : "Create token"}
                </Button>
                <Button variant="ghost" onClick={() => setForm(false)}>
                  Cancel
                </Button>
              </div>
            </form>
          ) : (
            <div className="mt-2 flex flex-wrap items-center gap-2">
              <Button variant="ghost" disabled={busy} onClick={() => (setForm(true), setError(null))}>
                {st.configured ? "Replace token" : "Create token"}
              </Button>
              {st.configured &&
                (confirm ? (
                  <span className="flex flex-wrap items-center gap-2 text-[12.5px]">
                    Provisioning stops at once.
                    <Button variant="ghost" disabled={busy} onClick={revoke}>
                      Revoke for good
                    </Button>
                    <Button variant="ghost" onClick={() => setConfirm(false)}>
                      Cancel
                    </Button>
                  </span>
                ) : (
                  <Button variant="ghost" disabled={busy} onClick={() => setConfirm(true)}>
                    Revoke
                  </Button>
                ))}
            </div>
          )}
          {alertLine(error)}
        </>
      )}
    </div>
  );
}
