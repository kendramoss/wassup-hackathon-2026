import { userClearCookie, userFromCookieHeader } from "./user";
import { isHttps, newVisitorId, visitorFromCookieHeader, visitorSetCookie } from "./visitor";

// Server-only helpers for calling the Go backend from route handlers.

export class BackendError extends Error {
  constructor(
    message: string,
    public status: number,
    public retryAfter?: string | null,
  ) {
    super(message);
  }
}

// Cloud Run appends the browser to X-Forwarded-For, so the rightmost entry is
// the visitor. Without it every caller shares one rate-limit bucket (D45).
export function clientIP(request: Request): string | undefined {
  const parts = (request.headers.get("x-forwarded-for") ?? "")
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
  return parts.at(-1);
}

// Proxy failure logging (#285). The console line carries FIXED words and
// numbers only: a kind, the route PATTERN (dynamic segments are replaced, so a
// case id, a member email or a supplier name in the URL never reaches the
// log), the status, one of the backend's closed error codes, the error CLASS
// and the backend's own normalised request ref. It never carries the response
// body, the error object or message, the query or a header, because any of
// them can echo a user's BOM, an email or a token. A value outside its closed
// set is replaced by a constant, never printed.
const ROUTE_WORDS = new Set([
  "v1", "api", "cases", "predict", "batch", "products", "import", "extract", "money-prefill", "hts-suggest", "parts-hts-suggest", "cost-estimate",
  "family-check", "bom-version", "scenarios", "compare-targets", "ask", "economics", "pin", "watch", "run", "plan", "estimate",
  "export", "answers", "rulings", "eval", "summary", "duty-rates", "demos", "workspaces", "mine", "members", "join", "role",
  "admin", "api-keys", "audit", "verify", "health", "mcp", "review", "reviews", "decision", "rcep", "check", "jurisdictions", "rules", "canada-marking",
  "usmca", "eu-origin", "uk-origin",
]);
const ERROR_CODES = new Set([
  "budget_reached", "conflict", "forbidden", "internal", "invalid_input", "not_found", "quota_exceeded", "rate_limited",
  "unauthenticated", "unauthorized", "audit_unavailable", "model_bad_response", "model_unavailable", "backend_unreachable",
  "link_expired", "link_revoked", "review_not_found", "auth_rate_limited",
]);
const ERROR_CLASSES = new Set(["AbortError", "TimeoutError", "TypeError", "SyntaxError", "RangeError", "BackendError", "Error"]);

// routeOf turns a URL path into its pattern: a known static word stays, any
// other segment (an id, an email, a name) becomes "{}". The query is dropped.
export function routeOf(path: string): string {
  const bare = path.split(/[?#]/, 1)[0];
  const segs = bare.split("/").filter(Boolean).map((seg) => (ROUTE_WORDS.has(seg) ? seg : "{}"));
  return "/" + segs.join("/");
}

// errorClass names the kind of a thrown value from a closed set; the message
// and any other property are never read.
export function errorClass(err: unknown): string {
  const name = err instanceof BackendError ? "BackendError" : err instanceof Error ? err.name : "";
  return ERROR_CLASSES.has(name) ? name : "unclassified";
}

// logProxyFailure writes one fixed-shape console.error line.
export function logProxyFailure(kind: "backend_status" | "unreachable" | "proxy_error", info: { path?: string; status?: number; body?: string; err?: unknown; requestId?: string | null }): void {
  const parts = [`kind=${kind}`, `route=${info.path === undefined ? "-" : routeOf(info.path)}`];
  const status = info.status;
  if (typeof status === "number" && Number.isInteger(status) && status >= 100 && status <= 599) parts.push(`status=${status}`);
  if (info.body !== undefined) {
    let code = "";
    try {
      const c: unknown = JSON.parse(info.body)?.code;
      if (typeof c === "string") code = c;
    } catch {
      /* not JSON */
    }
    parts.push(`code=${ERROR_CODES.has(code) ? code : "other"}`);
  }
  if (info.err !== undefined) parts.push(`class=${errorClass(info.err)}`);
  if (info.requestId && /^(h_)?[0-9a-f]{16}$/.test(info.requestId)) parts.push(`request_id=${info.requestId}`);
  console.error(`proxy failure ${parts.join(" ")}`);
}

// Backend statuses that describe the user's request; their message is shown
// as-is. Anything else (auth failures, crashes) is our problem, not theirs.
const userErrors = new Set([400, 403, 409, 413, 429]);

// Calls path on the backend. method defaults to POST.
// On Cloud Run (K_SERVICE is set) the backend is private, so this
// authenticates with an ID token for this service's identity; locally it
// calls the backend as-is.
//
// The token's audience must be the backend's service URL. BACKEND_AUDIENCE
// overrides it when BACKEND_URL is a revision tag URL (previews), which Cloud
// Run doesn't accept as an audience.
export async function callBackend(
  path: string,
  body: string,
  signal?: AbortSignal,
  options?: { method?: string; request?: Request },
): Promise<Response> {
  const backendUrl = process.env.BACKEND_URL?.replace(/\/$/, "");
  if (!backendUrl) {
    throw new BackendError("BACKEND_URL is not set", 500);
  }

  const method = options?.method ?? "POST";
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  const ip = options?.request ? clientIP(options.request) : undefined;
  if (ip) headers["X-Client-IP"] = ip;
  const auth = await serviceAuth(backendUrl);
  if (auth) headers.Authorization = auth;

  const res = await fetch(`${backendUrl}${path}`, {
    method,
    headers,
    body: method === "GET" || method === "HEAD" ? undefined : body,
    signal,
  });
  if (!res.ok) {
    const text = (await res.text()).trim();
    if (userErrors.has(res.status)) {
      throw new BackendError(text, res.status, res.headers.get("retry-after"));
    }
    logProxyFailure("backend_status", { path, status: res.status, body: text, requestId: res.headers.get("x-request-id") });
    throw new BackendError("The agent is unavailable right now. Try again in a moment.", 502);
  }
  return res;
}

// Turns any error from callBackend into a JSON error response.
export function errorResponse(err: unknown): Response {
  if (err instanceof BackendError) {
    const headers = new Headers();
    if (err.retryAfter) headers.set("Retry-After", err.retryAfter);
    // 429 keeps the API body, including details.retry_after_s.
    if (err.status === 429) {
      try {
        return Response.json(JSON.parse(err.message), { status: 429, headers });
      } catch {
        /* not JSON; fall through with the text */
      }
    }
    return Response.json({ error: err.message }, { status: err.status, headers });
  }
  logProxyFailure("proxy_error", { err });
  return Response.json({ error: "Could not reach the Unde API. Try again in a moment." }, { status: 502 });
}

// The Authorization header for a backend call. On Cloud Run (K_SERVICE
// set): an ID token for this service's identity, as before. In local dev
// only, when BACKEND_ID_TOKEN_CMD is set (for example
// "gcloud auth print-identity-token"): the token that command prints, so a
// local frontend can reach the private Cloud Run backend (#944). Never in a
// production build. The token is kept 45 minutes (Google ID tokens last 60).
let devToken: { value: string; at: number } | null = null;
export async function serviceAuth(backendUrl: string): Promise<string | undefined> {
  if (process.env.K_SERVICE) return `Bearer ${await idToken(process.env.BACKEND_AUDIENCE || backendUrl)}`;
  const cmd = process.env.BACKEND_ID_TOKEN_CMD;
  if (!cmd || process.env.NODE_ENV === "production") return undefined;
  if (!devToken || Date.now() - devToken.at > 45 * 60 * 1000) {
    const { execSync } = await import("node:child_process");
    const value = execSync(cmd, { encoding: "utf8", timeout: 20_000, stdio: ["ignore", "pipe", "ignore"] }).trim();
    devToken = { value, at: Date.now() };
  }
  return `Bearer ${devToken.value}`;
}

// Fetches a Google-signed ID token for audience from the metadata server.
export async function idToken(audience: string): Promise<string> {
  const res = await fetch(
    "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/identity?audience=" +
      encodeURIComponent(audience),
    { headers: { "Metadata-Flavor": "Google" }, cache: "no-store" },
  );
  if (!res.ok) {
    throw new Error(`metadata server returned ${res.status}`);
  }
  return res.text();
}

// MAX_BODY caps a proxied request body (security review L1, D58). The
// largest legitimate body is an image or scanned-PDF import
// (/v1/products/extract-media), which the backend caps at 8 MB of files plus
// 64 KB of form overhead; BOM and portfolio uploads are capped at 5 MB. This
// leaves room for multipart boundaries. Without it the proxy buffered
// anything up to Cloud Run's 32 MiB into memory.
export const MAX_BODY = 9 << 20;

// readCapped reads a request body, or returns null once it passes max bytes
// (a declared Content-Length is checked first; a chunked body is counted).
export async function readCapped(request: Request, max = MAX_BODY): Promise<ArrayBuffer | null> {
  const declared = Number(request.headers.get("content-length"));
  if (declared > max) return null;
  if (!request.body) return new ArrayBuffer(0);
  const reader = request.body.getReader();
  const chunks: Uint8Array[] = [];
  let size = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    size += value.byteLength;
    if (size > max) {
      await reader.cancel();
      return null;
    }
    chunks.push(value);
  }
  const out = new Uint8Array(size);
  let at = 0;
  for (const c of chunks) {
    out.set(c, at);
    at += c.byteLength;
  }
  return out.buffer;
}

// Forwards a browser request to the backend's /v1 API unchanged (method,
// body, content type, Accept) and returns the backend's response as-is,
// errors included, so the client sees the API's {code, message} bodies and
// server-sent events stream through.
// Sends the visitor ID (lib/visitor.ts) as X-Visitor-ID. A page load has
// usually set the cookie already (src/proxy.ts); a first request straight to
// the API gets one here. A signed-in user's Google ID token (lib/user.ts)
// goes as X-User-Token beside it (D68), so their anonymous cases still show.
// A broker's X-API-Key (D69) is passed through unchanged, so brokers reach
// the private backend through this public proxy; the backend checks the key.
// A broker review page's X-Review-Token passes through the same way; like
// every header it is never logged.
// The backend answers an expired or invalid token with 401
// "unauthenticated"; then the cookie is cleared and the next request is
// anonymous.
// Per-user answers (a case's can_write, a workspace role) must never be reused
// for another identity by a browser, CDN or other shared cache (#283). Every
// non-streaming response from the proxy, errors included, is private and
// uncacheable. SSE keeps its own streaming headers below.
const PRIVATE = "private, no-store";
const noStore = (init?: ResponseInit): ResponseInit => ({ ...init, headers: { ...(init?.headers as Record<string, string> | undefined), "Cache-Control": PRIVATE } });

export async function proxyBackend(request: Request, path: string): Promise<Response> {
  const cookies = request.headers.get("cookie");
  const known = visitorFromCookieHeader(cookies);
  const id = known ?? newVisitorId();
  const user = userFromCookieHeader(cookies);
  let res = await forward(request, path, id, user);
  if (!known) res.headers.append("Set-Cookie", visitorSetCookie(request, id));
  // #786: under STRICT_AUTH an anonymous visitor's case list is a 401, which
  // the browser logs as a failed resource on / and /cases. The page asks for
  // the expected anonymous answer (X-Signin-Optional), so a caller with no
  // sign-in and no API key gets it as 200 with the error beside an empty
  // list; the page raises the same 401 itself. Everyone else gets the 401.
  if (res.status === 401 && !user && !request.headers.get("x-api-key") && request.headers.get("x-signin-optional") && request.method === "GET" && path === "cases") {
    const text = await res.text();
    let body: { code?: unknown } | null = null;
    try {
      body = JSON.parse(text);
    } catch {
      /* not the API's error body */
    }
    const headers = new Headers(res.headers);
    if (body?.code === "unauthenticated") {
      headers.set("Content-Type", "application/json");
      res = new Response(JSON.stringify({ cases: [], signin_required: body }), { status: 200, headers });
    } else {
      res = new Response(text, { status: 401, headers });
    }
  }
  if (user && res.status === 401) {
    const text = await res.text();
    const headers = new Headers(res.headers);
    try {
      if (JSON.parse(text).code === "unauthenticated") headers.append("Set-Cookie", userClearCookie(isHttps(request)));
    } catch {
      /* not the API's error body */
    }
    res = new Response(text, { status: 401, headers });
  }
  if (!res.headers.get("content-type")?.startsWith("text/event-stream")) res.headers.set("Cache-Control", PRIVATE);
  return res;
}

// LIMIT_HEADERS are the backend's limit headers a caller needs to pace
// itself (D137): Retry-After and the IETF draft RateLimit fields. They pass
// through the proxy; no other backend header does.
const LIMIT_HEADERS = ["Retry-After", "RateLimit-Limit", "RateLimit-Remaining", "RateLimit-Reset"];

function copyLimitHeaders(from: Headers, to: Headers) {
  for (const h of LIMIT_HEADERS) {
    const v = from.get(h);
    if (v) to.set(h, v);
  }
}

async function forward(request: Request, path: string, visitorId: string, userToken?: string): Promise<Response> {
  const backendUrl = process.env.BACKEND_URL?.replace(/\/$/, "");
  if (!backendUrl) {
    return Response.json({ code: "internal", message: "BACKEND_URL is not set" }, noStore({ status: 500 }));
  }
  const headers: Record<string, string> = {};
  for (const h of ["content-type", "accept", "x-api-key", "x-review-token"]) {
    const v = request.headers.get(h);
    if (v) headers[h] = v;
  }
  const ip = clientIP(request);
  if (ip) headers["X-Client-IP"] = ip;
  headers["X-Visitor-ID"] = visitorId;
  if (userToken) headers["X-User-Token"] = userToken;
  const auth = await serviceAuth(backendUrl);
  if (auth) headers.Authorization = auth;
  const search = new URL(request.url).search;
  const method = request.method.toUpperCase();
  let body: ArrayBuffer | undefined;
  if (method !== "GET" && method !== "HEAD") {
    const b = await readCapped(request);
    if (b === null) {
      return Response.json({ code: "invalid_input", message: `the upload is larger than ${MAX_BODY >> 20} MB` }, noStore({ status: 413 }));
    }
    body = b;
  }
  try {
    const res = await fetch(`${backendUrl}/v1/${path}${search}`, {
      method,
      headers,
      body,
      signal: request.signal,
      cache: "no-store",
    });
    // DELETE answers 204 with no body. Parsing it, or inventing a JSON
    // content type, turns a success into a client error.
    if (res.status === 204 || res.status === 205) {
      const empty = new Headers({ "Cache-Control": PRIVATE });
      copyLimitHeaders(res.headers, empty);
      return new Response(null, { status: res.status, headers: empty });
    }
    const out = new Headers({ "Content-Type": res.headers.get("content-type") ?? "application/json", "Cache-Control": PRIVATE });
    copyLimitHeaders(res.headers, out);
    if (out.get("Content-Type")?.startsWith("text/event-stream")) {
      // no-transform keeps Next's compression from buffering the stream.
      out.set("Cache-Control", "no-cache, no-transform");
      out.set("X-Accel-Buffering", "no");
    }
    return new Response(res.status === 204 ? null : res.body, { status: res.status, headers: out });
  } catch (err) {
    if (request.signal.aborted) return new Response(null, { status: 499 });
    logProxyFailure("unreachable", { path: `/v1/${path}`, err });
    return Response.json(
      // Not a model failure: the API itself didn't answer, so no "the model
      // didn't answer" prefix on the page.
      { code: "backend_unreachable", message: "Couldn't reach the Unde API. Nothing is lost; try again in a moment." },
      noStore({ status: 502 }),
    );
  }
}
