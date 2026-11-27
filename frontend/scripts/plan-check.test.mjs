import "./render-support.mjs";
import assert from "node:assert/strict";
import { afterEach, test } from "node:test";
import { createElement as h } from "react";
import { json, stubNavigation } from "./lib/hook-runtime.mjs";
import { deepMount, textOf } from "./lib/deep-mount.mjs";

// D220 (judge panel round 2): one verdict per move. Scenarios shows the plan
// search's precedent check for the same move in Plan's words, never calls a
// below-bar move auto-accepted or the best, and says why the Origin step and
// the "As today" what-if can differ. Shaped on the pinned heat-gun demo.
globalThis.window = new EventTarget();
globalThis.document = new EventTarget();
stubNavigation({ id: "c1" });
const live = [];
afterEach(() => live.splice(0).forEach((a) => a.unmount()));

const { engineLabel, highestNet, planCheckWords, readsDiffer, asToday } = await import("../src/lib/plan-check.ts");
const { movePhrase, movedParts } = await import("../src/components/op/plan-bits.tsx");
const ScenariosView = (await import("../src/components/op/screens/scenarios-view.tsx")).ScenariosView;
const OriginStep = (await import("../src/components/op/screens/origin-step.tsx")).OriginStep;

const motorCheck = { shown: false, confidence: 0.9, bar: 0.92, rejection_kind: "below_confidence_bar", rejection_reason: "precedent-supported but verifier confidence 0.90 is below 0.92", supporting_ruling: "N325810", description: "Move the motor-fan assembly to Thailand", net_per_year: 380000, target: "Thailand" };
const bothCheck = { shown: false, confidence: 0.85, bar: 0.92, rejection_kind: "no_supporting_precedent", rejection_reason: "no earlier ruling found where CBP held this kind of change decisive", description: "Move the motor-fan assembly and the PCBA (SMT) to Thailand", net_per_year: 320000, target: "Thailand" };
const product = {
  name: "heat gun",
  plants: [{ id: "cn", name: "China plant", country: "China" }, { id: "th", name: "Rayong", country: "Thailand" }],
  components: [
    { name: "electric motor", made_in: "China", plant: "cn", group: "motor_fan_assembly" },
    { name: "PCBA", made_in: "China", plant: "cn", group: "pcba" },
    { name: "main housing", made_in: "Thailand", plant: "th", group: "housing_moulding" },
  ],
  operations: [{ name: "wind the motor stator", plant: "cn", group: "motor_fan_assembly" }],
};
const scenarios = [
  { label: "As today", product, prediction: { origin_country: "Thailand", confidence: 0.78, calibrated_confidence: 0.6 }, triage: { route: "needs_review" }, comparison: { net_usd: 0, moved: [] } },
  { label: "motor_fan_assembly in Thailand", product, prediction: { origin_country: "Thailand", confidence: 0.98, calibrated_confidence: 0.99 }, triage: { route: "auto" }, comparison: { net_usd: 380000, moved: [{ group: "motor_fan_assembly", to_plant: "th" }] }, plan_check: motorCheck },
  { label: "motor_fan_assembly + pcba in Thailand", product, prediction: { origin_country: "Thailand", confidence: 0.99, calibrated_confidence: 0.99 }, triage: { route: "auto" }, comparison: { net_usd: 320000, moved: [{ group: "motor_fan_assembly", to_plant: "th" }, { group: "pcba", to_plant: "th" }] }, plan_check: bothCheck },
];
const best = { description: "Move the motor-fan assembly, the PCBA (SMT) and the switch, cord and minor parts to Thailand", shown: true, net_per_year: 285000, net_priced: true, move: [{ group: "motor_fan_assembly", to_plant: "th" }, { group: "pcba", to_plant: "th" }], verdict: { confidence: 0.96, supporting_ruling: "N345938" }, prediction: { origin_country: "Thailand", confidence: 0.99 } };
const caseJson = {
  id: "c1", pinned: true, can_write: false, date: "2026-01-14", display_name: "Heat gun", step: "plan", facts: "In your letter dated October 25, 2022, you requested a ruling.", facts_source: "pasted",
  prediction: { origin_country: "China", confidence: 0.85, calibrated_confidence: 0.6 }, triage: { route: "needs_info" }, current_origin: { country: "China", ruling: "N328954" },
  product, scenarios, plan: { target: "Thailand", plans: [best], rejected: [], baseline: { prediction: { origin_country: "Thailand", confidence: 0.78, calibrated_confidence: 0.6 } } },
  exclude_rulings: ["N328954", "N357771"],
  watch: { status: "unchanged", checked_at: "2026-10-16T16:26:11Z", prediction: { origin_country: "China" }, newer_rulings: [] },
};

async function screen(Step, c) {
  globalThis.fetch = async (url, init = {}) => {
    const u = String(url);
    if (/\/cases\/c1$/.test(u) && (init.method ?? "GET") === "GET") return { ok: true, status: 200, headers: new Headers(), json: async () => c };
    if (u.includes("/demos")) return json({ demos: [] });
    if (u.endsWith("/api/session")) return json({ email: null });
    return json({});
  };
  const el = Step();
  const app = deepMount(h(el.type, el.props));
  live.push(app);
  app.refresh();
  await app.flush();
  return app;
}
const all = (app) => app.all().map((e) => textOf(e.props?.children)).join(" ");

test("a below-bar move reads as Plan reads it", () => {
  const w = planCheckWords(motorCheck);
  assert.equal(w.supported, false);
  assert.equal(w.chip, "△ Below the bar");
  assert.equal(w.line, "0.90 < 0.92");
  assert.match(planCheckWords(bothCheck).line, /^No earlier ruling/);
  assert.equal(planCheckWords({ ...motorCheck, shown: true, confidence: 0.96 }).supported, true);
});

test("no 'highest net' for a move the plan search did not show", () => {
  assert.equal(highestNet(scenarios), null);
  // A shown move can win; an unchecked one wins only with a caption.
  const shown = scenarios.map((s, i) => (i === 1 ? { ...s, plan_check: { ...motorCheck, shown: true } } : s));
  assert.deepEqual(highestNet(shown), { index: 1, checked: true });
  const unchecked = scenarios.map((s) => ({ ...s, plan_check: undefined }));
  assert.deepEqual(highestNet(unchecked), { index: 1, checked: false });
});

test("the engine's own answer is labelled as the engine's", () => {
  assert.equal(engineLabel(scenarios[1]), "Engine: Thailand, 0.99");
});

test("two inputs, two answers, said where both appear", () => {
  const t = readsDiffer(caseJson, asToday(scenarios));
  assert.match(t, /CBP's own text says China/);
  assert.match(t, /the parts list says Thailand/);
  assert.match(t, /Both are unsure/);
  assert.equal(readsDiffer({ ...caseJson, facts_source: "parts_list" }, scenarios[0]), null);
});

test("scenarios: the heat-gun cards agree with Plan", async () => {
  const text = all(await screen(ScenariosView, caseJson));
  assert.doesNotMatch(text, /Auto-accepted/);
  assert.doesNotMatch(text, /highest net/);
  assert.match(text, /Below the bar\s*0\.90 < 0\.92/);
  assert.match(text, /The plan search recommends:/);
  assert.match(text, /Move the motor-fan assembly, the PCBA \(SMT\) and the switch, cord and minor parts to Thailand/);
  assert.match(text, /Engine: Thailand, 0\.99/);
  assert.match(text, /Two inputs, two answers/);
});

test("origin: the pinned demo explains its watch and the second reading", async () => {
  const c = { ...caseJson, watch: { ...caseJson.watch, basis: undefined } };
  const text = all(await screen(OriginStep, c));
  assert.match(text, /Two inputs, two answers/);
  assert.match(text, /not available on this demo/);
  assert.doesNotMatch(text, /older version of UndeLabs that did not record/);
});

test("a plan names what it moves", () => {
  assert.equal(movePhrase(best), "the motor-fan assembly, the PCBA (SMT) and the switch, cord and minor parts");
  // Parts, not process steps; a group's steps only when none of its parts move (D270).
  assert.deepEqual(movedParts(product, { move: [{ group: "motor_fan_assembly", to_plant: "th" }] }), [{ group: "motor_fan_assembly", items: ["electric motor"] }]);
  const stepsOnly = { ...product, components: product.components.filter((c) => c.group !== "motor_fan_assembly") };
  assert.deepEqual(movedParts(stepsOnly, { move: [{ group: "motor_fan_assembly", to_plant: "th" }] }), [{ group: "motor_fan_assembly", items: ["wind the motor stator"] }]);
});
