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

test("a response for another case does not overwrite the current one", () => {
  const out = run(`
    const merged = mergeEconomics(fullCase, { case_id: "other", economics: { annual_import_value_usd: 7 } });
    console.log(JSON.stringify(merged));
  `);
  assert.equal(out.economics.annual_import_value_usd, 100);
  assert.equal(out.economics_priced.duty_rates_note, "stale");
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
