// The market picker (D680): the codes are the API's, only supported markets
// can be picked, and the form sends a market only once the user picks one.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import "./render-support.mjs"; // resolves the app's extensionless imports

const { markets, marketOf } = await import("../src/lib/market.ts");
const { copy } = await import("../src/lib/copy.ts");

test("the market codes are the API's JurisdictionCode enum, United States first", async () => {
  const spec = await readFile(new URL("../../docs/api/openapi.yaml", import.meta.url), "utf8");
  const i = spec.indexOf("    JurisdictionCode:");
  const enumLine = spec.slice(i).match(/enum: \[([^\]]+)\]/)[1];
  assert.deepEqual(markets.map((m) => m.code), enumLine.split(", "));
  assert.equal(markets[0].code, "us");
  assert.equal(marketOf(undefined).code, "us", "a case made before D680 reads as U.S.");
});

test("every market but other has a flag the bundle copies", async () => {
  const src = await readFile(new URL("../src/lib/countries.ts", import.meta.url), "utf8");
  for (const m of markets.filter((x) => x.code !== "other")) assert.ok(src.includes(`"${m.flag}"`) || src.includes(`${m.flag}:`), m.flag);
});

test("the picker lists only supported markets, with no 'soon' labels", async () => {
  const src = await readFile(new URL("../src/components/op/market-picker.tsx", import.meta.url), "utf8");
  assert.match(src, /\.filter\(\(x\) => supported\.includes\(x\.code\)\)/);
  assert.doesNotMatch(src, /soon/);
  assert.equal(copy.market.soon, undefined);
  assert.equal(copy.market.unsupported("the European Union"), "Origin rules for the European Union aren't supported yet.");
});

test("the new-case form sends a market only once the user picks one", async () => {
  const src = await readFile(new URL("../src/components/op/screens/product-step.tsx", import.meta.url), "utf8");
  assert.match(src, /jurisdiction: marketPicked \? market : undefined/);
});

test("the result screen shows no verdict for an unsupported market and offers the U.S. question", async () => {
  const src = await readFile(new URL("../src/components/op/screens/origin-step.tsx", import.meta.url), "utf8");
  assert.match(src, /jurisdiction_detail\?\.supported === false\) return <UnsupportedMarket/);
  assert.match(src, /jurisdiction: "us"/);
});

// #733: a compact supported-markets summary beside the picker, consistent
// with the Japan availability line the picker already uses.
test("the picker carries a short summary of what each market offers today", async () => {
  const { html, text } = await import("./render-support.mjs");
  const { MarketPicker } = await import("../src/components/op/market-picker.tsx");
  const t = text(html(MarketPicker, { id: "m", value: "us", onChange() {} }));
  assert.match(t, /U\.S\.: live/);
  assert.match(t, /Canada: rules check/);
  assert.match(t, /EU, UK: rules check, listed goods/);
  assert.match(t, /South Korea, Australia: RCEP check/);
  assert.doesNotMatch(t, /Japan:/, "Japan's line shows only once the API lists Japan");
  assert.doesNotMatch(t, /not yet|soon/);
  assert.equal(copy.market.summary.length, 5);
  for (const line of copy.market.summary) assert.ok(line.length <= 40, line);
});
