import { claims, googleClientId, userClearCookie, userFromCookieHeader, userSetCookie, validToken } from "@/lib/user";
import { isHttps } from "@/lib/visitor";

// The signed-in user (D68). GET says who is signed in (the cookie is
// HttpOnly, so the page can't read it) and the OAuth client ID; POST keeps
// a Google ID token; DELETE signs out.

export async function GET(request: Request) {
  const clientId = googleClientId();
  const token = userFromCookieHeader(request.headers.get("cookie"));
  const c = token ? claims(token) : null;
  const live = c?.exp && c.exp * 1000 > Date.now();
  return Response.json(
    { client_id: clientId ?? null, email: live ? (c?.email ?? null) : null, exp: live ? c?.exp : null },
    { headers: { "Cache-Control": "no-store" } },
  );
}

export async function POST(request: Request) {
  // Only this site's own script may sign someone in: a cross-site form
  // can't send JSON, and the Origin must be ours (login CSRF).
  const origin = request.headers.get("origin");
  const ours = !origin || origin === new URL(request.url).origin || origin === forwardedOrigin(request) || origin.replace(/^https?:\/\//, "") === request.headers.get("host");
  if (!request.headers.get("content-type")?.startsWith("application/json") || !ours) {
    return Response.json({ code: "forbidden", message: "sign-in must come from this site" }, { status: 403 });
  }
  const clientId = googleClientId();
  if (!clientId) return Response.json({ code: "not_found", message: "sign-in is off" }, { status: 404 });
  let credential: unknown;
  try {
    ({ credential } = await request.json());
  } catch {
    credential = undefined;
  }
  const token = typeof credential === "string" ? credential : "";
  const c = validToken(token) ? claims(token) : null;
  const now = Date.now() / 1000;
  if (!c?.email || !c.exp || c.exp <= now || c.aud !== clientId || !/^(https:\/\/)?accounts\.google\.com$/.test(c.iss ?? "")) {
    return Response.json({ code: "invalid_input", message: "not a current Google sign-in for this site" }, { status: 400 });
  }
  return Response.json(
    { email: c.email, exp: c.exp },
    { headers: { "Set-Cookie": userSetCookie(isHttps(request), token, c.exp - now), "Cache-Control": "no-store" } },
  );
}

export async function DELETE(request: Request) {
  return new Response(null, { status: 204, headers: { "Set-Cookie": userClearCookie(isHttps(request)) } });
}

// Behind Cloud Run the request URL's host is the container's; the public
// one is in X-Forwarded-Host.
function forwardedOrigin(request: Request): string | null {
  const host = request.headers.get("x-forwarded-host");
  if (!host) return null;
  const proto = (request.headers.get("x-forwarded-proto") ?? "https").split(",")[0].trim();
  return `${proto}://${host.split(",")[0].trim()}`;
}
