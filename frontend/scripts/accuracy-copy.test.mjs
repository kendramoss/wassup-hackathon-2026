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
    "historical synthetic-image study",
    "It runs high",
    "Not a ruling. A licensed broker confirms",
    // #685: the pretraining caveat is visible without opening a fold.
    "Gemini may have seen some of these rulings in pretraining; we can’t measure how many.",
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
  // #685: plain label. The historical 0.92 bar on the model's own (stated)
  // confidence and today's 0.90 calibrated bar stay named side by side.
  assert.ok(pageText.includes(`Right when it was sure ${rate(fastPath.value)} ${autoRight} / ${fastPath.n} right when the model’s own confidence was ≥ 0.92, the bar at the time, and the case was in scope, so it was accepted without review. Today’s bar is 0.90 on calibrated confidence.`));
  assert.ok(pageText.includes(`Sent to human review ${rate(review / overall.n)} ${review} sent to review. The model had ${reviewRight} right and ${review - reviewRight} wrong.`));
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
  assert.match(markup, /href="#misses"[^>]*>See all 37 known failures/);
  assert.match(markup, /href="\/method"[^>]*>How it is built/);
  const failures = markup.match(/<section[^>]*aria-labelledby="miss-h"[\s\S]*?<\/section>/)?.[0];
  assert.ok(failures, "known failures section");
  assert.equal((failures.match(/<tbody>/g) || []).length, 1);
  assert.equal((failures.split("<tbody>")[1].split("</tbody>")[0].match(/<tr[ >]/g) || []).length, overall.n - correct);
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
  assert.ok(pageText.includes("CBP's starting origin supplied Plan precision"));
  assert.ok(pageText.includes("Repeat runs and intervals"));
  assert.ok(pageText.includes(`Strict count: ${strictPass}/${strictTotal}`));
});

test("repeat plan checks are labelled stability, with the distinct-pair interval and no pooled interval", async () => {
  const { AccuracyPage } = await import("../src/components/op/screens/accuracy-page.tsx");
  const pageText = text(html(AccuracyPage, {}));
  const start = pageText.indexOf("Production-move plan evaluation");
  const end = pageText.indexOf("Strict count:");
  assert.ok(start >= 0 && end > start);
  const section = pageText.slice(start, end);
  assert.doesNotMatch(section, /87\.5/);
  assert.match(section, /Stability/);
  assert.match(section, /27\/27/);
  assert.match(section, /9\/9 distinct pairs/);
  assert.ok(section.includes("70.1–100%"));
  assert.ok(section.includes("same 9 distinct pairs"));
  assert.match(section, /selected moves/);
  assert.match(section, /starting origin (was )?supplied/);
  assert.doesNotMatch(pageText, /87\.5–100%/);
});

test("RCEP reports known positives, criterion agreement and separate synthetic results", async () => {
  const { AccuracyPage } = await import("../src/components/op/screens/accuracy-page.tsx");
  const markup = html(AccuracyPage, {});
  const markets = text(markup.split('id="market-evidence"')[1].split("</section>")[0]);
  const { held_out: unseen, development } = JSON.parse(read("../../eval/rcep/japan-summary.json"));
  const { outcomes, deciding_criterion_agreement: criteria } = unseen.published_positives;

  assert.ok(markets.includes(`All ${outcomes.yes.total} unseen Japan Customs rulings qualified in the published decisions.`));
  assert.ok(markets.includes(`${outcomes.yes.count}/${outcomes.yes.total} yes ${outcomes.undetermined.count} undetermined`));
  assert.ok(markets.includes(`${outcomes.no.count} said no · ${outcomes.error.count} input errors`));
  assert.ok(markets.includes(`${criteria.count}/${criteria.total} deciding criteria matched`));
  assert.ok(markets.includes(`All ${development.synthetic_negatives.undetermined.count} development and ${unseen.synthetic_negatives.undetermined.count} unseen synthetic tariff-code variants were undetermined.`));
  assert.match(markets, /Refusal performance on real cases remains unmeasured/);
  assert.match(markets, /Customs specialist review is pending/);
  assert.match(markets, /Canada marking Checks the marking rules only\. No accuracy figure/);
  assert.match(markets, /EU and UK origin Checks the list rules only\. No accuracy figure/);
  assert.match(markets, /Japan model Open\. Right rule on 37 of 44 new rulings\./);
});

test("Accuracy page shows Japan, USMCA and ATIGA, and ATIGA is not called an accuracy rate", async () => {
  const { AccuracyPage } = await import("../src/components/op/screens/accuracy-page.tsx");
  const t = text(html(AccuracyPage, {}));
  assert.ok(t.includes("Japan: tested on new rulings"));
  assert.ok(t.includes("37/44"));
  assert.ok(t.includes("42/44"));
  assert.ok(t.includes("272 → 374/400"));
  assert.ok(t.includes("0/80"));
  assert.ok(t.includes("21/23"));
  assert.ok(t.includes("13/14"));
  assert.ok(t.includes("26/27"));
  assert.ok(t.includes("79/122"));
  assert.ok(t.includes("31/69"));
  assert.ok(t.includes("48/53"));
  assert.ok(t.includes("100% (7/7) match the declared outcomes or rule checks."));
  assert.ok(t.includes("Constructed rule checks: 100% (35/35)."));
  assert.ok(t.includes("Real-world accuracy is not measured."));
  assert.match(t, /Korea 8\/8\s*, China 3\/5 reach the stated result\. Not an accuracy rate\./);
  assert.ok(!t.includes("No accuracy figure. Details"));
});

test("the no-starting-origin plan bar and interval come from the committed repeats summary", async () => {
  const { AccuracyPage } = await import("../src/components/op/screens/accuracy-page.tsx");
  const pageText = text(html(AccuracyPage, {}));
  const runs = JSON.parse(read("../../eval/results/planner-post-repeats-summary.json")).modes["unknown-a"].per_run;
  const recalls = Object.values(runs).map((run) => run.recall_at_bar);
  assert.equal(recalls.length, 3);
  const [{ k, n, value, ci95 }] = recalls;
  for (const x of recalls) assert.deepEqual([x.k, x.n, x.ci95], [k, n, ci95]);
  assert.ok(pageText.includes(`Starting origin not supplied, same in each run Real shifts found ${k}/${n} (${Math.round(value * 100)}%)`));
  assert.ok(pageText.includes(`in each of the 3 runs: real shifts found ${(Math.round(ci95[0] * 1000) / 10).toFixed(1)}–${(Math.round(ci95[1] * 1000) / 10).toFixed(1)}%`));
  assert.ok(pageText.indexOf("CBP's starting origin supplied") < pageText.indexOf("Starting origin not supplied"));
  const { held_out: unseen } = JSON.parse(read("../../eval/rcep/japan-summary.json"));
  const c = unseen.published_positives.deciding_criterion_agreement;
  assert.ok(pageText.includes(`${c.count}/${c.total} deciding criteria matched.`));
});
