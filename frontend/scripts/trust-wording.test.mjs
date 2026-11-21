import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { test } from "node:test";

// Issue #398: fixes called "done" in the trust audit left copies of the old
// wording behind. Keep the served label, the case flow and the model card
// saying the same thing, or this test fails.
const read = (relative) =>
  readFileSync(fileURLToPath(new URL(relative, import.meta.url)), "utf8");

const summary = JSON.parse(read("../../backend/internal/evalsummary/summary.json"));
const product = read("../src/components/op/screens/product-step.tsx");
const card = read("../../docs/MODEL-CARD.md");

test("the served router label names Part 102, not USMCA", () => {
  for (const key of ["router_out_of_scope_recall", "router_final2_out_of_scope_recall"]) {
    const item = summary.items.find((i) => i.key === key);
    assert.ok(item, `summary must serve ${key}`);
    assert.match(item.label, /Part 102/i, `${key}: label must name Part 102`);
    assert.doesNotMatch(item.label, /USMCA rulings/i, `${key}: stale USMCA label`);
  }
});

test("the case flow does not claim an unmeasured prediction time", () => {
  assert.doesNotMatch(product, /usually takes under a minute/i);
  assert.match(product, /Progress is shown below/);
});

test("the model card states which baselines are recomputed and covers both splits", () => {
  assert.match(card, /simple-rule baselines were recomputed from committed facts/i);
  assert.match(card, /go run \.\/cmd\/eval baselines/);
  assert.match(card, /first country named 239\/377 = 63\.4%/);
  assert.match(card, /most-mentioned 229\/377 = 60\.7%/);
  assert.match(card, /majority country 46\/377 = 12\.2%/);
  assert.match(card, /final assembly 40\.6% and last-mentioned 39\.8%/);
  assert.match(card, /second held-out split of 83 rulings/i);
});
