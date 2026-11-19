import type { ApiError } from "./api/client";

// Recording an export (Download, Print) can be refused with 409 "conflict"
// when the case changed under the draft on screen (D97). That must be seen,
// and the letter must not go out as if it had been recorded. Any other
// failure to record does not hold back a letter that already previewed.
export type Recorded = { ok: true } | { ok: false; error: ApiError };

export async function recordExport(record: () => Promise<unknown>): Promise<Recorded> {
  try {
    await record();
    return { ok: true };
  } catch (e) {
    const err = e as ApiError | null;
    if (err && err.status === 409 && (err.code as string) === "conflict") return { ok: false, error: err };
    return { ok: true };
  }
}
