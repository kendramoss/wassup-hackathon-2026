import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createHash } from "node:crypto";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

test("the public bundle is byte-provenanced to selected completed source studies", async () => {
  const bundle = JSON.parse(readFileSync(new URL("../src/lib/challenge-data.json", import.meta.url), "utf8"));
  const index = JSON.parse(readFileSync(new URL("../../eval/challenges/accuracy-sources.json", import.meta.url), "utf8"));
  const { parseChallengeSummary } = await import("../src/lib/challenge-evidence.ts");
  assert.deepEqual(bundle.sources.map((s) => s.sourcePath), index.sources);
  const ids = new Set();
  for (const source of bundle.sources) {
    const raw = readFileSync(new URL(`../../${source.sourcePath}`, import.meta.url), "utf8");
    assert.equal(source.sourceSHA256, createHash("sha256").update(raw).digest("hex"));
    assert.deepEqual(source.summary, JSON.parse(raw));
    for (const f of parseChallengeSummary(source.summary, source.sourcePath)) {
      assert.ok(!ids.has(f.id), `repeated ${f.id}`);
      ids.add(f.id);
      if (f.id === "move_verifier") {
        const original = source.summary.features.find((item) => item.id === f.id);
        assert.equal(original.source_isolation_issue, true);
        assert.match(f.public_note, /not a source-clean holdout/);
      }
    }
  }
  assert.ok(ids.has("atiga"));
  assert.ok(ids.has("move_verifier"));
});

test("Accuracy leads the rule challenges with the larger selected studies", async () => {
  const { AccuracyPage } = await import("../src/components/op/screens/accuracy-page.tsx");
  const t = text(html(AccuracyPage, {}));
  assert.match(t, /Generated challenge tests/);
  assert.match(t, /168\/169/);
  assert.match(t, /147\/154/);
  assert.match(t, /210\/210/);
  assert.match(t, /Original authority examples/);
});

test("current model results do not substitute the stronger experimental textile profile", async () => {
  const { challengeStudies } = await import("../src/lib/challenge-studies.ts");
  const { ChallengeEvidence } = await import("../src/components/op/challenge-evidence.tsx");
  for (const [id, matched, cases] of [["textile_live_v2", 108, 114], ["pharma_specialist", 113, 113], ["part102_specialist", 103, 112], ["japan_live_unmodified", 104, 112], ["legal_routing", 425, 437]]) {
    const feature = challengeStudies.find((f) => f.id === id);
    assert.ok(feature, id);
    assert.deepEqual([feature.test.matched, feature.test.cases], [matched, cases], id);
  }
  assert.ok(!challengeStudies.some((f) => ["textile_specialist", "japan_engine"].includes(f.id)));
  const table = text(html(ChallengeEvidence, {features: challengeStudies}).split("</table>")[0]);
  assert.doesNotMatch(table, /usmca-v5|router-v4|textile-v2/);
  assert.match(table, /Decided correct, confidence ≥ 0.92 \(study\)/);
  assert.match(table, /U.S. marking origin/);
  assert.doesNotMatch(table, /Decided cases answered confidently and correctly/);
  assert.match(table, /potential HS-edition issues/);
});

test("larger image evidence retains strict misses, field counts and the sheet-size mix", async () => {
  const { challengeStudies } = await import("../src/lib/challenge-studies.ts");
  const { ChallengeEvidence } = await import("../src/components/op/challenge-evidence.tsx");
  const feature = challengeStudies.find((f) => f.id === "media_extraction");
  assert.ok(feature);
  assert.deepEqual([feature.test.matched, feature.test.cases, feature.test.errors], [129, 132, 0]);
  assert.equal(challengeStudies.length, 14);
  const table = text(html(ChallengeEvidence, {features: [feature]}).split("</table>")[0]);
  for (const value of ["629/632", "618/621", "21/24", "52/132", "not real photos"]) assert.ok(table.includes(value), value);
});

test("the small original image study is historical and closed by default", async () => {
  const { HistoricalMediaEvidence } = await import("../src/components/op/screens/accuracy-page.tsx");
  const section = html(HistoricalMediaEvidence, {});
  assert.match(section, /aria-labelledby="acc-media"/);
  const fold = section.match(/<details([^>]*)>[\s\S]*?<\/details>/);
  assert.ok(fold);
  assert.match(text(fold[0]), /Original 20-image study/);
  assert.doesNotMatch(fold[1], /\bopen\b/);
  assert.match(text(fold[0]), /151 of 170/);
  assert.match(text(section.split("<details")[0]), /Real photos, scans and handwriting are not measured yet/);
});

const fixture = () => ({
  schema_version: 1,
  evidence_kind: "source_grounded_synthetic",
  features: [{
    id: "example_rules", label: "Example rules", candidate_revision: "a".repeat(40), dataset_sha256: "b".repeat(64),
    source_families: 20, development: { cases: 30, matched: 28, errors: 0 },
    test: { cases: 120, matched: 116, mismatched: 3, errors: 1 },
    cost_usd: 0, latency_p50_ms: 1, latency_p95_ms: 2,
    limitations: ["Synthetic variations; not independent customs decisions."],
  }],
});

test("challenge evidence preserves the complete denominator and provenance", async () => {
  const { parseChallengeSummary, challengePercent } = await import("../src/lib/challenge-evidence.ts");
  const [f] = parseChallengeSummary(fixture(), "eval/challenges/example/summary.json");
  assert.equal(f.test.cases, 120);
  assert.equal(f.test.errors, 1);
  assert.equal(f.test.matched, 116);
  assert.equal(f.source_families, 20);
  assert.equal(f.sourcePath, "eval/challenges/example/summary.json");
  assert.equal(challengePercent(f), "96.7%");
});

test("challenge table identifies generated evidence, counts every error and links its records", async () => {
  const { parseChallengeSummary } = await import("../src/lib/challenge-evidence.ts");
  const { ChallengeEvidence } = await import("../src/components/op/challenge-evidence.tsx");
  const features = parseChallengeSummary(fixture(), "eval/challenges/example/summary.json");
  const markup = html(ChallengeEvidence, { features });
  const t = text(markup);
  assert.match(t, /Generated challenge tests/);
  assert.match(t, /96\.7%/);
  assert.match(t, /116\/120/);
  assert.match(t, /1 error included/);
  assert.match(t, /real-case studies/);
  assert.match(t, /requests for missing facts/);
  assert.match(t, /20 source groups/);
  assert.ok(markup.includes("eval/challenges/example/summary.json"));
  assert.doesNotMatch(t, /independent test cases|real-world accuracy|95% confidence/i);
  assert.equal(html(ChallengeEvidence, { features: [] }), "");
});

test("explicit current-path evidence replaces experimental rows and keeps field units", async () => {
  const { parseChallengeSummary } = await import("../src/lib/challenge-evidence.ts");
  const { ChallengeEvidence } = await import("../src/components/op/challenge-evidence.tsx");
  const data = fixture();
  data.features[0].id = "old_experiment";
  data.public_features = [{ ...fixture().features[0], id: "current_path", display_metrics: [{id: "names", label: "Exact name matches", num: 629, den: 632}] }];
  const features = parseChallengeSummary(data, "eval/challenges/example/summary.json");
  assert.deepEqual(features.map((f) => f.id), ["current_path"]);
  assert.match(text(html(ChallengeEvidence, { features })), /Exact name matches 629\/632/);
  data.public_features[0].display_metrics[0].num = 633;
  assert.throws(() => parseChallengeSummary(data, "eval/challenges/example/summary.json"));
  data.public_features = [];
  assert.throws(() => parseChallengeSummary(data, "eval/challenges/example/summary.json"));
  data.public_features = null;
  assert.throws(() => parseChallengeSummary(data, "eval/challenges/example/summary.json"));
});

test("source limitations and authoring defects are readable without leaving Accuracy", async () => {
  const { parseChallengeSummary } = await import("../src/lib/challenge-evidence.ts");
  const { ChallengeEvidence } = await import("../src/components/op/challenge-evidence.tsx");
  const data = fixture();
  data.features[0].error_note = "The retained errors are invalid authoring inputs, not engine defects.";
  const features = parseChallengeSummary(data, "eval/challenges/example/summary.json");
  const t = text(html(ChallengeEvidence, { features }));
  assert.ok(t.includes(data.features[0].limitations[0]));
  assert.ok(t.includes(data.features[0].error_note));
});

test("move verification cannot hide low coverage behind conditional precision", async () => {
  const { parseChallengeSummary } = await import("../src/lib/challenge-evidence.ts");
  const { ChallengeEvidence } = await import("../src/components/op/challenge-evidence.tsx");
  const data = fixture();
  const f = data.features[0];
  Object.assign(f, { id: "move_verifier", test: {cases: 220, matched: 199, mismatched: 20, errors: 1}, shown_precision: {num: 47, den: 47}, positive_recall: {num: 47, den: 100}, negative_controls: {num: 99, den: 100}, incomplete_controls: {num: 20, den: 20}, raw_metrics: {cases: 220, matched: 199, mismatched: 20, errors: 1, positive_cases: 100, negative_cases: 100, uncertain_cases: 20, true_positive: 47, false_positive: 0, false_negative: 53, true_negative: 99} });
  const features = parseChallengeSummary(data, "eval/challenges/example/summary.json");
  const t = text(html(ChallengeEvidence, { features }));
  for (const value of ["Shown moves correct 47/47", "Valid changes shown 47/100", "Unchanged controls withheld 99/100", "Missing-fact controls withheld 20/20"]) assert.ok(t.includes(value), value);
  delete f.positive_recall;
  assert.throws(() => parseChallengeSummary(data, "eval/challenges/example/summary.json"));
  f.positive_recall = {num: 101, den: 100};
  assert.throws(() => parseChallengeSummary(data, "eval/challenges/example/summary.json"));
});

test("verifier conditional metrics must agree with their original raw counts", async () => {
  const { parseChallengeSummary } = await import("../src/lib/challenge-evidence.ts");
  const { ChallengeEvidence } = await import("../src/components/op/challenge-evidence.tsx");
  const original = JSON.parse(readFileSync(new URL("../../eval/challenges/verifier/summary.json", import.meta.url), "utf8"));
  const features = parseChallengeSummary(original, "eval/challenges/verifier/summary.json");
  const visibleTable = text(html(ChallengeEvidence, { features }).split("</table>")[0]);
  assert.match(visibleTable, /starting origin supplied/);
  assert.match(visibleTable, /not plan search/);
  for (const mutate of [
    (f) => { f.positive_recall.den = 47; },
    (f) => { f.shown_precision.den = 100; },
    (f) => { f.negative_controls.num = 100; },
  ]) {
    const data = structuredClone(original); mutate(data.features[0]);
    assert.throws(() => parseChallengeSummary(data, "eval/challenges/verifier/summary.json"));
  }
});

test("source-isolation qualifications cannot disappear from the public table", async () => {
  const { parseChallengeSummary } = await import("../src/lib/challenge-evidence.ts");
  const { ChallengeEvidence } = await import("../src/components/op/challenge-evidence.tsx");
  const data = fixture();
  data.features[0].source_isolation_issue = true;
  assert.throws(() => parseChallengeSummary(data, "eval/challenges/example/summary.json"));
  data.features[0].public_note = "Related source rulings remained available; not a clean holdout.";
  const features = parseChallengeSummary(data, "eval/challenges/example/summary.json");
  assert.match(text(html(ChallengeEvidence, { features })), /Related source rulings remained available; not a clean holdout/);
});

test("challenge evidence rejects inflated, incomplete or mislabeled headline studies", async () => {
  const { parseChallengeSummary } = await import("../src/lib/challenge-evidence.ts");
  for (const mutate of [
    (x) => { x.evidence_kind = "real_world_accuracy"; },
    (x) => { x.features[0].test.cases = 7; x.features[0].test.matched = 7; x.features[0].test.errors = 0; x.features[0].test.mismatched = 0; },
    (x) => { x.features[0].test.matched = 120; },
    (x) => { x.features[0].test.errors = -1; },
    (x) => { x.features[0].candidate_revision = "uncommitted"; },
    (x) => { x.features[0].dataset_sha256 = ""; },
    (x) => { x.features.push(x.features[0]); },
    (x) => { x.features[0].limitations = []; },
    (x) => { x.features[0].source_families = 0; },
    (x) => { x.features[0].simulated = true; },
    (x) => { x.features[0].llm_provider = "fake"; },
    (x) => { x.features[0].llm_provider = "vertex"; },
  ]) {
    const data = fixture(); mutate(data);
    assert.throws(() => parseChallengeSummary(data, "eval/challenges/example/summary.json"));
  }
  assert.throws(() => parseChallengeSummary(fixture(), "../../private.json"));
});
