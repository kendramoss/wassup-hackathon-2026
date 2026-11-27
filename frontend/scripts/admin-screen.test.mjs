import "./render-support.mjs";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { afterEach, test } from "node:test";
import { byName, findAll, instance, json, tick } from "./lib/hook-runtime.mjs";

// #870: the workspace admin screen. The data handling (who sees it, the key
// shown once, empty and off states, filters) is tested through the real
// components in a hook runtime, with the server played by the test.
globalThis.window = new EventTarget();
const screen = await import("../src/components/op/screens/admin-screen.tsx");
const signin = await import("../src/components/op/screens/admin-signin.tsx");
const admin = await import("../src/lib/admin.ts");
const { ApiError } = await import("../src/lib/api/client.ts");
const { WorkspacePanel } = await import("../src/components/op/workspace-panel.tsx");

const live = [];
// The hook runtime renders one component, not its children. A child element of
// a rendered tree is mounted here with the same routes and call log, so the
// tests can drive it as the page would.
const sub = async (app, name, nth = 0) => {
  app.refresh();
  const el = findAll(app.inst.tree, byName(name))[nth];
  assert.ok(el, `no <${name}> in the tree`);
  return mount(el.type, el.props, app.routes, { calls: app.calls }).start();
};
afterEach(() => {
  for (const inst of live.splice(0)) {
    inst.mounted = false;
    for (const h of inst.hooks) h?.cleanup?.();
  }
});
const textOf = (n) => (n == null || typeof n === "boolean" ? "" : typeof n !== "object" ? String(n) : Array.isArray(n) ? n.map(textOf).join("") : textOf(n.props?.children));
const err = (status, code = "forbidden", message = "no") => json({ code, message }, status);

function mount(Component, props, routes = {}, { signedIn = true, calls = [] } = {}) {
  const app = { calls, routes };
  let sched = false;
  const inst = instance(Component, () => {
    if (sched) return;
    sched = true;
    queueMicrotask(() => {
      sched = false;
      app.refresh();
    });
  });
  live.push(inst);
  app.inst = inst;
  app.refresh = () => {
    for (let n = 0; n < 10 && (n === 0 || inst.dirty); n++) inst.render(props);
  };
  app.flush = async () => {
    for (let n = 0; n < 8; n++) {
      await tick();
      app.refresh();
    }
  };
  globalThis.fetch = async (url, init = {}) => {
    const u = String(url);
    if (u === "/api/session") return json({ email: signedIn ? "amy@x.com" : null });
    const method = init.method ?? "GET";
    const path = u.replace("/api/v1", "");
    app.calls.push({ method, path, body: init.body === undefined ? undefined : JSON.parse(init.body) });
    const h = app.routes[`${method} ${path}`];
    if (!h) throw new Error(`unexpected ${method} ${path}`);
    return typeof h === "function" ? h(init) : h.clone();
  };
  app.text = () => textOf(inst.tree).replace(/\s+/g, " ");
  app.btn = (label) => findAll(inst.tree, (e) => byName("Button")(e) && textOf(e.props.children).trim() === label)[0];
  app.has = (name) => findAll(inst.tree, byName(name)).length > 0;
  app.alert = () => findAll(inst.tree, (e) => e.props?.role === "alert")[0];
  app.click = async (label) => {
    app.btn(label).props.onClick();
    app.refresh();
    await app.flush();
  };
  app.submit = async (nth = 0) => {
    findAll(inst.tree, (e) => e.type === "form")[nth].props.onSubmit({ preventDefault() {} });
    app.refresh();
    await app.flush();
  };
  app.input = (placeholderOrLabel) =>
    findAll(inst.tree, (e) => e.type === "input" && (e.props.placeholder === placeholderOrLabel || e.props["aria-label"] === placeholderOrLabel))[0];
  app.start = async () => {
    app.refresh();
    await app.flush();
    return app;
  };
  return app;
}

const key = (id, over = {}) => ({ id, name: `key ${id}`, created_at: "2026-11-27T10:00:00Z", daily_quota: 1000, disabled: false, ...over });
const WS = { id: "ws1", name: "Acme", members: ["amy@x.com"], invited: [], created_by: "amy@x.com", created_at: "2026-01-01T00:00:00Z", admin: true, role: "admin" };
const OV = {
  workspace: { id: "ws1", name: "Acme", created_at: "2026-01-01T00:00:00Z", member_count: 2 },
  members: [
    { email: "amy@x.com", role: "admin" },
    { email: "bob@x.com", role: "viewer" },
  ],
  invited: ["cy@x.com"],
  features: { sso: true, scim: true, retention: true, metering: true, audit: true, api_keys: true },
  sso: { connections: [{ id: "c1", protocol: "oidc", enabled: true, enforced: false, domains: [{ domain: "acme.com", verified: true }] }] },
  scim: { configured: false, domains: [] },
  retention: { case_days: 0, audit_days: 0 },
  routes: {},
};
const mineOf = (workspace) => ({ email: "amy@x.com", workspace, invites: [] });

// ---- who sees the screen ----

test("a signed-out visitor gets the sign-in prompt and no workspace call", async () => {
  const app = await mount(screen.AdminScreen, {}, {}, { signedIn: false }).start();
  assert.match(app.text(), /Sign in to manage your workspace/);
  assert.equal(app.calls.length, 0);
});

test("Teams off (404 on /workspaces/mine) is a plain line, not an error", async () => {
  const app = await mount(screen.AdminScreen, {}, { "GET /workspaces/mine": err(404, "not_found") }).start();
  assert.match(app.text(), /Team workspaces are not on here/);
  assert.equal(app.alert(), undefined);
});

test("no workspace, and a viewer: no overview request is made for them", async () => {
  for (const [workspace, re] of [
    [null, /not in a workspace/],
    [{ ...WS, admin: false, role: "viewer" }, /Only workspace admins/],
  ]) {
    const app = await mount(screen.AdminScreen, {}, { "GET /workspaces/mine": json(mineOf(workspace)) }).start();
    assert.match(app.text(), re);
    assert.ok(!app.calls.some((c) => c.path.endsWith("/admin")), "no admin read");
    assert.equal(app.has("NameSection"), false);
  }
});

test("the server has the last word: an admin whose overview is 403 or 404 sees a clean refusal", async () => {
  for (const status of [403, 404]) {
    const app = await mount(screen.AdminScreen, {}, { "GET /workspaces/mine": json(mineOf(WS)), "GET /workspaces/ws1/admin": err(status) }).start();
    assert.match(app.text(), status === 403 ? /Only workspace admins/ : /not in a workspace/);
    assert.equal(app.alert(), undefined);
    assert.equal(app.has("NameSection"), false, "no section is shown");
    assert.ok(app.text().includes("Back to cases"));
  }
});

test("an overview failure is an error with a retry that re-reads", async () => {
  let fail = true;
  const app = mount(screen.AdminScreen, {}, { "GET /workspaces/mine": json(mineOf(WS)), "GET /workspaces/ws1/admin": () => (fail ? err(500, "internal", "boom") : json(OV)) });
  await app.start();
  assert.ok(app.alert());
  fail = false;
  await app.click("Try again");
  assert.ok(app.has("NameSection"));
});

test("an admin sees every section the server turned on, and only those", async () => {
  const app = await mount(screen.AdminScreen, {}, { "GET /workspaces/mine": json(mineOf(WS)), "GET /workspaces/ws1/admin": json(OV) }).start();
  for (const n of ["NameSection", "MembersSection", "SignInSection", "KeysSection", "RetentionSection", "UsageSection", "AuditSection", "WebhooksSection"]) assert.ok(app.has(n), n);
  const off = { ...OV, features: { sso: false, scim: false, retention: false, metering: false, audit: false, api_keys: false }, sso: undefined, scim: undefined, retention: undefined };
  const app2 = await mount(screen.AdminScreen, {}, { "GET /workspaces/mine": json(mineOf(WS)), "GET /workspaces/ws1/admin": json(off) }).start();
  for (const n of ["KeysSection", "RetentionSection", "UsageSection", "AuditSection"]) assert.equal(app2.has(n), false, n);
  assert.ok(app2.has("MembersSection") && app2.has("WebhooksSection"));
});

// ---- the link ----

test("workspace settings link to the admin screen only for an admin", async () => {
  const link = async (workspace) => {
    const app = mount(WorkspacePanel, {}, { "GET /workspaces/mine": json(mineOf(workspace)) });
    await app.start();
    return findAll(app.inst.tree, (e) => e.props?.["data-testid"] === "admin-link").map((e) => e.props.href);
  };
  assert.deepEqual(await link(WS), ["/settings/workspace"]);
  assert.deepEqual(await link({ ...WS, admin: false, role: "editor" }), []);
});

// ---- API keys: the secret is shown once ----

test("a new key's secret is shown once, then gone on the next load; list never carries it", async () => {
  const SECRET = "op_" + "A".repeat(43);
  let keys = [key("aaaaaaaaaaaaaaaa")];
  const routes = {
    "GET /workspaces/ws1/api-keys": () => json({ api_keys: keys }),
    "POST /workspaces/ws1/api-keys": () => {
      keys = [key("bbbbbbbbbbbbbbbb", { name: "Claude" }), ...keys];
      return json({ key: SECRET, api_key: keys[0] }, 201);
    },
  };
  const app = mount(screen.KeysSection, { ws: "ws1" }, routes);
  await app.start();
  assert.match(app.text(), /key aaaaaaaaaaaaaaaa/);
  assert.doesNotMatch(app.text(), /op_A/);
  app.input("Claude Desktop").props.onChange({ target: { value: "Claude" } });
  app.refresh();
  await app.submit();
  assert.deepEqual(app.calls.at(-1).body, { name: "Claude", role: "viewer" }, "create sends a name and the least role, no quota");
  assert.match(app.text(), /Copy this key now\. It is not shown again/);
  assert.ok(app.text().includes(SECRET));
  await app.click("Done");
  assert.ok(!app.text().includes(SECRET), "gone once dismissed");
  assert.match(app.text(), /Claude/, "the key stays listed without its secret");
  // A fresh mount (a reload) re-reads the list: no secret anywhere.
  const again = await mount(screen.KeysSection, { ws: "ws1" }, routes).start();
  assert.ok(!again.text().includes(SECRET));
  assert.ok(!JSON.stringify(keys).includes(SECRET));
});

test("keys: empty state, revoke needs a second click, revoked stays listed, 409 at the cap", async () => {
  const none = await mount(screen.KeysSection, { ws: "ws1" }, { "GET /workspaces/ws1/api-keys": json({ api_keys: [] }) }).start();
  assert.match(none.text(), /No keys yet/);

  const app = mount(
    screen.KeysSection,
    { ws: "ws1" },
    {
      "GET /workspaces/ws1/api-keys": json({ api_keys: [key("aaaaaaaaaaaaaaaa")] }),
      "DELETE /workspaces/ws1/api-keys/aaaaaaaaaaaaaaaa": new Response(null, { status: 204 }),
      "POST /workspaces/ws1/api-keys": err(409, "conflict", "limit"),
    },
  );
  await app.start();
  await app.click("Revoke");
  assert.equal(app.calls.filter((c) => c.method === "DELETE").length, 0, "first click only asks");
  await app.click("Revoke for good");
  assert.equal(app.calls.filter((c) => c.method === "DELETE").length, 1);
  assert.match(app.text(), /revoked/);
  app.input("Claude Desktop").props.onChange({ target: { value: "x" } });
  app.refresh();
  await app.submit();
  assert.match(app.alert().props.children, /20 active keys/);
});

test("keys off (404) or not for this user (403) is 'Not enabled'; a server error offers retry", async () => {
  for (const s of [404, 403]) {
    const app = await mount(screen.KeysSection, { ws: "ws1" }, { "GET /workspaces/ws1/api-keys": err(s) }).start();
    assert.match(app.text(), /Not enabled/);
  }
  const bad = await mount(screen.KeysSection, { ws: "ws1" }, { "GET /workspaces/ws1/api-keys": err(500, "internal") }).start();
  assert.ok(bad.btn("Try again"));
});

test("active key count and the cap disable Create", () => {
  const ks = Array.from({ length: 20 }, (_, i) => key(String(i)));
  assert.equal(admin.activeKeys(ks).length, 20);
  assert.equal(admin.activeKeys([...ks.slice(1), key("x", { disabled: true })]).length, 19);
  const s = admin.keyIssued(admin.keysLoaded([key("a")]), { key: "op_x", api_key: key("b") });
  assert.equal(s.secret, "op_x");
  assert.equal(admin.secretDismissed(s).secret, null);
  assert.equal(admin.keyRevoked(s, "a").keys.find((k) => k.id === "a").disabled, true);
  assert.equal(admin.keysLoaded(s.keys).secret, null, "a reload never has a secret");
});

// ---- webhooks: only when the API answers ----

test("webhooks off in production: one line 'Not enabled', no form", async () => {
  for (const s of [404, 403, 500]) {
    const app = await mount(screen.WebhooksSection, { ws: "ws1" }, { "GET /workspaces/ws1/webhooks": err(s) }).start();
    assert.match(app.text(), /Not enabled/);
    assert.equal(findAll(app.inst.tree, (e) => e.type === "form").length, 0);
  }
});

test("webhooks on: endpoints with the secret hint only; a new secret is shown once", async () => {
  let eps = [];
  const routes = {
    "GET /workspaces/ws1/webhooks": () => json({ endpoints: eps, available_events: ["batch.finished", "budget.alert"], max_endpoints: 10 }),
    "POST /workspaces/ws1/webhooks": () => {
      eps = [{ id: "wh_1", url: "https://h.example.com/x", events: ["batch.finished"], description: "", enabled: true, secret_hint: "abcd" }];
      return json({ endpoint: { ...eps[0], secret: "whsec_FULLSECRET" } }, 201);
    },
  };
  const app = mount(screen.WebhooksSection, { ws: "ws1" }, routes);
  await app.start();
  assert.match(app.text(), /No endpoints yet/);
  app.input("https://hooks.example.com/unde").props.onChange({ target: { value: "https://h.example.com/x" } });
  const box = findAll(app.inst.tree, (e) => e.type === "input" && e.props.type === "checkbox")[0];
  box.props.onChange({ target: { checked: true } });
  app.refresh();
  await app.submit();
  assert.deepEqual(app.calls.find((c) => c.method === "POST").body, { url: "https://h.example.com/x", events: ["batch.finished"] });
  assert.ok(app.text().includes("whsec_FULLSECRET"));
  await app.click("Done");
  assert.ok(!app.text().includes("whsec_FULLSECRET"));
  assert.match(app.text(), /…abcd/);
});

// ---- audit ----

test("audit filters become query parameters; empty ones are left out; a day ends at 23:59:59 UTC", () => {
  assert.equal(admin.auditQuery(admin.noFilters), "?limit=50");
  const q = new URLSearchParams(admin.auditQuery({ actor_kind: "apikey", action: "apikey.issue", outcome: "denied", from: "2026-11-01", to: "2026-11-02", order: "asc" }, "cur", 20));
  assert.deepEqual(Object.fromEntries(q), { limit: "20", order: "asc", actor_kind: "apikey", action: "apikey.issue", outcome: "denied", from: "2026-11-01T00:00:00Z", to: "2026-11-02T23:59:59Z", cursor: "cur" });
  assert.equal(new URLSearchParams(admin.auditQuery({ ...admin.noFilters, from: "garbage" })).has("from"), false);
});

test("audit: first page, filter search replaces it, More appends by cursor, empty state", async () => {
  const ev = (seq, action = "case.create") => ({ tenant: "ws:ws1", seq, time: "2026-11-27T10:00:00.123456Z", actor_kind: "user", action, phase: "complete", outcome: "allowed", prev_hash: "", hash: "" });
  const pages = {
    "GET /workspaces/ws1/admin/audit?limit=50": json({ tenant: "ws:ws1", retention_days: 90, order: "desc", events: [ev(2), ev(1)], next_cursor: "c2" }),
    "GET /workspaces/ws1/admin/audit?limit=50&cursor=c2": json({ tenant: "ws:ws1", retention_days: 90, order: "desc", events: [ev(0, "workspace.rename")] }),
    "GET /workspaces/ws1/admin/audit?limit=50&outcome=failed": json({ tenant: "ws:ws1", retention_days: 90, order: "desc", events: [] }),
  };
  const app = mount(screen.AuditSection, { ws: "ws1" }, pages);
  await app.start();
  assert.equal(findAll(app.inst.tree, (e) => e.type === "tr").length, 3, "head + two events");
  assert.match(app.text(), /Case created/);
  await app.click("More");
  assert.match(app.text(), /Workspace renamed/);
  assert.equal(app.btn("More"), undefined, "no cursor at the end");
  const sel = findAll(app.inst.tree, (e) => e.type === "select")[2];
  sel.props.onChange({ target: { value: "failed" } });
  app.refresh();
  await app.submit();
  assert.match(app.text(), /No events/);
});

test("audit off or refused is 'Not enabled'; every action the API can send has a plain label", async () => {
  const app = await mount(screen.AuditSection, { ws: "ws1" }, { "GET /workspaces/ws1/admin/audit?limit=50": err(404) }).start();
  assert.match(app.text(), /Not enabled/);
  const yaml = readFileSync(new URL("../../docs/api/openapi.yaml", import.meta.url), "utf8");
  const enumLine = yaml.match(/AuditAction:\s*\n\s*type: string\s*\n\s*enum: \[([^\]]+)\]/)[1];
  for (const a of enumLine.split(",").map((s) => s.trim())) assert.ok(admin.ACTION_LABEL[a], `label for ${a}`);
});

// ---- retention, budget, name ----

test("retention and budget are checked before they are sent", () => {
  const lim = { min_case_days: 7, max_case_days: 3650, min_audit_days: 7, max_audit_days: 365 };
  assert.equal(admin.retentionProblem("0", "0", lim), null);
  assert.equal(admin.retentionProblem("30", "90", lim), null);
  assert.ok(admin.retentionProblem("3", "0", lim));
  assert.ok(admin.retentionProblem("4000", "0", lim));
  assert.ok(admin.retentionProblem("30", "400", lim), "audit cannot be longer than the trail window");
  assert.ok(admin.retentionProblem("", "0", lim));
  assert.ok(admin.retentionProblem("1.5", "0", lim));
  assert.deepEqual(admin.retentionBody("30", "0"), { case_days: 30, audit_days: 0 });
  assert.equal(admin.budgetProblem("100", "80"), null);
  assert.ok(admin.budgetProblem("0", "80"));
  assert.ok(admin.budgetProblem("-5", "80"));
  assert.ok(admin.budgetProblem("100", "101"));
  assert.equal(admin.budgetProblem("100", ""), null, "blank alert means the default");
});

test("rename sends the collapsed name to PUT /name, shows the server's name, and a 403 says so", async () => {
  let renamed = null;
  const app = mount(screen.NameSection, { id: "ws1", name: "Acme", onRenamed: (n) => (renamed = n) }, { "PUT /workspaces/ws1/name": (init) => json({ ...WS, name: JSON.parse(init.body).name }) });
  await app.start();
  assert.ok(app.btn("Rename").props.disabled, "unchanged name: nothing to save");
  const input = findAll(app.inst.tree, (e) => e.type === "input")[0];
  input.props.onChange({ target: { value: "  Acme   Imports " } });
  app.refresh();
  await app.submit();
  assert.deepEqual(app.calls.at(-1).body, { name: "Acme Imports" });
  assert.equal(renamed, "Acme Imports");
  assert.match(app.text(), /Saved/);

  const denied = mount(screen.NameSection, { id: "ws1", name: "Acme", onRenamed() {} }, { "PUT /workspaces/ws1/name": err(403) });
  await denied.start();
  findAll(denied.inst.tree, (e) => e.type === "input")[0].props.onChange({ target: { value: "Other" } });
  denied.refresh();
  await denied.submit();
  assert.match(denied.alert().props.children, /Only workspace admins/);
});

test("gate and section mapping", () => {
  const e = (s) => new ApiError("x", s, "forbidden", false);
  assert.equal(admin.overviewGate(e(401)), "signin");
  assert.equal(admin.overviewGate(e(403)), "not_admin");
  assert.equal(admin.overviewGate(e(404)), "no_workspace");
  assert.equal(admin.overviewGate(e(500)), "error");
  assert.equal(admin.overviewGate(new Error("x")), "error");
  assert.equal(admin.sectionState(e(404)), "off");
  assert.equal(admin.sectionState(e(403)), "off");
  assert.equal(admin.sectionState(e(500)), "error");
});

test("the typed client never sends a tenant, a role or a quota with a key", () => {
  const client = readFileSync(new URL("../src/lib/api/client.ts", import.meta.url), "utf8");
  const start = client.indexOf("  adminOverview:");
  const end = client.indexOf("  // Broker review links, owner side");
  assert.doesNotMatch(client.slice(start, end), /daily_quota|tenant|owner/);
});

// ---- member roles ----

const putRole = (email, role, members = ["amy@x.com", "bob@x.com"]) =>
  json({ ...WS, members, member_roles: Object.fromEntries(members.map((m) => [m, m === email ? role : m === "amy@x.com" ? "admin" : "viewer"])) });

test("a role change is a PUT of the role; the row shows the server's answer; the only admin is locked", async () => {
  const app = mount(
    screen.MembersSection,
    { ws: "ws1", members: OV.members, invited: ["cy@x.com"] },
    { "PUT /workspaces/ws1/members/bob%40x.com/role": () => putRole("bob@x.com", "editor") },
  );
  await app.start();
  const selects = findAll(app.inst.tree, (e) => e.type === "select");
  assert.equal(selects.length, 2, "invited people have no role yet");
  assert.equal(selects[0].props.disabled, true, "amy is the only admin");
  assert.match(app.text(), /The only admin/);
  assert.equal(selects[1].props.disabled, false);
  selects[1].props.onChange({ target: { value: "editor" } });
  app.refresh();
  await app.flush();
  assert.deepEqual(app.calls.at(-1), { method: "PUT", path: "/workspaces/ws1/members/bob%40x.com/role", body: { role: "editor" } });
  assert.equal(findAll(app.inst.tree, (e) => e.type === "select")[1].props.value, "editor");
  assert.match(app.text(), /bob@x.com is now an editor/);
  // Making bob an admin frees amy.
  const again = findAll(app.inst.tree, (e) => e.type === "select")[1];
  app.routes["PUT /workspaces/ws1/members/bob%40x.com/role"] = () => json({ ...WS, member_roles: { "amy@x.com": "admin", "bob@x.com": "admin" } });
  again.props.onChange({ target: { value: "admin" } });
  app.refresh();
  await app.flush();
  assert.equal(findAll(app.inst.tree, (e) => e.type === "select")[0].props.disabled, false);
});

test("a refused role change keeps the old role and says why; the same role sends nothing", async () => {
  for (const [status, re] of [
    [409, /at least one admin/],
    [403, /Only a workspace admin/],
    [404, /not a member/],
  ]) {
    const app = mount(
      screen.MembersSection,
      { ws: "ws1", members: [...OV.members, { email: "dee@x.com", role: "admin" }], invited: [] },
      { "PUT /workspaces/ws1/members/bob%40x.com/role": err(status, status === 409 ? "conflict" : "forbidden") },
    );
    await app.start();
    const sel = findAll(app.inst.tree, (e) => e.type === "select")[1];
    sel.props.onChange({ target: { value: "admin" } });
    app.refresh();
    await app.flush();
    assert.match(app.alert().props.children, re);
    assert.equal(findAll(app.inst.tree, (e) => e.type === "select")[1].props.value, "viewer");
  }
  const same = mount(screen.MembersSection, { ws: "ws1", members: OV.members, invited: [] }, {});
  await same.start();
  findAll(same.inst.tree, (e) => e.type === "select")[1].props.onChange({ target: { value: "viewer" } });
  same.refresh();
  await same.flush();
  assert.equal(same.calls.length, 0);
});

test("the last-admin rule", () => {
  const ms = [{ email: "a", role: "admin" }, { email: "b", role: "viewer" }];
  assert.equal(admin.isLastAdmin(ms, "a"), true);
  assert.equal(admin.isLastAdmin(ms, "b"), false);
  assert.equal(admin.isLastAdmin([...ms, { email: "c", role: "admin" }], "a"), false);
  assert.deepEqual(admin.withRole(ms, "b", "editor").map((m) => m.role), ["admin", "editor"]);
});

// ---- keys carry a role ----

test("a workspace key is made with a role the admin picks: viewer first, editor, never admin; the list shows it", async () => {
  let keys = [key("aaaaaaaaaaaaaaaa", { role: "editor" }), key("cccccccccccccccc")];
  const app = mount(screen.KeysSection, { ws: "ws1" }, {
    "GET /workspaces/ws1/api-keys": () => json({ api_keys: keys }),
    "POST /workspaces/ws1/api-keys": (init) => {
      const b = JSON.parse(init.body);
      keys = [key("bbbbbbbbbbbbbbbb", { name: b.name, role: b.role }), ...keys];
      return json({ key: "op_" + "B".repeat(43), api_key: keys[0] }, 201);
    },
  });
  await app.start();
  assert.match(app.text(), /Editor/);
  assert.match(app.text(), /Own cases only/, "a key from before roles");
  const sel = findAll(app.inst.tree, (e) => e.type === "select")[0];
  assert.deepEqual(findAll(sel, (e) => e.type === "option").map((o) => o.props.value), ["viewer", "editor"], "no admin choice");
  sel.props.onChange({ target: { value: "editor" } });
  app.input("Claude Desktop").props.onChange({ target: { value: "ERP" } });
  app.refresh();
  await app.submit();
  assert.deepEqual(app.calls.find((c) => c.method === "POST").body, { name: "ERP", role: "editor" });
  assert.match(app.text(), /ERP/);
  assert.match(app.text(), /Never an admin/);
  assert.deepEqual(admin.KEY_ROLES, ["viewer", "editor"]);
});

// ---- SSO and SCIM ----

const conn = (over = {}) => ({
  id: "c1",
  protocol: "oidc",
  domains: [{ domain: "acme.com", verified: false, record_name: "_undelabs-verify.acme.com", record_value: "undelabs-verify=abc123" }],
  default_role: "viewer",
  enabled: false,
  enforced: false,
  oidc_issuer: "https://login.acme.com",
  oidc_client_id: "cid",
  has_client_secret: true,
  service_provider: { oidc_redirect_url: "https://site/sso/oidc/callback" },
  created_at: "2026-11-27T10:00:00Z",
  updated_at: "2026-11-27T10:00:00Z",
  ...over,
});
const typeInto = (app, placeholderOrLabel, v) => app.input(placeholderOrLabel).props.onChange({ target: { value: v } });

test("SSO and SCIM are one line 'Not enabled' when the overview has neither (production)", async () => {
  const app = mount(screen.SignInSection, { ws: "ws1", sso: undefined, scim: undefined });
  await app.start();
  assert.equal(app.text().match(/Not enabled/g).length, 2);
  assert.equal(app.has("SsoPart"), false);
  assert.equal(app.has("ScimPart"), false);
  assert.equal(findAll(app.inst.tree, (e) => e.type === "form").length, 0);
  for (const s of [404, 403]) {
    const sso = await mount(signin.SsoPart, { ws: "ws1" }, { "GET /workspaces/ws1/sso": err(s) }).start();
    assert.match(sso.text(), /Not enabled/);
    const scim = await mount(signin.ScimPart, { ws: "ws1", domains: [] }, { "GET /workspaces/ws1/scim": err(s) }).start();
    assert.match(scim.text(), /Not enabled/);
  }
});

test("SSO: a connection lists with its DNS TXT record, Check DNS verifies, Turn on enables", async () => {
  let c = conn();
  const seen = [];
  const app = mount(signin.SsoPart, { ws: "ws1", onDomains: (d) => seen.push(d) }, {
    "GET /workspaces/ws1/sso": () => json({ connections: [c] }),
    "POST /workspaces/ws1/sso/c1/verify": (init) => {
      const { enable } = JSON.parse(init.body);
      c = conn({ enabled: enable, domains: [{ ...c.domains[0], verified: true }] });
      return json(c);
    },
  });
  await app.start();
  let card = await sub(app, "ConnectionCard");
  assert.match(card.text(), /OIDC/);
  assert.match(card.text(), /secret saved/);
  const dns = findAll(card.inst.tree, (e) => e.props?.["data-testid"] === "dns-acme.com")[0];
  assert.equal(textOf(dns.props.children), "_undelabs-verify.acme.com  TXT  undelabs-verify=abc123");
  assert.deepEqual(seen.at(-1), []);
  await card.click("Check DNS");
  assert.deepEqual(app.calls.at(-1).body, { enable: false });
  assert.deepEqual(seen.at(-1), ["acme.com"], "SCIM learns the verified domain");
  card = await sub(app, "ConnectionCard");
  assert.equal(findAll(card.inst.tree, (e) => e.props?.["data-testid"] === "dns-acme.com").length, 0, "no record to add once verified");
  await card.click("Turn on");
  assert.deepEqual(app.calls.at(-1).body, { enable: true });
  card = await sub(app, "ConnectionCard");
  assert.ok(card.btn("Turn off"));
});

test("SSO: Turn on with an unverified domain says which and why", async () => {
  const app = mount(signin.SsoPart, { ws: "ws1" }, {
    "GET /workspaces/ws1/sso": json({ connections: [conn()] }),
    "POST /workspaces/ws1/sso/c1/verify": json(conn({ checks: [{ domain: "acme.com", record: "_undelabs-verify.acme.com", verified: false, problem: "no TXT record found" }] })),
  });
  await app.start();
  const card = await sub(app, "ConnectionCard");
  await card.click("Turn on");
  assert.match(card.text(), /acme\.com: no TXT record found/);
  assert.ok(card.btn("Turn on"), "still off");
});

test("SSO: add an OIDC connection sends the settings and the secret once; the list never shows the secret", async () => {
  const SECRET = "s3cr3t-value";
  let list = [];
  const app = mount(signin.SsoPart, { ws: "ws1" }, {
    "GET /workspaces/ws1/sso": () => json({ connections: list }),
    "POST /workspaces/ws1/sso": (init) => {
      list = [conn({ id: "c2" })];
      assert.equal(JSON.parse(init.body).oidc_client_secret, SECRET);
      return json(list[0], 201);
    },
  });
  await app.start();
  assert.match(app.text(), /No connection/);
  await app.click("Add connection");
  const form = await sub(app, "SsoFormView");
  assert.ok(form.btn("Add connection").props.disabled, "nothing to send yet");
  typeInto(form, "acme.com, acme.co.uk", "Acme.com acme.com; acme.co.uk");
  form.refresh();
  typeInto(form, "https://login.acme.com", "https://login.acme.com");
  form.refresh();
  const cid = findAll(form.inst.tree, (e) => e.type === "input" && e.props.autoComplete === "off")[0];
  cid.props.onChange({ target: { value: "cid" } });
  form.refresh();
  const secret = findAll(form.inst.tree, (e) => e.type === "input" && e.props.type === "password")[0];
  assert.equal(secret.props.autoComplete, "new-password");
  secret.props.onChange({ target: { value: SECRET } });
  form.refresh();
  await form.submit();
  app.refresh();
  await app.flush();
  assert.deepEqual(app.calls.find((c) => c.method === "POST").body, {
    protocol: "oidc",
    domains: ["acme.com", "acme.co.uk"],
    default_role: "viewer",
    oidc_issuer: "https://login.acme.com",
    oidc_client_id: "cid",
    oidc_client_secret: SECRET,
  });
  const serial = (n) => JSON.stringify(n, (k, v) => (typeof v === "function" ? undefined : v));
  assert.ok(!serial(app.inst.tree).includes(SECRET), "the secret is not in the screen after saving");
  const card = await sub(app, "ConnectionCard");
  assert.match(card.text(), /secret saved/);
  assert.ok(!card.text().includes(SECRET));
});

test("SSO: editing keeps the secret (left out of the request) and shows only that one exists", async () => {
  const app = mount(signin.SsoPart, { ws: "ws1" }, {
    "GET /workspaces/ws1/sso": json({ connections: [conn()] }),
    "PUT /workspaces/ws1/sso/c1": (init) => json(conn({ default_role: JSON.parse(init.body).default_role })),
  });
  await app.start();
  const card = await sub(app, "ConnectionCard");
  card.btn("Edit").props.onClick();
  app.refresh();
  const form = await sub(app, "SsoFormView");
  const secret = findAll(form.inst.tree, (e) => e.type === "input" && e.props.type === "password")[0];
  assert.equal(secret.props.value, "");
  assert.equal(secret.props.placeholder, "Secret saved. Leave empty to keep it.");
  const role = findAll(form.inst.tree, (e) => e.type === "select")[1];
  assert.deepEqual(findAll(role, (e) => e.type === "option").map((o) => o.props.value), ["viewer", "editor"], "no admin default role to pick");
  role.props.onChange({ target: { value: "editor" } });
  form.refresh();
  await form.submit();
  const body = app.calls.find((c) => c.method === "PUT").body;
  assert.equal("oidc_client_secret" in body, false);
  assert.equal("enabled" in body || "enforced" in body, false, "an edit leaves on/off and required as they are");
  assert.equal(body.default_role, "editor");
  assert.equal(body.oidc_issuer, "https://login.acme.com");
});

test("SSO: add a SAML connection takes the sign-in URL, entity ID and 1 to 3 PEM certificates", () => {
  const pem = (n) => `-----BEGIN CERTIFICATE-----\nMII${n}\n-----END CERTIFICATE-----`;
  const f = { ...admin.emptySsoForm, protocol: "saml", domains: "acme.com", saml_sso_url: "https://idp.acme.com/sso", saml_issuer: "urn:idp", saml_certs: `${pem(1)}\n\n${pem(2)}` };
  assert.equal(admin.ssoProblem(f), null);
  assert.deepEqual(admin.ssoBody(f), { protocol: "saml", domains: ["acme.com"], default_role: "viewer", saml_sso_url: "https://idp.acme.com/sso", saml_issuer: "urn:idp", saml_certs_pem: [pem(1), pem(2)] });
  assert.ok(admin.ssoProblem({ ...f, saml_certs: "" }));
  assert.ok(admin.ssoProblem({ ...f, saml_certs: [1, 2, 3, 4].map(pem).join("\n") }));
  assert.ok(admin.ssoProblem({ ...f, saml_sso_url: "http://idp.acme.com" }));
  assert.ok(admin.ssoProblem({ ...f, saml_issuer: " " }));
  assert.ok(admin.ssoProblem({ ...f, domains: " , " }));
  assert.ok(admin.ssoProblem({ ...admin.emptySsoForm, domains: "a.com", oidc_issuer: "http://x.com", oidc_client_id: "c" }));
});

test("SSO: Require SSO asks first, then replaces the connection with enforced true; Allow Google sign-in undoes it", async () => {
  let c = conn({ enabled: true, domains: [{ domain: "acme.com", verified: true, record_name: "r", record_value: "v" }] });
  const app = mount(signin.SsoPart, { ws: "ws1" }, {
    "GET /workspaces/ws1/sso": () => json({ connections: [c] }),
    "PUT /workspaces/ws1/sso/c1": (init) => {
      c = { ...c, enforced: JSON.parse(init.body).enforced };
      return json(c);
    },
  });
  await app.start();
  let card = await sub(app, "ConnectionCard");
  await card.click("Require SSO");
  assert.equal(app.calls.filter((x) => x.method === "PUT").length, 0, "the first click only asks");
  assert.match(card.text(), /Google sign-in stops/);
  await card.click("Require SSO");
  const put = app.calls.find((x) => x.method === "PUT").body;
  assert.equal(put.enforced, true);
  assert.equal("oidc_client_secret" in put, false);
  card = await sub(app, "ConnectionCard");
  assert.match(card.text(), /required/);
  await card.click("Allow Google sign-in");
  assert.equal(app.calls.filter((x) => x.method === "PUT").at(-1).body.enforced, false);
  // Off connections have no switch to make it required.
  const offApp = await mount(signin.SsoPart, { ws: "ws1" }, { "GET /workspaces/ws1/sso": json({ connections: [conn()] }) }).start();
  const offCard = await sub(offApp, "ConnectionCard");
  assert.equal(offCard.btn("Require SSO"), undefined);
});

test("SSO: the server's refusals are plain; remove asks twice", async () => {
  const app = mount(signin.SsoPart, { ws: "ws1" }, {
    "GET /workspaces/ws1/sso": json({ connections: [conn()] }),
    "POST /workspaces/ws1/sso/c1/verify": err(409, "sso_unverified"),
    "DELETE /workspaces/ws1/sso/c1": new Response(null, { status: 204 }),
  });
  await app.start();
  const card = await sub(app, "ConnectionCard");
  await card.click("Turn on");
  assert.match(card.alert().props.children, /Verify every domain/);
  await card.click("Remove");
  assert.equal(app.calls.filter((c) => c.method === "DELETE").length, 0);
  await card.click("Remove for good");
  assert.equal(app.calls.filter((c) => c.method === "DELETE").length, 1);
  app.refresh();
  assert.match(app.text(), /No connection/);
  const e = (status, code, message = "m") => new ApiError(message, status, code, false);
  assert.match(admin.ssoError(e(409, "sso_domain_taken")), /Another workspace/);
  assert.match(admin.ssoError(e(409, "sso_limit")), /5 connections/);
  assert.match(admin.ssoError(e(400, "invalid_input", "the OIDC issuer could not be discovered")), /could not be discovered/);
  assert.match(admin.ssoError(e(403, "forbidden")), /Only workspace admins/);
});

const scimStatus = (over = {}) => ({ configured: false, domains: [], allow_admin: false, base_url: "https://site/api/v1/scim/v2", token_header: "X-SCIM-Token", ...over });

test("SCIM: the token is created for verified domains, shown once, then only its last four characters", async () => {
  const TOKEN = "scim_" + "T".repeat(43);
  let status = scimStatus();
  const routes = {
    "GET /workspaces/ws1/scim": () => json(status),
    "POST /workspaces/ws1/scim/token": (init) => {
      const b = JSON.parse(init.body);
      status = scimStatus({ configured: true, hint: "TTTT", domains: b.domains, default_role: b.default_role });
      return json({ ...status, token: TOKEN }, 201);
    },
  };
  const app = mount(signin.ScimPart, { ws: "ws1", domains: ["acme.com", "acme.co.uk"] }, routes);
  await app.start();
  assert.match(app.text(), /Not set up/);
  await app.click("Create token");
  const boxes = findAll(app.inst.tree, (e) => e.type === "input" && e.props.type === "checkbox");
  assert.equal(boxes.length, 2);
  boxes[1].props.onChange({ target: { checked: false } });
  app.refresh();
  await app.submit();
  assert.deepEqual(app.calls.find((c) => c.method === "POST").body, { domains: ["acme.com"], default_role: "viewer" });
  const once = await sub(app, "Once");
  assert.ok(once.text().includes(TOKEN));
  assert.match(once.text(), /not shown again/);
  assert.match(app.text(), /token ends …TTTT/);
  await once.click("Done");
  app.refresh();
  assert.ok(!app.text().includes(TOKEN), "gone once dismissed");
  assert.equal(app.has("Once"), false);
  const again = await mount(signin.ScimPart, { ws: "ws1", domains: ["acme.com"] }, routes).start();
  assert.ok(!again.text().includes(TOKEN), "a reload never has it");
  assert.match(again.text(), /X-SCIM-Token/);
});

test("SCIM: revoke asks first; with no verified domain there is nothing to create; 409 says verify first", async () => {
  const on = scimStatus({ configured: true, hint: "abcd", domains: ["acme.com"], default_role: "viewer" });
  const app = mount(signin.ScimPart, { ws: "ws1", domains: ["acme.com"] }, {
    "GET /workspaces/ws1/scim": json(on),
    "DELETE /workspaces/ws1/scim/token": new Response(null, { status: 204 }),
    "POST /workspaces/ws1/scim/token": err(409, "scim_unverified_domain"),
  });
  await app.start();
  assert.ok(app.btn("Replace token"));
  await app.click("Revoke");
  assert.equal(app.calls.filter((c) => c.method === "DELETE").length, 0);
  await app.click("Revoke for good");
  assert.equal(app.calls.filter((c) => c.method === "DELETE").length, 1);
  assert.match(app.text(), /Not set up/);
  await app.click("Create token");
  await app.submit();
  assert.match(app.alert().props.children, /Verify a domain under SSO first/);
  const none = await mount(signin.ScimPart, { ws: "ws1", domains: [] }, { "GET /workspaces/ws1/scim": json(scimStatus()) }).start();
  assert.match(none.text(), /Verify an SSO domain first/);
  assert.equal(none.btn("Create token"), undefined);
});
