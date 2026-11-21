import "./render-support.mjs";
import assert from "node:assert/strict";
import { test } from "node:test";

// The public /mcp proxy (src/lib/mcp-proxy.ts): forwards an allowlist of
// headers, maps "Authorization: Bearer op_..." to X-API-Key, refuses the admin
// token, and passes an event stream through as a stream.
process.env.BACKEND_URL = "http://backend.test";
delete process.env.K_SERVICE;
const { proxyMcp, apiKeyOf } = await import("../src/lib/mcp-proxy.ts");

const realFetch = globalThis.fetch;
let sent;
function backend(respond) {
  sent = [];
  globalThis.fetch = async (url, init = {}) => {
    const u = String(url);
    if (!u.startsWith("http://backend.test")) return realFetch(url, init);
    sent.push({ u, method: init.method, cache: init.cache, headers: Object.fromEntries(new Headers(init.headers)), body: init.body ? new TextDecoder().decode(init.body) : "" });
    return respond();
  };
}
const rpc = `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`;
const req = (headers = {}) =>
  new Request("http://front.test/mcp", {
    method: "POST",
    body: rpc,
    headers: { "content-type": "application/json", accept: "application/json, text/event-stream", "x-forwarded-for": "198.51.100.8", ...headers },
  });

test("forwards the MCP headers, the bearer API key and the client address only", async () => {
  backend(() => new Response(`{"jsonrpc":"2.0","id":1,"result":{}}`, { headers: { "content-type": "application/json", "mcp-session-id": "s1", "set-cookie": "x=1" } }));
  const res = await proxyMcp(req({ authorization: "Bearer op_abc", "mcp-protocol-version": "2025-06-18", cookie: "op_visitor=v; op_user=u", "x-user-token": "jwt", "x-visitor-id": "v123", "x-role": "admin" }));
  assert.equal(res.status, 200);
  assert.equal(sent.length, 1);
  assert.equal(sent[0].u, "http://backend.test/mcp");
  assert.equal(sent[0].cache, "no-store");
  assert.equal(sent[0].body, rpc);
  const h = sent[0].headers;
  assert.equal(h["x-api-key"], "op_abc");
  assert.equal(h["x-client-ip"], "198.51.100.8");
  assert.equal(h["mcp-protocol-version"], "2025-06-18");
  assert.deepEqual(Object.keys(h).sort(), ["accept", "content-type", "mcp-protocol-version", "x-api-key", "x-client-ip"]);
  assert.equal(res.headers.get("mcp-session-id"), "s1");
  assert.equal(res.headers.get("set-cookie"), null, "no backend cookie reaches the client");
  assert.match(res.headers.get("cache-control"), /no-store/);
});

test("a non-key bearer (an identity token) is not forwarded", async () => {
  backend(() => new Response("{}", { headers: { "content-type": "application/json" } }));
  await proxyMcp(req({ authorization: "Bearer eyJhbGciOi.payload.sig" }));
  assert.equal(sent[0].headers["x-api-key"], undefined);
  assert.equal(sent[0].headers.authorization, undefined);
});

test("refuses the admin token and two different keys before calling the backend", async () => {
  backend(() => new Response("{}"));
  assert.equal((await proxyMcp(req({ "x-admin-token": "t" }))).status, 403);
  assert.equal((await proxyMcp(req({ "x-api-key": "op_a", authorization: "Bearer op_b" }))).status, 400);
  assert.equal(sent.length, 0);
  assert.equal(apiKeyOf(req({ "x-api-key": "op_a", authorization: "bearer op_a" })), "op_a");
});

test("an event stream is passed through chunk by chunk", async () => {
  let push;
  const stream = new ReadableStream({ start(c) { push = c; } });
  backend(() => new Response(stream, { headers: { "content-type": "text/event-stream" } }));
  const res = await proxyMcp(req());
  assert.equal(res.headers.get("content-type"), "text/event-stream");
  assert.equal(res.headers.get("x-accel-buffering"), "no");
  const reader = res.body.getReader();
  push.enqueue(new TextEncoder().encode("event: message\ndata: 1\n\n"));
  const first = await reader.read();
  assert.equal(new TextDecoder().decode(first.value), "event: message\ndata: 1\n\n", "the first event arrives before the stream ends");
  push.close();
  assert.equal((await reader.read()).done, true);
});

test("a body over 1 MB is refused", async () => {
  backend(() => new Response("{}"));
  const big = new Request("http://front.test/mcp", { method: "POST", body: "x".repeat((1 << 20) + 1), headers: { "content-type": "application/json" } });
  assert.equal((await proxyMcp(big)).status, 413);
  assert.equal(sent.length, 0);
});
