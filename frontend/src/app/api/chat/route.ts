import { callBackend, errorResponse } from "@/lib/backend";

// Proxies a chat message to the backend's /chat and returns the whole reply.
// The page uses /api/chat/stream; this stays for scripts and the smoke test.
export async function POST(request: Request) {
  try {
    const res = await callBackend("/chat", await request.text(), undefined, { request });
    return Response.json(await res.json());
  } catch (err) {
    return errorResponse(err);
  }
}
