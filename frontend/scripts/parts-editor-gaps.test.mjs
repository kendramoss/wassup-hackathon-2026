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
function editor(initial) {
  const ed = { product: initial };
  const tree = () => PartsEditor({ product: ed.product, onChange: (p) => (ed.product = p) });
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
