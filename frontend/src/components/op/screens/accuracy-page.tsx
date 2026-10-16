"use client";

import { useEffect, useState } from "react";
import { api, type EvalSummary } from "@/lib/api/client";
import { ErrorBox, Main } from "../chrome";
import { Chip, Panel, ScrollX, Spinner, cx } from "../ui";

const pctOf = (v?: number) => (v === undefined ? "—" : `${(v * 100).toFixed(1)}%`);

// Items are ratios unless their key says otherwise: *_usd is dollars,
// *_s is seconds.
function valueOf(key: string | undefined, v?: number) {
  if (v === undefined) return "—";
  if (key?.endsWith("_usd")) return `$${v < 1 ? v.toFixed(4).replace(/0+$/, "") : v.toFixed(2)}`;
  if (key?.endsWith("_s")) return `${v.toFixed(v < 10 ? 1 : 0)} s`;
  return pctOf(v);
}

// "How accurate is it?": GET /v1/eval/summary. Every number shows its split,
// n and CI; proxy numbers are labelled until the Vertex re-run.
export function AccuracyPage() {
  const [data, setData] = useState<EvalSummary | null>(null);
  const [error, setError] = useState<unknown>(null);
  const load = () => {
    setError(null);
    api.evalSummary().then(setData, setError);
  };
  useEffect(() => {
    api.evalSummary().then(setData, setError);
  }, []);
  const items = data?.items ?? [];
  const [lead, ...rest] = items;
  return (
    <Main>
      <p className="eyebrow">How accurate is it?</p>
      <h1 className="mt-1 text-[28px] tracking-[-0.05em]">Measured on CBP rulings the model never saw</h1>
      <p className="duo mt-2 max-w-[80ch] text-sm text-dim">
        Each case is predicted using only earlier rulings dated before it, then compared with what CBP actually held. The held-out split was scored once and never used for tuning.
      </p>
      {data?.status && (
        <p className="mt-3">
          <Chip className="whitespace-normal" tone={/^official/i.test(data.status) ? "ok" : "ask"}>
            {data.status}
          </Chip>
        </p>
      )}
      {error ? (
        <div className="mt-6">
          <ErrorBox error={error} onRetry={load} />
        </div>
      ) : null}
      {!data && !error && (
        <div className="mt-6">
          <Spinner label="Loading the numbers…" />
        </div>
      )}
      {lead && (
        <Panel className="mt-8">
          <p className="eyebrow">{lead.label}</p>
          <p className="mt-1 text-[64px] leading-none tracking-[-0.06em]">{pctOf(lead.value)}</p>
          <p className="mt-2 text-xs text-dim">
            {lead.ci_low !== undefined && `95% CI ${pctOf(lead.ci_low)}–${pctOf(lead.ci_high)} · `}n = {lead.n} · {lead.split}
            {lead.development_only ? " · proxy" : ""}
          </p>
        </Panel>
      )}
      {rest.length > 0 && (
        <div className="mt-8">
          <ScrollX label="All measured numbers">
          <table className="w-full min-w-[640px] border-collapse border border-edge bg-sheet text-[13px] shadow-[4px_4px_0_var(--shadow)]">
            <thead>
              <tr>
                {["Measure", "Value", "95% CI", "n", "Split", "Source"].map((h) => (
                  <th key={h} scope="col" className="border-b border-line bg-paper px-3.5 py-2.5 text-left text-[10.5px] font-medium uppercase tracking-[0.08em] text-dim">
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rest.map((it) => (
                <tr key={it.key} className={cx("border-b border-line last:border-0", it.key?.startsWith("baseline") && "text-dim")}>
                  <td className="duo px-3.5 py-2.5">
                    {it.label}
                    {it.development_only && <span className="ml-2 text-[11px] text-ask">proxy</span>}
                  </td>
                  <td className="whitespace-nowrap px-3.5 py-2.5 text-[15px]">{valueOf(it.key, it.value)}</td>
                  <td className="whitespace-nowrap px-3.5 py-2.5 text-dim">{it.ci_low !== undefined ? `${pctOf(it.ci_low)}–${pctOf(it.ci_high)}` : "—"}</td>
                  <td className="px-3.5 py-2.5">{it.n}</td>
                  <td className="px-3.5 py-2.5">{it.split}</td>
                  <td className="max-w-[28ch] truncate px-3.5 py-2.5 text-xs text-dim" title={it.source}>
                    {it.source}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          </ScrollX>
        </div>
      )}
      {data?.generated_at && <p className="mt-4 text-xs text-dim">Generated {data.generated_at}</p>}
    </Main>
  );
}
