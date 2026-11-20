import assert from "node:assert/strict";
import { test } from "node:test";
import { cellKey, estimateProductKey, mergeCostEstimates } from "../src/lib/economics-estimate.ts";

const estimate = (group, plant, usd) => ({ group, plant, usd, low: usd * 0.8, high: usd * 1.2, reason: "similar rulings" });

test("a late estimate merges into current inputs and keeps manual edits", () => {
  const started = { annual_import_value_usd: 100000, group_costs: { pcba: {} } };
  // While the answer was in flight the user changed imports and filled pcba|th.
  const current = { annual_import_value_usd: 200000, group_costs: { pcba: { th: 9000 } } };
  const out = mergeCostEstimates(current, [estimate("pcba", "th", 500), estimate("pcba", "cn", 1200)]);
  assert.equal(out.economics.annual_import_value_usd, 200000);
  assert.equal(out.economics.group_costs.pcba.th, 9000);
  assert.equal(out.economics.group_costs.pcba.cn, 1200);
  assert.equal(out.economics.illustrative, true);
  assert.equal(started.group_costs.pcba.th, undefined, "the captured object is not mutated");
  assert.deepEqual(Object.keys(out.cells), [cellKey("pcba", "cn")]);
});

test("a superseded product and a dismissed form have different guards", () => {
  const a = { name: "Heat gun", hts: "8419", plants: [{ id: "th" }] };
  const b = { ...a, hts: "8507" };
  const c = { ...a, plants: [{ id: "vn" }] };
  assert.equal(estimateProductKey(a), estimateProductKey({ ...a }));
  assert.notEqual(estimateProductKey(a), estimateProductKey(b));
  assert.notEqual(estimateProductKey(a), estimateProductKey(c));
});
