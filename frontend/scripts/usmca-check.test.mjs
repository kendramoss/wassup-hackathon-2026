import assert from "node:assert/strict";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

// D740: the USMCA origin-rule check. Rules on the server; the screen sends
// what was typed as typed, never defaults a confirmation to true, and shows
// the selected-rule status with its calculation and what is still needed.
const L = await import("../src/lib/usmca-check.ts");
const { UsmcaResult, UsmcaCheck } = await import("../src/components/op/usmca-check.tsx");
const { copy } = await import("../src/lib/copy.ts");

const BANNED = /originating|qualif|eligib|duty-free|tariff|saving|certificate/i;

test("a blank form never confirms anything and sends nothing it was not told", () => {
  const f = L.blankForm("8202.31");
  assert.equal(f.complete, false);
  assert.equal(f.simple, false);
  const b = L.body(f);
  assert.deepEqual(b, { destination: "US", regime: "usmca", good_hs: "8202.31", profile: { simple_goods_confirmed: false, materials_list_complete: false } });
});

test("amounts travel as the typed strings; blank is left out, never zero", () => {
  const f = { ...L.blankForm("8207.13"), method: "net_cost", nc: " 1250.50 ", tv: "999", vnm: "" };
  const b = L.body(f);
  assert.equal(b.net_cost, "1250.50");
  assert.equal(typeof b.net_cost, "string");
  assert.equal("vnm" in b, false);
  assert.equal("transaction_value" in b, false);
  const z = L.body({ ...f, vnm: "0" });
  assert.equal(z.vnm, "0");
  // switching the method never reinterprets an amount: each method keeps its own
  const tv = L.body({ ...f, method: "transaction_value" });
  assert.equal(tv.transaction_value, "999");
  assert.equal("net_cost" in tv, false);
  const none = L.body({ ...f, method: "" });
  assert.equal("net_cost" in none || "transaction_value" in none, false);
});

test("no silent coercion: separators, signs, exponents and words are named, not fixed", () => {
  for (const bad of ["1,250", "$10", "-5", "1e3", "1.", ".5", "01", "ten", "NaN", "1".repeat(31), "1.1234567890123"]) {
    const e = L.check({ ...L.blankForm("8202.31"), method: "transaction_value", tv: bad });
    assert.ok(e.amount, bad);
  }
  for (const ok of ["0", "10", "1250.50", "0.25", "1".repeat(30), "1.123456789012"]) assert.equal(L.check({ ...L.blankForm("8202.31"), method: "net_cost", nc: ok }).amount, undefined, ok);
  // an invalid amount for the other method is not in play
  assert.equal(L.check({ ...L.blankForm("8202.31"), method: "net_cost", tv: "1,0", nc: "5" }).amount, undefined);
  assert.equal(L.check({ ...L.blankForm("82"), }).hs, copy.usmcaCheck.badHs);
  assert.equal(L.check(L.blankForm("8202.31.00")).hs, undefined);
  const e = L.check({ ...L.blankForm("8202.31"), materials: [{ hs: "8202.3x", origin: "unknown" }, { hs: "", origin: "unknown" }] });
  assert.ok(e.materials[0]);
  assert.equal(e.materials[1], undefined);
  assert.equal(L.keyOf({ ...L.blankForm("8202.31"), method: "net_cost", nc: "1,0" }), null);
});

test("the simple-goods wording names every provision the tick covers", () => {
  const s = copy.usmcaCheck.simple.toLowerCase();
  for (const w of ["sets", "accessories or packing", "fungible", "intermediate", "cost averaging", "de minimis", "outside-party", "special valuation", "parts route", "accumulation", "remanufactured", "automotive"]) assert.ok(s.includes(w), w);
  assert.match(s, /without using these special provisions/);
  const v = [copy.usmcaCheck.vnm, copy.usmcaCheck.complete].join(" ");
  assert.equal((v.match(/unknown origin/g) ?? []).length, 2);
});

test("the simple-goods tick is the one place the unmodeled provisions are set, all not applicable", () => {
  const p = L.body({ ...L.blankForm("9503.00"), simple: true, complete: true }).profile;
  assert.equal(p.simple_goods_confirmed, true);
  assert.equal(p.materials_list_complete, true);
  for (const k of ["sets", "accessories_packing", "fungibles", "intermediate_materials", "cost_averaging", "de_minimis", "outside_party_operations", "special_valuation", "parts_route"]) assert.equal(p[k], "not_applicable", k);
});

test("materials: blank code is unknown, the origin answer is sent as chosen", () => {
  const b = L.body({ ...L.blankForm("8202.31"), materials: [{ hs: "8202.39", origin: "non_originating" }, { hs: "", origin: "unknown" }] });
  assert.deepEqual(b.materials, [{ hs: "8202.39", originating: "non_originating" }, { originating: "unknown" }]);
});

test("the value fields appear only when they can change the answer", () => {
  assert.equal(L.needsValue(L.blankForm("8202.31"), null), false);
  assert.equal(L.needsValue(L.blankForm("8202.31"), { missing_facts: [{ field: "profile.sets" }] }), false);
  assert.equal(L.needsValue(L.blankForm("8202.31"), { missing_facts: [{ field: "rvc_method" }] }), true);
  assert.equal(L.needsValue({ ...L.blankForm("8202.31"), vnm: "5" }, null), true);
});

test("missing facts read as plain sentences, once each, naming the material", () => {
  const facts = [
    { code: "profile_unconfirmed", field: "profile.sets", message: "Confirm whether sets applies; it is not supported by this check." },
    { code: "profile_unconfirmed", field: "profile.fungibles", message: "x" },
    { code: "origin_status_unknown", field: "materials[1].originating", message: "State whether this material is originating or non-originating." },
    { code: "future_code", field: "x", message: "Server sentence." },
  ];
  const out = L.factList(facts);
  assert.deepEqual(out, ["Confirm the simple-goods statement.", "Material 2: Say whether it has USMCA origin.", "Server sentence."]);
});

test("only the newest check may land", () => {
  const s = L.sequencer();
  const a = s.start();
  const b = s.start();
  assert.equal(a.signal.aborted, true);
  assert.equal(s.live(a.id), false);
  assert.equal(s.live(b.id), true);
  s.retire(); // an edit
  assert.equal(s.live(b.id), false);
  assert.equal(b.signal.aborted, true);
});

const source = { data_version: "pilot", snapshot: { source: "HTSUS GN 11(o)", revision: "Rev 23", retrieved_on: "2026-11-24", source_hash: "ab" }, row: { id: "r", code: "8202.31", description: "Saw blades", source: { citation: "GN11(o) ch82", excerpt: "A change to subheading 8202.31 from any other subheading." } } };
const base = { destination: "US", regime: "usmca", good_hs: "820231", source, limits: ["Scope statement."] };

test("met: plain status, the label, the exact calculation and a rounding note", () => {
  const t = text(
    html(UsmcaResult, {
      res: { ...base, status: "met", rvc: { method: "transaction_value", status: "computed", exact: "200/3", display: "66.67", display_note: "Rounded for display only." }, alternatives: [{ id: "a", label: "Value", result: "satisfied", conditions: [] }] },
    }),
  );
  assert.match(t, /Rule met/);
  assert.match(t, /Selected origin rule only/);
  assert.match(t, /66\.67%/);
  assert.match(t, /exact 200\/3%/);
  assert.match(t, /Rounded for display only/);
  assert.match(t, /Source and scope/);
});

test("not met says it concerns the selected rule's alternatives only", () => {
  const t = text(html(UsmcaResult, { res: { ...base, status: "not_met" } }));
  assert.match(t, /Rule not met/);
  assert.match(t, /Only the alternatives of the selected rule were checked/);
  assert.match(t, /does not say the good lacks USMCA origin/);
});

test("needs information lists what is missing; not supported says not covered yet", () => {
  const n = html(UsmcaResult, { res: { ...base, status: "needs_information", missing_facts: [{ code: "rvc_method_required", field: "rvc_method", message: "m" }] } });
  assert.match(text(n), /More facts needed/);
  assert.match(text(n), /Choose a value method/);
  assert.ok(n.includes("data-usmca-needs"));
  const u = text(html(UsmcaResult, { res: { ...base, good_hs: "6109", status: "not_supported", unsupported_reasons: ["the good is not covered by the rule data snapshot"] } }));
  assert.match(u, /Not covered yet/);
  assert.match(u, /8202\.31, 8207\.13 and 9503\.00 only/);
});

test("an undefined denominator is not shown as a percentage", () => {
  const t = text(html(UsmcaResult, { res: { ...base, status: "needs_information", rvc: { method: "net_cost", status: "undefined_denominator", problems: [] }, missing_facts: [{ code: "rvc_undefined", field: "net_cost", message: "m" }] } }));
  assert.match(t, /Cannot be calculated/);
  assert.doesNotMatch(t, /%/);
});

test("a result for older facts is hidden: only the request to check again shows", () => {
  const m = html(UsmcaResult, { res: { ...base, status: "met", rvc: { method: "net_cost", status: "computed", exact: "1", display: "1.00" } }, stale: true });
  assert.equal(text(m), "Facts changed. Check again.");
  assert.doesNotMatch(m, /opacity|Rule met|data-usmca=/);
});

test("a whole-number percentage is not repeated as exact; a fraction is", () => {
  assert.equal(L.exactNote({ exact: "60", display: "60.00" }), null);
  assert.equal(L.exactNote({ exact: "200/3", display: "66.67" }), "200/3");
  assert.equal(L.exactNote({ exact: "125/2", display: "62.50" }), "125/2");
  assert.equal(L.exactNote({ exact: "1/3", display: "33.33" }), "1/3");
});

const deferred = () => {
  let resolve, reject;
  const p = new Promise((a, b) => ((resolve = a), (reject = b)));
  return { p, resolve, reject };
};
const tick = () => new Promise((r) => setTimeout(r, 0));

test("a slow earlier answer never overwrites a newer check or an edit", async () => {
  const r = L.runner();
  const seen = [];
  const h = { busy: (b) => seen.push(["busy", b]), done: (x) => seen.push(["done", x]), fail: (e) => seen.push(["fail", e.message]) };
  const a = deferred();
  const b = deferred();
  const sigs = [];
  r.run((s) => (sigs.push(s), a.p), h);
  r.run((s) => (sigs.push(s), b.p), h);
  assert.equal(sigs[0].aborted, true);
  b.resolve("new");
  await tick();
  a.resolve("old");
  await tick();
  assert.deepEqual(seen.filter(([k]) => k !== "busy"), [["done", "new"]]);
  // an edit retires a check in flight: nothing lands, not even a failure
  const c = deferred();
  const seen2 = [];
  r.run(() => c.p, { busy: () => {}, done: (x) => seen2.push(x), fail: (e) => seen2.push(e.message) });
  r.retire();
  c.reject(new Error("aborted"));
  await tick();
  assert.deepEqual(seen2, []);
});

test("the form shell is static markup, so the browser pass covers the live wiring", () => {
  // The repo's component tests render to static markup (no DOM). The ordering
  // rules above are tested on the runner the component uses; focus and
  // aria-describedby are checked in the browser pass.
  const m = html(UsmcaCheck, { c: { id: "c", product: { hts: "8202.31" } } });
  assert.match(m, /id="usmca-hs"/);
  assert.doesNotMatch(m, /role="alert"/);
});

test("the panel starts closed to questions: code, materials, two confirmations; no value fields", () => {
  const m = html(UsmcaCheck, { c: { id: "c", product: { hts: "8202.31.00" } } });
  const t = text(m);
  assert.match(t, /USMCA origin-rule check/);
  assert.ok(m.includes('value="8202.31.00"'));
  assert.ok(!m.includes("data-usmca-value"));
  assert.equal((m.match(/type="checkbox"/g) ?? []).length, 2);
  assert.doesNotMatch(m, /checked=""/);
  assert.match(m, /min-h-11/);
});

test("our own wording avoids the words the feature must not use", () => {
  const strings = JSON.stringify(copy.usmcaCheck, (_k, v) => (typeof v === "function" ? v(1) : v));
  assert.doesNotMatch(strings, BANNED);
  const m = text(html(UsmcaCheck, { c: { id: "c", product: { hts: "8202.31" } } }));
  assert.doesNotMatch(m, BANNED);
  const r = text(html(UsmcaResult, { res: { ...base, status: "not_met", rvc: { method: "net_cost", status: "computed", exact: "1", display: "1.00", display_note: "n" } } }));
  assert.doesNotMatch(r.replace(/Scope statement\./, ""), BANNED);
});
