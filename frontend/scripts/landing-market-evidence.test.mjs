import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const read = (p) => readFileSync(new URL(p, import.meta.url), "utf8");
const lib = read("../src/lib/market-evidence.ts");
const view = read("../src/components/op/landing/market-evidence.tsx");
const landing = read("../src/components/op/landing/landing.tsx");

test("landing RCEP counts match the committed held-out summary", () => {
  const { held_out: h } = JSON.parse(read("../../eval/rcep/japan-summary.json"));
  const o = h.published_positives.outcomes;
  assert.deepEqual([o.yes.count, o.undetermined.count, o.yes.total], [71, 27, 98]);
  assert.equal(h.published_positives.deciding_criterion_agreement.count, 71);
  assert.match(lib, /rulings: 98,\s+yes: 71,\s+undetermined: 27,\s+criterionMatches: 71/);
  assert.match(lib, /71 of 71 yes answers/);
});

test("landing states the RCEP limits", () => {
  assert.match(lib, /It is not an accuracy test/);
  assert.match(lib, /No rulings that deny the preference were tested/);
  assert.match(lib, /Customs specialist review is pending/);
  assert.match(view, /<details className="mkt-details">/);
  assert.match(view, /href="\/accuracy#market-evidence"/);
});

test("market statuses invent no Canada or Japan scores", () => {
  assert.match(lib, /name: "Canada marking", status: "Rules only", note: "No score\."/);
  assert.match(lib, /name: "Japan origin", status: "Needs setup", note: "Not available yet\. Not measured\."/);
  const markets = lib.slice(lib.indexOf("markets: ["), lib.indexOf("detailsTitle"));
  const lines = markets.split("\n").filter((l) => /Canada|Japan/.test(l));
  assert.equal(lines.length, 2);
  for (const l of lines) assert.doesNotMatch(l, /\d/);
});

test("the market block is rendered on the landing page", () => {
  assert.match(landing, /<MarketEvidence \/>/);
});
