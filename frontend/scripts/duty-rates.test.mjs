import "./render-support.mjs";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { afterEach, test } from "node:test";
import { createElement as h, useState } from "react";
import { deepMount } from "./lib/deep-mount.mjs";
import { stubNavigation } from "./lib/hook-runtime.mjs";

// #718 (R2): the money inputs are keyed by the plant's own spelling but the
// backend fills and prices a duty rate by country identity. The read/edit
// helpers match by that identity, so a rate typed for "Chinese Taipei" lands
// next to the derived "Taiwan" one as ONE rate, not a second key that loses.
// Real MoneyDrawer and EconomicsEditor callbacks; no browser, no network.
globalThis.window = new EventTarget();
globalThis.document = Object.assign(new EventTarget(), { body: { style: {} }, activeElement: null });
stubNavigation({ id: "c1", search: "" });
const D = await import("../src/lib/duty-rates.ts");
const { mergeDutyPrefill } = await import("../src/components/op/money-prefill.tsx");
const { MoneyDrawer } = await import("../src/components/op/money-drawer.tsx");
const { EconomicsEditor } = await import("../src/components/op/plan-bits.tsx");

const live = [];
afterEach(() => live.splice(0).forEach((a) => a.unmount()));

// ---- cross-language contract with Go domain.DutyCountry -------------------------------------
const fixture = JSON.parse(readFileSync(new URL("../../backend/internal/domain/testdata/duty_country.json", import.meta.url), "utf8"));
test("dutyCountry agrees with the Go DutyCountry on the shared fixture", () => {
  assert.ok(fixture.cases.length > 40);
  for (const [input, want] of fixture.cases) assert.equal(D.dutyCountry(input), want, `dutyCountry(${JSON.stringify(input)})`);
});

// ---- the traced state -------------------------------------------------------------------
const PREFILLED = () => ({
  illustrative: true,
  annual_import_value_usd: 1000000,
  duty_rates: { Taiwan: 0.1, China: 0.412 },
  duty_rates_live_mfn: ["Taiwan", "China"],
  duty_rates_from_table: ["Taiwan"],
  duty_additional: { Taiwan: [{ heading: "9903.05.76", name: "Section 301", rate: 0.063 }], China: [{ heading: "9903.05.31", name: "Section 301", rate: 0.125 }] },
  duty_rates_note: "Duty rates for Taiwan, China are the live HTS MFN rate.",
});
const product = { name: "Heater", components: [], plants: [{ id: "p1", name: "Taipei plant", country: "Chinese Taipei" }, { id: "p2", name: "Shenzhen", country: "China" }] };

test("helpers: an alias plant displays the derived canonical rate; edit makes one usable rate and clears its auto provenance", () => {
  const e = PREFILLED();
  const frozen = JSON.parse(JSON.stringify(e));
  assert.equal(D.dutyRateFor(e.duty_rates, "Chinese Taipei"), 0.1);
  const out = D.setDutyRate(e, "Chinese Taipei", 0.2);
  assert.deepEqual(e, frozen, "input not mutated");
  assert.deepEqual(out.duty_rates, { "Chinese Taipei": 0.2, China: 0.412 });
  assert.deepEqual(out.duty_rates_live_mfn, ["China"]);
  assert.equal("duty_rates_from_table" in out, false);
  assert.deepEqual(Object.keys(out.duty_additional), ["China"]);
  assert.equal(out.duty_rates_note, e.duty_rates_note, "note kept");
  assert.equal(out.annual_import_value_usd, 1000000);
});

test("helpers: canonical plant over an alias-keyed rate, 0 stays 0, empty clears the logical rate, others untouched", () => {
  const e = { duty_rates: { "Taiwan, China": 0.3, Vietnam: 0.05 }, duty_rates_live_mfn: ["Taiwan China", "Vietnam"] };
  assert.equal(D.dutyRateFor(e.duty_rates, "Taiwan"), 0.3);
  const zero = D.setDutyRate(e, "Taiwan", 0);
  assert.deepEqual(zero.duty_rates, { Taiwan: 0, Vietnam: 0.05 });
  assert.equal(D.dutyRateFor(zero.duty_rates, "Chinese Taipei"), 0);
  assert.deepEqual(zero.duty_rates_live_mfn, ["Vietnam"]);
  const cleared = D.setDutyRate(e, "Taiwan", undefined);
  assert.deepEqual(cleared.duty_rates, { Vietnam: 0.05 });
  assert.equal(D.dutyRateFor(cleared.duty_rates, "Chinese Taipei"), undefined);
});

test("helpers: legacy conflicting duplicates show the backend's choice and a note; equal duplicates do not warn; edit resolves", () => {
  const rates = { Taiwan: 0.1, "Chinese Taipei": 0.2, "Taiwan, China": 0.3 };
  assert.equal(D.dutyRateFor(rates, "Chinese Taipei"), 0.1, "the canonical key is the one priced");
  const c = D.dutyRateConflict(rates, "Chinese Taipei");
  assert.equal(c.used, "Taiwan");
  assert.deepEqual(c.shadowed.map((s) => s.key).sort(), ["Chinese Taipei", "Taiwan, China"]);
  assert.match(D.dutyConflictNote(c), /Taiwan 10% is used, not .*Chinese Taipei 20%/);
  assert.equal(D.dutyRateConflict({ Taiwan: 0.1, "Chinese Taipei": 0.1 }, "Taiwan"), undefined);
  assert.equal(D.dutyRateConflict({ Taiwan: 0.1 }, "Taiwan"), undefined);
  // no canonical key: sorted-first, as the backend
  assert.equal(D.dutyRateFor({ "Taiwan, China": 0.3, "Chinese Taipei": 0.4 }, "Taiwan"), 0.4);
  const fixed = D.setDutyRate({ duty_rates: rates }, "Taiwan", 0.25);
  assert.deepEqual(fixed.duty_rates, { Taiwan: 0.25 });
  assert.equal(D.dutyRateConflict(fixed.duty_rates, "Taiwan"), undefined);
});

test("mergeDutyPrefill: a lookup does not land its canonical key beside a rate the user typed under another spelling", () => {
  const started = { duty_rates: {} };
  const latest = { duty_rates: { "Chinese Taipei": 0.2 } };
  const merged = mergeDutyPrefill(latest, started, { duty_rates: { Taiwan: 0.1, China: 0.4 }, duty_rates_live_mfn: ["Taiwan", "China"] });
  assert.deepEqual(merged.duty_rates, { "Chinese Taipei": 0.2, China: 0.4 });
  assert.deepEqual(merged.duty_rates_live_mfn, ["China"]);
});

// ---- MoneyDrawer (real callbacks) -----------------------------------------------------------
const label = (c) => `Duty rate for ${c}, percent`;
const rateInput = (app, c) => app.find((e) => e.type === "input" && e.props["aria-label"] === label(c))[0];

async function drawer(value, countries = ["Chinese Taipei", "China"]) {
  const saved = [];
  const app = deepMount(h(MoneyDrawer, { value, product, countries, onSave: (e) => saved.push(e), onClose() {} }));
  live.push(app);
  app.refresh();
  await app.flush();
  return Object.assign(app, { saved });
}

test("MoneyDrawer: plant Chinese Taipei + prefilled Taiwan 10% displays 10%; typing 20% saves one 0.2 and drops the auto provenance", async () => {
  const value = PREFILLED();
  const app = await drawer(value);
  assert.equal(rateInput(app, "Chinese Taipei").props.value, "10");
  assert.equal(rateInput(app, "China").props.value, "41.2");
  rateInput(app, "Chinese Taipei").props.onChange({ target: { value: "20" } });
  await app.flush();
  assert.equal(rateInput(app, "Chinese Taipei").props.value, "20");
  await app.btn("Save money inputs").props.onClick();
  await app.flush();
  const out = app.saved[0];
  assert.deepEqual(out.duty_rates, { "Chinese Taipei": 0.2, China: 0.412 });
  assert.deepEqual(out.duty_rates_live_mfn, ["China"]);
  assert.equal("duty_rates_from_table" in out, false);
  assert.deepEqual(Object.keys(out.duty_additional), ["China"]);
  assert.equal(value.duty_rates.Taiwan, 0.1, "input economics not mutated");
});

test("MoneyDrawer: canonical plant over an alias-keyed rate; 0 stays 0; clearing removes the rate; unrelated rate unchanged", async () => {
  const app = await drawer({ duty_rates: { "Taiwan, China": 0.3, China: 0.25 }, annual_import_value_usd: 5 }, ["Taiwan", "China"]);
  assert.equal(rateInput(app, "Taiwan").props.value, "30");
  rateInput(app, "Taiwan").props.onChange({ target: { value: "0" } });
  await app.flush();
  await app.btn("Save money inputs").props.onClick();
  await app.flush();
  assert.deepEqual(app.saved[0].duty_rates, { Taiwan: 0, China: 0.25 });
  rateInput(app, "Taiwan").props.onChange({ target: { value: "" } });
  await app.flush();
  await app.btn("Save money inputs").props.onClick();
  await app.flush();
  assert.deepEqual(app.saved[1].duty_rates, { China: 0.25 });
});

test("MoneyDrawer: legacy conflicting keys show the priced value and a precedence note; editing collapses them", async () => {
  const app = await drawer({ duty_rates: { Taiwan: 0.1, "Chinese Taipei": 0.2 }, annual_import_value_usd: 5 });
  assert.equal(rateInput(app, "Chinese Taipei").props.value, "10");
  const notes = () => app.all().map((e) => (typeof e.props?.children === "string" ? e.props.children : "")).filter((t) => /Saved under more than one spelling/.test(t));
  assert.equal(notes().length, 1);
  assert.match(notes()[0], /Taiwan 10% is used, not Chinese Taipei 20%/);
  rateInput(app, "Chinese Taipei").props.onChange({ target: { value: "20" } });
  await app.flush();
  assert.equal(notes().length, 0);
  await app.btn("Save money inputs").props.onClick();
  await app.flush();
  assert.deepEqual(app.saved[0].duty_rates, { "Chinese Taipei": 0.2 });
});

// ---- EconomicsEditor (plan inline) ------------------------------------------------------
function editor(initial, countries = ["Chinese Taipei", "China"]) {
  const box = { latest: initial };
  function Host() {
    const [v, setV] = useState(initial);
    box.latest = v;
    return h(EconomicsEditor, { value: v, onChange: setV, product, countries });
  }
  const app = deepMount(h(Host));
  live.push(app);
  app.refresh();
  return Object.assign(app, { box });
}

test("EconomicsEditor (plan inline): same traced edit makes one usable 0.2 and clears the matching auto provenance", async () => {
  const value = PREFILLED();
  const app = editor(value);
  await app.flush();
  assert.equal(rateInput(app, "Chinese Taipei").props.value, "10");
  rateInput(app, "Chinese Taipei").props.onChange({ target: { value: "20" } });
  await app.flush();
  const e = app.box.latest;
  assert.deepEqual(e.duty_rates, { "Chinese Taipei": 0.2, China: 0.412 });
  assert.deepEqual(e.duty_rates_live_mfn, ["China"]);
  assert.equal("duty_rates_from_table" in e, false);
  assert.deepEqual(Object.keys(e.duty_additional), ["China"]);
  assert.equal(value.duty_rates.Taiwan, 0.1, "input economics not mutated");
  // 0 and clear, through the same callbacks
  rateInput(app, "Chinese Taipei").props.onChange({ target: { value: "0" } });
  await app.flush();
  assert.equal(app.box.latest.duty_rates["Chinese Taipei"], 0);
  rateInput(app, "Chinese Taipei").props.onChange({ target: { value: "" } });
  await app.flush();
  assert.deepEqual(app.box.latest.duty_rates, { China: 0.412 });
});

test("EconomicsEditor (plan inline): canonical plant over an alias-keyed rate; legacy conflict is shown and resolved by editing", async () => {
  const app = editor({ duty_rates: { "Chinese Taipei": 0.2, Taiwan: 0.1 }, annual_import_value_usd: 5 }, ["Taiwan", "China"]);
  await app.flush();
  assert.equal(rateInput(app, "Taiwan").props.value, "10");
  assert.ok(app.all().some((e) => typeof e.props?.children === "string" && /Taiwan 10% is used, not Chinese Taipei 20%/.test(e.props.children)));
  rateInput(app, "Taiwan").props.onChange({ target: { value: "15" } });
  await app.flush();
  assert.deepEqual(app.box.latest.duty_rates, { Taiwan: 0.15 });
});

// ---- S. Korea duty alias (#718 review): duty reads it as South Korea; normalisation is unchanged ----
test("dutyCountry: S. Korea is South Korea for duty and a rate keyed South Korea is shown for it", () => {
  assert.equal(D.dutyCountry("S. Korea"), "South Korea");
  assert.equal(D.dutyRateFor({ "South Korea": 0.08 }, "S. Korea"), 0.08);
});

// ---- inherited property names are free text, not table keys ---------------------------------
const PROTO_NAMES = ["constructor", "__proto__", "toString", "hasOwnProperty", "valueOf"];

test("dutyCountry: prototype-named free text is a plain string, never an inherited value or a throw", () => {
  for (const n of PROTO_NAMES) {
    const got = D.dutyCountry(n);
    assert.equal(typeof got, "string", n);
    assert.equal(got, D.dutyCountry(n), "stable");
    assert.ok(got.toLowerCase().includes(n.toLowerCase().replaceAll("_", "")), `${n} -> ${got}`);
  }
  assert.equal(D.dutyCountry("constructor/Taiwan, China"), "Constructor/Taiwan");
});

test("rate helpers: prototype-named countries read, write and clear as ordinary keys", () => {
  for (const n of PROTO_NAMES) {
    assert.equal(D.dutyRateFor({ China: 0.2 }, n), undefined, n);
    const set = D.setDutyRate({ duty_rates: { China: 0.2 } }, n, 0.07);
    assert.equal(Object.getPrototypeOf(set.duty_rates), Object.prototype, `${n}: prototype of the rate map untouched`);
    assert.equal(Object.keys(set.duty_rates).length, 2, `${n}: key stored as own property`);
    assert.equal(D.dutyRateFor(set.duty_rates, n), 0.07, n);
    assert.deepEqual(Object.keys(D.setDutyRate(set, n, undefined).duty_rates), ["China"]);
  }
});

test("MoneyDrawer: a plant country typed as 'constructor' or '__proto__' renders and saves its rate", async () => {
  for (const n of ["constructor", "__proto__"]) {
    const app = await drawer({ duty_rates: { China: 0.25 }, annual_import_value_usd: 5 }, [n, "China"]);
    const input = rateInput(app, n);
    assert.ok(input, `${n}: rate input rendered`);
    input.props.onChange({ target: { value: "9" } });
    await app.flush();
    assert.equal(rateInput(app, n).props.value, "9");
    await app.btn("Save money inputs").props.onClick();
    await app.flush();
    assert.equal(Object.keys(app.saved[0].duty_rates).length, 2, n);
    assert.equal(D.dutyRateFor(app.saved[0].duty_rates, n), 0.09);
  }
});
