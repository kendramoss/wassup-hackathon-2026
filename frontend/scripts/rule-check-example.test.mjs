import { html, text } from "./render-support.mjs";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { stubNavigation } from "./lib/hook-runtime.mjs";

// Judge panel round 8, N11: the demo tour reached neither the per-part codes
// nor the rule check, because the demo case has no part codes. The tour now
// ends on a recorded example: one development ruling's committed suggestions
// and the real rule check's result on them, labelled recorded, read only.
stubNavigation({ id: "case_x", search: "tour=1&from=case_878d76540b91" });
const { RuleCheckExample } = await import("../src/components/op/screens/rule-check-example.tsx");
const { DemoTour } = await import("../src/components/op/demo-tour.tsx");
const ex = JSON.parse(readFileSync(new URL("../src/lib/rule-check-example.json", import.meta.url), "utf8"));

test("the example is a development ruling whose check decides only with the suggested codes", () => {
  assert.equal(ex.split, "dev");
  assert.notEqual(ex.before.status, "decided");
  assert.equal(ex.after.status, "decided");
  assert.equal(ex.after.origin, ex.cbp_origin);
  assert.match(ex.run, /^eval\/tariffshift\/parts\/suggest-dev-v1\.jsonl$/);
});

test("the page shows the recorded suggestions, the check before and after, and CBP's answer", () => {
  const m = html(RuleCheckExample, {});
  const t = text(m);
  assert.match(t, /Recorded example/);
  assert.match(t, /recorded: nothing here calls a model or saves anything/);
  assert.match(t, new RegExp(ex.product));
  for (const p of ex.parts) {
    assert.ok(t.includes(p.name), p.name);
    assert.ok(t.includes(p.suggestions[0].hts), p.suggestions[0].hts);
  }
  assert.match(t, /earlier CBP ruling/);
  assert.match(t, /Unverified suggestions/);
  assert.match(t, /With only the codes the ruling's text states Rule check \(19 CFR 102\.20\): add HTS codes for 1 part to run it\. \(flat coiled stainless steel\)/);
  assert.match(t, /Canada under the tariff-shift rules, from the codes given\./);
  assert.match(t, /The check agrees with CBP's ruling\./);
  // Judge panel round 9, N2: the top code for the strip is for widths of
  // 600 mm or more; the page says so, and that the answer is the same under
  // the second code. The recorded suggestions are not edited.
  assert.equal(ex.parts[0].suggestions[0].hts, "7219.33");
  assert.equal(ex.robustness.stable, true);
  assert.match(t, /Same answer for every suggested code: 2 combinations of the codes suggested for flat coiled stainless steel\./);
  assert.match(t, /Our note, added after the run: the top suggestion, 7219\.33, is for stainless steel 600 mm wide or more\./);
  assert.match(t, /7220\.20 \(under 600 mm\), is the likely code/);
  assert.match(t, /On most Part 102 rulings it still does not decide/);
  assert.match(m, /href="\/cases\/case_878d76540b91\/draft\?tour=1"/, "back to the tour");
  // Read only: nothing to press that writes, no form, no "Add codes" link.
  assert.doesNotMatch(m, /<form|<button|Add codes|cases\/new/);
});

test("the tour's last step points at the example", () => {
  const m = html(DemoTour, { id: "case_878d76540b91", tab: "draft" });
  assert.match(m, /href="\/examples\/rule-check\?tour=1&amp;from=case_878d76540b91"/);
  assert.match(text(m), /Next: per-part codes and the rule check \(recorded example\) →/);
});

// Judge panel round 9, N3 and N4: /method and /accuracy link the example, and
// /method gives the held-out figure without the inputs that carried a code.
test("/method and /accuracy link the recorded rule check and the evaluation keeps both figures", () => {
  const src = (file) => readFileSync(new URL(`../src/components/op/screens/${file}`, import.meta.url), "utf8");
  const numbers = readFileSync(new URL("../../docs/NUMBERS.md", import.meta.url), "utf8");
  assert.match(src("method-page.tsx"), /href="\/examples\/rule-check"/);
  assert.match(src("accuracy-page.tsx"), /href="\/examples\/rule-check"/);
  assert.match(numbers, /11 of 24 decided, 10 agree/);
  assert.match(numbers, /without the inputs that carried a code 9 of 24 \(8 agree\)/);
});
