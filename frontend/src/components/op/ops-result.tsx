"use client";

import type { Plan } from "@/lib/api/client";
import type { components } from "@/lib/api/schema";
import { copy } from "@/lib/copy";
import { money } from "@/lib/format";
import { VIEWER_REASON } from "@/lib/roles";
import { Button, cx } from "./ui";
import { NOT_MODELLED, UNKNOWN, checkText, componentRows, feasibilityText, figureText, netText, notModelledLine, opsNet, unknownLine, type PlanOperations } from "./ops-view";
import { groupName } from "./plan-bits";

type Objective = components["schemas"]["PlanObjective"];

const fig = (v: number | null | undefined) => (v === null || v === undefined ? null : money(v));

// Beside every operations-mode money figure, priced or not: what is left out.
// Read from the result, never a fixed list, and shown whenever it is not empty.
export function OpsScope({ ops, className }: { ops: PlanOperations; className?: string }) {
  const nm = notModelledLine(ops);
  return (
    <div className={cx("text-xs text-dim", className)}>
      <p>{copy.opsResult.scopeLine}</p>
      {ops.horizon_years !== undefined && <p>{copy.opsResult.horizon(ops.horizon_years)}</p>}
      {nm && <p>{nm}</p>}
    </div>
  );
}

// Why a figure is unknown, in the user's terms (from operations.unknown).
export function OpsUnknown({ ops, className }: { ops: PlanOperations; className?: string }) {
  if (!ops.unknown.length) return null;
  return (
    <div className={cx("text-xs text-ask", className)}>
      <p className="font-medium">{copy.opsResult.unknownTitle}</p>
      <ul className="list-disc pl-4">
        {ops.unknown.map((u, i) => (
          <li key={i}>{unknownLine(u)}</li>
        ))}
      </ul>
    </div>
  );
}

function Row({ k, v, strong }: { k: string; v: string; strong?: boolean }) {
  return (
    <tr className="border-t border-line">
      <th scope="row" className="py-1 pr-3 text-left font-normal text-dim">
        {k}
      </th>
      <td className={cx("py-1 text-right tabular-nums", strong && "font-medium")}>{v}</td>
    </tr>
  );
}

// The full breakdown of one operations result: absolute totals today and with
// the move, the components, feasibility and each check, assumptions and what
// was ignored. null reads "Unknown" or "Not modelled", never $0.
export function OpsDetail({ ops, objective, className }: { ops: PlanOperations; objective?: Objective; className?: string }) {
  const R = copy.opsResult;
  const pick = (v: number | null | undefined, fallback: string) => fig(v) ?? fallback;
  const baseOp = ops.baseline.operating_usd;
  const propOp = ops.proposed.operating_usd;
  const missing = ops.unknown.some((u) => u.category === "financial") ? UNKNOWN : NOT_MODELLED;
  return (
    <details className={cx("group text-xs text-dim", className)}>
      <summary className="min-h-6 cursor-pointer list-none py-0.5 [&::-webkit-details-marker]:hidden">
        <span aria-hidden="true" className="mr-1.5 inline-block w-2.5 text-ok transition-transform group-open:rotate-90">
          ▸
        </span>
        <span className="text-ok hover:underline">{R.detail}</span>
      </summary>
      <div className="mt-2 flex flex-col gap-3 border-l border-line pl-3 text-[12.5px] text-body">
        {objective && <p className="read">{objective.statement}</p>}
        <table className="w-full max-w-[420px] border-collapse">
          <thead>
            <tr>
              <th scope="col" className="sr-only">
                Figure
              </th>
              <th scope="col" className="pb-1 text-right text-[11px] font-medium uppercase tracking-[0.06em] text-dim">
                {R.baseline} → {R.proposed}
              </th>
            </tr>
          </thead>
          <tbody>
            <Row k={R.duty} v={`${pick(ops.baseline.duty_usd, missing)} → ${pick(ops.proposed.duty_usd, missing)}`} />
            <Row k={R.operating} v={`${pick(baseOp, missing)} → ${pick(propOp, missing)}`} />
            {ops.proposed.amortized_one_time_usd !== undefined && <Row k={R.amortized} v={pick(ops.proposed.amortized_one_time_usd, missing)} />}
            <Row k={R.total} v={`${pick(ops.baseline.total_usd, missing)} → ${pick(ops.proposed.total_usd, missing)}`} strong />
          </tbody>
        </table>
        <table className="w-full max-w-[420px] border-collapse">
          <caption className="pb-1 text-left text-[11px] font-medium uppercase tracking-[0.06em] text-dim">{R.net}</caption>
          <tbody>
            {componentRows(ops).map((r) => (
              <Row key={r.key} k={r.label} v={r.text} />
            ))}
          </tbody>
        </table>
        <div>
          <p className="font-medium text-ink">{R.feasibility}</p>
          <p>{feasibilityText(ops)}</p>
          {ops.checks.length > 0 && (
            <ul className="list-disc pl-4">
              {ops.checks.map((c, i) => (
                <li key={i}>{checkText(c)}</li>
              ))}
            </ul>
          )}
        </div>
        <OpsUnknown ops={ops} />
        {ops.sunk_ignored.length > 0 && (
          <div>
            <p className="font-medium text-ink">{R.sunk}</p>
            <ul className="list-disc pl-4">
              {ops.sunk_ignored.map((s, i) => (
                <li key={i}>
                  {groupName(s.group)} · {s.plant} · {s.factor}
                </li>
              ))}
            </ul>
          </div>
        )}
        {ops.stale_refs.length > 0 && (
          <div>
            <p className="font-medium text-ink">{R.stale}</p>
            <ul className="list-disc pl-4">
              {ops.stale_refs.map((s, i) => (
                <li key={i}>
                  {groupName(s.group)} · {s.plant}
                </li>
              ))}
            </ul>
          </div>
        )}
        {ops.assumptions.length > 0 && (
          <div>
            <p className="font-medium text-ink">{R.assumptions}</p>
            <ul className="list-disc pl-4">
              {ops.assumptions.map((a, i) => (
                <li key={i}>{a}</li>
              ))}
            </ul>
          </div>
        )}
        <OpsScope ops={ops} />
      </div>
    </details>
  );
}

// The money of a stored result is out of date: say so, offer the re-run. The
// inputs stay editable and the legal facts and the draft stay as they were.
export function StaleMoneyBanner({ kind, onRun, readOnly, busy, className }: { kind: "plan" | "targets"; onRun?: () => void; readOnly?: boolean; busy?: boolean; className?: string }) {
  const S = copy.stale;
  return (
    <div role="status" className={cx("flex flex-wrap items-center gap-3 border border-ask-line bg-ask-wash px-4 py-3 text-[13px]", className)}>
      <div className="min-w-0 flex-1">
        <p className="font-medium text-ink">{S.title}</p>
        <p className="duo mt-0.5 max-w-[80ch] text-xs text-body">{kind === "plan" ? S.plan : S.targets}</p>
      </div>
      {onRun && (
        <Button onClick={onRun} disabled={readOnly || busy} title={readOnly ? VIEWER_REASON : undefined}>
          {kind === "plan" ? S.run : S.compare}
        </Button>
      )}
    </div>
  );
}

// The big "Net per year" block of a result band (plan and country comparison).
// Legacy plans keep the markup they always had; an operations plan reads its
// own nullable figures with what is left out beside them; a stale plan shows
// no amounts at all.
export function BandNet({ plan, below, stale, objective }: { plan: Plan; below?: boolean; stale?: boolean; objective?: Objective }) {
  const ops = plan.operations;
  if (stale) {
    return (
      <>
        <p className="eyebrow">Net per year · illustrative</p>
        <p className="mt-1 text-[22px] leading-tight text-dim">{copy.stale.hidden}</p>
        <p className="mt-2 text-xs text-dim">{copy.stale.title}</p>
      </>
    );
  }
  if (ops) {
    const net = opsNet(plan);
    return (
      <>
        <p className="eyebrow">Net per year · illustrative</p>
        <p className={cx("mt-1 leading-none tracking-[-0.05em]", net !== null && net !== 0 ? "text-[44px]" : "text-[26px]", below && "text-dim")}>{netText(plan)}</p>
        <p className="mt-2 text-xs text-dim">
          {below && (
            <>
              if a broker confirms it
              <br />
            </>
          )}
          {figureText(ops.components.duty_effect_per_year, ops, ["duty"])} duty effect · {figureText(ops.components.operating_delta_per_year, ops, ["operating", "group_cost", "labour", "logistics"])} operating change ·{" "}
          {figureText(ops.components.amortized_one_time_per_year, ops, ["amortized_one_time", "one_time", "capex", "retooling", "horizon"])} one-time a year
        </p>
        <OpsScope ops={ops} className="mt-2" />
        <OpsUnknown ops={ops} className="mt-2" />
        <OpsDetail ops={ops} objective={objective} className="mt-2" />
      </>
    );
  }
  return (
    <>
      <p className="eyebrow">Net per year · illustrative</p>
      <p className={cx("mt-1 text-[44px] leading-none tracking-[-0.05em]", below && "text-dim")}>{money(plan.net_per_year)}</p>
      <p className="mt-2 text-xs text-dim">
        {below && (
          <>
            if a broker confirms it
            <br />
          </>
        )}
        {money(plan.duty_effect_per_year)} duty saved − {money(plan.added_cost_per_year)} added cost
      </p>
    </>
  );
}
