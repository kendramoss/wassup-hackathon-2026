import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { test } from "node:test";

// Issue #411: the landing's 0.96 is a recorded 18 Oct production run. Since
// D270 the copy also says the pinned demo shows it (the pinned case's 18 Oct
// plan search is that run). If either side changes, this test fails
// so the stage copy cannot silently drift into a self-contradiction again.
const read = (relative) =>
  readFileSync(fileURLToPath(new URL(relative, import.meta.url)), "utf8");

const workflow = read("../src/components/op/landing/judge-workflow.tsx");
const demoDoc = read("../../docs/DEMO.md");
const seed = JSON.parse(read("../../eval/results/demo-seed-vertex-prod.json"));
const heatgun = seed.find((row) => row.demo === "heatgun-2022");

test("the pinned production seed is below the bar", () => {
  assert.ok(heatgun, "demo seed must contain heatgun-2022");
  assert.equal(heatgun.plan_status, "below_bar");
  assert.match(heatgun.best_plan, /0\.75/);
});

test("every landing 0.96 plan copy is labelled as a recorded 18 Oct run", () => {
  const lines = workflow.split(/\r?\n/).filter((line) => line.includes("0.96"));
  assert.ok(lines.length > 0, "the landing should still carry the recorded plan");
  for (const line of lines) assert.match(line, /18 Oct/i);
  // D270: the pinned case's plan search was re-run on 18 Oct and is the run
  // in DEMO.md section 6 (its history: "7 moves searched, 1 shown"), so the
  // landing says the pinned case shows the same plan, no longer that it may not.
  assert.doesNotMatch(workflow, /may (?:instead )?show no plan above the bar/i);
  assert.match(workflow, /pinned case shows 0\.96/);
  assert.match(demoDoc, /## 6\. Heat gun: the plan that clears the bar \(production, 18 Oct\)/);
  assert.match(demoDoc, /pinned case \(`case_878d76540b91`, updated 18 Oct\) shows the same plan at 0\.96/);
});

test("the demo script has one canonical branch for the below-bar screen", () => {
  assert.match(demoDoc, /Plan screen: one canonical instruction, two branches/);
  assert.match(demoDoc, /If the Plan screen shows a plan/);
  assert.match(
    demoDoc,
    /If it shows no plan above the bar \(an earlier pinned seed\)/,
  );
  assert.match(demoDoc, /recorded 18 Oct(?: 2026)? production run/i);
});
