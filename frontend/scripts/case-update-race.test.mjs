import "./render-support.mjs";
import assert from "node:assert/strict";
import { test } from "node:test";
import { byName, findAll, instance, json, stubNavigation, tick } from "./lib/hook-runtime.mjs";

// #363 (was #355): a plan search or draft preview that finishes after an Ask
// turn was saved must merge into the newest case, not the copy the screen
// rendered with. The real PlanStep/DraftStep, CaseShell and useCase run in a
// hook runtime with fetch held by the test; there is no DOM and no network.
stubNavigation({ id: "c1", search: "target=Vietnam" });
const { PlanStep } = await import("../src/components/op/screens/plan-step.tsx");
const { DraftStep } = await import("../src/components/op/screens/draft-step.tsx");

const T0 = { question: "q0", answer: "a0", at: "2026-01-01T00:00:00Z" };
const T1 = { question: "q1", answer: "a1", at: "2026-01-01T00:01:00Z" };
const PLAN = { target: "Vietnam", moves: [], description: "p" };
const caseJson = (extra = {}) => ({
  id: "c1", facts: "f", step: "origin", date: "2026-01-01", prediction: { origin_country: "China" }, display_name: "kept-name",
  product: { name: "W", plants: [{ id: "a", name: "A", country: "China" }, { id: "b", name: "B", country: "Vietnam" }], components: [], operations: [] },
  chat: [T0], ...extra,
});

// Mounts a step (the CaseShell element it returns) and the screens below it.
function mount(Step, screenName, innerName) {
  const app = {};
  let sched = false;
  const schedule = () => {
    if (sched) return;
    sched = true;
    queueMicrotask(() => {
      sched = false;
      app.refresh();
    });
  };
  const shellEl = Step();
  const shell = instance(shellEl.type, schedule);
  const screen = instance(null, schedule);
  const inner = instance(null, schedule);
  app.refresh = () => {
    for (let n = 0; n < 30 && (shell.dirty || screen.dirty || inner.dirty || !shell.tree); n++) {
      shell.render(shellEl.props);
      const sEl = findAll(shell.tree, byName(screenName))[0];
      if (!sEl) continue;
      screen.Component = sEl.type;
      screen.render(sEl.props);
      if (innerName) {
        inner.Component = screen.tree.type;
        inner.render(screen.tree.props);
      }
    }
  };
  app.flush = async () => {
    for (let n = 0; n < 6; n++) {
      await tick();
      app.refresh();
    }
  };
  app.shell = shell;
  app.inner = innerName ? inner : screen;
  app.drawer = () => findAll(shell.tree, byName("AskDrawer"))[0];
  app.tabs = () => findAll(shell.tree, byName("CaseTabs"))[0];
  // What AskDrawer does with a saved answer: onChat replaces the shell's chat.
  app.answerAsk = (chat) => {
    app.drawer().props.onChat(chat);
    app.refresh();
  };
  app.openAsk = async () => {
    app.tabs().props.onAsk();
    app.refresh();
    await app.flush();
  };
  app.current = () => findAll(shell.tree, byName(screenName))[0].props.c;
  return app;
}

test("Ask answered during a running plan search survives the plan completing", async () => {
  const app = mount(PlanStep, "PlanScreen", true);
  let planResolve;
  globalThis.fetch = async (url, init) => {
    const u = String(url), m = init?.method ?? "GET";
    if (u.endsWith("/plan/estimate")) return json({ estimate: null });
    if (u.endsWith("/plan") && m === "POST") return new Promise((res) => (planResolve = () => res(json({ plan: PLAN }))));
    if (u.endsWith("/cases/c1")) return json(caseJson());
    return json({});
  };
  app.refresh();
  await app.flush();
  findAll(app.inner.tree, (e) => typeof e.type === "function" && e.type.name === "Button" && e.props.onClick && !e.props.disabled)[0].props.onClick();
  app.refresh();
  await app.flush();
  assert.ok(planResolve, "the plan request is in flight");
  await app.openAsk();
  app.answerAsk([T0, T1]);
  assert.equal(app.current().chat.length, 2, "the answer is visible before the plan finishes");
  planResolve();
  await app.flush();
  assert.ok(app.current().plan, "the plan result is applied");
  assert.equal(app.current().step, "plan");
  assert.deepEqual(app.current().chat, [T0, T1], "the newer chat turn is kept");
  assert.equal(app.current().display_name, "kept-name", "unrelated case fields are kept");
});

test("control: Ask answered after the plan is done keeps both", async () => {
  const app = mount(PlanStep, "PlanScreen", true);
  globalThis.fetch = async (url, init) => {
    const u = String(url), m = init?.method ?? "GET";
    if (u.endsWith("/plan/estimate")) return json({ estimate: null });
    if (u.endsWith("/plan") && m === "POST") return json({ plan: PLAN });
    if (u.endsWith("/cases/c1")) return json(caseJson());
    return json({});
  };
  app.refresh();
  await app.flush();
  findAll(app.inner.tree, (e) => typeof e.type === "function" && e.type.name === "Button" && e.props.onClick && !e.props.disabled)[0].props.onClick();
  app.refresh();
  await app.flush();
  await app.openAsk();
  app.answerAsk([T0, T1]);
  assert.deepEqual(app.current().chat, [T0, T1]);
  assert.ok(app.current().plan);
});

test("a plan finishing after the case reached the export step keeps the export step", async () => {
  const app = mount(PlanStep, "PlanScreen", true);
  let planResolve;
  globalThis.fetch = async (url, init) => {
    const u = String(url), m = init?.method ?? "GET";
    if (u.endsWith("/plan/estimate")) return json({ estimate: null });
    if (u.endsWith("/plan") && m === "POST") return new Promise((res) => (planResolve = () => res(json({ plan: PLAN }))));
    if (u.endsWith("/cases/c1")) return json(caseJson());
    return json({});
  };
  app.refresh();
  await app.flush();
  findAll(app.inner.tree, (e) => typeof e.type === "function" && e.type.name === "Button" && e.props.onClick && !e.props.disabled)[0].props.onClick();
  app.refresh();
  await app.flush();
  // Another screen of the same case records the export while the search runs.
  findAll(app.shell.tree, byName("PlanScreen"))[0].props.setCase((d) => ({ ...d, step: "export" }));
  app.refresh();
  assert.equal(app.current().step, "export");
  planResolve();
  await app.flush();
  assert.ok(app.current().plan);
  assert.equal(app.current().step, "export", "the later export step is not reset to plan");
});

test("draft preview finishing after an Ask turn keeps the turn and moves to export", async () => {
  const app = mount(DraftStep, "Draft", false);
  let previewResolve;
  globalThis.fetch = async (url, init) => {
    const u = String(url), m = init?.method ?? "GET";
    if (u.endsWith("/export") && m === "POST") {
      const body = JSON.parse(init.body);
      if (body.format === "cfr177") return json({ message: "off", code: "bad_request" }, 400);
      return new Promise((res) => (previewResolve = () => res(json({ content: "# letter", notes: [] }))));
    }
    if (u.endsWith("/cases/c1")) return json(caseJson());
    return json({});
  };
  app.refresh();
  await app.flush();
  assert.ok(previewResolve, "the preview request is in flight");
  await app.openAsk();
  app.answerAsk([T0, T1]);
  previewResolve();
  await app.flush();
  assert.equal(app.current().step, "export");
  assert.deepEqual(app.current().chat, [T0, T1], "the Ask turn is kept");
});

test("draft preview on a case already at export leaves the case object untouched", async () => {
  const app = mount(DraftStep, "Draft", false);
  globalThis.fetch = async (url, init) => {
    const u = String(url), m = init?.method ?? "GET";
    if (u.endsWith("/export") && m === "POST") return json(JSON.parse(init.body).format === "cfr177" ? { message: "off", code: "bad_request" } : { content: "# letter", notes: [] }, JSON.parse(init.body).format === "cfr177" ? 400 : 200);
    if (u.endsWith("/cases/c1")) return json(caseJson({ step: "export" }));
    return json({});
  };
  app.refresh();
  await tick();
  await app.flush();
  const before = app.current();
  await app.flush();
  assert.equal(before.step, "export");
  assert.equal(app.current(), before, "a no-op update returns the existing object");
});
