import "./render-support.mjs";
import assert from "node:assert/strict";
import { test } from "node:test";

// D740: the real UsmcaCheck hooks (useState, useRef, useEffect) run under the
// repo's hook shim (same style as family-race.test.mjs; there is no DOM
// harness) with api.usmcaCheck held as a deferred promise. Fire A, edit, fire
// B, resolve B then A: only B lands, and the chip is hidden while stale.
const React = await import("react");
const internals = React.__CLIENT_INTERNALS_DO_NOT_USE_OR_WARN_USERS_THEY_CANNOT_UPGRADE;
const { UsmcaCheck, UsmcaResult } = await import("../src/components/op/usmca-check.tsx");
const { api } = await import("../src/lib/api/client.ts");

const tick = () => new Promise((r) => setImmediate(r));

function mount(props) {
  const inst = { hooks: [], i: 0, effects: [], tree: null, mounted: true };
  const dispatcher = {
    useState(init) {
      const h = (inst.hooks[inst.i] ??= { v: typeof init === "function" ? init() : init });
      inst.i++;
      h.set ??= (n) => {
        if (!inst.mounted) return;
        const nv = typeof n === "function" ? n(h.v) : n;
        if (Object.is(nv, h.v)) return;
        h.v = nv;
        inst.render();
      };
      return [h.v, h.set];
    },
    useRef(init) {
      return (inst.hooks[inst.i++] ??= { current: init });
    },
    useEffect(fn, deps) {
      const h = (inst.hooks[inst.i++] ??= { deps: undefined, first: true });
      const changed = h.first || !deps || !h.deps || deps.some((d, k) => !Object.is(d, h.deps[k]));
      if (changed) inst.effects.push({ h, fn, deps });
    },
  };
  inst.render = () => {
    inst.i = 0;
    const prev = internals.H;
    internals.H = dispatcher;
    try {
      inst.tree = UsmcaCheck(props);
    } finally {
      internals.H = prev;
    }
    for (const e of inst.effects.splice(0)) {
      e.h.cleanup?.();
      e.h.first = false;
      e.h.deps = e.deps;
      e.h.cleanup = e.fn() ?? undefined;
    }
  };
  inst.unmount = () => {
    inst.mounted = false;
    for (const h of inst.hooks) h?.cleanup?.();
  };
  inst.render();
  return inst;
}

const walk = (node, visit) => {
  if (node == null || typeof node !== "object") return;
  if (Array.isArray(node)) return node.forEach((n) => walk(n, visit));
  if (!React.isValidElement(node)) return;
  visit(node);
  walk(node.props.children, visit);
};
const findAll = (tree, pred) => {
  const out = [];
  walk(tree, (el) => pred(el) && out.push(el));
  return out;
};
const byId = (inst, id) => findAll(inst.tree, (el) => el.props?.id === id)[0];
const type = (inst, id, value) => byId(inst, id).props.onChange({ target: { value } });
const tick_ = (inst, label, checked) => findAll(inst.tree, (el) => el.type === "input" && el.props.type === "checkbox")[label].props.onChange({ target: { checked } });
const submit = (inst) => findAll(inst.tree, (el) => el.type === "form")[0].props.onSubmit({ preventDefault() {} });
const result = (inst) => findAll(inst.tree, (el) => el.type === UsmcaResult)[0];

const source = { data_version: "d", snapshot: { source: "s", revision: "r", retrieved_on: "t", source_hash: "h" }, limits: [] };
const res = (status, tag) => ({ status, destination: "US", regime: "usmca", good_hs: "820231", source, limits: [], missing_facts: [], tag });

function stub() {
  const calls = [];
  api.usmcaCheck = (body, signal) => {
    const d = {};
    d.p = new Promise((a, b) => ((d.resolve = a), (d.reject = b)));
    d.body = body;
    d.signal = signal;
    calls.push(d);
    return d.p;
  };
  return calls;
}
const c = { id: "c", product: { hts: "8202.31" } };

test("fire A, edit, fire B, resolve B then A: only B lands; chip hidden while stale", async () => {
  const calls = stub();
  const inst = mount({ c });
  submit(inst);
  assert.equal(calls.length, 1);
  type(inst, "usmca-hs", "8207.13"); // edit after A
  assert.equal(calls[0].signal.aborted, true);
  submit(inst);
  assert.equal(calls.length, 2);
  assert.equal(calls[1].body.good_hs, "8207.13");
  calls[1].resolve(res("met", "B"));
  await tick();
  assert.equal(result(inst).props.res.tag, "B");
  assert.equal(result(inst).props.stale, false);
  calls[0].resolve(res("not_met", "A")); // late, out of order
  await tick();
  assert.equal(result(inst).props.res.tag, "B");
  // any change to any query input hides the old chip at once
  tick_(inst, 0, true); // "list complete" tick (inputs order: complete, simple)
  assert.equal(result(inst).props.stale, true);
  tick_(inst, 0, false);
  assert.equal(result(inst).props.stale, false); // back to the facts it was computed for
  inst.unmount();
});

test("an old answer never lands on edited facts, even if it arrives after the edit", async () => {
  const calls = stub();
  const inst = mount({ c });
  submit(inst);
  type(inst, "usmca-hs", "9503.00");
  calls[0].resolve(res("met", "A"));
  await tick();
  assert.equal(result(inst), undefined);
  inst.unmount();
});

const select = (inst) => findAll(inst.tree, (el) => el.type === "select")[0];
const needsValue = { ...res("needs_information", "A"), missing_facts: [{ code: "rvc_method_required", field: "rvc_method", message: "m" }] };

test("every query input retires the old result at once: HS, method, amounts, materials, ticks", async () => {
  const edits = {
    hs: (i) => type(i, "usmca-hs", "8202.39"),
    complete: (i) => tick_(i, 0, true),
    simple: (i) => tick_(i, 1, true),
    material: (i) => findAll(i.tree, (el) => el.props?.variant === "ghost")[0].props.onClick(),
    method: (i) => select(i).props.onChange({ target: { value: "net_cost" } }),
  };
  for (const [name, edit] of Object.entries(edits)) {
    const calls = stub();
    const inst = mount({ c });
    submit(inst);
    calls[0].resolve(needsValue);
    await tick();
    assert.equal(result(inst).props.stale, false, name);
    edit(inst);
    assert.equal(result(inst).props.stale, true, name);
    inst.unmount();
  }
  // amounts and the aggregate value, once a method is chosen
  for (const id of ["usmca-amount", "usmca-vnm"]) {
    const calls = stub();
    const inst = mount({ c });
    submit(inst);
    calls[0].resolve(needsValue);
    await tick();
    select(inst).props.onChange({ target: { value: "net_cost" } });
    submit(inst);
    calls[1].resolve(needsValue);
    await tick();
    assert.equal(result(inst).props.stale, false, id);
    type(inst, id, "5");
    assert.equal(result(inst).props.stale, true, id);
    inst.unmount();
  }
});

test("0 is not unknown, bad input stays visible, TV and NC are separate fields", async () => {
  const calls = stub();
  const inst = mount({ c });
  assert.equal(findAll(inst.tree, (el) => el.type === "select").length, 0); // value fields not asked for yet
  submit(inst);
  calls[0].resolve(needsValue);
  await tick();
  select(inst).props.onChange({ target: { value: "transaction_value" } });
  type(inst, "usmca-amount", "100");
  select(inst).props.onChange({ target: { value: "net_cost" } });
  assert.equal(byId(inst, "usmca-amount").props.value, ""); // TV is not carried to NC
  select(inst).props.onChange({ target: { value: "transaction_value" } });
  assert.equal(byId(inst, "usmca-amount").props.value, "100"); // restored only for its own method
  type(inst, "usmca-amount", "1,0");
  type(inst, "usmca-vnm", "0");
  submit(inst);
  assert.equal(calls.length, 1); // invalid: nothing sent
  assert.equal(byId(inst, "usmca-amount").props.value, "1,0"); // raw input kept
  assert.equal(byId(inst, "usmca-amount").props["aria-invalid"], true);
  type(inst, "usmca-amount", "100");
  submit(inst);
  assert.equal(calls[1].body.vnm, "0");
  assert.equal(calls[1].body.transaction_value, "100");
  assert.equal("net_cost" in calls[1].body, false);
  inst.unmount();
});
