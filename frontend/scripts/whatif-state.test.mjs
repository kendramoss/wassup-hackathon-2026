import assert from "node:assert/strict";
import { test } from "node:test";
import { whatIfView } from "../src/lib/whatif-state.ts";

const base = { change: "assembled in Vietnam", origin: "Vietnam", confidence: 0.97, decisive_step: "sewing" };
const confirmed = (regime) => ({ ...base, regime, scope: "confirmed", route: "auto", triage_reason: "confidence 0.97 >= 0.92 and in scope" });

test("a known regime on the auto route without a provisional note shows the ruler", () => {
  for (const regime of ["textile_102_21", "usmca_part102", "pharma_api", "substantial_transformation"]) {
    const v = whatIfView(confirmed(regime));
    assert.equal(v.kind, "confirmed", regime);
    assert.equal(v.showRuler, true, regime);
    assert.equal(v.scenarioWarning, "", regime);
  }
});

test("regimes read in words, never as identifiers", () => {
  assert.match(whatIfView(confirmed("textile_102_21")).regimeLabel, /^Textile rules \(19 CFR 102\.21\)$/);
  assert.match(whatIfView(confirmed("usmca_part102")).regimeLabel, /Part 102/);
  assert.match(whatIfView(confirmed("pharma_api")).regimeLabel, /^Pharmaceutical rules/);
  assert.equal(whatIfView(confirmed("substantial_transformation")).regimeLabel, "Substantial transformation");
  for (const r of ["textile_102_21", "usmca_part102", "pharma_api", "substantial_transformation", "unknown", "unclassified"]) {
    assert.doesNotMatch(whatIfView(confirmed(r)).regimeLabel, /_/, r);
  }
});

const uncertain = {
  "needs_review route": { ...confirmed("textile_102_21"), route: "needs_review", triage_reason: "confidence 0.60 is below the textile bar" },
  "needs_info route": { ...confirmed("pharma_api"), route: "needs_info", triage_reason: "more facts needed" },
  "provisional note": { ...confirmed("substantial_transformation"), provisional: "the textile_102_21 engine failed, so this is a provisional general prediction", route: "needs_review", triage_reason: "regime not confirmed" },
  "provisional note with auto route": { ...confirmed("substantial_transformation"), provisional: "the regime that produced this prediction is unknown" },
  "unknown regime": { ...confirmed("unknown"), provisional: "the regime that produced this prediction is unknown" },
  "unknown regime, no note": { ...confirmed("unknown") },
  "unclassified": { ...base, regime: "unclassified", provisional: "this run had no regime dispatch or review route" },
  "legacy saved what-if": { ...base, regime: "unknown", provisional: "the regime of this saved what-if was not recorded, so it is not confirmed" },
  "no provenance fields at all": { ...base },
  "scope unavailable": { ...confirmed("substantial_transformation"), scope: "configured_failed", route: "needs_review", triage_reason: "scope unavailable" },
  "label missing": { ...confirmed("substantial_transformation"), scope: "label_missing", route: "needs_review", triage_reason: "no stored scope label" },
  "auto route with a missing route": { ...confirmed("textile_102_21"), route: undefined },
  "auto but scope not confirmed": { ...confirmed("textile_102_21"), scope: "label_missing" },
};

test("no uncertain what-if ever shows the auto-accept ruler", () => {
  for (const [name, w] of Object.entries(uncertain)) {
    const v = whatIfView(w);
    assert.equal(v.showRuler, false, name);
    assert.notEqual(v.kind, "confirmed", name);
    assert.doesNotMatch(`${v.headline} ${v.note}`, /auto-accept/i, name);
  }
});

test("an uncertain what-if says Needs review or Provisional, with its reason in words", () => {
  const nr = whatIfView(uncertain["needs_review route"]);
  assert.equal(nr.kind, "review");
  assert.equal(nr.headline, "Needs review");
  assert.match(nr.note, /below the textile bar/);
  const ni = whatIfView(uncertain["needs_info route"]);
  assert.equal(ni.headline, "Needs more facts");
  const prov = whatIfView(uncertain["provisional note"]);
  assert.equal(prov.kind, "provisional");
  assert.equal(prov.headline, "Provisional answer");
  assert.match(prov.note, /provisional general prediction/);
  assert.equal(prov.originLabel, "Provisional origin");
  assert.equal(whatIfView(confirmed("textile_102_21")).originLabel, "");
});

test("the provisional note never leaks raw regime identifiers", () => {
  const v = whatIfView(uncertain["provisional note"]);
  assert.doesNotMatch(v.note, /textile_102_21|usmca_part102|pharma_api/);
  assert.match(v.note, /textile rules/i);
});

test("a what-if with no provenance reads as unknown and in need of review", () => {
  const v = whatIfView(uncertain["no provenance fields at all"]);
  assert.equal(v.kind, "review");
  assert.equal(v.regimeLabel, "Regime not confirmed");
  assert.match(v.note, /not (recorded|confirmed)/i);
});

test("Add-as-scenario warns exactly when the result is uncertain", () => {
  for (const [name, w] of Object.entries(uncertain)) {
    const v = whatIfView(w);
    assert.match(v.scenarioWarning, /re-run|full (regime )?(check|policy)|may (come out|differ)|review/i, name);
    assert.match(v.scenarioWarning, /scenario/i, name);
  }
  assert.equal(whatIfView(confirmed("textile_102_21")).scenarioWarning, "");
});
