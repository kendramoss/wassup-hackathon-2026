// D917: an undetermined Japan result asks the facts that settle it, in place.
import assert from "node:assert/strict";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

const { JapanPreference, answerPayload, headingFor } = await import("../src/components/op/japan-preference.tsx");
const { copy } = await import("../src/lib/copy.ts");

const questions = [
  { id: "originating:0", text: "Does insole have proof of origin under CPTPP?", options: ["Yes", "No", "Not sure"] },
  { id: "steps", text: "What are the production steps, and in which country?" },
];
const c = (outcome, extra = {}) => ({
  id: "case_q1",
  jurisdiction: "jp",
  prediction: { japan_preference: { agreement: "CPTPP", hs: "6403.99", outcome, questions, ...extra } },
});
const noop = () => {};

test("an undetermined result shows each question with answer buttons and one Check again", () => {
  const m = html(JapanPreference, { c: c("undetermined"), setCase: noop });
  const t = text(m);
  assert.match(m, /data-japan-questions/);
  assert.match(t, /Answer to decide/);
  assert.match(t, /Does insole have proof of origin under CPTPP\?\s*Yes\s*No\s*Not sure/);
  assert.match(t, /What are the production steps, and in which country\?/);
  assert.equal((m.match(/type="radio"/g) ?? []).length, 3);
  assert.match(m, /<legend[^>]*>Does insole/);
  assert.match(m, /<label for="[^"]+"[^>]*>What are the production steps/);
  assert.equal((t.match(/Check again/g) ?? []).length, 1);
  assert.match(m, /aria-disabled="true"[^>]*>Check again/, "disabled until one answer adds a fact");
  assert.match(t, /Answer at least one/);
});

test("no card when the result is decided, the case is pinned, or the page cannot update", () => {
  assert.doesNotMatch(html(JapanPreference, { c: c("qualifies"), setCase: noop }), /data-japan-questions/);
  assert.doesNotMatch(html(JapanPreference, { c: { ...c("undetermined"), pinned: true }, setCase: noop }), /data-japan-questions/);
  assert.doesNotMatch(html(JapanPreference, { c: c("undetermined") }), /data-japan-questions/);
  assert.doesNotMatch(html(JapanPreference, { c: c("undetermined", { questions: [] }), setCase: noop }), /data-japan-questions/);
});

test("Not sure and blank answers add no fact; the question text goes with each answer", () => {
  assert.deepEqual(answerPayload(questions, { "originating:0": "Not sure", steps: "  " }), []);
  assert.deepEqual(answerPayload(questions, { "originating:0": "Yes", steps: " Lasting and sewing in Vietnam " }), [
    { question: "Does insole have proof of origin under CPTPP?", answer: "Yes" },
    { question: "What are the production steps, and in which country?", answer: "Lasting and sewing in Vietnam" },
  ]);
});

test("the card's words stay few and plain", () => {
  for (const k of ["ask", "checkAgain", "pickOne", "yes", "no", "notSure", "was"]) assert.ok(copy.japan[k].split(" ").length <= 4, k);
  assert.equal(headingFor("undetermined"), copy.japan.undetermined);
  assert.equal(headingFor("qualifies"), copy.japan.qualifies);
});
