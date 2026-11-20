import "./render-support.mjs";
import assert from "node:assert/strict";
import { afterEach, test } from "node:test";
import { stubNavigation } from "./lib/hook-runtime.mjs";

// Final browser blocker (C2): a case whose STORED economics carry active
// operating inputs and an annual value of 0, where the user turns Mode Off in
// the plan screen's money drawer and searches again. A plan POST with annual 0
// and no operations key means "use the saved money" (kept for legacy clients),
// so the discard must be saved first with the EXISTING PUT /economics. Real
// component callbacks (CaseShell + useCase + PlanStep) over a fetch spy; no
// browser, no network, no backend.
globalThis.window = new EventTarget();
globalThis.document = Object.assign(new EventTarget(), { body: { style: {} }, activeElement: null });
stubNavigation({ id: "c1", search: "target=Thailand" });
const F = await import("./lib/ops-fixtures.mjs");
const { move, planResult, caseJson, json, flatText } = F;
const { PlanStep } = await import("../src/components/op/screens/plan-step.tsx");
const { ACCESS_STALE } = await import("../src/lib/api/client.ts");

const live = [];
afterEach(() => live.splice(0).forEach((a) => a.unmount()));
const mount = F.makeMount(live);

const OPS = { horizon_years: 4, groups: { motor_fan_assembly: { cost_basis: "additional", plants: { th: { ref: { name: "Chonburi works", country: "Thailand" }, labour_usd_per_year: 9 } } } } };
const MONEY = { illustrative: true, annual_import_value_usd: 0, duty_rates: { China: 0.417, Thailand: 0.167 }, group_costs: { motor_fan_assembly: { cn: 300000, th: 340000 } }, duty_rates_note: "from the table", duty_rates_from_table: ["China"], duty_additional: { China: [{ name: "301", rate: 0.25 }] } };
const stored = (extra = {}) => ({ ...MONEY, operations: OPS, ...extra });
const discardedMoney = () => ({ ...MONEY });
const OLD = planResult();
const NEW = planResult({ plans: [move({ net_per_year: 0, operations: undefined })], objective: undefined, economics_basis: undefined });
delete NEW.objective;

const isPlan = (x) => x.m === "POST" && x.u.endsWith("/plan");
const isPut = (x) => x.m === "PUT" && x.u.endsWith("/economics");
const writes = (app) => app.calls.filter((x) => isPlan(x) || isPut(x));
const sw = (app) => app.find((e) => e.type === "input" && e.props.role === "switch")[0];
const openDrawer = async (app) => {
  app.find((e) => e.type === "button" && e.props["aria-haspopup"] === "dialog")[0].props.onClick();
  await app.flush();
};
const modeOff = async (app) => {
  sw(app).props.onChange({ target: { checked: false } });
  await app.flush();
  await app.btn("Discard inputs").props.onClick();
  await app.flush();
};
const searchAgain = async (app) => {
  await app.btn("Search again with these inputs").props.onClick();
  await app.flush();
};
// A server that stores the PUT body whole and returns the case as stored.
function server(c, o = {}) {
  const { planBody = { case_id: "c1", plan: NEW }, gate } = o;
  return (u, m, body, state) => {
    if (m === "PUT" && u.endsWith("/economics")) {
      const putStatus = o.putStatus;
      if (putStatus) return json({ code: putStatus === 403 ? "forbidden" : "internal", message: "refused" }, putStatus);
      const go = () => {
        if (state.current.id === "c1") {
          state.current = { ...state.current, economics: body };
          delete state.current.plan_economics_stale;
        }
        return json({ case_id: "c1", economics: body });
      };
      return gate ? gate.then(go) : go();
    }
    if (m === "POST" && u.endsWith("/plan")) {
      state.current = { ...state.current, plan: planBody.plan, economics: state.current.economics };
      return json(planBody);
    }
    return null;
  };
}
const base = (extra = {}) => caseJson({ plan: OLD, economics: stored(), ...extra });

test("(a)(i) stored ops + annual 0 + Mode Off + Search again: one PUT (whole economics, no operations) THEN one plan POST (no operations); case has no ops, no stale flag", async () => {
  const c = base({ plan_economics_stale: true });
  const app = await mount(PlanStep, c, server(c));
  await openDrawer(app);
  await modeOff(app);
  await searchAgain(app);
  const w = writes(app);
  assert.deepEqual(w.map((x) => x.m + " " + x.u.replace(/^.*\/cases\/c1/, "")), ["PUT /economics", "POST /plan"]);
  assert.equal("operations" in w[0].body, false, "the PUT body has no operations key");
  assert.deepEqual(w[0].body, discardedMoney(), "whole current economics, other money and provenance intact");
  assert.equal(w[0].raw, JSON.stringify(discardedMoney()));
  assert.equal("operations" in w[1].body.economics, false, "plan POST has no operations key");
  assert.equal(w[1].body.economics.annual_import_value_usd, 0);
  assert.equal(w[1].body.target, "Thailand");
  assert.equal(app.state.current.economics.operations, undefined, "stored operations are gone");
  assert.equal("plan_economics_stale" in app.state.current, false);
  assert.doesNotMatch(flatText(app), /operating costs entered/);
  assert.equal(app.state.current.plan.objective, undefined, "legacy result: no objective");
});

test("(b) the PUT fails (403, 409, 500): no plan POST, error shown, drawer still open, inputs as the user left them", async () => {
  for (const status of [403, 409, 500]) {
    const c = base();
    const app = await mount(PlanStep, c, server(c, { putStatus: status }));
    await openDrawer(app);
    await modeOff(app);
    await searchAgain(app);
    assert.equal(app.calls.filter(isPlan).length, 0, `no plan after a ${status}`);
    assert.equal(app.calls.filter(isPut).length, 1);
    assert.ok(app.btn("Search again with these inputs"), "drawer is still open");
    assert.equal(sw(app).props.checked, false, "Mode Off stays as left");
    assert.match(flatText(app), /refused|viewer|Try again/i, "an error is shown");
    assert.equal(app.state.current.economics.operations.horizon_years, 4, "nothing was stored");
    app.unmount();
  }
});

test("(c) positive annual: no PUT, one plan POST, unchanged", async () => {
  const c = base({ economics: stored({ annual_import_value_usd: 2000000 }) });
  const app = await mount(PlanStep, c, server(c));
  await openDrawer(app);
  await modeOff(app);
  await searchAgain(app);
  const w = writes(app);
  assert.deepEqual(w.map((x) => x.m), ["POST"]);
  assert.equal("operations" in w[0].body.economics, false);
  assert.equal(w[0].body.economics.annual_import_value_usd, 2000000);
});

test("(d) a legacy case that never had operations: no PUT, plan POST byte-identical to the legacy body", async () => {
  const c = base({ plan: undefined, economics: MONEY });
  const app = await mount(PlanStep, c, server(c));
  await app.btn("Search moves to").props.onClick();
  await app.flush();
  const w = writes(app);
  assert.deepEqual(w.map((x) => x.m), ["POST"]);
  assert.equal(w[0].raw, JSON.stringify({ target: "Thailand", economics: MONEY }));
  // same through the drawer with Mode Off never touched
  const c2 = base({ economics: MONEY });
  const app2 = await mount(PlanStep, c2, server(c2));
  await openDrawer(app2);
  await searchAgain(app2);
  assert.deepEqual(writes(app2).map((x) => x.m), ["POST"]);
  assert.equal(writes(app2)[0].raw, JSON.stringify({ target: "Thailand", economics: MONEY }));
});

test("(e) operations still active: no PUT, the operations travel with the run", async () => {
  const c = base();
  const app = await mount(PlanStep, c, server(c));
  await openDrawer(app);
  await searchAgain(app);
  const w = writes(app);
  assert.deepEqual(w.map((x) => x.m), ["POST"]);
  assert.deepEqual(w[0].body.economics.operations, OPS);
});

test("(f) the user clears the last operating fields (no Mode Off dialog): same PUT then POST", async () => {
  const c = base({ economics: stored({ operations: { horizon_years: 4, groups: { motor_fan_assembly: { plants: { th: { ref: { name: "Chonburi works", country: "Thailand" }, labour_usd_per_year: 9 } } } } } }) });
  const app = await mount(PlanStep, c, server(c));
  await openDrawer(app);
  // clear every active operating field: the horizon and the one labour figure
  const horizon = app.find((e) => e.type === "input" && e.props.inputMode === "decimal" && String(e.props.value) === "4")[0] ?? app.find((e) => e.type === "input" && String(e.props.value) === "4")[0];
  assert.ok(horizon, "horizon field found");
  horizon.props.onChange({ target: { value: "" } });
  await app.flush();
  const labour = app.find((e) => e.type === "input" && /^9$/.test(String(e.props.value)))[0];
  assert.ok(labour, "labour field found");
  labour.props.onChange({ target: { value: "" } });
  await app.flush();
  await searchAgain(app);
  const w = writes(app);
  assert.deepEqual(w.map((x) => x.m), ["PUT", "POST"]);
  assert.equal("operations" in w[0].body, false);
  assert.equal("operations" in w[1].body.economics, false);
  assert.equal(app.state.current.economics.operations, undefined);
});

// The clear predicate is made TRUE first (stored ops active, local ops inactive
// after a confirmed Mode Off, annual 0, writable): the writable press really
// PUTs, but that PUT fails, so the discarded inputs are held on screen and the
// stored money stays active. Then the capability changes underneath (the shell
// re-reads the case: viewer, pinned, or a demo) and the SAME inputs are pressed
// again: no PUT may leave. A last writable press (PUT now succeeds) is the control.
test("(g) read-only, pinned and demo never PUT even with stored active / local inactive; the writable control does", async () => {
  const conditions = [
    ["viewer (can_write false)", { can_write: false }, null],
    ["pinned", { pinned: true }, null],
    ["pinned + viewer", { pinned: true, can_write: false }, null],
    ["demo case (demo store entry)", {}, "heat-gun-demo"],
  ];
  for (const [name, caps, demoKey] of conditions) {
    const c = base();
    const o = { putStatus: 500 };
    const app = await mount(PlanStep, c, server(c, o));
    try {
      await openDrawer(app);
      await modeOff(app);
      await searchAgain(app); // writable: the predicate holds, so a PUT is attempted (and fails)
      assert.equal(app.calls.filter(isPut).length, 1, `${name}: the predicate is live while writable`);
      assert.equal(app.calls.filter(isPlan).length, 0);
      assert.equal(sw(app).props.checked, false, `${name}: local operations are inactive`);
      // capability changes under the screen
      if (demoKey) globalThis.localStorage = { getItem: () => demoKey, setItem() {}, removeItem() {} };
      else {
        app.state.current = { ...app.state.current, ...caps };
        globalThis.window.dispatchEvent(new Event(ACCESS_STALE));
        await app.flush();
      }
      assert.equal(sw(app).props.checked, false, `${name}: the held inputs are still inactive`);
      assert.equal(app.state.current.economics.operations.horizon_years, 4, `${name}: stored operations are active`);
      if (caps.can_write === false && !caps.pinned) assert.equal(app.btn("Search again with these inputs").props.disabled, true, "the gate disables the button");
      await app.btn("Search again with these inputs").props.onClick(); // the handler itself must refuse to write
      await app.flush();
      assert.equal(app.calls.filter(isPut).length, 1, `${name}: no further PUT`);
      // (the unchanged legacy path still sends its plan POST and closes the drawer; only the save is refused)
      assert.equal(app.calls.filter(isPlan).length, 1, `${name}: the plan alone goes, with no operations key`);
      assert.equal("operations" in app.calls.filter(isPlan)[0].body.economics, false);
      // control: back to writable and the save works, the PUT is sent
      delete globalThis.localStorage;
      app.state.current = { ...app.state.current, can_write: true, pinned: false };
      globalThis.window.dispatchEvent(new Event(ACCESS_STALE));
      await app.flush();
      o.putStatus = undefined;
      await openDrawer(app);
      // a re-read reseeds the editor from the stored money: discard again, as a user would
      if (sw(app).props.checked) await modeOff(app);
      await app.btn("Search again with these inputs").props.onClick();
      await app.flush();
      assert.equal(app.calls.filter(isPut).length, 2, `${name}: writable control PUTs`);
      assert.equal(app.calls.filter(isPlan).length, 2, `${name}: and then plans after it`);
    } finally {
      delete globalThis.localStorage;
      app.unmount();
    }
  }
});

test("(h) the screen moves to another case while the PUT is in flight: nothing is applied, no plan POST", async () => {
  let release;
  const gate = new Promise((r) => (release = r));
  const c = base();
  const app = await mount(PlanStep, c, server(c, { gate }));
  await openDrawer(app);
  await modeOff(app);
  const pending = app.btn("Search again with these inputs").props.onClick();
  await app.flush();
  assert.equal(app.calls.filter(isPut).length, 1);
  // the shell now holds another case
  app.state.current = caseJson({ id: "c2", plan: OLD, economics: stored() });
  globalThis.window.dispatchEvent(new Event(ACCESS_STALE));
  await app.flush();
  release();
  await pending;
  await app.flush();
  assert.equal(app.calls.filter(isPlan).length, 0, "no plan for the case that was left");
  assert.equal(app.state.current.id, "c2");
  assert.equal(app.state.current.economics.operations.horizon_years, 4, "the other case is untouched");
});

test("(h2) unmounted while the PUT is in flight: no plan POST", async () => {
  let release;
  const gate = new Promise((r) => (release = r));
  const c = base();
  const app = await mount(PlanStep, c, server(c, { gate }));
  await openDrawer(app);
  await modeOff(app);
  const pending = app.btn("Search again with these inputs").props.onClick();
  await app.flush();
  app.unmount();
  release();
  await pending;
  assert.equal(app.calls.filter(isPlan).length, 0);
});

test("(h3) a second press while the PUT is in flight does nothing", async () => {
  let release;
  const gate = new Promise((r) => (release = r));
  const c = base();
  const app = await mount(PlanStep, c, server(c, { gate }));
  await openDrawer(app);
  await modeOff(app);
  const first = app.btn("Search again with these inputs").props.onClick();
  await app.flush();
  await app.btn("Search again with these inputs").props.onClick();
  release();
  await first;
  await app.flush();
  assert.equal(app.calls.filter(isPut).length, 1);
  assert.equal(app.calls.filter(isPlan).length, 1);
});

test("(i) duty provenance fields (note, table/live-MFN origins, Section 301 detail), notes and other money survive in the PUT body", async () => {
  const rich = stored({ duty_rates_note: "from the live table", duty_rates_live_mfn: ["Thailand"], group_costs: { pcba: { cn: 1, th: 2 } } });
  const c = base({ economics: rich });
  const app = await mount(PlanStep, c, server(c));
  await openDrawer(app);
  await modeOff(app);
  await searchAgain(app);
  const put = writes(app)[0];
  const want = { ...rich };
  delete want.operations;
  assert.deepEqual(put.body, want);
});

test("(j) the frozen browser case C2: PUT then plan POST, the result has no objective, stored operations gone", async () => {
  const c = caseJson({ plan: undefined, economics: { illustrative: true, annual_import_value_usd: 0, duty_rates: null, group_costs: null, operations: OPS } });
  const app = await mount(PlanStep, c, server(c));
  await app.btn("Search moves to").props.onClick();
  await app.flush();
  // the inline editor is where a user clears: use the Mode Off switch of the open fold
  app.unmount();
  const c2 = caseJson({ plan: OLD, economics: { illustrative: true, annual_import_value_usd: 0, operations: OPS } });
  const app2 = await mount(PlanStep, c2, server(c2));
  await openDrawer(app2);
  await modeOff(app2);
  await searchAgain(app2);
  const w = writes(app2);
  assert.deepEqual(w.map((x) => x.m), ["PUT", "POST"]);
  assert.equal("operations" in w[0].body, false);
  assert.equal("operations" in w[1].body.economics, false);
  assert.equal(app2.state.current.plan.objective, undefined);
  assert.equal(app2.state.current.economics.operations, undefined);
});
