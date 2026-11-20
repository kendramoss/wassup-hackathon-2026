// Workspace roles (#283), as the UI words and uses them. The role is only ever
// read from the server (GET /v1/workspaces/mine); nothing here sends a role,
// workspace or tenant to the API as the caller's authority. Everything that
// hides or disables a control is a convenience: the server decides, and every
// call site also handles its 403.
import { ApiError, type Workspace, type WorkspaceRole } from "./api/client";

export const ROLES: readonly WorkspaceRole[] = ["viewer", "editor", "admin"];

export function roleLabel(r: WorkspaceRole): string {
  return r === "admin" ? "Admin" : r === "editor" ? "Editor" : "Viewer";
}

export function roleNoun(r: WorkspaceRole): string {
  return r === "viewer" ? "a viewer" : `an ${r}`;
}

export function roleSummary(r: WorkspaceRole): string {
  if (r === "admin") return "You can change cases and manage members and their roles.";
  if (r === "editor") return "You can read and change the workspace's cases.";
  return "You can read the workspace's cases and preview exports. Changing them needs editor access.";
}

// Creating a case from a workspace role. A user with no workspace (null) keeps
// owner behaviour. This is only for create/batch, which have no case yet; for
// an existing case the server's own answer about that case is used, never the
// role alone (a viewer still owns their personal pre-workspace cases).
export function canCreate(role: WorkspaceRole | null | undefined): boolean {
  return role !== "viewer";
}

export const VIEWER_REASON = "Your role in this workspace is viewer. Ask a workspace admin for editor access to do this.";

export const isStatus = (e: unknown, status: number, code?: string): boolean =>
  e instanceof ApiError && e.status === status && (code === undefined || (e.code as string) === code);

// 403 forbidden: the role, or the case, does not allow this. Pinned demos use
// the same status; the server's message tells them apart.
export const isForbidden = (e: unknown): boolean => isStatus(e, 403, "forbidden");

// 401 unauthenticated on a private case operation: sign in is required (strict
// mode), or the sign-in lapsed. A bad API key is 401 "unauthorized", which is
// a different problem and is not this.
export const isSigninRequired = (e: unknown): boolean => isStatus(e, 401, "unauthenticated");

// A pinned demo's refusal is worded about the demo or the admin token; a role
// refusal is not.
export function isDemoRefusal(e: ApiError): boolean {
  return /pinned demo|admin token/i.test(e.message);
}

export function roleRefusal(e: ApiError): string {
  const m = e.message?.trim();
  return m ? m[0].toUpperCase() + m.slice(1) + (/[.!?]$/.test(m) ? "" : ".") : VIEWER_REASON;
}

export type MemberAction = "role" | "remove" | "leave" | "invite" | "join" | "create";

// What a failed workspace call says, per status. 409 on role/remove/leave is
// the last admin.
export function memberMessage(e: unknown, action: MemberAction): string {
  if (!(e instanceof ApiError)) return "Something went wrong. Try again.";
  if (e.code === "network") return e.message;
  if (e.status === 401) return "Sign in again to manage the workspace.";
  if (e.status === 403) return action === "leave" ? "You can't leave right now." : "Only a workspace admin can do that. Your role may have changed; this page now shows the current one.";
  if (e.status === 404) {
    if (action === "join") return "That invitation is no longer open.";
    if (action === "role" || action === "remove") return "That person is not a member of this workspace any more.";
    return "This workspace is no longer available to you.";
  }
  if (e.status === 409) {
    if (action === "role" || action === "remove" || action === "leave") return "A workspace needs at least one admin. Make another member an admin first.";
    return e.message ? e.message[0].toUpperCase() + e.message.slice(1) + "." : "Already a member, or the workspace is full.";
  }
  if (e.status === 400) return action === "role" ? "That role isn't valid." : e.message || "That didn't look right.";
  return e.message || "Something went wrong. Try again.";
}

export type MemberRow = { email: string; role: WorkspaceRole | null; you: boolean };

// The member list as the panel shows it. Roles come from member_roles, which
// the server sends to admins only; for everyone else the row has no role.
export function memberRows(ws: Workspace, myEmail: string | null): MemberRow[] {
  return ws.members.map((email) => ({ email, role: ws.member_roles?.[email] ?? null, you: email === myEmail }));
}

// Per-case write access, from the server's own answer for that case
// (can_write on CaseResponse and CaseSummary). Only an explicit false on a
// case that is not a pinned demo makes a screen read-only. Absent means the
// answer is unknown: nothing is granted by it and nothing is hidden by it, and
// the server's 403 still decides. A pinned demo keeps its existing read-only
// journeys (Ask, plan, scenarios and compare answer without saving), so it is
// never gated here, whatever can_write says. The role alone is never used for
// an existing case: a viewer still owns their personal pre-workspace cases.
export function caseReadOnly(c: { pinned?: boolean | null; can_write?: boolean } | null | undefined): boolean {
  return Boolean(c) && !c!.pinned && c!.can_write === false;
}

// Props for a control that writes: disabled with the plain reason.
export const writeGate = (readOnly: boolean): { disabled?: true; title?: string } => (readOnly ? { disabled: true, title: VIEWER_REASON } : {});
