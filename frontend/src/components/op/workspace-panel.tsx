"use client";

import Link from "next/link";
import { useEffect, useId, useRef, useState } from "react";
import { ACCESS_STALE, ApiError, api, type WorkspaceRole, type WorkspacesMine } from "@/lib/api/client";
import { ROLES, memberMessage, memberRows, roleLabel, roleNoun, roleSummary, type MemberAction } from "@/lib/roles";
import { Button, Chip, Panel, PanelHead, Spinner, cx } from "./ui";

const field = "h-10 min-w-0 flex-1 rounded-[2px] border border-edge bg-white px-3 text-[13.5px] focus:border-ok";

// The signed-in user's team workspace (D68, roles #283): create one, accept
// an invitation, see your role, leave, and (the admin) invite, set roles and
// remove members. Shown only when signed in and the server has TEAMS on
// (otherwise /v1/workspaces/mine is a 404). The role and member roles are what
// the server returns; every change is followed by a re-read, and a refused
// call (403, 404) re-reads too because the role may have changed.
export function WorkspacePanel() {
  const ids = useId();
  const [mine, setMine] = useState<WorkspacesMine | null>(null);
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);
  // A removal or leave waits for a second click: it ends access to cases.
  const [confirm, setConfirm] = useState<{ kind: "leave" | "remove"; email: string } | null>(null);
  // #604: focus the safe action on open, keep Tab local, and return focus to
  // whichever trigger opened the confirmation.
  const actionRef = useRef<HTMLButtonElement>(null);
  const cancelRef = useRef<HTMLButtonElement>(null);
  const restoreTriggerRef = useRef<string | null>(null);
  useEffect(() => {
    if (confirm) {
      cancelRef.current?.focus();
    } else if (restoreTriggerRef.current && typeof document !== "undefined") {
      const key = restoreTriggerRef.current;
      restoreTriggerRef.current = null;
      [...document.querySelectorAll<HTMLButtonElement>("button[data-confirm-trigger]")]
        .find((button) => button.dataset.confirmTrigger === key)
        ?.focus();
    }
  }, [confirm]);
  const cancelConfirm = () => setConfirm(null);

  // Server truth. A 404 means Teams is off; any other failure keeps what is shown.
  const load = () =>
    api.workspacesMine().then(setMine, (e) => {
      if (e instanceof ApiError && e.status === 404) setMine(null);
    });
  useEffect(() => {
    let live = true;
    // Ask only when signed in: an anonymous request would be a 401.
    fetch("/api/session", { cache: "no-store" })
      .then((r) => r.json() as Promise<{ email: string | null }>)
      .then((s) => {
        if (live && s.email) api.workspacesMine().then((m) => live && setMine(m), () => {});
      }, () => {});
    // A write elsewhere was refused: the role may have changed, so re-read it.
    const stale = () => {
      if (live) api.workspacesMine().then((m) => live && setMine(m), () => {});
    };
    window.addEventListener(ACCESS_STALE, stale);
    return () => {
      live = false;
      window.removeEventListener(ACCESS_STALE, stale);
    };
  }, []);

  if (!mine) return null;

  const act = async (f: () => Promise<unknown>, action: MemberAction, done?: string) => {
    setBusy(true);
    setError(null);
    setNote(null);
    restoreTriggerRef.current = null;
    setConfirm(null);
    try {
      await f();
      await load();
      if (done) setNote(done);
    } catch (e) {
      setError(memberMessage(e, action));
      // The server may know something the screen does not (a changed role).
      await load();
    } finally {
      setBusy(false);
    }
  };

  const ws = mine.workspace;
  const rows = ws ? memberRows(ws, mine.email) : [];
  const leave = (w: NonNullable<typeof ws>) => act(() => api.removeMember(w.id, mine.email), "leave", `You left ${w.name}.`);
  return (
    <section className="mt-12" aria-labelledby={`${ids}-h`}>
      <h2 id={`${ids}-h`} className="eyebrow mb-3">
        Team workspace
      </h2>
      <Panel>
        {ws ? (
          <>
            <PanelHead right={`you're ${roleNoun(ws.role)}`}>{ws.name}</PanelHead>
            {ws.admin && (
              <p className="mb-2 text-[13px]">
                <Link href="/settings/workspace" className="text-ok underline underline-offset-2" data-testid="admin-link">
                  Workspace admin →
                </Link>
              </p>
            )}
            <p className="duo text-[13px]" data-testid="role-summary">
              <b className="font-medium">{roleLabel(ws.role)}.</b> {roleSummary(ws.role)}
            </p>
            <p className="duo mt-1 text-[13px] text-dim">Members see each other&apos;s cases created after they joined, and the demos.</p>
            <ul className="mt-3 flex flex-col gap-1.5 text-[13px]">
              {rows.map((m) => (
                <li key={m.email} className="flex flex-wrap items-center gap-2">
                  <span>
                    {m.email}
                    {m.you && <span className="text-dim"> (you)</span>}
                  </span>
                  {ws.admin && m.role ? (
                    <label className="flex items-center gap-1.5 text-[12.5px]">
                      <span className="sr-only">Role for {m.email}</span>
                      <select
                        className="h-8 rounded-[2px] border border-edge bg-white px-2 text-[12.5px]"
                        value={m.role}
                        disabled={busy}
                        onChange={(e) => {
                          const role = e.target.value as WorkspaceRole;
                          if (role !== m.role) act(() => api.setMemberRole(ws.id, m.email, role), "role", `${m.email} is now ${roleNoun(role)}.`);
                        }}
                      >
                        {ROLES.map((r) => (
                          <option key={r} value={r}>
                            {roleLabel(r)}
                          </option>
                        ))}
                      </select>
                    </label>
                  ) : (
                    m.role && <Chip>{roleLabel(m.role).toLowerCase()}</Chip>
                  )}
                  {ws.admin && !m.you && (
                    <Button
                      variant="ghost"
                      disabled={busy}
                      data-confirm-trigger={`remove:${m.email}`}
                      onClick={() => {
                        restoreTriggerRef.current = `remove:${m.email}`;
                        setConfirm({ kind: "remove", email: m.email });
                      }}
                    >
                      Remove
                    </Button>
                  )}
                </li>
              ))}
              {ws.invited.map((m) => (
                <li key={m} className="flex flex-wrap items-center gap-2 text-dim">
                  {m}
                  <Chip tone="ask">invited</Chip>
                </li>
              ))}
            </ul>
            {confirm && (
              <div
                role="alertdialog"
                aria-label="Confirm"
                className="mt-3 border border-dashed border-guide bg-sheet px-4 py-3 text-[13px]"
                onKeyDown={(e) => {
                  if (e.key === "Escape") {
                    e.preventDefault();
                    e.nativeEvent.stopImmediatePropagation();
                    cancelConfirm();
                    return;
                  }
                  if (e.key !== "Tab" || !actionRef.current || !cancelRef.current) return;
                  const first = actionRef.current;
                  const last = cancelRef.current;
                  if (e.shiftKey && document.activeElement === first) {
                    e.preventDefault();
                    e.nativeEvent.stopImmediatePropagation();
                    last.focus();
                  } else if (!e.shiftKey && document.activeElement === last) {
                    e.preventDefault();
                    e.nativeEvent.stopImmediatePropagation();
                    first.focus();
                  }
                }}
              >
                <p className="duo">
                  {confirm.kind === "leave"
                    ? `Leave ${ws.name}? You lose access to every case in the workspace, including ones you created. Your personal cases stay yours.`
                    : `Remove ${confirm.email}? They lose access to every case in the workspace, including ones they created.`}
                </p>
                <div className="mt-2 flex gap-2">
                  <Button
                    ref={actionRef}
                    variant="ghost"
                    disabled={busy}
                    onClick={() => (confirm.kind === "leave" ? leave(ws) : act(() => api.removeMember(ws.id, confirm.email), "remove", `Removed ${confirm.email}.`))}
                  >
                    {confirm.kind === "leave" ? "Leave workspace" : "Remove member"}
                  </Button>
                  <Button ref={cancelRef} variant="ghost" onClick={cancelConfirm}>
                    Cancel
                  </Button>
                </div>
              </div>
            )}
            {!confirm && (
              <div className="mt-3">
                <Button
                  variant="ghost"
                  disabled={busy}
                  data-confirm-trigger="leave"
                  onClick={() => {
                    restoreTriggerRef.current = "leave";
                    setConfirm({ kind: "leave", email: mine.email });
                  }}
                >
                  Leave workspace
                </Button>
              </div>
            )}
            {ws.admin && (
              <form
                className="mt-4 flex flex-wrap items-end gap-2.5"
                onSubmit={(e) => {
                  e.preventDefault();
                  if (email.trim()) act(() => api.inviteMember(ws.id, email.trim()), "invite", `Invited ${email.trim()}. They join when they next sign in and accept; nothing is shared before that.`).then(() => setEmail(""));
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
                      {inv.invited_by ? <>{inv.invited_by} invited you to </> : <>You were invited to </>}<b className="font-medium">{inv.name}</b>.
                    </span>
                    <Button variant="ghost" disabled={busy} onClick={() => act(() => api.joinWorkspace(inv.workspace_id), "join", `You joined ${inv.name}.`)}>
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
                if (name.trim()) act(() => api.createWorkspace(name.trim()), "create").then(() => setName(""));
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
