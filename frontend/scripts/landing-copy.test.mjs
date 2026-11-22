import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const screen = (file) => readFileSync(new URL(`../src/components/op/landing/${file}`, import.meta.url), "utf8");
const landing = screen("landing.tsx");
const copy = readFileSync(new URL("../src/lib/copy.ts", import.meta.url), "utf8");
const proof = screen("judge-proof.tsx");
const tabs = screen("judge-tabs.tsx");

test("the heat-gun plan links to its before, comparison, and after CBP rulings", () => {
  for (const id of ["N328954", "N345938", "N357771"]) {
    assert.ok(landing.includes(`<RulingLink id="${id}"`), id);
  }
  assert.match(copy, /fig2BacktestNote: .*Not a blind test/);
});

test("the animated proof points to the accuracy page", () => {
  assert.match(landing, /className="track" data-k="track"/);
  assert.match(landing, /href="\/accuracy#plan-checks"/);
  assert.match(copy, /claimAccuracy: .*63\.4%/);
  assert.match(copy, /claimAuto: .*The rest go to a person/);
});

test("impact keeps measured and illustrative claim sources visible", () => {
  assert.match(proof, /Recorded heat-gun plan search/);
  assert.match(proof, /Model cost per prediction at promo rates/);
  assert.match(proof, /Illustrative unit-economics case study/);
});

test("the developer tab uses the hosted MCP endpoint", () => {
  assert.match(tabs, /gemini mcp add --transport http unde https:\/\/undelabs\.dev\/mcp/);
});
