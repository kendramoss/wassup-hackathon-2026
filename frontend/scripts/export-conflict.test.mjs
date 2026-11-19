import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

// D97: recording an export answers with the letter as the case stands now.
// The Draft screen must hold back a letter whose case changed since the
// preview. Runs the real api.exportDraft and recordExport against a mocked
// fetch.
const client = fileURLToPath(new URL("../src/lib/api/client.ts", import.meta.url));
const rec = fileURLToPath(new URL("../src/lib/record-export.ts", import.meta.url));

function run({ status, body, previewed, pick = {}, format = "markdown", network = false }) {
  const code = `
    import { api } from ${JSON.stringify(client)};
    import { recordExport } from ${JSON.stringify(rec)};
    const seen = [];
    globalThis.fetch = async (url, init) => {
      if (${network}) throw new TypeError("fetch failed");
      seen.push({ url, body: JSON.parse(init.body) });
      return new Response(JSON.stringify(${JSON.stringify(body)}), { status: ${status}, headers: { "content-type": "application/json" } });
    };
    const pick = ${JSON.stringify(pick)};
    const r = await recordExport(() => api.exportDraft("c1", { ...pick, format: ${JSON.stringify(format)} }), ${JSON.stringify(previewed)});
    console.log(JSON.stringify({ ok: r.ok, reason: r.reason, code: r.error?.code, status: r.error?.status, retryable: r.error?.retryable, message: r.error?.message, seen }));
  `;
  const r = spawnSync(process.execPath, ["--experimental-transform-types", "--no-warnings", "--input-type=module", "-e", code], { encoding: "utf8" });
  assert.equal(r.status, 0, r.stderr || r.stdout);
  return JSON.parse(r.stdout);
}

const LETTER = "Dear CBP,\n\nThe motor is made in China.\n";

test("same letter recorded: allowed, and it is the non-preview recording call with the same pick and format", () => {
  const r = run({ status: 200, body: { content: LETTER, format: "markdown" }, previewed: LETTER, pick: { move_key: "m@th" } });
  assert.equal(r.ok, true);
  assert.equal(r.seen.length, 1);
  assert.match(r.seen[0].url, /\/cases\/c1\/export$/);
  assert.equal(r.seen[0].body.preview, undefined);
  assert.equal(r.seen[0].body.move_key, "m@th");
  assert.equal(r.seen[0].body.format, "markdown");
});

test("recorded letter differs from the preview (case changed since): held back", () => {
  const r = run({ status: 200, body: { content: "Dear CBP,\n\nThe motor is made in Thailand.\n" }, previewed: LETTER });
  assert.equal(r.ok, false);
  assert.equal(r.reason, "changed");
});

test("same-format comparison: a cfr177 record compared against the cfr177 preview, not the short draft", () => {
  const cfr = "# 19 CFR 177.2 letter\n\n[FILL IN: port]\n";
  assert.equal(run({ status: 200, body: { content: cfr, format: "cfr177" }, previewed: cfr, format: "cfr177" }).ok, true);
  // The short draft as the preview for a cfr177 record is a different letter.
  assert.equal(run({ status: 200, body: { content: cfr }, previewed: LETTER, format: "cfr177" }).reason, "changed");
});

test("requester fields are filled in locally: the raw content is what is compared, so they never cause a false mismatch", () => {
  const raw = "Requester: [REQUESTER NAME]\nSigned: [NAME]\n";
  // The screen text would be "Requester: Acme\n..."; recordExport is given the raw preview.
  assert.equal(run({ status: 200, body: { content: raw }, previewed: raw }).ok, true);
});

test("409 conflict is returned, retryable, with the server message", () => {
  const msg = "conflict: the case changed while this ran; the result was discarded, run it again";
  const r = run({ status: 409, body: { code: "conflict", message: msg }, previewed: LETTER });
  assert.equal(r.ok, false);
  assert.equal(r.reason, "failed");
  assert.equal(r.code, "conflict");
  assert.equal(r.status, 409);
  assert.equal(r.retryable, true);
  assert.equal(r.message, msg);
});

test("chosen plan gone (400 invalid_input with a move or plan selection): held back, not treated as ok", () => {
  const body = { code: "invalid_input", message: 'invalid input: move_key "m@th": run a plan search first' };
  const a = run({ status: 400, body, previewed: LETTER, pick: { move_key: "m@th" } });
  assert.equal(a.ok, false);
  assert.equal(a.reason, "failed");
  assert.equal(a.code, "invalid_input");
  assert.match(a.message, /run a plan search first/, "the server's own message is kept");
  assert.equal(run({ status: 400, body, previewed: LETTER, pick: { plan_index: 0 } }).ok, false);
});

test("fail closed: 500, network, 404 and a 400 with no selection all hold the letter back, with the client's own error", () => {
  for (const [status, code] of [[500, "internal"], [404, "not_found"], [400, "invalid_input"]]) {
    const r = run({ status, body: { code, message: `server said ${status}` }, previewed: LETTER });
    assert.equal(r.ok, false, `${status}`);
    assert.equal(r.reason, "failed");
    assert.equal(r.status, status);
    assert.equal(r.code, code);
    assert.equal(r.message, `server said ${status}`, "the real message is kept");
  }
  const net = run({ status: 200, body: {}, previewed: LETTER, network: true });
  assert.equal(net.ok, false);
  assert.equal(net.reason, "failed");
  assert.equal(net.code, "network");
  assert.equal(net.retryable, true);
});

test("authorization failures are held back as themselves, not relabelled as a conflict", () => {
  for (const [status, code] of [[401, "unauthenticated"], [403, "forbidden"]]) {
    const r = run({ status, body: { code, message: "no" }, previewed: LETTER });
    assert.equal(r.ok, false);
    assert.equal(r.reason, "failed");
    assert.equal(r.code, code);
    assert.notEqual(r.code, "conflict");
    assert.equal(r.retryable, false, "no Retry offered for an authorization failure");
  }
});

test("a success with no letter is not trusted", () => {
  assert.equal(run({ status: 200, body: { case_id: "c1" }, previewed: LETTER }).reason, "malformed");
  assert.equal(run({ status: 200, body: { content: 42 }, previewed: LETTER }).reason, "malformed");
});
