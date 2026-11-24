import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const read = (p) => readFileSync(new URL(p, import.meta.url), "utf8");
const { japanEvidence: j } = await import("../src/lib/market-evidence.ts");
const held = JSON.parse(read("../../eval/jp/jp-held_out-gemini-4-argon-jp-engine-v1-litellm-d741-heldout-1-metrics.json"));
const syn = JSON.parse(read("../../eval/jp/jp-synthetic-gemini-4-argon-jp-engine-v1-litellm-d741-synthetic-1-metrics.json"));
const pct = (x) => (x * 100).toFixed(2);
const span = (c) => `${pct(c.lo)}–${pct(c.hi)}`;

test("Japan experiment figures match the committed D741 metrics", () => {
  assert.equal(j.cases, held.cases);
  assert.equal(j.criterionMatch, held.criterion_match.k);
  assert.equal(j.criterionMatch + j.otherQualifies, held.qualifies);
  assert.equal(j.recall, held.positive_recall.k);
  assert.equal(j.undetermined, held.undetermined);
  assert.equal(j.errors, held.errors);
  assert.equal(j.criterionMatch + j.otherQualifies + j.undetermined + j.errors, held.cases);
  assert.equal(j.matchPct, `${pct(held.criterion_match.p)}%`);
  assert.equal(j.matchInterval, span(held.criterion_match));
  assert.equal(j.recallPct, `${pct(held.positive_recall.p)}%`);
  assert.equal(j.recallInterval, span(held.positive_recall));
  assert.equal(j.syntheticRefused, syn.synthetic_refusals.k);
  assert.equal(j.syntheticCases, syn.synthetic_refusals.n);
  assert.equal(j.syntheticInterval, `${pct(syn.synthetic_refusals.lo)}–100`);
});

test("Accuracy page labels the Japan result experimental and keeps Japan unavailable", () => {
  const src = read("../src/components/op/screens/accuracy-page.tsx");
  assert.ok(src.includes("Experimental and unreviewed. Japan is not available in the product."));
  assert.ok(src.includes("not an accuracy rate"));
  assert.ok(src.includes("{copy.japan.notAvailable}"));
});
