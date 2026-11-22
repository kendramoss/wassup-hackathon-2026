import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

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
