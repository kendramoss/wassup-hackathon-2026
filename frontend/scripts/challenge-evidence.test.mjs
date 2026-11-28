import assert from "node:assert/strict";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

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
  ]) {
    const data = fixture(); mutate(data);
    assert.throws(() => parseChallengeSummary(data, "eval/challenges/example/summary.json"));
  }
  assert.throws(() => parseChallengeSummary(fixture(), "../../private.json"));
});
