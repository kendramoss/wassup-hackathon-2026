import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

// #291: PUT /cases/{id}/economics returns only {case_id, economics}. The
// client must type it that way and the money save must merge it into the
// current case instead of replacing the case. client.ts uses TypeScript
// parameter properties, so it runs in a child node with type transforms.
const client = fileURLToPath(new URL("../src/lib/api/client.ts", import.meta.url));

function run(body) {
  const code = `
    import { api, mergeEconomics } from ${JSON.stringify(client)};
    const calls = [];
    globalThis.fetch = async (url, init) => {
      calls.push({ url, method: init.method, body: init.body ? JSON.parse(init.body) : undefined });
      return new Response(JSON.stringify(globalThis.__reply), { status: 200, headers: { "content-type": "application/json" } });
    };
    const fullCase = {
      id: "c1", display_name: "Heat gun", facts: "Made in China", pinned: false,
      product: { name: "Heat gun", components: [{ name: "housing", group: "housing", plant: "p1", made_in: "CN" }], plants: [{ id: "p1", country: "CN" }, { id: "p2", country: "VN" }] },
      prediction: { origin_country: "CN", confidence: 0.8 },
      scenarios: [{ label: "As today" }],
      economics: { annual_import_value_usd: 100, duty_rates: { CN: 0.25 } },
      economics_priced: { annual_import_value_usd: 100, duty_rates: { CN: 0.25 }, duty_rates_note: "stale" },
    };
    ${body}
  `;
  const r = spawnSync(process.execPath, ["--experimental-transform-types", "--no-warnings", "--input-type=module", "-e", code], { encoding: "utf8" });
  assert.equal(r.status, 0, r.stderr || r.stdout);
  return JSON.parse(r.stdout);
}

test("putEconomics response is the partial {case_id, economics}, merged without losing the case", () => {
  const out = run(`
    globalThis.__reply = { case_id: "c1", economics: { annual_import_value_usd: 500, duty_rates: { CN: 0.1, VN: 0.05 } } };
    const resp = await api.putEconomics("c1", globalThis.__reply.economics);
    const merged = mergeEconomics(fullCase, resp);
    console.log(JSON.stringify({ resp, merged, calls }));
  `);
  assert.deepEqual(Object.keys(out.resp).sort(), ["case_id", "economics"]);
  assert.equal(out.calls[0].method, "PUT");
  const m = out.merged;
  assert.equal(m.id, "c1");
  assert.equal(m.display_name, "Heat gun");
  assert.equal(m.facts, "Made in China");
  assert.equal(m.product.plants.length, 2);
  assert.equal(m.prediction.origin_country, "CN");
  assert.equal(m.scenarios.length, 1);
  assert.equal(m.economics.annual_import_value_usd, 500);
  assert.equal(m.economics.duty_rates.VN, 0.05);
});

test("merged case drops the stale priced economics so old rates are not shown", () => {
  const out = run(`
    const merged = mergeEconomics(fullCase, { case_id: "c1", economics: { annual_import_value_usd: 7 } });
    console.log(JSON.stringify(merged));
  `);
  assert.equal("economics_priced" in out, false);
});

test("a response for another case throws instead of silently succeeding", () => {
  const out = run(`
    let err = null;
    try { mergeEconomics(fullCase, { case_id: "other", economics: {} }); } catch (e) { err = e.message; }
    console.log(JSON.stringify({ err }));
  `);
  assert.match(out.err, /different case/);
});

// saveEconomics is the logic saveMoney runs (scenarios-view.tsx). These drive
// it with controlled promises; they are not React render tests.
const flow = (body) => run(`
  import { saveEconomics } from ${JSON.stringify(client)};
  const gate = () => { let go; const p = new Promise((r) => (go = r)); return { p, go }; };
  let state = fullCase;
  const io = { latest: () => state, apply: (c) => { state = c; } };
  const priced = { annual_import_value_usd: 500, duty_rates: { CN: 0.1 }, duty_rates_note: "USITC fetched 2026-11-01" };
  const serverCase = (extra = {}) => ({ ...fullCase, economics: { annual_import_value_usd: 500, duty_rates: { CN: 0.1 } }, economics_priced: priced, ...extra });
  const scenarioDone = { ...fullCase, scenarios: [{ label: "As today" }, { label: "Moved" }] };
  ${body}
`.replace(/^\s*import \{ api, mergeEconomics \}.*$/m, ""));

test("save keeps backend-priced economics: applies the re-read case", () => {
  const out = flow(`
    globalThis.fetch = async (url, init) => new Response(JSON.stringify(init.method === "PUT" ? { case_id: "c1", economics: serverCase().economics } : serverCase()), { status: 200 });
    await saveEconomics("c1", serverCase().economics, io);
    console.log(JSON.stringify(state));
  `);
  assert.equal(out.economics_priced.duty_rates_note, "USITC fetched 2026-11-01");
  assert.equal(out.product.name, "Heat gun");
});

test("a scenario run finishing during the save is not overwritten by a stale re-read", () => {
  const out = flow(`
    const getGate = gate();
    let gets = 0;
    globalThis.fetch = async (url, init) => {
      if (init.method === "PUT") return new Response(JSON.stringify({ case_id: "c1", economics: serverCase().economics }));
      gets++;
      if (gets === 1) { await getGate.p; return new Response(JSON.stringify(serverCase())); } // predates the run: 1 scenario
      return new Response(JSON.stringify(serverCase({ scenarios: scenarioDone.scenarios })));
    };
    const saving = saveEconomics("c1", serverCase().economics, io);
    await new Promise((r) => setTimeout(r, 10));
    state = scenarioDone; // run() finished while the first re-read was in flight
    getGate.go();
    await saving;
    console.log(JSON.stringify({ state, gets }));
  `);
  assert.equal(out.state.scenarios.length, 2);
  assert.equal(out.state.economics_priced.duty_rates_note, "USITC fetched 2026-11-01");
  assert.equal(out.gets, 2);
});

test("a scenario run finishing during the PUT survives when the re-read fails", () => {
  const out = flow(`
    const putGate = gate();
    globalThis.fetch = async (url, init) => {
      if (init.method === "PUT") { await putGate.p; return new Response(JSON.stringify({ case_id: "c1", economics: { annual_import_value_usd: 9 } })); }
      return new Response(JSON.stringify({ code: "internal", message: "down" }), { status: 500 });
    };
    const saving = saveEconomics("c1", { annual_import_value_usd: 9 }, io);
    state = scenarioDone;
    putGate.go();
    await saving;
    console.log(JSON.stringify(state));
  `);
  assert.equal(out.scenarios.length, 2);
  assert.equal(out.economics.annual_import_value_usd, 9);
});

test("a PUT for another case surfaces an error and leaves the case alone", () => {
  const out = flow(`
    globalThis.fetch = async () => new Response(JSON.stringify({ case_id: "zzz", economics: {} }));
    let err = null;
    try { await saveEconomics("c1", {}, io); } catch (e) { err = e.message; }
    console.log(JSON.stringify({ err, same: state === fullCase }));
  `);
  assert.match(out.err, /different case/);
  assert.equal(out.same, true);
});

test("next scenario submit sends the product and the refreshed economics", () => {
  const out = run(`
    const merged = mergeEconomics(fullCase, { case_id: "c1", economics: { annual_import_value_usd: 500 } });
    globalThis.__reply = merged;
    await api.scenarios("c1", { scenarios: [{ label: "moved", product: merged.product }], economics: merged.economics });
    console.log(JSON.stringify(calls[0]));
  `);
  assert.equal(out.body.scenarios[0].product.name, "Heat gun");
  assert.equal(out.body.economics.annual_import_value_usd, 500);
});

// Round 2: a run and a save must not overlap, and a late re-read must not
// land on another case. Controlled promises; not React render tests.
test("opLock refuses a second operation until the first releases (double action)", () => {
  const out = run(`
    import { opLock } from ${JSON.stringify(client)};
    const l = opLock();
    const a = l.start(); const b = l.start();
    a(); const c = l.start();
    console.log(JSON.stringify({ a: !!a, b: b === null, c: !!c }));
  `);
  assert.deepEqual(out, { a: true, b: true, c: true });
});

test("a save while a run is outstanding is refused, so the run cannot overwrite it", () => {
  const out = run(`
    import { opLock } from ${JSON.stringify(client)};
    const l = opLock();
    const run1 = l.start();            // run() in flight (pending promise)
    const save = l.start();            // saveMoney attempted meanwhile
    console.log(JSON.stringify({ saveRefused: save === null, runHeld: l.held }));
  `);
  assert.deepEqual(out, { saveRefused: true, runHeld: true });
});

test("a late re-read does not apply when the screen moved to another case", () => {
  const out = flow(`
    const getGate = gate();
    globalThis.fetch = async (url, init) => {
      if (init.method === "PUT") return new Response(JSON.stringify({ case_id: "c1", economics: {} }));
      await getGate.p;
      return new Response(JSON.stringify(serverCase()));
    };
    const saving = saveEconomics("c1", {}, io);
    await new Promise((r) => setTimeout(r, 10));
    const other = { ...fullCase, id: "c2", display_name: "Other" };
    state = other;
    getGate.go();
    await saving;
    console.log(JSON.stringify({ same: state === other }));
  `);
  assert.equal(out.same, true);
});

test("a re-read for the wrong case is not applied", () => {
  const out = flow(`
    globalThis.fetch = async (url, init) => new Response(JSON.stringify(init.method === "PUT" ? { case_id: "c1", economics: { annual_import_value_usd: 3 } } : { ...serverCase(), id: "c9" }));
    await saveEconomics("c1", {}, io);
    console.log(JSON.stringify(state));
  `);
  assert.equal(out.id, "c1");
  assert.equal(out.economics.annual_import_value_usd, 3);
});

// #347: the response carries the scenarios repriced with the saved money. When
// the authoritative re-read fails, the fallback merge must show those, never
// the old stored comparison; a case without scenarios keeps its own.
test("failed re-read: the repriced scenarios from the PUT replace the stale comparison", () => {
  const out = flow(`
    const stale = { ...fullCase, scenarios: [{ label: "Moved", comparison: { net_usd: 380000 } }] };
    state = stale;
    globalThis.fetch = async (url, init) => init.method === "PUT"
      ? new Response(JSON.stringify({ case_id: "c1", economics: { annual_import_value_usd: 4000000 }, scenarios: [{ label: "Moved", comparison: { net_usd: 880000 } }] }))
      : new Response(JSON.stringify({ code: "internal", message: "down" }), { status: 500 });
    await saveEconomics("c1", { annual_import_value_usd: 4000000 }, io);
    console.log(JSON.stringify(state));
  `);
  assert.equal(out.scenarios[0].comparison.net_usd, 880000);
  assert.equal(out.scenarios[0].label, "Moved");
  assert.equal(out.economics.annual_import_value_usd, 4000000);
  assert.equal("economics_priced" in out, false);
});

test("a PUT response without scenarios keeps the case's own (a case with none stays empty)", () => {
  const out = run(`
    const kept = mergeEconomics(fullCase, { case_id: "c1", economics: { annual_import_value_usd: 5 } });
    const none = mergeEconomics({ ...fullCase, scenarios: undefined }, { case_id: "c1", economics: { annual_import_value_usd: 5 } });
    console.log(JSON.stringify({ kept: kept.scenarios, none: none.scenarios ?? null }));
  `);
  assert.equal(out.kept.length, 1);
  assert.equal(out.none, null);
});
