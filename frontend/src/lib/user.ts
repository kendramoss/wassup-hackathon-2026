// Google sign-in (D68): the browser gets a Google ID token from Google
// Identity Services and posts it to /api/session, which keeps it in an
// HttpOnly cookie. The /api/v1 proxy sends it as X-User-Token; the backend
// verifies it on every request. Scripts never read the token, and it never
// goes in localStorage or a URL. Server-side only.

export const USER_COOKIE = "op_user";

// A JWT: three base64url parts. Google ID tokens are about 1 KB.
const JWT = /^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/;

export function validToken(v: string | undefined | null): v is string {
  return Boolean(v && v.length <= 4096 && JWT.test(v));
}

export function userFromCookieHeader(header: string | null): string | undefined {
  for (const part of (header ?? "").split(";")) {
    const [k, ...v] = part.trim().split("=");
    if (k === USER_COOKIE && validToken(v.join("="))) return v.join("=");
  }
  return undefined;
}

export type Claims = { email?: string; exp?: number; aud?: string; iss?: string; name?: string; picture?: string };

// Google profile photos are served from googleusercontent.com; anything
// else in the claim is ignored.
export function profilePicture(c: Claims | null): string | null {
  const p = c?.picture;
  return typeof p === "string" && /^https:\/\/[a-z0-9-]+\.googleusercontent\.com\//.test(p) ? p : null;
}

// The token's claims, unverified: for display and the cookie's lifetime
// only. The backend checks the signature, audience and expiry.
export function claims(token: string): Claims | null {
  try {
    const part = token.split(".")[1].replace(/-/g, "+").replace(/_/g, "/");
    return JSON.parse(Buffer.from(part, "base64").toString("utf8")) as Claims;
  } catch {
    return null;
  }
}

// The OAuth client ID, read at run time so the deploy sets it with the
// other env vars (NEXT_PUBLIC_ is accepted too). Unset: no sign-in.
export function googleClientId(): string | undefined {
  return process.env.GOOGLE_CLIENT_ID || process.env.NEXT_PUBLIC_GOOGLE_CLIENT_ID || undefined;
}

export function userSetCookie(https: boolean, token: string, maxAge: number): string {
  return `${USER_COOKIE}=${token}; HttpOnly;${https ? " Secure;" : ""} SameSite=Lax; Path=/; Max-Age=${Math.max(0, Math.floor(maxAge))}`;
}

export function userClearCookie(https: boolean): string {
  return `${USER_COOKIE}=; HttpOnly;${https ? " Secure;" : ""} SameSite=Lax; Path=/; Max-Age=0`;
}
