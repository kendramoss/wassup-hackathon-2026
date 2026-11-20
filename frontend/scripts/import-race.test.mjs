import "./render-support.mjs";
import assert from "node:assert/strict";
import { register } from "node:module";
import { test } from "node:test";

// #346: overlapping BOM uploads. Only the latest selected file may fill the
// product, the name/family note, the busy spinner and the error box. The test
// runs the real ProductStep and BomImport function components (their real
// onChange/onDrop/onImported/onFailed wiring) against a tiny hook runtime, with
// api.importBom / api.importCsv held as deferred promises. There is no DOM:
// state setters re-render synchronously on the next tick and effects run after
// each render, which is enough for these components.

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
const pdf = (n) => new File(["%PDF"], n, { type: "application/pdf" });

async function scenario(run, opts) {
  const b = boundaries();
  const app = mountApp(opts);
  try {
    await run(app, b);
  } finally {
    app.unmount();
    b.restore();
  }
}
const settle = async (app) => app.flush();

// A then B, B first then A (the reported bug), CSV and PDF, plus mixed.
const kinds = [
  ["csv/csv", csv("older.csv"), csv("newer.csv"), "bom:older.csv", "bom:newer.csv"],
  ["pdf/pdf", pdf("older.pdf"), pdf("newer.pdf"), "csv:pdf:older.pdf", "csv:pdf:newer.pdf"],
  ["csv then pdf", csv("older.csv"), pdf("newer.pdf"), "bom:older.csv", "csv:pdf:newer.pdf"],
  ["pdf then csv", pdf("older.pdf"), csv("newer.csv"), "csv:pdf:older.pdf", "bom:newer.csv"],
];

for (const [label, fa, fb, ka, kb] of kinds) {
  test(`${label}: B finishes first, A last: product, name, family, note and prediction stay B`, () =>
    scenario(async (app, b) => {
      app.select(fa);
      await settle(app);
      app.select(fb);
      await settle(app);
      assert.ok(b.has(ka) && b.has(kb), "both imports were requested");
      assert.equal(app.spinner(), true);
      b.ok(kb, "NEWER");
      await settle(app);
      assert.equal(app.spinner(), false);
      b.ok(ka, "OLDER");
      await settle(app);
      assert.equal(app.spinner(), false);
      assert.equal(app.dropLabel(), fb.name);
      assert.match(app.text(), /NEWER/);
      assert.match(app.text(), /family_NEWER|Product family/i);
      assert.doesNotMatch(app.text(), /OLDER/);
      const body = await app.submit();
      assert.equal(body.product.name, "NEWER");
      assert.deepEqual(body.product.components.map((c) => c.name), ["NEWER-part"]);
      assert.equal(body.family, "family_NEWER");
      assert.equal(body.display_name, "NEWER");
    }));

  test(`${label}: A finishes first, then B: B replaces A`, () =>
    scenario(async (app, b) => {
      app.select(fa);
      await settle(app);
      app.select(fb);
      await settle(app);
      b.ok(ka, "OLDER");
      await settle(app);
      assert.equal(app.spinner(), true, "A's completion must not clear B's busy state");
      assert.doesNotMatch(app.text(), /OLDER/, "A's result is not applied while B is the latest");
      b.ok(kb, "NEWER");
      await settle(app);
      assert.equal(app.spinner(), false);
      const body = await app.submit();
      assert.equal(body.product.name, "NEWER");
      assert.equal(body.family, "family_NEWER");
    }));

  test(`${label}: A fails after B succeeded: no error, B stays usable`, () =>
    scenario(async (app, b) => {
      app.select(fa);
      await settle(app);
      app.select(fb);
      await settle(app);
      b.ok(kb, "NEWER");
      await settle(app);
      b.fail(ka, "older failed");
      await settle(app);
      assert.equal(app.errorBox(), null);
      assert.equal(app.goButton().props["aria-disabled"], false);
      const body = await app.submit();
      assert.equal(body.product.name, "NEWER");
    }));

  test(`${label}: A fails while B is pending: no error and B keeps its spinner`, () =>
    scenario(async (app, b) => {
      app.select(fa);
      await settle(app);
      app.select(fb);
      await settle(app);
      b.fail(ka, "older failed");
      await settle(app);
      assert.equal(app.errorBox(), null);
      assert.equal(app.spinner(), true);
      b.ok(kb, "NEWER");
      await settle(app);
      assert.equal(app.spinner(), false);
      assert.equal((await app.submit()).product.name, "NEWER");
    }));

  test(`${label}: B fails, A succeeds late: B's error stands, nothing from A is applied`, () =>
    scenario(async (app, b) => {
      app.select(fa);
      await settle(app);
      app.select(fb);
      await settle(app);
      b.fail(kb, "newer failed");
      await settle(app);
      assert.match(String(app.errorBox()?.message), /newer failed/);
      b.ok(ka, "OLDER");
      await settle(app);
      assert.match(String(app.errorBox()?.message), /newer failed/);
      assert.equal(app.spinner(), false);
      assert.doesNotMatch(app.text(), /OLDER/);
      assert.equal(app.goButton().props["aria-disabled"], true, "latest import failed: not ready");
    }));
}

test("a dropped file is an upload too: drop B over a pending A", () =>
  scenario(async (app, b) => {
    app.select(csv("older.csv"));
    await settle(app);
    app.drop(csv("newer.csv"));
    await settle(app);
    b.ok("bom:newer.csv", "NEWER");
    b.ok("bom:older.csv", "OLDER");
    await settle(app);
    assert.equal((await app.submit()).product.name, "NEWER");
  }));

test("a stale error cannot overwrite an earlier error that the latest upload cleared", () =>
  scenario(async (app, b) => {
    app.select(csv("bad.csv"));
    await settle(app);
    b.fail("bom:bad.csv", "bad rows");
    await settle(app);
    assert.match(String(app.errorBox()?.message), /bad rows/);
    app.select(csv("older.csv"));
    await settle(app);
    assert.equal(app.errorBox(), null, "a new file clears the previous error");
    app.select(csv("newer.csv"));
    await settle(app);
    b.ok("bom:newer.csv", "NEWER");
    await settle(app);
    b.fail("bom:older.csv", "older failed");
    await settle(app);
    assert.equal(app.errorBox(), null);
  }));

test("a PDF still being read when B is chosen never sends its import", () =>
  scenario(async (app, b) => {
    const read = deferred();
    globalThis.__pdfToBomCsv = (f) => (f.name === "older.pdf" ? read.promise : Promise.resolve({ csv: `pdf:${f.name}` }));
    app.select(pdf("older.pdf"));
    await settle(app);
    app.select(csv("newer.csv"));
    await settle(app);
    b.ok("bom:newer.csv", "NEWER");
    await settle(app);
    read.resolve({ csv: "pdf:older.pdf" });
    await settle(app);
    assert.equal(b.has("csv:pdf:older.pdf"), false);
    assert.equal(app.spinner(), false);
    assert.equal((await app.submit()).product.name, "NEWER");
  }));

test("a PDF text-only failure from an obsolete upload does not offer its text", () =>
  scenario(async (app, b) => {
    const read = deferred();
    globalThis.__pdfToBomCsv = (f) => (f.name === "older.pdf" ? read.promise : Promise.resolve({ csv: `pdf:${f.name}` }));
    app.select(pdf("older.pdf"));
    await settle(app);
    app.select(csv("newer.csv"));
    await settle(app);
    b.ok("bom:newer.csv", "NEWER");
    await settle(app);
    read.reject(new Error("no table"));
    await settle(app);
    assert.equal(app.errorBox(), null);
  }));

// ---- invalidation boundaries -------------------------------------------
test("leaving the Import tab abandons the pending upload: it cannot overwrite the parts the user edits", () =>
  scenario(async (app, b) => {
    app.select(csv("older.csv"));
    await settle(app);
    app.tab("parts");
    assert.equal(app.child, null, "BomImport is unmounted off the Import tab");
    b.ok("bom:older.csv", "OLDER");
    await settle(app);
    assert.doesNotMatch(app.text(), /OLDER/);
    app.tab("bom");
    assert.equal(app.spinner(), false, "a fresh Import tab is idle");
    assert.equal(app.errorBox(), null);
    assert.equal(app.goButton().props["aria-disabled"], true, "nothing imported");
  }));

test("an upload abandoned by leaving the tab cannot clear or fail the busy state of an upload made after returning", () =>
  scenario(async (app, b) => {
    app.select(csv("older.csv"));
    await settle(app);
    app.tab("paste");
    app.tab("bom");
    app.select(csv("newer.csv"));
    await settle(app);
    b.fail("bom:older.csv", "older failed");
    await settle(app);
    assert.equal(app.errorBox(), null);
    assert.equal(app.spinner(), true);
    b.ok("bom:newer.csv", "NEWER");
    await settle(app);
    assert.equal((await app.submit()).product.name, "NEWER");
  }));

test("leaving the tab and failing late does not mark the next visit as a failed import", () =>
  scenario(async (app, b) => {
    app.select(csv("older.csv"));
    await settle(app);
    app.tab("parts");
    b.fail("bom:older.csv", "older failed");
    await settle(app);
    app.tab("bom");
    assert.equal(app.errorBox(), null);
    assert.doesNotMatch(app.text(), /didn't import/);
  }));

test("typing the product name while a file imports does not cancel it (existing behaviour kept)", () =>
  scenario(async (app, b) => {
    app.select(csv("older.csv"));
    await settle(app);
    app.typeName("My heat gun");
    b.ok("bom:older.csv", "OLDER");
    await settle(app);
    const body = await app.submit();
    assert.equal(body.product.components[0].name, "OLDER-part");
    assert.equal(body.display_name, "My heat gun", "the user's typed name still wins");
  }));

test("an unmounted page (navigation or case change) ignores a late completion", () =>
  scenario(async (app, b) => {
    app.select(csv("older.csv"));
    await settle(app);
    app.unmount();
    b.ok("bom:older.csv", "OLDER");
    await settle(app);
    assert.equal(b.calls.prefill, 0, "the parent's onImported never ran");
  }));

test("a late completion after the user left the Import tab does not reach the parent", () =>
  scenario(async (app, b) => {
    app.select(csv("older.csv"));
    await settle(app);
    app.tab("parts");
    b.ok("bom:older.csv", "OLDER");
    await settle(app);
    assert.equal(b.calls.prefill, 0, "the parent's onImported never ran");
  }));

test("React strict mode (effects set up, cleaned up, set up again) leaves uploads working", () =>
  scenario(
    async (app, b) => {
      app.select(csv("older.csv"));
      await settle(app);
      app.select(csv("newer.csv"));
      await settle(app);
      b.ok("bom:newer.csv", "NEWER");
      b.ok("bom:older.csv", "OLDER");
      await settle(app);
      assert.equal((await app.submit()).product.name, "NEWER");
    },
    { strict: true },
  ));

// ---- #349: the duty lookup that follows an import ----------------------
const withMoney = (tag, hts, imports) => {
  const r = reply(tag);
  return { ...r, product: { ...r.product, hts }, economics: { illustrative: true, annual_import_value_usd: imports }, economics_sources: { annual_import_value_usd: "file" } };
};
// Holds api.moneyPrefill per product name; answers with the rates given.
function holdLookups() {
  const calls = [];
  api.moneyPrefill = (body) => {
    const d = deferred();
    calls.push({ body, ...d });
    return d.promise;
  };
  const rates = (call, duty, hts, sources) =>
    call.resolve({ economics: { ...call.body.economics, duty_rates: duty }, duty_sources: sources ?? Object.fromEntries(Object.keys(duty).map((c) => [c, "table"])), hts });
  return { calls, rates };
}
// The lookup note's figures (PrefillNote is a component: read its props).
const noteDuty = (app) => findAll(app.parent.tree, (e) => typeof e.type === "function" && e.type.name === "PrefillNote")[0]?.props.f;
const openMoney = (app) => {
  findAll(app.parent.tree, (e) => e.type === "button" && /money inputs/.test(textOf(e)))[0].props.onClick();
  app.refresh();
  return findAll(app.parent.tree, (e) => typeof e.type === "function" && e.type.name === "MoneyDrawer")[0];
};
const saveMoney = (app, e) => {
  openMoney(app).props.onSave(e);
  app.refresh();
};

test("duty lookup after an import: money, rates and costs typed while it is out survive; blank countries still fill", () =>
  scenario(async (app, b) => {
    const l = holdLookups();
    app.select(csv("a.csv"));
    await settle(app);
    b.okWith("bom:a.csv", withMoney("A", "8516.31.00", 100000));
    await settle(app);
    assert.equal(l.calls.length, 1);
    saveMoney(app, { illustrative: true, annual_import_value_usd: 200000, duty_rates: { CN: 0.3 }, group_costs: { g: { p1: 9000 } } });
    l.rates(l.calls[0], { CN: 0.25, VN: 0.1 }, "8516.31.00");
    await settle(app);
    const body = await app.submit();
    assert.equal(body.economics.annual_import_value_usd, 200000);
    assert.equal(body.economics.duty_rates.CN, 0.3);
    assert.equal(body.economics.duty_rates.VN, 0.1);
    assert.equal(body.economics.group_costs.g.p1, 9000);
  }));

test("duty lookup with no edits still lands the rates (no regression)", () =>
  scenario(async (app, b) => {
    const l = holdLookups();
    app.select(csv("a.csv"));
    await settle(app);
    b.okWith("bom:a.csv", withMoney("A", "8516.31.00", 100000));
    await settle(app);
    l.rates(l.calls[0], { CN: 0.25 }, "8516.31.00");
    await settle(app);
    const body = await app.submit();
    assert.equal(body.economics.annual_import_value_usd, 100000);
    assert.equal(body.economics.duty_rates.CN, 0.25);
  }));

test("upload A, import B, A's lookup lands late: B's money, HTS and note are not touched", () =>
  scenario(async (app, b) => {
    const l = holdLookups();
    app.select(csv("a.csv"));
    await settle(app);
    b.okWith("bom:a.csv", withMoney("A", "8516.31.00", 100000));
    await settle(app);
    app.select(csv("b.csv"));
    await settle(app);
    b.okWith("bom:b.csv", withMoney("B", "8516.99.00", 555));
    await settle(app);
    assert.equal(l.calls.length, 2);
    l.rates(l.calls[0], { CN: 0.25 }, "9999.99.99", { CN: "table" }); // A, late
    await settle(app);
    assert.equal(noteDuty(app), undefined, "A's late lookup sets no note");
    l.rates(l.calls[1], { VN: 0.1 }, "8516.99.00"); // B
    await settle(app);
    assert.deepEqual(noteDuty(app).duty.map((d) => d.country), ["VN"]);
    const body = await app.submit();
    assert.equal(body.economics.annual_import_value_usd, 555);
    assert.deepEqual(body.economics.duty_rates, { VN: 0.1 });
    assert.doesNotMatch(JSON.stringify(body), /9999\.99|8516\.31/);
  }));

test("a lookup that fails or lands for a replaced file leaves the new file's note alone", () =>
  scenario(async (app, b) => {
    const l = holdLookups();
    app.select(csv("a.csv"));
    await settle(app);
    b.okWith("bom:a.csv", withMoney("A", "8516.31.00", 100000));
    await settle(app);
    app.select(csv("b.csv"));
    await settle(app);
    l.calls[0].reject(new Error("lookup down")); // A fails after B was picked
    await settle(app);
    assert.equal(noteDuty(app), undefined, "A's failure sets no note on B");
  }));

test("an unmounted page ignores a late duty lookup", () =>
  scenario(async (app, b) => {
    const l = holdLookups();
    app.select(csv("a.csv"));
    await settle(app);
    b.okWith("bom:a.csv", withMoney("A", "8516.31.00", 100000));
    await settle(app);
    app.unmount();
    l.rates(l.calls[0], { CN: 0.25 }, "8516.31.00");
    await settle(app); // nothing throws and nothing is set on the dead page
  }));
