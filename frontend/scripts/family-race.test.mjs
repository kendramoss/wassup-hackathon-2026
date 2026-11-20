import "./render-support.mjs";
import assert from "node:assert/strict";
import { register } from "node:module";
import { test } from "node:test";

// #351: an obsolete family check or import family suggestion must not override
// the latest explicit family choice. Same hook-shim style as import-race.test.mjs;
// the real Combobox onChange (pickFamily), FamilyNote onUse and BomImport wiring
// run with api.familyCheck / api.importBom held as deferred promises.




// The PDF reader needs PDF.js and a real file; swap it for a controllable one.
register(
  "data:text/javascript," +
    encodeURIComponent(`
export async function resolve(spec, ctx, next) {
  if (spec === "@/lib/pdf-bom") return { url: "data:text/javascript," + encodeURIComponent(${JSON.stringify(`
    export class PdfTableError extends Error { constructor(m, text) { super(m); this.text = text; } }
    export const pdfToBomCsv = (f) => globalThis.__pdfToBomCsv(f);
  `)}), shortCircuit: true };
  return next(spec, ctx);
}`),
);

const React = await import("react");
const internals = React.__CLIENT_INTERNALS_DO_NOT_USE_OR_WARN_USERS_THEY_CANNOT_UPGRADE;
const { ProductStep } = await import("../src/components/op/screens/product-step.tsx");
const { api } = await import("../src/lib/api/client.ts");

// ---- hook runtime -------------------------------------------------------
const tick = () => new Promise((r) => setImmediate(r));
function instance(Component, onDirty, strict = false) {
  const inst = { hooks: [], i: 0, effects: [], mounted: true, tree: null, dirty: false, Component, strict, rendered: false };
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
    const pending = inst.effects.splice(0);
    const run = (list) => {
      for (const e of list) {
        e.h.cleanup?.();
        e.h.first = false;
        e.h.deps = e.deps;
        e.h.cleanup = e.fn() ?? undefined;
      }
    };
    run(pending);
    // React strict mode: the first mount is set up, cleaned up and set up again.
    if (inst.strict && !inst.rendered) {
      for (const e of pending) {
        e.h.cleanup?.();
        e.h.cleanup = undefined;
      }
      run(pending);
    }
    inst.rendered = true;
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
  // Field passes its input as a render function of the id.
  const kids = node.props.children;
  walk(typeof kids === "function" ? kids(":field") : kids, visit);
};
const findAll = (tree, pred) => {
  const out = [];
  walk(tree, (el) => pred(el) && out.push(el));
  return out;
};
const textOf = (tree) => {
  let s = "";
  const go = (n) => {
    if (n == null || typeof n === "boolean") return;
    if (typeof n === "string" || typeof n === "number") s += ` ${n}`;
    else if (Array.isArray(n)) n.forEach(go);
    else if (React.isValidElement(n)) go(n.props.children);
  };
  go(tree);
  return s.replace(/\s+/g, " ").trim();
};

// ProductStep with its BomImport child, mounted while the tab is "bom".
function mountApp({ strict = false } = {}) {
  const app = { child: null, childUnmounts: 0 };
  const scheduled = { v: false };
  const parent = instance(ProductStep, () => schedule(), strict);
  function schedule() {
    if (scheduled.v) return;
    scheduled.v = true;
    queueMicrotask(() => {
      scheduled.v = false;
      app.refresh();
    });
  }
  app.refresh = () => {
    for (let n = 0; n < 20 && (parent.dirty || app.child?.dirty); n++) {
      if (parent.dirty) parent.render({});
      const el = findAll(parent.tree, (e) => typeof e.type === "function" && e.type.name === "BomImport")[0];
      if (el && !app.child) app.child = instance(el.type, schedule, strict);
      if (!el && app.child) {
        app.child.unmount();
        app.child = null;
        app.childUnmounts++;
      }
      if (el) app.child.render(el.props);
    }
  };
  parent.render({});
  parent.dirty = true; // lets refresh() mount the child
  app.refresh();
  app.parent = parent;
  app.flush = async () => {
    for (let n = 0; n < 5; n++) await tick();
  };
  app.text = () => textOf(parent.tree);
  app.childTree = () => app.child?.tree;
  app.spinner = () => findAll(app.childTree(), (e) => e.props?.label === "Importing…").length > 0;
  app.errorBox = () => findAll(app.childTree(), (e) => "error" in (e.props ?? {}) && typeof e.type === "function" && e.type.name === "ErrorBox")[0]?.props.error ?? null;
  app.dropLabel = () => textOf(findAll(app.childTree(), (e) => e.type === "span" && /\.(csv|pdf|xlsx)$/.test(textOf(e)))[0]);
  const pickWith = (file) => {
    const input = findAll(app.childTree(), (e) => e.type === "input" && e.props.type === "file")[0];
    input.props.onChange({ target: { files: [file] } });
  };
  const dropWith = (file) => {
    const label = findAll(app.childTree(), (e) => e.type === "label")[0];
    label.props.onDrop({ preventDefault() {}, dataTransfer: { files: [file] } });
  };
  app.select = (file) => {
    pickWith(file);
    app.refresh();
  };
  app.drop = (file) => {
    dropWith(file);
    app.refresh();
  };
  app.tab = (t) => {
    findAll(parent.tree, (e) => e.type === "button" && e.props.id === `step1-tab-${t}`)[0].props.onClick();
    app.refresh();
  };
  app.typeName = (v) => {
    findAll(parent.tree, (e) => e.type === "input" && e.props["data-field"] === "name")[0].props.onChange({ target: { value: v } });
    app.refresh();
  };
  app.goButton = () => findAll(parent.tree, (e) => typeof e.type === "function" && e.type.name === "Button" && "aria-busy" in e.props)[0];
  // What the next prediction would be sent with.
  app.submit = async () => {
    let body = null;
    const realFetch = globalThis.fetch;
    globalThis.fetch = async (url, init) => {
      body = JSON.parse(init.body);
      return new Response(JSON.stringify({ id: "c1" }), { status: 200, headers: { "content-type": "application/json" } });
    };
    try {
      app.goButton().props.onClick();
      app.refresh();
      await app.flush();
    } finally {
      globalThis.fetch = realFetch;
    }
    return body;
  };
  app.unmount = () => {
    app.child?.unmount();
    parent.unmount();
  };
  return app;
}

// ---- controlled boundaries ---------------------------------------------
const deferred = () => {
  let resolve, reject;
  const promise = new Promise((a, b) => ((resolve = a), (reject = b)));
  return { promise, resolve, reject };
};
const reply = (tag) => ({
  product: { name: tag, components: [{ name: `${tag}-part`, group: "g", plant: "p1", made_in: "VN" }], plants: [{ id: "p1", name: "P1", country: "VN" }], operations: [] },
  name_source: "file_name",
  family: `family_${tag}`,
  family_source: "file",
  warnings: [],
});

function boundaries() {
  const held = new Map(); // key -> deferred
  const hold = (key) => {
    const d = deferred();
    held.set(key, d);
    return d.promise;
  };
  const saved = { importBom: api.importBom, importCsv: api.importCsv, moneyPrefill: api.moneyPrefill };
  api.importBom = (form) => hold(`bom:${form.get("file").name}`);
  api.importCsv = (body) => hold(`csv:${body.csv}`);
  const calls = { prefill: 0 };
  api.moneyPrefill = async () => {
    calls.prefill++;
    throw new Error("no prefill in tests");
  };
  globalThis.__pdfToBomCsv = async (f) => ({ csv: `pdf:${f.name}` });
  return {
    ok: (key, tag) => held.get(key).resolve(reply(tag)),
    okWith: (key, r) => held.get(key).resolve(r),
    fail: (key, message) => held.get(key).reject(new Error(message)),
    has: (key) => held.has(key),
    calls,
    restore: () => Object.assign(api, saved),
  };
}
const csv = (n) => new File(["name,country\nx,VN"], n, { type: "text/csv" });

const settle = async (app) => app.flush();

// ---- #351 family tests ---------------------------------------------------
const checks = new Map();
function familyBoundaries() {
  const b = boundaries();
  checks.clear();
  const savedFc = api.familyCheck;
  api.familyCheck = (body) => {
    const d = deferred();
    checks.set(body.text, { ...d, body });
    return d.promise;
  };
  return { ...b, restore: () => (b.restore(), (api.familyCheck = savedFc)) };
}
async function fscenario(run) {
  const b = familyBoundaries();
  const app = mountApp();
  try {
    await run(app, b);
  } finally {
    app.unmount();
    b.restore();
  }
}
const combo = (app) => findAll(app.parent.tree, (e) => typeof e.type === "function" && e.type.name === "Combobox")[0];
const pick = async (app, v) => {
  combo(app).props.onChange(v);
  app.refresh();
  await app.flush();
};
const comboValue = (app) => combo(app).props.value;
const match = (family, confidence = 0.9) => ({ result: { family, confidence, reason: "r", typed_unclear: false } });
const noMatch = () => ({ result: { family: "none", confidence: 0.9, reason: "r", typed_unclear: false } });
const importReady = async (app, b) => {
  app.select(csv("f.csv"));
  await settle(app);
  b.ok("bom:f.csv", "X");
  await settle(app);
};

// The family note under the field, as the user reads it ("" when none shows).
const note = (app) => {
  const el = findAll(app.parent.tree, (e) => typeof e.type === "function" && e.type.name === "FamilyNote")[0];
  return el ? textOf(el.type(el.props)) : "";
};

test("custom A then B, A resolves after B: B stays authoritative, note and family", () =>
  fscenario(async (app) => {
    await pick(app, "hair dryer");
    await pick(app, "floodlight");
    assert.match(note(app), /Checking .\s*floodlight\s*./);
    checks.get("floodlight").resolve(match("led_lights"));
    await app.flush();
    assert.equal(comboValue(app), "led_lights");
    checks.get("hair dryer").resolve(match("lidar_modules"));
    await app.flush();
    assert.equal(comboValue(app), "led_lights");
    assert.match(note(app), /floodlight\s*. matched/);
    assert.doesNotMatch(note(app), /hair dryer/);
  }));

test("custom A then B, A resolves first: A is ignored, B applies, loading stays for B", () =>
  fscenario(async (app) => {
    await pick(app, "hair dryer");
    await pick(app, "floodlight");
    checks.get("hair dryer").resolve(match("lidar_modules"));
    await app.flush();
    assert.notEqual(comboValue(app), "lidar_modules", "A's match not applied");
    assert.match(note(app), /Checking .\s*floodlight\s*./, "B's loading note not cleared by A");
    checks.get("floodlight").resolve(match("led_lights"));
    await app.flush();
    assert.equal(comboValue(app), "led_lights");
  }));

test("custom A fails late after B succeeded: no error note", () =>
  fscenario(async (app) => {
    await pick(app, "hair dryer");
    await pick(app, "floodlight");
    checks.get("floodlight").resolve(match("led_lights"));
    await app.flush();
    checks.get("hair dryer").reject(new Error("boom"));
    await app.flush();
    assert.doesNotMatch(note(app), /Couldn.t check/);
    assert.equal(comboValue(app), "led_lights");
  }));

test("custom A fails first, B succeeds later: B applies, no error note", () =>
  fscenario(async (app) => {
    await pick(app, "hair dryer");
    await pick(app, "floodlight");
    checks.get("hair dryer").reject(new Error("boom"));
    await app.flush();
    assert.match(note(app), /Checking .\s*floodlight\s*./);
    checks.get("floodlight").resolve(match("led_lights"));
    await app.flush();
    assert.equal(comboValue(app), "led_lights");
    assert.doesNotMatch(note(app), /Couldn.t check/);
  }));

for (const [label, settleA] of [
  ["success", (d) => d.resolve(match("lidar_modules"))],
  ["none result", (d) => d.resolve(noMatch())],
  ["failure", (d) => d.reject(new Error("boom"))],
]) {
  test(`custom check then listed family, check ${label} late: listed family and no stale note`, () =>
    fscenario(async (app, b) => {
      await importReady(app, b);
      await pick(app, "hair dryer");
      assert.match(note(app), /Checking/);
      await pick(app, "led_lights");
      assert.equal(note(app), "", "listed selection clears the loading note");
      settleA(checks.get("hair dryer"));
      await app.flush();
      assert.equal(comboValue(app), "led_lights");
      assert.equal(note(app), "", "no stale success, none or error note");
      const body = await app.submit();
      assert.equal(body.family, "led_lights");
    }));

  test(`custom check then None, check ${label} late: stays None, submits no family, facts kept (#345)`, () =>
    fscenario(async (app, b) => {
      await importReady(app, b);
      await pick(app, "hair dryer");
      await pick(app, "");
      settleA(checks.get("hair dryer"));
      await app.flush();
      assert.equal(comboValue(app), "");
      assert.equal(note(app), "", "no stale success, none or error note");
      const body = await app.submit();
      assert.ok(!body.family, `family not submitted: ${JSON.stringify(body.family)}`);
      assert.deepEqual(body.product.components.map((c) => c.name), ["X-part"]);
    }));
}

test("a custom check sends the typed text and the current product (request body)", () =>
  fscenario(async (app, b) => {
    await importReady(app, b);
    await pick(app, "hair dryer");
    assert.equal(checks.get("hair dryer").body.text, "hair dryer");
    assert.deepEqual(checks.get("hair dryer").body.product.components.map((c) => c.name), ["X-part"]);
    checks.get("hair dryer").resolve(match("motorised_tools_small_appliances"));
    await app.flush();
    const body = await app.submit();
    assert.equal(body.family, "motorised_tools_small_appliances");
  }));

test("newest custom check still applies and submits when nothing supersedes it", () =>
  fscenario(async (app, b) => {
    await importReady(app, b);
    await pick(app, "floodlight");
    checks.get("floodlight").resolve(match("led_lights"));
    await app.flush();
    assert.equal((await app.submit()).family, "led_lights");
  }));

test("typing too-short text supersedes an in-flight check", () =>
  fscenario(async (app, b) => {
    await importReady(app, b);
    await pick(app, "hair dryer");
    await pick(app, "q");
    checks.get("hair dryer").resolve(match("lidar_modules"));
    await app.flush();
    assert.notEqual(comboValue(app), "lidar_modules");
    assert.match(note(app), /.\s*q\s*. doesn.t name a product type/);
  }));

test("import suggestion arriving after an explicit listed pick does not override it", () =>
  fscenario(async (app, b) => {
    app.select(csv("f.csv"));
    await settle(app);
    await pick(app, "led_lights");
    b.ok("bom:f.csv", "X");
    await settle(app);
    assert.equal(comboValue(app), "led_lights");
    assert.doesNotMatch(app.text(), /Product family/, "no import family note for a superseded suggestion");
    const body = await app.submit();
    assert.equal(body.family, "led_lights");
    assert.deepEqual(body.product.components.map((c) => c.name), ["X-part"]);
  }));

test("import suggestion arriving after an explicit None does not override it", () =>
  fscenario(async (app, b) => {
    app.select(csv("f.csv"));
    await settle(app);
    await pick(app, "");
    b.ok("bom:f.csv", "X");
    await settle(app);
    assert.equal(comboValue(app), "");
    assert.ok(!(await app.submit()).family);
  }));

test("import suggestion with no explicit pick still fills the family and note", () =>
  fscenario(async (app, b) => {
    await importReady(app, b);
    assert.equal(comboValue(app), "family_X");
    assert.match(app.text(), /Product family/);
    assert.equal((await app.submit()).family, "family_X");
  }));

test("import completes while an earlier check is in flight: import family wins, check is obsolete", () =>
  fscenario(async (app, b) => {
    await pick(app, "hair dryer");
    app.select(csv("f.csv"));
    await settle(app);
    b.ok("bom:f.csv", "X");
    await settle(app);
    assert.equal(comboValue(app), "family_X");
    assert.equal(note(app), "");
    checks.get("hair dryer").resolve(match("lidar_modules"));
    await app.flush();
    assert.equal(comboValue(app), "family_X");
    assert.equal((await app.submit()).family, "family_X");
  }));

test("check started after the file was picked beats the import suggestion", () =>
  fscenario(async (app, b) => {
    app.select(csv("f.csv"));
    await settle(app);
    await pick(app, "floodlight");
    b.ok("bom:f.csv", "X");
    await settle(app);
    assert.match(note(app), /Checking .\s*floodlight\s*./, "import does not clear the newer check");
    checks.get("floodlight").resolve(match("led_lights"));
    await app.flush();
    assert.equal(comboValue(app), "led_lights");
    assert.equal((await app.submit()).family, "led_lights");
  }));

test("a check that finishes after the page unmounted does nothing", () =>
  fscenario(async (app) => {
    await pick(app, "hair dryer");
    app.unmount();
    checks.get("hair dryer").resolve(match("motorised_tools_small_appliances"));
    await app.flush();
    checks.get("hair dryer").reject?.(new Error("late"));
  }));
