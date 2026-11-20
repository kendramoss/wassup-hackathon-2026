import assert from "node:assert/strict";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

// #348: the scenario editor must start from the scenarios the case stores,
// not from the default examples. Real Scenarios/Editor components (static
// markup; Editor's onChange called from its element tree) and the real
// api.scenarios request over a stubbed fetch.
const { Scenarios, Editor } = await import("../src/components/op/screens/scenarios-view.tsx");
const drafts = await import("../src/lib/scenario-drafts.ts");
const { api } = await import("../src/lib/api/client.ts");

const product = () => ({
  name: "Heat gun",
  description: "Handheld heat gun",
  plants: [
    { id: "p1", name: "Shenzhen", country: "CN", capabilities: [] },
    { id: "p2", name: "Hanoi", country: "VN", capabilities: [] },
  ],
  components: [
    { name: "Housing", made_in: "CN", plant: "p1", group: "housing" },
    { name: "Heating element", made_in: "CN", plant: "p1", group: "heater" },
    { name: "Steel screws", made_in: "JP", group: "fasteners" },
  ],
  operations: [
    { name: "Moulding", plant: "p1", group: "housing" },
    { name: "Final assembly", plant: "p1", group: "final_assembly" },
  ],
});
const movedHousing = () => {
  const p = product();
  p.components[0] = { ...p.components[0], plant: "p2", made_in: "VN" };
  p.operations[0] = { ...p.operations[0], plant: "p2" };
  // A saved choice the move picker cannot express: it must survive anyway.
  p.components[2] = { ...p.components[2], made_in: "KR" };
  return p;
};
const saved = () => [
  { label: "Cheaper resin", facts: "Housing resin changed to PP.", prediction: { origin_country: "CN" } },
  { label: "Housing to Hanoi", product: movedHousing(), prediction: { origin_country: "VN" } },
  { label: "What if the motor were Korean?", facts: "Heat gun.\n\nChange: motor Korean", prediction: { origin_country: "KR" } },
];
const caseOf = (scenarios) => ({
  id: "c1", display_name: "Heat gun", date: "2026-01-01", facts: "Heat gun made in China.", pinned: false,
  product: product(), prediction: { origin_country: "CN", confidence: 0.8 }, scenarios,
});
const freeze = (o) => (Object.values(o).forEach((v) => v && typeof v === "object" && freeze(v)), Object.freeze(o));
const find = (el, pred) => {
  if (!el || typeof el !== "object") return null;
  if (Array.isArray(el)) { for (const x of el) { const r = find(x, pred); if (r) return r; } return null; }
  if (el.props && pred(el.props)) return el;
  return el.props ? find(el.props.children, pred) : null;
};
const findAll = (el, pred, out = []) => {
  if (!el || typeof el !== "object") return out;
  if (Array.isArray(el)) { el.forEach((x) => findAll(x, pred, out)); return out; }
  if (el.props) { if (pred(el.props)) out.push(el); findAll(el.props.children, pred, out); }
  return out;
};
const groupsOf = (c) => [...new Set([...c.product.components, ...c.product.operations].map((x) => x.group).filter((g) => g && g !== "final_assembly"))];
const initial = (c, add, revised = null) => drafts.initialDrafts(c, add, () => revised);

test("saved facts, move and Ask-created scenarios hydrate the drafts with labels and inputs", () => {
  const d = initial(freeze(caseOf(saved())));
  assert.deepEqual(d.map((x) => x.label), ["Cheaper resin", "Housing to Hanoi", "What if the motor were Korean?"]);
  assert.equal(d[0].mode, "facts");
  assert.equal(d[0].facts, "Housing resin changed to PP.");
  assert.equal(d[1].mode, "move");
  assert.deepEqual(d[1].groups, ["housing"]);
  assert.equal(d[1].plant, "p2");
  assert.equal(d[2].mode, "facts");
  assert.equal(d[2].facts, "Heat gun.\n\nChange: motor Korean");
});

test("no saved scenarios (missing or empty) keep the default examples", () => {
  for (const s of [undefined, []]) {
    const d = initial(caseOf(s));
    assert.equal(d[0].label, "As today");
    assert.deepEqual(d.map((x) => x.mode), d.map(() => "move"));
    assert.deepEqual(d, drafts.exampleDrafts(caseOf(s)));
  }
  const noProduct = { id: "c2", facts: "Only a description" };
  assert.deepEqual(initial(noProduct), [{ label: "Scenario 1", mode: "facts", groups: [], plant: "", facts: "Only a description" }]);
});

test("the real new-scenario editor still opens on the defaults; a saved case shows its cards", () => {
  const fresh = html(Scenarios, { id: "c1", c: caseOf(undefined), setCase() {} });
  assert.match(text(fresh), /Predict the scenarios/);
  assert.match(fresh, /value="As today"/);
  assert.match(text(fresh), /Reset to examples/);
  assert.doesNotMatch(text(fresh), /Cancel/);
  const back = text(html(Scenarios, { id: "c1", c: caseOf(saved()), setCase() {} }));
  assert.match(back, /Edit scenarios/);
  assert.match(back, /Housing to Hanoi/);
});

test("the real Editor shows the saved labels, facts and move choices", () => {
  const c = caseOf(saved());
  const markup = html(Editor, { drafts: initial(c), setDrafts() {}, c, groups: groupsOf(c) });
  assert.match(markup, /value="Cheaper resin"/);
  assert.match(markup, /Housing resin changed to PP\./);
  assert.match(markup, /value="Housing to Hanoi"/);
  assert.match(markup, /<option value="p2" selected="">/);
  assert.doesNotMatch(markup, /As today/);
});

test("edit one field then Run sends the saved inputs, including choices the picker cannot show", async () => {
  const c = freeze(caseOf(saved()));
  let next = null;
  const tree = Editor({ drafts: initial(c), setDrafts: (x) => (next = x), c, groups: groupsOf(c) });
  find(tree, (p) => p.id === "sl0").props.onChange({ target: { value: "Cheaper resin v2" } });
  assert.equal(next[0].label, "Cheaper resin v2");
  const calls = [];
  globalThis.fetch = async (url, init) => {
    calls.push({ url: String(url), method: init.method, body: JSON.parse(init.body) });
    return new Response(JSON.stringify({ id: "c1", scenarios: [] }), { status: 200, headers: { "content-type": "application/json" } });
  };
  await api.scenarios("c1", { scenarios: drafts.scenarioBody(next, c.product), economics: undefined });
  assert.equal(calls[0].method, "POST");
  assert.deepEqual(calls[0].body.scenarios, [
    { label: "Cheaper resin v2", facts: "Housing resin changed to PP." },
    { label: "Housing to Hanoi", product: movedHousing() },
    { label: "What if the motor were Korean?", facts: "Heat gun.\n\nChange: motor Korean" },
  ]);
});

test("editing a move's groups rebuilds that variant from the case product", () => {
  const c = freeze(caseOf(saved()));
  let next = null;
  const tree = Editor({ drafts: initial(c), setDrafts: (x) => (next = x), c, groups: groupsOf(c) });
  // Only the move draft renders checkboxes: housing, heater, fasteners.
  const boxes = findAll(tree, (p) => p.type === "checkbox");
  assert.equal(boxes.length, 3);
  boxes[1].props.onChange({ target: { checked: true } }); // heater joins the move
  const body = drafts.scenarioBody(next, c.product);
  const p = body[1].product;
  assert.equal(p.components.find((x) => x.group === "heater").plant, "p2");
  assert.equal(p.components.find((x) => x.group === "housing").plant, "p2");
  assert.equal(p.components.find((x) => x.group === "fasteners").made_in, "JP");
});

test("Cancel restores the saved drafts and does not mutate the stored scenarios; Reset gives the examples", () => {
  const c = freeze(caseOf(saved()));
  const before = JSON.stringify(c);
  const edited = initial(c).map((x) => ({ ...x, label: "scratch" }));
  assert.equal(edited[0].label, "scratch");
  assert.equal(initial(c)[0].label, "Cheaper resin");
  assert.equal(drafts.exampleDrafts(c)[0].label, "As today");
  assert.equal(JSON.stringify(c), before);
});

test("Add from Ask keeps the first two saved drafts and adds the what-if", () => {
  const d = initial(caseOf(saved()), "ship by sea", "Revised facts");
  assert.equal(d.length, 3);
  assert.deepEqual(d.slice(0, 2).map((x) => x.label), ["Cheaper resin", "Housing to Hanoi"]);
  assert.equal(d[2].label, "ship by sea");
  assert.equal(d[2].facts, "Revised facts");
});
