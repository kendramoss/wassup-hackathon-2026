"use client";

import { useRouter } from "next/navigation";
import { useCallback, useEffect, useRef, useState } from "react";
import { streamChat, type Approval, type StreamEvent } from "@/lib/chat-stream";
import { site } from "@/lib/site";
import { AlertCircle, ArrowUp, Check, Copy, Plus, Retry, Stop, X } from "../icons";
import { SiteHeader } from "../site-header";
import { Markdown } from "./markdown";
import { ToolCall, type ToolPart } from "./tool-call";

const MAX_CHARS = 2000; // backend limit (maxMessageRunes)

type TextPart = { kind: "text"; text: string };
// A change the agent wants to make (deploy, rollback), waiting for a person.
type ApprovalPart = {
  kind: "approval";
  id: string;
  name?: string;
  args?: Record<string, unknown>;
  summary?: string;
  decision?: "approved" | "rejected";
  reason?: string;
};
type Part = TextPart | ToolPart | ApprovalPart;
type StreamBody = Parameters<typeof streamChat>[0];
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
    case "approval_request":
      return [...parts, { kind: "approval", id: e.id, name: e.name, args: e.args, summary: e.text }];
    case "tool_result": {
      const i = parts.findIndex(
        (p) => p.kind === "tool" && p.result === undefined && (e.id ? p.id === e.id : p.name === e.name),
      );
      // No matching call in this reply: a tool that ran after its approval
      // card was answered. Show it with its result.
      if (i === -1) return [...parts, { kind: "tool", id: e.id, name: e.name, result: e.result ?? {} }];
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

// Cache Components keeps pages alive (hidden) after you navigate away, so
// /chat would come back with the last conversation and a landing-page prompt
// (?q=) wouldn't be sent. Keying on bfcacheId starts a fresh chat on every
// link or push to /chat, and still restores it on browser back/forward.
export function FreshChat() {
  const { bfcacheId } = useRouter();
  return <Chat key={bfcacheId} />;
}

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

  // Streams one agent turn into a new assistant message. userText, when set,
  // is shown as the user's message first (approvals don't add one).
  const run = useCallback(
    async (body: StreamBody, userText?: string) => {
      const assistantId = newId();
      setMessages((ms) => [
        ...ms,
        ...(userText ? [{ id: newId(), role: "user" as const, text: userText }] : []),
        { id: assistantId, role: "assistant", parts: [], status: "streaming" },
      ]);
      setBusy(true);
      setAnnouncement("Agent is replying");
      stickToBottom.current = true;

      const controller = new AbortController();
      abortRef.current = controller;
      let finalParts: Part[] = [];
      try {
        await streamChat(
          body,
          (e) => {
            if (e.type === "session") setSessionId(e.session_id);
            else if (e.type === "error") throw new Error(e.error);
            else {
              if (e.type === "approval_request") setAnnouncement(`Approval needed: ${e.text ?? e.name}`);
              finalParts = applyEvent(finalParts, e);
              const parts = finalParts;
              updateAssistant(assistantId, (m) => ({ ...m, parts }));
            }
          },
          controller.signal,
        );
        updateAssistant(assistantId, (m) => ({ ...m, status: "done" }));
        if (!finalParts.some((p) => p.kind === "approval" && !p.decision)) {
          setAnnouncement(replyText(finalParts) || "Agent replied");
        }
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
    [updateAssistant],
  );

  const send = useCallback(
    (text: string) => {
      const message = text.trim();
      if (!message || busy) return;
      setInput("");
      run({ message, session_id: sessionId }, message);
    },
    [busy, run, sessionId],
  );

  // Answers an approval card: marks it decided and lets the agent continue.
  const decide = (approval: Approval) => {
    if (!sessionId || busy) return;
    const decision = approval.approved ? ("approved" as const) : ("rejected" as const);
    setMessages((ms) =>
      ms.map((m) =>
        m.role === "assistant"
          ? {
              ...m,
              parts: m.parts.map((p) =>
                p.kind === "approval" && p.id === approval.id ? { ...p, decision, reason: approval.reason } : p,
              ),
            }
          : m,
      ),
    );
    run({ approval, session_id: sessionId });
  };

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
                    <AssistantMessage message={m} onRetry={() => retry(m.id)} onDecide={decide} busy={busy} />
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
  onDecide,
  busy,
}: {
  message: Extract<Message, { role: "assistant" }>;
  onRetry: () => void;
  onDecide: (a: Approval) => void;
  busy: boolean;
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
        ) : p.kind === "approval" ? (
          <ApprovalCard key={p.id} part={p} disabled={busy} onDecide={onDecide} />
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

function ApprovalCard({
  part,
  disabled,
  onDecide,
}: {
  part: ApprovalPart;
  disabled: boolean;
  onDecide: (a: Approval) => void;
}) {
  const [rejecting, setRejecting] = useState(false);
  const [reason, setReason] = useState("");
  const reasonId = `reason-${part.id}`;
  const details = Object.entries(part.args ?? {});

  return (
    <section
      aria-label={part.decision ? `Change ${part.decision}` : "Approval needed"}
      className={`rounded-lg border p-4 text-sm ${part.decision ? "border-border" : "border-accent"}`}
    >
      <p className="font-medium">{part.summary || `Run ${part.name}`}</p>
      {details.length > 0 && (
        <dl className="mt-2 grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 font-mono text-xs text-muted">
          {details.map(([k, v]) => (
            <div key={k} className="contents">
              <dt>{k}</dt>
              <dd className="min-w-0 break-all">{typeof v === "string" ? v : JSON.stringify(v)}</dd>
            </div>
          ))}
        </dl>
      )}

      {part.decision ? (
        <p className="mt-3 flex items-center gap-1.5 text-muted">
          {part.decision === "approved" ? <Check className="text-success" /> : <X />}
          {part.decision === "approved" ? "Approved" : "Rejected"}
          {part.reason ? `: ${part.reason}` : ""}
        </p>
      ) : rejecting ? (
        <form
          className="mt-3 flex flex-wrap items-end gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            onDecide({ id: part.id, approved: false, reason: reason.trim() || undefined });
          }}
        >
          <div className="flex min-w-48 flex-1 flex-col gap-1">
            <label htmlFor={reasonId} className="text-xs text-muted">
              Reason (optional, the agent sees it)
            </label>
            <input
              id={reasonId}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              maxLength={500}
              autoComplete="off"
              className="h-9 rounded-md border border-border bg-bg px-2 focus-visible:border-accent focus-visible:outline-none"
            />
          </div>
          <button
            type="submit"
            disabled={disabled}
            className="h-9 rounded-md border border-danger/60 px-3 font-medium text-danger hover:bg-subtle disabled:opacity-40"
          >
            Reject change
          </button>
          <button
            type="button"
            onClick={() => setRejecting(false)}
            className="h-9 rounded-md px-3 text-muted hover:bg-subtle hover:text-fg"
          >
            Cancel
          </button>
        </form>
      ) : (
        <div className="mt-3 flex flex-wrap gap-2">
          <button
            type="button"
            disabled={disabled}
            onClick={() => onDecide({ id: part.id, approved: true })}
            className="h-9 rounded-md bg-accent px-4 font-medium text-accent-fg disabled:opacity-40"
          >
            Approve
          </button>
          <button
            type="button"
            disabled={disabled}
            onClick={() => setRejecting(true)}
            className="h-9 rounded-md border border-border px-4 hover:bg-subtle disabled:opacity-40"
          >
            Reject
          </button>
        </div>
      )}
    </section>
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
