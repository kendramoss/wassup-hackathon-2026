"use client";

import { useEffect, useId, useState } from "react";
import { ApiError, api, type WorkspacesMine } from "@/lib/api/client";
import { Button, Chip, Panel, PanelHead, Spinner, cx } from "./ui";

const field = "h-10 min-w-0 flex-1 rounded-[2px] border border-edge bg-white px-3 text-[13.5px] focus:border-ok";

// The signed-in user's team workspace (D68): create one, accept an
// invitation, or (the admin) invite by email. Shown only when signed in and
// the server has TEAMS on (otherwise /v1/workspaces/mine is a 404).
export function WorkspacePanel() {
  const ids = useId();
  const [mine, setMine] = useState<WorkspacesMine | null>(null);
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);

  const load = () => api.workspacesMine().then(setMine, () => setMine(null));
  useEffect(() => {
    let live = true;
    // Ask only when signed in: an anonymous request would be a 401.
    fetch("/api/session", { cache: "no-store" })
      .then((r) => r.json() as Promise<{ email: string | null }>)
      .then((s) => {
        if (live && s.email) api.workspacesMine().then((m) => live && setMine(m), () => {});
      }, () => {});
    return () => {
      live = false;
    };
  }, []);

  if (!mine) return null;

  const act = async (f: () => Promise<unknown>, done?: string) => {
    setBusy(true);
    setError(null);
    setNote(null);
    try {
      await f();
      await load();
      if (done) setNote(done);
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  };

  const ws = mine.workspace;
  return (
    <section className="mt-12" aria-labelledby={`${ids}-h`}>
      <h2 id={`${ids}-h`} className="eyebrow mb-3">
        Team workspace
      </h2>
      <Panel>
        {ws ? (
          <>
            <PanelHead right={ws.admin ? "you're the admin" : undefined}>{ws.name}</PanelHead>
            <p className="duo text-[13px] text-dim">Members see each other&apos;s cases created after they joined, and the demos.</p>
            <ul className="mt-3 flex flex-col gap-1.5 text-[13px]">
              {ws.members.map((m) => (
                <li key={m} className="flex flex-wrap items-center gap-2">
                  {m}
                  {m === ws.created_by && <Chip>admin</Chip>}
                </li>
              ))}
              {ws.invited.map((m) => (
                <li key={m} className="flex flex-wrap items-center gap-2 text-dim">
                  {m}
                  <Chip tone="ask">invited</Chip>
                </li>
              ))}
            </ul>
            {ws.admin && (
              <form
                className="mt-4 flex flex-wrap items-end gap-2.5"
                onSubmit={(e) => {
                  e.preventDefault();
                  if (email.trim()) act(() => api.inviteMember(ws.id, email.trim()), `Invited ${email.trim()}. They join when they next sign in and accept; nothing is shared before that.`).then(() => setEmail(""));
                }}
              >
                <label className="flex min-w-[240px] flex-1 flex-col gap-1.5 text-[12.5px]">
                  Invite member (email)
                  <input type="email" required className={field} value={email} onChange={(e) => setEmail(e.target.value)} placeholder="amy@acme.com" />
                </label>
                <Button type="submit" disabled={busy || !email.trim()}>
                  Invite member
                </Button>
              </form>
            )}
          </>
        ) : (
          <>
            <PanelHead>Share cases with your team</PanelHead>
            {mine.invites.length > 0 && (
              <ul className="mb-4 flex flex-col gap-2.5">
                {mine.invites.map((inv) => (
                  <li key={inv.workspace_id} className="flex flex-wrap items-center gap-x-3 gap-y-1.5 text-[13.5px]">
                    <span>
                      {inv.invited_by} invited you to <b className="font-medium">{inv.name}</b>.
                    </span>
                    <Button variant="ghost" disabled={busy} onClick={() => act(() => api.joinWorkspace(inv.workspace_id), `You joined ${inv.name}.`)}>
                      Join
                    </Button>
                  </li>
                ))}
                <li className="text-xs text-dim">Only cases created after you join are shared. You can be in one workspace.</li>
              </ul>
            )}
            <form
              className="flex flex-wrap items-end gap-2.5"
              onSubmit={(e) => {
                e.preventDefault();
                if (name.trim()) act(() => api.createWorkspace(name.trim())).then(() => setName(""));
              }}
            >
              <label className="flex min-w-[240px] flex-1 flex-col gap-1.5 text-[12.5px]">
                Create a team workspace
                <input required className={field} value={name} onChange={(e) => setName(e.target.value)} placeholder="Acme Imports" />
              </label>
              <Button type="submit" disabled={busy || !name.trim()}>
                Create workspace
              </Button>
            </form>
          </>
        )}
        {busy && (
          <div className="mt-3">
            <Spinner label="Saving…" />
          </div>
        )}
        {note && <p className="mt-3 text-[13px] text-ok">{note}</p>}
        {error && (
          <p role="alert" className={cx("mt-3 text-[13px] text-fail")}>
            {error}
          </p>
        )}
      </Panel>
    </section>
  );
}

function message(e: unknown): string {
  if (!(e instanceof ApiError)) return "Something went wrong. Try again.";
  if (e.status === 403) return "Only the admin can invite members.";
  if (e.status === 409) return e.message ? e.message[0].toUpperCase() + e.message.slice(1) + "." : "Already a member, or the workspace is full.";
  if (e.status === 404) return "That invitation is no longer open.";
  return e.message || "Something went wrong. Try again.";
}
