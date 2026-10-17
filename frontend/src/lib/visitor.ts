// Visitor ID (D58): each browser gets a random ID in an HttpOnly cookie, and
// the /api/v1 proxy sends it as X-Visitor-ID so the backend lists and opens
// only that visitor's cases (plus the pinned demos). Scripts can't read the
// cookie and the ID never goes in a URL. Shared by src/proxy.ts (set on the
// first page load, before any API call) and lib/backend.ts (fallback for a
// first request that goes straight to the API).

export const VISITOR_COOKIE = "op_visitor";
const VISITOR_ID = /^[A-Za-z0-9_-]{16,128}$/;

export function validVisitorId(v: string | undefined | null): v is string {
  return Boolean(v && VISITOR_ID.test(v));
}

// 32 random bytes, base64url without padding: 43 characters.
export function newVisitorId(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(32));
  return btoa(String.fromCharCode(...bytes)).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

export function visitorFromCookieHeader(header: string | null): string | undefined {
  for (const part of (header ?? "").split(";")) {
    const [k, ...v] = part.trim().split("=");
    if (k === VISITOR_COOKIE && validVisitorId(v.join("="))) return v.join("=");
  }
  return undefined;
}

// Secure only over https (Cloud Run); a local http dev server would drop it.
export function isHttps(request: Request): boolean {
  const proto = request.headers.get("x-forwarded-proto") ?? new URL(request.url).protocol.replace(":", "");
  return proto.split(",")[0].trim() === "https";
}

export const visitorCookieOptions = (https: boolean) =>
  ({ httpOnly: true, secure: https, sameSite: "lax", path: "/", maxAge: 31536000 }) as const;

export function visitorSetCookie(request: Request, id: string): string {
  return `${VISITOR_COOKIE}=${id}; HttpOnly;${isHttps(request) ? " Secure;" : ""} SameSite=Lax; Path=/; Max-Age=31536000`;
}
