import "./render-support.mjs";
import assert from "node:assert/strict";
import { test } from "node:test";

// #349: a late AI estimate (cost-estimate) or duty prefill (money-prefill)
// must merge into the money inputs as they are when it lands, not replace them
// with what they were when it started. The real MoneyDrawer, EconomicsEditor,
// PrefillBar and EstimateCosts run against a tiny hook runtime (no DOM), with
// api.costEstimate / api.moneyPrefill held as deferred promises. Edits go
// through the components' real onChange / onClick props.

const React = await import("react");
const internals = React.__CLIENT_INTERNALS_DO_NOT_USE_OR_WARN_USERS_THEY_CANNOT_UPGRADE;
const { MoneyDrawer } = await import("../src/components/op/money-drawer.tsx");
const { EconomicsEditor } = await import("../src/components/op/plan-bits.tsx");
const { EstimateCosts, PrefillBar } = await import("../src/components/op/money-prefill.tsx");
const { api } = await import("../src/lib/api/client.ts");

const tick = () => new Promise((r) => setImmediate(r));
const flush = async () => {
  for (let n = 0; n < 5; n++) await tick();
};

function instance(Component, onDirty) {
  const inst = { hooks: [], i: 0, effects: [], mounted: true, tree: null, dirty: false, Component };
  inst.dispatcher = {
    useState(init) {
      const h = (inst.hooks[inst.i] ??= { v: typeof init === "function" ? init() : init });
      inst.i++;
      h.set ??= (n) => {
        if (!inst.mounted) return;
        const nv = typeof n === "function" ? n(h.v) : n;
        if (Object.is(nv, h.v)) return;
        h.v = nv;
        inst.dirty = true;
        onDirty();
      };
      return [h.v, h.set];
    },
    useRef(init) {
      return (inst.hooks[inst.i++] ??= { current: init });
    },
    useId() {
      return `:id${inst.i++}`;
    },
    useEffect(fn, deps) {
      const h = (inst.hooks[inst.i++] ??= { deps: undefined, first: true });
      const changed = h.first || !deps || !h.deps || deps.some((d, k) => !Object.is(d, h.deps[k]));
      if (changed) inst.effects.push({ h, fn, deps });
    },
  };
  inst.render = (props) => {
    inst.i = 0;
    inst.dirty = false;
    const prev = internals.H;
    internals.H = inst.dispatcher;
    try {
      inst.tree = inst.Component(props);
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
  return inst;
}

const walk = (node, visit) => {
  if (node == null || typeof node !== "object") return;
  if (Array.isArray(node)) return node.forEach((n) => walk(n, visit));
  if (!React.isValidElement(node)) return;
  visit(node);
  const p = node.props;
  walk(typeof p.children === "function" ? p.children(":f") : p.children, visit);
  walk(p.footer, visit);
};
const findAll = (tree, pred) => {
  const out = [];
  walk(tree, (el) => pred(el) && out.push(el));
  return out;
};
const textOf = (n) => {
  if (n == null || typeof n === "boolean") return "";
  if (typeof n === "string" || typeof n === "number") return ` ${n}`;
  if (Array.isArray(n)) return n.map(textOf).join("");
  return React.isValidElement(n) ? textOf(n.props.children) : "";
};

// Mounts Root (a host component) and one child of type `childName` found in
// its tree, re-rendering both until settled.
function mount(Root, rootProps, childName) {
  const app = { rootProps, child: null };
  let scheduled = false;
  const schedule = () => {
    if (scheduled) return;
    scheduled = true;
    queueMicrotask(() => {
      scheduled = false;
      app.refresh();
    });
  };
  const root = instance(Root, schedule);
  app.refresh = () => {
    for (let n = 0; n < 20 && (root.dirty || app.child?.dirty); n++) {
      if (root.dirty) root.render(app.rootProps);
      const el = findAll(root.tree, (e) => typeof e.type === "function" && e.type.name === childName)[0];
      if (el && !app.child) app.child = instance(el.type, schedule);
      if (!el && app.child) {
        app.child.unmount();
        app.child = null;
      }
      if (el) app.child.render(el.props);
    }
  };
  root.render(rootProps);
  root.dirty = true;
  app.refresh();
  app.root = root;
  app.setProps = (p) => {
    app.rootProps = { ...app.rootProps, ...p };
    root.dirty = true;
    app.refresh();
  };
  app.input = (label) => findAll(root.tree, (e) => e.type === "input" && e.props["aria-label"] === label)[0];
  app.type = (label, v) => {
    app.input(label).props.onChange({ target: { value: v } });
    app.refresh();
  };
  app.typeBy = (pred, v) => {
    findAll(root.tree, (e) => e.type === "input" && pred(e.props))[0].props.onChange({ target: { value: v } });
    app.refresh();
  };
  app.button = (text) => findAll(root.tree, (e) => (e.type === "button" || (typeof e.type === "function" && e.type.name === "Button")) && textOf(e).includes(text))[0];
  app.click = (text) => {
    app.button(text).props.onClick();
    app.refresh();
  };
  app.childButton = () => findAll(app.child.tree, (e) => e.type === "button")[0];
  app.childText = () => textOf(app.child.tree);
  app.unmount = () => {
    app.child?.unmount();
    root.unmount();
  };
  return app;
}

const deferred = () => {
  let resolve, reject;
  const promise = new Promise((a, b) => ((resolve = a), (reject = b)));
  return { promise, resolve, reject };
};
function hold(method) {
  const real = api[method];
  const calls = [];
  api[method] = (body) => {
    const d = deferred();
    calls.push({ body, ...d });
    return d.promise;
  };
  return { calls, restore: () => (api[method] = real) };
}

const product = (tag = "A") => ({
  name: `Heat gun ${tag}`,
  components: [
    { name: "housing", group: "housing", plant: "p1", made_in: "CN" },
    { name: "motor", group: "motor", plant: "p1", made_in: "CN" },
  ],
  plants: [
    { id: "p1", country: "CN", name: "Shenzhen" },
    { id: "p2", country: "VN", name: "Hanoi" },
  ],
});
const est = (group, plant, usd) => ({ group, plant, usd, low: usd - 1, high: usd + 1, reason: "r" });
const plantName = { p1: "Shenzhen", p2: "Hanoi" };
const typeCell = (app, g, p, v) => {
  const label = `${g} at ${plantName[p]}`.toLowerCase();
  findAll(app.root.tree, (e) => e.type === "input" && String(e.props["aria-label"] ?? "").toLowerCase().startsWith(label))[0].props.onChange({ target: { value: v } });
  app.refresh();
};
const importsInput = (pr) => pr.placeholder === "$2,000,000";

function drawer(value, p = product()) {
  const saved = [];
  const closed = [];
  const app = mount(MoneyDrawer, { value, product: p, countries: ["CN", "VN"], onSave: (e) => saved.push(e), onClose: () => closed.push(1) }, "EstimateCosts");
  app.saved = saved;
  app.closed = closed;
  app.toEach = () => app.click("Each group");
  app.save = () => {
    app.click("Save money inputs");
    return saved.at(-1);
  };
  return app;
}

test("MoneyDrawer: estimate held, manual cost + unrelated edit survive; only blank cells filled", async () => {
  const h = hold("costEstimate");
  try {
    const app = drawer({ annual_import_value_usd: 100000 });
    app.toEach();
    app.childButton().props.onClick();
    app.refresh();
    assert.equal(h.calls.length, 1);
    typeCell(app, "housing", "p1", "9000");
    app.typeBy(importsInput, "200000");
    h.calls[0].resolve({ estimates: [est("housing", "p1", 500), est("housing", "p2", 700), est("motor", "p1", 400)] });
    await flush();
    const out = app.save();
    assert.equal(out.annual_import_value_usd, 200000);
    assert.equal(out.group_costs.housing.p1, 9000);
    assert.equal(out.group_costs.housing.p2, 700);
    assert.equal(out.group_costs.motor.p1, 400);
    assert.match(app.childText(), /Filled 2 blank costs/);
    app.unmount();
  } finally {
    h.restore();
  }
});

test("MoneyDrawer: a typed 0 is a figure and stays; a cell cleared while waiting stays blank", async () => {
  const h = hold("costEstimate");
  try {
    const app = drawer({ annual_import_value_usd: 100000, group_costs: { motor: { p1: 5000 } } });
    app.childButton().props.onClick();
    app.refresh();
    typeCell(app, "housing", "p1", "0");
    typeCell(app, "motor", "p1", "");
    h.calls[0].resolve({ estimates: [est("housing", "p1", 500), est("motor", "p1", 450), est("motor", "p2", 600)] });
    await flush();
    const out = app.save();
    assert.equal(out.group_costs.housing.p1, 0);
    assert.equal(out.group_costs.motor.p1, undefined);
    assert.equal(out.group_costs.motor.p2, 600);
    app.unmount();
  } finally {
    h.restore();
  }
});

test("MoneyDrawer: untouched drawer still gets every estimate (no regression)", async () => {
  const h = hold("costEstimate");
  try {
    const app = drawer({ annual_import_value_usd: 100000, group_costs: { housing: { p1: 1234 } } });
    app.childButton().props.onClick();
    app.refresh();
    h.calls[0].resolve({ estimates: [est("housing", "p1", 500), est("housing", "p2", 700)] });
    await flush();
    const out = app.save();
    assert.equal(out.group_costs.housing.p1, 1234);
    assert.equal(out.group_costs.housing.p2, 700);
    assert.equal(out.annual_import_value_usd, 100000);
    app.unmount();
  } finally {
    h.restore();
  }
});

test("MoneyDrawer: late error keeps manual edits and shows the retry message", async () => {
  const h = hold("costEstimate");
  try {
    const app = drawer({ annual_import_value_usd: 100000 });
    app.toEach();
    app.childButton().props.onClick();
    app.refresh();
    typeCell(app, "housing", "p1", "9000");
    h.calls[0].reject(new Error("boom"));
    await flush();
    assert.match(app.childText(), /Couldn.t estimate/);
    assert.match(app.childButton().props.children.toString(), /Estimate the blank costs/);
    const out = app.save();
    assert.equal(out.group_costs.housing.p1, 9000);
    assert.equal(out.annual_import_value_usd, 100000);
    app.unmount();
  } finally {
    h.restore();
  }
});

test("EstimateCosts: a form dismissed before the estimate lands never receives it", async () => {
  const h = hold("costEstimate");
  try {
    const fills = [];
    const app = mount(EstimateCosts, { product: product(), economics: {}, onFill: (e) => fills.push(e) }, "__none__");
    app.root.tree.props.children[0].props.onClick();
    app.unmount();
    h.calls[0].resolve({ estimates: [est("housing", "p1", 500)] });
    await flush();
    assert.deepEqual(fills, []);
  } finally {
    h.restore();
  }
});

test("EstimateCosts: result for a superseded product (case change) is ignored, the new product's run applies", async () => {
  const h = hold("costEstimate");
  try {
    const fills = [];
    const A = product("A");
    const B = product("B");
    const app = mount(EstimateCosts, { product: A, economics: { annual_import_value_usd: 1 }, onFill: (e, c) => fills.push({ e, c }) }, "__none__");
    const click = () => app.root.tree.props.children[0].props.onClick();
    click();
    assert.equal(h.calls.length, 1);
    app.setProps({ product: B, economics: { annual_import_value_usd: 2 } });
    click(); // the new product's own run
    assert.equal(h.calls.length, 2);
    h.calls[0].resolve({ estimates: [est("housing", "p1", 111)] }); // stale, late
    await flush();
    assert.equal(fills.length, 0);
    h.calls[1].resolve({ estimates: [est("housing", "p1", 222)] });
    await flush();
    assert.equal(fills.length, 1);
    assert.equal(fills[0].e.group_costs.housing.p1, 222);
    assert.equal(fills[0].e.annual_import_value_usd, 2);
    app.unmount();
  } finally {
    h.restore();
  }
});

test("EstimateCosts: a superseded product's late error does not show a message on the new product", async () => {
  const h = hold("costEstimate");
  try {
    const app = mount(EstimateCosts, { product: product("A"), economics: {}, onFill: () => {} }, "__none__");
    app.root.tree.props.children[0].props.onClick();
    app.setProps({ product: product("B") });
    h.calls[0].reject(new Error("boom"));
    await flush();
    assert.equal(app.root.tree.props.children[1], null);
    assert.equal(app.root.tree.props.children[0].props.disabled, false);
    app.unmount();
  } finally {
    h.restore();
  }
});

test("EconomicsEditor (plan flow): duty rate, imports and cost edited while waiting survive", async () => {
  const h = hold("costEstimate");
  try {
    const p = product();
    let latest;
    const Host = () => {
      const [v, setV] = React.useState({ annual_import_value_usd: 100000 });
      latest = v;
      return EconomicsEditor({ value: v, onChange: setV, product: p, countries: ["CN", "VN"] });
    };
    const app = mount(Host, {}, "EstimateCosts");
    app.childButton().props.onClick();
    app.refresh();
    app.type("Duty rate for CN, percent", "25");
    app.typeBy((pr) => pr.inputMode === "numeric" && pr.id?.endsWith("-v"), "200000");
    typeCell(app, "housing", "p1", "9000");
    h.calls[0].resolve({ estimates: [est("housing", "p1", 500), est("motor", "p2", 800)] });
    await flush();
    app.refresh();
    assert.equal(latest.annual_import_value_usd, 200000);
    assert.equal(latest.duty_rates.CN, 0.25);
    assert.equal(latest.group_costs.housing.p1, 9000);
    assert.equal(latest.group_costs.motor.p2, 800);
    assert.equal(latest.illustrative, true);
    app.unmount();
  } finally {
    h.restore();
  }
});

test("PrefillBar (plan flow): duty prefill lands without overwriting edits made while waiting", async () => {
  const h = hold("moneyPrefill");
  try {
    const p = { ...product(), hts: "8516.31.00" };
    let latest;
    const filled = [];
    const Host = () => {
      const [v, setV] = React.useState({ annual_import_value_usd: 100000, group_costs: { housing: { p1: 10 } } });
      latest = v;
      return PrefillBar({ product: p, economics: v, ready: false, onFill: (e) => (filled.push(e), setV(e)) });
    };
    const app = mount(Host, {}, "__none__");
    app.root.tree.props.children.props.onClick();
    app.refresh();
    assert.equal(h.calls.length, 1);
    // The user edits while the lookup is out: imports, a new cost, a duty rate.
    app.setProps({});
    latest = null;
    const set = (fn) => {
      const hook = app.root.hooks.find((x) => x && "v" in x && x.v && typeof x.v === "object" && "annual_import_value_usd" in x.v);
      hook.set(fn(hook.v));
      app.refresh();
    };
    set((v) => ({ ...v, annual_import_value_usd: 200000, duty_rates: { CN: 0.3 }, group_costs: { housing: { p1: 10 }, motor: { p1: 9000 } } }));
    h.calls[0].resolve({
      economics: { ...h.calls[0].body.economics, duty_rates: { CN: 0.25, VN: 0.1 } },
      duty_sources: { CN: "table", VN: "table" },
      hts: "8516.31.00",
    });
    await flush();
    app.refresh();
    assert.equal(filled.length, 1);
    assert.equal(latest.annual_import_value_usd, 200000);
    assert.equal(latest.duty_rates.CN, 0.3);
    assert.equal(latest.duty_rates.VN, 0.1);
    assert.equal(latest.group_costs.motor.p1, 9000);
    app.unmount();
  } finally {
    h.restore();
  }
});

test("PrefillBar: a lookup that lands after the product changed is ignored", async () => {
  const h = hold("moneyPrefill");
  try {
    const fills = [];
    const app = mount(PrefillBar, { product: { ...product("A"), hts: "8516.31.00" }, economics: { annual_import_value_usd: 1 }, ready: false, onFill: (e) => fills.push(e) }, "__none__");
    app.root.tree.props.children.props.onClick();
    app.setProps({ product: { ...product("B"), hts: "8516.31.00" } });
    h.calls[0].resolve({ economics: { duty_rates: { CN: 0.25 } }, duty_sources: { CN: "table" } });
    await flush();
    assert.deepEqual(fills, []);
    app.unmount();
  } finally {
    h.restore();
  }
});
