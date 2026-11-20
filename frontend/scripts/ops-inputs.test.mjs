import "./render-support.mjs";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { afterEach, test } from "node:test";
import { createElement as h } from "react";
import { fileURLToPath } from "node:url";
import { deepMount, textOf } from "./lib/deep-mount.mjs";
import { json, stubNavigation } from "./lib/hook-runtime.mjs";

// #281 inputs: the operating-cost fields in the real money drawer, the client
// that sends them, and the stale-flag merge. Real component callbacks
// (onChange/onClick) over a fetch spy; no browser, no network, no real backend.
globalThis.window = new EventTarget();
globalThis.document = Object.assign(new EventTarget(), { body: { style: {} }, activeElement: null });
stubNavigation({ id: "c1", search: "" });
const F = await import("./lib/ops-fixtures.mjs");
const { MoneyDrawer, hasAnyMoney, moneySummary } = await import("../src/components/op/money-drawer.tsx");
const O = await import("../src/lib/ops.ts");
const C = await import("../src/lib/api/client.ts");
const { prefillMoney, mergeDutyPrefill, mergeEstimates } = await import("../src/components/op/money-prefill.tsx");
const { ScenariosView } = await import("../src/components/op/screens/scenarios-view.tsx");
const { PlanStep } = await import("../src/components/op/screens/plan-step.tsx");

const live = [];
afterEach(() => live.splice(0).forEach((a) => a.unmount()));
const mount = F.makeMount(live);
const product = F.product;

const legacy = () => ({ illustrative: true, annual_import_value_usd: 2000000, duty_rates: { China: 0.417, Thailand: 0.167 }, group_costs: { motor_fan_assembly: { cn: 300000, th: 340000 } } });

async function drawer(value, extra = {}) {
  const saved = [];
  const app = deepMount(h(MoneyDrawer, { value, product, countries: ["China", "Thailand"], onSave: (e) => saved.push(e), onClose() {}, ...extra }));
  live.push(app);
  app.refresh();
  await app.flush();
  return Object.assign(app, { saved });
}
const inputOf = (app, label) => F.fields(app, label).map((f) => f.input);
const type = async (app, input, v) => {
  input.props.onChange({ target: { value: v } });
  await app.flush();
};
const save = async (app) => {
  await app.btn("Save money inputs").props.onClick();
  await app.flush();
  return app.saved.at(-1);
};
const turnOn = async (app) => {
  app.find((e) => e.type === "input" && e.props.role === "switch")[0].props.onChange({ target: { checked: true } });
  await app.flush();
};
const text = (app) => app.all().map((e) => textOf(e.props?.children)).join(" ¦ ");

// ---- helpers -----------------------------------------------------------------

test("parseAmount: blank is not entered, 0 is a known zero, money punctuation is ignored, garbage is refused", () => {
  assert.deepEqual(O.parseAmount(""), { ok: true, value: undefined });
  assert.deepEqual(O.parseAmount("  "), { ok: true, value: undefined });
  assert.deepEqual(O.parseAmount("0"), { ok: true, value: 0 });
  assert.deepEqual(O.parseAmount("0.0"), { ok: true, value: 0 });
  assert.deepEqual(O.parseAmount("$ 1,234.5"), { ok: true, value: 1234.5 });
  assert.deepEqual(O.parseAmount("12."), { ok: true, value: 12 });
  for (const bad of ["12a", "1.2.3", "-5", "1e3", "NaN", "Infinity"]) assert.equal(O.parseAmount(bad).ok, false, bad);
});

test("economicsOnWire leaves out a block with nothing entered (note, ref, sources only) and keeps an entered one, same object", () => {
  const base = { annual_import_value_usd: 1 };
  for (const ops of [{}, { note: "n" }, { groups: {} }, { groups: { g: { plants: { p: { ref: { name: "a", country: "b" }, sources: { labour: "quote" } } } } } }]) {
    const out = C.economicsOnWire({ ...base, operations: ops });
    assert.equal("operations" in out, false, JSON.stringify(ops));
  }
  const zero = { ...base, operations: { groups: { g: { plants: { p: { labour_usd_per_year: 0 } } } } } };
  assert.equal(C.economicsOnWire(zero), zero, "an explicit 0 is entered");
  const noOps = { ...base };
  assert.equal(C.economicsOnWire(noOps), noOps);
  assert.equal(C.economicsOnWire(undefined), undefined);
});

test("setPlant binds a NEW entry to the product plant, never rebinds an old one; clearing prunes to absent", () => {
  const th = product.plants[1];
  let e = O.setPlant({}, "motor_fan_assembly", th, "labour_usd_per_year", 0);
  assert.deepEqual(e.operations.groups.motor_fan_assembly.plants.th, { labour_usd_per_year: 0, ref: { name: "Chonburi works", country: "Thailand" } });
  const old = { operations: { groups: { motor_fan_assembly: { plants: { th: { ref: { name: "Old", country: "Vietnam" }, labour_usd_per_year: 5, sources: { labour: "quote" } } } } } } };
  const e2 = O.setPlant(old, "motor_fan_assembly", th, "logistics_usd_per_year", 7);
  assert.deepEqual(e2.operations.groups.motor_fan_assembly.plants.th.ref, { name: "Old", country: "Vietnam" });
  assert.equal(e2.operations.groups.motor_fan_assembly.plants.th.sources.labour, "quote", "provenance kept");
  assert.equal(O.refMismatch(e2.operations.groups.motor_fan_assembly.plants.th, th), true);
  e = O.setPlant(e, "motor_fan_assembly", th, "labour_usd_per_year", undefined);
  assert.equal("operations" in e, false, "nothing left: the key is gone");
});

test("server error text: operations slugs, path placement, outer decode error, several problems", () => {
  const m = "invalid input: economics: operations.cost_basis_required: groups.motor_fan_assembly.cost_basis: required where labour is entered; economics: operations.value_out_of_range: groups.motor_fan_assembly.plants.th.labour_usd_per_year: must be 0 to 1e12; economics: operations.too_large: operations block is larger than 64 KiB; request body: json: unknown field \"x\"";
  const issues = O.parseOpsIssues(m);
  assert.deepEqual(issues.map((i) => i.slug), ["cost_basis_required", "value_out_of_range", "too_large"]);
  const { byKey, general } = O.issuesByField(m);
  assert.deepEqual(byKey["g|motor_fan_assembly|cost_basis"], ["required where labour is entered"]);
  assert.deepEqual(byKey["p|motor_fan_assembly|th|labour_usd_per_year"], ["must be 0 to 1e12"]);
  assert.equal(general.length, 1);
  assert.match(general[0].text, /too_large/);
  assert.deepEqual(O.parseOpsIssues("economics: duty rate for X must be between 0 and 1"), []);
  assert.equal(O.issueKey("horizon_years"), "horizon_years");
  assert.equal(O.issueKey("operations.max_lead_time_days"), "max_lead_time_days");
});

// ---- the money drawer -------------------------------------------------------------

test("legacy drawer: Save sends exactly the legacy economics, no operations key, nothing defaulted", async () => {
  const before = JSON.stringify(legacy());
  const app = await drawer(legacy());
  assert.match(text(app), /Operating costs · optional/);
  assert.match(text(app), /Off: moves are ranked on duty saved and the group costs above, as before/);
  const out = await save(app);
  assert.equal("operations" in out, false);
  assert.equal(JSON.stringify(out), before, "byte-identical to the legacy economics");
});

test("turning the section on writes nothing: no defaults, no zeros, every operating field starts blank", async () => {
  const app = await drawer(legacy());
  await turnOn(app);
  for (const l of ["Horizon", "Your lead-time limit", "Annual demand", "Orders a year", "Labour (USD", "Logistics (USD", "CapEx", "Retooling", "Lead time (days)", "Minimum order (units)", "Unit", "Minimum-order unit"]) {
    const inputs = inputOf(app, l);
    assert.ok(inputs.length > 0, l);
    for (const i of inputs) assert.equal(i.props.value, "", `${l} is blank`);
  }
  assert.equal(app.find((e) => e.type === "select")[0].props.value, "");
  const out = await save(app);
  assert.equal("operations" in out, false, "on but nothing entered is the legacy request");
  assert.equal(JSON.stringify(out), JSON.stringify(legacy()));
});

test("entering costs: a typed 0 is kept as a known zero, a blank is absent, refs bind to the product plant, units keep their case", async () => {
  const app = await drawer(legacy());
  await turnOn(app);
  const labour = inputOf(app, "Labour (USD"); // motor_fan_assembly at cn, th, then pcba at cn, th
  await type(app, labour[1], "0");
  await type(app, inputOf(app, "Logistics (USD")[1], "$ 35,000");
  await type(app, inputOf(app, "CapEx")[1], "900,000");
  await type(app, inputOf(app, "Retooling")[1], "100000");
  await type(app, inputOf(app, "Lead time (days)")[1], "120");
  await type(app, inputOf(app, "Minimum order (units)")[1], "500");
  await type(app, inputOf(app, "Minimum-order unit")[1], "PCS");
  await type(app, inputOf(app, "Horizon")[0], "4");
  await type(app, inputOf(app, "Your lead-time limit")[0], "90");
  await type(app, inputOf(app, "Unit")[0], "pcs");
  await type(app, inputOf(app, "Annual demand")[0], "1200000");
  await type(app, inputOf(app, "Orders a year")[0], "12");
  app.find((e) => e.type === "select")[0].props.onChange({ target: { value: "additional" } });
  await app.flush();
  const out = await save(app);
  const ops = out.operations;
  assert.equal(ops.horizon_years, 4);
  assert.equal(ops.max_lead_time_days, 90);
  const g = ops.groups.motor_fan_assembly;
  assert.equal(g.cost_basis, "additional");
  assert.equal(g.unit, "pcs");
  assert.equal(g.annual_demand_units, 1200000);
  assert.equal(g.orders_per_year, 12);
  assert.deepEqual(g.plants.th, {
    labour_usd_per_year: 0, logistics_usd_per_year: 35000, capex_usd: 900000, retooling_usd: 100000, lead_time_days: 120, moq_units: 500, moq_unit: "PCS",
    ref: { name: "Chonburi works", country: "Thailand" },
  });
  assert.equal(g.plants.cn, undefined, "the untouched plant has no entry, not a zero");
  assert.equal(out.group_costs.motor_fan_assembly.th, 340000, "legacy group costs untouched");
  // clearing a field removes the key
  await type(app, inputOf(app, "Labour (USD")[1], "");
  const out2 = await save(app);
  assert.equal("labour_usd_per_year" in out2.operations.groups.motor_fan_assembly.plants.th, false);
});

test("numeric fields refuse non-numbers and whole-day fields refuse decimals (the typed text is not taken)", async () => {
  const app = await drawer(legacy());
  await turnOn(app);
  await type(app, inputOf(app, "Labour (USD")[1], "12a");
  assert.equal(inputOf(app, "Labour (USD")[1].props.value, "");
  await type(app, inputOf(app, "Lead time (days)")[1], "1.5");
  assert.equal(inputOf(app, "Lead time (days)")[1].props.value, "");
  await type(app, inputOf(app, "Your lead-time limit")[0], "90.5");
  assert.equal(inputOf(app, "Your lead-time limit")[0].props.value, "");
  await type(app, inputOf(app, "Horizon")[0], "2.5"); // years may be fractional
  assert.equal(inputOf(app, "Horizon")[0].props.value, "2.5");
  const out = await save(app);
  assert.deepEqual(out.operations, { horizon_years: 2.5 });
});

test("Mode Off removes the operations key entirely and the saved economics are the legacy ones", async () => {
  const withOps = { ...legacy(), operations: { horizon_years: 3, note: "keep?", groups: { motor_fan_assembly: { cost_basis: "included", plants: { th: { labour_usd_per_year: 9, ref: { name: "Chonburi works", country: "Thailand" } } } } } } };
  const app = await drawer(withOps);
  const sw = app.find((e) => e.type === "input" && e.props.role === "switch")[0];
  assert.equal(sw.props.checked, true);
  sw.props.onChange({ target: { checked: false } });
  await app.flush();
  const out = await save(app);
  assert.equal("operations" in out, false);
  assert.equal(JSON.stringify(out), JSON.stringify(legacy()), "byte-identical to the legacy request");
});

test("editing a legacy field never drops the operations block, its note, sources or provenance", async () => {
  const ops = { horizon_years: 4, note: "from the quote of Oct", groups: { motor_fan_assembly: { cost_basis: "additional", unit: "pcs", plants: { th: { labour_usd_per_year: 1, ref: { name: "Chonburi works", country: "Thailand" }, sources: { labour: "supplier quote 12" } } } } } };
  const app = await drawer({ ...legacy(), operations: ops });
  await type(app, app.find((e) => e.type === "input" && e.props.inputMode === "numeric" && /\$2,000,000/.test(String(e.props.value)))[0], "$3,000,000");
  const out = await save(app);
  assert.equal(out.annual_import_value_usd, 3000000);
  assert.deepEqual(out.operations, ops);
});

test("an AI cost estimate or a duty lookup landing on the economics keeps operations, and neither request carries them", async () => {
  const ops = { horizon_years: 2, groups: { pcba: { plants: { th: { capex_usd: 5, ref: { name: "Chonburi works", country: "Thailand" } } } } } };
  const e = { illustrative: true, duty_rates: {}, operations: ops };
  assert.deepEqual(mergeEstimates(e, e, [{ group: "pcba", plant: "th", usd: 100, low: 90, high: 110, reason: "x" }]).economics.operations, ops);
  assert.deepEqual(mergeDutyPrefill(e, e, { duty_rates: { China: 0.1 } }).operations, ops);
  const sent = [];
  globalThis.fetch = async (u, init) => {
    sent.push({ u: String(u), body: JSON.parse(init.body) });
    return json({ economics: { illustrative: true, duty_rates: { China: 0.1 } }, duty_sources: { China: "table" }, hts: "8516.79.00", estimates: [] });
  };
  const r = await prefillMoney(product, e);
  assert.equal(sent[0].body.economics.operations, undefined, "money-prefill never receives the operating-cost block");
  assert.deepEqual(r.economics.operations, ops, "and the merged economics keep it");
  await C.api.costEstimate({ product, economics: e });
  assert.equal(sent[1].body.economics.operations, undefined);
});

test("a late AI estimate or duty lookup lands on the operating costs typed meanwhile without touching them", () => {
  const started = { illustrative: true, duty_rates: {} };
  const typed = { ...started, operations: { horizon_years: 4, groups: { pcba: { plants: { th: { labour_usd_per_year: 0, ref: { name: "Chonburi works", country: "Thailand" } } } } } } };
  const est = mergeEstimates(typed, started, [{ group: "pcba", plant: "th", usd: 100, low: 90, high: 110, reason: "x" }]).economics;
  assert.deepEqual(est.operations, typed.operations);
  assert.equal(est.group_costs.pcba.th, 100);
  const duty = mergeDutyPrefill(typed, started, { duty_rates: { China: 0.1 } });
  assert.deepEqual(duty.operations, typed.operations);
  assert.equal(duty.duty_rates.China, 0.1);
});

test("existing plant entries for another plant are flagged and never rebound; removing one lets the plant be entered afresh", async () => {
  const old = { ...legacy(), operations: { horizon_years: 4, groups: { motor_fan_assembly: { plants: { th: { labour_usd_per_year: 5, ref: { name: "Rayong plant", country: "Thailand" } } } } } } };
  const app = await drawer(old);
  assert.match(text(app), /Entered for Rayong plant \(Thailand\); this plant is now Chonburi works \(Thailand\)/);
  await type(app, inputOf(app, "Logistics (USD")[1], "10");
  let out = await save(app);
  assert.deepEqual(out.operations.groups.motor_fan_assembly.plants.th.ref, { name: "Rayong plant", country: "Thailand" }, "editing does not silently rebind");
  await app.btn("Remove this entry").props.onClick();
  await app.flush();
  assert.doesNotMatch(text(app), /Entered for Rayong plant/);
  await type(app, inputOf(app, "Logistics (USD")[1], "10");
  out = await save(app);
  assert.deepEqual(out.operations.groups.motor_fan_assembly.plants.th, { logistics_usd_per_year: 10, ref: { name: "Chonburi works", country: "Thailand" } });
});

test("baseline capital is called sunk, target capital is a one-time cost, and the horizon/basis/unit hints show without blocking Save", async () => {
  const app = await drawer(legacy());
  await turnOn(app);
  const t = text(app);
  assert.match(t, /This is the plant today\. Capital already spent is sunk: it is ignored and never refunded\./);
  assert.match(t, /One-time cost to set up this plant\. Only counted if a move goes here\./);
  assert.match(t, /Already inside: the group cost above already contains them/);
  assert.match(t, /Case-sensitive, and no conversion is done: pcs and PCS are different units/);
  assert.match(t, /lead times are assumed to run in parallel/);
  await type(app, inputOf(app, "CapEx")[1], "5");
  await type(app, inputOf(app, "Labour (USD")[1], "5");
  await type(app, inputOf(app, "Minimum order (units)")[1], "5");
  const t2 = text(app);
  assert.match(t2, /A horizon above 0 is needed because a one-time cost is entered\./);
  assert.match(t2, /Choose whether labour and logistics are inside or on top of the group cost\./);
  assert.match(t2, /Add the unit of the minimum order\./);
  assert.ok(await save(app), "hints never block: the server decides");
});

test("a capex entered on the plant the group is at today needs no horizon (sunk), only a target does", () => {
  const sunk = { groups: { motor_fan_assembly: { plants: { cn: { capex_usd: 5 } } } } };
  assert.equal(O.oneTimeEntered(sunk, product), false);
  assert.equal(O.oneTimeEntered({ groups: { motor_fan_assembly: { plants: { th: { capex_usd: 5 } } } } }, product), true);
});

test("viewers (readOnly) see the operating-cost inputs, disabled", async () => {
  const withOps = { ...legacy(), operations: { horizon_years: 3 } };
  const app = await drawer(withOps, { readOnly: true });
  const inputs = [...inputOf(app, "Horizon"), ...inputOf(app, "Labour (USD"), ...inputOf(app, "Unit")];
  assert.ok(inputs.length > 3);
  for (const i of inputs) assert.equal(i.props.disabled, true);
  assert.equal(app.find((e) => e.type === "input" && e.props.role === "switch")[0].props.disabled, true);
  assert.equal(inputOf(app, "Horizon")[0].props.value, "3");
});

test("server messages land beside the named field; unplaced ones and unknown_field/too_large are listed whole", async () => {
  const msg = "economics: operations.horizon_too_small: horizon_years: must be above 0; economics: operations.value_out_of_range: groups.motor_fan_assembly.plants.th.labour_usd_per_year: negative; economics: operations.unknown_field: operations: unknown field \"cost_bais\"; economics: operations.too_large: the operations block is larger than 64 KiB";
  const app = await drawer({ ...legacy(), operations: { horizon_years: 0 } }, { serverError: msg });
  assert.deepEqual(F.fields(app, "Horizon")[0].errors, ["must be above 0"]);
  const labour = F.fields(app, "Labour (USD")[1];
  assert.equal(labour.input.props["aria-invalid"], true);
  assert.deepEqual(labour.errors, ["negative"]);
  assert.equal(F.fields(app, "Labour (USD")[0].errors, undefined, "the other plant has no message");
  const t = text(app);
  assert.match(t, /Operating-cost inputs to fix/);
  assert.match(t, /operations\.unknown_field: operations: unknown field "cost_bais"/);
  assert.match(t, /operations\.too_large: the operations block is larger than 64 KiB/);
});

test("summary and has-any-money know about operating costs; a legacy economics summary is unchanged", () => {
  assert.equal(moneySummary(legacy()), "$2M imports a year · China 41.7%, Thailand 16.7% · 1 group costed");
  assert.equal(hasAnyMoney({ operations: { horizon_years: 3 } }), true, "operations alone travel with a new case");
  assert.equal(hasAnyMoney({ operations: { note: "x" } }), false);
  assert.match(moneySummary({ ...legacy(), operations: { horizon_years: 3 } }), /operating costs entered$/);
});

// ---- the screens' real callbacks -------------------------------------------------

test("scenarios: a 400 on the operating costs reopens the drawer with what was typed and the message beside its field", async () => {
  const msg = "economics: operations.horizon_required: horizon_years: a horizon is required when a one-time cost applies";
  const c = F.caseJson({ step: "origin" });
  let puts = 0;
  const app = await mount(ScenariosView, c, (u, m) => (m === "PUT" && u.endsWith("/economics") ? (puts++, json({ code: "invalid_input", message: msg }, 400)) : null));
  await app.btn("Edit inputs").props.onClick();
  await app.flush();
  app.find((e) => e.type === "input" && e.props.role === "switch")[0].props.onChange({ target: { checked: true } });
  await app.flush();
  const field = (i) => F.fields(app, "Labour (USD")[i].input;
  field(1).props.onChange({ target: { value: "1500" } });
  await app.flush();
  await app.btn("Save money inputs").props.onClick();
  await app.flush();
  assert.equal(puts, 1);
  const sent = JSON.parse(app.calls.find((x) => x.m === "PUT").raw);
  assert.equal(sent.operations.groups.motor_fan_assembly.plants.th.labour_usd_per_year, 1500);
  // the drawer is back, with the typed value and the message tied to the horizon field
  assert.equal(field(1).props.value, "1500");
  assert.deepEqual(F.fields(app, "Horizon")[0].errors, ["a horizon is required when a one-time cost applies"]);
  assert.match(text(app), /economics: operations\.horizon_required/, "the whole message is also in the error box");
});

test("scenarios: a non-operations save failure keeps the existing behaviour (drawer closed, Try again, no inline errors)", async () => {
  const app = await mount(ScenariosView, F.caseJson({ step: "origin" }), (u, m) => (m === "PUT" && u.endsWith("/economics") ? json({ code: "internal", message: "down" }, 500) : null));
  await app.btn("Edit inputs").props.onClick();
  await app.flush();
  await app.btn("Save money inputs").props.onClick();
  await app.flush();
  assert.equal(app.btn("Save money inputs"), undefined, "drawer closed");
  assert.ok(app.btn("Try again"));
});

test("plan: an untouched run sends the legacy economics byte for byte; operating costs entered in the editor go with the run, nothing else added", async () => {
  const c = F.caseJson({ plan: undefined });
  const serve = (u, m) => (m === "POST" && u.endsWith("/plan") ? json({ case_id: "c1", plan: F.planResult() }) : null);
  const app = await mount(PlanStep, c, serve);
  await app.btn("Search moves to").props.onClick();
  await app.flush();
  assert.equal(JSON.stringify(JSON.parse(app.calls.find((x) => x.m === "POST" && x.u.endsWith("/plan")).raw).economics), JSON.stringify(c.economics), "untouched: the legacy request");
  const app2 = await mount(PlanStep, c, serve);
  app2.find((e) => e.type === "input" && e.props.role === "switch")[0].props.onChange({ target: { checked: true } });
  await app2.flush();
  F.fields(app2, "Horizon")[0].input.props.onChange({ target: { value: "5" } });
  await app2.flush();
  await app2.btn("Search moves to").props.onClick();
  await app2.flush();
  const body = JSON.parse(app2.calls.find((x) => x.m === "POST" && x.u.endsWith("/plan")).raw);
  assert.deepEqual(body.economics.operations, { horizon_years: 5 });
  assert.equal(body.economics.annual_import_value_usd, c.economics.annual_import_value_usd);
});

test("plan: no positive annual value is needed to enter or send operating costs (v4.2: the form's own economics travel)", async () => {
  const c = F.caseJson({ plan: undefined, economics: { illustrative: true } });
  const app = await mount(PlanStep, c, (u, m) => (m === "POST" && u.endsWith("/plan") ? json({ case_id: "c1", plan: F.planResult() }) : null));
  app.find((e) => e.type === "input" && e.props.role === "switch")[0].props.onChange({ target: { checked: true } });
  await app.flush();
  F.fields(app, "Your lead-time limit")[0].input.props.onChange({ target: { value: "90" } });
  await app.flush();
  const run = app.btn("Search moves to");
  assert.notEqual(run.props.disabled, true);
  await run.props.onClick();
  await app.flush();
  const body = JSON.parse(app.calls.find((x) => x.m === "POST" && x.u.endsWith("/plan")).raw);
  assert.deepEqual(body.economics, { illustrative: true, operations: { max_lead_time_days: 90 } });
});

// ---- client: requests and stale flags ---------------------------------------------------

async function sent(fn) {
  const calls = [];
  globalThis.fetch = async (u, init = {}) => {
    calls.push({ u: String(u), m: init.method, raw: init.body });
    return json({});
  };
  await fn();
  return calls;
}

test("requests: an inactive operations block never travels; legacy money bodies are byte-identical", async () => {
  const legacyE = legacy();
  const noteOnly = { ...legacyE, operations: { note: "n" } };
  const active = { ...legacyE, operations: { horizon_years: 3 } };
  const calls = await sent(async () => {
    await C.api.putEconomics("c1", legacyE);
    await C.api.putEconomics("c1", noteOnly);
    await C.api.putEconomics("c1", active);
    await C.api.plan("c1", { target: "Thailand", economics: noteOnly });
    await C.api.scenarios("c1", { scenarios: [], economics: active });
    await C.api.predict({ facts: "x", economics: noteOnly });
    await C.api.planEstimate("c1", { target: "Thailand", economics: noteOnly });
  });
  assert.equal(calls[0].raw, JSON.stringify(legacyE));
  assert.equal(calls[1].raw, JSON.stringify(legacyE), "note-only is left out");
  assert.equal(JSON.parse(calls[2].raw).operations.horizon_years, 3);
  assert.equal(JSON.parse(calls[3].raw).economics.operations, undefined);
  assert.equal(JSON.parse(calls[4].raw).economics.operations.horizon_years, 3);
  assert.equal(JSON.parse(calls[5].raw).economics.operations, undefined);
  assert.equal(JSON.parse(calls[6].raw).economics.operations, undefined);
});

test("streamPlan sends the operations of the run and a legacy run is the legacy body", async () => {
  const bodies = [];
  globalThis.fetch = async (u, init) => (bodies.push(init.body), json({ plan: {} }));
  await C.streamPlan("c1", { target: "Thailand", economics: legacy() }, () => {});
  await C.streamPlan("c1", { target: "Thailand", economics: { ...legacy(), operations: { horizon_years: 2 } } }, () => {});
  assert.equal(bodies[0], JSON.stringify({ target: "Thailand", economics: legacy() }));
  assert.equal(JSON.parse(bodies[1]).economics.operations.horizon_years, 2);
});

const full = (extra = {}) => ({ id: "c1", display_name: "Heat gun", pinned: false, product, prediction: { origin_country: "China" }, scenarios: [{ label: "A", product, prediction: { origin_country: "Thailand" } }], economics: legacy(), ...extra });
const putReply = (extra = {}) => ({ case_id: "c1", economics: legacy(), ...extra });

test("mergeEconomics: a true flag in the response is set; a MISSING flag clears a cached true (restore, mode off, new plan, bom change)", () => {
  const stale = full({ plan_economics_stale: true, targets_economics_stale: true });
  const keep = C.mergeEconomics(stale, putReply({ plan_economics_stale: true, targets_economics_stale: true }));
  assert.equal(keep.plan_economics_stale, true);
  assert.equal(keep.targets_economics_stale, true);
  // the money is restored to the original basis: the server omits both keys
  const restored = C.mergeEconomics(stale, putReply());
  assert.equal("plan_economics_stale" in restored, false);
  assert.equal("targets_economics_stale" in restored, false);
  // Mode Off: no operations in the stored economics, no flags
  const off = C.mergeEconomics(full({ economics: { ...legacy(), operations: { horizon_years: 1 } }, plan_economics_stale: true }), putReply());
  assert.equal("operations" in off.economics, false);
  assert.equal("plan_economics_stale" in off, false);
  // only the plan is stale: the other cached flag still clears
  const one = C.mergeEconomics(stale, putReply({ plan_economics_stale: true }));
  assert.equal(one.plan_economics_stale, true);
  assert.equal("targets_economics_stale" in one, false);
  // a new plan / a bom change that clears the plan: a full-case response without the keys replaces the cache
  assert.equal("plan_economics_stale" in C.withStaleFlags(stale, {}), false);
  assert.equal("plan_economics_stale" in C.withStaleFlags(stale, { plan_economics_stale: undefined }), false);
});

test("mergeEconomics: the response's repriced scenarios replace the case's; without them the scenario definitions stay untouched", () => {
  const priced = [{ label: "A", product, prediction: { origin_country: "Thailand" }, comparison: { net_usd: 7 } }];
  assert.equal(C.mergeEconomics(full(), putReply({ scenarios: priced })).scenarios[0].comparison.net_usd, 7);
  const c = full();
  assert.equal(C.mergeEconomics(c, putReply()).scenarios, c.scenarios);
});

function io(start) {
  const s = { state: start };
  return { s, io: { latest: () => s.state, apply: (c) => (s.state = c) } };
}

test("saveEconomics, normal PUT + GET: the re-read case (flags as the server says) replaces the cache, stale clears and sets", async () => {
  for (const [getFlags, expect] of [[{}, false], [{ plan_economics_stale: true }, true]]) {
    const { s, io: x } = io(full({ plan_economics_stale: true, targets_economics_stale: true }));
    globalThis.fetch = async (u, init) => json(init.method === "PUT" ? putReply() : full(getFlags));
    await C.saveEconomics("c1", legacy(), x);
    assert.equal(Boolean(s.state.plan_economics_stale), expect);
    assert.equal("targets_economics_stale" in s.state, false);
  }
});

test("saveEconomics, forced GET failure: the fallback merge clears or keeps the cached flags from the PUT response only", async () => {
  for (const [reply, plan, targets] of [[putReply(), false, false], [putReply({ plan_economics_stale: true }), true, false], [putReply({ plan_economics_stale: true, targets_economics_stale: true }), true, true]]) {
    const { s, io: x } = io(full({ plan_economics_stale: true, targets_economics_stale: true }));
    globalThis.fetch = async (u, init) => (init.method === "PUT" ? json(reply) : json({ code: "internal", message: "down" }, 500));
    await C.saveEconomics("c1", legacy(), x);
    assert.equal(Boolean(s.state.plan_economics_stale), plan);
    assert.equal(Boolean(s.state.targets_economics_stale), targets);
  }
});

test("a changed cost makes the stored result stale, restoring it makes it fresh, a note-only edit changes nothing: only the backend's flags decide", async () => {
  const { s, io: x } = io(full());
  const e1 = { ...legacy(), operations: { horizon_years: 4, groups: { motor_fan_assembly: { cost_basis: "additional", plants: { th: { labour_usd_per_year: 2 } } } } } };
  globalThis.fetch = async (u, init) => json(init.method === "PUT" ? putReply({ economics: e1, plan_economics_stale: true }) : full({ economics: e1, plan_economics_stale: true }));
  await C.saveEconomics("c1", e1, x);
  assert.equal(s.state.plan_economics_stale, true);
  globalThis.fetch = async (u, init) => json(init.method === "PUT" ? putReply() : full());
  await C.saveEconomics("c1", legacy(), x);
  assert.equal("plan_economics_stale" in s.state, false);
  const noted = { ...legacy(), operations: { note: "only a note" } };
  globalThis.fetch = async (u, init) => json(init.method === "PUT" ? putReply({ economics: noted }) : full({ economics: noted }));
  await C.saveEconomics("c1", noted, x);
  assert.equal("plan_economics_stale" in s.state, false);
});

// ---- guards on the source ------------------------------------------------------------------------

const src = (p) => readFileSync(fileURLToPath(new URL(`../src/${p}`, import.meta.url)), "utf8");
const touched = ["lib/ops.ts", "lib/api/client.ts", "components/op/operational-fields.tsx", "components/op/ops-result.tsx", "components/op/ops-view.ts", "components/op/money-drawer.tsx", "components/op/screens/plan-step.tsx", "components/op/screens/compare-view.tsx", "components/op/screens/scenarios-view.tsx"];

test("no client-side staleness authority: no hashing or fingerprinting in the #281 UI files", () => {
  for (const f of touched) assert.doesNotMatch(src(f), /createHash|crypto\.subtle|sha256|digest\(/i, f);
  for (const f of ["components/op/ops-result.tsx", "components/op/ops-view.ts", "components/op/operational-fields.tsx", "components/op/screens/plan-step.tsx", "components/op/screens/compare-view.tsx"]) assert.doesNotMatch(src(f), /economics_basis/, f);
});

test("no console output and no handwritten API types in the #281 UI files", () => {
  for (const f of ["lib/ops.ts", "components/op/operational-fields.tsx", "components/op/ops-result.tsx", "components/op/ops-view.ts"]) {
    assert.doesNotMatch(src(f), /console\./, f);
    assert.doesNotMatch(src(f), /:\s*any\b|as any\b/, f);
  }
});

// ---- v4.2: active operations travel WITH the whole form economics ------------------------------

test("an existing $2M case with typed operations: plan, PUT and scenarios all carry value, rates, group costs, illustrative AND operations", async () => {
  const base = legacy();
  const serve = (u, m) => (m === "POST" && u.endsWith("/plan") ? json({ case_id: "c1", plan: F.planResult() }) : m === "PUT" ? json({ case_id: "c1", economics: base }) : m === "POST" && u.endsWith("/scenarios") ? json(F.caseJson()) : null);
  const c = F.caseJson({ plan: undefined, economics: base });
  const app = await mount(PlanStep, c, serve);
  app.find((e) => e.type === "input" && e.props.role === "switch")[0].props.onChange({ target: { checked: true } });
  await app.flush();
  await type(app, inputOf(app, "Labour (USD")[1], "0");
  await app.btn("Search moves to").props.onClick();
  await app.flush();
  const plan = JSON.parse(app.calls.find((x) => x.m === "POST" && x.u.endsWith("/plan")).raw).economics;
  for (const k of ["annual_import_value_usd", "duty_rates", "group_costs", "illustrative"]) assert.deepEqual(plan[k], base[k], `plan keeps ${k}`);
  assert.equal(plan.operations.groups.motor_fan_assembly.plants.th.labour_usd_per_year, 0);

  // the money drawer: typing operations after a quick total / late AI fill still saves the whole economics
  const d = await drawer(base);
  await turnOn(d);
  await type(d, inputOf(d, "Horizon")[0], "3");
  const out = await save(d);
  for (const k of ["annual_import_value_usd", "duty_rates", "group_costs", "illustrative"]) assert.deepEqual(out[k], base[k], `drawer keeps ${k}`);
  assert.equal(out.operations.horizon_years, 3);

  const sc = await mount(ScenariosView, F.caseJson({ step: "origin", economics: { ...base, operations: { horizon_years: 3 } }, scenarios: [{ label: "A", product, prediction: { origin_country: "Thailand" } }] }), serve);
  await sc.btn("Edit scenarios").props.onClick();
  await sc.flush();
  await sc.btn("Run again").props.onClick();
  await sc.flush();
  const body = JSON.parse(sc.calls.find((x) => x.u.endsWith("/scenarios")).raw).economics;
  for (const k of ["annual_import_value_usd", "duty_rates", "group_costs"]) assert.deepEqual(body[k], base[k], `scenarios keep ${k}`);
  assert.equal(body.operations.horizon_years, 3);
});

test("clearing the annual value with active operations shows a notice that saving replaces the stored money; with operations off there is none", async () => {
  const d = await drawer({ ...legacy(), annual_import_value_usd: undefined, operations: { max_lead_time_days: 90 } });
  assert.match(text(d), /No annual import value is set, so money figures will be unpriced/);
  const off = await drawer({ ...legacy(), annual_import_value_usd: undefined });
  assert.doesNotMatch(text(off), /No annual import value is set/);
  const out = await save(off);
  assert.equal("operations" in out, false, "mode off: no operations, nothing else invented");
});

test("ref name is the trimmed plant name, else the id (empty or whitespace names); unmapped error paths stay in the general list", () => {
  const th = { id: "th", name: "  Chonburi works ", country: "Thailand" };
  for (const [name, want] of [["  Chonburi works ", "Chonburi works"], ["", "th"], ["   ", "th"], [undefined, "th"]]) {
    assert.equal(O.plantRef({ ...th, name }).name, want);
  }
  assert.equal(O.refMismatch({ ref: { name: "th", country: "Thailand" } }, { ...th, name: "  " }), false);
  const e = O.setPlant({}, "g", { ...th, name: " " }, "labour_usd_per_year", 1);
  assert.equal(e.operations.groups.g.plants.th.ref.name, "th");
  const m = "economics: operations.unknown_source_key: groups.g.plants.th.sources.freight: unknown source key";
  const r = O.issuesByField(m);
  assert.equal(Object.keys(r.byKey).length, 0);
  assert.equal(r.general.length, 1);
  assert.match(r.general[0].text, /unknown_source_key/);
});
