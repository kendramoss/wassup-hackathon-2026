import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

// Copy sweep 4 (#680, rule 16): /developers is written for a developer who
// wants to connect in two minutes. Setup detail moved into Details folds, but
// the tool list must stay complete and match docs/MCP.md (which
// api_docs_test.go checks against server.go), and the key facts stay visible.
const read = (p) => readFileSync(new URL(p, import.meta.url), "utf8");
const page = read("../src/app/developers/page.tsx");
const mcp = read("../../docs/MCP.md");
const flat = page.replace(/\s+/g, " ");

// The page source with every <Details …>…</Details> block removed.
const outside = (src) =>
  src
    .split(/<Details[ >]/)
    .map((s, i) => (i === 0 ? s : s.slice(s.indexOf("</Details>") + 10)))
    .join(" ");
const jsx = outside(flat.slice(flat.indexOf("export default function Page")));

const pageTools = [...page.matchAll(/^\s*\["([a-z_]+)", "[^"]+", "(basic|light|heavy|lookup)"\],$/gm)].map((m) => [m[1], m[2]]);
const docTools = [...mcp.matchAll(/^\| `([a-z_]+)` \| `[A-Z]+ [^`]+`[^|]*\| ([^|]+) \|/gm)].map((m) => [m[1], m[2].trim()]);

test("the tool table lists every tool in docs/MCP.md, in order", () => {
  assert.ok(docTools.length >= 16, `read ${docTools.length} tools from MCP.md`);
  assert.deepEqual(
    pageTools.map((t) => t[0]),
    docTools.map((t) => t[0]),
  );
  for (const t of ["check_rcep_preference", "check_canada_marking"]) assert.ok(pageTools.some((p) => p[0] === t), t);
});

test("each tool's limit is its class in docs/MCP.md", () => {
  const words = { basic: "basic", light: "light", heavy: "heavy", lookup: "ruling lookup" };
  const doc = Object.fromEntries(docTools);
  for (const [name, cls] of pageTools) assert.ok(doc[name].startsWith(words[cls]), `${name}: page ${cls}, MCP.md ${doc[name]}`);
  assert.match(page, /basic: "120 a minute", light: "20 a minute", heavy: "4 a minute", lookup: "30 a minute"/);
  assert.match(mcp, /basic 120 a minute \(no model call\), light 20 a minute, heavy 4 a minute, ruling lookup 30 a minute/);
});

test("the limits table names every limit error code", () => {
  for (const code of ["rate_limited", "too_many_in_flight", "quota_exceeded", "client_budget_reached", "budget_reached"])
    assert.ok(jsx.includes(code) || page.includes(`>429 ${code}<`) || page.includes(`>503 ${code}<`), code);
});

test("the market codes in Details are the ones docs/MCP.md lists", () => {
  const codes = [...mcp.match(/the market the goods enter \(([^)]+)\)/)[1].matchAll(/`([a-z]+)`/g)].map((m) => m[1]);
  const shown = [...flat.match(/the market the goods enter \((.+?)\)\. /)[1].matchAll(/<C>([a-z]+)<\/C>/g)].map((m) => m[1]);
  assert.deepEqual(shown, codes);
});

test("the quickstart leads: MCP config, a prompt, one curl", () => {
  const steps = [...page.matchAll(/<Step n=\{(\d)\} title="([^"]+)">/g)].map((m) => m[2]);
  assert.deepEqual(steps, ["Add the MCP server", "Ask your agent", "Or call the API"]);
  assert.ok(page.indexOf("<Step n={1}") < page.indexOf('id="mcp"'), "the quickstart comes first");
});

test("the key facts stay visible, outside Details", () => {
  for (const s of [
    "Every answer is a prediction, not a ruling",
    "No key needed to start",
    "Only U.S. imports get an origin prediction",
    "No tool deletes anything",
    "Call from your server",
    "Built, off here: the audit trail and required sign-in",
    "case_access_key",
    "X-API-Key: op_…",
    "Authorization: Bearer op_…",
  ])
    assert.ok(jsx.includes(s), s);
});

test("the open page has no gateway, decision numbers or cookie internals", () => {
  assert.doesNotMatch(jsx, /gateway|LiteLLM|\bD\d{2,3}\b|op_visitor|X-Visitor-ID|HttpOnly/);
  // Each visible paragraph stays short (the issue counted 6 over 350 characters).
  for (const m of jsx.matchAll(/<p className="duo[^"]*">(.*?)<\/p>/g)) {
    const text = m[1].replace(/<[^>]+>/g, "").replace(/\{" "\}/g, " ");
    assert.ok(text.length <= 200, `paragraph over 200 characters: ${text.slice(0, 80)}…`);
  }
});
