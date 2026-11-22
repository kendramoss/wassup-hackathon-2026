import "./render-support.mjs";
import assert from "node:assert/strict";
import { test } from "node:test";
import { instance, findAll, byName, tick } from "./lib/hook-runtime.mjs";

// Judge panel round 16 (N4): "Turn into parts and plants" wrote its failure to
// the same error as Find origin, and the ErrorBox retry was always Find
// origin. The draft now has its own error, and its retry re-runs the draft.

const { ProductStep } = await import("../src/components/op/screens/product-step.tsx");
const { api, ApiError } = await import("../src/lib/api/client.ts");

const textOf = (n) => (n == null || typeof n === "boolean" ? "" : typeof n !== "object" ? String(n) : Array.isArray(n) ? n.map(textOf).join("") : textOf(n.props?.children));

function mount() {
  let scheduled = false;
  const inst = instance(ProductStep, () => {
    if (scheduled) return;
    scheduled = true;
    queueMicrotask(() => {
      scheduled = false;
      refresh();
    });
  });
  const refresh = () => {
    for (let n = 0; n < 20 && inst.dirty; n++) inst.render({});
  };
  inst.render({});
  refresh();
  const app = {
    inst,
    refresh,
    flush: async () => {
      for (let n = 0; n < 5; n++) await tick();
      refresh();
    },
    tab(t) {
      findAll(inst.tree, (e) => e.type === "button" && e.props.id === `step1-tab-${t}`)[0].props.onClick();
      refresh();
    },
    type(v) {
      findAll(inst.tree, (e) => e.type === "textarea")[0].props.onChange({ target: { value: v } });
      refresh();
    },
    draftButton: () => findAll(inst.tree, (e) => e.type === "button" && /Turn into parts and plants|Drafting/.test(textOf(e)))[0],
    errorBoxes: () => findAll(inst.tree, byName("ErrorBox")),
  };
  return app;
}

const facts = "Each widget is assembled in Thailand from a motor made in China and a housing moulded in Thailand.";

test("a failed draft shows its own error, and its retry drafts again instead of finding the origin", async () => {
  const saved = { extract: api.extract, familyCheck: api.familyCheck, moneyPrefill: api.moneyPrefill };
  const realFetch = globalThis.fetch;
  const calls = { extract: 0, predict: 0 };
  api.extract = async () => {
    calls.extract++;
    throw new ApiError("the model is unavailable; try again", 502, "model_unavailable", true);
  };
  api.moneyPrefill = async () => ({});
  globalThis.fetch = async (url) => {
    if (String(url).includes("/cases/predict")) calls.predict++;
    return new Response("{}", { status: 200, headers: { "content-type": "application/json" } });
  };
  try {
    const app = mount();
    app.tab("paste");
    app.type(facts);
    app.draftButton().props.onClick();
    app.refresh();
    await app.flush();
    assert.equal(calls.extract, 1);

    const boxes = app.errorBoxes();
    assert.equal(boxes.length, 1, "one error box, the draft's");
    assert.equal(boxes[0].props.lead, "Drafting parts and plants didn't work.");
    assert.ok(boxes[0].props.error instanceof ApiError);

    boxes[0].props.onRetry();
    app.refresh();
    await app.flush();
    assert.equal(calls.extract, 2, "the retry drafted again");
    assert.equal(calls.predict, 0, "the retry never ran Find origin");
  } finally {
    Object.assign(api, saved);
    globalThis.fetch = realFetch;
  }
});

test("a successful draft clears the draft's error", async () => {
  const saved = { extract: api.extract, moneyPrefill: api.moneyPrefill };
  let fail = true;
  api.extract = async () => {
    if (fail) throw new ApiError("the model is unavailable; try again", 502, "model_unavailable", true);
    return { product: { name: "Widget", components: [{ name: "Motor", plant: "p1", made_in: "CN", group: "motor_fan_assembly" }], plants: [{ id: "p1", name: "P1", country: "CN" }], operations: [] }, warnings: [] };
  };
  api.moneyPrefill = async () => ({});
  try {
    const app = mount();
    app.tab("paste");
    app.type(facts);
    app.draftButton().props.onClick();
    await app.flush();
    assert.equal(app.errorBoxes().length, 1);
    fail = false;
    app.errorBoxes()[0].props.onRetry();
    await app.flush();
    assert.equal(app.errorBoxes().length, 0);
  } finally {
    Object.assign(api, saved);
  }
});
