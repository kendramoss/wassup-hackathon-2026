import assert from "node:assert/strict";
import { test } from "node:test";
import "./render-support.mjs";

// D160: origin screens show the calibrated confidence against the 0.90 bar
// when the API returns one, else the stated confidence against 0.92.
const { originConf, originConfCaption, BAR, CAL_BAR } = await import("../src/lib/format.ts");
const { copy } = await import("../src/lib/copy.ts");

test("calibrated value wins, with the calibrated bar and both numbers in the caption", () => {
  const o = originConf({ confidence: 0.97, calibrated_confidence: 0.9355 });
  assert.deepEqual(o, { value: 0.9355, stated: 0.97, bar: CAL_BAR, calibrated: true });
  assert.equal(originConfCaption(o), "confidence 0.94 · model said 0.97");
  assert.equal(copy.result.whyAuto(o), "Why · auto-accepted: confidence 0.94, above the 0.90 auto-accept bar");
});

test("at the calibrated bar reads 'at'", () => {
  assert.equal(copy.result.whyAuto(originConf({ confidence: 0.92, calibrated_confidence: 0.9 })), "Why · auto-accepted: confidence 0.90, at the 0.90 auto-accept bar");
});

test("no calibrated value: stated confidence against 0.92, caption as before", () => {
  const o = originConf({ confidence: 0.92 });
  assert.deepEqual(o, { value: 0.92, stated: 0.92, bar: BAR, calibrated: false });
  assert.equal(originConfCaption(o), "confidence 0.92");
  assert.equal(copy.result.whyAuto(o), "Why · auto-accepted: confidence 0.92, at the 0.92 auto-accept bar");
  assert.match(copy.result.review.body(originConf({ confidence: 0.85, calibrated_confidence: 0.6 })), /^Confidence 0\.60 \(model said 0\.85\)\./);
});
