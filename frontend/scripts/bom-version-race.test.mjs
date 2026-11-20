import "./render-support.mjs";
import assert from "node:assert/strict";
import { test } from "node:test";
import { byName, findAll, instance, json, tick } from "./lib/hook-runtime.mjs";

// #364 (was #356): a BOM version comparison and its Apply belong to one file.
// The real BomVersion component runs in a hook runtime with the upload held by
// the test. Picking another file while a comparison runs must not let the
// earlier file's preview authorise applying the new one.
const { BomVersion } = await import("../src/components/op/bom-version.tsx");

const file = (n) => new File(["x"], n, { type: "text/csv" });

function mount() {
  let sched = false;
  const app = { reloads: 0, calls: [], pending: {} };
  const schedule = () => {
    if (sched) return;
    sched = true;
    queueMicrotask(() => {
      sched = false;
      app.refresh();
    });
  };
  const inst = instance(BomVersion, schedule);
  const props = { id: "c1", c: { id: "c1", pinned: false, prediction: { origin_country: "China" }, product: { components: [{ name: "x" }] } }, reload: () => app.reloads++ };
  app.inst = inst;
  app.refresh = () => {
    for (let n = 0; n < 10 && (n === 0 || inst.dirty); n++) inst.render(props);
  };
  app.flush = async () => {
    for (let n = 0; n < 6; n++) {
      await tick();
      app.refresh();
    }
  };
  app.input = () => findAll(inst.tree, (e) => e.type === "input" && e.props.type === "file")[0];
  app.pick = (f) => {
    app.input().props.onChange({ target: { files: [f] } });
    app.refresh();
  };
  app.button = (label) => findAll(inst.tree, (e) => byName("Button")(e) && JSON.stringify(e.props.children).includes(label))[0];
  app.markup = () => JSON.stringify(inst.tree);
  // Compare requests are held until the test settles them; Apply answers at once.
  globalThis.fetch = async (_url, init) => {
    const f = init.body.get("file");
    if (!f) return json({ message: "file required" }, 400); // the probe: feature is on
    const apply = init.body.get("apply") === "true";
    app.calls.push({ file: f.name, apply });
    const ok = () => json({ changes: [{ kind: "component_added", summary: `changes of ${f.name}` }], old_origin: "China", new_origin: "China", origin_changed: false, new_confidence: 0.9 });
    if (apply) return ok();
    return new Promise((res) => {
      app.pending[f.name] = { ok: () => res(ok()), fail: () => res(json({ message: `boom for ${f.name}`, code: "internal" }, 500)) };
    });
  };
  return app;
}
const applied = (app) => app.calls.filter((c) => c.apply).map((c) => c.file);

test("compare A, select B, resolve A: A's preview cannot authorise applying B", async () => {
  const app = mount();
  app.refresh();
  await app.flush();
  app.pick(file("A.csv"));
  app.button("Compare").props.onClick();
  app.refresh();
  await app.flush();
  assert.ok(app.pending["A.csv"], "compare A is in flight");
  app.pick(file("B.csv"));
  app.pending["A.csv"].ok();
  await app.flush();
  assert.doesNotMatch(app.markup(), /changes of A\.csv/, "A's result is not shown for B");
  assert.equal(app.button("Apply to this case"), undefined, "Apply is not offered for a file that was not compared");
  assert.deepEqual(applied(app), []);
});

test("a superseded comparison's error is ignored", async () => {
  const app = mount();
  app.refresh();
  await app.flush();
  app.pick(file("A.csv"));
  app.button("Compare").props.onClick();
  app.refresh();
  await app.flush();
  app.pick(file("B.csv"));
  app.pending["A.csv"].fail();
  await app.flush();
  assert.equal(findAll(app.inst.tree, byName("ErrorBox")).length, 0, "no error box for a superseded comparison");
  assert.ok(!app.button("Compare").props.disabled, "the new file can be compared");
});

test("after B is selected, comparing B then applying uses B", async () => {
  const app = mount();
  app.refresh();
  await app.flush();
  app.pick(file("A.csv"));
  app.button("Compare").props.onClick();
  app.refresh();
  await app.flush();
  app.pick(file("B.csv"));
  app.pending["A.csv"].ok();
  await app.flush();
  app.button("Compare").props.onClick();
  app.refresh();
  await app.flush();
  app.pending["B.csv"].ok();
  await app.flush();
  assert.match(app.markup(), /changes of B\.csv/);
  app.button("Apply to this case").props.onClick();
  app.refresh();
  await app.flush();
  assert.deepEqual(applied(app), ["B.csv"]);
  assert.equal(app.reloads, 1);
});

test("control: current-file compare then apply still works", async () => {
  const app = mount();
  app.refresh();
  await app.flush();
  app.pick(file("A.csv"));
  app.button("Compare").props.onClick();
  app.refresh();
  await app.flush();
  app.pending["A.csv"].ok();
  await app.flush();
  assert.match(app.markup(), /changes of A\.csv/);
  app.button("Apply to this case").props.onClick();
  app.refresh();
  await app.flush();
  assert.deepEqual(applied(app), ["A.csv"]);
  assert.equal(app.reloads, 1);
  assert.match(app.markup(), /Applied/);
});

test("an Apply that finishes after another file was picked still reloads the case, without claiming B was applied", async () => {
  const app = mount();
  app.refresh();
  await app.flush();
  app.pick(file("A.csv"));
  app.button("Compare").props.onClick();
  app.refresh();
  await app.flush();
  app.pending["A.csv"].ok();
  await app.flush();
  // Hold the apply request this time.
  const real = globalThis.fetch;
  let release;
  globalThis.fetch = async (url, init) => {
    if (init.body.get("apply") === "true") {
      app.calls.push({ file: init.body.get("file").name, apply: true });
      return new Promise((res) => (release = () => res(json({ changes: [], old_origin: "China", new_origin: "China", origin_changed: false, new_confidence: 0.9 }))));
    }
    return real(url, init);
  };
  app.button("Apply to this case").props.onClick();
  app.refresh();
  await app.flush();
  app.pick(file("B.csv"));
  release();
  await app.flush();
  assert.deepEqual(applied(app), ["A.csv"]);
  assert.equal(app.reloads, 1, "the applied case is reloaded");
  assert.doesNotMatch(app.markup(), /Applied\./, "no applied message for the newly selected file");
});
