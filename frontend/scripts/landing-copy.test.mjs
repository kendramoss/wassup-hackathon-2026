import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const screen = (file) => readFileSync(new URL(`../src/components/op/landing/${file}`, import.meta.url), "utf8");
const landing = screen("landing.tsx");
const proof = screen("judge-proof.tsx");
const tabs = screen("judge-tabs.tsx");

test("the heat-gun plan links to its before, comparison, and after CBP rulings", () => {
  for (const id of ["N328954", "N345938", "N357771"]) {
    assert.ok(landing.includes(`<VerificationPill id="${id}"`), id);
  }
  assert.match(landing, /Not a blind test/);
});

test("the landing keeps plan scope, human review, and financial provenance beside their claims", () => {
  assert.match(proof, /with the starting origin given/);
  assert.match(proof, /20\.4% routed to human review/);
  assert.match(proof, /\/accuracy#misses/);
  assert.match(proof, /Recorded heat-gun plan search/);
  assert.match(proof, /Model cost per prediction at promo rates/);
  assert.match(proof, /Illustrative unit-economics case study/);
});

test("the developer tab uses the hosted MCP endpoint", () => {
  assert.match(tabs, /gemini mcp add --transport http unde https:\/\/undelabs\.dev\/mcp/);
});
