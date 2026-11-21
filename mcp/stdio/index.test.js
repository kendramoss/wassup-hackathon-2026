// Spawns the relay against a mock Streamable HTTP endpoint and checks it
// lists tools, relays SSE answers, sends the key as a bearer token and maps
// HTTP failures to JSON-RPC errors.
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { createServer } from "node:http";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

const script = fileURLToPath(new URL("./index.js", import.meta.url));

function mock() {
  const seen = [];
  const srv = createServer((req, res) => {
    let body = "";
    req.on("data", (c) => (body += c));
    req.on("end", () => {
      const msg = JSON.parse(body);
      seen.push({ url: req.url, auth: req.headers.authorization, version: req.headers["mcp-protocol-version"], method: msg.method });
      if (req.url !== "/mcp") return res.writeHead(404).end("no");
      if (msg.method === "initialize") {
        res.writeHead(200, { "content-type": "application/json" });
        return res.end(JSON.stringify({ jsonrpc: "2.0", id: msg.id, result: { protocolVersion: "2025-06-18", capabilities: { tools: {} }, serverInfo: { name: "unde", version: "test" } } }));
      }
      if (msg.method === "notifications/initialized") return res.writeHead(202).end();
      if (msg.method === "tools/list") {
        res.writeHead(200, { "content-type": "text/event-stream" });
        res.write(`event: message\ndata: ${JSON.stringify({ jsonrpc: "2.0", id: msg.id, result: { tools: [{ name: "predict_origin", inputSchema: { type: "object" } }] } })}\n\n`);
        return res.end();
      }
      res.writeHead(403, { "content-type": "application/json" }).end(`{"code":"forbidden","message":"no"}`);
    });
  });
  return new Promise((resolve) => srv.listen(0, "127.0.0.1", () => resolve({ srv, seen, url: `http://127.0.0.1:${srv.address().port}` })));
}

function run(env, lines) {
  return new Promise((resolve, reject) => {
    const child = spawn(process.execPath, [script], { env: { ...process.env, ...env }, stdio: ["pipe", "pipe", "pipe"] });
    let out = "";
    child.stdout.on("data", (c) => (out += c));
    child.on("error", reject);
    child.on("close", () => resolve(out.trim().split("\n").filter(Boolean).map((l) => JSON.parse(l))));
    // Send one line at a time so the version from initialize is known.
    (async () => {
      for (const l of lines) {
        child.stdin.write(JSON.stringify(l) + "\n");
        await new Promise((r) => setTimeout(r, 150));
      }
      child.stdin.end();
    })();
  });
}

test("initialize, tools/list over SSE, a notification, and an HTTP error", async () => {
  const { srv, seen, url } = await mock();
  try {
    const out = await run({ UNDE_BASE_URL: url + "/", UNDE_API_KEY: "op_test" }, [
      { jsonrpc: "2.0", id: 1, method: "initialize", params: { protocolVersion: "2025-06-18", capabilities: {}, clientInfo: { name: "t", version: "0" } } },
      { jsonrpc: "2.0", method: "notifications/initialized" },
      { jsonrpc: "2.0", id: 2, method: "tools/list" },
      { jsonrpc: "2.0", id: 3, method: "tools/call", params: { name: "x" } },
    ]);
    const byId = Object.fromEntries(out.map((m) => [m.id, m]));
    assert.equal(byId[1].result.serverInfo.name, "unde");
    assert.deepEqual(byId[2].result.tools.map((t) => t.name), ["predict_origin"]);
    assert.match(byId[3].error.message, /HTTP 403/);
    assert.equal(out.length, 3, "nothing written for the notification");
    assert.ok(seen.every((s) => s.url === "/mcp" && s.auth === "Bearer op_test"));
    assert.equal(seen.find((s) => s.method === "tools/list").version, "2025-06-18");
  } finally {
    srv.close();
  }
});

test("without a key no Authorization header is sent; an unreachable endpoint is a JSON-RPC error", async () => {
  const { srv, seen, url } = await mock();
  try {
    await run({ UNDE_BASE_URL: url, UNDE_API_KEY: "" }, [{ jsonrpc: "2.0", id: 1, method: "tools/list" }]);
    assert.equal(seen[0].auth, undefined);
  } finally {
    srv.close();
  }
  const out = await run({ UNDE_BASE_URL: "http://127.0.0.1:9" }, [{ jsonrpc: "2.0", id: 7, method: "tools/list" }]);
  assert.equal(out[0].id, 7);
  assert.match(out[0].error.message, /could not reach/);
});
