import assert from "node:assert/strict";
import { test } from "node:test";
import { predictBody, productSignature, structuredFacts } from "../src/lib/product-facts-reconcile.ts";

// #345: a fresh (or imported) origin-only product has no family and no carried
// narrative. The request must still carry facts written from the current tables.
const product = () => ({
  name: "Heat gun",
  components: [
    { id: "motor", name: "Motor", made_in: "CN", plant: "p1" },
    { id: "housing", name: "Housing", made_in: "VN" },
  ],
  operations: [{ id: "assembly", name: "Final assembly", plant: "p1" }],
  plants: [{ id: "p1", name: "Plant 1", country: "VN", capabilities: [] }],
});
const input = (over = {}) => ({ date: "2026-01-01", tab: "parts", name: "Heat gun", hts: "", family: "", product: product(), extra: {}, ...over });

test("fresh manual origin-only product submits structured facts and no family", () => {
  const b = predictBody(input());
  assert.equal(b.family, undefined);
  assert.equal(b.facts, structuredFacts({ ...product(), name: "Heat gun" }));
  assert.match(b.facts, /Motor: made in CN at Plant 1, VN/);
  assert.equal(b.product.components.length, 2);
});

test("imported origin-only product with matching baseline and no facts submits structured facts", () => {
  const b = predictBody(input({ extra: { baseline: productSignature(product()) } }));
  assert.equal(b.family, undefined);
  assert.equal(b.facts, structuredFacts({ ...product(), name: "Heat gun" }));
});

test("facts follow the current tables, not stale ones", () => {
  const p = product();
  p.components[1].made_in = "TW";
  assert.match(predictBody(input({ product: p })).facts, /Housing: made in TW/);
});

test("facts-present control keeps carried facts", () => {
  const b = predictBody(input({ extra: { facts: "carried narrative", baseline: productSignature(product()) } }));
  assert.equal(b.facts, "carried narrative");
});

test("a chosen family sends no guessed facts", () => {
  const b = predictBody(input({ family: "motorised_tools_small_appliances" }));
  assert.equal(b.family, "motorised_tools_small_appliances");
  assert.equal(b.facts, undefined);
});

test("edited baseline with no family still reconciles to structured facts", () => {
  const p = product();
  p.components[0].made_in = "TW";
  const b = predictBody(input({ product: p, extra: { facts: "old", baseline: productSignature(product()) } }));
  assert.match(b.facts, /Motor: made in TW/);
});

test("paste tab is unchanged", () => {
  assert.equal(predictBody(input({ tab: "paste", facts: "pasted" })).facts, "pasted");
});
