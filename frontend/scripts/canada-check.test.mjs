import assert from "node:assert/strict";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

// Canada's marking rules (D720, D725): the label is always shown; details
// (row text, basis, assumptions) sit behind "Details" (rule 16).
const { CanadaCheckView, canCheckCanada } = await import("../src/components/op/canada-check.tsx");

const decided = {
  jurisdiction: "ca",
  rules: "SOR/94-23 Schedule III",
  table_version: "ca-sor-94-23-v1",
  label: "Rules check only — no accuracy figure yet (no published rulings to test against)",
  country: "Mexico",
  note: "n",
  result: {
    status: "decided",
    origin: "Mexico",
    basis: "SOR/94-23 s. 4(1)(c); SOR/94-23 Sch. III, 6301.20-6301.90",
    reason: "each foreign material undergoes the change in tariff classification the rule requires, in Mexico",
    assumes: ["the goods are imported into Canada from the United States or Mexico (s. 3(1))"],
    rule: { id: "SOR/94-23 Sch. III, 6301.20-6301.90", hts: "6301.20-6301.90", text: "A change to subheadings 6301.20 through 6301.90 from any other heading.", alternatives: 1, encoded: 1 },
    materials: [{ name: "cotton yarn", hts: "5205.11", origin: "China", foreign: true, status: "shift", alternative: 1 }],
  },
};

test("decided: origin, short label, details folded", () => {
  const m = html(CanadaCheckView, { v: decided });
  const t = text(m);
  assert.match(t, /Canada marking rules/);
  assert.match(t, /Rules check only — no accuracy figure yet \(no published rulings to test against\)/);
  assert.match(t, /Mexico under Canada's marking rules, from the codes given\./);
  assert.match(m, /<details[^>]*>.*Details.*SOR\/94-23 s\. 4\(1\)\(c\)/s);
});

test("undetermined keeps the label and shows no origin", () => {
  const v = { ...decided, result: { ...decided.result, status: "undetermined", origin: undefined, reason: "the shift cannot be decided" } };
  const t = text(html(CanadaCheckView, { v }));
  assert.match(t, /no accuracy figure yet/);
  assert.match(t, /Not decided from the codes given\./);
});

test("the button needs a code on the good and on a part", () => {
  assert.equal(canCheckCanada({ product: { name: "p", hts: "6301.30", components: [{ name: "a", made_in: "China", hts: "5205.11" }] } }), true);
  assert.equal(canCheckCanada({ product: { name: "p", hts: "6301.30", components: [{ name: "a", made_in: "China" }] } }), false);
  assert.equal(canCheckCanada({ product: { name: "p", components: [{ name: "a", made_in: "China", hts: "5205.11" }] } }), false);
});
