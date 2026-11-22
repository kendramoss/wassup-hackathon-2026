import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

// Copy sweep 2 (#669, rule 16): /accuracy is written for a first-time
// visitor. Jargon moved into Details folds, but every honesty label must stay
// visible in its shorter form. These pins fail if a later edit drops one.
const read = (p) => readFileSync(new URL(p, import.meta.url), "utf8").replace(/\s+/g, " ");
const page = read("../src/components/op/screens/accuracy-page.tsx");
const check = read("../src/components/op/verifier-calibration.tsx");
const misses = read("../src/components/op/failure-cases.tsx");

test("the run labels stay on the page: official run, not the official run, mixed runs", () => {
  assert.match(page, /<Chip tone="ok">official run<\/Chip>/);
  assert.match(page, /<Chip>not the official run<\/Chip>/);
  assert.match(page, /<Chip>mixed runs<\/Chip>/);
  assert.match(check, /<Chip>not the official run<\/Chip>/);
  assert.match(misses, /<Chip tone="ok">official run<\/Chip>/);
  // The gateway is named once, in a Details fold, with its parity check.
  assert.match(page, /<p id="gateway"/);
  assert.match(page, /OpenAI-compatible gateway, not Vertex AI/);
});

test("the honesty lines stay outside the Details folds", () => {
  const visible = page.split("<Details>").map((s, i) => (i === 0 ? s : s.slice(s.indexOf("</Details>")))).join(" ");
  for (const s of [
    "reported, not reproducible here",
    "not that it improved",
    "Partly tuned on newer rulings, so not a clean test",
    "Partly tuned on these rulings, so not a clean test",
    "Real photos, scans and handwriting are not measured yet",
    "synthetic images, illustrative · small",
    "It runs high",
    "Not a ruling. A licensed broker confirms",
    "copy.jurisdiction.notCovered",
    "copy.jurisdiction.noDetector",
  ])
    assert.ok(visible.includes(s), s);
  assert.match(check, /The 2026 pairs are not a clean test\./);
});

test("the headline uses plain names, not internal ids", () => {
  const visible = page.split("<Details>").map((s, i) => (i === 0 ? s : s.slice(s.indexOf("</Details>")))).join(" ");
  const jsx = visible.slice(visible.indexOf("return ( <Main>"));
  assert.doesNotMatch(jsx, /held-out|McNemar|usmca-v\d|router-v\d|engine-v1|calibration error/i);
});

const summary = JSON.parse(read("../../backend/internal/evalsummary/summary.json"));
const items = new Map(summary.items.map((it) => [it.key, it]));
const rate = (n) => `${(n * 100).toFixed(1)}%`;

test("the first render shows the official counts before the API loaded", async () => {
  const { AccuracyPage } = await import("../src/components/op/screens/accuracy-page.tsx");
  const markup = html(AccuracyPage, {});
  const pageText = text(markup);
  const overall = items.get("final_accuracy");
  const fastPath = items.get("final_fastpath_accuracy");
  const correct = Math.round(overall.value * overall.n);
  const autoRight = Math.round(fastPath.value * fastPath.n);
  const review = overall.n - fastPath.n;
  const reviewRight = correct - autoRight;

  assert.ok(pageText.includes(`Overall accuracy ${rate(overall.value)} ${correct} / ${overall.n} right`));
  assert.ok(pageText.includes(rate(items.get("baseline_first_mentioned").value)));
  assert.ok(pageText.includes(`Auto-accepted accuracy ${rate(fastPath.value)} ${autoRight} / ${fastPath.n} right at ≥ 0.92`));
  assert.ok(pageText.includes(`Human escalation rate ${rate(review / overall.n)} ${review} sent to review. The model had ${reviewRight} right and ${review - reviewRight} wrong.`));
  assert.doesNotMatch(pageText, /Loading the numbers|How it's built/);

  const groupCounts = new Map([
    ["auto-right", autoRight],
    ["auto-wrong", fastPath.n - autoRight],
    ["review-right", reviewRight],
    ["review-wrong", review - reviewRight],
  ]);
  for (const [key, count] of groupCounts) {
    assert.equal([...markup.matchAll(new RegExp(`data-outcome="${key}"`, "g"))].length, count, key);
  }
  assert.match(markup, /href="#misses"[^>]*>Inspect all 37 known failures/);
  assert.match(markup, /href="\/method"[^>]*>See system architecture/);
  assert.equal((markup.match(/<tbody>/g) || []).length, 1);
  assert.equal((markup.split("<tbody>")[1].split("</tbody>")[0].match(/<tr[ >]/g) || []).length, overall.n - correct);
});

test("the recorded plan bars match the committed known-starting-origin results", async () => {
  const { AccuracyPage } = await import("../src/components/op/screens/accuracy-page.tsx");
  const pageText = text(html(AccuracyPage, {}));
  for (const [label, name] of [
    ["Plan precision", "precision_at_bar"],
    ["Real shifts found", "recall_at_bar"],
    ["False positives on unchanged goods", "controls_shown"],
  ]) {
    const { value, n } = items.get(`planner_post_combined_known-a_${name}`);
    assert.ok(pageText.includes(`${label} ${Math.round(value * n)}/${n} (${Math.round(value * 100)}%)`));
  }
  const historicalSplits = summary.verifier_calibration.splits;
  const strictPass = historicalSplits.reduce((k, r) => k + r.pair_level.shown_every_run, 0);
  const strictTotal = historicalSplits.reduce((n, r) => n + r.pair_level.real_change_pairs, 0);
  assert.ok(pageText.includes("CBP's starting origin was supplied."));
  assert.ok(pageText.includes("Strict Multi-Run & Interval Details"));
  assert.ok(pageText.includes(`Strict count: ${strictPass}/${strictTotal}`));
});
