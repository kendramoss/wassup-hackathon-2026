import type { ApiError } from "./api/client";

// Recording an export (Download, Print) answers with the letter as the case
// stands now. The letter on screen came from an earlier preview, so it goes
// out only when that answer proves it is still current (D97). Everything
// else holds it back:
//
//  - the recording call fails for any reason (409 conflict, a chosen move or
//    plan that no longer exists, 401/403, 404, 5xx, network): "failed", with
//    the client's own error so its message and retry rule are shown as is
//  - the recorded letter differs from the previewed one (same format, raw
//    content, before the requester's own fields are filled in): "changed"
//  - a success without a letter cannot be checked: "malformed"
//
// There is no fail-open path: if freshness cannot be verified the letter is
// not downloaded or printed, and the user retries.
export type Recorded =
  | { ok: true }
  | { ok: false; reason: "failed"; error: ApiError | null }
  | { ok: false; reason: "changed" | "malformed" };

export async function recordExport(
  record: () => Promise<{ content?: unknown } | null | undefined>,
  previewed: string,
): Promise<Recorded> {
  let res: { content?: unknown } | null | undefined;
  try {
    res = await record();
  } catch (e) {
    const err = e as Partial<ApiError> | null;
    return { ok: false, reason: "failed", error: err && typeof err.status === "number" ? (err as ApiError) : null };
  }
  if (!res || typeof res.content !== "string") return { ok: false, reason: "malformed" };
  if (res.content !== previewed) return { ok: false, reason: "changed" };
  return { ok: true };
}
