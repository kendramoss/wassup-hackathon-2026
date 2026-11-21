// Judge panel round 5 (D310): the heat-gun backtest counts only parts whose
// origin CBP's real change switched, and says what the plan moves that the
// change kept in China. The pinned production result predates the backend's
// new fields, so the view must give the same answer from the stored backtest
// plus the demo's actual_change and the product.
import "./render-support.mjs";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

const { backtestView, backtestMatch, andList } = await import("../src/components/op/plan-bits.tsx");

const fixture = JSON.parse(readFileSync(fileURLToPath(new URL("../../backend/internal/demos/fixtures/heatgun_2022.json", import.meta.url)), "utf8"));
const change = fixture.plan.actual_change;
const product = { ...fixture.predict.product, family: "motorised_tools_small_appliances" };
const triple = [
  { group: "motor_fan_assembly", to_plant: "th" },
  { group: "pcba", to_plant: "th" },
  { group: "switch_cord_minor_parts", to_plant: "th" },
];
const six = ["electric motor", "fan", "printed circuit board assembly (PCBA)", "knob", "power cord", "back cover"];

// What production stored for case_878d76540b91 (docs/NUMBERS.md).
const stored = {
  source: "CBP ruling N357771 (29 Jan 2026): CBP held Thailand",
  actual_moved: [...six, "outer shells"],
  moved_by_plan: six,
  in_actual_change_not_in_plan: ["outer shells"],
};

test("the stored heat-gun backtest reads as all 6 changed parts, plus 3 kept in China", () => {
  const v = backtestView(stored, { change, product, move: triple });
  assert.deepEqual(v.changed, six);
  assert.deepEqual(v.moved, six);
  assert.deepEqual(v.missed, []);
  assert.deepEqual(v.already, ["outer shells"]);
  assert.deepEqual(v.alsoKept, ["switch", "wires", "fasteners"]);
  assert.equal(v.keptGroups.length, 1);
  assert.equal(v.ruling, "N357771");
  assert.equal(v.matches, false, "moving parts CBP's change kept in China is not a match");
});

test("a backtest with the backend's new fields gives the same view", () => {
  const fresh = { ...stored, actual_moved: six, in_actual_change_not_in_plan: [], already_in_target: ["outer shells"], plan_also_moves_kept: ["switch", "wires", "fasteners"] };
  const v = backtestView(fresh, { change, product, move: triple });
  assert.deepEqual(v.changed, six);
  assert.deepEqual(v.alsoKept, ["switch", "wires", "fasteners"]);
  assert.equal(backtestMatch(fresh).matches, false);
});

test("a closest move below the bar is recounted from its groups", () => {
  const v = backtestView({ ...stored, moved_by_plan: [] }, { change, product, move: [{ group: "motor_fan_assembly", to_plant: "th" }], recount: true });
  assert.deepEqual(v.moved, ["electric motor", "fan"]);
  assert.equal(v.missed.length, 4);
  assert.deepEqual(v.alsoKept, []);
});

test("andList", () => {
  assert.equal(andList(["a"]), "a");
  assert.equal(andList(["a", "b", "c"]), "a, b and c");
});

// #519: demos other than the heat gun have no still_from_china_as map; a kept
// part is then found by a component of the same name, in the plan step and in
// the cases-list badge alike.
const demo = (name) => JSON.parse(readFileSync(fileURLToPath(new URL(`../../backend/internal/demos/fixtures/${name}.json`, import.meta.url)), "utf8"));
const live = JSON.parse(readFileSync(fileURLToPath(new URL("../../eval/results/families-live-checks.json", import.meta.url)), "utf8"));

test("the committed gas-grill plan moves parts CBP kept in China, so it is not a match", () => {
  const g = demo("gas_grill_2023");
  const bt = live.runs.find((r) => r.case_id === "gas-grill-2023").backtest;
  const move = ["lid_and_firebox", "burners_and_gas_train", "cooking_surface_frame"].map((group) => ({ group, to_plant: "id" }));
  const opts = { change: g.plan.actual_change, product: g.predict.product, move };
  const v = backtestView(bt, opts);
  assert.deepEqual(v.moved, ["Deep draw main lids", "Formed and punched fireboxes"]);
  assert.deepEqual(v.alsoKept, ["Burners", "Control panels", "Cart frames"]);
  assert.equal(v.matches, false);
  assert.equal(backtestMatch(bt, opts).matches, false, "the cases-list badge agrees with the plan step");
  // The exact move, the lids and fireboxes alone, still matches.
  const exact = { ...opts, move: [{ group: "lid_and_firebox", to_plant: "id" }] };
  assert.equal(backtestView(bt, exact).matches, true);
  assert.equal(backtestMatch(bt, exact).matches, true);
});

test("the tub light: the LED PCBA alone matches; all three groups do not", () => {
  const t = demo("tublight_2025");
  const bt = { source: "CBP ruling", actual_moved: t.plan.actual_change.moved_to_thailand, moved_by_plan: t.plan.actual_change.moved_to_thailand };
  const opts = { change: t.plan.actual_change, product: t.predict.product };
  assert.equal(backtestView(bt, { ...opts, move: [{ group: "led_pcba", to_plant: "th" }] }).matches, true);
  const all = ["led_pcba", "lens", "base_harness_seals"].map((group) => ({ group, to_plant: "th" }));
  const v = backtestView(bt, { ...opts, move: all });
  assert.deepEqual(v.alsoKept, ["lens", "lamp base", "wiring harness", "PVC sealing and O-ring"]);
  assert.equal(v.matches, false);
});

test("every demo with a kept-in-China list resolves each part, but the heat gun's terminal blocks", async () => {
  const { keptPart } = await import("../src/components/op/plan-bits.tsx");
  const { readdirSync } = await import("node:fs");
  const names = readdirSync(fileURLToPath(new URL("../../backend/internal/demos/fixtures/", import.meta.url))).filter((f) => f.endsWith(".json")).map((f) => f.slice(0, -5));
  const withChange = names.filter((n) => demo(n).plan?.actual_change);
  assert.ok(withChange.length >= 3, String(withChange));
  for (const name of withChange) {
    const d = demo(name);
    for (const w of d.plan.actual_change.still_from_china ?? []) {
      if (w === "terminal blocks") continue;
      assert.ok(keptPart(d.plan.actual_change, d.predict.product, w), `${name}: ${w}`);
    }
  }
});
