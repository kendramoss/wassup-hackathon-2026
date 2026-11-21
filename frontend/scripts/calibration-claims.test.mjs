import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { test } from "node:test";

// Issue #397 / D262 and its unfinished second acceptance item #493 / D306:
// every published before/after ECE pair must either share a partition or name
// the bin counts and carry the same-bin figure beside it.
const read = (relative) =>
  readFileSync(fileURLToPath(new URL(relative, import.meta.url)), "utf8");

const evaluation = read("../../docs/EVALUATION.md");
const decisions = read("../../docs/DECISIONS.md");
const results = read("../../eval/RESULTS.md");
const numbers = read("../../docs/NUMBERS.md");
const builder = read("../../eval/charts/build_accuracy.py");
const metrics = JSON.parse(read("../../eval/results/accuracy-v2-metrics.json"));
const summary = JSON.parse(read("../../backend/internal/evalsummary/summary.json"));
const page = read("../src/components/op/screens/accuracy-page.tsx");
const layer = metrics.calibration_layer_engine_v1;

const near = (got, want, label) =>
  assert.ok(typeof got === "number" && Math.abs(got - want) < 0.00005, `${label}: ${got}, want ${want}`);

const sets = [
  { name: "cal-v1 final 377", d: layer, prefix: "heldout_", same: 0.0528, levels: [11, 6] },
  { name: "cal-v1 final2 83", d: metrics.heldout.final2.calibration_layer, prefix: "", same: 0.0614, levels: [9, 6] },
  { name: "cal-v1 pooled 460", d: metrics.heldout.pooled_final_final2.calibration_layer, prefix: "", same: 0.0500, levels: [11, 6] },
  { name: "cal-v2 final 377", d: metrics.calibration_v2.final, prefix: "", same: 0.0528, levels: [11, 6] },
  { name: "cal-v2 final2 83", d: metrics.calibration_v2.final2, prefix: "", same: 0.0614, levels: [9, 6] },
  { name: "cal-v2 pooled 460", d: metrics.calibration_v2.pooled_final_final2, prefix: "", same: 0.0500, levels: [11, 6] },
];

test("the docs no longer claim the calibration map preserves ranking exactly", () => {
  assert.doesNotMatch(evaluation, /ranks cases exactly as before/i);
  assert.match(evaluation, /weakly\*\* monotone/);
  assert.match(evaluation, /11 stated levels become 6/);
  assert.match(evaluation, /9 stated levels become 6/);
  assert.match(evaluation, /0\.72–0\.88 \(42 cases\) pool at 0\.6136/);
  assert.match(evaluation, /0\.98, 0\.99 and 1\.00 \(180 cases\) cap at 0\.99/);
  assert.match(evaluation, /no calibrated cut equivalent to a stated bar of 0\.99 or 1\.00/);
  assert.match(evaluation, /11 bins before and 6 after/);
  assert.match(evaluation, /\*\*0\.0528\*\*/);
  assert.match(evaluation, /\*\*0\.0614\*\*/);
  assert.match(evaluation, /\*\*0\.0500\*\*/);
  assert.match(evaluation, /stated on the map's bins/);
  assert.match(results, /Read the ECE drop with care \(D262, #397, #493\)/);
  assert.match(results, /0\.0528/);
  assert.match(results, /0\.0614/);
  assert.match(results, /0\.0500/);
  assert.match(numbers, /stated on those bins 0\.0528/);
  assert.match(numbers, /same-bin 0\.0614/);
  assert.match(numbers, /same-bin 0\.0500/);
  assert.match(decisions, /## D262 — Correction to D156/);
  assert.match(decisions, /## D306 - Same-bin ECE on every held-out set/);
  assert.match(decisions, /weakly monotone/);
});

test("all six generated sets carry the same-bin ECE and level counts", () => {
  for (const s of sets) {
    near(s.d[s.prefix + "stated_on_calibrated_bins"], s.same, s.name + " same-bin ECE");
    assert.deepEqual(s.d[s.prefix + "level_counts"], { stated: s.levels[0], calibrated: s.levels[1] }, s.name + " level counts");
    assert.ok(Array.isArray(s.d[s.prefix + "calibrated_levels"]), s.name + " per-level ties");
  }
});

test("make reproduce recomputes every common-bin figure", () => {
  assert.match(builder, /def ece_on_partition\(/);
  assert.match(builder, /def calibration_partition\(/);
  assert.match(builder, /\*\*calibration_partition\(fin_c, "heldout_"\)/);
  assert.match(builder, /\*\*calibration_partition\(f2c\)/);
  assert.match(builder, /\*\*calibration_partition\(pc\)/);
  assert.match(builder, /\*\*calibration_partition\(fin2v\)/);
  assert.match(builder, /\*\*calibration_partition\(f2v_rows\)/);
  assert.match(builder, /\*\*calibration_partition\(pv_rows\)/);
});

test("the served ECE labels and /accuracy name the different bins", () => {
  const byKey = Object.fromEntries(summary.items.map((i) => [i.key, i]));
  assert.match(byKey.final_calibration_ece_stated.label, /different bins: 11 stated levels, 6 calibrated levels/);
  assert.match(byKey.final_calibration_ece_calibrated.label, /different bins: 11 stated levels, 6 calibrated levels/);
  assert.match(byKey.final_calibration_ece_stated_same_bins.label, /same 6 calibrated bins/);
  near(byKey.final_calibration_ece_stated_same_bins.value, 0.0528, "served same-bin ECE");
  assert.match(page, /final_calibration_ece_stated_same_bins/);
  assert.match(page, /do not use the same bins/);
  assert.match(page, /on the map's bins/);
});
