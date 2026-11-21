import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { test } from "node:test";

// D97: a plan, comparison or export refused because the case changed answers
// 409 {code: "conflict"}. The JSON client must mark it retryable, as the
// streamed error event is, and must not widen retry to other 4xx codes.
const client = new URL("../src/lib/api/client.ts", import.meta.url).href;
const registerTs = new URL("./register-ts-loader.mjs", import.meta.url).href;

function errorFor(status, body) {
  const code = `
    import { api } from ${JSON.stringify(client)};
    globalThis.fetch = async () => new Response(JSON.stringify(${JSON.stringify(body)}), { status: ${status}, headers: { "content-type": "application/json" } });
    try { await api.plan("c1", {}); console.log(JSON.stringify({ threw: false })); }
    catch (e) { console.log(JSON.stringify({ threw: true, name: e.name, message: e.message, status: e.status, code: e.code, retryable: e.retryable })); }
  `;
  const r = spawnSync(process.execPath, ["--import", registerTs, "--no-warnings", "--input-type=module", "-e", code], { encoding: "utf8" });
  assert.equal(r.status, 0, r.stderr || r.stdout);
  return JSON.parse(r.stdout);
}

test("409 conflict keeps its message and code and is retryable", () => {
  const msg = "conflict: the case changed while this ran; the result was discarded, run it again";
  const e = errorFor(409, { code: "conflict", message: msg });
  assert.deepEqual(e, { threw: true, name: "Error", message: msg, status: 409, code: "conflict", retryable: true });
});

test("controls: other client errors stay not retryable", () => {
  for (const [status, code] of [[400, "invalid_input"], [404, "not_found"], [403, "forbidden"], [401, "unauthenticated"]]) {
    const e = errorFor(status, { code, message: "x" });
    assert.equal(e.code, code);
    assert.equal(e.retryable, false, `${status} ${code}`);
  }
});

test("controls: existing retryable and non-retryable server codes unchanged", () => {
  assert.equal(errorFor(500, { code: "internal", message: "x" }).retryable, true);
  assert.equal(errorFor(502, { code: "model_unavailable", message: "x" }).retryable, true);
  assert.equal(errorFor(429, { code: "rate_limited", message: "x" }).retryable, true);
  assert.equal(errorFor(503, { code: "budget_reached", message: "x" }).retryable, false);
});

test("409 without a body code is not retried (only the explicit conflict code is)", () => {
  const e = errorFor(409, { message: "Already a member." });
  assert.equal(e.status, 409);
  assert.equal(e.retryable, false);
});
