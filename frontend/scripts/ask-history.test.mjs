import assert from "node:assert/strict";
import { test } from "node:test";
import { askBody, askHistory } from "../src/lib/ask-history.ts";

const turn = (n, extra = {}) => ({ question: `q${n}`, answer: `a${n}`, ...extra });
const local = (t) => t.local === true;

test("only this visit's completed turns are sent, never the seeded ones", () => {
  const chat = [turn(0), turn(1, { local: true }), turn(2, { local: true })];
  assert.deepEqual(askHistory(chat, local), [
    { question: "q1", answer: "a1" },
    { question: "q2", answer: "a2" },
  ]);
});

test("turns without a visible question or answer are skipped", () => {
  const chat = [turn(1, { local: true }), { question: "q2", answer: "  ", local: true }, { question: "", answer: "a3", local: true }, { local: true }];
  assert.deepEqual(askHistory(chat, local), [{ question: "q1", answer: "a1" }]);
});

test("at most the last 6 turns", () => {
  const chat = Array.from({ length: 9 }, (_, i) => turn(i, { local: true }));
  const h = askHistory(chat, local);
  assert.equal(h.length, 6);
  assert.equal(h[0].question, "q3");
  assert.equal(h[5].question, "q8");
});

test("the request carries history for a pinned demo only", () => {
  const h = [{ question: "q", answer: "a" }];
  assert.deepEqual(askBody("next?", true, h), { question: "next?", history: h });
  assert.deepEqual(askBody("next?", true, []), { question: "next?" });
  assert.deepEqual(askBody("next?", false, h), { question: "next?" });
  assert.equal(askBody("n", true, Array.from({ length: 9 }, () => h[0])).history.length, 6);
});

import { ASK_OMITTED, askContext } from "../src/lib/ask-history.ts";

test("an answer over 4000 runes is replaced whole by the marker, counted in runes", () => {
  const exact = "語".repeat(4000);
  const long = "語".repeat(4001);
  const c = askContext([turn(1, { local: true, answer: exact }), turn(2, { local: true, answer: long })], local);
  assert.equal(c.history[0].answer, exact); // at the limit: unchanged
  assert.equal(c.history[1].answer, ASK_OMITTED); // no prefix kept
  assert.equal(c.history[1].question, "q2"); // the question is kept
  assert.equal(c.omitted, true);
  assert.equal(askContext([turn(1, { local: true })], local).omitted, false);
});

test("a long answer older than the last 6 turns does not raise the notice", () => {
  const chat = [turn(0, { local: true, answer: "x".repeat(5000) }), ...Array.from({ length: 6 }, (_, i) => turn(i + 1, { local: true }))];
  assert.equal(askContext(chat, local).omitted, false);
});
