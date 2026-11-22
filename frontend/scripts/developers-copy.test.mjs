import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { filterTools, rates, toolGroups, tools } from "../src/app/developers/catalog.ts";
import { endpoints, keyHeader, mcpEndpoint, setupExamples, toolExamples } from "../src/app/developers/examples.ts";

const read = (path) => readFileSync(new URL(path, import.meta.url), "utf8");
const page = read("../src/app/developers/page.tsx");
const portal = read("../src/app/developers/portal.tsx");
const mcp = read("../../docs/MCP.md");
const openapi = read("../../docs/api/openapi.yaml");
const documented = [...mcp.matchAll(/^\| `([a-z_]+)` \| `[A-Z]+ [^`]+`[^|]*\| ([^|]+) \|/gm)].map(([, name, cls]) => [name, cls.trim()]);

test("the tool explorer covers every documented tool and has an input and output sample", () => {
  const classes = { basic: "basic", light: "light", heavy: "heavy", lookup: "ruling lookup" };
  assert.ok(documented.length >= 16);
  assert.deepEqual(tools.map(([name]) => name), documented.map(([name]) => name));
  assert.deepEqual(Object.keys(toolExamples), tools.map(([name]) => name));
  const docsByName = Object.fromEntries(documented);
  for (const [name, group, rate, input, output] of tools) {
    assert.ok(docsByName[name].startsWith(classes[rate]), name);
    assert.ok(toolGroups[group] && input && output, name);
    assert.equal(typeof toolExamples[name].input, "object", name);
    assert.ok(Object.keys(toolExamples[name].output).length, name);
  }
  assert.ok(toolExamples.plan_production_moves.input.target_country);
  assert.ok(!("plan_production_move" in toolExamples));
  assert.ok(!("check_tariff_shift" in toolExamples));
  assert.ok(toolExamples.check_canada_marking && toolExamples.check_rcep_preference);
});

test("catalog search accepts the tool name and task, and intersects the category", () => {
  assert.equal(filterTools("", "").length, tools.length);
  assert.deepEqual(filterTools("  PREDICT origin  ", "").map(([name]) => name), ["predict_origin"]);
  assert.deepEqual(filterTools("review decisions", "broker").map(([name]) => name), ["get_broker_review_status"]);
  assert.deepEqual(filterTools("predict_origin", "tariff"), []);
});

test("the three setup examples follow the hosted endpoint and repository instructions", () => {
  assert.equal(mcpEndpoint, "https://undelabs.dev/mcp");
  assert.equal(setupExamples.gemini.code, mcp.match(/^gemini mcp add --transport http unde\s+\S+/m)[0]);
  assert.match(JSON.parse(setupExamples.claude.code).mcpServers.unde.args[0], /\/mcp\/stdio\/index\.js$/);
  assert.match(setupExamples.adk.code, /mcp\.StreamableClientTransport/);
  assert.match(setupExamples.adk.note, /Illustrative/);
});

test("the REST console paths, response status and key name follow OpenAPI", () => {
  assert.equal(keyHeader, "X-API-Key: op_...");
  for (const { label } of Object.values(endpoints)) {
    const [, path] = label.split(" ");
    assert.ok(openapi.includes(`  ${path}:\n`), path);
  }
  assert.match(endpoints.predict.command, /\/api\/v1\/cases\/predict/);
  assert.match(endpoints.plan.command, /\/api\/v1\/cases\/\$CASE_ID\/plan/);
  assert.match(endpoints.ruling.command, /\/api\/v1\/rulings\/N328954/);
  assert.equal(endpoints.predict.status, "201");
  assert.equal(endpoints.predict.output.prediction.confidence, 0.96);
  assert.match(endpoints.plan.output.disclaimer, /^Not a ruling\./);
  assert.match(endpoints.predict.output.disclaimer, /^Not a ruling\./);
  assert.match(openapi, /^openapi: 3\.1\./);
  assert.match(page, /OpenAPI 3\.1 Spec/);
  assert.deepEqual(rates, { basic: "120 req/min", light: "20 req/min", heavy: "4 req/min", lookup: "30 req/min" });
});

test("the playground labels responses as examples and sends no real calls", () => {
  assert.match(page, /Local examples only/);
  assert.match(portal, /Structured output · example/);
  assert.match(portal, /response · example/);
  assert.doesNotMatch(portal, /\bfetch\s*\(|XMLHttpRequest/);
  assert.doesNotMatch(page + portal, /LiteLLM|\bD\d{2,3}\b|op_visitor|X-Visitor-ID|HttpOnly/);
});
