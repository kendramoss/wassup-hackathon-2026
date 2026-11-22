import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { kobalt, methodExperiments, methodStages } from "../src/lib/method-inspector.ts";

const read = (path) => readFileSync(new URL(path, import.meta.url), "utf8").replace(/\s+/g, " ");
const page = read("../src/components/op/screens/method-page.tsx");
const architecture = read("../src/components/op/method-architecture.tsx");

test("Method uses a connected stage inspector and comparison cards without text dumps", () => {
  assert.doesNotMatch(page + architecture, /<(?:details|ul|ol|Fold|Details)\b/i);
  assert.match(architecture, /role="tablist"[^>]*aria-orientation="vertical"/);
  assert.match(architecture, /aria-controls="architecture-detail"/);
  assert.match(architecture, /role="tabpanel"/);
  assert.equal(methodStages.length, 6);
  assert.ok(methodStages.every((stage) => stage.input.length && stage.output.length && stage.guard && stage.trace.length));
  assert.equal(methodExperiments.length, 3);
  assert.match(architecture, /aria-pressed=\{!fixed\} onClick=\{\(\) => setFixed\(false\)\}>Failed approach/);
  assert.match(architecture, /aria-pressed=\{fixed\} onClick=\{\(\) => setFixed\(true\)\}>Production fix/);
});

test("Method scopes its temporal claim and keeps the 2022 and 2026 dates distinct", () => {
  assert.match(page, /Zero production fine-tuning/);
  assert.match(page, /Gemini’s pretraining exposure is unknown/);
  assert.match(page, /BM25 word counts span the full corpus/);
  assert.doesNotMatch(page, /zero training leakage/i);
  assert.match(page, /Illustrated 2022 cutoff/);
  assert.match(page, /Same-day rulings are blocked\..*The saved demo uses \{kobalt\.cutoff\.label\}/);
  assert.ok(kobalt.setupRuling.date < kobalt.supporting.date);
  assert.ok(kobalt.supporting.date < kobalt.cutoff.date);
  assert.ok(kobalt.cutoff.date < kobalt.backtestRuling.date);
});

test("The archived and current confidence gates stay distinct", () => {
  assert.match(methodStages[2].guard, /saved run used stated ≥0\.92/);
  assert.match(methodStages[2].guard, /in-scope cases at calibrated ≥0\.90/);
  assert.match(methodStages[4].guard, /eligible support and scores at least 0\.92/);
  assert.match(page, /Only CBP’s ruling binds/);
  assert.match(page, /No qualifying counter-ruling recorded/);
  assert.match(page, /It was not a blind test/);
});
