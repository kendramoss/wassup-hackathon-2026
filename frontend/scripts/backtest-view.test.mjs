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
