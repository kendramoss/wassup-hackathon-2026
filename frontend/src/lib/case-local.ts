import type { CaseResponse } from "./api/client";

// A pinned demo is read-only for visitors (backend D57), so this tab keeps the
// latest state a screen produced for it (use-case.ts). That copy must never
// hide what the server says now: a copy saved against an older server state,
// or before a field existed, made the flagship lose `facts_source` and with it
// the "Two inputs, two answers" line (judge panel round 3, D270).
//
// The stored copy records the server's updated_at it was based on. It is used
// only while the server still reports that same updated_at, and it is laid
// over the server's case, so a field the server now sends and the copy lacks
// still comes through. A copy in the old format (the bare case) is dropped.
export type LocalCase = { base: string | null; case: CaseResponse };

export function localEntry(c: CaseResponse, base: string | null | undefined): LocalCase {
  return { base: base ?? null, case: c };
}

export function pickLocal(server: CaseResponse, stored: unknown): CaseResponse {
  const s = stored as Partial<LocalCase> | null | undefined;
  if (!s || typeof s !== "object" || !("base" in s) || !s.case || typeof s.case !== "object") return server;
  if ((s.base ?? null) !== (server.updated_at ?? null)) return server;
  if (s.case.id !== server.id) return server;
  return { ...server, ...s.case } as CaseResponse;
}
