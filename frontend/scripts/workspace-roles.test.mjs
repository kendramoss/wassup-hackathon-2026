import "./render-support.mjs";
import assert from "node:assert/strict";
import { afterEach, test } from "node:test";
import { byName, findAll, instance, json, tick } from "./lib/hook-runtime.mjs";
import { html, text } from "./render-support.mjs";

// #283 UI: the workspace panel (role, member roles, remove, leave) driven
// through the real component in a hook runtime, with the server played by the
// test. Responses can be held (deferred) and settled by hand.
globalThis.window = new EventTarget();
const { WorkspacePanel } = await import("../src/components/op/workspace-panel.tsx");
const { ErrorBox } = await import("../src/components/op/chrome.tsx");
const { ApiError, ACCESS_STALE, api } = await import("../src/lib/api/client.ts");
const roles = await import("../src/lib/roles.ts");

const live = [];
afterEach(() => {
  // Unmount: drop each panel's window listener so tests don't hear each other.
  for (const inst of live.splice(0)) {
    inst.mounted = false;
    for (const h of inst.hooks) h?.cleanup?.();
  }
});
const textOf = (n) => (n == null || typeof n === "boolean" ? "" : typeof n !== "object" ? String(n) : Array.isArray(n) ? n.map(textOf).join("") : textOf(n.props?.children));

const ME = "amy@x.com";
function ws(role, over = {}) {
  const admin = role === "admin";
  return {
    id: "ws1",
    name: "Acme",
    members: [ME, "bob@x.com"],
    invited: ["cy@x.com"],
    created_by: ME,
    created_at: "2026-01-01T00:00:00Z",
    admin,
    role,
    ...(admin ? { member_roles: { [ME]: "admin", "bob@x.com": "editor" } } : {}),
    ...over,
  };
}
const mineOf = (workspace, invites = []) => ({ email: ME, workspace, invites });

// A fake server. routes: "METHOD /path" -> (init) => Response | Promise<Response>.
function mount({ mine, routes = {}, signedIn = true }) {
  const app = { calls: [], mine, routes };
  let sched = false;
  const schedule = () => {
    if (sched) return;
    sched = true;
    queueMicrotask(() => {
      sched = false;
      app.refresh();
    });
  };
  const inst = instance(WorkspacePanel, schedule);
  app.inst = inst;
  live.push(inst);
  app.refresh = () => {
    for (let n = 0; n < 10 && (n === 0 || inst.dirty); n++) inst.render({});
  };
  app.flush = async () => {
    for (let n = 0; n < 8; n++) {
      await tick();
      app.refresh();
    }
  };
  globalThis.fetch = async (url, init = {}) => {
    const u = String(url);
    if (u === "/api/session") return json({ email: signedIn ? ME : null });
    const method = init.method ?? "GET";
    const path = u.replace("/api/v1", "");
    app.calls.push({ method, path, headers: init.headers ?? {}, body: init.body === undefined ? undefined : JSON.parse(init.body) });
    const h = app.routes[`${method} ${path}`];
    if (h) return h(init);
    if (method === "GET" && path === "/workspaces/mine") return app.mine === 404 ? json({ code: "not_found", message: "not found" }, 404) : json(app.mine);
    throw new Error(`unexpected ${method} ${path}`);
  };
  app.markup = () => JSON.stringify(inst.tree);
  app.text = () => textOf(inst.tree).replace(/\s+/g, " ");
  app.btn = (label) => findAll(inst.tree, (e) => byName("Button")(e) && JSON.stringify(e.props.children).includes(label))[0];
  app.selects = () => findAll(inst.tree, (e) => e.type === "select");
  app.alert = () => findAll(inst.tree, (e) => e.props?.role === "alert")[0];
  app.click = async (label) => {
    app.btn(label).props.onClick();
    app.refresh();
    await app.flush();
  };
  app.start = async () => {
    app.refresh();
    await app.flush();
    return app;
  };
  return app;
}
const lastCall = (app, method) => app.calls.filter((c) => c.method === method).at(-1);
const mineGets = (app) => app.calls.filter((c) => c.method === "GET" && c.path === "/workspaces/mine").length;
const err = (status, code, message) => json({ code, message }, status);

test("admin sees their role, a role selector per member and Remove for others", async () => {
  const app = await mount({ mine: mineOf(ws("admin")) }).start();
  assert.match(app.text(), /Admin\./);
  assert.match(app.text(), /manage members and their roles/);
  assert.equal(app.selects().length, 2);
  assert.deepEqual(app.selects().map((s) => s.props.value), ["admin", "editor"]);
  assert.equal(findAll(app.inst.tree, (e) => byName("Button")(e) && JSON.stringify(e.props.children).includes("Remove")).length, 1, "no Remove for yourself");
  assert.ok(app.btn("Leave workspace"));
  assert.match(app.text(), /invited/, "pending invitations still listed");
});

test("a role change sends only {role}, no caller authority, then re-reads the server", async () => {
  const state = { ...ws("admin") };
  const app = mount({
    mine: mineOf(state),
    routes: {
      "PUT /workspaces/ws1/members/bob%40x.com/role": () => {
        state.member_roles = { ...state.member_roles, "bob@x.com": "viewer" };
        return json(state);
      },
    },
  });
  await app.start();
  const before = mineGets(app);
  app.selects()[1].props.onChange({ target: { value: "viewer" } });
  app.refresh();
  await app.flush();
  const put = lastCall(app, "PUT");
  assert.deepEqual(put.body, { role: "viewer" });
  assert.equal(put.path, "/workspaces/ws1/members/bob%40x.com/role");
  assert.equal(mineGets(app), before + 1, "refetched /workspaces/mine after the change");
  assert.equal(app.selects()[1].props.value, "viewer", "shows the server's role");
  assert.match(app.text(), /bob@x.com is now a viewer/);
});

test("while a change is in flight controls are disabled; a late answer is shown, not assumed", async () => {
  let settle;
  const app = mount({ mine: mineOf(ws("admin")), routes: { "PUT /workspaces/ws1/members/bob%40x.com/role": () => new Promise((r) => (settle = () => r(json(ws("admin"))))) } });
  await app.start();
  app.selects()[1].props.onChange({ target: { value: "admin" } });
  app.refresh();
  await app.flush();
  assert.ok(app.selects().every((s) => s.props.disabled), "selectors disabled while saving");
  assert.equal(app.selects()[1].props.value, "editor", "the selector still shows the server role until it answers");
  settle();
  await app.flush();
  assert.ok(app.selects().every((s) => !s.props.disabled));
});

test("role change errors: 400, 403 (re-reads), 404, 409 last admin", async () => {
  const cases = [
    [400, "invalid_input", /role isn't valid/],
    [403, "forbidden", /Only a workspace admin/],
    [404, "not_found", /not a member of this workspace any more/],
    [409, "conflict", /needs at least one admin\. Make another member an admin first/],
  ];
  for (const [status, code, re] of cases) {
    const app = mount({ mine: mineOf(ws("admin")), routes: { "PUT /workspaces/ws1/members/amy%40x.com/role": () => err(status, code, "server words") } });
    await app.start();
    const before = mineGets(app);
    app.selects()[0].props.onChange({ target: { value: "viewer" } });
    app.refresh();
    await app.flush();
    assert.match(app.alert().props.children, re, `status ${status}`);
    assert.ok(mineGets(app) >= before + 1, `status ${status}: re-read the role after the refusal`);
  }
});

test("a demoted admin: 403 on a role change re-reads and the controls go away", async () => {
  const state = { m: mineOf(ws("admin")) };
  const app = mount({
    mine: state.m,
    routes: {
      "PUT /workspaces/ws1/members/bob%40x.com/role": () => {
        app.mine = mineOf(ws("viewer"));
        return err(403, "forbidden", "only a workspace admin can do that");
      },
    },
  });
  await app.start();
  app.selects()[1].props.onChange({ target: { value: "admin" } });
  app.refresh();
  await app.flush();
  assert.equal(app.selects().length, 0);
  assert.match(app.text(), /Viewer\./);
  assert.ok(app.alert());
});

test("remove needs a confirmation, then sends DELETE and re-reads", async () => {
  const app = mount({
    mine: mineOf(ws("admin")),
    routes: { "DELETE /workspaces/ws1/members/bob%40x.com": () => ((app.mine = mineOf(ws("admin", { members: [ME], member_roles: { [ME]: "admin" } }))), json({ removed: "bob@x.com", workspace: app.mine.workspace })) },
  });
  await app.start();
  await app.click("Remove");
  assert.equal(app.calls.filter((c) => c.method === "DELETE").length, 0, "first click only asks");
  assert.match(app.text(), /lose access to every case in the workspace/);
  await app.click("Remove member");
  assert.equal(app.calls.filter((c) => c.method === "DELETE").length, 1);
  assert.equal(lastCall(app, "DELETE").body, undefined);
  assert.match(app.text(), /Removed bob@x.com/);
  assert.equal(app.selects().length, 1);
});

test("cancel on the confirmation sends nothing", async () => {
  const app = await mount({ mine: mineOf(ws("admin")) }).start();
  await app.click("Remove");
  await app.click("Cancel");
  assert.equal(app.calls.filter((c) => c.method === "DELETE").length, 0);
  assert.ok(!/alertdialog/.test(app.markup()));
});

test("leave: every member can; the last admin gets the 409 explanation", async () => {
  const app = mount({ mine: mineOf(ws("admin")), routes: { "DELETE /workspaces/ws1/members/amy%40x.com": () => err(409, "conflict", "last admin") } });
  await app.start();
  await app.click("Leave workspace");
  assert.match(app.text(), /Leave Acme\?/);
  await app.click("Leave workspace");
  assert.equal(lastCall(app, "DELETE").path, "/workspaces/ws1/members/amy%40x.com");
  assert.match(app.alert().props.children, /Make another member an admin first/);
  assert.ok(app.text().includes("Acme"), "still in the workspace");

  const ed = mount({ mine: mineOf(ws("editor")), routes: { "DELETE /workspaces/ws1/members/amy%40x.com": () => ((ed.mine = mineOf(null)), json({ removed: ME, workspace: null })) } });
  await ed.start();
  await ed.click("Leave workspace");
  await ed.click("Leave workspace");
  assert.match(ed.text(), /Create a team workspace/, "after leaving, the server says no workspace");
});

for (const role of ["viewer", "editor"]) {
  test(`${role}: role in plain words, no admin controls, but can leave`, async () => {
    const app = await mount({ mine: mineOf(ws(role)) }).start();
    assert.match(app.text(), role === "viewer" ? /Viewer\. You can read/ : /Editor\. You can read and change/);
    assert.equal(app.selects().length, 0);
    assert.equal(app.btn("Remove"), undefined);
    assert.equal(app.btn("Invite member"), undefined);
    assert.ok(app.btn("Leave workspace"));
    assert.ok(!/is now/.test(app.text()));
  });
}

test("no workspace: create form; invitations still join; teams off (404) shows nothing", async () => {
  const app = await mount({ mine: mineOf(null, [{ workspace_id: "w9", name: "Zed", invited_by: "z@x.com", created_at: "" }]) }).start();
  assert.match(app.text(), /Create a team workspace/);
  assert.match(app.text(), /z@x.com invited you to/);
  assert.equal(app.selects().length, 0);
  const off = await mount({ mine: 404 }).start();
  assert.equal(off.inst.tree, null);
  const anon = await mount({ mine: mineOf(null), signedIn: false }).start();
  assert.equal(anon.inst.tree, null);
  assert.equal(anon.calls.length, 0, "an anonymous visitor makes no workspace request");
});

test("a refused write elsewhere (ACCESS_STALE) re-reads the role", async () => {
  const app = await mount({ mine: mineOf(ws("editor")) }).start();
  const before = mineGets(app);
  app.mine = mineOf(ws("viewer"));
  window.dispatchEvent(new Event(ACCESS_STALE));
  await app.flush();
  assert.equal(mineGets(app), before + 1);
  assert.match(app.text(), /Viewer\./);
});

test("no request carries a role, workspace or tenant header; the only body role is the admin's change", async () => {
  const app = mount({ mine: mineOf(ws("admin")), routes: { "PUT /workspaces/ws1/members/bob%40x.com/role": () => json(ws("admin")), "DELETE /workspaces/ws1/members/bob%40x.com": () => json({ removed: "bob@x.com", workspace: ws("admin") }) } });
  await app.start();
  app.selects()[1].props.onChange({ target: { value: "viewer" } });
  await app.flush();
  await app.click("Remove");
  await app.click("Remove member");
  assert.ok(app.calls.length >= 4);
  for (const c of app.calls) {
    const keys = Object.keys(c.headers).map((k) => k.toLowerCase());
    assert.deepEqual(keys.filter((k) => /role|workspace|tenant|owner|x-/.test(k)), [], `${c.method} ${c.path}`);
    if (c.method !== "PUT") assert.equal(c.body, undefined);
    else assert.deepEqual(Object.keys(c.body), ["role"]);
  }
});

test("api client: a refused write (403 forbidden) raises ACCESS_STALE; a refused read does not; 401 unauthorized is not unauthenticated", async () => {
  let stale = 0;
  const on = () => stale++;
  window.addEventListener(ACCESS_STALE, on);
  const body = { code: "forbidden", message: "your role in this workspace does not allow this; ask a workspace admin for editor access" };
  globalThis.fetch = async () => json(body, 403);
  await assert.rejects(api.rename("c1", "x"), (e) => e instanceof ApiError && e.status === 403);
  await assert.rejects(api.ask("c1", { question: "q" }), ApiError);
  assert.equal(stale, 2);
  await assert.rejects(api.getCase("c1"), ApiError);
  assert.equal(stale, 2, "a GET never raises it");
  window.removeEventListener(ACCESS_STALE, on);
  globalThis.fetch = async () => json({ code: "unauthorized", message: "bad key" }, 401);
  const e = await api.getCase("c1").catch((x) => x);
  assert.equal(roles.isSigninRequired(e), false);
  globalThis.fetch = async () => json({ code: "unauthenticated", message: "sign in" }, 401);
  assert.equal(roles.isSigninRequired(await api.getCase("c1").catch((x) => x)), true);
});

test("ErrorBox: strict 401 shows sign-in required with the demos link and no retry; a role 403 is not the demo note", () => {
  const strict = text(html(ErrorBox, { error: new ApiError("sign in or use an API key", 401, "unauthenticated", false), onRetry: () => {} }));
  assert.match(strict, /Sign-in required/);
  assert.match(strict, /demos stay open/);
  assert.match(strict, /Open a demo case/);
  assert.doesNotMatch(strict, /Try again/, "no retry for a visitor with no sign-in");
  const viewer = text(html(ErrorBox, { error: new ApiError("your role in this workspace does not allow this; ask a workspace admin for editor access", 403, "forbidden", false), onRetry: () => {} }));
  assert.match(viewer, /role in this workspace is viewer/);
  assert.match(viewer, /Check my access again/);
  assert.doesNotMatch(viewer, /pinned demo/);
  const demo = text(html(ErrorBox, { error: new ApiError("this is a pinned demo case; start a new case to change it", 403, "forbidden", false) }));
  assert.match(demo, /pinned demo case/);
  assert.match(demo, /Start a new case from this demo/);
});

test("roles helpers: create is closed to viewers only; messages per status", () => {
  assert.equal(roles.canCreate("viewer"), false);
  assert.equal(roles.canCreate("editor"), true);
  assert.equal(roles.canCreate("admin"), true);
  assert.equal(roles.canCreate(null), true, "no workspace keeps owner behaviour");
  assert.match(roles.memberMessage(new ApiError("x", 409, "conflict", false), "leave"), /at least one admin/);
  assert.match(roles.memberMessage(new ApiError("x", 401, "unauthenticated", false), "role"), /Sign in again/);
});
