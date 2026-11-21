import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { test } from "node:test";

// Issue #397 / D262: the calibration layer ties confidence levels and the
// before/after ECE uses different bins. Guard the corrected wording and the
// recomputed fields that make it checkable offline.
const read = (relative) =>
  readFileSync(fileURLToPath(new URL(relative, import.meta.url)), "utf8");

const evaluation = read("../../docs/EVALUATION.md");
const decisions = read("../../docs/DECISIONS.md");
const results = read("../../eval/RESULTS.md");
const builder = read("../../eval/charts/build_accuracy.py");
const metrics = JSON.parse(read("../../eval/results/accuracy-v2-metrics.json"));
const layer = metrics.calibration_layer_engine_v1;

test("the docs no longer claim the calibration map preserves ranking exactly", () => {
  assert.doesNotMatch(evaluation, /ranks cases exactly as before/i);
  assert.match(evaluation, /weakly\*\* monotone/);
  assert.match(evaluation, /11 stated levels become 6/);
  assert.match(evaluation, /0\.72–0\.88 \(42 cases\) pool at 0\.6136/);
  assert.match(evaluation, /0\.98, 0\.99 and 1\.00 \(180 cases\) cap at 0\.99/);
  assert.match(evaluation, /no calibrated cut equivalent to a stated bar of 0\.99 or 1\.00/);
  assert.match(evaluation, /11 bins before and 6 after/);
  assert.match(evaluation, /gives \*\*0\.053\*\*/);
  assert.match(results, /Read the ECE drop with care \(D262, #397\)/);
  assert.match(results, /0\.0528/);
  assert.match(decisions, /## D262 — Correction to D156/);
  assert.match(decisions, /weakly monotone/);
});

test("the generated metrics expose the tie counts and the common-bin ECE", () => {
  assert.deepEqual(layer.heldout_level_counts, { stated: 11, calibrated: 6 });
  assert.equal(layer.heldout_stated_on_calibrated_bins, 0.0528);
  const levels = layer.heldout_calibrated_levels;
  const pooled = levels.find((l) => l.calibrated === 0.6136);
  const capped = levels.find((l) => l.calibrated === 0.99);
  assert.ok(pooled && capped, "both tied levels must be recorded");
  assert.equal(pooled.n, 42);
  assert.deepEqual(pooled.stated, [0.72, 0.78, 0.85, 0.88]);
  assert.equal(capped.n, 180);
  assert.deepEqual(capped.stated, [0.98, 0.99, 1.0]);
});

test("make reproduce recomputes the common-bin figure", () => {
  assert.match(builder, /def ece_on_partition\(/);
  assert.match(builder, /heldout_stated_on_calibrated_bins/);
  assert.match(builder, /heldout_calibrated_levels/);
});
