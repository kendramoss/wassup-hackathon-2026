import { clientIP, idToken, logProxyFailure, readCapped } from "./backend";

// Proxies the public MCP endpoint (https://undelabs.dev/mcp, docs/MCP.md) to
// the backend's POST /mcp. The backend is private on Cloud Run, so this is
// the only way in, as /api/v1 is for the REST API.
//
// What goes through, and nothing else:
// - the MCP transport headers (content type, accept, protocol version, and
//   the session and last-event ids, which the stateless backend ignores);
// - the caller's API key, from X-API-Key or from "Authorization: Bearer
//   op_..." (the form Claude's connector settings send). Any other
//   Authorization value is dropped: on Cloud Run that header carries this
//   service's identity token to the backend;
// - the caller's address as X-Client-IP, for the anonymous rate limits.
// Cookies, the visitor ID, the user token and the admin token are never
// forwarded; a request with the admin token is refused.
// The response body is passed through as a stream (an MCP answer may be a
// text/event-stream), with the MCP headers and Retry-After.

// MAX_MCP_BODY matches the backend's cap on one MCP request: room for
// extract_product_from_media's 8 MB of files in base64 and the JSON around it.
export const MAX_MCP_BODY = 12 << 20;

const FORWARD = ["content-type", "accept", "mcp-protocol-version", "mcp-session-id", "last-event-id"];
const RETURN = ["content-type", "mcp-session-id", "mcp-protocol-version", "retry-after", "x-request-id", "allow"];

const jsonError = (status: number, code: string, message: string) =>
  Response.json({ code, message }, { status, headers: { "Cache-Control": "no-store" } });

// apiKeyOf returns the caller's API key, "" for none, or null when the two
// headers carry different keys.
export function apiKeyOf(request: Request): string | null {
  const header = request.headers.get("x-api-key")?.trim() ?? "";
  const m = /^bearer\s+(op_\S+)\s*$/i.exec(request.headers.get("authorization") ?? "");
  const bearer = m?.[1] ?? "";
  if (header && bearer && header !== bearer) return null;
  return header || bearer;
}

export async function proxyMcp(request: Request): Promise<Response> {
  const backendUrl = process.env.BACKEND_URL?.replace(/\/$/, "");
  if (!backendUrl) return jsonError(500, "internal", "BACKEND_URL is not set");
  if (request.headers.get("x-admin-token")) return jsonError(403, "forbidden", "the admin token is not accepted on /mcp; use an API key");
  const key = apiKeyOf(request);
  if (key === null) return jsonError(400, "invalid_input", "X-API-Key and the Authorization bearer key differ; send one");

  const headers: Record<string, string> = {};
  for (const h of FORWARD) {
    const v = request.headers.get(h);
    if (v) headers[h] = v;
  }
  if (key) headers["X-API-Key"] = key;
  const ip = clientIP(request);
  if (ip) headers["X-Client-IP"] = ip;
  if (process.env.K_SERVICE) {
    headers.Authorization = `Bearer ${await idToken(process.env.BACKEND_AUDIENCE || backendUrl)}`;
  }
  const body = await readCapped(request, MAX_MCP_BODY);
  if (body === null) return jsonError(413, "invalid_input", "the MCP request is larger than 12 MB");

  try {
    const res = await fetch(`${backendUrl}/mcp`, { method: "POST", headers, body, signal: request.signal, cache: "no-store" });
    const out = new Headers({ "Cache-Control": "no-store, no-transform" });
    for (const h of RETURN) {
      const v = res.headers.get(h);
      if (v) out.set(h, v);
    }
    if (out.get("content-type")?.startsWith("text/event-stream")) out.set("X-Accel-Buffering", "no");
    if (res.status >= 500) logProxyFailure("backend_status", { path: "/mcp", status: res.status, requestId: res.headers.get("x-request-id") });
    return new Response(res.status === 202 || res.status === 204 ? null : res.body, { status: res.status, headers: out });
  } catch (err) {
    if (request.signal.aborted) return new Response(null, { status: 499 });
    logProxyFailure("unreachable", { path: "/mcp", err });
    return jsonError(502, "backend_unreachable", "Couldn't reach the Unde API. Try again in a moment.");
  }
}
