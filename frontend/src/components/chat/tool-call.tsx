import { AlertCircle, Check, ChevronRight, Spinner } from "../icons";

export type ToolPart = {
  kind: "tool";
  id?: string;
  name: string;
  args?: Record<string, unknown>;
  result?: Record<string, unknown>;
};

// One tool call: a summary line (name, arguments, status) that expands to the
// full arguments and result.
export function ToolCall({ tool, running }: { tool: ToolPart; running: boolean }) {
  const failed = tool.result !== undefined && "error" in tool.result;
  const pending = tool.result === undefined;
  const status = failed ? "failed" : pending ? (running ? "running" : "no result") : "done";
  const argSummary = tool.args
    ? Object.entries(tool.args)
        .map(([k, v]) => `${k}: ${typeof v === "string" ? v : JSON.stringify(v)}`)
        .join(", ")
    : "";

  return (
    <details className="group rounded-lg border border-border text-sm">
      <summary className="flex cursor-pointer list-none items-center gap-2 rounded-lg px-3 py-2 text-muted hover:bg-subtle [&::-webkit-details-marker]:hidden">
        <ChevronRight className="shrink-0 transition-transform group-open:rotate-90" />
        <span className="font-mono text-fg">{tool.name}</span>
        <span className="min-w-0 flex-1 truncate font-mono text-xs">{argSummary}</span>
        <span className="flex shrink-0 items-center gap-1 text-xs">
          {status === "running" && <Spinner />}
          {status === "done" && <Check className="text-success" />}
          {status === "failed" && <AlertCircle className="text-danger" />}
          <span className={status === "done" ? "sr-only" : ""}>{status}</span>
        </span>
      </summary>
      <div className="grid gap-3 border-t border-border px-3 py-3">
        <JsonBlock label="Arguments" value={tool.args ?? {}} />
        {!pending && <JsonBlock label="Result" value={tool.result} />}
      </div>
    </details>
  );
}

function JsonBlock({ label, value }: { label: string; value: unknown }) {
  return (
    <div>
      <div className="mb-1 text-xs font-medium text-muted">{label}</div>
      <pre className="overflow-x-auto rounded-md bg-subtle px-3 py-2 font-mono text-xs leading-relaxed">
        {JSON.stringify(value, null, 2)}
      </pre>
    </div>
  );
}
