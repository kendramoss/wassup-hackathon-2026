import "./render-support.mjs";
import assert from "node:assert/strict";
import { test } from "node:test";
import { byName, findAll, instance, json, stubNavigation, tick } from "./lib/hook-runtime.mjs";
import { html, text } from "./render-support.mjs";

// #283 UI: per-case write gating from the server's can_write, with the real
// Draft screen (CaseShell + useCase), the Ask drawer and the Cases home menu.
// Gating is a convenience; one test proves a server 403 is handled when the
// control was enabled.
globalThis.window = new EventTarget();
stubNavigation({ id: "c1", search: "" });
const { DraftStep } = await import("../src/components/op/screens/draft-step.tsx");
const { AskDrawer } = await import("../src/components/op/ask-drawer.tsx");
const { ErrorBox } = await import("../src/components/op/chrome.tsx");
const { caseReadOnly } = await import("../src/lib/roles.ts");

const caseJson = (extra = {}) => ({ id: "c1", facts: "f", step: "export", date: "2026-01-01", prediction: { origin_country: "China" }, display_name: "Widget", product: { name: "W", plants: [], components: [], operations: [] }, ...extra });

test("caseReadOnly: only an explicit false on a non-pinned case; absent is unknown, pinned is never gated", () => {
  assert.equal(caseReadOnly({ can_write: false }), true);
  assert.equal(caseReadOnly({ can_write: false, pinned: false }), true);
  assert.equal(caseReadOnly({ can_write: true }), false);
  assert.equal(caseReadOnly({}), false, "unknown: nothing hidden, nothing granted; the server decides");
  assert.equal(caseReadOnly({ pinned: true, can_write: false }), false, "pinned demo journeys stay");
  assert.equal(caseReadOnly(null), false);
});

function mount(c, { exportPost } = {}) {
  const app = { calls: [] };
  let sched = false;
  const schedule = () => {
    if (sched) return;
    sched = true;
    queueMicrotask(() => {
      sched = false;
      app.refresh();
    });
  };
  const shellEl = DraftStep();
  const shell = instance(shellEl.type, schedule);
  const screen = instance(null, schedule);
  app.refresh = () => {
    for (let n = 0; n < 30 && (shell.dirty || screen.dirty || !shell.tree); n++) {
      shell.render(shellEl.props);
      const sEl = findAll(shell.tree, byName("Draft"))[0];
      if (!sEl) continue;
      screen.Component = sEl.type;
      screen.render(sEl.props);
    }
  };
  app.flush = async () => {
    for (let n = 0; n < 8; n++) {
      await tick();
      app.refresh();
    }
  };
  globalThis.fetch = async (url, init = {}) => {
    const u = String(url);
    const m = init.method ?? "GET";
    const body = init.body ? JSON.parse(init.body) : undefined;
    app.calls.push({ m, u, body });
    if (u.endsWith("/export") && m === "POST") {
      if (body.format === "cfr177") return json({ message: "off", code: "bad_request" }, 400);
      if (!body.preview && exportPost) return exportPost();
      return json({ content: "# letter", notes: [] });
    }
    if (u.endsWith("/cases/c1")) return json(c.current);
    return json({});
  };
  app.screen = screen;
  app.shell = shell;
  app.button = (label) => findAll(screen.tree, (e) => byName("Button")(e) && JSON.stringify(e.props.children).includes(label))[0];
  app.all = () => JSON.stringify(shell.tree) + JSON.stringify(screen.tree);
  app.recordings = () => app.calls.filter((x) => x.u.endsWith("/export") && x.m === "POST" && !x.body.preview);
  app.cases = () => app.calls.filter((x) => x.m === "GET" && x.u.endsWith("/cases/c1")).length;
  return app;
}
const holder = (extra) => ({ current: caseJson(extra) });

test("viewer case: Download and Print are disabled with the plain reason, preview still loads, nothing is recorded", async () => {
  const app = mount(holder({ can_write: false }));
  app.refresh();
  await app.flush();
  assert.ok(app.calls.some((x) => x.body?.preview === true), "the preview loaded");
  for (const label of ["Download Markdown", "Print the letter"]) {
    const b = app.button(label);
    assert.equal(b.props.disabled, true, label);
    assert.match(b.props.title, /Your role in this workspace is viewer/);
  }
  assert.match(app.all(), /Printing or downloading records the export, which needs editor access/);
  assert.match(app.all(), /You can read this case but not change it/, "the shell says why");
  await app.button("Download Markdown").props.onClick();
  await app.button("Print the letter").props.onClick();
  assert.equal(app.recordings().length, 0, "even a forced click sends no recording call");
});

for (const [name, extra] of [["can_write true", { can_write: true }], ["can_write absent (unknown)", {}], ["pinned demo with can_write false", { pinned: true, can_write: false }]]) {
  test(`${name}: the export controls are not disabled by the UI`, async () => {
    const app = mount(holder(extra));
    app.refresh();
    await app.flush();
    for (const label of ["Download Markdown", "Print the letter"]) assert.notEqual(app.button(label).props.disabled, true, label);
    assert.doesNotMatch(app.all(), /You can read this case but not change it/);
  });
}

test("a server 403 on a recording export while the control was enabled: honest message, nothing printed, the case is re-read", async () => {
  const c = holder({ can_write: true });
  let printed = 0;
  window.print = () => printed++;
  globalThis.print = window.print;
  const app = mount(c, { exportPost: () => json({ code: "forbidden", message: "your role in this workspace does not allow this; ask a workspace admin for editor access" }, 403) });
  app.refresh();
  await app.flush();
  const before = app.cases();
  c.current = caseJson({ can_write: false }); // the role changed meanwhile
  await app.button("Print the letter").props.onClick();
  await app.flush();
  assert.equal(printed, 0, "the letter is not printed");
  assert.equal(app.recordings().length, 1, "one attempt, no retry loop");
  assert.ok(app.cases() > before, "the case was re-read after the refusal");
  assert.equal(app.button("Print the letter").props.disabled, true, "now gated from the server's new answer");
  const err = findAll(app.screen.tree, byName("ErrorBox"))[0];
  assert.ok(err, "the refusal is shown");
  assert.equal(err.props.error.status, 403);
  assert.match(text(html(ErrorBox, { error: err.props.error })), /role in this workspace is viewer/);
});

test("Ask drawer: a read-only case cannot ask; editor, unknown and pinned can", () => {
  const props = (extra) => ({ c: caseJson(extra), onClose() {}, onChat() {} });
  const ro = html(AskDrawer, props({ can_write: false }));
  assert.match(ro, /Your role in this workspace is viewer/);
  assert.match(ro, /<input[^>]*\sdisabled=""/);
  for (const extra of [{ can_write: true }, {}, { pinned: true, can_write: false }]) {
    const ok = html(AskDrawer, props(extra));
    assert.doesNotMatch(ok, /Your role in this workspace is viewer/, JSON.stringify(extra));
    assert.doesNotMatch(ok, /<input[^>]*\sdisabled=""/, JSON.stringify(extra));
  }
});
