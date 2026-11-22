// Plan, Draft and Scenarios are U.S. steps (#678): the case tabs follow the API's
// jurisdiction_detail.follow_ups, not a list of market codes in the UI.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { html, text } from "./render-support.mjs";

const { CaseTabs, caseTabs } = await import("../src/components/op/chrome.tsx");
const { hasFollowUps } = await import("../src/lib/market.ts");
const { copy } = await import("../src/lib/copy.ts");

const usCase = { jurisdiction: "us", jurisdiction_detail: { code: "us", name: "United States", source: "chosen", supported: true, follow_ups: true } };
const caCase = { jurisdiction: "ca", jurisdiction_detail: { code: "ca", name: "Canada", source: "chosen", supported: true, follow_ups: false } };
const euCase = { jurisdiction: "eu", jurisdiction_detail: { code: "eu", name: "European Union", source: "detected", supported: false, follow_ups: false } };

const tabs = (c, current = "origin") =>
  text(html(CaseTabs, { current, reached: 2, title: "t", followUps: hasFollowUps(c) }));

test("a U.S. case shows the Plan and Draft tabs", () => {
  assert.equal(hasFollowUps(usCase), true);
  const t = tabs(usCase);
  assert.match(t, /3 Plan/);
  assert.match(t, /4 Draft/);
  assert.match(t, /Scenarios/);
  assert.doesNotMatch(t, /U\.S\. only/);
});

test("a Canada or EU case hides them and says so in few words", () => {
  for (const c of [caCase, euCase]) {
    assert.equal(hasFollowUps(c), false, c.jurisdiction);
    const t = tabs(c);
    assert.doesNotMatch(t, /\bPlan\b/, c.jurisdiction);
    assert.doesNotMatch(t, /\bDraft\b/, c.jurisdiction);
    assert.doesNotMatch(t, /Scenarios/, c.jurisdiction);
    assert.match(t, /Origin/);
    assert.ok(t.includes(copy.market.followUpsUSOnly), c.jurisdiction);
  }
  assert.ok(copy.market.followUpsUSOnly.split(" ").length <= 7, "rule 16: few words");
});

test("the tab being viewed stays, so a direct link still has its tab", () => {
  assert.deepEqual(caseTabs(false, "plan").map((t) => t.key), ["product", "origin", "plan"]);
  assert.deepEqual(caseTabs(true, "origin").map((t) => t.key), ["product", "origin", "plan", "draft", "scenarios"]);
});

test("the flag comes from the API, with a fallback for cases read before it", async () => {
  assert.equal(hasFollowUps({}), true, "a case before D680 reads as U.S.");
  assert.equal(hasFollowUps({ jurisdiction_detail: { supported: false } }), false);
  const shell = await readFile(new URL("../src/components/op/case-shell.tsx", import.meta.url), "utf8");
  assert.match(shell, /followUps=\{data \? hasFollowUps\(data\) : true\}/);
  assert.match(shell, /origin_country && hasFollowUps\(data\) \? \(\) => setAsking\(true\)/, "Ask is a U.S. step too");
  const market = await readFile(new URL("../src/lib/market.ts", import.meta.url), "utf8");
  assert.doesNotMatch(market.slice(market.indexOf("export function hasFollowUps")), /"us"|"ca"/, "no hard-coded market list");
});
