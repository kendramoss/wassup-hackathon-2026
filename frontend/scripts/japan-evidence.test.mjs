import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const read = (p) => readFileSync(new URL(p, import.meta.url), "utf8");
const { japanEvidence: j } = await import("../src/lib/market-evidence.ts");
const held = JSON.parse(read("../../eval/jp/jp-held_out-gemini-4-argon-jp-engine-v1-litellm-d741-heldout-1-metrics.json"));
const syn = JSON.parse(read("../../eval/jp/jp-synthetic-gemini-4-argon-jp-engine-v1-litellm-d741-synthetic-1-metrics.json"));
const pct = (x) => (x * 100).toFixed(2);
const span = (c) => `${pct(c.lo)}–${pct(c.hi)}`;

test("Japan experiment figures match the committed D741 metrics", () => {
  assert.equal(j.cases, held.cases);
  assert.equal(j.criterionMatch, held.criterion_match.k);
  assert.equal(j.criterionMatch + j.otherQualifies, held.qualifies);
  assert.equal(j.recall, held.positive_recall.k);
  assert.equal(j.undetermined, held.undetermined);
  assert.equal(j.errors, held.errors);
  assert.equal(j.criterionMatch + j.otherQualifies + j.undetermined + j.errors, held.cases);
  assert.equal(j.matchPct, `${pct(held.criterion_match.p)}%`);
  assert.equal(j.matchInterval, span(held.criterion_match));
  assert.equal(j.recallPct, `${pct(held.positive_recall.p)}%`);
  assert.equal(j.recallInterval, span(held.positive_recall));
  assert.equal(j.syntheticRefused, syn.synthetic_refusals.k);
  assert.equal(j.syntheticCases, syn.synthetic_refusals.n);
  assert.equal(j.syntheticInterval, `${pct(syn.synthetic_refusals.lo)}–100`);
});

test("Accuracy page shows the earlier Japan test as earlier, not as the live result", () => {
  const src = read("../src/components/op/screens/accuracy-page.tsx");
  assert.ok(src.includes("Earlier test, 24 Nov, before the rule text and evidence changes"));
  assert.ok(src.includes("That test is used up"));
  assert.ok(!src.includes("not the live Japan route"));
});

const d922 = JSON.parse(read("../../eval/jp/d922-answered-summary.json"));
const fresh = JSON.parse(read("../../eval/jp/jp-fresh_1126-gemini-4-argon-jp-engine-v1-litellm-d919-fresh-metrics.json"));
const one = (x) => (Math.round(x * 1000) / 10).toFixed(1);
const m = (await import("../src/lib/market-evidence.ts")).japanMeasured;

test("Japan figures on the Accuracy page match D920 and D922 result files", () => {
  assert.equal(m.unseen, fresh.cases);
  assert.equal(m.unseenRight, fresh.criterion_match.k);
  assert.equal(m.unseenInterval, `${one(fresh.criterion_match.lo)}–${one(fresh.criterion_match.hi)}`);
  assert.equal(fresh.criterion_match.p.toFixed(3), "0.841");
  const f = d922.fresh_1126;
  assert.equal(m.unseenAnswered, f.criterion_after);
  assert.equal(m.unseenAnsweredPct, `${one(f.criterion_after / f.cases)}%`);
  assert.equal(m.unseenAnsweredInterval, `${one(f.ci_after[0])}–${one(f.ci_after[1])}`);
  const d = d922.dev_eval;
  assert.equal(m.dev, d.cases);
  assert.equal(m.devRight, d.criterion_before);
  assert.equal(m.devAnswered, d.criterion_after);
  assert.equal(m.devAnsweredPct, `${one(d.criterion_after / d.cases)}%`);
  assert.equal(m.devAnsweredInterval, `${one(d.ci_after[0])}–${one(d.ci_after[1])}`);
  assert.equal(m.negatives, d922.dev_neg.cases);
  assert.equal(m.negativesApproved, d922.dev_neg.approved_after);
  assert.equal(m.euCases, d.by_agreement["JP-EU EPA"].n);
  assert.equal(m.euAnswered, d.by_agreement["JP-EU EPA"].after);
  assert.equal(d.by_agreement["JP-EU EPA"].before, 128);
});

test("USMCA and ATIGA figures match the committed results", async () => {
  const { rulesChecks: r } = await import("../src/lib/market-evidence.ts");
  const u = JSON.parse(read("../../eval/usmca/summary-held_out.json")).extended;
  assert.equal(r.usmcaProducts, u.scored);
  assert.equal(r.usmcaDecided, u.decided);
  assert.equal(r.usmcaAgreed, u.agreed);
  assert.equal(r.usmcaPct, `${one(u.agreement_on_decided)}%`);
  assert.equal(r.usmcaInterval, `${one(u.agreement_wilson_lo)}–${one(u.agreement_wilson_hi)}`);
  // 7 of 7 is enforced by backend/internal/atiga/reference_test.go.
  assert.equal(r.atigaExamples, JSON.parse(read("../../eval/atiga/reference-cases.json")).cases.length);
  assert.equal(r.atigaReached, r.atigaExamples);
});

test("textile rules figures match the committed results", async () => {
  const { textileRules: t } = await import("../src/lib/market-evidence.ts");
  const cells = JSON.parse(read("../../eval/textile102/summary.json")).cells.filter(
    (c) => c.source === "facts+suggested" && (c.split === "final" || c.split === "final2"),
  );
  assert.equal(cells.length, 2);
  const sum = (k) => cells.reduce((a, c) => a + c[k], 0);
  assert.equal(t.rulings, sum("n"));
  assert.equal(t.inScope, sum("n") - sum("out_of_scope"));
  assert.equal(t.decided, sum("decided"));
  assert.equal(t.agreed, sum("agree_cbp"));
  assert.equal(t.pct, `${one(t.agreed / t.decided)}%`);
  const n = t.decided, p = t.agreed / n, z = 1.959964;
  const d = 1 + (z * z) / n, c = p + (z * z) / (2 * n), a = z * Math.sqrt((p * (1 - p)) / n + (z * z) / (4 * n * n));
  assert.equal(t.interval, `${one((c - a) / d)}–${one((c + a) / d)}`);
});

test("AD/CVD figures match the committed results", async () => {
  const { adcvdCheck: a } = await import("../src/lib/market-evidence.ts");
  const h = JSON.parse(read("../../eval/adcvd/heldout/summary.json"));
  const m = h.methods["scope+reader+guard"];
  const plain = h.methods.base;
  assert.equal(a.rulings, h.scored);
  assert.equal(a.within, h.within);
  assert.equal(a.flagged, m.own_order.Recall.k);
  assert.equal(a.correctFlags, m.own_order.Precision.k);
  assert.equal(a.flaggedTotal, m.own_order.Precision.n);
  assert.equal(a.precisionPct, `${one(m.own_order.Precision.p)}%`);
  assert.equal(a.precisionInterval, `${one(m.own_order.Precision.lo)}–${one(m.own_order.Precision.hi)}`);
  assert.equal(a.recallPct, `${one(m.own_order.Recall.p)}%`);
  assert.equal(a.recallInterval, `${one(m.own_order.Recall.lo)}–${one(m.own_order.Recall.hi)}`);
  assert.equal(a.plainCorrect, plain.own_order.Precision.k);
  assert.equal(a.plainTotal, plain.own_order.Precision.n);
  assert.equal(a.plainPct, `${one(plain.own_order.Precision.p)}%`);
  assert.equal(a.baselinePct, `${one(h.baseline_flag_every_ruling.Precision.p)}%`);
  // the reader keeps the plain check's recall: it only removes flags
  assert.equal(m.own_order.Recall.k, plain.own_order.Recall.k);
  const e = a.earlier;
  const d = JSON.parse(read("../../eval/adcvd/summary.json"));
  assert.equal(e.rulings, d.scored);
  assert.equal(e.within, d.within);
  assert.equal(e.flagged, d.own_order.Recall.K);
  assert.equal(e.correctFlags, d.own_order.Precision.K);
  assert.equal(e.flaggedTotal, d.own_order.Precision.N);
  assert.equal(e.precisionPct, `${one(d.own_order.Precision.P)}%`);
  assert.equal(e.precisionInterval, `${one(d.own_order.Precision.Lo)}–${one(d.own_order.Precision.Hi)}`);
  assert.equal(e.recallPct, `${one(d.own_order.Recall.P)}%`);
  assert.equal(e.recallInterval, `${one(d.own_order.Recall.Lo)}–${one(d.own_order.Recall.Hi)}`);
  assert.equal(e.baselinePct, `${one(d.baseline_flag_every_ruling.Precision.P)}%`);
  // the seen set under the same method: the sentence on the page says 11/19 (57.9%)
  const s = JSON.parse(read("../../eval/adcvd/seen/summary.json")).methods["scope+reader+guard"].own_order.Precision;
  assert.equal(`${s.k}/${s.n} (${one(s.p)}%)`, "11/19 (57.9%)");
});
