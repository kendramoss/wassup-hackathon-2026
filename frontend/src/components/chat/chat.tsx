"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { streamChat, type StreamEvent } from "@/lib/chat-stream";
import { site } from "@/lib/site";
import { AlertCircle, ArrowUp, Check, Copy, Plus, Retry, Stop } from "../icons";
import { SiteHeader } from "../site-header";
import { Markdown } from "./markdown";
import { ToolCall, type ToolPart } from "./tool-call";

const MAX_CHARS = 2000; // backend limit (maxMessageRunes)

type TextPart = { kind: "text"; text: string };
type Part = TextPart | ToolPart;
type Status = "streaming" | "done" | "stopped" | "error";
type Message =
  | { id: string; role: "user"; text: string }
  | { id: string; role: "assistant"; parts: Part[]; status: Status; error?: string };

// Applies one stream event to an assistant message's parts.
function applyEvent(parts: Part[], e: StreamEvent): Part[] {
  switch (e.type) {
    case "text": {
      const last = parts.at(-1);
      if (last?.kind === "text") return [...parts.slice(0, -1), { kind: "text", text: last.text + e.text }];
      return [...parts, { kind: "text", text: e.text }];
    }
    case "tool_call":
      return [...parts, { kind: "tool", id: e.id, name: e.name, args: e.args }];
    case "tool_result": {
      const i = parts.findIndex(
        (p) => p.kind === "tool" && p.result === undefined && (e.id ? p.id === e.id : p.name === e.name),
      );
      if (i === -1) return parts;
      return parts.map((p, j) => (j === i ? { ...(p as ToolPart), result: e.result ?? {} } : p));
    }
    default:
      return parts;
  }
}

const replyText = (parts: Part[]) =>
  parts
    .filter((p): p is TextPart => p.kind === "text")
    .map((p) => p.text)
    .join("\n\n");

export function Chat() {
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState("");
  const [sessionId, setSessionId] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [announcement, setAnnouncement] = useState("");
  const abortRef = useRef<AbortController | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const stickToBottom = useRef(true);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const idCounter = useRef(0);
  const newId = () => `m${++idCounter.current}`;

  const updateAssistant = useCallback((id: string, fn: (m: Extract<Message, { role: "assistant" }>) => Message) => {
    setMessages((ms) => ms.map((m) => (m.id === id && m.role === "assistant" ? fn(m) : m)));
  }, []);

  const send = useCallback(
    async (text: string) => {
      const message = text.trim();
      if (!message || busy) return;
      const assistantId = newId();
      setMessages((ms) => [
        ...ms,
        { id: newId(), role: "user", text: message },
        { id: assistantId, role: "assistant", parts: [], status: "streaming" },
      ]);
      setInput("");
      setBusy(true);
      setAnnouncement("Agent is replying");
      stickToBottom.current = true;

      const controller = new AbortController();
      abortRef.current = controller;
      let finalParts: Part[] = [];
      try {
        await streamChat(
          { message, session_id: sessionId },
          (e) => {
            if (e.type === "session") setSessionId(e.session_id);
            else if (e.type === "error") throw new Error(e.error);
            else {
              finalParts = applyEvent(finalParts, e);
              const parts = finalParts;
              updateAssistant(assistantId, (m) => ({ ...m, parts }));
            }
          },
          controller.signal,
        );
        updateAssistant(assistantId, (m) => ({ ...m, status: "done" }));
        setAnnouncement(replyText(finalParts) || "Agent replied");
      } catch (err) {
        if (controller.signal.aborted) {
          updateAssistant(assistantId, (m) => ({ ...m, status: "stopped" }));
          setAnnouncement("Stopped");
        } else {
          const error = err instanceof Error ? err.message : String(err);
          updateAssistant(assistantId, (m) => ({ ...m, status: "error", error }));
          setAnnouncement(`Error: ${error}`);
        }
      } finally {
        abortRef.current = null;
        setBusy(false);
        inputRef.current?.focus();
      }
    },
    [busy, sessionId, updateAssistant],
  );

  // Removes a failed reply and its question, then asks again.
  const retry = (assistantId: string) => {
    const i = messages.findIndex((m) => m.id === assistantId);
    const question = messages[i - 1];
    if (question?.role !== "user") return;
    setMessages((ms) => ms.filter((_, j) => j !== i && j !== i - 1));
    send(question.text);
  };

  const newChat = () => {
    abortRef.current?.abort();
    setMessages([]);
    setSessionId(undefined);
    setInput("");
    setAnnouncement("New chat");
    inputRef.current?.focus();
  };

  // A prompt in the URL (?q=..., from the landing page) is sent once.
  const sentFromUrl = useRef(false);
  useEffect(() => {
    if (sentFromUrl.current) return;
    sentFromUrl.current = true;
    const q = new URLSearchParams(window.location.search).get("q");
    if (q) {
      window.history.replaceState(null, "", window.location.pathname);
      queueMicrotask(() => send(q.slice(0, MAX_CHARS)));
    }
  }, [send]);

  // Keep the newest content in view unless the user has scrolled up.
  useEffect(() => {
    const el = scrollRef.current;
    if (el && stickToBottom.current) el.scrollTop = el.scrollHeight;
  }, [messages]);

  // Grow the input with its content, up to max-h.
  useEffect(() => {
    const el = inputRef.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${el.scrollHeight}px`;
  }, [input]);

  const empty = messages.length === 0;

  return (
    <div className="flex h-dvh flex-col">
      <SiteHeader>
        <button
          type="button"
          onClick={newChat}
          disabled={empty}
          className="inline-flex h-9 items-center gap-1.5 rounded-md px-3 text-sm text-muted hover:bg-subtle hover:text-fg disabled:pointer-events-none disabled:opacity-40"
        >
          <Plus />
          New chat
        </button>
      </SiteHeader>

      <div
        ref={scrollRef}
        onScroll={(e) => {
          const el = e.currentTarget;
          stickToBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
        }}
        className="flex-1 overflow-y-auto"
      >
        <main id="main" className="mx-auto flex min-h-full max-w-3xl flex-col px-4 py-6">
          <h1 className="sr-only">Chat with {site.name}</h1>
          {empty ? (
            <EmptyState onPick={send} />
          ) : (
            <ol className="flex flex-col gap-6" aria-label="Conversation">
              {messages.map((m) =>
                m.role === "user" ? (
                  <li key={m.id} className="flex justify-end">
                    <div className="max-w-[85%] whitespace-pre-wrap rounded-2xl bg-subtle px-4 py-2.5 leading-relaxed">
                      <span className="sr-only">You: </span>
                      {m.text}
                    </div>
                  </li>
                ) : (
                  <li key={m.id}>
                    <AssistantMessage message={m} onRetry={() => retry(m.id)} />
                  </li>
                ),
              )}
            </ol>
          )}
        </main>
      </div>

      <div className="border-t border-border bg-bg">
        <form
          aria-label="Send a message"
          className="mx-auto max-w-3xl px-4 py-3"
          onSubmit={(e) => {
            e.preventDefault();
            send(input);
          }}
        >
          <div className="flex items-end gap-2 rounded-2xl border border-border bg-bg px-3 py-2 focus-within:border-accent">
            <label htmlFor="message" className="sr-only">
              Message
            </label>
            <textarea
              id="message"
              ref={inputRef}
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
                  e.preventDefault();
                  send(input);
                }
              }}
              rows={1}
              maxLength={MAX_CHARS}
              placeholder="Ask anything"
              aria-describedby="composer-hint"
              className="max-h-48 min-h-6 flex-1 resize-none bg-transparent py-1 leading-6 outline-none placeholder:text-muted focus-visible:outline-none"
            />
            {busy ? (
              <button
                type="button"
                onClick={() => abortRef.current?.abort()}
                aria-label="Stop"
                title="Stop"
                className="inline-flex size-8 shrink-0 items-center justify-center rounded-full bg-fg text-bg"
              >
                <Stop />
              </button>
            ) : (
              <button
                type="submit"
                disabled={!input.trim()}
                aria-label="Send"
                title="Send"
                className="inline-flex size-8 shrink-0 items-center justify-center rounded-full bg-accent text-accent-fg disabled:opacity-30"
              >
                <ArrowUp />
              </button>
            )}
          </div>
          <p id="composer-hint" className="mt-1.5 flex justify-between px-1 text-xs text-muted">
            <span>Enter to send, Shift+Enter for a new line</span>
            {input.length > MAX_CHARS * 0.9 && (
              <span>
                {input.length} / {MAX_CHARS}
              </span>
            )}
          </p>
        </form>
      </div>

      <div role="status" aria-live="polite" className="sr-only">
        {announcement}
      </div>
    </div>
  );
}

function EmptyState({ onPick }: { onPick: (text: string) => void }) {
  return (
    <div className="m-auto flex w-full max-w-xl flex-col items-center gap-6 py-12 text-center">
      <div>
        <p className="text-2xl font-semibold tracking-tight">What can I help with?</p>
        <p className="mt-2 text-muted">{site.tagline}</p>
      </div>
      <ul className="flex w-full flex-col gap-2" aria-label="Example questions">
        {site.examples.map((ex) => (
          <li key={ex}>
            <button
              type="button"
              onClick={() => onPick(ex)}
              className="w-full rounded-lg border border-border px-4 py-3 text-left text-sm hover:bg-subtle"
            >
              {ex}
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}

function AssistantMessage({
  message,
  onRetry,
}: {
  message: Extract<Message, { role: "assistant" }>;
  onRetry: () => void;
}) {
  const { parts, status, error } = message;
  const text = replyText(parts);
  const waiting = status === "streaming" && parts.length === 0;

  return (
    <div className="flex flex-col gap-3">
      <span className="sr-only">{site.name}: </span>
      {waiting && <TypingIndicator />}
      {parts.map((p, i) =>
        p.kind === "tool" ? (
          <ToolCall key={i} tool={p} running={status === "streaming"} />
        ) : (
          <Markdown key={i}>{p.text}</Markdown>
        ),
      )}
      {status === "error" && (
        <div className="flex flex-wrap items-center gap-3 rounded-lg border border-danger/40 px-3 py-2 text-sm text-danger">
          <AlertCircle className="shrink-0" />
          <span className="flex-1">{error || "Something went wrong."}</span>
          <button
            type="button"
            onClick={onRetry}
            className="inline-flex items-center gap-1.5 rounded-md px-2 py-1 font-medium hover:bg-subtle"
          >
            <Retry />
            Retry
          </button>
        </div>
      )}
      {status === "stopped" && <p className="text-sm text-muted">Stopped</p>}
      {status === "done" && text && <CopyButton text={text} />}
    </div>
  );
}

function TypingIndicator() {
  return (
    <div className="flex h-6 items-center gap-1" aria-hidden="true">
      {[0, 150, 300].map((delay) => (
        <span
          key={delay}
          className="size-1.5 animate-pulse rounded-full bg-muted"
          style={{ animationDelay: `${delay}ms` }}
        />
      ))}
    </div>
  );
}

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      type="button"
      onClick={async () => {
        try {
          await navigator.clipboard.writeText(text);
          setCopied(true);
          setTimeout(() => setCopied(false), 2000);
        } catch {
          // Clipboard blocked: nothing useful to show.
        }
      }}
      aria-label={copied ? "Copied" : "Copy reply"}
      title={copied ? "Copied" : "Copy"}
      className="-ml-1.5 inline-flex size-8 items-center justify-center self-start rounded-md text-muted hover:bg-subtle hover:text-fg"
    >
      {copied ? <Check /> : <Copy />}
    </button>
  );
}
