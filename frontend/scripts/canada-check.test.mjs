import assert from "node:assert/strict";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

// Canada's marking rules (D720, D725): the label is always shown; details
// (row text, basis, assumptions) sit behind "Details" (rule 16).
const { CanadaCheckView, CanadaCheck, canCheckCanada } = await import("../src/components/op/canada-check.tsx");

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
  assert.match(t, /Country-of-origin marking for Canada \(rules check\)/);
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

// #731: a plain label, and missing codes as a short checklist. Never a
// preferential (USMCA) claim.
const { copy } = await import("../src/lib/copy.ts");
const ask = (product) => text(html(CanadaCheck, { c: { id: "case_1", product }, auto: true }));

test("the label is plain language, not a statute name", () => {
  assert.equal(copy.canadaCheck.title, "Country-of-origin marking for Canada (rules check)");
  assert.doesNotMatch(copy.canadaCheck.title, /SOR|Schedule/);
});

test("missing codes: a checklist of what to add, only the missing items", () => {
  const none = ask({ name: "p", components: [{ name: "a", made_in: "China" }] });
  assert.match(none, /Product HS code/);
  assert.match(none, /HS code for at least one part/);
  const goodOnly = ask({ name: "p", hts: "6301.30", components: [{ name: "a", made_in: "China" }] });
  assert.doesNotMatch(goodOnly, /Product HS code/);
  assert.match(goodOnly, /HS code for at least one part/);
  const partOnly = ask({ name: "p", components: [{ name: "a", made_in: "China", hts: "5205.11" }] });
  assert.match(partOnly, /Product HS code/);
  assert.doesNotMatch(partOnly, /HS code for at least one part/);
  const m = html(CanadaCheck, { c: { id: "c", product: { name: "p", components: [] } }, auto: true });
  assert.match(m, /<ul[^>]*data-canada-needs/);
  assert.match(m, /Add codes/);
});

test("the copy never implies preferential USMCA qualification", () => {
  const all = JSON.stringify(copy.canadaCheck) + ask({ name: "p", components: [] }) + text(html(CanadaCheckView, { v: decided }));
  assert.doesNotMatch(all, /USMCA|CUSMA|preferential|duty[- ]free|qualif/i);
});
