import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const read = (p) => readFileSync(new URL(p, import.meta.url), "utf8");
const t = (await import("../src/lib/market-evidence.ts")).tariffWatch;
const s = JSON.parse(read("../../eval/tariffwatch/summary.json"));
const d2 = s.held_out_22_to_23.entry_day_2;
const one = (x) => (Math.round(x * 1000) / 10).toFixed(1);

test("tariff watch figures match eval/tariffwatch/summary.json", () => {
  assert.equal(t.lines, s.universe_subheadings);
  assert.equal(t.affected, d2.TP + d2.FN);
  assert.equal(t.found, d2.TP);
  assert.equal(t.falseAlarms, d2.FP);
  assert.equal(t.precisionInterval, `${one(d2.precision_wilson_lo)}–${one(d2.precision_wilson_hi)}`);
  assert.equal(t.recallPct, `${one(d2.recall)}%`);
  assert.equal(t.recallInterval, `${one(d2.recall_wilson_lo)}–${one(d2.recall_wilson_hi)}`);
  assert.equal(t.rateRight, s.held_out_22_to_23.rate_change.maritime_exact);
  assert.equal(t.rateLines, s.held_out_22_to_23.rate_change.maritime_n);
});

test("Accuracy page states the tariff watch limits", () => {
  const src = read("../src/components/op/screens/accuracy-page.tsx").replace(/\s+/g, " ");
  for (const w of ["Tariff watch", "Not a blind test", "One pair of revisions", "4 lines", "Not dollars"])
    assert.ok(src.includes(w), w);
});
