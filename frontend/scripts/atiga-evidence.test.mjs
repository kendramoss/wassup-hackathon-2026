import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

const read = (p) => readFileSync(new URL(p, import.meta.url), "utf8");
const result = JSON.parse(read("../../eval/atiga/evaluation-results.json"));
const cases = read("../../eval/atiga/evaluation-cases.json");

test("ATIGA figures retain separate authority and constructed denominators", async () => {
  const { rulesChecks: r } = await import("../src/lib/market-evidence.ts");
  assert.equal(result.dataset_sha256, createHash("sha256").update(cases).digest("hex"));
  assert.equal(r.atigaExamples, result.groups.authority_example.cases);
  assert.equal(r.atigaReached, result.groups.authority_example.passed);
  assert.equal(r.atigaPct, `${100 * r.atigaReached / r.atigaExamples}%`);
  assert.equal(r.atigaControls, result.groups.synthetic_control.cases);
  assert.equal(r.atigaControlsPassed, result.groups.synthetic_control.passed);
  assert.equal(r.atigaControlsPct, `${100 * r.atigaControlsPassed / r.atigaControls}%`);
  for (const [kind, counts] of Object.entries(result.groups)) {
    const rows = result.results.filter((c) => c.kind === kind);
    assert.equal(rows.length, counts.cases);
    assert.equal(rows.filter((c) => c.passed).length, counts.passed);
    assert.equal(rows.filter((c) => !c.passed).length, counts.failed);
  }
});

test("Accuracy renders ATIGA pass rates without claiming measured real-world accuracy", async () => {
  const { AccuracyPage } = await import("../src/components/op/screens/accuracy-page.tsx");
  const rendered = html(AccuracyPage, {});
  const pageText = text(rendered);
  assert.match(pageText, /ASEAN \(ATIGA\) worked-example agreement/);
  assert.match(pageText, /100% \(7\/7\)/);
  assert.match(pageText, /Constructed rule checks: 100% \(35\/35\)/);
  assert.match(pageText, /Real-world accuracy is not measured/);
  assert.match(pageText, /not independent customs decisions/);
  assert.ok(rendered.includes("eval/atiga/evaluation-results.json"));
  assert.doesNotMatch(pageText, /no ASEAN Member State publishes origin rulings/);
});
