import assert from "node:assert/strict";
import { test } from "node:test";
import { predictBody, productSignature } from "../src/lib/product-facts-reconcile.ts";

// #292: extract -> edit country/operation -> the prediction request must not
// carry the pre-edit pasted facts or the demo's current-origin holding.
const extracted = () => ({
  name: "Heat gun",
  components: [
    { id: "motor", name: "Motor", made_in: "CN" },
    { id: "housing", name: "Housing", made_in: "VN" },
  ],
  operations: [{ id: "assembly", name: "Final assembly", plant: "p1" }],
  plants: [{ id: "p1", name: "Plant 1", country: "VN", capabilities: [] }],
});
const pasted = "Heat gun, motor made in CN, assembled in VN.";
const base = (product) => ({
  date: "2026-01-01",
  tab: "parts",
  name: "Heat gun",
  hts: "",
  family: "motorised_tools_small_appliances",
  product,
  extra: { facts: pasted, current_origin: { country: "VN", ruling: "N123" }, exclude_rulings: ["N1"], baseline: productSignature(extracted()) },
});

test("unchanged extracted/demo product keeps pasted facts and current origin", () => {
  const b = predictBody(base(extracted()));
  assert.equal(b.facts, pasted);
  assert.deepEqual(b.current_origin, { country: "VN", ruling: "N123" });
  assert.deepEqual(b.exclude_rulings, ["N1"]);
});

test("editing a component country drops stale facts and current origin", () => {
  const p = extracted();
  p.components[0].made_in = "TW";
  const b = predictBody(base(p));
  assert.equal(b.facts, undefined);
  assert.equal(b.current_origin, undefined);
  assert.equal(b.product.components[0].made_in, "TW");
  assert.deepEqual(b.exclude_rulings, ["N1"]);
});

test("editing an operation drops stale facts and current origin", () => {
  const p = extracted();
  p.operations[0].plant = "p2";
  const b = predictBody(base(p));
  assert.equal(b.facts, undefined);
  assert.equal(b.current_origin, undefined);
});

test("name and HTS edits are not material", () => {
  const p = { ...extracted(), name: "Renamed", hts: "8419.89.95" };
  const b = predictBody({ ...base(p), name: "Renamed", hts: "8419.89.95" });
  assert.equal(b.facts, pasted);
  assert.ok(b.current_origin);
});

test("reverting an edit restores the original facts", () => {
  const p = extracted();
  p.components[0].made_in = "TW";
  p.components[0].made_in = "CN";
  assert.equal(predictBody(base(p)).facts, pasted);
});

test("paste tab sends facts only, with current origin untouched", () => {
  const b = predictBody({ ...base(extracted()), tab: "paste", facts: "typed facts" });
  assert.equal(b.facts, "typed facts");
  assert.equal(b.product, undefined);
  assert.ok(b.current_origin);
});
