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
  assert.ok(sent.every((k) => ["x-visitor-id", "x-user-token", "x-client-ip", "content-type", "accept", "x-api-key"].includes(k)), sent.join(","));
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

test("recorded fact: the proxy sets no Cache-Control on a JSON response (absence is not proof for a CDN)", async () => {
  backend();
  const res = await proxyBackend(req(jwt("editor")), "cases/c1");
  assert.equal(res.headers.get("cache-control"), null);
  assert.deepEqual([...res.headers.keys()].filter((k) => k !== "set-cookie").sort(), ["content-type"]);
});
