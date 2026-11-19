import assert from "node:assert/strict";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

// The real AskDrawer, rendered to static markup (the repo has no DOM test
// library), over a per-tab sessionStorage stand-in. Ownership of visitor turns
// lives only in that storage, so a "reload" is: same storage, fresh props.
const mem = new Map();
globalThis.sessionStorage = { getItem: (k) => mem.get(k) ?? null, setItem: (k, v) => mem.set(k, String(v)), removeItem: (k) => mem.delete(k) };
const { AskDrawer } = await import("../src/components/op/ask-drawer.tsx");
const { markOwned, clearOwned, askRequest } = await import("../src/lib/ask-history.ts");

const seeded = { question: "Seeded?", answer: "Seeded answer.", cited_rulings: [], at: "2026-01-01T00:00:00.000Z" };
const visitor = (n, answer = "Visitor answer.") => ({ question: `Visitor ${n}?`, answer, cited_rulings: [], at: `2026-11-20T00:00:0${n}.000Z` });
const caseOf = (id, pinned, chat) => ({ id, pinned, date: "2026-01-01", chat });
const render = (c) => text(html(AskDrawer, { c, onClose() {}, onChat() {} }));
const NOTE = /too long to carry into follow-up questions/;

test("reload: restored visitor turns are sent, the seeded turn is not", () => {
  mem.clear();
  const v1 = visitor(1);
  markOwned("demo", v1); // the first session answered and marked it
  // Full reload: the chat comes back from use-case's sessionStorage copy, seeded first.
  const reloaded = JSON.parse(JSON.stringify([seeded, v1]));
  const { body } = askRequest("demo", true, reloaded, "next?");
  assert.deepEqual(body, { question: "next?", history: [{ question: "Visitor 1?", answer: "Visitor answer." }] });
});

test("nothing is sent when the visitor owns no turn (only seeded)", () => {
  mem.clear();
  assert.deepEqual(askRequest("demo", true, [seeded], "q?").body, { question: "q?" });
});

test("normal case ignores ownership and history", () => {
  mem.clear();
  markOwned("own", visitor(1));
  assert.deepEqual(askRequest("own", false, [visitor(1)], "q?").body, { question: "q?" });
});

test("ownership is per case: another case's turns are not sent", () => {
  mem.clear();
  markOwned("demo-a", visitor(1));
  assert.deepEqual(askRequest("demo-b", true, [visitor(1)], "q?").body, { question: "q?" });
});

test("Reset clears ownership", () => {
  mem.clear();
  markOwned("demo", visitor(1));
  clearOwned("demo");
  assert.deepEqual(askRequest("demo", true, [visitor(1)], "q?").body, { question: "q?" });
});

test("only the last 6 owned turns are kept and sent", () => {
  mem.clear();
  const turns = [1, 2, 3, 4, 5, 6, 7, 8].map((n) => visitor(n));
  turns.forEach((t) => markOwned("demo", t));
  assert.equal(JSON.parse(mem.get("op:ask-own:demo")).length, 6);
  const { body } = askRequest("demo", true, [seeded, ...turns], "q?");
  assert.deepEqual(body.history.map((h) => h.question), ["Visitor 3?", "Visitor 4?", "Visitor 5?", "Visitor 6?", "Visitor 7?", "Visitor 8?"]);
});

test("a long (4001) restored visitor answer shows the notice on the real drawer and is sent as the marker", () => {
  mem.clear();
  const v = visitor(1, "A".repeat(4001));
  markOwned("demo", v);
  const c = caseOf("demo", true, [seeded, v]);
  const out = render(c);
  assert.match(out, NOTE);
  assert.match(out, /A{200}/); // the original answer stays visible
  assert.match(askRequest("demo", true, c.chat, "q?").body.history[0].answer, /^\[Earlier answer omitted/);
});

test("no notice at 4000, on a normal case, or for a seeded turn", () => {
  mem.clear();
  const v = visitor(1, "A".repeat(4000));
  markOwned("demo", v);
  assert.doesNotMatch(render(caseOf("demo", true, [seeded, v])), NOTE);
  const own = visitor(2, "A".repeat(9000));
  markOwned("own", own);
  assert.doesNotMatch(render(caseOf("own", false, [own])), NOTE);
  const longSeed = { ...seeded, answer: "A".repeat(9000) };
  assert.doesNotMatch(render(caseOf("demo2", true, [longSeed])), NOTE);
});
