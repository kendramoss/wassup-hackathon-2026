import "./render-support.mjs";
import assert from "node:assert/strict";
import { test } from "node:test";

// #285: what the Next proxy writes to the console when the backend fails. The
// REAL callBackend / proxyBackend / errorResponse run against a stubbed fetch;
// console.error is captured. Synthetic sentinels only: an email, a token, a
// supplier name and a BOM line in the URL, the response body, the error and the
// headers must never be printed, and the line is one fixed shape.
process.env.BACKEND_URL = "http://backend.test";
delete process.env.K_SERVICE;
const { callBackend, proxyBackend, errorResponse, BackendError, routeOf, errorClass } = await import("../src/lib/backend.ts");

const SENT_EMAIL = "zz.user285+leak@example.invalid";
const SENT_TOKEN = "tok_285_SYNTHETIC_DO_NOT_USE_0123456789";
const SENT_SUPPLIER = "Zzyzx-Synthetic-Supplier-285 GmbH";
const SENT_BOM = "PN-285-SYNTH-0001 stator laminations 4200 pcs @ USD 1.234";
const SENTINELS = [SENT_EMAIL, SENT_TOKEN, SENT_SUPPLIER, SENT_BOM, "Zzyzx", "PN-285", "zz.user285"];

const realFetch = globalThis.fetch;
const realError = console.error;
let lines = [];
function capture(fetchImpl) {
  lines = [];
  console.error = (...args) => lines.push(args);
  globalThis.fetch = async (url, init = {}) => {
    if (!String(url).startsWith("http://backend.test")) return realFetch(url, init);
    return fetchImpl(String(url), init);
  };
}
function restore() {
  console.error = realError;
  globalThis.fetch = realFetch;
}
function assertClean() {
  const flat = JSON.stringify(lines);
  for (const s of SENTINELS) assert.ok(!flat.includes(s), `console output leaked ${s}: ${flat}`);
  for (const args of lines) {
    assert.equal(args.length, 1, "one string argument: no error object is passed to the console");
    assert.equal(typeof args[0], "string");
    assert.match(args[0], /^proxy failure kind=(backend_status|unreachable|proxy_error) route=[-/{}a-z0-9]+( status=\d{3})?( code=[a-z_]+)?( class=[A-Za-z]+)?( request_id=(h_)?[0-9a-f]{16})?$/);
  }
}

test("callBackend: a failing backend logs status, route pattern, a closed code and the ref, never the body", async () => {
  capture(async () => new Response(JSON.stringify({ code: "internal", message: `failed on ${SENT_BOM} for ${SENT_EMAIL}`, details: SENT_SUPPLIER }), {
    status: 500,
    headers: { "x-request-id": "h_0123456789abcdef" },
  }));
  try {
    await assert.rejects(callBackend(`/v1/workspaces/${SENT_SUPPLIER}/members/${SENT_EMAIL}/role?token=${SENT_TOKEN}`, "{}"), (e) => e instanceof BackendError && e.status === 502);
  } finally {
    restore();
  }
  assert.equal(lines.length, 1);
  assert.equal(lines[0][0], "proxy failure kind=backend_status route=/v1/workspaces/{}/members/{}/role status=500 code=internal request_id=h_0123456789abcdef");
  assertClean();
});

test("callBackend: a body that is not JSON, or a code outside the closed set, logs the constant 'other'", async () => {
  for (const body of [SENT_BOM + " " + SENT_EMAIL, JSON.stringify({ code: SENT_TOKEN }), JSON.stringify({ code: { a: SENT_EMAIL } }), ""]) {
    capture(async () => new Response(body, { status: 503, headers: { "x-request-id": SENT_TOKEN } }));
    try {
      await assert.rejects(callBackend("/v1/cases/predict", "{}"));
    } finally {
      restore();
    }
    assert.equal(lines.length, 1);
    assert.equal(lines[0][0], "proxy failure kind=backend_status route=/v1/cases/predict status=503 code=other", "a token-shaped request id header is not echoed either");
    assertClean();
  }
});

test("callBackend: a user-error status keeps its behaviour (the API body is the message) and logs nothing", async () => {
  capture(async () => new Response(JSON.stringify({ code: "invalid_input", message: `bad ${SENT_BOM}` }), { status: 400 }));
  try {
    await assert.rejects(callBackend("/v1/cases/predict", "{}"), (e) => e.status === 400 && e.message.includes(SENT_BOM));
  } finally {
    restore();
  }
  assert.equal(lines.length, 0);
});

test("proxyBackend: an unreachable backend logs the class and the route pattern, not the error object or its message", async () => {
  capture(async () => {
    const e = new TypeError(`fetch failed: connect ECONNREFUSED for ${SENT_EMAIL} ${SENT_TOKEN}`);
    e.cause = new Error(SENT_SUPPLIER);
    throw e;
  });
  let res;
  try {
    res = await proxyBackend(new Request(`http://front.test/api/v1/cases/${SENT_SUPPLIER}/export?token=${SENT_TOKEN}`, { method: "POST", body: SENT_BOM }), `cases/${SENT_SUPPLIER}/export`);
  } finally {
    restore();
  }
  assert.equal(res.status, 502);
  assert.deepEqual(await res.json(), { code: "backend_unreachable", message: "Couldn't reach the UndeLabs API. Nothing is lost; try again in a moment." }, "the response is unchanged");
  assert.equal(res.headers.get("cache-control"), "private, no-store");
  assert.equal(lines.length, 1);
  assert.equal(lines[0][0], "proxy failure kind=unreachable route=/v1/cases/{}/export class=TypeError");
  assertClean();
});

test("proxyBackend: a backend error body passes through untouched and logs nothing; SSE headers are unchanged", async () => {
  capture(async () => new Response(JSON.stringify({ code: "model_unavailable", message: SENT_BOM }), { status: 502, headers: { "content-type": "application/json" } }));
  let res;
  try {
    res = await proxyBackend(new Request("http://front.test/api/v1/cases/c1/plan", { method: "POST", body: "{}" }), "cases/c1/plan");
  } finally {
    restore();
  }
  assert.equal(res.status, 502);
  assert.equal((await res.json()).message, SENT_BOM, "the requester still gets the API body");
  assert.equal(lines.length, 0, "the proxy logs nothing for a pass-through body");

  capture(async () => new Response("event: progress\ndata: {}\n\n", { status: 200, headers: { "content-type": "text/event-stream" } }));
  try {
    res = await proxyBackend(new Request("http://front.test/api/v1/cases/c1/plan", { method: "POST", body: "{}" }), "cases/c1/plan");
  } finally {
    restore();
  }
  assert.equal(res.headers.get("cache-control"), "no-cache, no-transform");
  assert.equal(res.headers.get("x-accel-buffering"), "no");
  assert.equal(await res.text(), "event: progress\ndata: {}\n\n");
});

test("errorResponse: an arbitrary thrown value logs only its closed class", () => {
  console.error = (...args) => lines.push(args);
  lines = [];
  try {
    const e = new Error(`${SENT_EMAIL} ${SENT_TOKEN}`);
    e.name = SENT_SUPPLIER; // a hostile name is not a class
    errorResponse(e);
    errorResponse({ toString: () => SENT_BOM });
    errorResponse(new BackendError(SENT_BOM, 502));
    errorResponse(new DOMException("x", "AbortError"));
  } finally {
    console.error = realError;
  }
  assert.deepEqual(
    lines.map((l) => l[0]),
    [
      "proxy failure kind=proxy_error route=- class=unclassified", // a hostile name is not a class
      "proxy failure kind=proxy_error route=- class=unclassified", // not an Error at all
      "proxy failure kind=proxy_error route=- class=AbortError",
    ],
    "a BackendError is turned into its response and not logged here",
  );
  assertClean();
});

test("routeOf keeps only known static words", () => {
  assert.equal(routeOf(`/v1/cases/${SENT_SUPPLIER}/plan?x=${SENT_TOKEN}#frag`), "/v1/cases/{}/plan");
  assert.equal(routeOf("/v1/workspaces/ws_1/members/a@b.c"), "/v1/workspaces/{}/members/{}");
  assert.equal(routeOf(""), "/");
  assert.equal(errorClass(new RangeError("x")), "RangeError");
  assert.equal(errorClass("string"), "unclassified");
});
