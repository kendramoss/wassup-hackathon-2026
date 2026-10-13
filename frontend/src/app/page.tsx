"use client";

import { useState } from "react";

type ChatResponse = {
  reply: string;
  session_id: string;
  tool_calls: string[];
};

export default function Home() {
  const [message, setMessage] = useState("");
  const [sessionId, setSessionId] = useState<string>();
  const [result, setResult] = useState<ChatResponse>();
  const [error, setError] = useState<string>();
  const [loading, setLoading] = useState(false);

  async function send(e: React.FormEvent) {
    e.preventDefault();
    if (!message.trim()) return;
    setLoading(true);
    setError(undefined);
    try {
      const res = await fetch("/api/chat", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ message, session_id: sessionId }),
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error ?? `HTTP ${res.status}`);
      setResult(data);
      setSessionId(data.session_id);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="mx-auto flex w-full max-w-2xl flex-col gap-4 p-8">
      <h1 className="text-2xl font-semibold">Agent hello world</h1>
      <form onSubmit={send} className="flex gap-2">
        <input
          className="flex-1 rounded border px-3 py-2"
          value={message}
          onChange={(e) => setMessage(e.target.value)}
          placeholder="Try: what time is it in Singapore?"
        />
        <button
          className="rounded bg-black px-4 py-2 text-white disabled:opacity-50"
          disabled={loading}
        >
          {loading ? "…" : "Send"}
        </button>
      </form>
      {error && <p className="text-red-600">{error}</p>}
      {result && (
        <div className="rounded border p-4">
          <p className="whitespace-pre-wrap">{result.reply}</p>
          <p className="mt-2 text-sm text-gray-500">
            Tools called: {result.tool_calls.length ? result.tool_calls.join(", ") : "none"}
          </p>
        </div>
      )}
    </main>
  );
}
