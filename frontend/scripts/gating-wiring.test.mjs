import "./render-support.mjs";
import assert from "node:assert/strict";
import { afterEach, test } from "node:test";
import { json, stubNavigation } from "./lib/hook-runtime.mjs";
import { deepMount, textOf } from "./lib/deep-mount.mjs";
import { createElement as h } from "react";

// #283: the gating WIRING, not just the helper. Each exposed write area is
// mounted for real (CaseShell + useCase where it is a case screen) and its
// control is read per case state: can_write false (gated), true, absent
// (unknown), and a pinned demo with can_write false (never gated, existing
// read-only journeys stay). A server 403 on an enabled control is handled.
globalThis.window = new EventTarget();
globalThis.document = new EventTarget();
stubNavigation({ id: "c1", search: "target=Vietnam" });
const REASON = /Your role in this workspace is viewer/;
const live = [];
afterEach(() => live.splice(0).forEach((a) => a.unmount()));

const caseJson = (extra = {}) => ({
  id: "c1", facts: "f", step: "origin", date: "2026-01-01", display_name: "Widget", family: "pcba_electronic_devices",
  prediction: { origin_country: "China", confidence: 0.8 },
  triage: { route: "auto" },
  product: {
    name: "W",
    plants: [{ id: "a", name: "A", country: "China" }, { id: "b", name: "B", country: "Vietnam" }, { id: "c", name: "C", country: "India" }],
    components: [{ name: "x", made_in: "China" }],
    operations: [],
  },
  ...extra,
});
const STATES = [
  ["can_write false", { can_write: false }, true],
  ["can_write true", { can_write: true }, false],
  ["can_write absent (unknown)", {}, false],
  ["pinned demo, can_write false", { pinned: true, can_write: false }, false],
];

// Serves the case; everything else is recorded and answered by `extra`.
function server(c, extra = () => null) {
  const calls = [];
  globalThis.fetch = async (url, init = {}) => {
    const u = String(url);
    const m = init.method ?? "GET";
    let body;
    try {
      body = typeof init.body === "string" ? JSON.parse(init.body) : undefined;
    } catch {}
    calls.push({ m, u, body });
    const r = extra(u, m, body);
    if (r) return r;
    if (/\/cases\/c1$/.test(u) && m === "GET") return json(c.current);
    if (u.endsWith("/plan/estimate")) return json({});
    if (u.includes("/demos")) return json({ demos: [] });
    if (u.endsWith("/api/session")) return json({ email: null });
    if (u.endsWith("/bom-version")) return json({ message: "file required" }, 400);
    return json({});
  };
  return calls;
}
async function screen(Step, extra) {
  const c = { current: caseJson(extra.case) };
  const calls = server(c, extra.serve);
  const el = Step();
  const app = deepMount(h(el.type, el.props));
  live.push(app);
  app.refresh();
  await app.flush();
  return Object.assign(app, { c, calls });
}
const check = (app, label, gated, why) => {
  const b = app.btn(label);
  assert.ok(b, `${why}: "${label}" is on screen`);
  if (gated) {
    assert.equal(b.props.disabled, true, `${why}: disabled`);
    assert.match(b.props.title ?? "", REASON, `${why}: plain reason`);
  } else {
    assert.doesNotMatch(b.props.title ?? "", REASON, `${why}: no viewer reason`);
    assert.notEqual(b.props.disabled, true, `${why}: not disabled by the gate`);
  }
};

const origin = (await import("../src/components/op/screens/origin-step.tsx")).OriginStep;
const scenarios = (await import("../src/components/op/screens/scenarios-view.tsx")).ScenariosView;
const compare = (await import("../src/components/op/screens/compare-view.tsx")).CompareView;
const plan = (await import("../src/components/op/screens/plan-step.tsx")).PlanStep;

for (const [name, extra, gated] of STATES) {
  test(`watch re-check [${name}]`, async () => {
    const app = await screen(origin, { case: extra });
    const b = app.btn("Re-check");
    assert.ok(b);
    assert.equal(Boolean(b.props.disabled), gated);
    assert.equal(REASON.test(b.props.title ?? ""), gated);
  });

  test(`answers re-run [${name}]`, async () => {
    const app = await screen(origin, { case: { ...extra, clarify: { questions: ["Where is it assembled?"] } } });
    if (extra.pinned) return assert.match(app.all().map((e) => textOf(e.props?.children)).join(" "), /pinned demo, so its answers stay/, "pinned keeps its existing note");
    const b = app.btn("re-run");
    assert.ok(b);
    assert.equal(REASON.test(b.props.title ?? ""), gated);
    assert.equal(b.props.disabled, true, "disabled with no typed answer in every state; the reason differs");
    if (!gated) {
      app.find((e) => e.type === "input")[0].props.onChange({ target: { value: "Vietnam" } });
      app.refresh();
      assert.notEqual(app.btn("re-run").props.disabled, true, "enabled once an answer is typed");
    } else {
      app.find((e) => e.type === "input")[0].props.onChange({ target: { value: "Vietnam" } });
      app.refresh();
      assert.equal(app.btn("re-run").props.disabled, true, "still disabled for a read-only case");
    }
  });

  test(`scenarios [${name}]`, async () => {
    const app = await screen(scenarios, { case: extra });
    check(app, "Predict the scenarios", gated, name);
  });

  test(`compare [${name}]`, async () => {
    const app = await screen(compare, { case: extra });
    if (extra.pinned) {
      // Existing demo behaviour: the recorded comparison, no run button. The gate adds nothing.
      assert.equal(app.btn("Compare these countries"), undefined);
      assert.equal(app.find((e) => REASON.test(e.props?.title ?? "")).length, 0);
      return;
    }
    check(app, "Compare these countries", gated, name);
  });

  test(`plan search [${name}]`, async () => {
    const app = await screen(plan, { case: extra });
    check(app, "Search moves to", gated, name);
  });
}

test("scenarios with saved scenarios: Edit scenarios is gated for a read-only case and open otherwise", async () => {
  const sc = [{ label: "s", origin_country: "China" }];
  for (const [name, extra, gated] of STATES) {
    const app = await screen(scenarios, { case: { ...extra, scenarios: sc } });
    const b = app.btn("Edit scenarios");
    assert.ok(b, name);
    assert.equal(Boolean(b.props.disabled), gated, name);
  }
});

test("compare 403 on an enabled control: honest message, one attempt, case re-read and now gated", async () => {
  const forbidden = () => json({ code: "forbidden", message: "your role in this workspace does not allow this; ask a workspace admin for editor access" }, 403);
  const app = await screen(compare, { case: { can_write: true }, serve: (u) => (u.endsWith("/compare-targets") ? forbidden() : null) });
  const gets = () => app.calls.filter((x) => x.m === "GET" && x.u.endsWith("/cases/c1")).length;
  const before = gets();
  app.c.current = caseJson({ can_write: false });
  await app.btn("Compare these countries").props.onClick();
  await app.flush();
  assert.equal(app.calls.filter((x) => x.u.endsWith("/compare-targets")).length, 1, "no retry loop");
  assert.ok(gets() > before, "the case was re-read");
  assert.equal(app.btn("Compare these countries").props.disabled, true);
  assert.match(app.all().map((e) => textOf(e.props?.children)).join(" "), /You can read this case but not change it/);
});

test("scenarios 403 on an enabled control is shown, not retried", async () => {
  const app = await screen(scenarios, { case: { can_write: true }, serve: (u) => (u.endsWith("/scenarios") ? json({ code: "forbidden", message: "your role in this workspace does not allow this; ask a workspace admin for editor access" }, 403) : null) });
  await app.btn("Predict the scenarios").props.onClick();
  await app.flush();
  assert.equal(app.calls.filter((x) => x.u.endsWith("/scenarios")).length, 1);
  assert.ok(app.find((e) => e.type?.name === "ErrorBox").length > 0);
});

test("pinned demo with can_write false: public Ask/scenario/plan journeys are not blocked by the UI", async () => {
  const app = await screen(scenarios, { case: { pinned: true, can_write: false } });
  check(app, "Predict the scenarios", false, "pinned scenarios");
  const p = await screen(plan, { case: { pinned: true, can_write: false } });
  check(p, "Search moves to", false, "pinned plan");
});

// ---- BOM version ----
const { BomVersion } = await import("../src/components/op/bom-version.tsx");
for (const [name, extra, gated] of STATES) {
  test(`bom-version [${name}]`, async () => {
    server({ current: {} });
    const app = deepMount(h(BomVersion, { id: "c1", c: caseJson(extra), reload() {} }));
    live.push(app);
    app.refresh();
    await app.flush();
    const input = app.find((e) => e.type === "input" && e.props.type === "file")[0];
    input.props.onChange({ target: { files: [new File(["x"], "b.csv")] } });
    app.refresh();
    const b = app.btn("Compare with this case");
    assert.ok(b, name);
    assert.equal(Boolean(b.props.disabled), gated, name);
    assert.equal(REASON.test(b.props.title ?? ""), gated, name);
  });
}

// ---- Cases home menu ----
const { YourCases } = await import("../src/components/op/screens/cases-home.tsx");
for (const [name, extra, gated] of STATES) {
  test(`cases menu rename/delete [${name}]`, async () => {
    server({ current: {} });
    const row = { id: "a", display_name: "A", product_name: "A", date: "2026-01-01", predicted_origin: "China", triage_route: "auto", confidence: 0.9, step: "origin", updated_at: "2026-01-01T00:00:00Z", watch_status: null, scenario_count: 0, pinned: false, ...extra };
    const app = deepMount(h(YourCases, { cases: [row], onChange() {} }));
    live.push(app);
    app.refresh();
    app.find((e) => e.type === "button" && e.props["aria-expanded"] !== undefined)[0].props.onClick();
    app.refresh();
    for (const label of ["Rename", "Delete case"]) {
      const b = app.find((e) => e.props?.role === "menuitem" && textOf(e.props.children).includes(label))[0];
      assert.ok(b, `${name}: ${label}`);
      assert.equal(Boolean(b.props.disabled), gated, `${name}: ${label}`);
    }
  });
}

// ---- Create / batch (workspace role, no case yet) ----
const productStep = (await import("../src/components/op/screens/product-step.tsx")).ProductStep;
const triage = (await import("../src/components/op/screens/triage-page.tsx")).TriagePage;
const withRole = (role) => (u) => {
  if (u.endsWith("/api/session")) return json({ email: "amy@x.com" });
  if (u.endsWith("/workspaces/mine")) return json({ email: "amy@x.com", invites: [], workspace: role ? { id: "w", name: "W", role, members: [], invited: [], admin: role === "admin", created_by: "a", created_at: "" } : null });
  return null;
};
for (const [role, gated] of [["viewer", true], ["editor", false], ["admin", false], [null, false]]) {
  test(`product creation [workspace role ${role}]`, async () => {
    const calls = server({ current: {} }, withRole(role));
    const app = deepMount(h(productStep, {}));
    app.calls = calls;
    live.push(app);
    app.refresh();
    await app.flush();
    const go = app.find((e) => e.type?.name === "Button" && e.props.disabled === true && REASON.test(e.props.title ?? ""));
    assert.equal(go.length > 0, gated, "the Find-origin buttons carry the viewer reason only for a viewer");
    if (!gated) assert.equal(app.find((e) => e.type?.name === "Button" && REASON.test(e.props.title ?? "")).length, 0);
  });

  test(`batch triage [workspace role ${role}]`, async () => {
    const calls = server({ current: {} }, withRole(role));
    const app = deepMount(h(triage, {}));
    app.calls = calls;
    live.push(app);
    app.refresh();
    await app.flush();
    const input = app.find((e) => e.type === "input" && e.props.type === "file")[0];
    await input.props.onChange({ target: { files: [new File(["name,facts\nA,a very long fact about a widget\n"], "p.csv", { type: "text/csv" })] } });
    await app.flush();
    const b = app.btn("Triage");
    assert.ok(b);
    assert.equal(Boolean(b.props.disabled), gated);
    assert.equal(REASON.test(b.props.title ?? ""), gated);
    // Even a forced click creates nothing for a viewer.
    await b.props.onClick();
    await app.flush();
    const created = app.calls.filter((x) => x.m === "POST" && x.u.endsWith("/cases/batch"));
    assert.equal(created.length, gated ? 0 : 1);
  });
}
