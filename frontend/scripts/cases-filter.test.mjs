import "./render-support.mjs";
import assert from "node:assert/strict";
import { test } from "node:test";

// #715: the cases-list review status and the one display-only filter.
const F = await import("../src/lib/cases-follow-up.ts");

const NOW = Date.parse("2026-11-23T12:00:00Z");
const HOUR = 3600e3;
const at = (h) => new Date(NOW + h * HOUR).toISOString();
const row = (o = {}) => ({ id: "c1", display_name: "Fan", ...o });

test("the window is one named constant of 72 hours", () => {
  assert.equal(F.FOLLOW_UP_HOURS, 72);
});

test("changes requested needs follow-up whatever else the row says", () => {
  assert.equal(F.needsFollowUp(row({ review_status: "changes_requested" }), NOW), true);
});

test("a pending or viewed link needs follow-up only when it expires within 72 hours", () => {
  for (const s of ["pending", "viewed"]) {
    assert.equal(F.needsFollowUp(row({ review_status: s, review_expires_at: at(72) }), NOW), true, `${s} at exactly 72h`);
    assert.equal(F.needsFollowUp(row({ review_status: s, review_expires_at: at(1) }), NOW), true, `${s} at 1h`);
    assert.equal(F.needsFollowUp(row({ review_status: s, review_expires_at: at(72.001) }), NOW), false, `${s} just past 72h`);
    assert.equal(F.needsFollowUp(row({ review_status: s, review_expires_at: at(0) }), NOW), false, `${s} already expired`);
    assert.equal(F.needsFollowUp(row({ review_status: s, review_expires_at: at(-5) }), NOW), false, `${s} lapsed`);
  }
});

test("approved, file requested and expired are never follow-up on their own", () => {
  for (const s of ["approved", "file_requested", "expired"]) {
    assert.equal(F.needsFollowUp(row({ review_status: s, review_expires_at: at(10) }), NOW), false, s);
    assert.equal(F.needsFollowUp(row({ review_status: s }), NOW), false, s);
  }
});

test("a final ruling alert needs follow-up; a proposal alone does not", () => {
  assert.equal(F.needsFollowUp(row({ ruling_alerts: { final: 1, proposed: 0 } }), NOW), true);
  assert.equal(F.needsFollowUp(row({ ruling_alerts: { final: 0, proposed: 2 } }), NOW), false);
  assert.equal(F.needsFollowUp(row({ ruling_alerts: { final: 0, proposed: 0 } }), NOW), false);
});

test("omitted fields never match, and a bad expiry is ignored", () => {
  assert.equal(F.needsFollowUp(row(), NOW), false);
  assert.equal(F.needsFollowUp(row({ review_status: "pending" }), NOW), false);
  assert.equal(F.needsFollowUp(row({ review_status: "pending", review_expires_at: "soon" }), NOW), false);
  assert.equal(F.needsFollowUp(row({ review_status: "viewed", review_expires_at: "" }), NOW), false);
});

test("an approved case with an alert is still listed because of the alert", () => {
  assert.equal(F.needsFollowUp(row({ review_status: "approved", ruling_alerts: { final: 1, proposed: 0 } }), NOW), true);
});

test("the row reading: status words, expiry only for pending or viewed, nothing when omitted", () => {
  assert.equal(F.reviewRowReading(row()), null);
  assert.deepEqual(F.reviewRowReading(row({ review_status: "changes_requested" })), { label: "△ Changes requested", tone: "ask", expires: undefined });
  const p = F.reviewRowReading(row({ review_status: "pending", review_expires_at: "2026-11-25T09:00:00Z" }));
  assert.equal(p.label, "Not opened yet");
  assert.equal(p.expires, "2026-11-25T09:00:00Z");
  assert.equal(F.reviewRowReading(row({ review_status: "approved", review_expires_at: "2026-11-25T09:00:00Z" })).expires, undefined);
});

test("the filter keeps only the matching rows and keeps their order", () => {
  const rows = [
    row({ id: "a", review_status: "approved" }),
    row({ id: "b", review_status: "changes_requested" }),
    row({ id: "c" }),
    row({ id: "d", ruling_alerts: { final: 1, proposed: 0 } }),
  ];
  assert.deepEqual(F.followUpRows(rows, NOW).map((r) => r.id), ["b", "d"]);
});

test("the list shows the review reading, the filter and its 72-hour note; rows without fields show none", async () => {
  const { html, text } = await import("./render-support.mjs");
  const { YourCases } = await import("../src/components/op/screens/cases-home.tsx");
  const mk = (id, o) => ({ id, display_name: `Case ${id}`, step: "origin", watch_status: null, scenario_count: 0, pinned: false, ...o });
  const t = text(html(YourCases, { cases: [mk("a", { review_status: "changes_requested" }), mk("b", { review_status: "pending", review_expires_at: "2026-12-30T09:00:00Z" }), mk("c", {})], onChange() {} }));
  assert.match(t, /Needs follow-up · \d/);
  assert.match(t, /72 hours/);
  assert.match(t, /Broker: △ Changes requested/);
  assert.match(t, /Broker: Not opened yet · link ends 30 Dec 2026/);
  assert.equal((t.match(/Broker:/g) ?? []).length, 2);
});
