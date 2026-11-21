import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import { test } from "node:test";
import { families, groupNames, groupSuggestions } from "../src/lib/families.ts";

// The family picker must list exactly the planner's templates (D138), with
// their group ids, so a family the backend plans is never missing from the UI.
const dir = new URL("../../backend/internal/planner/templates/", import.meta.url);
const templates = readdirSync(dir)
  .filter((f) => f.endsWith(".json"))
  .map((f) => JSON.parse(readFileSync(new URL(f, dir), "utf8")));

test("the picker lists every template family once", () => {
  assert.deepEqual(families.map((f) => f.id).sort(), templates.map((t) => t.family).sort());
});

test("group ids match each template", () => {
  for (const t of templates) {
    const f = families.find((x) => x.id === t.family);
    assert.deepEqual(f.groups, t.groups.map((g) => g.id), t.family);
  }
});

test("group suggestions follow the chosen family", () => {
  assert.deepEqual(groupSuggestions("water_filters"), ["carbon_block", "cartridge_housings", "dispenser_electronics", "final_assembly"]);
  const all = groupSuggestions();
  assert.ok(all.includes("pcba") && all.includes("carbon_block") && all.includes("final_assembly"));
  assert.equal(new Set(all).size, all.length);
});

test("group names match each template, so a plan never shows a bare id (D270)", () => {
  for (const t of templates) {
    for (const g of t.groups) {
      if (g.id === "final_assembly") continue;
      assert.equal(groupNames[g.id], g.name, `${t.family}/${g.id}`);
    }
  }
  assert.equal(groupNames.final_assembly, "Final assembly");
  assert.equal(Object.keys(groupNames).length, new Set(templates.flatMap((t) => t.groups.map((g) => g.id))).size);
});
