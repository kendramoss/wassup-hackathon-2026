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
  Object.assign(f, { id: "move_verifier", shown_precision: {num: 47, den: 47}, positive_recall: {num: 47, den: 100}, negative_controls: {num: 99, den: 100}, incomplete_controls: {num: 20, den: 20} });
  const features = parseChallengeSummary(data, "eval/challenges/example/summary.json");
  const t = text(html(ChallengeEvidence, { features }));
  for (const value of ["Shown moves correct 47/47", "Valid changes shown 47/100", "Unchanged controls withheld 99/100", "Missing-fact controls withheld 20/20"]) assert.ok(t.includes(value), value);
  delete f.positive_recall;
  assert.throws(() => parseChallengeSummary(data, "eval/challenges/example/summary.json"));
  f.positive_recall = {num: 101, den: 100};
  assert.throws(() => parseChallengeSummary(data, "eval/challenges/example/summary.json"));
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
