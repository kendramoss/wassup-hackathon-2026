import assert from "node:assert/strict";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

const { TurnView } = await import("../src/components/op/ask-drawer.tsx");
const { WatchLine } = await import("../src/components/op/screens/origin-step.tsx");
const { YourCases } = await import("../src/components/op/screens/cases-home.tsx");

const base = { change: "sewn in Vietnam", origin: "Vietnam", confidence: 0.97, decisive_step: "sewing", facts: "f" };
const turn = (w) => ({ question: "What if?", answer: "It would change.", cited_rulings: [], whatIfs: [w] });
const card = (w) => html(TurnView, { t: turn(w), caseId: "c1" });

// ---- F1: the Ask card ----------------------------------------------------

test("a confirmed auto what-if keeps the auto-accept ruler", () => {
  const m = card({ ...base, regime: "textile_102_21", scope: "confirmed", route: "auto", triage_reason: "ok" });
  assert.match(m, /against the 0\.92 auto-accept bar/);
  assert.match(text(m), /Textile rules \(19 CFR 102\.21\)/);
  assert.doesNotMatch(text(m), /Needs review|Provisional/);
});

test("uncertain what-ifs render no ruler and no auto-accept wording", () => {
  const cases = {
    unknown: { ...base, regime: "unknown", provisional: "the regime that produced this prediction is unknown", route: "needs_review", triage_reason: "regime not confirmed" },
    legacy: { ...base, regime: "unknown", provisional: "the regime of this saved what-if was not recorded, so it is not confirmed" },
    provisional: { ...base, regime: "substantial_transformation", scope: "confirmed", specialist: "failed", route: "needs_review", triage_reason: "specialist unavailable", provisional: "the textile_102_21 engine failed, so this is a provisional general prediction" },
    review: { ...base, regime: "pharma_api", scope: "confirmed", route: "needs_review", triage_reason: "confidence 0.60 is below the bar" },
    bare: { ...base },
  };
  for (const [name, w] of Object.entries(cases)) {
    const m = card(w);
    assert.doesNotMatch(m, /role="img"/, name);
    assert.doesNotMatch(text(m), /auto-accept/i, name);
    assert.match(text(m), /Needs review|Provisional answer|Needs more facts/, name);
    assert.doesNotMatch(text(m), /textile_102_21|usmca_part102|pharma_api|substantial_transformation/, name);
  }
});

test("a provisional origin is labelled provisional and its reason is visible text", () => {
  const m = text(card({ ...base, regime: "substantial_transformation", specialist: "failed", route: "needs_review", triage_reason: "specialist unavailable", provisional: "the textile_102_21 engine failed, so this is a provisional general prediction" }));
  assert.match(m, /Provisional origin/);
  assert.match(m, /provisional general prediction/);
  assert.match(m, /Vietnam/);
});

test("Add as a scenario carries a truthful warning only on uncertain cards", () => {
  const unsure = text(card({ ...base, regime: "unknown", provisional: "the regime that produced this prediction is unknown" }));
  assert.match(unsure, /Add as a scenario/);
  assert.match(unsure, /scenario[^.]*re-run/i);
  const sure = text(card({ ...base, regime: "textile_102_21", scope: "confirmed", route: "auto", triage_reason: "ok" }));
  assert.match(sure, /Add as a scenario/);
  assert.doesNotMatch(sure, /may come out differently/);
});

// ---- F3: heading order -----------------------------------------------------

const watch = (extra) => ({ id: "c1", date: "2026-02-01", prediction: { origin_country: "China", confidence: 0.97 }, watch: { checked_at: "2026-11-19T00:00:00Z", status: "changed", ...extra } });
const line = (c) => html(WatchLine, { c, setCase() {} });

test("the watch box has no heading that can skip a level", () => {
  for (const extra of [{ basis: "precedent" }, { basis: "method", triage: { route: "needs_review", reason: "an expert reviews" } }, { basis: "regime" }, {}]) {
    const m = line(watch(extra));
    assert.doesNotMatch(m, /<h[1-6][\s>]/, JSON.stringify(extra));
    assert.match(m, /Ruling watch/);
  }
});

test("the watch box keeps its region name and its look", () => {
  const m = line(watch({ basis: "method", triage: { route: "needs_review", reason: "an expert reviews" } }));
  assert.match(m, /aria-label="Ruling watch"/);
  assert.match(m, /border-ask-line/);
  assert.match(m, /class="eyebrow/);
});

// ---- F4 / F2 as rendered ---------------------------------------------------

test("a method/regime watch shows its own explanation, not the approving triage reason", () => {
  for (const basis of ["method", "regime"]) {
    const m = text(line(watch({ basis, triage: { route: "auto", reason: "confidence 0.97 >= 0.92 and in scope" } })));
    assert.match(m, /Needs review/);
    assert.doesNotMatch(m, /0\.97 >= 0\.92|in scope/);
    assert.match(m, /not a ruling change/);
  }
});

test("a legacy watch reads Re-check needed in the detail, never Changed or Unchanged", () => {
  for (const status of ["changed", "unchanged"]) {
    const m = text(line(watch({ status })));
    assert.match(m, /Re-check needed/);
    assert.doesNotMatch(m, /Changed:|Unchanged with today/);
    assert.match(m, /Re-check now|Re-check again/);
  }
});

// ---- F2 in the list ---------------------------------------------------------

const row = (extra) => ({ id: "c" + Math.random(), product_name: "p", display_name: "p", date: "2026-02-01", predicted_origin: "China", triage_route: "auto", confidence: 0.9, step: "origin", updated_at: "2026-11-19T00:00:00Z", watch_status: null, scenario_count: 0, pinned: false, ...extra });

test("the cases list shows Re-check needed for legacy watches and counts it with the review set", () => {
  const m = text(html(YourCases, { onChange() {}, cases: [row({ watch_status: "changed" }), row({ watch_status: "unchanged" }), row({ watch_status: "changed", watch_basis: "precedent" }), row({ watch_status: "unchanged", watch_basis: "method" })] }));
  assert.equal((m.match(/Re-check needed/g) ?? []).length, 2);
  assert.equal((m.match(/Needs review/g) ?? []).length, 1);
  assert.equal((m.match(/Changed/g) ?? []).length, 1);
  assert.doesNotMatch(m, /Unchanged/);
});

// ---- Export notes: separate paragraphs, same words -----------------------------

const { ReviewNotes } = await import("../src/components/op/screens/draft-step.tsx");
const DISCLAIMER = "Draft for review by a licensed customs broker or counsel. This is a draft aid. This draft was generated by Unde from the facts below; check every fact and citation before filing.";
const FRAMEWORK = "Legal framework not confirmed: the engine that produced this prediction is not recorded, so Unde cannot tell which legal framework it was made under. The broker or counsel must select the appropriate framework before filing.";

test("the framework note is its own paragraph, apart from the disclaimer, with its words unchanged", () => {
  const m = html(ReviewNotes, { notes: [DISCLAIMER, "Laid out on 19 C.F.R. § 177.2.", FRAMEWORK] });
  const paras = [...m.matchAll(/<p[^>]*>([\s\S]*?)<\/p>/g)].map((x) => text(x[1]));
  assert.equal(paras.length, 3);
  assert.ok(paras.includes(DISCLAIMER));
  assert.ok(paras.includes(FRAMEWORK));
  assert.ok(!paras.some((p) => p.includes("Legal framework not confirmed") && p.includes("licensed customs broker")), "framework note must not share a paragraph with the disclaimer");
  assert.match(m, /<p[^>]*role="note"/);
});

test("the below-bar note stays the amber one and nothing is dropped", () => {
  const below = "Below the confidence bar: Unde's precedent check rated this change 0.80, under its 0.90 bar.";
  const m = text(html(ReviewNotes, { notes: [DISCLAIMER, below, FRAMEWORK] }));
  for (const n of [DISCLAIMER, below, FRAMEWORK]) assert.ok(m.includes(n.slice(0, 60)));
  assert.match(html(ReviewNotes, { notes: [below] }), /border-ask-line bg-ask-wash/);
  assert.equal(html(ReviewNotes, { notes: [] }), "");
});

// ---- Judge panel round 10, N7: the list stacks below 640 px ---------------

test("below 640 px the cases list stacks: no minimum width, no header row, each cell names its column", () => {
  const m = html(YourCases, { onChange() {}, cases: [row({ watch_status: "changed" })] });
  const table = m.match(/<table[^>]*class="([^"]*)"/)[1].split(" ");
  assert.ok(table.includes("sm:min-w-[760px]") && !table.includes("min-w-[760px]"), "the 760 px minimum applies from 640 px only");
  assert.ok(table.includes("max-sm:block"));
  assert.match(m, /<thead class="max-sm:hidden">/);
  assert.match(m, /<tr class="[^"]*max-sm:grid[^"]*"/);
  for (const label of ["Likely origin", "Status", "Ruling watch", "Scenarios", "Where you left off"]) {
    assert.match(m, new RegExp(`<td data-label="${label}" class="[^"]*max-sm:before:content-\\[attr\\(data-label\\)\\]`), label);
  }
});

// ---- N1: the list cell's colour class and the changed+review detail ---------

const cellOf = (extra) => {
  const m = html(YourCases, { onChange() {}, cases: [row(extra)] });
  // The watch cell is the 4th <td> of the row (Product, origin, Status, Ruling watch).
  const cells = [...m.matchAll(/<td(?: data-label="[^"]*")? class="([^"]*)">([\s\S]*?)<\/td>/g)];
  assert.ok(cells.length >= 4, "row has its cells");
  return { cls: cells[3][1], label: text(cells[3][2]) };
};

test("the list cell is styled as attention for Re-check needed, Needs review and Changed, and as quiet for Unchanged", () => {
  for (const [extra, label] of [
    [{ watch_status: "changed" }, "Re-check needed"],
    [{ watch_status: "unchanged" }, "Re-check needed"],
    [{ watch_status: "unchanged", watch_basis: "method" }, "Needs review"],
    [{ watch_status: "unchanged", watch_basis: "precedent", watch_route: "needs_review" }, "Needs review"],
    [{ watch_status: "changed", watch_basis: "precedent", watch_route: "auto" }, "Changed"],
    [{ watch_status: "changed", watch_basis: "precedent", watch_route: "needs_review" }, "Changed"],
  ]) {
    const c = cellOf(extra);
    assert.equal(c.label, label, JSON.stringify(extra));
    assert.match(c.cls, /\bfont-medium\b/, `${label} ${JSON.stringify(extra)}`);
    assert.match(c.cls, /\btext-ask\b/, `${label} ${JSON.stringify(extra)}`);
    assert.doesNotMatch(c.cls, /\btext-ok\b/, `${label} ${JSON.stringify(extra)}`);
  }
  const quiet = cellOf({ watch_status: "unchanged", watch_basis: "precedent", watch_route: "auto" });
  assert.equal(quiet.label, "Unchanged");
  assert.match(quiet.cls, /\btext-ok\b/);
  assert.doesNotMatch(quiet.cls, /\btext-ask\b/);
  assert.match(cellOf({ watch_status: null }).cls, /\btext-dim\b/);
});

test("a changed precedent result needing review shows Changed and the review reason in the detail", () => {
  const m = text(line(watch({ basis: "precedent", triage: { route: "needs_review", reason: "confidence 0.60 is below the bar" } })));
  assert.match(m, /Changed:/);
  assert.match(m, /needs a person to look/);
  assert.match(m, /below the bar/);
  const auto = text(line(watch({ basis: "precedent", triage: { route: "auto", reason: "confidence 0.97 >= 0.92 and in scope" } })));
  assert.match(auto, /Changed:/);
  assert.doesNotMatch(auto, /needs a person|0\.97 >= 0\.92|in scope/);
});
