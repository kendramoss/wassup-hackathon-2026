// D340/D341: shown plans rank by tier (clear margin, then at the bar), then
// net; the smallest supported move is a label. A result stored before D341
// has no tier fields, so the plan step works them out with the same rule.
import "./render-support.mjs";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

const { planTier, smallestSupported, partsMoved, clearFrom, CLEAR_FROM } = await import("../src/components/op/plan-bits.tsx");

const read = (p) => JSON.parse(readFileSync(fileURLToPath(new URL(p, import.meta.url)), "utf8"));
const run2 = read("../../eval/results/plan-heatgun-2022-relaxed-verifier-v2-split-run2-litellm.json");
const product = read("../../backend/testdata/heatgun_2022.json").product;
const key = (p) => p.move.map((m) => m.group).sort().join("+");

test("D332 run 2, stored before D341: CBP's move is the only clear-margin plan", () => {
  assert.equal(run2.ranking, undefined);
  const tiers = Object.fromEntries(run2.plans.map((p) => [key(p), planTier(p, run2)]));
  assert.equal(tiers["cord_knobs_covers+motor_fan_assembly+pcba"], "clear_margin");
  assert.equal(tiers["motor_fan_assembly+pcba"], "at_bar");
  assert.equal(tiers["cord_knobs_covers+motor_fan_assembly"], "at_bar");
  // 0.93 with a counter-ruling stays at the bar.
  assert.equal(tiers["motor_fan_assembly+pcba+switch_cord_minor_parts"], "at_bar");
});

test("the smallest supported move is counted from the product when not stored", () => {
  const s = smallestSupported(run2.plans, product);
  assert.equal(key(s), "motor_fan_assembly+pcba");
  assert.equal(partsMoved(s, product), 4);
});

test("a new result's own tier, threshold and mark win", () => {
  const plans = [
    { shown: true, tier: "at_bar", verdict: { confidence: 0.99 }, smallest_supported: false, parts_moved: 2 },
    { shown: true, tier: "clear_margin", verdict: { confidence: 0.94 }, smallest_supported: true, parts_moved: 5 },
  ];
  const result = { plans, ranking: { rule: "tier_then_net", bar: 0.92, clear_from: 0.95 } };
  assert.equal(planTier(plans[0], result), "at_bar");
  assert.equal(planTier(plans[1], result), "clear_margin");
  assert.equal(smallestSupported(plans), plans[1]);
  assert.equal(clearFrom(result), 0.95);
  assert.equal(clearFrom(undefined), CLEAR_FROM);
  assert.equal(planTier({ shown: false, verdict: { confidence: 0.99 } }, result), undefined);
});

test("one shown plan has no smallest-move label", () => {
  assert.equal(smallestSupported([run2.plans[0]], product), undefined);
});

test("the fallback threshold is the backend's bar plus its default clear margin", () => {
  const src = readFileSync(fileURLToPath(new URL("../../backend/internal/planner/plan.go", import.meta.url)), "utf8");
  const m = Number(/const DefaultClearMargin = ([0-9.]+)/.exec(src)[1]);
  assert.equal(Math.round((0.92 + m) * 100) / 100, CLEAR_FROM);
});
