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

// Fix round 1: origin-only flow (family "") must not 400 and must not carry stale facts.
const edited = (fn) => {
  const p = extracted();
  fn(p);
  return p;
};

test("empty family + material edit sends facts built from the edited tables only", () => {
  const p = edited((x) => (x.components[0].made_in = "TW"));
  const b = predictBody({ ...base(p), family: "" });
  assert.equal(b.family, undefined);
  assert.ok(b.facts && b.facts.trim().length > 20, "server needs non-empty facts without a family");
  assert.match(b.facts, /Motor: made in TW/);
  assert.match(b.facts, /Housing: made in VN/);
  assert.match(b.facts, /Final assembly at Plant 1, VN/);
  assert.ok(!b.facts.includes("motor made in CN"), "stale narrative must be gone");
  assert.ok(!b.facts.includes("CN"), "old country must be gone");
  assert.equal(b.current_origin, undefined);
});

test("derived facts add nothing the user did not supply", () => {
  const p = { name: "Widget", components: [{ name: "Gear", made_in: "DE" }], operations: [], plants: [] };
  const b = predictBody({ ...base(p), family: "", extra: { facts: pasted, baseline: "x" } });
  assert.equal(b.facts, "Product: Heat gun.\n\nParts:\n- Gear: made in DE\n");
});

test("empty family + plant edit / add / remove all reconcile", () => {
  for (const fn of [
    (x) => (x.plants[0].country = "TH"),
    (x) => x.plants.push({ id: "p2", name: "Plant 2", country: "MX", capabilities: ["mould"] }),
    (x) => ((x.plants = []), (x.operations[0].plant = ""), (x.components = x.components.map((c) => ({ ...c })))),
  ]) {
    const b = predictBody({ ...base(edited(fn)), family: "" });
    assert.ok(b.facts && b.facts !== pasted && !b.facts.includes("assembled in VN"));
    assert.equal(b.current_origin, undefined);
  }
  const added = predictBody({ ...base(edited((x) => x.plants.push({ id: "p2", name: "Plant 2", country: "MX", capabilities: ["mould"] }))), family: "" });
  assert.match(added.facts, /Plant 2, MX; capabilities: mould/);
});

test("empty family + part added/removed reconciles", () => {
  const b = predictBody({ ...base(edited((x) => x.components.pop())), family: "" });
  assert.ok(!b.facts.includes("Housing"));
});

test("valid family + material edit still omits facts (server renders); no derived facts", () => {
  const b = predictBody(base(edited((x) => (x.components[0].made_in = "TW"))));
  assert.equal(b.facts, undefined);
  assert.equal(b.family, "motorised_tools_small_appliances");
});

test("empty family + unchanged product keeps original facts and current origin", () => {
  const b = predictBody({ ...base(extracted()), family: "" });
  assert.equal(b.facts, pasted);
  assert.deepEqual(b.current_origin, { country: "VN", ruling: "N123" });
});

test("product-step wires baselines and the helper (source contract only, not a render)", async () => {
  const { readFile } = await import("node:fs/promises");
  const src = await readFile(new URL("../src/components/op/screens/product-step.tsx", import.meta.url), "utf8");
  assert.match(src, /baseline: p\.product \? productSignature\(p\.product\)/);
  assert.match(src, /baseline: productSignature\(r\.product!\)/);
  assert.match(src, /predictBody\(\{ date, tab, name, hts, family, facts, product, extra \}\)/);
});
