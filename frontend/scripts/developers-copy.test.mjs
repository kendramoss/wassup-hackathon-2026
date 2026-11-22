import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { filterTools, rates, toolGroups, tools } from "../src/app/developers/catalog.ts";

const read = (path) => readFileSync(new URL(path, import.meta.url), "utf8");
const page = read("../src/app/developers/page.tsx");
const portal = read("../src/app/developers/portal.tsx");
const mcp = read("../../docs/MCP.md");
const documented = [...mcp.matchAll(/^\| `([a-z_]+)` \| `[A-Z]+ [^`]+`[^|]*\| ([^|]+) \|/gm)].map(([, name, cls]) => [name, cls.trim()]);

test("the catalog lists each documented tool once, with its rate class", () => {
  const classes = { basic: "basic", light: "light", heavy: "heavy", lookup: "ruling lookup" };
  assert.ok(documented.length >= 16, `read ${documented.length} tools from MCP.md`);
  assert.deepEqual(tools.map(([name]) => name), documented.map(([name]) => name));
  assert.equal(new Set(tools.map(([name]) => name)).size, tools.length);
  const docsByName = Object.fromEntries(documented);
  for (const [name, group, rate, input, output] of tools) {
    assert.ok(docsByName[name].startsWith(classes[rate]), name);
    assert.ok(toolGroups[group] && input && output, name);
  }
});

test("catalog search accepts the tool name and task, and intersects the category", () => {
  assert.equal(filterTools("", "").length, tools.length);
  assert.deepEqual(filterTools("  PREDICT origin  ", "").map(([name]) => name), ["predict_origin"]);
  assert.deepEqual(filterTools("review decisions", "broker").map(([name]) => name), ["get_broker_review_status"]);
  assert.deepEqual(filterTools("predict_origin", "tariff"), []);
  assert.deepEqual(Object.keys(toolGroups), ["origin", "planning", "tariff", "broker"]);
});

test("copyable connection examples use the documented endpoint, key header and cookie jar", () => {
  const command = mcp.match(/^gemini mcp add --transport http unde\s+\S+/m)[0];
  for (const expected of [command, "https://undelabs.dev/api/v1/cases/predict", "X-API-Key: op_...", "curl -c jar -b jar"])
    assert.ok(portal.includes(expected), expected);
  assert.match(page, /OpenAPI spec/);
  assert.match(page, /docs\/api\/openapi\.yaml/);
});

test("both modes preserve scope, case access and error disclosure", () => {
  for (const text of [
    "No key needed to start",
    "case_access_key",
    "No tool deletes anything",
    "Call from your server",
    "Canada check is rules only, with no accuracy figure",
    "Not a ruling. Confirm with a licensed customs broker or a CBP binding ruling.",
    "illustrative",
    "unauthorized",
    "rate_limited",
    "quota_exceeded",
    "too_many_in_flight",
    "client_budget_reached",
    "auth_rate_limited",
    "budget_reached",
    "Retry-After",
  ]) assert.ok(portal.includes(text), text);
  assert.deepEqual(rates, { basic: "120 req/min", light: "20 req/min", heavy: "4 req/min", lookup: "30 req/min" });
  assert.doesNotMatch(page + portal, /LiteLLM|\bD\d{2,3}\b|op_visitor|X-Visitor-ID|HttpOnly/);
});
