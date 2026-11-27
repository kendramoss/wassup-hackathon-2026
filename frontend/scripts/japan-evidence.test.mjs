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

test("AD/CVD figures match the committed results", async () => {
  const { adcvdCheck: a } = await import("../src/lib/market-evidence.ts");
  const d = JSON.parse(read("../../eval/adcvd/summary.json"));
  assert.equal(a.rulings, d.scored);
  assert.equal(a.within, d.within);
  assert.equal(a.flagged, d.own_order.Recall.K);
  assert.equal(a.correctFlags, d.own_order.Precision.K);
  assert.equal(a.flaggedTotal, d.own_order.Precision.N);
  assert.equal(a.precisionPct, `${one(d.own_order.Precision.P)}%`);
  assert.equal(a.precisionInterval, `${one(d.own_order.Precision.Lo)}–${one(d.own_order.Precision.Hi)}`);
  assert.equal(a.recallPct, `${one(d.own_order.Recall.P)}%`);
  assert.equal(a.recallInterval, `${one(d.own_order.Recall.Lo)}–${one(d.own_order.Recall.Hi)}`);
  assert.equal(a.baselinePct, `${one(d.baseline_flag_every_ruling.Precision.P)}%`);
});
