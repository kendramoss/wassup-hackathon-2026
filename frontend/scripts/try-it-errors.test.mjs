import { html, text } from "./render-support.mjs";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { test } from "node:test";

// Judge panel round 16, item 3 and N1/N4: the live try-it flows' failure
// paths. A spent per-client budget is not a rate limit, a case with no plan
// template gets a next step instead of backend text, step 1's draft retry
// re-runs the draft, a cut long call points at Saved cases, and backend
// prefixes never reach the user.

const client = new URL("../src/lib/api/client.ts", import.meta.url).href;
const registerTs = new URL("./register-ts-loader.mjs", import.meta.url).href;

// What the JSON client throws for a response (run in a child so fetch is ours).
function errorFor(status, body, headers = {}) {
  const raw = typeof body === "string" ? body : JSON.stringify(body);
  const ct = typeof body === "string" ? "text/html" : "application/json";
  const code = `
    import { api } from ${JSON.stringify(client)};
    globalThis.fetch = async () => new Response(${JSON.stringify(raw)}, { status: ${status}, headers: { "content-type": ${JSON.stringify(ct)}, ...${JSON.stringify(headers)} } });
    try { await api.compareTargets("c1", ["VN"]); console.log(JSON.stringify({ threw: false })); }
    catch (e) { console.log(JSON.stringify({ message: e.message, status: e.status, code: e.code, retryable: e.retryable, retryAfter: e.retryAfter ?? null })); }
  `;
  const r = spawnSync(process.execPath, ["--import", registerTs, "--no-warnings", "--input-type=module", "-e", code], { encoding: "utf8" });
  assert.equal(r.status, 0, r.stderr || r.stdout);
  return JSON.parse(r.stdout);
}

const { ApiError, errorText, isDailyLimit, TIMEOUT_MESSAGE } = await import("../src/lib/api/client.ts");
const { ErrorBox } = await import("../src/components/op/chrome.tsx");

// ---- 1. 429: short limits count down, daily ones don't -------------------
const clientBudgetMsg = "you have used your share of today's model budget; saved cases still work, and new analyses open again tomorrow (UTC)";

test("429 client_budget_reached keeps its code and message, is not retryable, and is a daily limit", () => {
  const e = errorFor(429, { code: "client_budget_reached", message: clientBudgetMsg, details: { retry_after_s: 41237 } }, { "Retry-After": "41237" });
  assert.deepEqual(e, { message: clientBudgetMsg, status: 429, code: "client_budget_reached", retryable: false, retryAfter: 41237 });
});

test("429 quota_exceeded is a daily limit too", () => {
  const e = errorFor(429, { code: "quota_exceeded", message: "this API key's daily quota of 100 requests is used up; it resets at 00:00 UTC" }, { "Retry-After": "3600" });
  assert.equal(e.code, "quota_exceeded");
  assert.equal(e.retryable, false);
});

test("429 rate_limited, too_many_in_flight and an empty body stay short rate limits", () => {
  assert.deepEqual(errorFor(429, { code: "rate_limited", message: "x" }, { "Retry-After": "12" }), { message: "x", status: 429, code: "rate_limited", retryable: true, retryAfter: 12 });
  const inflight = errorFor(429, { code: "too_many_in_flight", message: "you have 2 analyses running; wait for one to finish", details: { retry_after_s: 5 } }, { "Retry-After": "5" });
  assert.equal(inflight.code, "rate_limited");
  assert.equal(inflight.retryAfter, 5);
  const empty = errorFor(429, "");
  assert.equal(empty.code, "rate_limited");
  assert.equal(empty.retryAfter, 30);
});

test("the daily-limit note has the backend message, a demo link and the UTC reset, and no countdown", () => {
  const e = new ApiError(clientBudgetMsg, 429, "client_budget_reached", false, 41237);
  assert.ok(isDailyLimit(e));
  const m = html(ErrorBox, { error: e, onRetry: () => {} });
  const t = text(m);
  assert.match(t, /Today's model budget for this network is used up\./);
  assert.match(t, /You have used your share of today's model budget; saved cases still work/);
  assert.match(t, /resets after 00:00 UTC/);
  assert.match(t, /Open a demo case →/);
  assert.match(m, /href="\/cases"/);
  assert.doesNotMatch(t, /Too many requests|41237| s\b|Try again/);
  assert.match(m, /role="status"/);
});

test("the quota note names the API key, not the network", () => {
  const t = text(html(ErrorBox, { error: new ApiError("this API key's daily quota of 100 requests is used up; it resets at 00:00 UTC", 429, "quota_exceeded", false, 3600) }));
  assert.match(t, /request quota for this API key is used up/);
  assert.match(t, /This API key's daily quota of 100 requests is used up; it resets at 00:00 UTC\./);
  assert.doesNotMatch(t, /UTC\.\./);
});

test("control: a true rate limit still counts down", () => {
  const t = text(html(ErrorBox, { error: new ApiError("Too many requests.", 429, "rate_limited", true, 12), onRetry: () => {} }));
  assert.match(t, /Too many requests\. Try again in 12 s/);
});

// ---- 4. a long call cut by the hosting's timeout --------------------------
test("a non-JSON 504 becomes the timeout note, not 'The API returned 504.'", () => {
  const e = errorFor(504, "<html><body>upstream request timeout</body></html>");
  assert.deepEqual(e, { message: TIMEOUT_MESSAGE, status: 504, code: "timeout", retryable: false, retryAfter: null });
  const m = html(ErrorBox, { error: new ApiError(e.message, 504, "timeout", false), onRetry: () => {} });
  const t = text(m);
  assert.match(t, /taking longer than the page can wait\. The run may still finish: check Saved cases\./);
  assert.match(t, /Open Saved cases →/);
  assert.match(m, /href="\/cases"/);
  assert.doesNotMatch(t, /504|Try again/);
});

test("control: a JSON 504 with a code keeps that code", () => {
  const e = errorFor(504, { code: "model_unavailable", message: "the model is unavailable; try again" });
  assert.equal(e.code, "model_unavailable");
  assert.equal(e.retryable, true);
});

// ---- 5. backend prefixes ---------------------------------------------------
test("errorText strips Go-style prefixes and replaces a bare internal error", () => {
  assert.equal(errorText("bom: invalid input: not a readable Excel file: zip: not a valid zip file"), "Not a readable Excel file: zip: not a valid zip file");
  assert.equal(errorText("invalid input: plan search needs a case created from a structured product with a family"), "Plan search needs a case created from a structured product with a family");
  assert.equal(errorText("planner: invalid input: target and family are required"), "Target and family are required");
  assert.equal(errorText("internal error"), "Something went wrong on our side. Try again in a moment.");
  assert.equal(errorText(""), "Something went wrong on our side. Try again in a moment.");
  // Controls: a sentence written for people is left alone, and a field-coded
  // validation message keeps its code.
  assert.equal(errorText("economics: operations.horizon_required: horizon_years: a horizon is required"), "economics: operations.horizon_required: horizon_years: a horizon is required");
  assert.equal(errorText("Couldn't reach UndeLabs. Check your connection and try again."), "Couldn't reach UndeLabs. Check your connection and try again.");
  assert.equal(errorText("the model returned an unusable answer; try again", false), "the model returned an unusable answer; try again");
});

test("ErrorBox shows the stripped message", () => {
  const t = text(html(ErrorBox, { error: new ApiError("bom: invalid input: not a readable Excel file: zip: not a valid zip file", 400, "invalid_input", false) }));
  assert.equal(t, "Not a readable Excel file: zip: not a valid zip file");
  const model = text(html(ErrorBox, { error: new ApiError("the model returned an unusable answer; try again", 502, "model_bad_response", true), onRetry: () => {} }));
  assert.match(model, /^The model didn't answer: the model returned an unusable answer; try again/);
});

test("ErrorBox lead names the action that failed", () => {
  const t = text(html(ErrorBox, { error: new ApiError("the model is unavailable; try again", 502, "model_unavailable", true), onRetry: () => {}, lead: "Drafting parts and plants didn't work." }));
  assert.match(t, /^Drafting parts and plants didn't work\. The model didn't answer: the model is unavailable; try again Try again$/);
});

// ---- 2. plan search on a case with no plan template ------------------------
const { hasPlanTemplate, isNoTemplateError, NoPlanTemplate } = await import("../src/components/op/screens/plan-step.tsx");
const product = { name: "Widget", components: [{ name: "a", plant: "p1", made_in: "CN" }], plants: [{ id: "p1", name: "P1", country: "CN" }] };

test("hasPlanTemplate needs a structured product and a family with a template", () => {
  assert.equal(hasPlanTemplate({ product, family: "motorised_tools_small_appliances" }), true);
  assert.equal(hasPlanTemplate({ product, family: "toys_build_kits" }), true);
  assert.equal(hasPlanTemplate({ product }), false);
  assert.equal(hasPlanTemplate({ product, family: "" }), false);
  assert.equal(hasPlanTemplate({ product, family: "garden_furniture_none" }), false);
  assert.equal(hasPlanTemplate({ family: "motorised_tools_small_appliances" }), false);
});

test("the backend's refusals are recognised, and nothing else is", () => {
  assert.ok(isNoTemplateError(new ApiError("invalid input: plan search needs a case created from a structured product with a family", 400, "invalid_input", false)));
  assert.ok(isNoTemplateError(new ApiError('planner: unknown product family: "x"', 400, "invalid_input", false)));
  // The streamed error event arrives as a plain Error with the code attached.
  assert.ok(isNoTemplateError(Object.assign(new Error("invalid input: plan search needs a case created from a structured product with a family"), { code: "invalid_input" })));
  assert.ok(!isNoTemplateError(new ApiError("invalid input: target is required", 400, "invalid_input", false)));
  assert.ok(!isNoTemplateError(new ApiError("plan search needs a case created from a structured product", 502, "internal", true)));
});

test("the no-template panel offers a ruling request for a broker, not a search", () => {
  const m = html(NoPlanTemplate, { id: "c 1", family: "garden" });
  const t = text(m);
  assert.match(t, /No plan template for this product yet/);
  assert.match(t, /family has no template yet/);
  // The footer's "Draft for a broker →" renders through the action bar
  // portal; the Next steps column carries the same link.
  assert.match(t, /Draft for the current setup/);
  assert.match(m, /href="\/cases\/c%201\/draft"/);
  assert.match(m, /href="\/cases\/new\?from=c%201"/);
  assert.doesNotMatch(t, /\bSearch\b|invalid input/);
  assert.match(text(html(NoPlanTemplate, { id: "c1" })), /has no product family/);
});
