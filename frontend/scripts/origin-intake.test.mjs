import assert from "node:assert/strict";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

// The EU and UK intake (#770, D800-D802): missing information is asked with
// the document that settles it; a rule that is not modelled gets no control;
// answers go beside the case, never into the product. Synthetic responses
// shaped like the committed tables' answers (the API tests run the tables).
const { OriginIntake, OriginRulesView } = await import("../src/components/op/origin-rules-check.tsx");
const { CheckCard } = await import("../src/components/op/screens/checks-view.tsx");
const I = await import("../src/lib/origin-intake.ts");
const { copy } = await import("../src/lib/copy.ts");

const c = {
  id: "case_1",
  product: {
    name: "Radio",
    hts: "8527.13",
    components: [
      { name: "tuner", hts: "8529.90", made_in: "China" },
      { name: "case", hts: "3926.90", made_in: "Malaysia" },
    ],
  },
};
const view = (needs, status = "undetermined") => ({
  jurisdiction: "eu",
  rules: "r",
  table_version: "eu-da2015-2446-annex22-01-v1",
  label: "Rules check only — no accuracy figure (no published origin decisions with their facts to test against)",
  chapters: ["85"],
  country: "Malaysia",
  administration: "a",
  note: "n",
  source: { title: "t", instrument: "i", url: "u", version: "v", as_of: "a", retrieved: "r", sha256: "s", licence: "l" },
  result: { status, covered: true, reason: "the primary rule for 8527 needs facts the codes cannot show", needs },
});
const share = { key: "value_share", kind: "share", material: 0, label: "Value share of tuner", evidence: "The supplier's bill of materials with values, and the good's ex-works price" };
const fact = { key: "ch85.r", kind: "fact", label: "Does the chapter rule for goods made from blanks apply, and was it met in Malaysia?", evidence: "A production record showing the good was made from blanks, and where" };
const row = { key: "row", kind: "row", label: "Which of these is the good?", options: [{ key: "6205xa", code: "ex 62 05 (a)", description: "- finished or complete" }, { key: "none", code: "", description: "None of these" }], evidence: "The product specification or catalogue description, to match the good to one of the listed descriptions" };
const review = { key: "review", kind: "review", label: "Is tuner one of the excepted goods?", detail: "except from split heading ex 7228 (a)" };
const country = { key: "country", kind: "code", label: "Country where the good is made", evidence: "The production record or plant address for the last stage of production" };

test("each missing fact gets a control and the document that settles it", () => {
  const m = html(OriginIntake, { c, v: view([share, fact, row]), save: async () => {} });
  const t = text(m);
  for (const n of [share, fact, row]) {
    assert.ok(t.includes(n.label), n.label);
    assert.ok(t.includes(n.evidence), n.evidence);
  }
  assert.equal((m.match(/type="radio"/g) ?? []).length, 4, "yes/no for the fact, two rows");
  assert.match(m, /<input[^>]*id="oi-share-value-share-0"/);
  assert.match(t, /% of the good/);
  assert.match(t, new RegExp(copy.originRules.recheck));
});

test("a rule a person must read, or a fact for the product, gets no control", () => {
  const m = html(OriginIntake, { c, v: view([review, country]), save: async () => {} });
  assert.ok(!/<(input|select|form)/.test(m), "no controls for review or product needs");
  const t = text(m);
  assert.ok(t.includes(copy.originRules.person) && t.includes(review.detail));
  assert.ok(t.includes(copy.originRules.onProduct) && t.includes(copy.originRules.editProduct));
});

test("the view labels missing information and a rule that is not modelled apart", () => {
  const ask = text(html(OriginRulesView, { v: view([share]) }));
  assert.ok(ask.includes(copy.originRules.missing));
  const judged = view(undefined, "needs_judgement");
  judged.result.basis = "Art. 60(2) UCC";
  const m = html(OriginRulesView, { v: judged, intake: "SHOULD NOT SHOW" });
  assert.ok(text(m).includes(copy.originRules.notModelled));
  assert.ok(!text(m).includes("SHOULD NOT SHOW"), "no intake without needs");
  assert.ok(!/<(input|button)/.test(m.replace(/<details[\s\S]*<\/details>/, "")));
});

test("percentages read as shares; bad ones are named", () => {
  assert.equal(I.pct("70"), 0.7);
  assert.equal(I.pct("12.5%"), 0.125);
  assert.equal(I.pct(""), undefined);
  assert.ok(Number.isNaN(I.pct("150")));
  assert.ok(Number.isNaN(I.pct("abc")));
  assert.deepEqual(I.formErrors({ ...I.blankForm(), value: { 0: "150" }, hts: { 1: "85x" } }), ["value:0", "hts:1"]);
});

test("answers merge with what was saved and name each part as it stands", () => {
  const saved = { source: "user-supplied", updated_at: "2026-11-24T06:00:00Z", answers: { "ch85.r": false }, materials: [{ index: 0, name: "tuner", part_hts: "8529.90", value_share: 0.7 }] };
  const f = { ...I.blankForm(), value: { 1: "20" } };
  assert.deepEqual(I.body(c, saved, f), {
    answers: { "ch85.r": false },
    materials: [
      { index: 0, name: "tuner", part_hts: "8529.90", value_share: 0.7 },
      { index: 1, name: "case", part_hts: "3926.90", value_share: 0.2 },
    ],
  });
  // A saved fact for a part that changed is dropped, not resent.
  const moved = { ...c, product: { ...c.product, components: [{ name: "speaker", hts: "8518.29", made_in: "China" }, c.product.components[1]] } };
  assert.deepEqual(I.body(moved, saved, I.blankForm()), { answers: { "ch85.r": false } });
  assert.equal(I.suppliedCount(saved), 2);
  assert.equal(I.hasInput(I.blankForm()), false);
  assert.equal(I.hasInput({ ...I.blankForm(), answers: { x: false } }), true);
});

test("a check card shows its own question and status; no card merges them", () => {
  const ch = {
    id: "eu_origin",
    title: "EU non-preferential origin (Annex 22-01)",
    question: "Which country is the good's non-preferential origin on import into the EU?",
    method: "rules",
    status: "needs_judgement",
    outcome: "Rule not modelled: needs a substantial-transformation judgement",
    label: "Rules check only — no accuracy figure",
    review: true,
    not_modelled: ["Art. 60(2) UCC: no list rule"],
    sources: [{ title: "Annex 22-01", version: "CELEX 02015R2446-20261101" }],
    rule_version: "eu-da2015-2446-annex22-01-v1",
    at: "2026-11-24T06:00:00Z",
  };
  const m = html(CheckCard, { ch, c });
  const t = text(m);
  for (const s of [ch.title, ch.question, copy.report.status.needs_judgement, copy.report.review, ch.not_modelled[0], ch.label]) assert.ok(t.includes(s), s);
  assert.ok(!t.includes(copy.report.answer), "no answer controls for a rule that is not modelled");
  assert.doesNotMatch(t.toLowerCase(), /overall|score/);
  const asked = html(CheckCard, { ch: { ...ch, status: "needs_information", outcome: "Not decided", not_modelled: undefined, missing: [{ label: "Value share of tuner", evidence: share.evidence }] }, c });
  assert.ok(text(asked).includes(copy.report.answer));
  assert.ok(text(asked).includes(share.evidence));
});

test("copy stays short (rule 16)", () => {
  const C = copy.originRules;
  for (const s of [C.missing, C.notModelled, C.ask, C.recheck, C.person, C.yours, copy.report.title, copy.report.lede]) assert.ok(s.split(" ").length <= 12, s);
});
