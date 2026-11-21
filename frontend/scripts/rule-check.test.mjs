import assert from "node:assert/strict";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

// D363 display states (coordinator review of #544): the full panel only when
// the check decided; a compact line otherwise; on a Part 102 case without
// part codes, a line saying how to run it; on a substantial-transformation
// case without part codes, nothing.
const { RuleCheckPanel, ruleCheckDisplay, headline } = await import("../src/components/op/rule-check.tsx");

const decided = {
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
const rule = decided.result.rule;
const product = (hts, parts) => ({ name: "p", hts, components: parts.map((h, i) => ({ name: `part ${i}`, made_in: "China", ...(h ? { hts: h } : {}) })) });
const part102 = { router: { regime: "usmca_part102" } };
const st = { router: { regime: "substantial_transformation" } };

test("decided and agreeing: the full panel, calm", () => {
  const c = { id: "x", rule_check: decided };
  assert.equal(ruleCheckDisplay(c).kind, "panel");
  const m = html(RuleCheckPanel, { c });
  const t = text(m);
  assert.match(t, /Rule check \(19 CFR 102\.20\)/);
  assert.match(t, /Mexico under the tariff-shift rules, from the codes given\./);
  assert.match(t, /19 CFR 102\.11\(a\)\(3\)/);
  assert.match(t, /does not change the answer above or what is auto-accepted/);
  assert.match(t, /aluminum fins · 7616\.99 · China · shifts/);
  assert.doesNotMatch(m, /bg-ask-wash/);
});

test("decided and differing from the model: the full panel, flagged", () => {
  const rc = { ...decided, review: true, agrees: false, model_origin: "China" };
  const m = html(RuleCheckPanel, { c: { id: "x", rule_check: rc } });
  assert.match(text(m), /The model said China: worth a broker's look\./);
  assert.match(m, /bg-ask-wash/);
  assert.equal(headline(rc).tone, "ask");
});

test("undetermined for missing part codes: one line with a link to add them", () => {
  const rc = { ...decided, result: { status: "undetermined", reason: "r", rule, materials: [
    { name: "a", hts: "7616.99", origin: "China", foreign: true, status: "shift" },
    { name: "b", origin: "China", foreign: true, status: "missing_hts" },
    { name: "c", origin: "China", foreign: true, status: "missing_hts" },
  ] } };
  const m = html(RuleCheckPanel, { c: { id: "case_1", rule_check: rc } });
  assert.match(text(m), /^Rule check \(19 CFR 102\.20\): add HTS codes for 2 parts to run it\. Add codes$/);
  assert.match(m, /href="\/cases\/new\?from=case_1#field-part-hts-0"/);
  assert.doesNotMatch(m, /<section|bg-ask-wash/);
});

test("undetermined on a fact or a value: one calm line, no panel", () => {
  const fact = { ...decided, result: { status: "undetermined", reason: "r", rule, materials: [{ name: "fan", hts: "8414.59", origin: "China", foreign: true, status: "needs_fact" }] } };
  assert.equal(text(html(RuleCheckPanel, { c: { id: "x", rule_check: fact } })), "Rule check (19 CFR 102.20): not decided from the codes given; it needs a fact the codes cannot show.");
  const dm = { ...decided, result: { status: "undetermined", reason: "r", rule, if_not_de_minimis: { origin: "China", basis: "b", condition: "c" } } };
  assert.match(text(html(RuleCheckPanel, { c: { id: "x", rule_check: dm } })), /: open on value: Mexico if the parts that do not shift are worth no more than 7% of the good, otherwise China\.$/);
  const oos = { ...decided, result: { status: "out_of_scope", reason: "r" } };
  assert.match(text(html(RuleCheckPanel, { c: { id: "x", rule_check: oos } })), /not encoded for this product's chapter yet\./);
});

test("Part 102 case with only the good's code: a line to add the part codes", () => {
  const c = { id: "x", ...part102, product: product("8473.30", ["", ""]) };
  assert.deepEqual(ruleCheckDisplay(c), { kind: "line", text: "add HTS codes for 2 parts to run it.", addCodes: true });
  const noGood = { id: "x", ...part102, product: product("", [""]) };
  assert.equal(ruleCheckDisplay(noGood).text, "add HTS codes for the product and 1 part to run it.");
});

test("substantial-transformation case without part codes: nothing", () => {
  assert.equal(html(RuleCheckPanel, { c: { id: "x", ...st, product: product("8516.31", ["", ""]) } }), "");
  assert.equal(html(RuleCheckPanel, { c: { id: "x" } }), "");
});

test("substantial-transformation case with part codes: shown, labelled for comparison", () => {
  const info = text(html(RuleCheckPanel, { c: { id: "x", ...st, rule_check: { ...decided, applies: "informational" } } }));
  assert.match(info, /decided under substantial transformation; the Part 102 check is for comparison only/);
});

// #552: the API also sends a result with no rule when the product HAS a code
// and the check stopped for another reason. The line names that reason and
// asks for the product's code only when the product has none.
const noCountry = { ...decided, country: "", country_from: "not known: add a plant and its operations", result: { status: "undetermined", reason: "the country where the good is produced is not given" } };
const shortCode = { ...decided, result: { status: "undetermined", reason: "8481 is too short to pick one rule; give the 6-digit subheading" } };

test("codes given but no country of production: asks for a plant, links to the plants", () => {
  const twoPlants = { ...product("8473.30.5100", ["7616.99"]), plants: [{ id: "a", country: "Mexico" }, { id: "b", country: "China" }] };
  const noPlants = product("8473.30.5100", ["7616.99"]);
  for (const p of [twoPlants, noPlants]) {
    const c = { id: "case_2", ...part102, product: p, rule_check: noCountry };
    const d = ruleCheckDisplay(c);
    assert.equal(d.kind, "line");
    assert.equal(d.jump, "plants");
    const m = html(RuleCheckPanel, { c });
    const t = text(m);
    assert.match(t, /country where the product is made is not known\. Add a plant and its final-assembly step\./);
    assert.doesNotMatch(t, /add HTS codes/);
    assert.match(m, /href="\/cases\/new\?from=case_2#field-plants"/);
    assert.doesNotMatch(m, /field-part-hts-0/);
  }
});

test("a heading-only product code: asks for the 6-digit subheading, links to the product code", () => {
  const c = { id: "case_3", ...part102, product: { ...product("8481", ["7616.99"]), plants: [{ id: "a", country: "Mexico" }] }, rule_check: shortCode };
  const m = html(RuleCheckPanel, { c });
  const t = text(m);
  assert.equal(t, "Rule check (19 CFR 102.20): not run: 8481 is too short to pick one rule; give the 6-digit subheading. Edit the product code");
  assert.doesNotMatch(t, /add HTS codes/);
  assert.match(m, /#field-good-hts"/);
});

test("no rule and no product code: still asks for the product's code", () => {
  const c = { id: "x", ...part102, product: product("", ["7616.99"]), rule_check: noCountry };
  assert.deepEqual(ruleCheckDisplay(c), { kind: "line", text: "add HTS codes for the product to run it.", addCodes: true });
});
