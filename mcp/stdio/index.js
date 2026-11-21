#!/usr/bin/env node
// undelabs-mcp: a stdio MCP server for MCP clients that prefer a local
// process (docs/MCP.md). It is a relay with no dependencies: each JSON-RPC
// message read from stdin (one per line) is POSTed to the hosted Streamable
// HTTP endpoint, and every JSON-RPC message in the answer (plain JSON or a
// text/event-stream) is written to stdout, one per line. The hosted server is
// stateless, so the relay keeps no session either. Logs go to stderr only.
//
// Environment:
//   UNDE_API_KEY   optional API key (op_...), sent as "Authorization: Bearer".
//                  Without it calls are anonymous, with per-address limits.
//   UNDE_BASE_URL  default https://undelabs.dev; the endpoint is <base>/mcp.
import { createInterface } from "node:readline";

const base = (process.env.UNDE_BASE_URL || "https://undelabs.dev").replace(/\/+$/, "");
const endpoint = `${base}/mcp`;
const key = (process.env.UNDE_API_KEY || "").trim();
if (key && !key.startsWith("op_")) {
  process.stderr.write("undelabs-mcp: UNDE_API_KEY should start with op_\n");
}
let protocolVersion = "";

const write = (msg) => process.stdout.write(JSON.stringify(msg) + "\n");
const rpcError = (id, message, code = -32000) => write({ jsonrpc: "2.0", id, error: { code, message } });

// sseMessages yields the JSON payload of each event in an SSE body.
async function* sseMessages(body) {
  const decoder = new TextDecoder();
  let buf = "";
  for await (const chunk of body) {
    buf += decoder.decode(chunk, { stream: true });
    let cut;
    while ((cut = buf.search(/\r?\n\r?\n/)) >= 0) {
      const event = buf.slice(0, cut);
      buf = buf.slice(cut).replace(/^\r?\n\r?\n/, "");
      const data = event.split(/\r?\n/).filter((l) => l.startsWith("data:")).map((l) => l.slice(5).replace(/^ /, "")).join("\n");
      if (data) yield data;
    }
  }
}

async function relay(line) {
  let msg;
  try {
    msg = JSON.parse(line);
  } catch {
    return rpcError(null, "parse error", -32700);
  }
  const id = msg && typeof msg === "object" && "id" in msg ? msg.id : undefined;
  const headers = { "Content-Type": "application/json", Accept: "application/json, text/event-stream" };
  if (key) headers.Authorization = `Bearer ${key}`;
  if (protocolVersion) headers["MCP-Protocol-Version"] = protocolVersion;
  let res;
  try {
    res = await fetch(endpoint, { method: "POST", headers, body: line });
  } catch (err) {
    if (id !== undefined) rpcError(id, `could not reach ${endpoint}: ${err?.cause?.code ?? err?.message ?? "network error"}`);
    return;
  }
  if (res.status === 202 || res.status === 204) return; // a notification or response was accepted
  const type = res.headers.get("content-type") || "";
  if (res.ok && type.includes("event-stream")) {
    for await (const data of sseMessages(res.body)) emit(data);
    return;
  }
  const text = await res.text().catch(() => "");
  if (res.ok || isRpc(text)) return emit(text); // an error the server already put in JSON-RPC form
  if (id !== undefined) rpcError(id, `HTTP ${res.status} from ${endpoint}: ${text.slice(0, 300)}`);
}

function isRpc(text) {
  try {
    return JSON.parse(text)?.jsonrpc === "2.0";
  } catch {
    return false;
  }
}

function emit(text) {
  if (!text.trim()) return;
  let m;
  try {
    m = JSON.parse(text);
  } catch {
    process.stderr.write("undelabs-mcp: dropped a non-JSON message from the server\n");
    return;
  }
  for (const one of Array.isArray(m) ? m : [m]) {
    const v = one?.result?.protocolVersion;
    if (typeof v === "string") protocolVersion = v; // sent on later requests, as the transport requires
    write(one);
  }
}

const pending = new Set();
const rl = createInterface({ input: process.stdin, crlfDelay: Infinity });
rl.on("line", (line) => {
  if (!line.trim()) return;
  const p = relay(line).catch((err) => process.stderr.write(`undelabs-mcp: ${err?.message ?? err}\n`));
  pending.add(p);
  p.finally(() => pending.delete(p));
});
rl.on("close", async () => {
  await Promise.allSettled([...pending]);
  process.exit(0);
});
