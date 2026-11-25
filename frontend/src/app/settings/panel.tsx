"use client";

import Link from "next/link";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { SignIn } from "@/components/op/sign-in";
import { Button, Chip, Panel, PanelHead, Spinner } from "@/components/op/ui";
import { ApiError, api, errorText, type ApiKey } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { formatDate } from "@/lib/format";

// Signed-in key management (#788). The secret is held in this component only
// for the one response that creates it; a reload loses it, as does the server.
type Session = { email: string | null };

export function SettingsPanel() {
  const [email, setEmail] = useState<string | null | undefined>(undefined);
  const [keys, setKeys] = useState<ApiKey[] | null>(null);
  const [name, setName] = useState("");
  const [secret, setSecret] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [creating, setCreating] = useState(false);
  const [revoking, setRevoking] = useState<string | null>(null);
  const [unavailable, setUnavailable] = useState(false);
  const [loadFailed, setLoadFailed] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    fetch("/api/session", { cache: "no-store" })
      .then((r) => r.json() as Promise<Session>)
      .then(
        (s) => {
          if (live) {
            setEmail(s.email);
            setLoadFailed(false);
          }
        },
        () => {
          if (live) {
            setEmail(null);
            setLoadFailed(false);
          }
        },
      );
    return () => {
      live = false;
    };
  }, []);

  const load = useCallback(() => {
    api.listApiKeys().then(
      (out) => {
        setKeys(out.api_keys);
        setUnavailable(false);
      },
      (e: unknown) => {
        if (e instanceof ApiError && e.status === 404) {
          setUnavailable(true);
          setKeys([]);
          return;
        }
        if (e instanceof ApiError && e.status === 401) {
          // The token lapsed between the session read and this call.
          setEmail(null);
          return;
        }
        // A transient failure must not leave a permanent spinner: show what
        // happened and let the user retry without reloading the page.
        setLoadFailed(true);
        setError(errorText(e instanceof Error ? e.message : String(e)));
      },
    );
  }, []);

  useEffect(() => {
    if (email) load();
  }, [email, load]);

  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!name.trim() || creating) return;
    setCreating(true);
    setError(null);
    try {
      const out = await api.createApiKey(name.trim());
      setSecret(out.key);
      setCopied(false);
      setName("");
      setKeys((prev) => [out.api_key, ...(prev ?? [])]);
    } catch (e) {
      setError(errorText(e instanceof Error ? e.message : String(e)));
    } finally {
      setCreating(false);
    }
  }

  async function copySecret() {
    if (!secret) return;
    try {
      await navigator.clipboard.writeText(secret);
      setCopied(true);
    } catch {
      setError(copy.settings.apiKeys.copyFailed);
    }
  }

  async function revoke(key: ApiKey) {
    if (!window.confirm(copy.settings.apiKeys.revokeConfirm)) return;
    setRevoking(key.id);
    setError(null);
    try {
      await api.revokeApiKey(key.id);
      setKeys((prev) => prev?.map((k) => (k.id === key.id ? { ...k, disabled: true } : k)) ?? []);
    } catch (e) {
      setError(errorText(e instanceof Error ? e.message : String(e)));
    } finally {
      setRevoking(null);
    }
  }

  if (email === undefined) return <Spinner label="Checking your account…" />;

  if (!email) {
    return (
      <Panel>
        <p className="duo text-sm text-ink">{copy.settings.signIn}</p>
        <div className="mt-3">
          <SignIn />
        </div>
        <Link href="/cases" className="mt-3 inline-block min-h-10 text-sm text-ok underline underline-offset-2">
          {copy.signIn.demosLink}
        </Link>
      </Panel>
    );
  }

  return (
    <Panel aria-label={copy.settings.apiKeys.heading}>
      <PanelHead right={unavailable ? undefined : copy.settings.apiKeys.perDay}>{copy.settings.apiKeys.heading}</PanelHead>
      <p className="duo text-sm text-dim">{copy.settings.apiKeys.intro}</p>
      {unavailable && <p className="duo mt-3 text-sm text-ask">{copy.settings.apiKeys.unavailable}</p>}

      {secret && (
        <div className="mt-4 border border-ok bg-ok-wash p-3">
          <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-ok">{copy.settings.apiKeys.secretLead}</p>
          <div className="mt-2 flex flex-wrap items-center gap-2">
            <code className="min-w-0 flex-1 break-all text-xs text-ok">{secret}</code>
            <Button variant="ghost" onClick={copySecret} className="min-h-8 px-3 text-xs">
              {copied ? copy.settings.apiKeys.copied : copy.settings.apiKeys.copy}
            </Button>
          </div>
          <p className="duo mt-2 text-xs text-dim">{copy.settings.apiKeys.secretNote}</p>
        </div>
      )}

      {!unavailable && (
        <form onSubmit={create} className="mt-4 flex flex-wrap items-end gap-2">
          <label className="w-full text-xs text-dim sm:min-w-64 sm:flex-1">
            {copy.settings.apiKeys.nameLabel}
            <input
              required
              maxLength={100}
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={copy.settings.apiKeys.namePlaceholder}
              className="mt-1 w-full rounded-[2px] border border-edge bg-sheet px-3 py-2 text-sm text-ink outline-none focus:border-ok"
            />
          </label>
          <Button type="submit" disabled={creating || !name.trim()} className="max-sm:min-h-11">
            {creating ? copy.settings.apiKeys.creating : copy.settings.apiKeys.create}
          </Button>
        </form>
      )}

      {error && (
        <p role="alert" className="duo mt-3 text-sm text-fail">
          {error}
        </p>
      )}

      <ul className="mt-5 grid gap-2 border-t border-line pt-4">
        {loadFailed ? (
          <li className="flex flex-wrap items-center gap-2 border border-edge bg-sheet px-3 py-2.5">
            <p className="duo flex-1 text-sm text-dim">{copy.settings.apiKeys.loadFailed}</p>
            <Button
              variant="ghost"
              onClick={() => {
                setLoadFailed(false);
                setError(null);
                load();
              }}
              className="min-h-8 px-3 text-xs"
            >
              {copy.settings.apiKeys.retry}
            </Button>
          </li>
        ) : keys === null ? (
          <li className="text-sm text-dim">
            <Spinner label="Loading…" />
          </li>
        ) : keys.length === 0 ? (
          <li className="duo text-sm text-dim">{copy.settings.apiKeys.empty}</li>
        ) : (
          keys.map((key) => (
            <li key={key.id} className="flex flex-wrap items-center justify-between gap-2 border border-edge bg-sheet px-3 py-2.5">
              <div className="min-w-0">
                <p className="truncate text-sm font-medium text-ink">{key.name}</p>
                <p className="mt-0.5 text-xs text-dim">
                  {copy.settings.apiKeys.created} {formatDate(key.created_at)} · {copy.settings.apiKeys.perDay}
                </p>
              </div>
              <div className="flex items-center gap-2">
                <Chip tone={key.disabled ? "ask" : "ok"}>{key.disabled ? copy.settings.apiKeys.revoked : copy.settings.apiKeys.active}</Chip>
                {!key.disabled && (
                  <Button variant="ghost" disabled={revoking === key.id} onClick={() => revoke(key)} className="min-h-8 px-3 text-xs">
                    {revoking === key.id ? copy.settings.apiKeys.revoking : copy.settings.apiKeys.revoke}
                  </Button>
                )}
              </div>
            </li>
          ))
        )}
      </ul>

      <Link href="/developers" className="mt-4 inline-block min-h-10 text-sm text-ok underline underline-offset-2">
        {copy.settings.apiKeys.developers}
      </Link>
    </Panel>
  );
}
