import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

// D97, F2: recording an export can answer 409 conflict. The Draft screen must
// get that error back (to show it and withhold the letter), not swallow it.
// Runs the real api.exportDraft and recordExport against a mocked fetch.
const client = fileURLToPath(new URL("../src/lib/api/client.ts", import.meta.url));
const rec = fileURLToPath(new URL("../src/lib/record-export.ts", import.meta.url));

function run(status, body) {
  const code = `
    import { api } from ${JSON.stringify(client)};
    import { recordExport } from ${JSON.stringify(rec)};
    const seen = [];
    globalThis.fetch = async (url, init) => {
      seen.push({ url, body: JSON.parse(init.body) });
      return new Response(JSON.stringify(${JSON.stringify(body)}), { status: ${status}, headers: { "content-type": "application/json" } });
    };
    const r = await recordExport(() => api.exportDraft("c1", { move_key: "m@th", format: "markdown" }));
    console.log(JSON.stringify({ ok: r.ok, code: r.error?.code, status: r.error?.status, retryable: r.error?.retryable, message: r.error?.message, seen }));
  `;
  const r = spawnSync(process.execPath, ["--experimental-transform-types", "--no-warnings", "--input-type=module", "-e", code], { encoding: "utf8" });
  assert.equal(r.status, 0, r.stderr || r.stdout);
  return JSON.parse(r.stdout);
}

test("a 409 conflict on recording is returned, retryable, with the server message", () => {
  const msg = "conflict: the case changed while this ran; the result was discarded, run it again";
  const r = run(409, { code: "conflict", message: msg });
  assert.equal(r.ok, false);
  assert.equal(r.code, "conflict");
  assert.equal(r.status, 409);
  assert.equal(r.retryable, true);
  assert.equal(r.message, msg);
  // It is the recording call: not a preview, same pick and format.
  assert.equal(r.seen.length, 1);
  assert.match(r.seen[0].url, /\/cases\/c1\/export$/);
  assert.equal(r.seen[0].body.preview, undefined);
  assert.equal(r.seen[0].body.move_key, "m@th");
});

test("controls: a recorded export is ok", () => {
  assert.equal(run(200, { content: "x" }).ok, true);
});

test("controls: other failures to record do not hold back a previewed letter", () => {
  assert.equal(run(500, { code: "internal", message: "x" }).ok, true);
  assert.equal(run(404, { code: "not_found", message: "x" }).ok, true);
  assert.equal(run(400, { code: "invalid_input", message: "x" }).ok, true);
});
