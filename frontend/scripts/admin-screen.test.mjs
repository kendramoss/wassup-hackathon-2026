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
const admin = await import("../src/lib/admin.ts");
const { ApiError } = await import("../src/lib/api/client.ts");
const { WorkspacePanel } = await import("../src/components/op/workspace-panel.tsx");

const live = [];
afterEach(() => {
  for (const inst of live.splice(0)) {
    inst.mounted = false;
    for (const h of inst.hooks) h?.cleanup?.();
  }
});
const textOf = (n) => (n == null || typeof n === "boolean" ? "" : typeof n !== "object" ? String(n) : Array.isArray(n) ? n.map(textOf).join("") : textOf(n.props?.children));
const err = (status, code = "forbidden", message = "no") => json({ code, message }, status);

function mount(Component, props, routes = {}, { signedIn = true } = {}) {
  const app = { calls: [], routes };
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
  assert.deepEqual(app.calls.at(-1).body, { name: "Claude" }, "create sends only a name, no quota");
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
