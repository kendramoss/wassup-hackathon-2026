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
  // #685: the rule is named in plain words beside the headline, with its
  // legal name (substantial transformation) in the term's explanation.
  assert.match(page, /compared with 377 real CBP rulings, Jan–Sep 2026\. Covers single-origin U\.S\. rulings under\{" "\} <Term explain="Substantial transformation: [^"]+">CBP’s main origin test<\/Term> only, not every market or rule set\./);
});

test("the unsupported 'never saw' and 'never used for tuning' claims are gone from copy", () => {
  for (const f of walk("../src/")) {
    const s = flat(f);
    if (f.endsWith("schema.d.ts")) continue; // API field description, not a benchmark claim
    assert.ok(!/never saw|never used for tuning/i.test(s), f);
  }
  assert.match(copy, /fig3Meta: "Jan–Sep 2026 · single-origin, multi-country U\.S\. rulings"/);
  assert.match(copy, /fig3Caveat: "Prompts were not tuned on these rulings\. The old 0\.92 stated bar looked at both test splits; the 0\.90 calibrated bar now used came from development data\."/);
  assert.match(flat("../src/components/op/landing/landing.tsx"), /\{L\.fig3Caveat\}/);
  assert.ok(!/it never saw/i.test(copy));
  assert.ok(!/never used for tuning/i.test(copy));
  for (const [name, s] of [["EVALUATION", evaluation], ["DETAILS", details], ["README", flat("../../README.md")]]) {
    assert.ok(!/never used for tuning|it never saw|never used to develop prompts/i.test(s), name);
  }
});

test("held-out labels do not say 'unseen' in the README, DETAILS or NUMBERS origin rows", () => {
  const numbers = read("../../docs/NUMBERS.md").split("\n").filter((l) => !/probe set 2|One-time unseen test/.test(l)).join("\n");
  for (const [name, s] of [["README", read("../../README.md")], ["DETAILS", read("../../docs/DETAILS.md")], ["NUMBERS", numbers]]) {
    assert.ok(!/unseen|never saw/i.test(s), name);
  }
});

test("the replacement wording separates retrieval, prompts, pretraining and threshold history", () => {
  assert.match(copy, /fig3Title: "Tested on 377 real past decisions, scored once"/);
  assert.match(page, /Gemini’s pretraining exposure is unknown/);
  assert.match(page, /original threshold selection also considered these rulings/);
});

test("0.92 historical stated confidence and 0.90 current calibrated threshold stay distinct", () => {
  assert.match(page, /original threshold selection also considered these rulings\. The current displayed threshold comes from development rulings/);
  assert.match(evaluation, /historical stated-confidence bar of 0\.92 was chosen with a rule that looked at both splits/);
  assert.match(evaluation, /current 0\.90 calibrated-confidence threshold was derived from development data/);
  assert.ok(!/chosen on the proxy dev run; held-out confirmed it and was not used to search for it/.test(evaluation));
  assert.match(flat("../../README.md"), /historical stated 0\.92 confidence bar also looked at the held-out rulings, D32; the 0\.90 calibrated bar now in use came from development rulings, D160/);
  assert.match(evaluation, /Held out means not used to tune prompts or retrieval; the historical 0\.92 bar also looked at these rulings \(D32, D260\)/);
  assert.ok(!/not used to tune anything/.test(evaluation));
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
  assert.match(decisions, /## D733 — Current status of the confidence-threshold history/);
  assert.match(decisions, /\*\*Correction:\*\* \/accuracy's "How it's built" said the held-out rulings were never used to tune thresholds\./);
  const note = decisions.indexOf("Current-status note, 23 Nov 2026 (#723)");
  assert.ok(note > decisions.indexOf("## D260"), "note comes after D260");
  assert.match(decisions.slice(note), /D32/);
  assert.match(decisions.slice(note), /D260/);
  assert.match(decisions.slice(note), /EVALUATION\.md/);
  // #685: plain wording, still scoped to the live (production) model.
  assert.match(flat("../src/components/op/screens/method-page.tsx"), /The live service uses Gemini without fine-tuning\./);
});

test("prospective rules stay", () => {
  assert.match(read("../../docs/DETAILS.md"), /never tune on it/);
});
