import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const read = (p) => readFileSync(new URL(p, import.meta.url), "utf8");
const { adcvdScopeFirst: a } = await import("../src/lib/market-evidence.ts");
const one = (x) => (x * 100).toFixed(1);

test("AD/CVD scope-first figures match the committed D960 results", () => {
  const h = JSON.parse(read("../../eval/adcvd/real/heldout/summary3.json")).truths["truth3-hs.jsonl"];
  const m = h.method;
  assert.equal(a.rulings, h.rulings);
  assert.equal(a.rulings, h.positive + h.negative + h.contested);
  assert.equal(a.determinable, h.positive + h.negative);
  assert.equal(a.contested, h.contested);
  assert.equal(a.contestedPct, `${one(h.contested / h.rulings)}%`);
  assert.equal(a.contestedFlagged, m.contested_flagged);
  assert.equal(a.correct, m.precision.k);
  assert.equal(a.flagged, m.precision.n);
  assert.equal(a.precisionPct, `${one(m.precision.p)}%`);
  assert.equal(a.precisionInterval, `${one(m.precision.lo)}–${one(m.precision.hi)}`);
  assert.equal(a.within, m.recall.n);
  assert.equal(m.recall.k, m.precision.k);
  assert.equal(a.recallPct, `${one(m.recall.p)}%`);
  assert.equal(a.recallInterval, `${one(m.recall.lo)}–${one(m.recall.hi)}`);
  assert.equal(a.plainCorrect, h.plain_hts_check.precision.k);
  assert.equal(a.plainFlagged, h.plain_hts_check.precision.n);
  assert.equal(a.plainPct, `${one(h.plain_hts_check.precision.p)}%`);
  // the bar the figures were registered against
  assert.ok(m.precision.p >= 0.85 && m.recall.p >= 0.85);
});

test("AD/CVD scope-ruling figures match the committed 2017 to 2018 result", () => {
  const s = JSON.parse(read("../../eval/adcvd/real/scope/heldout-2017/summary3.json")).method;
  const r = a.scopeRulings;
  assert.equal(r.correct, s.precision.k);
  assert.equal(r.flagged, s.precision.n);
  assert.equal(r.within, s.recall.n);
  assert.equal(r.rulings, s.rulings);
  assert.equal(r.precisionPct, `${one(s.precision.p)}%`);
  assert.equal(r.precisionInterval, `${one(s.precision.lo)}–${one(s.precision.hi)}`);
  assert.equal(r.recallPct, `${one(s.recall.p)}%`);
  assert.equal(r.recallInterval, `${one(s.recall.lo)}–${one(s.recall.hi)}`);
});

test("Accuracy page says the figure is on cases the reviewers agree on, with the contested share beside it", () => {
  const src = read("../src/components/op/screens/accuracy-page.tsx");
  assert.match(src, /adcvdScopeFirst/);
  assert.match(src, /three independent reviewers agree/);
  assert.match(src, /a person must decide/);
  assert.match(src, /\{s\.contested\}/);
});
