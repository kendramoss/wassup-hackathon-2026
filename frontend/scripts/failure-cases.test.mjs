import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

// The "Known failure cases" section on the Accuracy page reads this file.
// It is built by eval/charts/build_trust_data.py (which also has --check);
// these tests pin the invariants the page relies on, so a hand edit that
// hides a miss or breaks a count fails here too.
const data = JSON.parse(readFileSync(new URL("../src/lib/failure-cases.json", import.meta.url), "utf8"));

test("every miss is listed, none hidden", () => {
  assert.equal(data.misses.length, data.n - data.correct);
  assert.ok(data.misses.length > 0);
  for (const m of data.misses) {
    assert.match(m.ruling, /^[HN]\d{6}$/);
    assert.notEqual(m.cbp_held, m.predicted);
    assert.ok(m.subject.length > 0);
  }
  assert.equal(new Set(data.misses.map((m) => m.ruling)).size, data.misses.length);
});

test("auto-accepted misses come first and sit at or above the bar", () => {
  const firstReview = data.misses.findIndex((m) => !m.auto_accepted);
  const autos = data.misses.filter((m) => m.auto_accepted);
  assert.ok(autos.every((m) => m.confidence >= data.bar));
  assert.ok(data.misses.slice(firstReview).every((m) => !m.auto_accepted));
  assert.ok(autos.length <= data.auto_accepted);
});

test("by-origin rows cover every ruling, with intervals around the rate", () => {
  assert.equal(data.by_origin.reduce((s, o) => s + o.n, 0), data.n);
  assert.equal(data.by_origin.reduce((s, o) => s + o.correct, 0), data.correct);
  for (const o of data.by_origin) {
    const v = o.correct / o.n;
    assert.ok(o.ci_low <= v && v <= o.ci_high, o.origin);
  }
});

test("the China split adds up to every miss", () => {
  const c = data.china;
  assert.equal(c.missed_china + c.said_china + c.neither, data.misses.length);
  const autos = data.misses.filter((m) => m.auto_accepted);
  assert.equal(c.missed_china_auto, autos.filter((m) => m.cbp_held === "China").length);
  assert.equal(c.said_china_auto, autos.filter((m) => m.predicted === "China").length);
});

test("the accuracy table cites the first committed file, not the last token (D180)", async () => {
  const { sourceLabel } = await import("../src/lib/source-label.ts");
  assert.equal(
    sourceLabel("eval/results/final-gemini-4-argon-prec-engine-v1-vertex.jsonl + backend/internal/calibrate/map.json (cal-v2, fitted on eval/results/dev-gemini-4-argon-prec-engine-v1-vertex.jsonl)"),
    "final-gemini-4-argon-prec-engine-v1-vertex.jsonl + 2 more",
  );
  assert.equal(sourceLabel("eval/results/planner-post-summary.json (eval/data/flips_post.jsonl, frozen; scored once)"), "planner-post-summary.json + 1 more");
  assert.equal(sourceLabel("unavailable: research write-up RESULTS-v2.md, not in this repository"), "not in this repository");
  assert.equal(sourceLabel("eval/results/baselines-final.json (go run ./cmd/eval baselines, no model)"), "baselines-final.json");
});
