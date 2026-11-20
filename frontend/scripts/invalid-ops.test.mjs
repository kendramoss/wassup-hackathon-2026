import "./render-support.mjs";
import assert from "node:assert/strict";
import { afterEach, test } from "node:test";
import { createElement as h } from "react";
import { deepMount } from "./lib/deep-mount.mjs";
import { json, stubNavigation } from "./lib/hook-runtime.mjs";

// An operating-cost entry that cannot be read is NOT a clear and NOT a missing
// value: the editor keeps the previous model value, keeps the raw text and its
// error on screen, and blocks every submission (plan search, money-drawer
// save) until it is corrected, intentionally emptied, or Mode Off is confirmed.
// Real component callbacks over a fetch spy; no browser, no network.
globalThis.window = new EventTarget();
globalThis.document = Object.assign(new EventTarget(), { body: { style: {} }, activeElement: null });
stubNavigation({ id: "c1", search: "target=Thailand" });
const F = await import("./lib/ops-fixtures.mjs");
const { planResult, caseJson, flatText } = F;
const { PlanStep } = await import("../src/components/op/screens/plan-step.tsx");
const { MoneyDrawer } = await import("../src/components/op/money-drawer.tsx");

const live = [];
afterEach(() => live.splice(0).forEach((a) => a.unmount()));
const mount = F.makeMount(live);

// the browser probe's saved shape: only one plant lead time
const OPS = { groups: { motor_fan_assembly: { plants: { th: { ref: { name: "Chonburi works", country: "Thailand" }, lead_time_days: 60 } } } } };
const MONEY = { illustrative: true, annual_import_value_usd: 0, duty_rates: { China: 0.417, Thailand: 0.167 } };
const isPlan = (x) => x.m === "POST" && x.u.endsWith("/plan");
const isPut = (x) => x.m === "PUT" && x.u.endsWith("/economics");
const server = () => (u, m, body, state) => {
  if (isPut({ m, u })) {
    state.current = { ...state.current, economics: body };
    return json({ case_id: "c1", economics: body });
  }
  if (isPlan({ m, u })) return json({ case_id: "c1", plan: planResult({ objective: undefined }) });
  return null;
};
const base = (extra = {}) => caseJson({ plan: planResult(), economics: { ...MONEY, operations: OPS }, ...extra });
const sw = (app) => app.find((e) => e.type === "input" && e.props.role === "switch")[0];
const openDrawer = async (app) => {
  app.find((e) => e.type === "button" && e.props["aria-haspopup"] === "dialog")[0].props.onClick();
  await app.flush();
};
const lead = (app) => F.fields(app, "Lead time (days)")[1];
const typeIn = async (app, v) => {
  lead(app).input.props.onChange({ target: { value: v } });
  await app.flush();
};
const press = async (app) => {
  await app.btn("Search again with these inputs").props.onClick();
  await app.flush();
};
const writes = (app) => app.calls.filter((x) => isPlan(x) || isPut(x));

const BAD = ["abc", "60.5", "6,0", "-60", "$-5", "1e3"];

test("saved ops (one lead time) + annual 0: invalid text is kept on screen, the section stays, nothing is sent and the saved ops are retained", async () => {
  for (const bad of BAD) {
    const app = await mount(PlanStep, base(), server());
    await openDrawer(app);
    assert.equal(lead(app).input.props.value, "60");
    await typeIn(app, bad);
    assert.equal(lead(app).input.props.value, bad, `${bad}: raw text stays`);
    assert.match(lead(app).errors.join(" "), /Not a number this field can read/);
    assert.equal(sw(app).props.checked, true, `${bad}: the section and the switch stay`);
    await press(app);
    assert.equal(writes(app).length, 0, `${bad}: no PUT and no plan POST`);
    assert.equal(app.state.current.economics.operations.groups.motor_fan_assembly.plants.th.lead_time_days, 60, "saved ops retained");
    assert.ok(app.btn("Search again with these inputs"), "drawer stays open");
    assert.match(flatText(app), /can't be read|cannot be read|can.t be read/i, "a blocking message is visible");
    app.unmount();
  }
});

test("correcting the invalid text to a valid number lets the run go with that value (no clear, no PUT: ops still active)", async () => {
  const app = await mount(PlanStep, base(), server());
  await openDrawer(app);
  await typeIn(app, "abc");
  await press(app);
  assert.equal(writes(app).length, 0);
  await typeIn(app, "45");
  await press(app);
  const w = writes(app);
  assert.deepEqual(w.map((x) => x.m), ["POST"]);
  assert.equal(w[0].body.economics.operations.groups.motor_fan_assembly.plants.th.lead_time_days, 45);
});

test("an intentionally emptied last value is a clear: PUT then plan POST (also whitespace)", async () => {
  for (const empty of ["", "   "]) {
    const app = await mount(PlanStep, base(), server());
    await openDrawer(app);
    await typeIn(app, "abc");
    await typeIn(app, empty);
    await press(app);
    const w = writes(app);
    assert.deepEqual(w.map((x) => x.m), ["PUT", "POST"], JSON.stringify(empty));
    assert.equal("operations" in w[0].body, false);
    app.unmount();
  }
});

test("confirmed Mode Off discards the invalid draft and clears the block: PUT then POST", async () => {
  const app = await mount(PlanStep, base(), server());
  await openDrawer(app);
  await typeIn(app, "abc");
  sw(app).props.onChange({ target: { checked: false } });
  await app.flush();
  await app.btn("Discard inputs").props.onClick();
  await app.flush();
  await press(app);
  assert.deepEqual(writes(app).map((x) => x.m), ["PUT", "POST"]);
  assert.equal("operations" in writes(app)[0].body, false);
});

test("declining the Mode Off confirm keeps the invalid draft and the block", async () => {
  const app = await mount(PlanStep, base(), server());
  await openDrawer(app);
  await typeIn(app, "abc");
  sw(app).props.onChange({ target: { checked: false } });
  await app.flush();
  await app.btn("Keep them").props.onClick();
  await app.flush();
  await press(app);
  assert.equal(writes(app).length, 0);
});

test("positive annual + an invalid field: the plan POST is blocked too (the run would whole-replace the economics)", async () => {
  const app = await mount(PlanStep, base({ economics: { ...MONEY, annual_import_value_usd: 2000000, operations: OPS } }), server());
  await openDrawer(app);
  await typeIn(app, "abc");
  await press(app);
  assert.equal(writes(app).length, 0);
  assert.equal(app.state.current.economics.operations.groups.motor_fan_assembly.plants.th.lead_time_days, 60);
});

test("the inline editor (no result yet) blocks Search the same way", async () => {
  const app = await mount(PlanStep, base({ plan: undefined }), server());
  lead(app).input.props.onChange({ target: { value: "abc" } });
  await app.flush();
  assert.equal(sw(app).props.checked, true);
  await app.btn("Search moves to").props.onClick();
  await app.flush();
  assert.equal(writes(app).length, 0);
  lead(app).input.props.onChange({ target: { value: "61" } });
  await app.flush();
  await app.btn("Search moves to").props.onClick();
  await app.flush();
  assert.deepEqual(writes(app).map((x) => x.m), ["POST"]);
});

test("a legacy case without operations is not blocked by anything", async () => {
  const app = await mount(PlanStep, base({ economics: MONEY }), server());
  await openDrawer(app);
  await press(app);
  assert.deepEqual(writes(app).map((x) => x.m), ["POST"]);
});

// ---- MoneyDrawer (scenarios / new case) -------------------------------------------------

async function drawer(value) {
  const saved = [];
  const app = deepMount(h(MoneyDrawer, { value, product: F.product, countries: ["China", "Thailand"], onSave: (e) => saved.push(e), onClose() {} }));
  live.push(app);
  app.refresh();
  await app.flush();
  return Object.assign(app, { saved });
}

test("MoneyDrawer: an invalid operating field blocks Save (no onSave), keeps the section; a valid or emptied field saves", async () => {
  const app = await drawer({ ...MONEY, operations: OPS });
  const f = () => F.fields(app, "Lead time (days)")[1];
  f().input.props.onChange({ target: { value: "abc" } });
  await app.flush();
  assert.equal(f().input.props.value, "abc");
  await app.btn("Save money inputs").props.onClick();
  await app.flush();
  assert.equal(app.saved.length, 0, "nothing saved");
  assert.match(app.all().map((e) => String(e.props?.children ?? "")).join(" "), /can't be read/i);
  f().input.props.onChange({ target: { value: "50" } });
  await app.flush();
  await app.btn("Save money inputs").props.onClick();
  await app.flush();
  assert.equal(app.saved.length, 1);
  assert.equal(app.saved[0].operations.groups.motor_fan_assembly.plants.th.lead_time_days, 50);
  // emptied on purpose: saved without operations
  f().input.props.onChange({ target: { value: "abc" } });
  await app.flush();
  f().input.props.onChange({ target: { value: "" } });
  await app.flush();
  await app.btn("Save money inputs").props.onClick();
  await app.flush();
  assert.equal(app.saved.length, 2);
  assert.equal("operations" in app.saved[1], false);
});

test("MoneyDrawer: confirmed Mode Off with an invalid draft saves the legacy economics", async () => {
  const app = await drawer({ ...MONEY, operations: OPS });
  F.fields(app, "Lead time (days)")[1].input.props.onChange({ target: { value: "abc" } });
  await app.flush();
  app.find((e) => e.type === "input" && e.props.role === "switch")[0].props.onChange({ target: { checked: false } });
  await app.flush();
  await app.btn("Discard inputs").props.onClick();
  await app.flush();
  await app.btn("Save money inputs").props.onClick();
  await app.flush();
  assert.equal(app.saved.length, 1);
  assert.equal("operations" in app.saved[0], false);
});

test("native-locale valid values are kept; foreign or signed text blocks rather than dropping", async () => {
  const app = await drawer({ ...MONEY, operations: OPS });
  const f = () => F.fields(app, "Lead time (days)")[1];
  for (const ok of ["60", "1,234", "7"]) {
    f().input.props.onChange({ target: { value: ok } });
    await app.flush();
    assert.equal(f().errors?.length ?? 0, 0, ok);
    await app.btn("Save money inputs").props.onClick();
    await app.flush();
  }
  assert.equal(app.saved.length, 3);
  assert.equal(app.saved[1].operations.groups.motor_fan_assembly.plants.th.lead_time_days, 1234);
  for (const bad of ["1.234,5", "-7", "６０"]) {
    f().input.props.onChange({ target: { value: bad } });
    await app.flush();
    await app.btn("Save money inputs").props.onClick();
    await app.flush();
  }
  assert.equal(app.saved.length, 3, "none of the foreign/signed entries saved");
});

test("two editors on one validity: a valid edit in one never clears the other's invalid flag; fixing or discarding the offender does", async () => {
  const { useOpsValidity, OperationalFields } = await import("../src/components/op/operational-fields.tsx");
  const { useState } = await import("react");
  const probe = {};
  function Two() {
    const v = useOpsValidity();
    probe.v = v;
    const [a, setA] = useState({ ...MONEY, operations: OPS });
    const [b, setB] = useState({ ...MONEY, operations: OPS });
    return h("div", null, h(OperationalFields, { value: a, onChange: setA, product: F.product, validity: v }), h(OperationalFields, { value: b, onChange: setB, product: F.product, validity: v }));
  }
  const app = deepMount(h(Two));
  live.push(app);
  app.refresh();
  await app.flush();
  const lead = (n) => F.fields(app, "Lead time (days)")[n];
  const per = F.fields(app, "Lead time (days)").length / 2;
  assert.ok(per >= 2);
  lead(1).input.props.onChange({ target: { value: "abc" } }); // editor A, th
  await app.flush();
  assert.equal(probe.v.get(), true);
  lead(per + 1).input.props.onChange({ target: { value: "61" } }); // editor B valid edit
  await app.flush();
  assert.equal(probe.v.get(), true, "B being valid does not hide A's invalid entry");
  lead(per + 1).input.props.onChange({ target: { value: "bad" } });
  await app.flush();
  lead(per + 1).input.props.onChange({ target: { value: "" } }); // B emptied on purpose
  await app.flush();
  assert.equal(probe.v.get(), true, "still A's");
  lead(1).input.props.onChange({ target: { value: "62" } });
  await app.flush();
  assert.equal(probe.v.get(), false, "fixing A clears the flag");
  // an external reset of the value (restore, discard) drops the draft and the flag
  lead(1).input.props.onChange({ target: { value: "x" } });
  await app.flush();
  assert.equal(probe.v.get(), true);
  app.find((e) => e.type === "input" && e.props.role === "switch")[0].props.onChange({ target: { checked: false } });
  await app.flush();
  await app.btn("Discard inputs").props.onClick();
  await app.flush();
  assert.equal(probe.v.get(), false, "confirmed Mode Off clears the flag");
});
