"use client";

import { useEffect, useRef, useState } from "react";
import { ApiError, api, type Economics, type Product } from "@/lib/api/client";
import { cx } from "./ui";

// Money inputs filled for the user (#235): figures from the uploaded BOM,
// then duty rates from the product's HTS number (or a suggested one) via
// POST /v1/products/money-prefill. Every value stays editable; this only
// says where each one came from.

export type Filled = {
  fromFile: string[]; // "imports a year", "group costs"
  duty: { country: string; rate: number; source: "table" | "live" | "given" }[];
  hts?: string;
  htsSuggested?: boolean;
  failed?: boolean;
};

const fileLabels: Record<string, string> = { annual_import_value_usd: "imports a year", group_costs: "yearly cost of each group" };

// prefillMoney fills the duty rates the economics lack. The user's own rates
// win; a failed call leaves the economics as they were.
export async function prefillMoney(
  product: Product,
  economics: Economics | undefined,
  fileSources?: Record<string, string>,
): Promise<{ economics: Economics | undefined; filled: Filled }> {
  const fromFile = Object.keys(fileSources ?? {})
    .map((k) => fileLabels[k])
    .filter(Boolean);
  try {
    const r = await api.moneyPrefill({ product, economics: economics ?? { illustrative: true } });
    const duty = Object.entries(r.economics.duty_rates ?? {}).map(([country, rate]) => ({
      country,
      rate,
      source: (r.duty_sources?.[country] ?? "given") as Filled["duty"][number]["source"],
    }));
    const changed = duty.some((d) => d.source !== "given");
    return {
      economics: changed || economics ? { ...(economics ?? {}), ...r.economics } : economics,
      filled: { fromFile, duty, hts: r.hts, htsSuggested: r.hts_source === "suggested" },
    };
  } catch {
    return { economics, filled: { fromFile, duty: [], failed: true } };
  }
}

// PrefillBar sits above a money form. Once ready, a product with an HTS
// number and no duty rates yet is priced on its own (no model call); without
// an HTS the button asks for a suggestion first (one model call).
export function PrefillBar({ product, economics, ready, onFill }: { product?: Product; economics: Economics; ready: boolean; onFill: (e: Economics) => void }) {
  const [busy, setBusy] = useState(false);
  const [filled, setFilled] = useState<Filled | null>(null);
  const ran = useRef(false);
  const canPrice = Boolean(product?.plants?.length);
  const noRates = !Object.keys(economics.duty_rates ?? {}).length;
  const run = async () => {
    if (!product || busy) return;
    ran.current = true;
    setBusy(true);
    const r = await prefillMoney(product, economics);
    if (r.economics) onFill(r.economics);
    setFilled(r.filled);
    setBusy(false);
  };
  useEffect(() => {
    if (ready && !ran.current && canPrice && product?.hts && noRates) void run();
    // Runs once, when the money inputs have loaded.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready]);
  if (!canPrice) return null;
  return (
    <div className="mb-4">
      {filled ? (
        <PrefillNote f={filled} />
      ) : (
        <button
          type="button"
          onClick={run}
          disabled={busy}
          className="inline-flex min-h-8 items-center gap-1.5 border border-ok bg-white px-2.5 text-[12px] text-ok shadow-[2px_2px_0_var(--shadow)] hover:bg-ok-wash disabled:cursor-wait disabled:text-dim"
        >
          {busy ? "Filling…" : product?.hts ? "Fill duty rates for me" : "Find the HTS number and fill duty rates"}
        </button>
      )}
    </div>
  );
}

const pct = (r: number) => `${Math.round(r * 1000) / 10}%`;

export function PrefillNote({ f, onEdit, className }: { f: Filled; onEdit?: () => void; className?: string }) {
  const auto = f.duty.filter((d) => d.source !== "given");
  if (!f.fromFile.length && !auto.length && !f.failed) {
    return (
      <p className={cx("text-[12px] text-dim", className)}>
        No HTS number to price duty from yet. Add one (or use &ldquo;Suggest&rdquo;) and duty rates fill in.
      </p>
    );
  }
  return (
    <div role="status" className={cx("border border-ok bg-ok-wash px-4 py-3 text-[12.5px] leading-relaxed", className)}>
      <p className="font-medium text-ink">Money inputs filled for you · check and edit</p>
      <ul className="mt-1 text-body">
        {f.fromFile.length > 0 && <li>From your file: {f.fromFile.join(", ")}.</li>}
        {auto.length > 0 && (
          <li>
            Duty, {auto.some((d) => d.source === "live") ? "live U.S. tariff rate (MFN + Section 301)" : "sourced duty table"} for HTS {f.hts}:{" "}
            {auto.map((d) => `${d.country} ${pct(d.rate)}`).join(", ")}.
          </li>
        )}
        {f.htsSuggested && f.hts && <li>HTS {f.hts} was suggested from similar CBP rulings: confirm the classification with a broker.</li>}
        {f.failed && <li>Duty rates couldn&apos;t be looked up just now; enter them by hand.</li>}
      </ul>
      {onEdit && (
        <button type="button" onClick={onEdit} className="mt-1.5 min-h-7 text-[12px] text-ok hover:underline">
          Review or edit the figures
        </button>
      )}
    </div>
  );
}

// AI estimates for the blank group costs (#235 step 4): one call; only blank
// cells are filled, the user's own figures are anchors and never change.
export type EstimatedCells = Record<string, { low: number; high: number; reason: string }>;
export const cellKey = (group: string, plant: string) => `${group}|${plant}`;

export function EstimateCosts({
  product,
  economics,
  onFill,
  className,
}: {
  product?: Product;
  economics: Economics;
  onFill: (e: Economics, cells: EstimatedCells) => void;
  className?: string;
}) {
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ tone: "ok" | "ask"; text: string } | null>(null);
  if (!product?.plants?.length) return null;
  const run = async () => {
    setBusy(true);
    setMsg(null);
    try {
      const r = await api.costEstimate({ product, economics });
      const costs = { ...(economics.group_costs ?? {}) };
      const cells: EstimatedCells = {};
      for (const x of r.estimates ?? []) {
        const row = { ...(costs[x.group] ?? {}) };
        if (row[x.plant] !== undefined) continue; // the user's figure stays
        row[x.plant] = Math.round(x.usd);
        costs[x.group] = row;
        cells[cellKey(x.group, x.plant)] = { low: x.low, high: x.high, reason: x.reason };
      }
      const n = Object.keys(cells).length;
      onFill({ ...economics, illustrative: true, group_costs: costs }, cells);
      setMsg(
        n
          ? { tone: "ok", text: `Filled ${n} blank cost${n === 1 ? "" : "s"} with AI estimates (dashed; hover for the range and why). Illustrative: check them.` }
          : { tone: "ok", text: r.note || "Every cell already has a cost." },
      );
    } catch (e) {
      setMsg({ tone: "ask", text: e instanceof ApiError && e.status === 400 ? e.message.replace(/^.*?:\s*/, "") : "Couldn't estimate just now. Try again, or enter the costs by hand." });
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className={cx("flex flex-col gap-1.5", className)}>
      <button
        type="button"
        onClick={run}
        disabled={busy}
        className="inline-flex min-h-8 w-fit items-center gap-1.5 border border-ok bg-white px-2.5 text-[12px] text-ok shadow-[2px_2px_0_var(--shadow)] hover:bg-ok-wash disabled:cursor-wait disabled:text-dim"
      >
        {busy ? "Estimating…" : "Estimate the blank costs with AI"}
      </button>
      {msg && (
        <p role="status" className={cx("duo text-[12px]", msg.tone === "ok" ? "text-body" : "text-ask")}>
          {msg.text}
        </p>
      )}
    </div>
  );
}
