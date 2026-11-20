"use client";

import { useEffect, useState } from "react";
import { ACCESS_STALE, api, type WorkspaceRole } from "./api/client";

// The signed-in caller's workspace role, read from the server
// (GET /v1/workspaces/mine) on every mount and again after a refused write,
// never cached across identities. null: no workspace, signed out, Teams off or
// not known; the page then behaves as before and the server's 403 decides. Used
// only to decide whether create and batch (which have no case yet) are offered.
export function useWorkspaceRole(): WorkspaceRole | null {
  const [role, setRole] = useState<WorkspaceRole | null>(null);
  useEffect(() => {
    if (typeof window === "undefined") return;
    let live = true;
    const read = () =>
      fetch("/api/session", { cache: "no-store" })
        .then((r) => r.json() as Promise<{ email: string | null }>)
        .then((s) => (s.email ? api.workspacesMine().then((m) => live && setRole(m.workspace?.role ?? null)) : live && setRole(null)))
        .catch(() => live && setRole(null));
    read();
    window.addEventListener(ACCESS_STALE, read);
    return () => {
      live = false;
      window.removeEventListener(ACCESS_STALE, read);
    };
  }, []);
  return role;
}
