import "./render-support.mjs";
import assert from "node:assert/strict";
import { test } from "node:test";
import { byName, findAll } from "./lib/hook-runtime.mjs";

// #370 (was #362) and #369 (was #361): Step 1's readiness gate (findGap) and
// the parts editor's plant handling. PartsEditor has no hooks, so the real
// component is called with a controlled product and its real onClick/onChange
// handlers are driven; findGap is the gate the Continue button uses.
const { PartsEditor, findGap } = await import("../src/components/op/screens/product-step.tsx");

const plants = () => [
  { id: "p1", name: "Dongguan factory", country: "China", capabilities: [] },
  { id: "p2", name: "Chennai supplier", country: "India", capabilities: [] },
  { id: "p3", name: "Monterrey assembly", country: "Mexico", capabilities: [] },
];
const drill = () => ({
  name: "Cordless drill",
  plants: plants(),
  components: [
    { name: "gearbox", made_in: "Mexico", plant: "p3", group: "motor" },
    { name: "chuck", made_in: "India", plant: "p2" },
    { name: "casing", made_in: "China" },
  ],
  operations: [{ name: "final assembly", plant: "p3", group: "final_assembly" }],
});

// A tiny editor: the product lives here, every handler writes it back.
function editor(initial, economics) {
  const ed = { product: initial };
  const tree = () => PartsEditor({ product: ed.product, economics, onChange: (p) => (ed.product = p) });
  ed.remove = (label, index = 0) => findAll(tree(), (e) => byName("RemoveButton")(e) && e.props.label === label)[index].props.onClick();
  ed.add = (text) => findAll(tree(), (e) => byName("AddButton")(e) && JSON.stringify(e.props.children).includes(text))[0].props.onClick();
  ed.combo = (aria, index) => findAll(tree(), (e) => byName("Combobox")(e) && e.props.ariaLabel === aria)[index];
  ed.input = (aria, index) => findAll(tree(), (e) => e.type === "input" && e.props["aria-label"] === aria)[index];
  return ed;
}
const gap = (p) => findGap("parts", p.name, "2026-01-01", "", p, false);
const plantIds = (p) => new Set(p.plants.map((x) => x.id));
const dangling = (p) => {
  const ids = plantIds(p);
  return [...p.components.filter((c) => c.plant && !ids.has(c.plant)).map((c) => `component ${c.name}: ${c.plant}`), ...(p.operations ?? []).filter((o) => !ids.has(o.plant)).map((o) => `operation ${o.name}: ${JSON.stringify(o.plant)}`)];
};

test("a product with named steps at known plants is ready (control)", () => {
  assert.equal(gap(drill()), null);
});

// ---- #370 (was #362): blank processing step ---------------------------
test("a blank processing step blocks submission and points at its name field", () => {
  const ed = editor(drill());
  ed.add("Add a processing step");
  assert.equal(ed.product.operations.length, 2);
  assert.equal(ed.product.operations[1].name, "");
  const g = gap(ed.product);
  assert.ok(g, "readiness reports a gap for the blank step");
  assert.equal(g.field, "#field-op-name-1");
  assert.match(g.message, /Name this processing step or remove it/);
  assert.equal(ed.input("Processing step", 1).props.id, "field-op-name-1", "the gap's field exists on the input");
});

test("naming the step makes the product ready again", () => {
  const ed = editor(drill());
  ed.add("Add a processing step");
  ed.input("Processing step", 1).props.onChange({ target: { value: "solder and test" } });
  assert.equal(ed.product.operations[1].name, "solder and test");
  assert.equal(gap(ed.product), null);
});

test("removing the blank step also clears the gap", () => {
  const ed = editor(drill());
  ed.add("Add a processing step");
  assert.ok(gap(ed.product));
  ed.remove("Remove step", 1);
  assert.deepEqual(ed.product.operations.map((o) => o.name), ["final assembly"]);
  assert.equal(gap(ed.product), null);
});

// ---- #369 (was #361): plant removal -----------------------------------
test("removing a referenced plant clears its references but keeps made_in, and blocks until the step is reassigned", () => {
  const ed = editor(drill());
  ed.remove("Remove Monterrey assembly");
  const p = ed.product;
  assert.deepEqual(p.plants.map((x) => x.id), ["p1", "p2"]);
  assert.deepEqual(dangling(p).filter((d) => d.startsWith("component")), [], "no part holds the removed plant");
  const gearbox = p.components[0];
  assert.equal(gearbox.plant, undefined);
  assert.equal(gearbox.made_in, "Mexico", "the part's country is kept");
  assert.equal(gearbox.group, "motor");
  assert.equal(p.components[1].plant, "p2", "unrelated references are untouched");
  assert.equal(p.operations[0].plant, "", "the step is explicitly unassigned, not moved to another plant");
  assert.equal(p.operations[0].name, "final assembly");
  const g = gap(p);
  assert.ok(g, "submission is blocked");
  assert.equal(g.field, "#field-op-plant-0");
  assert.match(g.message, /final assembly/);
  assert.equal(ed.combo("At plant", 0).props.id, "field-op-plant-0", "the gap's field exists on the combobox");
});

test("remove the newest plant then add another: the unchanged step does not move to the new plant", () => {
  const ed = editor(drill());
  ed.remove("Remove Monterrey assembly");
  ed.add("Add a plant");
  const added = ed.product.plants[2];
  ed.input("Plant name", 2).props.onChange({ target: { value: "Gdansk works" } });
  assert.equal(ed.product.operations[0].plant, "", "the step is still unassigned");
  assert.notEqual(ed.product.operations[0].plant, added.id);
  assert.ok(gap(ed.product), "still blocked until the user picks a plant for the step");
  assert.equal(ed.product.components[0].plant, undefined, "the gearbox did not become a Polish-plant part");
});

test("deliberate reassignment through the At plant combobox unblocks submission", () => {
  const ed = editor(drill());
  ed.remove("Remove Monterrey assembly");
  ed.combo("At plant", 0).props.onChange("p1");
  assert.equal(ed.product.operations[0].plant, "p1");
  assert.deepEqual(dangling(ed.product), []);
  assert.equal(gap(ed.product), null);
});

test("removing an unreferenced plant, or a middle plant, only removes it", () => {
  const ed = editor({ ...drill(), components: [{ name: "casing", made_in: "China" }], operations: [{ name: "final assembly", plant: "p1" }] });
  ed.remove("Remove Chennai supplier");
  assert.deepEqual(ed.product.plants.map((x) => x.id), ["p1", "p3"]);
  assert.equal(ed.product.operations[0].plant, "p1");
  assert.equal(gap(ed.product), null);
});

test("a plant added later never takes an id that a part or step still references", () => {
  // A product that already holds a dangling reference (for example from an old
  // import) must not have it adopted by the next new plant.
  const p = drill();
  p.plants = p.plants.slice(0, 2);
  const ed = editor(p);
  ed.add("Add a plant");
  const ids = ed.product.plants.map((x) => x.id);
  assert.equal(new Set(ids).size, ids.length);
  assert.ok(!ids.includes("p3"), `new id ${ids[2]} must not adopt the dangling p3`);
});

test("typing a new plant name for a step creates a plant that only that step uses", () => {
  const ed = editor(drill());
  ed.remove("Remove Monterrey assembly");
  ed.combo("At plant", 0).props.onChange("Gdansk works");
  const created = ed.product.plants[2];
  assert.equal(created.name, "Gdansk works");
  assert.equal(ed.product.operations[0].plant, created.id);
  assert.equal(ed.product.components[0].plant, undefined, "the gearbox, cleared earlier, is not attached to the new plant");
});

// ---- #372: money inputs must not bind to a removed plant's id ----------
test("a new plant never inherits yearly costs keyed to a removed plant", () => {
  const economics = { group_costs: { final_assembly: { p1: 300000, p3: 400000 } } };
  const ed = editor(drill(), economics);
  ed.remove("Remove Monterrey assembly");
  ed.add("Add a plant");
  const added = ed.product.plants[2];
  assert.notEqual(added.id, "p3", `new id ${added.id} must not reuse the costed Monterrey id`);
  assert.equal(economics.group_costs.final_assembly[added.id], undefined, "the new plant has no yearly cost yet");
  ed.combo("At plant", 0).props.onChange(added.id);
  assert.equal(ed.product.operations[0].plant, added.id, "the step can still be reassigned to the new plant");
  assert.equal(economics.group_costs.final_assembly[added.id], undefined, "assigning the step does not adopt the old cost");
  assert.equal(economics.group_costs.final_assembly.p3, 400000, "the old entry is left untouched for the user to remove or re-enter");
});

test("a plant with no yearly costs still gets its old id back (control)", () => {
  const ed = editor(drill());
  ed.remove("Remove Monterrey assembly");
  ed.add("Add a plant");
  assert.equal(ed.product.plants[2].id, "p3", "unchanged behaviour when there is no cost to inherit");
});
