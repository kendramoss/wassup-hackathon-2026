// Client for /api/chat/stream. Mirrors streamEvent in backend/main.go.
export type StreamEvent =
  | { type: "session"; session_id: string }
  | { type: "text"; text: string }
  | { type: "tool_call"; id?: string; name: string; args?: Record<string, unknown> }
  | { type: "tool_result"; id?: string; name: string; result?: Record<string, unknown> }
  // A tool call (name, args) waiting for a person; answer with an Approval.
  | { type: "approval_request"; id: string; name?: string; args?: Record<string, unknown>; text?: string }
  | { type: "done" }
  | { type: "error"; error: string };

export type Approval = { id: string; approved: boolean; reason?: string };

// Sends a message (or an answer to an approval request) and calls onEvent
// for each server-sent event until the stream ends. Throws on HTTP errors with the server's error message.
export async function streamChat(
  body: { message: string; session_id?: string } | { approval: Approval; session_id: string },
  onEvent: (e: StreamEvent) => void,
  signal: AbortSignal,
): Promise<void> {
  const res = await fetch("/api/chat/stream", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
    signal,
  });
  if (!res.ok || !res.body) {
    const data = await res.json().catch(() => ({}));
    throw new Error(data.error || `Request failed (${res.status})`);
  }

  const reader = res.body.pipeThrough(new TextDecoderStream()).getReader();
  let buffer = "";
  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += value;
    let end;
    while ((end = buffer.indexOf("\n\n")) !== -1) {
      const chunk = buffer.slice(0, end);
      buffer = buffer.slice(end + 2);
      for (const line of chunk.split("\n")) {
        if (line.startsWith("data: ")) onEvent(JSON.parse(line.slice(6)));
      }
    }
  }
}
