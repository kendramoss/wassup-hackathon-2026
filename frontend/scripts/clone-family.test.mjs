import assert from "node:assert/strict";
import { test } from "node:test";
import { clonedFamily } from "../src/lib/clone-family.ts";

// #298: cloning must preserve the source family for every supported template.
for (const family of ["motorised_tools_small_appliances", "led_lights", "lidar_modules"]) {
  test(`clone preserves top-level ${family}`, () => {
    assert.equal(clonedFamily({ family, product: { family: "ignored_when_top_level_set" } }), family);
  });
}

test("clone falls back to product.family only when top-level family is absent", () => {
  assert.equal(clonedFamily({ product: { family: "lidar_modules" } }), "lidar_modules");
  assert.equal(clonedFamily({ family: "", product: { family: "led_lights" } }), "led_lights");
});

test("clone with no family returns undefined (caller keeps its default)", () => {
  assert.equal(clonedFamily({}), undefined);
  assert.equal(clonedFamily({ family: "", product: {} }), undefined);
});
