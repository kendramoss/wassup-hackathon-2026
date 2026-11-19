import type { ApiError } from "./api/client";

// Recording an export (Download, Print) answers with the letter as the case
// stands now. The letter on screen came from an earlier preview, so before it
// goes out it is checked against that answer (D97):
//
//  - 409 "conflict": the case changed during the request. Held back.
//  - the recorded letter differs from the previewed one (same format, raw
//    content, before the requester's own fields are filled in): the case
//    changed since the preview. Held back; the draft is fetched again.
//  - 400 "invalid_input" for a chosen move or plan: that choice no longer
//    exists on the case (its plan search was cleared). Held back.
//  - a success without a letter: not trusted. Held back.
//
// Limit: any other failure to record (a 5xx, the network, a 4xx without a
// chosen plan) leaves nothing to compare, so the previewed letter is let
// through unrecorded. That is the old behaviour, kept so a recording outage
// does not block a letter that previewed fine; it cannot prove freshness.
export type Recorded =
  | { ok: true }
  | { ok: false; reason: "conflict" | "changed" | "gone" | "malformed"; error?: ApiError };

type Selection = { move_key?: string; plan_index?: number };

export async function recordExport(
  record: () => Promise<{ content?: unknown } | null | undefined>,
  previewed: string,
  pick: Selection = {},
): Promise<Recorded> {
  let res: { content?: unknown } | null | undefined;
  try {
    res = await record();
  } catch (e) {
    const err = e as ApiError | null;
    if (err && err.status === 409 && (err.code as string) === "conflict") return { ok: false, reason: "conflict", error: err };
    const chosen = pick.move_key !== undefined || pick.plan_index !== undefined;
    if (err && err.status === 400 && (err.code as string) === "invalid_input" && chosen) return { ok: false, reason: "gone", error: err };
    return { ok: true };
  }
  if (!res || typeof res.content !== "string") return { ok: false, reason: "malformed" };
  if (res.content !== previewed) return { ok: false, reason: "changed" };
  return { ok: true };
}
