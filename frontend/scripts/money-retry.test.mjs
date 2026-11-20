import assert from "node:assert/strict";
import { test } from "node:test";
import "./render-support.mjs";
import * as React from "react";

// #350: Try again on a failed money save must retry that save with the edited
// values; Try again on a failed scenario run must still run. The real
// Scenarios component runs under a minimal hook dispatcher (state, refs,
// effects) so its real callbacks (MoneyStrip.onEdit, MoneyDrawer.onSave,
// ErrorBox.onRetry, buttons) are driven, over the real api client with a
// fetch spy whose responses the test resolves by hand. No browser, no network.
const { Scenarios } = await import("../src/components/op/screens/scenarios-view.tsx");

const INTERNALS = React.__CLIENT_INTERNALS_DO_NOT_USE_OR_WARN_USERS_THEY_CANNOT_UPGRADE;

function mount(initial) {
  const slots = [];
  let props = initial;
  let tree = null;
  let dirty = false;
  let effects = [];
  let cursor = 0;
  const dispatcher = {
    useState(init) {
      const i = cursor++;
      if (!slots[i]) {
        slots[i] = { v: typeof init === "function" ? init() : init };
        slots[i].set = (x) => {
          const nv = typeof x === "function" ? x(slots[i].v) : x;
          if (!Object.is(nv, slots[i].v)) { slots[i].v = nv; dirty = true; }
        };
      }
      return [slots[i].v, slots[i].set];
    },
    useRef(init) { const i = cursor++; return (slots[i] ??= { current: init }); },
    useMemo(fn, deps) {
      const i = cursor++;
      const s = slots[i];
      if (s && deps && s.deps.length === deps.length && deps.every((d, k) => Object.is(d, s.deps[k]))) return s.v;
      slots[i] = { v: fn(), deps };
      return slots[i].v;
    },
    useEffect(fn, deps) {
      const i = cursor++;
      const s = slots[i];
      if (s && deps && s.deps.length === deps.length && deps.every((d, k) => Object.is(d, s.deps[k]))) return;
      slots[i] = { deps };
      effects.push(fn);
    },
    useCallback(fn) { cursor++; return fn; },
  };
  const h = {
    get tree() { return tree; },
    render(next) {
      if (next) props = next;
      for (let n = 0; n < 20; n++) {
        dirty = false; cursor = 0; effects = [];
        const prev = INTERNALS.H;
        INTERNALS.H = dispatcher;
        try { tree = Scenarios(props); } finally { INTERNALS.H = prev; }
        effects.forEach((f) => f());
        if (!dirty) return;
      }
      throw new Error("render loop");
    },
    // Lets pending promise continuations run, then re-renders.
    async settle() { for (let i = 0; i < 30; i++) await Promise.resolve(); await new Promise((r) => setImmediate(r)); h.render(); },
  };
  h.render();
  return h;
}

const findAll = (el, pred, out = []) => {
  if (!el || typeof el !== "object") return out;
  if (Array.isArray(el)) { el.forEach((x) => findAll(x, pred, out)); return out; }
  if (el.props) { if (pred(el)) out.push(el); findAll(el.props.children, pred, out); }
  return out;
};
const byName = (name) => (el) => typeof el.type === "function" && el.type.name === name;
const one = (h, name) => findAll(h.tree, byName(name))[0] ?? null;
const button = (h, label) => findAll(h.tree, (el) => el.props.children === label && typeof el.props.onClick === "function")[0] ?? null;

const caseOf = (extra = {}) => ({
  id: "c1", display_name: "Heat gun", date: "2026-01-01", facts: "Heat gun made in China.", pinned: false,
  prediction: { origin_country: "CN", confidence: 0.8 },
  economics: { annual_import_value_usd: 100000 },
  ...extra,
});
const EDITED = { annual_import_value_usd: 900000 };
const SAVED = [{ label: "Cheaper resin", facts: "Resin PP.", prediction: { origin_country: "CN" } }];

function spy() {
  const calls = [];
  globalThis.fetch = (url, init = {}) =>
    new Promise((resolve) => {
      calls.push({ method: init.method ?? "GET", path: String(url).replace(/^.*?\/v1/, ""), body: init.body ? JSON.parse(init.body) : undefined, resolve });
    });
  const reply = (call, status, body) => call.resolve(new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } }));
  return { calls, reply };
}
const fail = { code: "internal", message: "temporary failure" };
const seq = (calls) => calls.map((c) => `${c.method} ${c.path}`);

function setup(c = caseOf()) {
  const set = [];
  const h = mount({ id: c.id, c, setCase: (x) => set.push(x) });
  return { h, set };
}
async function saveEdited(h) {
  one(h, "MoneyStrip").props.onEdit();
  h.render();
  const p = one(h, "MoneyDrawer").props.onSave(EDITED);
  h.render();
  return p;
}

test("failed money save: Try again repeats the PUT with the edited values, no scenarios POST, then succeeds", async () => {
  const { calls, reply } = spy();
  const { h, set } = setup();
  const saving = saveEdited(h);
  assert.deepEqual(seq(calls), ["PUT /cases/c1/economics"]);
  assert.deepEqual(calls[0].body, EDITED);
  reply(calls[0], 500, fail);
  await saving; await h.settle();
  assert.ok(one(h, "ErrorBox").props.error, "error shown");
  assert.equal(one(h, "MoneyDrawer"), null, "drawer closed");

  one(h, "ErrorBox").props.onRetry();
  h.render();
  assert.equal(one(h, "ErrorBox"), null, "error cleared while retrying");
  assert.deepEqual(seq(calls), ["PUT /cases/c1/economics", "PUT /cases/c1/economics"]);
  assert.deepEqual(calls[1].body, EDITED);
  reply(calls[1], 200, { case_id: "c1", economics: EDITED });
  await h.settle();
  assert.equal(calls[2].path, "/cases/c1");
  reply(calls[2], 200, caseOf({ economics: EDITED }));
  await h.settle();
  assert.equal(one(h, "ErrorBox"), null);
  assert.deepEqual(set.at(-1).economics, EDITED);
  assert.deepEqual(seq(calls), ["PUT /cases/c1/economics", "PUT /cases/c1/economics", "GET /cases/c1"]);
  assert.ok(!seq(calls).some((s) => s.includes("scenarios") || s.includes("predict")));
});

test("a second failure of the retried save still retries the save", async () => {
  const { calls, reply } = spy();
  const { h } = setup();
  const saving = saveEdited(h);
  reply(calls[0], 500, fail);
  await saving; await h.settle();
  one(h, "ErrorBox").props.onRetry();
  h.render();
  reply(calls[1], 500, fail);
  await h.settle();
  assert.ok(one(h, "ErrorBox"));
  one(h, "ErrorBox").props.onRetry();
  h.render();
  assert.deepEqual(seq(calls), ["PUT /cases/c1/economics", "PUT /cases/c1/economics", "PUT /cases/c1/economics"]);
  assert.deepEqual(calls[2].body, EDITED);
});

test("repeated Try again while the retry is in flight sends one request", async () => {
  const { calls, reply } = spy();
  const { h } = setup();
  const saving = saveEdited(h);
  reply(calls[0], 500, fail);
  await saving; await h.settle();
  const retry = one(h, "ErrorBox").props.onRetry;
  retry(); retry(); retry();
  h.render();
  assert.equal(calls.length, 2);
  assert.equal(button(h, "Predict the scenarios").props.disabled, true);
});

test("failed scenario run: Try again runs the scenarios with the current drafts and economics, never the money save", async () => {
  const { calls, reply } = spy();
  const { h, set } = setup();
  const run = button(h, "Predict the scenarios").props.onClick();
  h.render();
  assert.deepEqual(seq(calls), ["POST /cases/c1/scenarios"]);
  reply(calls[0], 502, { code: "model_unavailable", message: "model down" });
  await run; await h.settle();
  assert.ok(one(h, "ErrorBox").props.error);

  // The user edits a draft between the failure and Try again: the retry sends the current drafts.
  one(h, "Editor").props.setDrafts(one(h, "Editor").props.drafts.map((d, i) => (i === 0 ? { ...d, label: "edited label" } : d)));
  h.render();
  one(h, "ErrorBox").props.onRetry();
  h.render();
  assert.deepEqual(seq(calls), ["POST /cases/c1/scenarios", "POST /cases/c1/scenarios"]);
  assert.equal(calls[1].body.scenarios[0].label, "edited label");
  assert.deepEqual(calls[1].body.economics, { annual_import_value_usd: 100000 });
  reply(calls[1], 200, caseOf({ scenarios: SAVED }));
  await h.settle();
  assert.equal(one(h, "ErrorBox"), null);
  assert.equal(set.at(-1).scenarios.length, 1);
  assert.equal(seq(calls).length, 2);
});

test("the latest failure decides the retry: run failure after a save failure runs, save failure after a run failure saves", async () => {
  const { calls, reply } = spy();
  const { h } = setup();
  const saving = saveEdited(h);
  reply(calls[0], 500, fail);
  await saving; await h.settle();
  const run = button(h, "Predict the scenarios").props.onClick();
  h.render();
  reply(calls[1], 502, { code: "model_unavailable", message: "down" });
  await run; await h.settle();
  one(h, "ErrorBox").props.onRetry();
  h.render();
  assert.equal(calls[2].method + " " + calls[2].path, "POST /cases/c1/scenarios");
  reply(calls[2], 502, { code: "model_unavailable", message: "down" });
  await h.settle();

  const saving2 = saveEdited(h);
  reply(calls[3], 500, fail);
  await saving2; await h.settle();
  one(h, "ErrorBox").props.onRetry();
  h.render();
  assert.equal(calls[4].method + " " + calls[4].path, "PUT /cases/c1/economics");
  assert.deepEqual(calls[4].body, EDITED);
  assert.equal(calls.length, 5);
});

test("Cancel clears the failure; a later run failure retries the run", async () => {
  const { calls, reply } = spy();
  const { h } = setup(caseOf({ scenarios: SAVED }));
  button(h, "Edit scenarios").props.onClick();
  h.render();
  const saving = saveEdited(h);
  reply(calls[0], 500, fail);
  await saving; await h.settle();
  assert.ok(one(h, "ErrorBox"));
  button(h, "Cancel").props.onClick();
  h.render();
  assert.equal(one(h, "ErrorBox"), null);
  button(h, "Edit scenarios").props.onClick();
  h.render();
  const run = button(h, "Run again").props.onClick();
  h.render();
  reply(calls[1], 502, { code: "model_unavailable", message: "down" });
  await run; await h.settle();
  one(h, "ErrorBox").props.onRetry();
  h.render();
  assert.equal(calls[2].method + " " + calls[2].path, "POST /cases/c1/scenarios");
  assert.equal(calls[2].body.scenarios[0].label, "Cheaper resin");
});

test("a failed save from another case is not retried after navigation", async () => {
  const { calls, reply } = spy();
  const { h } = setup();
  const saving = saveEdited(h);
  reply(calls[0], 500, fail);
  await saving; await h.settle();
  h.render({ id: "c2", c: caseOf({ id: "c2" }), setCase() {} });
  const box = one(h, "ErrorBox");
  assert.ok(!box || box.props.onRetry === undefined);
  assert.equal(calls.length, 1);
});
