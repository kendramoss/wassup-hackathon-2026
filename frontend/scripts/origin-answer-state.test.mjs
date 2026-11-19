import assert from "node:assert/strict";
import { test } from "node:test";
import { blankAnswers, buildAnswerPayload, reconcileAnswers } from "../src/lib/answer-state.ts";

const edit = (s, i, answer) => ({ ...s, answers: s.answers.map((a, j) => (j === i ? answer : a)) });
const idk = (s, i) => ({ ...s, unknown: s.unknown.map((u, j) => (j === i ? !u : u)) });

// #308: one question grows into two while Questions stays mounted.
test("growth 1 -> 2 keeps the first answer and sizes both arrays", () => {
  let s = edit(blankAnswers(["A?"]), 0, "alpha");
  s = reconcileAnswers(["A?"], s, ["A?", "B?"]);
  assert.deepEqual(s.answers, ["alpha", ""]);
  assert.deepEqual(s.unknown, [false, false]);
  s = edit(s, 1, "beta");
  assert.deepEqual(buildAnswerPayload(["A?", "B?"], s), [
    { question: "A?", answer: "alpha" },
    { question: "B?", answer: "beta" },
  ]);
});

test("second submission after growth does not throw", () => {
  const s = reconcileAnswers(["A?"], blankAnswers(["A?"]), ["A?", "B?"]);
  assert.doesNotThrow(() => buildAnswerPayload(["A?", "B?"], s));
});

test("shrink drops the removed question's answer and unknown flag", () => {
  let s = edit(blankAnswers(["A?", "B?", "C?"]), 0, "a");
  s = edit(s, 2, "c");
  s = idk(s, 1);
  s = reconcileAnswers(["A?", "B?", "C?"], s, ["C?"]);
  assert.deepEqual(s.answers, ["c"]);
  assert.deepEqual(s.unknown, [false]);
});

test("replacement does not carry unknown or answer to an unrelated question", () => {
  let s = idk(blankAnswers(["Old?"]), 0);
  s = edit(s, 0, "stale");
  s = reconcileAnswers(["Old?"], s, ["New?"]);
  assert.deepEqual(s.answers, [""]);
  assert.deepEqual(s.unknown, [false]);
});

test("reordered questions keep state by identity, not index", () => {
  let s = idk(blankAnswers(["A?", "B?"]), 0);
  s = edit(s, 1, "bee");
  s = reconcileAnswers(["A?", "B?"], s, ["B?", "A?"]);
  assert.deepEqual(s.answers, ["bee", ""]);
  assert.deepEqual(s.unknown, [false, true]);
});

test("same questions on re-render keep typed answers", () => {
  const s = edit(blankAnswers(["A?", "B?"]), 1, "typed");
  assert.deepEqual(reconcileAnswers(["A?", "B?"], s, ["A?", "B?"]), s);
});

test("duplicate question text is matched once each", () => {
  let s = edit(blankAnswers(["Q?", "Q?"]), 0, "first");
  s = reconcileAnswers(["Q?", "Q?"], s, ["Q?", "Q?", "Q?"]);
  assert.deepEqual(s.answers, ["first", "", ""]);
});

test("payload uses I don't know for unknown and skips blanks", () => {
  let s = idk(edit(blankAnswers(["A?", "B?", "C?"]), 0, "  x  "), 1);
  assert.deepEqual(buildAnswerPayload(["A?", "B?", "C?"], s), [
    { question: "A?", answer: "x" },
    { question: "B?", answer: "I don't know." },
  ]);
});

test("payload tolerates state shorter than the questions", () => {
  assert.deepEqual(buildAnswerPayload(["A?", "B?"], { answers: ["a"], unknown: [] }), [{ question: "A?", answer: "a" }]);
});
