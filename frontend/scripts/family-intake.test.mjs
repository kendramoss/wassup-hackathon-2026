import assert from "node:assert/strict";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

// The family questions (#770, D810-D813): missing information is asked with
// the document that settles it and the rulings that turn on it; a family the
// case is not modelled for gets a reason and no control; answers sit beside
// the case. Synthetic intakes shaped like the API's (the Go tests run the
// committed templates).
const { FamilyIntakePanel } = await import("../src/components/op/family-intake.tsx");
const F = await import("../src/lib/family-intake.ts");
const { copy } = await import("../src/lib/copy.ts");

const cells = {
  id: "cells_made",
  kind: "country",
  question: "Where were the battery cells made?",
  evidence: "Bill of materials with the origin of each part, or the cell maker's certificate of origin",
  state: "missing",
  groups: ["battery_cells"],
  confers_origin: ["N347193", "N347172"],
  not_enough: ["N348871"],
};
const pack = {
  id: "pack_work",
  kind: "yes_no",
  question: "Does pack assembly include programming the battery management system (BMS), welding the cells and formation?",
  evidence: "Production routing or traveller for pack assembly",
  state: "missing",
  groups: ["battery_cells", "final_assembly"],
  confers_origin: [],
  not_enough: [],
};
const housing = { ...cells, id: "bms_made", question: "Where was the battery management board (BMS) made?", state: "known", known: "Mexico, from the parts list" };
const intake = (questions, extra = {}) => ({ version: "clarify-family-v1", status: "missing_information", family: "power_supplies_chargers", family_name: "Power supplies, chargers and battery packs", template_version: "2", questions, ...extra });
const c = (fi, extra = {}) => ({ id: "case_1", family_intake: fi, ...extra });

test("each missing fact gets a control, its document and its rulings", () => {
  const m = html(FamilyIntakePanel, { c: c(intake([cells, pack, housing])), setCase: () => {} });
  const t = text(m);
  for (const q of [cells, pack]) {
    assert.ok(t.includes(q.question), q.question);
    assert.ok(t.includes(q.evidence), q.evidence);
  }
  assert.ok(t.includes(copy.family.title));
  assert.match(m, /<input[^>]*id="[^"]*-cells_made"[^>]*list=/);
  assert.equal((m.match(/type="radio"/g) ?? []).length, 2, "yes and no for the pack question");
  assert.ok(t.includes("N347193") && t.includes(copy.family.notEnough), "rulings shown");
  // Known from the parts list: listed, not asked.
  assert.ok(t.includes("Mexico, from the parts list"));
  assert.ok(!/id="[^"]*-bms_made"/.test(m));
  // Phone-sized targets: 44px controls.
  assert.match(m, /min-h-11/);
});

test("not modelled: a reason, no control", () => {
  const fi = { version: "clarify-family-v1", status: "not_modelled", questions: [], not_modelled: { code: "guard", reason: "No questions for Power supplies, chargers and battery packs: a device that runs on a battery is not a battery pack or power supply." } };
  const m = html(FamilyIntakePanel, { c: c(fi), setCase: () => {} });
  assert.ok(!/<(input|select|form|button)/.test(m), "no controls");
  assert.ok(text(m).includes(copy.family.notModelled) && text(m).includes("not a battery pack"));
  assert.equal(html(FamilyIntakePanel, { c: { id: "x" }, setCase: () => {} }), "", "nothing without an intake");
});

test("answers are filled from what was saved and sent whole", () => {
  const fi = intake([{ ...cells, state: "answered", answer: { country: "South Korea" } }, { ...pack, state: "answered", answer: { yes: false } }, housing]);
  assert.deepEqual(F.initialForm(fi), { cells_made: "South Korea", pack_work: "no" });
  const saved = { source: "user-supplied", updated_at: "2026-11-24T06:00:00Z", family: "power_supplies_chargers", template_version: "2", answers: {} };
  assert.deepEqual(F.body(fi, { cells_made: " China ", pack_work: "yes", bms_made: "Peru" }, saved), {
    expected_updated_at: "2026-11-24T06:00:00Z",
    answers: { cells_made: { country: "China" }, pack_work: { yes: true } },
  });
  assert.deepEqual(F.body(fi, { cells_made: "" }), { expected_updated_at: "", answers: {} }, "blank is never an answer");
  assert.equal(F.changed(fi, F.initialForm(fi)), false);
  assert.equal(F.changed(fi, { ...F.initialForm(fi), cells_made: "Japan" }), true);
  assert.deepEqual(F.formErrors(fi, { cells_made: "<b>" }), ["cells_made"]);
  assert.deepEqual(F.asked({ status: "not_modelled", questions: [cells] }), []);
});

test("saved answers are listed apart, stale ones flagged", () => {
  const saved = { source: "user-supplied", updated_at: "2026-11-24T06:00:00Z", family: "power_supplies_chargers", template_version: "2", answers: { cells_made: { country: "China" } }, stale: true };
  const t = text(html(FamilyIntakePanel, { c: c(intake([cells], { stale: true }), { family_facts: saved }), setCase: () => {} }));
  assert.ok(t.includes(copy.family.yours) && t.includes(copy.family.stale));
});

test("the case list says when answers re-ran the check or came after it", () => {
  assert.equal(copy.cases.answers.rechecked, "Re-checked with your answers");
  assert.ok(copy.cases.answers.added.includes("Open"));
});
