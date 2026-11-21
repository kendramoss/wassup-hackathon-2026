import "./render-support.mjs";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { afterEach, test } from "node:test";
import { fileURLToPath } from "node:url";
import { stubNavigation } from "./lib/hook-runtime.mjs";

// Judge panel round 7, N1: on a cold load of the pinned heat-gun plan, the
// backtest was drawn before the demo's record of the real change arrived, so
// it read "switched 7 parts, moves 6" with no kept-in-China parts. The plan
// step now waits for that record (or says it did not load) and never draws
// the stored backtest without it.
globalThis.window = new EventTarget();
globalThis.document = Object.assign(new EventTarget(), { body: { style: {} }, activeElement: null });
stubNavigation({ id: "c1", search: "target=Thailand" });
const F = await import("./lib/ops-fixtures.mjs");
const { PlanStep } = await import("../src/components/op/screens/plan-step.tsx");
const { backtestState } = await import("../src/components/op/plan-bits.tsx");
const { copy } = await import("../src/lib/copy.ts");

const live = [];
afterEach(() => live.splice(0).forEach((a) => a.unmount()));
const mount = F.makeMount(live);

const fixture = JSON.parse(readFileSync(fileURLToPath(new URL("../../backend/internal/demos/fixtures/heatgun_2022.json", import.meta.url)), "utf8"));
const change = fixture.plan.actual_change;
const product = { ...fixture.predict.product, family: "motorised_tools_small_appliances" };
const six = ["electric motor", "fan", "printed circuit board assembly (PCBA)", "knob", "power cord", "back cover"];
// The backtest production stored for the pinned case: no already_in_target.
const stored = { source: change._source, actual_moved: [...six, "outer shells"], moved_by_plan: six, in_actual_change_not_in_plan: ["outer shells"] };
const triple = [
  { group: "motor_fan_assembly", to_plant: "th" },
  { group: "pcba", to_plant: "th" },
  { group: "switch_cord_minor_parts", to_plant: "th" },
];
const plan = F.planResult({ plans: [F.move({ key: "triple", move: triple, operations: undefined })], backtest: stored });
delete plan.objective;
delete plan.economics_basis;
const pinned = () => F.caseJson({ product, plan, pinned: true });

// A fake demos API whose list answers only when `release` is called (or
// fails when `fail` is).
function lateDemos({ demoFails = false } = {}) {
  let release, fail;
  const gate = new Promise((res, rej) => {
    release = res;
    fail = rej;
  });
  const serve = (u, m) => {
    if (m !== "GET") return null;
    if (u.endsWith("/demos")) return gate.then(() => F.json({ demos: [{ id: "heatgun_2022", has_plan: true }] }), () => F.json({ code: "unavailable", message: "down" }, 503));
    if (u.endsWith("/demos/heatgun_2022")) return demoFails ? F.json({ code: "unavailable", message: "down" }, 503) : F.json({ id: "heatgun_2022", plan: { actual_change: change } });
    return null;
  };
  return { serve, release: () => release(), fail: () => fail(new Error("down")) };
}

test("backtestState waits only for a stored backtest that needs the demo's record", () => {
  assert.equal(backtestState(undefined, false, null), "ready");
  assert.equal(backtestState(stored, true, null), "ready", "the plan request carries the change");
  assert.equal(backtestState({ ...stored, already_in_target: ["outer shells"] }, false, null), "ready", "new backtests carry it");
  assert.equal(backtestState(stored, false, null), "loading");
  assert.equal(backtestState(stored, false, { source: "another ruling", state: "found" }), "loading", "a record for another source does not count");
  assert.equal(backtestState(stored, false, { source: stored.source, state: "found" }), "ready");
  assert.equal(backtestState(stored, false, { source: stored.source, state: "none" }), "ready");
  assert.equal(backtestState(stored, false, { source: stored.source, state: "failed" }), "failed");
});

test("the backtest is not drawn until the demo's change arrives, then reads all 6 with the kept parts", async () => {
  const demos = lateDemos();
  const app = await mount(PlanStep, pinned(), demos.serve);
  const before = F.flatText(app);
  assert.match(before, new RegExp(copy.plan.backtestLoading.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  assert.doesNotMatch(before, /switched 7|of the 7 parts|7 parts that changed|\(not moved by this plan\)/);
  assert.doesNotMatch(before, /Backtest:/, "the band waits too");

  demos.release();
  await app.flush();
  await app.flush();
  const after = F.flatText(app);
  assert.doesNotMatch(after, new RegExp(copy.plan.backtestLoading.slice(0, 20)));
  assert.match(after, /all 6/);
  assert.doesNotMatch(after, /7 parts|of the 7/);
  assert.match(after, /switch, wires and fasteners/);
});

test("if the demo lookup fails, the stored backtest is not shown and the page says so", async () => {
  const demos = lateDemos();
  const app = await mount(PlanStep, pinned(), demos.serve);
  demos.fail();
  await app.flush();
  await app.flush();
  const text = F.flatText(app);
  assert.match(text, /did not load, so the backtest is not shown/);
  assert.doesNotMatch(text, /7 parts|of the 7|Backtest:/);
  assert.ok(app.btn(copy.plan.backtestRetry), "a retry button");
});

// #565: the list loads but the one demo that holds the record does not. That
// is a failed lookup, not "no record", so the stale 7-part view never draws.
test("if a demo in the list cannot be read, the lookup fails and offers Try again", async () => {
  const demos = lateDemos({ demoFails: true });
  const app = await mount(PlanStep, pinned(), demos.serve);
  demos.release();
  await app.flush();
  await app.flush();
  await app.flush();
  const text = F.flatText(app);
  assert.match(text, /did not load, so the backtest is not shown/);
  assert.doesNotMatch(text, /7 parts|of the 7/);
  assert.doesNotMatch(text, /\(not moved by this plan\)/);
  assert.doesNotMatch(text, /Backtest:/, "the band does not draw it either");
  assert.ok(app.btn(copy.plan.backtestRetry), "a retry button");
});
