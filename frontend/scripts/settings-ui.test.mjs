import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const read = (path) => readFileSync(new URL(path, import.meta.url), "utf8");
const page = read("../src/app/settings/page.tsx");
const panel = read("../src/app/settings/panel.tsx");
const signIn = read("../src/components/op/sign-in.tsx");
const developers = read("../src/app/developers/page.tsx");
const client = read("../src/lib/api/client.ts");
const mcp = read("../../docs/MCP.md");
const security = read("../../docs/api/SECURITY.md");
const openapi = read("../../docs/api/openapi.yaml");

test("settings manages only the signed-in user's keys", () => {
  assert.match(panel, /fetch\("\/api\/session", \{ cache: "no-store" \}\)/);
  assert.match(panel, /api\.listApiKeys\(\)/);
  assert.match(panel, /api\.createApiKey\(name\.trim\(\)\)/);
  assert.match(panel, /api\.revokeApiKey\(key\.id\)/);
  assert.match(panel, /setSecret\(out\.key\)/);
  assert.match(panel, /window\.confirm\(copy\.settings\.apiKeys\.revokeConfirm\)/);
  assert.match(panel, /setLoadFailed\(true\)/);
  assert.match(panel, /copy\.settings\.apiKeys\.retry/);
  assert.doesNotMatch(panel, /localStorage|sessionStorage|\bdaily_quota\b/);
  assert.match(page, /<SettingsPanel \/>/);
});

test("the typed client sends only a name, never a quota or owner", () => {
  assert.match(client, /listApiKeys: \(\) => call<ApiKeyList>\("\/api-keys"\)/);
  assert.match(client, /createApiKey: \(name: string\) => post<ApiKeyIssue>\("\/api-keys", \{ name \}\)/);
  assert.match(client, /revokeApiKey: \(id: string\)/);
  const start = client.indexOf("  listApiKeys:");
  const end = client.indexOf("  // Broker review links, owner side");
  assert.doesNotMatch(client.slice(start, end), /daily_quota|owner/);
});

test("the account menu and Developers page lead to Settings", () => {
  assert.match(signIn, /href="\/settings"/);
  assert.match(signIn, /\{copy\.settings\.apiKeys\.menu\}/);
  assert.match(developers, /href="\/settings"/);
  assert.match(developers, /copy\.developers\.keyLead/);
  assert.match(developers, /copy\.developers\.keyLink/);
});

test("the docs describe self-service keys without exposing a secret after creation", () => {
  assert.match(mcp, /POST \/v1\/api-keys/);
  assert.match(mcp, /200 requests per UTC day/);
  assert.match(mcp, /GET \/v1\/api-keys/);
  assert.match(mcp, /DELETE \/v1\/api-keys\/\{id\}/);
  assert.match(security, /quota \*\*fixed at 200\*\*/);
  assert.match(security, /apikey\.issue/, "audit actions are documented");
  assert.match(openapi, /Signed-in users also create, list and revoke their own keys under `\/v1\/api-keys`/);
});
