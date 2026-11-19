import assert from "node:assert/strict";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

const { PrefillNote, invalidHtsNote } = await import("../src/components/op/money-prefill.tsx");

// #306: money-prefill reports a malformed product HTS in hts_note (no
// hts_source). The screen must say so and must not claim there is simply no
// HTS number yet, nor that duty was filled.
const NOTE = "The product's HTS number is not a valid 8- or 10-digit HTS number, so no duty rates were filled; enter the rates yourself or correct the HTS.";

test("a malformed-HTS note is carried; a suggestion's broker note is not duplicated", () => {
  assert.equal(invalidHtsNote({ hts_note: NOTE }), NOTE);
  assert.equal(invalidHtsNote({ hts_note: "suggestion only", hts_source: "suggested" }), undefined);
  assert.equal(invalidHtsNote({}), undefined);
  assert.equal(invalidHtsNote({ hts_source: "product" }), undefined);
});

test("a malformed HTS shows the note and not the no-HTS-yet prompt or any filled duty", () => {
  const m = text(html(PrefillNote, { f: { fromFile: [], duty: [], htsNote: NOTE } }));
  assert.match(m, /not a valid 8- or 10-digit HTS number/);
  assert.doesNotMatch(m, /No HTS number to price duty from yet/);
  assert.doesNotMatch(m, /Duty, /);
});

test("with a note and a file figure, both show and no duty is claimed", () => {
  const m = text(html(PrefillNote, { f: { fromFile: ["imports a year"], duty: [], htsNote: NOTE } }));
  assert.match(m, /From your file: imports a year/);
  assert.match(m, /not a valid 8- or 10-digit HTS number/);
  assert.doesNotMatch(m, /Duty, /);
});

test("a suggested HTS still shows its existing broker line, once", () => {
  const f = { fromFile: [], duty: [{ country: "China", rate: 0.4, source: "table" }], hts: "8419.89.95", htsSuggested: true };
  const m = text(html(PrefillNote, { f }));
  assert.equal((m.match(/confirm the classification with a broker/g) ?? []).length, 1);
  assert.doesNotMatch(m, /not a valid 8- or 10-digit/);
});

test("without a note or figures the existing no-HTS prompt is unchanged", () => {
  const m = text(html(PrefillNote, { f: { fromFile: [], duty: [] } }));
  assert.match(m, /No HTS number to price duty from yet/);
});
