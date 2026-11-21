// D320: the frontend's map of revoked and modified rulings mirrors the
// backend's committed supersession.json (earliest edge per ruling), and a
// ruling link says "superseded by …", never as support for a case dated
// after the change (D370: on the change's own date it still counts).
import "./render-support.mjs";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

const { superseded, supersededAsOf, proposed, proposalFor } = await import("../src/lib/supersession.ts");
const map = JSON.parse(readFileSync(fileURLToPath(new URL("../../backend/internal/domain/supersession.json", import.meta.url)), "utf8"));

test("the frontend map is the backend's, earliest edge per ruling", () => {
  const want = {};
  for (const e of map.edges) {
    if (!want[e.ruling] || e.date < want[e.ruling].date) {
      want[e.ruling] = { by: e.superseded_by, kind: e.kind, date: e.date };
      if (e.bulletin) want[e.ruling].bulletin = cite(e.bulletin);
    }
  }
  assert.deepEqual(superseded, want);
  assert.equal(Object.keys(superseded).length, map.counts.superseded_rulings);
});

function cite(b) {
  return `Customs Bulletin vol. ${b.volume} no. ${b.number}${b.page ? `, p. ${b.page}` : ""}`;
}

// D400: proposals CBP published and has not made final are mirrored too,
// earliest per ruling, and only ever shown.
test("the frontend's proposals are the backend's", () => {
  const want = {};
  for (const p of map.proposed ?? []) {
    if (!want[p.ruling] || p.bulletin.date < want[p.ruling].date) want[p.ruling] = { by: p.proposed_by, kind: p.kind, bulletin: cite(p.bulletin), date: p.bulletin.date };
  }
  for (const k of Object.keys(want)) delete want[k].date;
  assert.deepEqual(proposed, want);
  assert.equal(proposalFor("N357771"), undefined);
});

test("a bulletin change cites the bulletin; a proposal is shown and still counted", async () => {
  const { renderToStaticMarkup } = await import("react-dom/server");
  const React = await import("react");
  const { RulingLink } = await import("../src/components/op/ui.tsx");
  const olaf = renderToStaticMarkup(React.createElement(RulingLink, { id: "N325599", asOf: "2024-07-23", support: true }));
  assert.match(olaf, /superseded by H325364/);
  assert.match(olaf, /Customs Bulletin vol\. 58 no\. 20/);
  assert.match(olaf, /not counted as support/);
  const prop = renderToStaticMarkup(React.createElement(RulingLink, { id: "080651", asOf: "2026-11-01", support: true }));
  assert.match(prop, /CBP proposed to revoke it in H346396/);
  assert.doesNotMatch(prop, /not counted|superseded/);
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
