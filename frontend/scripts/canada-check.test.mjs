import assert from "node:assert/strict";
import { test } from "node:test";
import { afterEach } from "node:test";
import { createElement as h, useState } from "react";
import { html, text } from "./render-support.mjs";
import { json } from "./lib/hook-runtime.mjs";
import { deepMount } from "./lib/deep-mount.mjs";

// Canada's marking rules (D720, D725): the label is always shown; details
// (row text, basis, assumptions) sit behind "Details" (rule 16).
const { CanadaCheckView, CanadaCheck, canCheckCanada } = await import("../src/components/op/canada-check.tsx");

const decided = {
  jurisdiction: "ca",
  rules: "SOR/94-23 Schedule III",
  table_version: "ca-sor-94-23-v1",
  label: "Rules check only — no accuracy figure yet (too few published rulings to test against)",
  country: "Mexico",
  note: "n",
  result: {
    status: "decided",
    origin: "Mexico",
    basis: "SOR/94-23 s. 4(1)(c); SOR/94-23 Sch. III, 6301.20-6301.90",
    reason: "each foreign material undergoes the change in tariff classification the rule requires, in Mexico",
    assumes: ["the goods are imported into Canada from the United States or Mexico (s. 3(1))"],
    rule: { id: "SOR/94-23 Sch. III, 6301.20-6301.90", hts: "6301.20-6301.90", text: "A change to subheadings 6301.20 through 6301.90 from any other heading.", alternatives: 1, encoded: 1 },
    materials: [{ name: "cotton yarn", hts: "5205.11", origin: "China", foreign: true, status: "shift", alternative: 1 }],
  },
};

test("decided: origin, short label, details folded", () => {
  const m = html(CanadaCheckView, { v: decided });
  const t = text(m);
  assert.match(t, /Country-of-origin marking for Canada \(rules check\)/);
  assert.match(t, /Rules check only — no accuracy figure yet \(too few published rulings to test against\)/);
  assert.match(t, /Mexico under Canada's marking rules, from the codes given\./);
  assert.match(m, /<details[^>]*>.*Details.*SOR\/94-23 s\. 4\(1\)\(c\)/s);
});

test("undetermined keeps the label and shows no origin", () => {
  const v = { ...decided, result: { ...decided.result, status: "undetermined", origin: undefined, reason: "the shift cannot be decided" } };
  const t = text(html(CanadaCheckView, { v }));
  assert.match(t, /no accuracy figure yet/);
  assert.match(t, /Not decided from the codes given\./);
});

test("the button needs a code on the good and on a part", () => {
  assert.equal(canCheckCanada({ product: { name: "p", hts: "6301.30", components: [{ name: "a", made_in: "China", hts: "5205.11" }] } }), true);
  assert.equal(canCheckCanada({ product: { name: "p", hts: "6301.30", components: [{ name: "a", made_in: "China" }] } }), false);
  assert.equal(canCheckCanada({ product: { name: "p", components: [{ name: "a", made_in: "China", hts: "5205.11" }] } }), false);
});

// #731: a plain label, and missing codes as a short checklist. Never a
// preferential (USMCA) claim.
const { copy } = await import("../src/lib/copy.ts");
const ask = (product) => text(html(CanadaCheck, { c: { id: "case_1", product }, auto: true }));

test("the label is plain language, not a statute name", () => {
  assert.equal(copy.canadaCheck.title, "Country-of-origin marking for Canada (rules check)");
  assert.doesNotMatch(copy.canadaCheck.title, /SOR|Schedule/);
});

test("missing codes: a checklist of what to add, only the missing items", () => {
  const none = ask({ name: "p", components: [{ name: "a", made_in: "China" }] });
  assert.match(none, /Product HS code/);
  assert.match(none, /HS code for at least one part/);
  const goodOnly = ask({ name: "p", hts: "6301.30", components: [{ name: "a", made_in: "China" }] });
  assert.doesNotMatch(goodOnly, /Product HS code/);
  assert.match(goodOnly, /HS code for at least one part/);
  const partOnly = ask({ name: "p", components: [{ name: "a", made_in: "China", hts: "5205.11" }] });
  assert.match(partOnly, /Product HS code/);
  assert.doesNotMatch(partOnly, /HS code for at least one part/);
  const m = html(CanadaCheck, { c: { id: "c", product: { name: "p", components: [] } }, auto: true });
  assert.match(m, /<ul[^>]*data-canada-needs/);
  assert.match(m, /Add codes/);
});

test("the copy never implies preferential USMCA qualification", () => {
  const all = JSON.stringify(copy.canadaCheck) + ask({ name: "p", components: [] }) + text(html(CanadaCheckView, { v: decided }));
  assert.doesNotMatch(all, /USMCA|CUSMA|preferential|duty[- ]free|qualif/i);
});

// Guided facts: Yes / No / Not sure per answerable fact; answers are local and
// sent with the request. Not sure leaves the key out; it is never false.
const { buildAnswers, setAnswer, factItems, itemText, buildBody, parsePercent, setShare } = await import("../src/components/op/canada-check.tsx");
const fact = (key, label) => ({ key, kind: "fact", label, detail: `why ${key}`, answerable: true, blocking: true });
const guided = (missing, extra = {}) => ({
  ...decided,
  result: { ...decided.result, status: "undetermined", origin: undefined, reason: "needs a fact" },
  guidance: { state: "answerable", missing, ...extra },
});

test("answers map to the request body: true, false, omitted", () => {
  let a = {};
  a = setAnswer(a, "simple_assembly", true);
  a = setAnswer(a, "gri_2a", false);
  assert.deepEqual(buildAnswers(a), { simple_assembly: true, gri_2a: false });
  assert.ok(!("bending_to_shape" in buildAnswers(a)));
  a = setAnswer(a, "gri_2a", null);
  assert.deepEqual(buildAnswers(a), { simple_assembly: true });
  assert.deepEqual(buildAnswers({ simple_assembly: undefined, gri_2a: "x" }), {});
});

test("only answerable facts get a control; review never does", () => {
  const g = guided([
    fact("simple_assembly", "Is it only simple assembly?"),
    { key: "goods_description", kind: "review", label: "Goods description", answerable: false, blocking: true },
    { key: "good_code", kind: "code", label: "Give the good's full HS code", answerable: false, blocking: true },
  ], { review: { reason: "A person has to read part of this rule." } });
  assert.deepEqual(factItems(g.guidance).map((x) => x.key), ["simple_assembly"]);
  const m = html(CanadaCheckView, { v: g, onAnswer: () => {} });
  assert.equal((m.match(/type="radio"/g) ?? []).length, 3);
  assert.match(m, /<legend[^>]*>Is it only simple assembly\?<\/legend>/);
  assert.match(text(m), /A person needs to read this rule\./);
  assert.match(text(m), /Why we ask.*A person has to read part of this rule\./);
  const r = html(CanadaCheckView, { v: guided([{ key: "person_reading", kind: "review", label: "x", answerable: false, blocking: true }]), onAnswer: () => {} });
  assert.doesNotMatch(r, /type="radio"/);
  assert.match(r, /data-canada-review/);
});

test("material index 0 is a real part; code and share items are plain text", () => {
  const item = { key: "material_hts", kind: "code", material: 0, label: "Give this material's HS code", answerable: false, blocking: true };
  assert.equal(itemText(item, ["cotton yarn", "thread"]), "Give this material's HS code (cotton yarn)");
  assert.equal(itemText({ ...item, material: 1 }, ["cotton yarn"]), "Give this material's HS code (Part 2)");
  assert.equal(itemText({ ...item, material: undefined }, ["cotton yarn"]), "Give this material's HS code");
  const v = guided([item, { key: "value_share", kind: "share", material: 0, label: "Add its value share", answerable: false, blocking: false }], { state: "needs_input" });
  const m = html(CanadaCheckView, { v, parts: ["cotton yarn"], caseId: "c1", onAnswer: () => {} });
  assert.match(text(m), /To decide, add: .*HS code \(cotton yarn\).*value share \(cotton yarn\)/);
  assert.doesNotMatch(m, /type="radio"/);
  assert.match(m, /href="\/cases\/new\?from=c1#field-good-hts"/);
});

test("a decided answer keeps answered facts clearable", () => {
  const m = html(CanadaCheckView, { v: decided, answers: { gri_2a: false }, onAnswer: () => {} });
  assert.match(text(m), /Your answers: Parts only gathered together no Clear all/);
});

// A rendered CanadaCheck with a fake api: answering re-POSTs the same product
// with the answers; the case is untouched; the last answer wins.
afterEach(() => live.splice(0).forEach((a) => a.unmount()));
const live = [];
const product = { name: "p", hts: "6301.30", components: [{ name: "cotton yarn", made_in: "China", hts: "5205.11" }] };
const radios = (app) => app.find((e) => e.type === "input" && e.props.type === "radio");
function mountCheck(respond) {
  const calls = [];
  globalThis.fetch = (_u, init) => {
    const body = JSON.parse(init.body);
    calls.push(body);
    return respond(body, calls.length - 1);
  };
  const c = { id: "c1", product };
  const app = deepMount(h(CanadaCheck, { c, auto: true }));
  live.push(app);
  app.calls = calls;
  app.c = c;
  return app;
}

test("rendered: answering re-POSTs with the answer, clearing removes the key, case unchanged", async () => {
  const before = JSON.stringify(product);
  const app = mountCheck((b) => json(b.simple_assembly === true ? { ...decided, result: { ...decided.result, status: "decided", origin: "Mexico" } } : guided([fact("simple_assembly", "Is it only simple assembly?")])));
  app.refresh();
  await app.flush();
  assert.deepEqual(app.calls, [{ product }]);
  assert.equal(radios(app).length, 3);
  radios(app)[0].props.onChange();
  app.refresh();
  await app.flush();
  assert.deepEqual(app.calls[1], { product, simple_assembly: true });
  assert.equal(radios(app).length, 0, "decided: no guidance, no controls");
  const clear = app.btn("Clear all");
  assert.ok(clear, "answered fact stays clearable");
  clear.props.onClick();
  app.refresh();
  await app.flush();
  assert.deepEqual(app.calls[2], { product });
  assert.equal(radios(app).length, 3);
  assert.equal(JSON.stringify(product), before);
});

test("rendered: No sends false and Not sure sends nothing", async () => {
  const app = mountCheck(() => json(guided([fact("gri_2a", "Is it unassembled?")])));
  app.refresh();
  await app.flush();
  radios(app)[1].props.onChange();
  app.refresh();
  await app.flush();
  assert.deepEqual(app.calls[1], { product, gri_2a: false });
  radios(app)[2].props.onChange();
  app.refresh();
  await app.flush();
  assert.ok(!("gri_2a" in app.calls[2]));
});

test("rendered: a slow earlier answer cannot overwrite a later one", async () => {
  const held = [];
  const app = mountCheck((b, i) => (i === 0 ? json(guided([fact("gri_2a", "Q")])) : new Promise((res) => held.push([b, res]))));
  app.refresh();
  await app.flush();
  radios(app)[0].props.onChange();
  app.refresh();
  await app.flush();
  radios(app)[1].props.onChange();
  app.refresh();
  await app.flush();
  assert.equal(held.length, 2);
  held[1][1](json(guided([fact("gri_2a", "Q")], { state: "answerable" })));
  await app.flush();
  held[0][1](json({ ...decided, result: { ...decided.result, status: "decided", origin: "Mexico" } }));
  await app.flush();
  assert.equal(app.all().some((e) => e.props?.["data-canada-check"] === "decided"), false, "stale reply ignored");
  assert.equal(radios(app).length, 3);
});

test("rendered: an error shows errorText and keeps the last answers", async () => {
  const app = mountCheck((b, i) => (i === 0 ? json(guided([fact("gri_2a", "Q")])) : json({ message: "boom", code: "internal" }, 500)));
  app.refresh();
  await app.flush();
  radios(app)[0].props.onChange();
  app.refresh();
  await app.flush();
  assert.ok(app.find((e) => e.props?.role === "alert").length > 0);
  assert.equal(radios(app)[2].props.checked, true, "back to Not sure");
});

// Real-table guidance: shares are typed as percents and sent through the
// good's code, country and materials (a product has nowhere to carry them).
const share = (key, material) => ({ key, kind: "share", material, label: `Value share of ${material === 0 ? "cotton yarn" : "thread"}`, answerable: false, blocking: true });

test("parsePercent: empty is unknown, 0 is a real 0, bad entries are refused", () => {
  assert.equal(parsePercent(""), null);
  assert.equal(parsePercent("  "), null);
  assert.equal(parsePercent("0"), 0);
  assert.equal(parsePercent("7"), 0.07);
  assert.ok(parsePercent("7.00004") > 0.07, "not rounded down onto the 7% limit");
  assert.equal(parsePercent("7.01"), 0.0701);
  assert.equal(parsePercent("12,5"), 0.125);
  assert.equal(parsePercent("100"), 1);
  for (const bad of ["101", "-1", "abc", "1e2", "NaN", "Infinity", "5%"]) assert.equal(parsePercent(bad), undefined, bad);
});

test("buildBody: product alone, or good + country + materials with the shares", () => {
  const p = { name: "p", hts: "1601", components: [{ name: "a", hts: "1601", made_in: "China" }, { name: "b", hts: "3926.90", made_in: "Mexico" }] };
  assert.deepEqual(buildBody(p, {}, {}), { product: p });
  assert.deepEqual(buildBody(p, { gri_2a: false }, {}), { product: p, gri_2a: false });
  const s = setShare(setShare({}, "value_share@0", 0), "weight_share@1", 0.2);
  assert.deepEqual(buildBody(p, { gri_2a: true }, s, "Mexico"), {
    good_hts: "1601",
    country: "Mexico",
    materials: [
      { name: "a", hts: "1601", origin: "China", value_share: 0 },
      { name: "b", hts: "3926.90", origin: "Mexico", weight_share: 0.2 },
    ],
    gri_2a: true,
  });
  assert.deepEqual(setShare(s, "value_share@0", null), { "weight_share@1": 0.2 });
});

test("share items get a percent input labelled with the part, no radios", () => {
  const v = guided([share("value_share", 0)], { state: "needs_input" });
  const m = html(CanadaCheckView, { v, parts: ["cotton yarn"], onAnswer: () => {}, onShare: () => {} });
  assert.match(m, /<label[^>]*>Value share of cotton yarn<\/label>/);
  assert.match(m, /<input[^>]*inputMode="decimal"/);
  assert.doesNotMatch(m, /type="radio"/);
  assert.doesNotMatch(text(m), /To decide, add/);
});

const real = { name: "p", hts: "1601", components: [{ name: "cotton yarn", made_in: "China", hts: "1601" }] };
const undet = () => guided([share("value_share", 0)], { state: "needs_input" });
const decidedMx = () => ({ ...decided, result: { ...decided.result, status: "decided", origin: "Mexico" } });
function mountWith(respond, product = real) {
  const calls = [];
  globalThis.fetch = (_u, init) => {
    calls.push(JSON.parse(init.body));
    return respond(calls[calls.length - 1], calls.length - 1);
  };
  let setP;
  const Wrap = function Wrap() {
    const [p, set] = useState(product);
    setP = set;
    return h(CanadaCheck, { c: { id: "c1", product: p }, auto: true });
  };
  const app = deepMount(h(Wrap));
  live.push(app);
  app.calls = calls;
  app.setProduct = (p) => setP(p);
  return app;
}
const inputs = (app) => app.find((e) => e.type === "input" && e.props.inputMode === "decimal");

test("rendered: a typed percent re-runs via good_hts + materials; clearing returns to the product", async () => {
  const app = mountWith((b) => json(b.materials ? decidedMx() : { ...undet(), country: "Mexico" }));
  app.refresh();
  await app.flush();
  assert.deepEqual(app.calls[0], { product: real });
  const box = inputs(app)[0];
  box.props.onChange({ target: { value: "5" } });
  app.refresh();
  inputs(app)[0].props.onBlur();
  app.refresh();
  await app.flush();
  assert.deepEqual(app.calls[1], { good_hts: "1601", country: "Mexico", materials: [{ name: "cotton yarn", hts: "1601", origin: "China", value_share: 0.05 }] });
  assert.match(app.all().map((e) => JSON.stringify(e.props?.children ?? "")).join(" "), /Value share \(cotton yarn\)/);
  app.btn("Clear all").props.onClick();
  app.refresh();
  await app.flush();
  assert.deepEqual(app.calls[2], { product: real });
});

test("rendered: a bad percent sends nothing; 0 is sent as 0", async () => {
  const app = mountWith(() => json({ ...undet(), country: "Mexico" }));
  app.refresh();
  await app.flush();
  inputs(app)[0].props.onChange({ target: { value: "150" } });
  app.refresh();
  inputs(app)[0].props.onBlur();
  app.refresh();
  await app.flush();
  assert.equal(app.calls.length, 1);
  inputs(app)[0].props.onChange({ target: { value: "0" } });
  app.refresh();
  inputs(app)[0].props.onBlur();
  app.refresh();
  await app.flush();
  assert.equal(app.calls[1].materials[0].value_share, 0);
});

test("rendered: Clear all with several hidden answers is one request", async () => {
  const q = () => ({ ...guided([fact("gri_2a", "Q"), share("value_share", 0)], { state: "answerable" }), country: "Mexico" });
  const app = mountWith((b) => json(b.materials ? decidedMx() : q()));
  app.refresh();
  await app.flush();
  radios(app)[0].props.onChange();
  app.refresh();
  await app.flush();
  inputs(app)[0].props.onChange({ target: { value: "5" } });
  app.refresh();
  inputs(app)[0].props.onBlur();
  app.refresh();
  await app.flush();
  // decided now: both answers are hidden but listed
  assert.match(app.all().map((e) => JSON.stringify(e.props?.children ?? "")).join(" "), /Parts only gathered together/);
  const before = app.calls.length;
  assert.deepEqual(app.calls[before - 1].gri_2a, true);
  assert.equal(app.calls[before - 1].materials[0].value_share, 0.05);
  app.btn("Clear all").props.onClick();
  app.refresh();
  await app.flush();
  assert.equal(app.calls.length, before + 1, "one request");
  assert.deepEqual(app.calls[before], { product: real });
});

test("rendered: a product change aborts the run in flight and busy clears", async () => {
  const held = [];
  const app = mountWith((b, i) => (i === 0 ? json(guided([fact("gri_2a", "Q")])) : new Promise((res) => held.push(res))));
  app.refresh();
  await app.flush();
  radios(app)[0].props.onChange();
  app.refresh();
  await app.flush();
  assert.equal(app.find((e) => e.props?.["aria-busy"] === true).length, 1, "busy while running");
  app.setProduct({ ...real, name: "other" });
  app.refresh();
  await app.flush();
  assert.equal(held.length, 2, "the answer run and the new product's check");
  held[0](json(decidedMx()));
  await app.flush();
  assert.equal(app.find((e) => e.props?.["aria-busy"] === true).length, 0, "busy cleared");
  assert.equal(app.all().some((e) => e.props?.["data-canada-check"] === "decided"), false, "old product's reply ignored");
});

test("rendered: 7% and 7.01% reach the API as different shares", async () => {
  const app = mountWith(() => json({ ...undet(), country: "Mexico" }));
  app.refresh();
  await app.flush();
  for (const t of ["7", "7.00004"]) {
    inputs(app)[0].props.onChange({ target: { value: t } });
    app.refresh();
    inputs(app)[0].props.onBlur();
    app.refresh();
    await app.flush();
  }
  assert.equal(app.calls[1].materials[0].value_share, 0.07);
  assert.ok(app.calls[2].materials[0].value_share > 0.07);
});

test("rendered: Enter then blur is one request and the box is not remounted", async () => {
  const app = mountWith(() => json({ ...undet(), country: "Mexico" }));
  app.refresh();
  await app.flush();
  const box = inputs(app)[0];
  box.props.onChange({ target: { value: "5" } });
  app.refresh();
  const same = inputs(app)[0];
  same.props.onKeyDown({ key: "Enter" });
  same.props.onBlur();
  app.refresh();
  await app.flush();
  assert.equal(app.calls.length, 2, "one request");
  assert.equal(inputs(app)[0].props.value, "5");
  inputs(app)[0].props.onBlur();
  app.refresh();
  await app.flush();
  assert.equal(app.calls.length, 2, "an unchanged blur sends nothing");
});

test("rendered: a new product drops the old result, its country and answers", async () => {
  const app = mountWith((b) => json(b.product?.name === "other" ? { ...undet(), country: "Canada" } : { ...undet(), country: "Mexico" }));
  app.refresh();
  await app.flush();
  inputs(app)[0].props.onChange({ target: { value: "5" } });
  app.refresh();
  inputs(app)[0].props.onBlur();
  app.refresh();
  await app.flush();
  assert.equal(app.calls[1].country, "Mexico");
  app.setProduct({ ...real, name: "other" });
  app.refresh();
  await app.flush();
  const last = app.calls[app.calls.length - 1];
  assert.deepEqual(last, { product: { ...real, name: "other" } });
  // The harness ignores React keys, so check what drives the remount: answers
  // are empty and the reset counter moved.
  const view = app.find((e) => e.type?.name === "CanadaCheckView")[0];
  assert.deepEqual(view.props.shares, {});
  assert.ok(view.props.epoch > 0);
});

test("rendered: Your answers stays until the new result lands, then goes", async () => {
  let release;
  const app = mountWith((b, i) => {
    if (!b.materials && i >= 2) return new Promise((r) => (release = () => r(json({ ...undet(), country: "Mexico" }))));
    return json(b.materials ? decidedMx() : { ...undet(), country: "Mexico" });
  });
  app.refresh();
  await app.flush();
  inputs(app)[0].props.onChange({ target: { value: "5" } });
  app.refresh();
  inputs(app)[0].props.onBlur();
  app.refresh();
  await app.flush();
  const row = () => app.find((e) => e.props?.["data-canada-answers"] !== undefined).length;
  assert.equal(row(), 1, "decided with a hidden answer");
  app.btn("Clear all").props.onClick();
  app.refresh();
  await app.flush();
  assert.equal(app.calls.length, 3, "clear request sent and held");
  assert.equal(row(), 1, "row stays while the request runs");
  release();
  await app.flush();
  assert.equal(row(), 0, "gone once the new result lands");
});
