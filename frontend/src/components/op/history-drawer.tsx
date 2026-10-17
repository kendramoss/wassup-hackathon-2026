"use client";

import type { CaseResponse } from "@/lib/api/client";
import { Drawer } from "./ui";

const kinds: Record<string, string> = {
  predicted: "Predicted",
  answered: "Answered questions",
  plan_searched: "Plan search",
  scenarios: "Scenarios",
  targets_compared: "Compared countries",
  watched: "Ruling watch",
  asked: "Asked",
  exported: "Draft for broker",
  economics: "Money inputs",
  pinned: "Pinned",
  unpinned: "Unpinned",
  renamed: "Renamed",
  bom_updated: "New BOM version",
};

function when(iso?: string) {
  if (!iso) return "";
  return new Date(iso).toLocaleString(undefined, { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" });
}

// Case history (D41, D45; p-history.html): one row per event with time, kind,
// summary as returned, model, cost · time and tokens; the case total last.
export function HistoryDrawer({ c, onClose }: { c: CaseResponse; onClose: () => void }) {
  // "Pinned" is an admin action on demo cases, not a step a visitor took.
  const events = (c.history ?? []).filter((e) => e.kind !== "pinned");
  const cost = events.reduce((s, e) => s + (e.cost_usd ?? 0), 0);
  const ms = events.reduce((s, e) => s + (e.latency_ms ?? 0), 0);
  return (
    <Drawer
      label="Case history"
      onClose={onClose}
      width={520}
      header={
        <>
          <b className="font-medium text-ink">◆ Case history</b>
          <span>{events.length} steps, oldest first</span>
        </>
      }
      footer={
        <p className="border-t border-dashed border-guide px-5 py-3 font-mono text-xs text-dim">
          Case total · ${cost.toFixed(3)} · {(ms / 1000).toFixed(1)} s of model time
        </p>
      }
    >
      <ol className="px-5 py-2">
        {events.map((e, i) => (
          <li key={i} className="grid grid-cols-[92px_minmax(0,1fr)] gap-3 border-b border-line py-3 text-[13px]">
            <span className="font-mono text-xs text-dim">{when(e.at)}</span>
            <div className="min-w-0">
              <p className="font-medium">
                {kinds[e.kind ?? ""] ?? e.kind}
                {/* Who did it, when signed in (D68). */}
                {e.actor && <span className="font-normal text-dim"> by {e.actor}</span>}
              </p>
              {e.summary && <p className="duo mt-0.5 text-body">{e.summary}</p>}
              <p className="mt-1 font-mono text-[11px] text-dim">
                {e.model_id ? `${e.model_id}${e.prompt_version ? ` · ${e.prompt_version}` : ""}` : "no model"}
                {e.cost_usd !== undefined ? ` · $${e.cost_usd.toFixed(3)}` : ""}
                {e.latency_ms !== undefined ? ` · ${(e.latency_ms / 1000).toFixed(1)} s` : ""}
                {e.input_tokens !== undefined ? ` · ${e.input_tokens + (e.output_tokens ?? 0)} tokens` : ""}
              </p>
            </div>
          </li>
        ))}
      </ol>
    </Drawer>
  );
}
