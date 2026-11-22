import assert from "node:assert/strict";
import { test } from "node:test";
import { alertTone, badgeText, changeWords, recheckWords, roleWords } from "../src/lib/ruling-alerts.ts";

const fmt = (s) => s;
const revoked = { ruling: "N345891", roles: ["cited", "plan_support"], status: "final", kind: "revoked", by: "H346161", date: "2025-05-05", in_effect: true, note: "revoked by H346161 on 2025-05-05" };
const scheduled = { ruling: "N023537", roles: ["cited"], status: "final", kind: "revoked", by: "H314991", date: "2026-11-29", note: "revoked by H314991 effective 2026-11-29 (Customs Bulletin vol. 60 no. 30, p. 1)" };
const proposed = { ruling: "080651", roles: ["cited"], status: "proposed", kind: "revoked", by: "H346396", bulletin: "Customs Bulletin vol. 60 no. 16, p. 58", note: "" };
const readOnly = { ...revoked, roles: ["read"] };

test("a final change reads as done or scheduled, with the date word of its source", () => {
  assert.equal(changeWords(revoked, fmt), "was revoked by H346161 on 2025-05-05");
  assert.equal(changeWords(scheduled, fmt), "will be revoked by H314991 effective 2026-11-29");
  assert.match(changeWords(proposed, fmt), /proposed to revoke it in H346396/);
});

test("roles in words", () => {
  assert.equal(roleWords(["cited", "plan_support"]), "cited by this answer and supporting a plan");
  assert.equal(roleWords(["read"]), "read by this answer");
});

test("attention only for a final change to a relied-on ruling", () => {
  assert.equal(alertTone({ counts: { final: 1, proposed: 0 }, items: [revoked] }), "attention");
  assert.equal(alertTone({ counts: { final: 0, proposed: 1 }, items: [proposed] }), "calm");
  assert.equal(alertTone({ counts: { final: 0, proposed: 0, read_only: 1 }, items: [readOnly] }), "calm");
  assert.equal(alertTone(undefined), null);
});

test("list badge", () => {
  assert.deepEqual(badgeText({ final: 1, proposed: 1 }), { text: "1 cited ruling revoked or modified", attention: true });
  assert.deepEqual(badgeText({ final: 0, proposed: 2 }), { text: "2 cited rulings: change proposed", attention: false });
  assert.equal(badgeText({ final: 0, proposed: 0 }), null);
  assert.equal(badgeText(null), null);
});

test("D531: the badge counts changes that bear on the answer, and the line says whether a re-check is still recommended", () => {
  assert.deepEqual(badgeText({ final: 2, proposed: 0, touches_answer: 1 }), { text: "2 cited rulings revoked or modified, 1 bearing on the answer", attention: true });
  assert.equal(recheckWords({ relevance: "not_answer" }), "Re-check still recommended.");
  assert.equal(recheckWords({ relevance: "holding_unchanged" }), "Re-check still recommended.");
  assert.equal(recheckWords({ relevance: "touches_answer" }), "Re-check recommended.");
  assert.equal(recheckWords({ relevance: "unknown" }), "Re-check recommended.");
  assert.equal(recheckWords({}), "Re-check recommended.");
});
