import "./render-support.mjs";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { afterEach, test } from "node:test";
import { createElement as h } from "react";
import { json, stubNavigation } from "./lib/hook-runtime.mjs";
import { deepMount, textOf } from "./lib/deep-mount.mjs";

// Judge panel round 3 (D270): the "Two inputs, two answers" line did not show
// on the live flagship. The API sent facts_source: "pasted", but the tab kept
// a copy of the pinned demo saved before that field existed, and the copy
// replaced the server's case whole. The fixture is the live response's shape.
const flagship = JSON.parse(readFileSync(new URL("./golden/flagship-case.json", import.meta.url), "utf8"));
const ID = flagship.id;

const store = new Map();
globalThis.sessionStorage = {
  getItem: (k) => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: (k) => store.delete(k),
  clear: () => store.clear(),
};
globalThis.window = new EventTarget();
globalThis.document = new EventTarget();
stubNavigation({ id: ID });
const live = [];
afterEach(() => {
  live.splice(0).forEach((a) => a.unmount());
  store.clear();
});

const { pickLocal, localEntry } = await import("../src/lib/case-local.ts");
const { readsDiffer, asToday } = await import("../src/lib/plan-check.ts");
const ScenariosView = (await import("../src/components/op/screens/scenarios-view.tsx")).ScenariosView;
const OriginStep = (await import("../src/components/op/screens/origin-step.tsx")).OriginStep;

const { facts_source: _drop, ...beforeField } = flagship;

test("the live flagship shape says two inputs, two answers", () => {
  assert.equal(flagship.facts_source, "pasted");
  assert.match(readsDiffer(flagship, asToday(flagship.scenarios)), /^Two inputs, two answers: .* says China; .* say Thailand\./);
});

test("an old-format copy of a demo never replaces the server's case", () => {
  assert.equal(pickLocal(flagship, beforeField), flagship);
  assert.equal(pickLocal(flagship, null), flagship);
  assert.equal(pickLocal(flagship, "junk"), flagship);
});

test("a copy based on the same server state keeps its results and gains new fields", () => {
  const kept = { ...beforeField, scenarios: flagship.scenarios.slice(0, 1) };
  const got = pickLocal(flagship, localEntry(kept, flagship.updated_at));
  assert.equal(got.scenarios.length, 1);
  assert.equal(got.facts_source, "pasted");
});

test("a copy based on an older server state is dropped", () => {
  const kept = { ...flagship, scenarios: [] };
  assert.equal(pickLocal(flagship, localEntry(kept, "2026-10-01T00:00:00Z")), flagship);
  assert.equal(pickLocal(flagship, localEntry({ ...kept, id: "other" }, flagship.updated_at)), flagship);
});

async function screen(Step) {
  globalThis.fetch = async (url, init = {}) => {
    const u = String(url);
    if (u.endsWith(`/cases/${ID}`) && (init.method ?? "GET") === "GET") return { ok: true, status: 200, headers: new Headers(), json: async () => flagship };
    if (u.includes("/demos")) return json({ demos: [] });
    if (u.endsWith("/api/session")) return json({ email: null });
    return json({});
  };
  const el = Step();
  const app = deepMount(h(el.type, el.props));
  live.push(app);
  app.refresh();
  await app.flush();
  return app.all().map((e) => textOf(e.props?.children)).join(" ");
}

for (const [name, Step] of [
  ["scenarios", ScenariosView],
  ["origin", OriginStep],
]) {
  test(`${name}: the line shows even with a copy kept from before the field existed`, async () => {
    sessionStorage.setItem(`op:local:${ID}`, JSON.stringify(beforeField));
    assert.match(await screen(Step), /Two inputs, two answers/);
  });
}
