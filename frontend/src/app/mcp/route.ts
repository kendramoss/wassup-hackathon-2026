import { proxyMcp } from "@/lib/mcp-proxy";

// The public MCP endpoint (docs/MCP.md): Streamable HTTP, POST only (the
// backend is stateless, so there is no GET stream and no session to DELETE).
export function POST(request: Request) {
  return proxyMcp(request);
}
