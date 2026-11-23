import assert from "node:assert/strict";
import { readFileSync, readdirSync } from "node:fs";
import { test } from "node:test";

// #723: the 377-ruling benchmark is scoped beside its headline, the
// "never saw / never used for tuning" claims are gone, and the historical
// 0.92 stated-confidence selection is kept apart from the current 0.90
// calibrated serving threshold. D32 and D260 stay as written.
const read = (p) => readFileSync(new URL(p, import.meta.url), "utf8");
const flat = (p) => read(p).replace(/\s+/g, " ");
const page = flat("../src/components/op/screens/accuracy-page.tsx");
const copy = flat("../src/lib/copy.ts");
const evaluation = flat("../../docs/EVALUATION.md");
const details = flat("../../docs/DETAILS.md");
const decisions = read("../../docs/DECISIONS.md");

function walk(dir) {
  return readdirSync(new URL(dir, import.meta.url), { withFileTypes: true }).flatMap((e) =>
    e.isDirectory() ? walk(`${dir}${e.name}/`) : /\.(tsx?|mjs)$/.test(e.name) ? [`${dir}${e.name}`] : [],
  );
}

test("the accuracy intro names the scope next to the headline", () => {
  assert.match(page, /single-origin, multi-country U\.S\. substantial-transformation rulings/);
  assert.match(page, /not accuracy across every market or rule set/);
});

test("the unsupported 'never saw' and 'never used for tuning' claims are gone from copy", () => {
  for (const f of walk("../src/")) {
    const s = flat(f);
    if (f.endsWith("schema.d.ts")) continue; // API field description, not a benchmark claim
    assert.ok(!/never saw|never used for tuning/i.test(s), f);
  }
  assert.ok(!/it never saw/i.test(copy));
  assert.ok(!/never used for tuning/i.test(copy));
  for (const [name, s] of [["EVALUATION", evaluation], ["DETAILS", details], ["README", flat("../../README.md")]]) {
    assert.ok(!/never used for tuning|it never saw|never used to develop prompts/i.test(s), name);
  }
});

test("the replacement wording separates retrieval, prompts, pretraining and threshold history", () => {
  assert.match(copy, /fig3Title: "Tested on 377 CBP rulings, scored once"/);
  assert.match(copy, /prompts not tuned on them/);
  assert.match(page, /Gemini’s pretraining exposure is unknown/);
  assert.match(page, /original threshold selection also considered these rulings/);
});

test("0.92 historical stated confidence and 0.90 current calibrated threshold stay distinct", () => {
  assert.match(page, /original threshold selection also considered these rulings\. The current displayed threshold comes from development rulings/);
  assert.match(evaluation, /historical stated-confidence bar of 0\.92 was chosen with a rule that looked at both splits/);
  assert.match(evaluation, /current 0\.90 calibrated-confidence threshold was derived from development data/);
  assert.ok(!/chosen on the proxy dev run; held-out confirmed it and was not used to search for it/.test(evaluation));
  assert.ok(!/0\.92 is the lowest stated bar at which accuracy stays at or above 95% on both splits \(D32\)\. /.test(details) || /both splits/.test(details));
  assert.match(details, /historical stated 0\.92 bar \(D32\) looked at both splits/);
});

test("the headline figures stay", () => {
  assert.match(flat("../../docs/NUMBERS.md"), /340\/377 right overall/);
  assert.match(details, /288 of 300/);
  assert.match(details, /377/);
  assert.match(flat("../../README.md"), /377-ruling/);
});

test("D32 and D260 keep their original text, with a dated current-status note after them", () => {
  assert.match(decisions, /The user chose the lowest confidence at which cumulative accuracy stays at or above 95% on both splits\./);
  assert.match(decisions, /Held-out confirmed the dev choice; it was not used to search for the bar\./);
  assert.match(decisions, /the first 0\.92 bar was chosen with a rule that looked at both splits \(D32; the 0\.90 calibrated bar now in use was re-derived on dev only, D160\)/);
  assert.match(decisions, /\*\*Correction:\*\* \/accuracy's "How it's built" said the held-out rulings were never used to tune thresholds\./);
  const note = decisions.indexOf("Current-status note, 23 Nov 2026 (#723)");
  assert.ok(note > decisions.indexOf("## D260"), "note comes after D260");
  assert.match(decisions.slice(note), /D32/);
  assert.match(decisions.slice(note), /D260/);
  assert.match(decisions.slice(note), /EVALUATION\.md/);
  assert.match(flat("../src/components/op/screens/method-page.tsx"), /Zero production fine-tuning/);
});

test("prospective rules stay", () => {
  assert.match(read("../../docs/DETAILS.md"), /never tune on it/);
});
