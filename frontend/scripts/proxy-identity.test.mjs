import "./render-support.mjs";
import assert from "node:assert/strict";
import { test } from "node:test";
import { json } from "./lib/hook-runtime.mjs";

// #283: per-identity case permissions are not shared. The real /api proxy
// (proxyBackend) and the real client (api.getCase) are driven with two
// identities asking for the same case URL; the stubbed backend answers
// can_write by the identity it sees. This shows what OUR code does (no Next
// data cache, no memoisation, one upstream call per request, identity headers
// forwarded per request, and no caller authority headers). It does not prove
// anything about a CDN or browser cache in front of the deployed service; the
// proxy sets no Cache-Control on JSON responses, and that absence is recorded
// here as a fact, not as proof.
process.env.BACKEND_URL = "http://backend.test";
delete process.env.K_SERVICE;
const { proxyBackend } = await import("../src/lib/backend.ts");
const { api } = await import("../src/lib/api/client.ts");

const jwt = (n) => `aaa${n}.bbb${n}.ccc${n}`;
const upstream = [];
const realFetch = globalThis.fetch;
function backend() {
  upstream.length = 0;
  globalThis.fetch = async (url, init = {}) => {
    const u = String(url);
    if (!u.startsWith("http://backend.test")) return realFetch(url, init);
    const h = new Headers(init.headers);
    upstream.push({ u, method: init.method, cache: init.cache, headers: Object.fromEntries(h) });
    const who = h.get("x-user-token");
    return json({ id: "c1", can_write: who === jwt("editor") ? true : who === jwt("viewer") ? false : undefined });
  };
}
const req = (token, extra = {}) =>
  new Request("http://front.test/api/v1/cases/c1", { headers: { cookie: `op_user=${token}; op_visitor=v_${token.slice(0, 3)}`, ...extra } });

test("two identities, same case URL: each gets its own can_write; the proxy calls the backend every time", async () => {
  backend();
  const a1 = await proxyBackend(req(jwt("editor")), "cases/c1");
  const b1 = await proxyBackend(req(jwt("viewer")), "cases/c1");
  const a2 = await proxyBackend(req(jwt("editor")), "cases/c1");
  const anon = await proxyBackend(new Request("http://front.test/api/v1/cases/c1"), "cases/c1");
  assert.equal((await a1.json()).can_write, true);
  assert.equal((await b1.json()).can_write, false);
  assert.equal((await a2.json()).can_write, true);
  assert.equal((await anon.json()).can_write, undefined, "no sign-in: nothing inherited from the earlier users");
  assert.equal(upstream.length, 4, "no response was reused between requests");
  assert.deepEqual(upstream.map((x) => x.headers["x-user-token"]), [jwt("editor"), jwt("viewer"), jwt("editor"), undefined]);
  assert.ok(upstream.every((x) => x.cache === "no-store"), "the upstream fetch opts out of Next's data cache");
});

test("the proxy forwards an allowlist: no caller role, workspace or tenant header reaches the backend", async () => {
  backend();
  await proxyBackend(req(jwt("viewer"), { "x-role": "admin", "x-workspace": "ws1", "x-tenant": "t", "x-workspace-id": "ws1", "x-user-role": "admin", "x-user-email": "a@b.c" }), "cases/c1");
  const sent = Object.keys(upstream[0].headers);
  assert.deepEqual(sent.filter((k) => /role|workspace|tenant|email/.test(k)), []);
  assert.ok(sent.every((k) => ["x-visitor-id", "x-user-token", "x-client-ip", "content-type", "accept", "x-api-key", "x-review-token"].includes(k)), sent.join(","));
});

test("a broker review page's X-Review-Token is forwarded as sent, on the bare /v1/review path", async () => {
  backend();
  const tok = "ubr_SYNTHETIC_test_token_0123";
  await proxyBackend(new Request("http://front.test/api/v1/review", { headers: { "x-review-token": tok } }), "review");
  assert.equal(upstream[0].headers["x-review-token"], tok);
  assert.equal(upstream[0].u, "http://backend.test/v1/review", "the token is not in the URL");
});

test("the client reads the case from the server on every call (no-store, no module cache)", async () => {
  const seen = [];
  globalThis.fetch = async (url, init = {}) => {
    seen.push({ url: String(url), cache: init.cache });
    return json({ id: "c1", can_write: seen.length === 1 });
  };
  const first = await api.getCase("c1");
  const second = await api.getCase("c1");
  assert.equal(first.can_write, true);
  assert.equal(second.can_write, false, "the second read is a new request, not the first answer");
  assert.equal(seen.length, 2);
  assert.ok(seen.every((s) => s.cache === "no-store"));
});

test("every non-streaming proxy response is private and uncacheable, errors included; identity answers differ", async () => {
  backend();
  const a = await proxyBackend(req(jwt("editor")), "cases/c1");
  const b = await proxyBackend(req(jwt("viewer")), "cases/c1");
  for (const r of [a, b]) assert.equal(r.headers.get("cache-control"), "private, no-store");
  assert.equal((await a.json()).can_write, true);
  assert.equal((await b.json()).can_write, false, "same URL, different identity, different answer");
  // backend error body
  globalThis.fetch = async () => json({ code: "forbidden", message: "no" }, 403);
  assert.equal((await proxyBackend(req(jwt("viewer")), "cases/c1")).headers.get("cache-control"), "private, no-store");
  // 401 that clears the user cookie
  globalThis.fetch = async () => json({ code: "unauthenticated", message: "x" }, 401);
  const r401 = await proxyBackend(req(jwt("viewer")), "cases/c1");
  assert.equal(r401.headers.get("cache-control"), "private, no-store");
  // 204
  globalThis.fetch = async () => new Response(null, { status: 204 });
  assert.equal((await proxyBackend(new Request("http://front.test/api/v1/cases/c1", { method: "DELETE" }), "cases/c1")).headers.get("cache-control"), "private, no-store");
  // backend unreachable
  globalThis.fetch = async () => {
    throw new TypeError("down");
  };
  const origErr = console.error;
  console.error = () => {};
  const r502 = await proxyBackend(req(jwt("viewer")), "cases/c1");
  console.error = origErr;
  assert.equal(r502.status, 502);
  assert.equal(r502.headers.get("cache-control"), "private, no-store");
});

test("SSE responses keep their streaming headers, not private/no-store", async () => {
  globalThis.fetch = async () => new Response("event: done\ndata: {}\n\n", { status: 200, headers: { "content-type": "text/event-stream" } });
  const r = await proxyBackend(req(jwt("editor"), { accept: "text/event-stream" }), "cases/c1/plan");
  assert.equal(r.headers.get("cache-control"), "no-cache, no-transform");
  assert.equal(r.headers.get("x-accel-buffering"), "no");
  assert.match(r.headers.get("content-type"), /^text\/event-stream/);
  assert.match(await r.text(), /event: done/);
});

test("the proxy passes the backend's limit headers through, and no other backend header", async () => {
  const limited = {
    "Retry-After": "7",
    "RateLimit-Limit": "20",
    "RateLimit-Remaining": "0",
    "RateLimit-Reset": "7",
    "X-Internal": "secret",
  };
  for (const status of [429, 204]) {
    globalThis.fetch = async (url, init = {}) => {
      if (!String(url).startsWith("http://backend.test")) return realFetch(url, init);
      const body = status === 204 ? null : JSON.stringify({ code: "rate_limited", message: "too many requests" });
      return new Response(body, { status, headers: { "Content-Type": "application/json", ...limited } });
    };
    const res = await proxyBackend(new Request("http://front.test/api/v1/cases"), "cases");
    assert.equal(res.status, status);
    for (const h of ["Retry-After", "RateLimit-Limit", "RateLimit-Remaining", "RateLimit-Reset"]) {
      assert.equal(res.headers.get(h), limited[h], `${status} ${h}`);
    }
    assert.equal(res.headers.get("X-Internal"), null);
  }
  globalThis.fetch = realFetch;
});
