import assert from "node:assert/strict";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

// D380: per-part HTS suggestions are offered, never filled in by themselves.
// acceptAll fills the top code only into parts without one, and only while the
// row still holds the part the suggestion was made for.
const { acceptAll, suggestionFor, PartSuggestionRow } = await import("../src/components/op/parts-hts.tsx");

const motor = { hts: "8501.31", source: "rulings", reason: "A DC motor, as in N100001.", cited: ["N100001"], supporting_rulings: [{ id: "N100001", url: "https://rulings.cbp.gov/ruling/N100001" }] };
const housing = { hts: "3926.90", source: "model", reason: "Plastic article.", cited: [], supporting_rulings: [] };
const sugg = {
  parts: new Map([
    [0, { name: "motor", candidates: [motor] }],
    [1, { name: "housing", candidates: [housing] }],
    [2, { name: "cord", candidates: [] }],
  ]),
  good: null,
  reasonsDown: false,
  capped: false,
};

test("acceptAll fills top codes into parts without one", () => {
  const product = {
    name: "fan",
    plants: [],
    operations: [],
    components: [
      { name: "motor", made_in: "China" },
      { name: "housing", made_in: "China", hts: "3923.90" },
      { name: "cord", made_in: "China" },
    ],
  };
  const r = acceptAll(product, sugg);
  assert.equal(r.filled, 1);
  assert.equal(r.product.components[0].hts, "8501.31");
  assert.equal(r.product.components[1].hts, "3923.90", "a typed code is kept");
  assert.equal(r.product.components[2].hts, undefined, "no suggestion, no code");
  assert.equal(product.components[0].hts, undefined, "the input is not mutated");
});

test("a suggestion follows its part, not its position", () => {
  assert.deepEqual(suggestionFor(sugg, 0, { name: "motor", made_in: "" }), [motor]);
  assert.equal(suggestionFor(sugg, 0, { name: "fan blade", made_in: "" }), null, "the row now holds another part");
  const moved = acceptAll({ name: "fan", plants: [], operations: [], components: [{ name: "housing", made_in: "China" }] }, sugg);
  assert.equal(moved.filled, 0);
});

test("each code says where it comes from", () => {
  const out = html(PartSuggestionRow, { cands: [motor, housing], value: "8501.31", onUse: () => {}, label: "motor", colSpan: 6 });
  const t = text(out);
  assert.match(t, /8501\.31/);
  assert.match(t, /earlier CBP ruling/);
  assert.match(t, /model's reading, no ruling found/);
  assert.match(out, /aria-pressed="true"/);
  assert.match(out, /N100001/);
});
