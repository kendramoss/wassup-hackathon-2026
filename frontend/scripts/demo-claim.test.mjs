import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { test } from "node:test";

// #558 (22 Nov 2026): pinned heat gun demo is live-seeded (case_000e08240f23).
// Copy must reflect live screen (China needs review, best plan 0.94 to Thailand) and must not
// promise a recorded 18 Oct 0.96 plan on the current pinned case.
const read = (relative) => readFileSync(fileURLToPath(new URL(relative, import.meta.url)), "utf8");

const copy = read("../src/lib/copy.ts");
const landing = read("../src/components/op/landing/landing.tsx");
const demoDoc = read("../../docs/DEMO.md");

test("landing and copy do not claim the old 0.96 pinned plan", () => {
  assert.doesNotMatch(copy, /pinned demo on this site shows the same plan at 0\.96/i);
  assert.doesNotMatch(landing, /0\.96/);
});

test("demo doc describes the live pinned heat gun flow", () => {
  assert.match(demoDoc, /pinned heat gun demo is `case_000e08240f23`/i);
  assert.match(demoDoc, /precedent check \*\*0\.94\*\*/i);
});
