import assert from "node:assert/strict";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

// Pinned-demo Ask what-if cards and steps survive a full reload (#112 residual).
// "Reload" = same per-tab sessionStorage stand-in, fresh props/modules state read from it.
const mem = new Map();
const writes = [];
globalThis.sessionStorage = {
  getItem: (k) => mem.get(k) ?? null,
  setItem: (k, v) => (writes.push(k), mem.set(k, String(v))),
  removeItem: (k) => mem.delete(k),
};
const { AskDrawer } = await import("../src/components/op/ask-drawer.tsx");
const { markOwned, clearOwned, askRequest } = await import("../src/lib/ask-history.ts");
const { saveRich, readRich, clearRich, commitPinnedAnswer, RICH_MAX_WHATIFS, RICH_MAX_STEPS } = await import("../src/lib/ask-rich.ts");

const wi = (o = {}) => ({
  change: "Motor made in Mexico",
  origin: "Mexico",
  confidence: 0.91,
  decisive_step: "Motor is the essential character",
  facts: "A drone with a motor made in Mexico.\n",
  regime: "substantial_transformation",
  specialist: "ok",
  scope: "confirmed",
  route: "auto",
  triage_reason: "above the bar",
  ...o,
});
const steps = [{ tool: "get_case" }, { tool: "what_if", arg: "Motor made in Mexico" }];
const turn = (n, extra = {}) => ({ question: `Visitor ${n}?`, answer: "Visitor answer.", cited_rulings: [], at: `2026-11-20T00:00:0${n}.000Z`, ...extra });
const seeded = { question: "Seeded?", answer: "Seeded answer.", cited_rulings: [], at: "2026-01-01T00:00:00.000Z" };
const caseOf = (id, chat, pinned = true) => ({ id, pinned, date: "2026-01-01", chat });
const render = (c) => text(html(AskDrawer, { c, onClose() {}, onChat() {} }));
const reload = (chat) => JSON.parse(JSON.stringify(chat)); // use-case's JSON copy of the chat

test("round trip: every card field and the steps come back after reload", () => {
  mem.clear();
  const t = turn(1);
  markOwned("demo", t);
  saveRich("demo", t, { what_ifs: [wi(), wi({ regime: "textile_102_21", route: "needs_review", provisional: "general answer after specialist failure", scope: "configured_failed", specialist: "failed" })], steps });
  const r = readRich("demo", reload([t])[0]);
  assert.deepEqual(r.whatIfs, [wi(), wi({ regime: "textile_102_21", route: "needs_review", provisional: "general answer after specialist failure", scope: "configured_failed", specialist: "failed" })]);
  assert.deepEqual(r.steps, steps);
  assert.ok(!r.lost);
});

test("real drawer renders the restored cards, Add as a scenario and steps", () => {
  mem.clear();
  const t = turn(1);
  markOwned("demo", t);
  saveRich("demo", t, { what_ifs: [wi()], steps });
  const out = render(caseOf("demo", reload([seeded, t])));
  assert.match(out, /What if · Motor made in Mexico/);
  assert.match(out, /Add as a scenario/);
  assert.match(out, /How it answered · 2 steps/);
  assert.match(out, /confidence/);
});

test("provisional / needs-review card keeps its truthful state after reload", () => {
  mem.clear();
  const t = turn(1);
  markOwned("demo", t);
  saveRich("demo", t, { what_ifs: [wi({ route: "needs_review", provisional: "stand-in answer" })], steps: [] });
  const out = render(caseOf("demo", [t]));
  assert.match(out, /Provisional answer/);
  assert.match(out, /stand-in answer/i);
});

test("old turn without rich fields, normal case and no cards render plain", () => {
  mem.clear();
  const t = turn(1);
  markOwned("demo", t);
  const out = render(caseOf("demo", [seeded, t]));
  assert.doesNotMatch(out, /What if ·|How it answered|weren.t kept/);
  assert.match(out, /Visitor answer\./);
  saveRich("own", t, { what_ifs: [wi()], steps });
  markOwned("own", t);
  assert.doesNotMatch(render(caseOf("own", [t], false)), /What if ·/); // a normal case reads the server only
});

test("ownership mismatch: stored cards for a turn the visitor does not own are ignored", () => {
  mem.clear();
  const t = turn(1);
  saveRich("demo", t, { what_ifs: [wi()], steps });
  assert.doesNotMatch(render(caseOf("demo", [t])), /What if ·/);
  markOwned("demo", turn(2)); // some other turn owned
  assert.doesNotMatch(render(caseOf("demo", [t])), /What if ·/);
  assert.equal(readRich("demo", { ...t, at: "2030-01-01T00:00:00.000Z" }), null);
});

test("two cases are isolated", () => {
  mem.clear();
  const t = turn(1);
  markOwned("demo-a", t);
  saveRich("demo-a", t, { what_ifs: [wi()], steps });
  markOwned("demo-b", t);
  assert.doesNotMatch(render(caseOf("demo-b", [t])), /What if ·/);
  assert.match(render(caseOf("demo-a", [t])), /What if ·/);
});

test("only the last 6 turns keep cards; older entries are pruned from storage", () => {
  mem.clear();
  const ts = [1, 2, 3, 4, 5, 6, 7, 8].map((n) => turn(n));
  for (const t of ts) {
    markOwned("demo", t);
    saveRich("demo", t, { what_ifs: [wi({ change: `c${t.question}` })], steps });
  }
  assert.equal(Object.keys(JSON.parse(mem.get("op:ask-rich:demo")).turns).length, 6);
  assert.equal(readRich("demo", ts[0]), null);
  assert.ok(readRich("demo", ts[7]));
});

test("Reset clears the stored cards", () => {
  mem.clear();
  const t = turn(1);
  markOwned("demo", t);
  saveRich("demo", t, { what_ifs: [wi()], steps });
  clearRich("demo");
  clearOwned("demo");
  assert.equal(mem.get("op:ask-rich:demo"), undefined);
  assert.doesNotMatch(render(caseOf("demo", [t])), /What if ·/);
});

test("stored shape is bounded: counts, string lengths and unknown fields", () => {
  mem.clear();
  const t = turn(1);
  markOwned("demo", t);
  const many = Array.from({ length: RICH_MAX_WHATIFS + 4 }, (_, i) => wi({ change: `c${i}`, raw_provider: { big: "x".repeat(1000) }, trace: "secret" }));
  const manySteps = Array.from({ length: RICH_MAX_STEPS + 9 }, () => ({ tool: "get_case", extra: "x" }));
  saveRich("demo", t, { what_ifs: many, steps: manySteps });
  const r = readRich("demo", t);
  assert.equal(r.whatIfs.length, RICH_MAX_WHATIFS);
  assert.equal(r.steps.length, RICH_MAX_STEPS);
  assert.ok(!("raw_provider" in r.whatIfs[0]) && !("trace" in r.whatIfs[0]) && !("extra" in r.steps[0]));
  assert.ok(!mem.get("op:ask-rich:demo").includes("secret"));
});

test("oversized turn is not stored as a partial card: it reads as lost, with a truthful note", () => {
  mem.clear();
  const t = turn(1);
  markOwned("demo", t);
  saveRich("demo", t, { what_ifs: Array.from({ length: 5 }, () => wi({ facts: "F".repeat(60000) })), steps });
  const r = readRich("demo", t);
  assert.equal(r.lost, true);
  assert.equal(r.whatIfs.length, 0);
  const out = render(caseOf("demo", [t]));
  assert.match(out, /What-if cards for this answer weren.t kept/);
  assert.doesNotMatch(out, /What if ·/);
});

test("invalid schema fails soft: bad version, bad JSON, bad entry, bad card", () => {
  const t = turn(1);
  const mark = `${t.at}|${t.question}`;
  for (const bad of ["{not json", '{"v":99,"turns":{}}', "[]", "null", '"x"']) {
    mem.clear();
    markOwned("demo", t);
    mem.set("op:ask-rich:demo", bad);
    assert.equal(readRich("demo", t), null, bad);
    assert.doesNotThrow(() => render(caseOf("demo", [t])));
  }
  mem.clear();
  markOwned("demo", t);
  mem.set("op:ask-rich:demo", JSON.stringify({ v: 1, turns: { [mark]: "garbage" } }));
  assert.equal(readRich("demo", t).lost, true);
  mem.set("op:ask-rich:demo", JSON.stringify({ v: 1, turns: { [mark]: { whatIfs: [{ origin: 5, confidence: "high" }, wi()], steps: [{ tool: 3 }] } } }));
  const r = readRich("demo", t);
  assert.equal(r.lost, true); // an invalid card is dropped, never shown as a verified one
  assert.deepEqual(r.whatIfs, [wi()]);
  assert.deepEqual(r.steps, []);
  assert.match(render(caseOf("demo", [t])), /weren.t kept/);
});

test("storage failures never throw", () => {
  const boom = () => {
    throw new Error("denied");
  };
  const denied = { getItem: boom, setItem: boom, removeItem: boom };
  const t = turn(1);
  assert.doesNotThrow(() => saveRich("demo", t, { what_ifs: [wi()], steps }, denied));
  assert.equal(readRich("demo", t, denied), null);
  assert.doesNotThrow(() => clearRich("demo", denied));
  assert.doesNotThrow(() => saveRich("demo", t, { what_ifs: [wi()], steps }, null));
});

test("rich fields are never part of the follow-up request context", () => {
  mem.clear();
  const t = turn(1);
  markOwned("demo", t);
  saveRich("demo", t, { what_ifs: [wi()], steps });
  const { body } = askRequest("demo", true, reload([seeded, t]), "next?");
  assert.deepEqual(body, { question: "next?", history: [{ question: "Visitor 1?", answer: "Visitor answer." }] });
});

test("saving cards writes only the per-tab session keys (no network, no other store)", () => {
  mem.clear();
  writes.length = 0;
  const t = turn(1);
  saveRich("demo", t, { what_ifs: [wi()], steps });
  assert.deepEqual([...new Set(writes)], ["op:ask-rich:demo"]);
});

test("the pinned submit path: commitPinnedAnswer adds the turn, marks it and stores the cards; a reload restores them", () => {
  mem.clear();
  writes.length = 0;
  const reply = { answer: "Moving the motor changes the origin.", cited_rulings: ["N123456"], what_ifs: [wi()], steps, model_calls: 3 };
  const next = commitPinnedAnswer("demo", [seeded], "What if?", reply, "2026-11-20T01:00:00.000Z");
  assert.equal(next.length, 2);
  assert.deepEqual(next[1], { question: "What if?", answer: reply.answer, cited_rulings: ["N123456"], at: "2026-11-20T01:00:00.000Z" }); // the saved chat stays plain
  assert.ok(!JSON.stringify(next).includes("what_ifs"));
  const out = render(caseOf("demo", reload(next)));
  assert.match(out, /What if · Motor made in Mexico/);
  assert.match(out, /Add as a scenario/);
  assert.deepEqual(askRequest("demo", true, reload(next), "n?").body.history, [{ question: "What if?", answer: reply.answer }]);
  assert.ok(writes.every((k) => k.startsWith("op:ask-")), "only per-tab ask keys are written");
});

test("a reply without cards stores nothing and leaves no lost note", () => {
  mem.clear();
  const next = commitPinnedAnswer("demo", [], "Why?", { answer: "Because." }, "2026-11-20T01:00:00.000Z");
  assert.equal(mem.get("op:ask-rich:demo"), undefined);
  assert.doesNotMatch(render(caseOf("demo", reload(next))), /weren.t kept/);
});
