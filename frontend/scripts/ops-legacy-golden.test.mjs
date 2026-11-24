import "./render-support.mjs";
import assert from "node:assert/strict";
import { readFileSync, writeFileSync } from "node:fs";
import { afterEach, test } from "node:test";
import { fileURLToPath } from "node:url";
import { stubNavigation } from "./lib/hook-runtime.mjs";
import { html } from "./render-support.mjs";

// #281 legacy goldens: with no operating costs (no `operations` anywhere) every
// result screen and the money forms render as they did before this feature.
// scripts/golden/ops-legacy.json was captured by running THIS file against the
// unmodified base source (4e76b99), not against the new code:
//   OPS_GOLDEN_WRITE=1 node --test scripts/ops-legacy-golden.test.mjs   (in a copy of the base tree)
// The only difference allowed in the money forms is the new, collapsed-by-
// default "Operating costs" section, which is removed before comparing.
// D341 re-captured the plan screen once, on purpose: the only changes are the
// added tier and smallest-move chips, the stored-order note and the list
// heading ("best net first"); every other piece is unchanged.
// D371 re-captured the compare screen once, on purpose: the only changes are
// the caption and fold text saying countries rank by their best plan's tier,
// and the tier chip under each shown plan's check.
// #557 re-captured the money drawer once, on purpose: the only changes are the
// ARIA tabs pattern on "How to enter costs" (tab ids, aria-controls, roving
// tabindex, and a labelled tabpanel wrapper). Every other piece is unchanged.
// D562 replaced the plan screen's scope line in place (planScope said plans
// apply no Part 102 rules, false since D500 and D541); no markup changed.
// D580 replaced the family count in that line in place (14 → 20 families); no
// markup changed. D660 did the same (20 → 26 families), and D826 (26 → 27).
// Copy sweep 1 re-captured plan, compare and scenarios once, on purpose: copy
// is shorter, and the review note's bounded and scope lines moved under a
// "Scope and limits" disclosure (same words), the step tabs say "Draft" and
// "Scenarios", and the Ask button says "Ask" (its accessible name is
// unchanged). No money or ops markup changed.
// #685 re-captured compare once, on purpose: the table caption is shorter
// ("Ranked, best plan first") and the full tier order it listed moved into the
// "How the ranking works" fold (one added sentence). No other markup changed.
// #771 re-captured plan, planBelow, planNone, compare and scenarios once, on
// purpose: the step tabs gain the optional "All checks" tab (one li, its link
// and its "· optional" mark). No other markup changed.
globalThis.window = new EventTarget();
globalThis.document = Object.assign(new EventTarget(), { body: { style: {} }, activeElement: null });
stubNavigation({ id: "c1", search: "target=Thailand" });
const F = await import("./lib/ops-fixtures.mjs");
const { PlanStep } = await import("../src/components/op/screens/plan-step.tsx");
const { CompareView } = await import("../src/components/op/screens/compare-view.tsx");
const { ScenariosView } = await import("../src/components/op/screens/scenarios-view.tsx");
const { MoneyDrawer } = await import("../src/components/op/money-drawer.tsx");
const { EconomicsEditor } = await import("../src/components/op/plan-bits.tsx");

const GOLDEN = fileURLToPath(new URL("./golden/ops-legacy.json", import.meta.url));
const WRITE = process.env.OPS_GOLDEN_WRITE === "1";
const live = [];
afterEach(() => live.splice(0).forEach((a) => a.unmount()));
const mount = F.makeMount(live);
const got = {};

const nameOf = (t) => (typeof t === "string" ? t : (t?.displayName ?? t?.name ?? "?"));
// Only the text a host element holds itself: the rest is its own entries in the
// flattened tree, so wrapping markup in a new component changes nothing here.
const own = (n) => (Array.isArray(n) ? n.map(own).join("") : typeof n === "string" || typeof n === "number" ? String(n) : "");
const serialize = (app) =>
  app
    .all()
    .filter((e) => typeof e.type === "string")
    .map((e) => {
      const p = e.props ?? {};
      return [e.type, p.className ?? "", p.disabled ? "disabled" : "", p.title ?? "", p["aria-label"] ?? "", p.href ?? "", own(p.children)].join("|");
    })
    .join("\n");
void nameOf;

// ---- legacy DTOs: no operations, no objective, no economics_basis ---------------------------
const verdict = (c = 0.96) => ({ confidence: c, agreement: "3/3", supporting_ruling: "N325810", counter_ruling: "N111111", reasoning: "Because." });
const legacyMove = (extra = {}) => ({
  key: "motor_fan_assembly@th", description: "Move the motor-fan assembly to Thailand", move: [{ group: "motor_fan_assembly", to_plant: "th" }], shown: true,
  prediction: { origin_country: "Thailand", confidence: 0.95 }, verdict: verdict(), duty_effect_per_year: 500000, added_cost_per_year: 115000, net_per_year: 385000, net_priced: true, ...extra,
});
const legacyPlan = () => ({
  target: "Thailand", plans: [legacyMove(), legacyMove({ key: "pcba@th", description: "Move the PCBA to Thailand", move: [{ group: "pcba", to_plant: "th" }], net_per_year: 120000, added_cost_per_year: 30000, duty_effect_per_year: 150000 })],
  rejected: [
    legacyMove({ key: "housing@th", description: "Move the housing to Thailand", shown: false, rejection_kind: "below_confidence_bar", verdict: verdict(0.8), net_per_year: 90000 }),
    legacyMove({ key: "switch@th", description: "Move the switch to Thailand", shown: false, rejection_kind: "no_supporting_precedent", rejection_reason: "no earlier ruling supports this move", net_per_year: 10000 }),
    legacyMove({ key: "", description: "No change", shown: false, rejection_kind: "current_setup", net_per_year: 0, move: [] }),
  ],
  stats: { moves_searched: 4, engine_calls: 5, wall_seconds: 12 }, duty_rates_note: "USITC fetched 2026-11-01",
});
const legacyCase = (extra = {}) => F.caseJson({ economics: { illustrative: true, annual_import_value_usd: 2000000, duty_rates: { China: 0.417, Thailand: 0.167 }, group_costs: { motor_fan_assembly: { cn: 300000, th: 340000 } } }, ...extra });
const ranked = [
  { rank: 1, target: "Thailand", status: "plan", moves_searched: 4, best: legacyMove() },
  { rank: 2, target: "India", status: "below_bar", moves_searched: 4, best: legacyMove({ description: "Move the PCBA to India", verdict: verdict(0.8), net_per_year: 5000 }) },
  { rank: 3, target: "Vietnam", status: "no_plan", moves_searched: 4 },
];
const scenario = (cmp) => ({ label: "Move the motor", prediction: { origin_country: "Thailand", confidence: 0.9 }, triage: { route: "auto" }, product: F.product, comparison: cmp });
const legacyScenarios = () => [
  scenario({ duty_rate: 0.167, annual_duty_usd: 334000, duty_savings_usd: 500000, added_cost_usd: 115000, net_usd: 385000, moved: [{ group: "motor_fan_assembly", to_plant: "th" }], illustrative: true, origin_differs_from_current: true }),
  scenario({ duty_rate: null, annual_duty_usd: null, duty_savings_usd: null, added_cost_usd: null, net_usd: null, moved: [{ group: "pcba", to_plant: "th" }], illustrative: true }),
];

// ---- screens ------------------------------------------------------------------------------------------
test("legacy plan screen (result, rejected rows, all moves open)", async () => {
  const app = await mount(PlanStep, legacyCase({ plan: legacyPlan() }));
  await app.btn("more")?.props.onClick();
  await app.flush();
  got.plan = serialize(app);
});
test("legacy plan screen, below the bar", async () => {
  const p = legacyPlan();
  p.plans = [];
  const app = await mount(PlanStep, legacyCase({ plan: p }));
  got.planBelow = serialize(app);
});
test("legacy plan screen, nothing cleared", async () => {
  const app = await mount(PlanStep, legacyCase({ plan: { ...legacyPlan(), plans: [], rejected: [legacyMove({ shown: false, rejection_kind: "no_supporting_precedent", rejection_reason: "none" })] } }));
  got.planNone = serialize(app);
});
test("legacy compare screen", async () => {
  const app = await mount(CompareView, legacyCase({ targets: { ranked } }));
  got.compare = serialize(app);
});
test("legacy scenarios screen (cards and matrix)", async () => {
  const app = await mount(ScenariosView, legacyCase({ step: "origin", scenarios: legacyScenarios() }));
  got.scenarios = serialize(app);
});

// ---- forms: the same markup apart from the new collapsed section -------------------------------------
const stripOps = (m) => m.replace(/<section aria-label="Operating costs"[\s\S]*?<\/section>/g, "");
test("legacy money drawer markup", () => {
  got.drawer = stripOps(html(MoneyDrawer, { value: legacyCase().economics, product: F.product, countries: ["China", "Thailand"], onSave() {}, onClose() {} }));
});
test("legacy economics editor markup", () => {
  got.editor = stripOps(html(EconomicsEditor, { value: legacyCase().economics, onChange() {}, product: F.product, countries: ["China", "Thailand"] }));
});

// ---- compare with / write the golden ------------------------------------------------------------------
test("the legacy goldens match the capture from the base source", () => {
  if (WRITE) {
    writeFileSync(GOLDEN, JSON.stringify(got, null, 1) + "\n");
    return;
  }
  const want = JSON.parse(readFileSync(GOLDEN, "utf8"));
  assert.deepEqual(Object.keys(got).sort(), Object.keys(want).sort());
  for (const k of Object.keys(want)) {
    if (got[k] !== want[k]) {
      const a = got[k].split(/\n|(?=<)/), b = want[k].split(/\n|(?=<)/);
      const i = a.findIndex((x, n) => x !== b[n]);
      assert.fail(`${k} differs at piece ${i}:\n now:  ${a[i]}\n base: ${b[i]}`);
    }
  }
});
