import { callBackend, errorResponse } from "@/lib/backend";

// Proxies a chat message to the backend's /chat/stream and passes the
// server-sent events through as they arrive. Aborting the browser request
// cancels the backend call too.
export async function POST(request: Request) {
  try {
    const res = await callBackend("/chat/stream", await request.text(), request.signal, { request });
    return new Response(res.body, {
      headers: {
        "Content-Type": "text/event-stream",
        // no-transform keeps Next's compression from buffering the stream.
        "Cache-Control": "no-cache, no-transform",
        "X-Accel-Buffering": "no",
      },
    });
  } catch (err) {
    return errorResponse(err);
  }
}
