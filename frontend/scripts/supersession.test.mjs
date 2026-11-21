// D320: the frontend's map of revoked and modified rulings mirrors the
// backend's committed supersession.json (earliest edge per ruling), and a
// ruling link says "superseded by …", never as support for a case dated
// after the change (D370: on the change's own date it still counts).
import "./render-support.mjs";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

const { superseded, supersededAsOf } = await import("../src/lib/supersession.ts");
const map = JSON.parse(readFileSync(fileURLToPath(new URL("../../backend/internal/domain/supersession.json", import.meta.url)), "utf8"));

test("the frontend map is the backend's, earliest edge per ruling", () => {
  const want = {};
  for (const e of map.edges) {
    if (!want[e.ruling] || e.date < want[e.ruling].date) want[e.ruling] = { by: e.superseded_by, kind: e.kind, date: e.date };
  }
  assert.deepEqual(superseded, want);
  assert.equal(Object.keys(superseded).length, map.counts.superseded_rulings);
});

test("a change on or after the case date is shown but does not bar the ruling", () => {
  const [id, s] = Object.entries(superseded)[0];
  assert.equal(supersededAsOf(id, s.date).inForce, false, "on the change's own date it was still CBP's position (D370)");
  assert.equal(supersededAsOf(id, `${s.date}T00:00:00Z`).inForce, false);
  const next = new Date(Date.parse(s.date) + 86400000).toISOString().slice(0, 10);
  assert.equal(supersededAsOf(id, next).inForce, true);
  const before = `${Number(s.date.slice(0, 4)) - 1}${s.date.slice(4)}`;
  assert.equal(supersededAsOf(id, before).inForce, false);
  assert.equal(supersededAsOf(id).inForce, true);
  assert.equal(supersededAsOf("N357771"), undefined);
});

test("a ruling link names the superseding ruling and, as support, says it is not counted", async () => {
  const { renderToStaticMarkup } = await import("react-dom/server");
  const React = await import("react");
  const { RulingLink } = await import("../src/components/op/ui.tsx");
  const [id, s] = Object.entries(superseded)[0];
  const next = new Date(Date.parse(s.date) + 86400000).toISOString().slice(0, 10);
  const html = renderToStaticMarkup(React.createElement(RulingLink, { id, asOf: next, support: true }));
  assert.match(html, new RegExp(`superseded by ${s.by}`));
  assert.match(html, /not counted as support/);
  const plain = renderToStaticMarkup(React.createElement(RulingLink, { id: "N357771" }));
  assert.doesNotMatch(plain, /superseded/);
});

// #527: case H325607 is the letter that revoked N331974. On its own date the
// link is marked superseded but not "not counted"; a day later it is.
test("the revoking case itself still counts the ruling it revokes", async () => {
  const { renderToStaticMarkup } = await import("react-dom/server");
  const React = await import("react");
  const { RulingLink } = await import("../src/components/op/ui.tsx");
  assert.equal(supersededAsOf("N331974", "2025-08-29").inForce, false);
  assert.equal(supersededAsOf("N331974", "2025-08-30").inForce, true);
  const same = renderToStaticMarkup(React.createElement(RulingLink, { id: "N331974", asOf: "2025-08-29", support: true }));
  assert.match(same, /superseded by H325607/);
  assert.doesNotMatch(same, /not counted as support/);
});
