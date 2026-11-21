import assert from "node:assert/strict";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

// D363: the 19 CFR 102.20 rule check panel. Absent unless the API sends it;
// says it is display only; flags a decided check that differs from the model.
const { RuleCheckPanel, headline } = await import("../src/components/op/rule-check.tsx");

const base = {
  rules: "19 CFR 102.20",
  applies: "part102",
  country: "Mexico",
  country_from: "final assembly plant",
  model_origin: "Mexico",
  review: false,
  note: "n",
  result: {
    status: "decided",
    origin: "Mexico",
    basis: "19 CFR 102.11(a)(3); 19 CFR 102.20(o), 8473",
    reason: "each foreign material undergoes the change in tariff classification the rule requires, in Mexico",
    rule: { id: "19 CFR 102.20(o), 8473", hts: "8473", text: "A change to heading 8473 from any other heading.", alternatives: 1, encoded: 1 },
    materials: [{ name: "aluminum fins", hts: "7616.99", origin: "China", foreign: true, status: "shift", alternative: 1 }],
  },
};

test("nothing without a rule_check", () => {
  assert.equal(html(RuleCheckPanel, { c: { id: "x" } }), "");
});

test("a decided check names the rule, the basis and that it changes nothing", () => {
  const m = html(RuleCheckPanel, { c: { id: "x", rule_check: base } });
  const t = text(m);
  assert.match(t, /Rule check \(19 CFR 102\.20\)/);
  assert.match(t, /Mexico under the tariff-shift rules, from the codes given\./);
  assert.match(t, /19 CFR 102\.11\(a\)\(3\)/);
  assert.match(t, /does not change the answer above or what is auto-accepted/);
  assert.match(t, /aluminum fins · 7616\.99 · China · shifts/);
  assert.match(m, /aria-label="Rule check \(19 CFR 102\.20\)"/);
  assert.doesNotMatch(m, /bg-ask-wash/);
});

test("a decided check that differs from the model is flagged, in the ask tone", () => {
  const rc = { ...base, review: true, agrees: false, model_origin: "China" };
  const m = html(RuleCheckPanel, { c: { id: "x", rule_check: rc } });
  assert.match(text(m), /The model said China: worth a broker's look\./);
  assert.match(m, /bg-ask-wash/);
  assert.equal(headline(rc).tone, "ask");
});

test("undetermined and de minimis read as open, never as an answer", () => {
  const open = { ...base, result: { status: "undetermined", reason: "the shift cannot be decided for fins; see what each needs", materials: [] } };
  assert.equal(headline(open).text, "Not decided from the codes given.");
  const dm = { ...base, result: { status: "undetermined", reason: "r", if_not_de_minimis: { origin: "China", basis: "b", condition: "c" } } };
  assert.match(headline(dm).text, /Open on value: Mexico if the parts that do not shift are worth no more than 7% of the good, otherwise China\./);
  const info = text(html(RuleCheckPanel, { c: { id: "x", rule_check: { ...base, applies: "informational" } } }));
  assert.match(info, /decided under substantial transformation; the Part 102 check is for comparison only/);
});
