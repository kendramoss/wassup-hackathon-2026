import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { test } from "node:test";

// Judge panel round 16, item 5: copy residue that sat in the repo but not on
// the live pages.
const read = (relative) =>
  readFileSync(fileURLToPath(new URL(relative, import.meta.url)), "utf8");

test("the recorded Part 102 page renders its note, which carries the D640 caveat", () => {
  const ex = JSON.parse(read("../src/lib/part102-plan-example.json"));
  assert.match(ex.note, /D640/);
  const page = read("../src/components/op/screens/part102-plan-example.tsx");
  assert.match(page, /ex\.note\.split/);
});

test("the landing's preparation time is measured, with its sources in NUMBERS", () => {
  const copy = read("../src/lib/copy.ts");
  assert.doesNotMatch(copy, /estimated, not measured/);
  assert.match(copy, /after: "33 s"/);
  const heat = JSON.parse(read("../../eval/results/plan-heatgun-2022-local-litellm.json"));
  assert.equal(Math.round(heat.stats.wall_seconds), 33);
  const live = JSON.parse(read("../../eval/results/families-r3-live-checks.json"));
  assert.equal(Math.round(live.wall_seconds_median), 49);
  assert.equal(Math.round(live.wall_seconds_max), 101);
  const numbers = read("../../docs/NUMBERS.md");
  assert.match(numbers, /\| First answer \| 33 s \(median 49 s, slowest 101 s/);
});

test("/cases names every family demo picked by family", () => {
  const copy = read("../src/lib/copy.ts");
  assert.match(copy, /ten family demos were picked by family/, "copy names all ten");
  for (const family of [
    "vehicle parts",
    "PCBA devices",
    "power supplies",
    "medical devices",
    "paper and tissue products",
    "off-road vehicles and machinery",
    "semiconductor devices",
    "golf and sporting goods",
    "toys and build kits",
    "heat pumps and heaters",
  ]) {
    assert.ok(copy.includes(family), `copy names ${family}`);
  }
});

test("/cases counts every per-family check, D171's and D661's", () => {
  const d171 = JSON.parse(read("../../eval/results/planner-families-summary.json")).runs;
  const d661 = JSON.parse(read("../../eval/results/families-r3-live-checks.json")).runs;
  const n = d171.length + d661.length;
  const off =
    d171.filter((r) => r.engine_current !== r.cbp_held).length +
    d661.filter((r) => r.engine_current_origin !== r.cbp_current_origin).length;
  assert.equal(n, 29);
  assert.equal(off, 8);
  const copy = read("../src/lib/copy.ts");
  assert.ok(copy.includes(`Across all ${n} per-family checks`), "copy names the count");
  assert.ok(copy.includes(`on the current setup in ${off}.`), "copy names the disagreements");
});
