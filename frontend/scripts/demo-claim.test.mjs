import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { test } from "node:test";

// Issue #411: the landing's 0.96 is a recorded 18 Oct production run, not the
// answer of the currently pinned demo. If either side changes, this test fails
// so the stage copy cannot silently drift into a self-contradiction again.
const read = (relative) =>
  readFileSync(fileURLToPath(new URL(relative, import.meta.url)), "utf8");

const copy = read("../src/lib/copy.ts");
const landing = read("../src/components/op/landing/landing.tsx");
const demoDoc = read("../../docs/DEMO.md");
const seed = JSON.parse(read("../../eval/results/demo-seed-vertex-prod.json"));
const heatgun = seed.find((row) => row.demo === "heatgun-2022");

test("the pinned production seed is below the bar", () => {
  assert.ok(heatgun, "demo seed must contain heatgun-2022");
  assert.equal(heatgun.plan_status, "below_bar");
  assert.match(heatgun.best_plan, /0\.75/);
});

test("every landing 0.96 plan copy is labelled as a recorded 18 Oct run", () => {
  for (const [name, source] of [
    ["copy.ts", copy],
    ["landing.tsx", landing],
  ]) {
    const lines = source.split(/\r?\n/).filter((line) => line.includes("0.96"));
    assert.ok(lines.length > 0, `${name} should still carry the recorded plan`);
    for (const line of lines) {
      assert.match(
        line,
        /18 Oct/i,
        `${name}: every 0.96 plan line must say it is a recorded 18 Oct run`,
      );
    }
    assert.match(source, /may (?:instead )?show no plan above the bar/i);
  }
});

test("the demo script has one canonical branch for the below-bar screen", () => {
  assert.match(demoDoc, /Plan screen: one canonical instruction, two branches/);
  assert.match(demoDoc, /If the Plan screen shows a plan/);
  assert.match(
    demoDoc,
    /If it shows no plan above the bar \(the current pinned seed\)/,
  );
  assert.match(demoDoc, /recorded 18 Oct(?: 2026)? production run/i);
});
