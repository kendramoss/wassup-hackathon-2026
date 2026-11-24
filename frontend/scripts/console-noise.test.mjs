import "./render-support.mjs";
import assert from "node:assert/strict";
import { test } from "node:test";
import { findAll, instance, json, tick } from "./lib/hook-runtime.mjs";

// #786: no console noise on page load. /cases/new must not call
// hts-suggest before there is a product, and an anonymous visitor's case list
// under STRICT_AUTH must not be a failed request, while signed-in users and
// API-key callers keep the 401 exactly as before.
process.env.BACKEND_URL = "http://backend.test";
delete process.env.K_SERVICE;
globalThis.window = new EventTarget();
const { proxyBackend } = await import("../src/lib/backend.ts");
const { api, ApiError, SIGNIN_EXPIRED } = await import("../src/lib/api/client.ts");
const { HtsField } = await import("../src/components/op/hts-field.tsx");

const unauth = { code: "unauthenticated", message: "sign in or use an API key" };
const upstream = [];
function backend(answer) {
  upstream.length = 0;
  globalThis.fetch = async (url, init = {}) => {
    const u = String(url);
    upstream.push({ u, method: init.method, headers: Object.fromEntries(new Headers(init.headers)) });
    return answer(new Headers(init.headers));
  };
}
// The strict backend: only a verified caller (signed in or keyed) may list.
const strict = (h) => (h.get("x-user-token") || h.get("x-api-key") ? json({ cases: [{ id: "c1" }] }) : json(unauth, 401));
const req = (headers = {}, method = "GET") => new Request("http://front.test/api/v1/cases", { method, headers });

test("proxy: an anonymous page list under STRICT_AUTH is 200 with the 401 body beside an empty list", async () => {
  backend(strict);
  const res = await proxyBackend(req({ "x-signin-optional": "1", cookie: "op_visitor=v_1" }), "cases");
  assert.equal(res.status, 200);
  assert.deepEqual(await res.json(), { cases: [], signin_required: unauth });
  assert.equal(res.headers.get("cache-control"), "private, no-store");
  assert.equal(upstream[0].headers["x-signin-optional"], undefined, "the marker never reaches the backend");
});

test("proxy: everyone else keeps the 401 (no marker, API key, signed in, other routes, other errors)", async () => {
  backend(() => json(unauth, 401));
  assert.equal((await proxyBackend(req(), "cases")).status, 401, "no marker: the API contract is unchanged");
  assert.equal((await proxyBackend(req({ "x-signin-optional": "1", "x-api-key": "op_bad" }), "cases")).status, 401);
  const signedIn = await proxyBackend(req({ "x-signin-optional": "1", cookie: "op_user=aaa.bbb.ccc" }), "cases");
  assert.equal(signedIn.status, 401, "an expired sign-in still reads as an expiry");
  assert.match(signedIn.headers.get("set-cookie") ?? "", /op_user=;/, "and the proxy still clears it");
  assert.equal((await proxyBackend(req({ "x-signin-optional": "1" }), "cases/c1")).status, 401);
  backend(() => json({ code: "forbidden", message: "no" }, 401));
  assert.equal((await proxyBackend(req({ "x-signin-optional": "1" }), "cases")).status, 401, "only unauthenticated is the anonymous path");
});

test("proxy: HEAD and POST with the marker are not rewritten", async () => {
  backend(() => json(unauth, 401));
  for (const method of ["HEAD", "POST"]) {
    const res = await proxyBackend(req({ "x-signin-optional": "1", cookie: "op_visitor=v_1" }, method), "cases");
    assert.equal(res.status, 401, method);
  }
});

// The client through the real proxy, as the browser sees it.
function browser(answer) {
  backend(answer);
  const toBackend = globalThis.fetch;
  globalThis.fetch = async (url, init = {}) => {
    const u = String(url);
    if (u.startsWith("http://backend.test")) return toBackend(url, init);
    const headers = new Headers(init.headers);
    if (browser.cookie) headers.set("cookie", browser.cookie);
    const res = await proxyBackend(new Request(`http://front.test${u}`, { ...init, headers }), u.replace(/^\/api\/v1\//, ""));
    browser.statuses.push(res.status);
    return res;
  };
  browser.statuses = [];
}

test("client: an anonymous visitor gets the same 401 error and sign-in note, with no failed request", async () => {
  browser.cookie = "op_visitor=v_1";
  browser(strict);
  let events = 0;
  const on = () => events++;
  window.addEventListener(SIGNIN_EXPIRED, on);
  const err = await api.listCases().then(() => null, (e) => e);
  window.removeEventListener(SIGNIN_EXPIRED, on);
  assert.ok(err instanceof ApiError);
  assert.equal(err.status, 401);
  assert.equal(err.code, "unauthenticated");
  assert.equal(err.message, unauth.message);
  assert.equal(events, 1, "the header's sign-in note still learns sign-in is required");
  assert.deepEqual(browser.statuses, [200], "the browser sees no failed request");
});

test("client: signed-in users and a non-strict visitor list cases as before", async () => {
  browser.cookie = "op_user=aaa.bbb.ccc; op_visitor=v_1";
  browser(strict);
  assert.deepEqual((await api.listCases()).cases, [{ id: "c1" }]);
  browser.cookie = "op_visitor=v_1";
  browser(() => json({ cases: [{ id: "mine" }] }));
  assert.deepEqual((await api.listCases()).cases, [{ id: "mine" }]);
});

test("client: a signed-in user's expired token is still a real 401", async () => {
  browser.cookie = "op_user=aaa.bbb.ccc";
  browser(() => json(unauth, 401));
  const err = await api.listCases().then(() => null, (e) => e);
  assert.equal(err?.status, 401);
  assert.deepEqual(browser.statuses, [401]);
});

// HtsField mounted with the hook runtime: effects run, fetch is recorded.
function mountHts(props) {
  const calls = [];
  globalThis.fetch = async (url, init = {}) => {
    calls.push({ url: String(url), body: init.body });
    return json({ candidates: [] });
  };
  let inst;
  inst = instance(HtsField, () => queueMicrotask(() => inst.render(props)));
  inst.render(props);
  return { inst, calls };
}

test("hts-field: nothing is sent on load, with or without a product", async () => {
  for (const source of [null, { facts: "A cordless heat gun with a lithium battery pack" }]) {
    const { calls, inst } = mountHts({ value: "", onChange() {}, source, inputClass: "w-full", onNeedSource() {} });
    for (let i = 0; i < 5; i++) await tick();
    assert.deepEqual(calls, [], "no empty probe (#786)");
    const btn = findAll(inst.tree, (e) => e?.props?.children === "Suggest from earlier rulings");
    assert.equal(btn.length, 1, "the button shows before the first call");
  }
});

// Runs last: a 404 is remembered for the rest of the page load.
test("hts-field: a click with a product sends that product; a 404 removes the button, keeps focus and says why", async () => {
  const source = { facts: "A cordless heat gun with a lithium battery pack" };
  const props = { value: "", onChange() {}, source, inputClass: "w-full" };
  const { calls, inst } = mountHts(props);
  globalThis.fetch = async (url, init = {}) => {
    calls.push({ url: String(url), body: init.body });
    return json({ code: "not_found", message: "not found" }, 404);
  };
  let focused = 0;
  const input = findAll(inst.tree, (e) => e?.type === "input")[0];
  input.props.ref.current = { focus: () => focused++ };
  const btn = (tree) => findAll(tree, (e) => e?.props?.children === "Suggest from earlier rulings");
  await btn(inst.tree)[0].props.onClick();
  for (let i = 0; i < 5; i++) await tick();
  assert.equal(calls.length, 1);
  assert.equal(calls[0].url, "/api/v1/products/hts-suggest");
  assert.deepEqual(JSON.parse(calls[0].body), source);
  assert.equal(btn(inst.tree).length, 0);
  assert.equal(focused, 1, "focus moves to the HTS input, not <body>");
  const alert = findAll(inst.tree, (e) => e?.props?.role === "alert");
  assert.equal(alert.length, 1);
  assert.equal(alert[0].props.children, "Suggestions are off here. Type the code.");
  assert.match(alert[0].props.className, /text-dim/);
  assert.doesNotMatch(alert[0].props.className, /text-fail/);
  const again = mountHts(props);
  for (let i = 0; i < 5; i++) await tick();
  assert.equal(btn(again.inst.tree).length, 0, "a remount remembers suggestions are off");
  assert.deepEqual(again.calls, []);
});
