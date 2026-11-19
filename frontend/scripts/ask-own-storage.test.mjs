import assert from "node:assert/strict";
import { test } from "node:test";
import { askRequest, clearOwned, markOwned, ownedMarks, turnMark } from "../src/lib/ask-history.ts";

const t = (n) => ({ question: `q${n}`, answer: `a${n}`, at: `2026-11-20T00:00:0${n}Z` });
const memory = (seed = {}) => {
  const m = { ...seed };
  return { getItem: (k) => (k in m ? m[k] : null), setItem: (k, v) => void (m[k] = v), removeItem: (k) => void delete m[k] };
};

test("storage that throws on every call degrades to no history and never throws", () => {
  const boom = () => {
    throw new Error("denied");
  };
  const denied = { getItem: boom, setItem: boom, removeItem: boom };
  assert.deepEqual(ownedMarks("demo", denied), []);
  assert.doesNotThrow(() => markOwned("demo", t(1), denied));
  assert.doesNotThrow(() => clearOwned("demo", denied));
  const { body } = askRequest("demo", true, [t(1)], "next", denied);
  assert.deepEqual(body, { question: "next" });
});

test("no storage at all behaves the same", () => {
  assert.deepEqual(ownedMarks("demo", null), []);
  assert.doesNotThrow(() => markOwned("demo", t(1), null));
  assert.deepEqual(askRequest("demo", true, [t(1)], "next", null).body, { question: "next" });
});

test("malformed, non-array and non-string markers are ignored", () => {
  assert.deepEqual(ownedMarks("demo", memory({ "op:ask-own:demo": "{not json" })), []);
  assert.deepEqual(ownedMarks("demo", memory({ "op:ask-own:demo": '{"a":1}' })), []);
  assert.deepEqual(ownedMarks("demo", memory({ "op:ask-own:demo": '[1,null,"ok"]' })), ["ok"]);
});

test("a quota error on write keeps the turn out of the history without breaking the ask", () => {
  const m = memory();
  const full = { ...m, setItem: () => { throw new DOMException("quota", "QuotaExceededError"); } };
  assert.doesNotThrow(() => markOwned("demo", t(1), full));
  assert.deepEqual(askRequest("demo", true, [t(1)], "next", full).body, { question: "next" });
});

test("markers are bounded to the last 6 and de-duplicated", () => {
  const m = memory();
  for (let i = 1; i <= 8; i++) markOwned("demo", t(i), m);
  markOwned("demo", t(8), m);
  const marks = ownedMarks("demo", m);
  assert.equal(marks.length, 6);
  assert.equal(marks.at(-1), turnMark(t(8)));
});
