import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

// The EU and UK origin rules checks (D760-D766): the label is always shown,
// a good with no list rule is a judgement (never an origin), and the source,
// version and hash sit behind "Details" (rule 16). Synthetic responses.
const { OriginRulesView, OriginRulesCheck, canCheckOrigin, firstSentence } = await import("../src/components/op/origin-rules-check.tsx");
const { copy } = await import("../src/lib/copy.ts");

const LABEL = "Rules check only — no accuracy figure (no published origin decisions with their facts to test against)";
const UK_LABEL = "Rules check only — no accuracy figure (no published origin rulings to test against)";
const source = {
  title: "Commission Delegated Regulation (EU) 2015/2446, Annex 22-01",
  instrument: "UCC Art. 59-63",
  url: "https://eur-lex.europa.eu/legal-content/EN/TXT/?uri=CELEX:02015R2446-20261101",
  version: "CELEX 02015R2446-20261101 (consolidated text, 2026-11-01)",
  as_of: "2026-11-01",
  retrieved: "2026-11-24",
  sha256: "87be4f1a432bc0ef35a6c27a94d237a65fc41d036278cf43fd4889a4f79818be",
  licence: "EUR-Lex reuse notice",
};
const decided = {
  jurisdiction: "eu",
  rules: "EU non-preferential origin",
  table_version: "eu-da2015-2446-annex22-01-v1",
  label: LABEL,
  chapters: ["84", "85"],
  country: "Malaysia",
  administration: "These are common EU rules. Each Member State's customs authority applies them, issues Binding Origin Information and checks declarations.",
  note: "n",
  source,
  result: {
    status: "decided",
    origin: "Malaysia",
    covered: true,
    basis: "Art. 32 DA; Annex 22-01, 8527",
    reason: "the primary rule for 8527 is met in Malaysia: CTH, except from heading 8529",
    assumes: ["the work was not done to avoid EU measures (Art. 33 DA)"],
    rule: { key: "8527", code: "8527", description: "Radios", text: "CTH, except from heading 8529", alternatives: [{ key: "8527.1", text: "CTH, except from heading 8529", kind: "change", outcome: "met" }] },
  },
};

test("the label is per market, stays visible, and the question is asked plainly", () => {
  assert.equal(copy.originRules.label.eu, LABEL);
  assert.equal(copy.originRules.label.uk, UK_LABEL);
  const t = text(html(OriginRulesView, { v: decided }));
  assert.ok(t.includes(LABEL));
  assert.match(t, /EU origin \(rules check\)/);
  assert.match(t, /Origin under EU non-preferential rules\?/);
  assert.match(t, /Malaysia under EU origin rules, from the codes given\./);
});

test("source, version, hash and who applies the rules sit behind Details", () => {
  const m = html(OriginRulesView, { v: decided });
  const details = m.match(/<details[^>]*>[\s\S]*<\/details>/)[0];
  assert.ok(!/<details[^>]*open/.test(details));
  for (const s of [source.version, source.sha256, "Member State", "Art. 32 DA"]) assert.ok(details.includes(s), s);
  const outside = text(m.replace(details, ""));
  assert.ok(!outside.includes(source.sha256));
});

test("a good with no list rule is a judgement, with no origin", () => {
  const v = { ...decided, jurisdiction: "uk", label: UK_LABEL, result: { status: "needs_judgement", covered: false, basis: "Taxation (Cross-border Trade) Act 2018 s. 17(3)", reason: "Part Four has no rules for chapter 84" } };
  const t = text(html(OriginRulesView, { v }));
  assert.match(t, /Part Four has no rules for chapter 84\./);
  assert.match(t, /A customs adviser decides where it was last substantially processed\./);
  assert.match(t, /UK origin \(rules check\)/);
  assert.match(t, /Needs a substantial-transformation judgement\./);
  assert.ok(!t.includes("under UK origin rules"));
  assert.ok(t.includes(UK_LABEL));
});

test("a UK minimal-operations judgement on a listed good does not say no list rule covers it", () => {
  const reason = "the last work was only operations that are not an important stage of manufacture (SI 2020/1433 reg. 9, Part Two), so it does not confer origin";
  const v = { ...decided, jurisdiction: "uk", label: UK_LABEL, result: { status: "needs_judgement", covered: true, basis: "SI 2020/1433 reg. 9; Part Two", reason } };
  const t = text(html(OriginRulesView, { v }));
  assert.ok(t.includes("Part Two"));
  assert.doesNotMatch(t, /No list rule/);
});

test("a fact need names the fact to confirm", () => {
  const v = {
    ...decided,
    result: {
      status: "undetermined",
      covered: true,
      reason: "the primary rule for ex 8473 needs facts the codes cannot show",
      needs: [{ key: "8473x.2", kind: "fact", label: "In Taiwan: Assembly of semi-conductor products?", detail: "Assembly of semi-conductor products" }],
    },
  };
  const t = text(html(OriginRulesView, { v }));
  assert.match(t, /In Taiwan: Assembly of semi-conductor products\?/);
});

test("undetermined lists what would decide it, with row choices, and no controls", () => {
  const v = {
    ...decided,
    result: {
      status: "undetermined",
      covered: true,
      reason: "the list covers only some goods of this code; say which the good is",
      needs: [
        { key: "row", kind: "row", label: "Which of these is the good?", options: [{ key: "8473x", code: "ex 8473", description: "Memory Modules" }, { key: "none", code: "", description: "None of these" }] },
        { key: "value_share", kind: "share", material: 0, label: "Value share of board" },
      ],
    },
  };
  const m = html(OriginRulesView, { v, parts: ["board"] });
  const t = text(m);
  assert.match(t, /Not decided yet\./);
  assert.match(t, /To decide:/);
  assert.match(t, /ex 8473: Memory Modules/);
  assert.match(t, /None of these/);
  assert.match(t, /Value share of board/);
  assert.ok(!/<(input|button|select)/.test(m), "no fake controls: the intake is later work");
});

test("the check needs a code on the good and on a part", () => {
  assert.equal(canCheckOrigin({ product: { name: "p", hts: "8527.13", components: [{ name: "a", made_in: "China", hts: "8534.00" }] } }), true);
  assert.equal(canCheckOrigin({ product: { name: "p", hts: "8527.13", components: [{ name: "a", made_in: "China" }] } }), false);
  const t = text(html(OriginRulesCheck, { c: { id: "case_1", product: { name: "p", components: [] } }, market: "uk" }));
  assert.match(t, /UK origin \(rules check\)/);
  assert.match(t, /Product HS code/);
});

test("EU and UK cases show the rules check, not a model answer", async () => {
  const src = await readFile(new URL("../src/components/op/screens/origin-step.tsx", import.meta.url), "utf8");
  assert.match(src, /jurisdiction === "eu" \|\| ctx\.c\.jurisdiction === "uk"\) return <OriginRulesMarket/);
});

test("copy stays short (rule 16)", () => {
  const C = copy.originRules;
  for (const s of [C.undetermined, C.judgement, C.toDecide, C.title.eu, C.title.uk]) assert.ok(s.split(" ").length <= 6, s);
});

test("a judgement shows its first sentence; the rest sits under Details", () => {
  const reason = "Annex 22-01 has no rule for this good (chapter 84 rules cover: ex 8443, ex 8473, ex 8482). This good has no list rule in Annex 22-01, so its origin is the country of its last substantial, economically justified processing (Art. 60(2) UCC). That is a judgement on the facts; it is not decided by code";
  assert.equal(firstSentence(reason), "Annex 22-01 has no rule for this good (chapter 84 rules cover: ex 8443, ex 8473, ex 8482)");
  assert.equal(firstSentence("only operations (SI 2020/1433 reg. 9, Part Two), so it does not confer origin. Where it was processed is a judgement"), "only operations (SI 2020/1433 reg. 9, Part Two), so it does not confer origin");
  const m = html(OriginRulesView, { v: { ...decided, result: { status: "needs_judgement", covered: true, basis: "Art. 60(2) UCC", reason } } });
  const details = m.match(/<details[^>]*>[\s\S]*<\/details>/)[0];
  assert.ok(!text(m.replace(details, "")).includes("Art. 60(2) UCC). That"));
  assert.ok(text(details).includes("That is a judgement on the facts"));
});
