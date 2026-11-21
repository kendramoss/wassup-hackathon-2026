import "./render-support.mjs";
import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import { test } from "node:test";
import { families, groupName, groupNames, groupSuggestions, planTemplateVersion, versionBefore } from "../src/lib/families.ts";

// The family picker must list exactly the planner's templates (D138), with
// their group ids and names, so a family the backend plans is never missing
// from the UI and no group is labelled with a bare id (#391).
const dir = new URL("../../backend/internal/planner/templates/", import.meta.url);
const templates = readdirSync(dir)
  .filter((f) => f.endsWith(".json"))
  .map((f) => JSON.parse(readFileSync(new URL(f, dir), "utf8")));

test("the picker lists every template family once", () => {
  assert.deepEqual(families.map((f) => f.id).sort(), templates.map((t) => t.family).sort());
});

test("each family carries its template's group ids and names (#391)", () => {
  for (const t of templates) {
    const f = families.find((x) => x.id === t.family);
    assert.deepEqual(f.groups, t.groups.map((g) => ({ id: g.id, name: g.name, ...(g.former ? { former: g.former } : {}) })), t.family);
  }
});

test("group suggestions follow the chosen family", () => {
  assert.deepEqual(groupSuggestions("water_filters"), ["carbon_block", "cartridge_housings", "dispenser_electronics", "final_assembly"]);
  const all = groupSuggestions();
  assert.ok(all.includes("pcba") && all.includes("carbon_block") && all.includes("final_assembly"));
  assert.equal(new Set(all).size, all.length);
});

test("group labels match the template even when an id is reused (#391)", () => {
  for (const t of templates) {
    for (const g of t.groups) {
      assert.equal(groupName(g.id, t.family), g.name, `${t.family}/${g.id}`);
    }
  }
  assert.equal(groupName("final_assembly"), "Final assembly");
  assert.equal(groupName("final_assembly", "motors_pumps_compressors"), "Final assembly and testing");
  assert.equal(groupName("pcba_manufacturing"), "PCBA (SMT)");
  assert.equal(groupName(undefined), "Other");
  assert.equal(Object.keys(groupNames).length, new Set(templates.flatMap((t) => t.groups.map((g) => g.id))).size);
});

test("move phrases come from template names, not a hard-coded legacy map (#391)", async () => {
  const { movePhrase } = await import("../src/components/op/plan-bits.tsx");
  const one = (group, family) => movePhrase({ move: [{ group, to_plant: "tw" }] }, family);
  assert.equal(one("pcba_manufacturing"), "the PCBA (SMT)");
  assert.equal(one("pcba"), "the PCBA (SMT)");
  assert.equal(one("led_pcba"), "the LED PCBA (SMT)");
  assert.equal(one("final_assembly"), "the final assembly");
  assert.equal(one("final_assembly", "motors_pumps_compressors"), "the final assembly and testing");
  assert.equal(
    movePhrase({
      move: [
        { group: "lid_and_firebox", to_plant: "tw" },
        { group: "burners_and_gas_train", to_plant: "tw" },
        { group: "cooking_surface_frame", to_plant: "tw" },
      ],
    }),
    "the lid and firebox forming, the burners and gas train and the cooking surface, panels and frame",
  );
});

test("a group split later keeps its former name only for a plan made before the split (D342, D372)", () => {
  const fam = "motorised_tools_small_appliances";
  const pre = { components: [{ name: "switch", group: "switch_cord_minor_parts" }, { name: "power cord", group: "switch_cord_minor_parts" }] };
  const post = { components: [{ name: "switch", group: "switch_cord_minor_parts" }, { name: "power cord", group: "cord_knobs_covers" }] };
  // The pinned heat gun: planned with template version 1, before the split.
  assert.equal(groupName("switch_cord_minor_parts", fam, pre, "1"), "Switch, cord and minor parts");
  // The same parts planned now, or with no plan, read the new name.
  assert.equal(groupName("switch_cord_minor_parts", fam, pre, "2"), "Switch, wiring and minor parts");
  assert.equal(groupName("switch_cord_minor_parts", fam, pre), "Switch, wiring and minor parts");
  assert.equal(groupName("switch_cord_minor_parts", fam, post, "1"), "Switch, wiring and minor parts");
  assert.equal(groupName("switch_cord_minor_parts", fam), "Switch, wiring and minor parts");
  assert.equal(groupName("pcba", fam, pre, "1"), "PCBA (SMT)");
});

// #533: motorised products with no cord, knobs or covers, a cordless tool
// included, read "Switch, wiring and minor parts" when planned today.
test("products without a cord read the new group name (#533)", () => {
  const fam = "motorised_tools_small_appliances";
  const cordless = { components: [{ name: "trigger switch", group: "switch_cord_minor_parts" }, { name: "battery pack", group: "pcba" }] };
  const stove = { components: [{ name: "wiring, switches and connectors", group: "switch_cord_minor_parts" }] };
  for (const p of [cordless, stove]) assert.equal(groupName("switch_cord_minor_parts", fam, p, "2"), "Switch, wiring and minor parts");
  assert.equal(groupName("switch_cord_minor_parts", fam, cordless), "Switch, wiring and minor parts");
});

test("the plan's template version comes from the stored plan", () => {
  const fam = "motorised_tools_small_appliances";
  assert.equal(planTemplateVersion({ plan: { family: fam, template_versions: { [fam]: "1" } } }), "1");
  assert.equal(planTemplateVersion({ product: { family: fam }, plan: { template_versions: { [fam]: "2" } } }), "2");
  assert.equal(planTemplateVersion({ product: { family: fam } }), undefined);
  assert.equal(versionBefore("1", "2"), true);
  assert.equal(versionBefore("2", "2"), false);
  assert.equal(versionBefore(undefined, "2"), false);
});
