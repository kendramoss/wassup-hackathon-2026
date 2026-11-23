"use client";

import { useEffect, useRef, useState } from "react";
import { ApiError, api, type Economics, type Product } from "@/lib/api/client";
import { dutyCountry } from "@/lib/duty-rates";
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
  // Set when the product's own HTS number is not a valid 8- or 10-digit
  // number: nothing was looked up or filled, and the user is told why (#306).
  htsNote?: string;
  failed?: boolean;
};

const fileLabels: Record<string, string> = { annual_import_value_usd: "imports a year", group_costs: "yearly cost of each group" };

// invalidHtsNote returns the API's hts_note only when it reports a malformed
// HTS (a note with no hts_source). With a suggested HTS the note is the broker
// reminder the screen already shows, so it is not repeated.
export function invalidHtsNote(r: { hts_note?: string; hts_source?: string }): string | undefined {
  return r.hts_note && !r.hts_source ? r.hts_note : undefined;
}

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
      filled: { fromFile, duty, hts: r.hts, htsSuggested: r.hts_source === "suggested", htsNote: invalidHtsNote(r) },
    };
  } catch {
    return { economics, filled: { fromFile, duty: [], failed: true } };
  }
}

// mergeDutyPrefill lands a duty-rate lookup on the economics as they are now
// (#349), not as they were when it started. Only countries that are still
// blank are filled: a rate the user entered or changed while the lookup was out
// stays, and so does one they cleared. Nothing else in `latest` is touched.
export function mergeDutyPrefill(latest: Economics, started: Economics | undefined, result: Economics | undefined): Economics {
  const now = latest.duty_rates ?? {};
  const was = started?.duty_rates ?? {};
  const got = result?.duty_rates ?? {};
  // By country identity (#718): a rate the user holds under another spelling of
  // the country is theirs, so the lookup's canonical key must not land beside it.
  const held = (m: Record<string, number>, c: string) => Object.keys(m).some((k) => m[k] !== undefined && dutyCountry(k) === dutyCountry(c));
  const add = Object.keys(got).filter((c) => !held(now, c) && !held(was, c));
  if (!add.length) return latest;
  const out: Economics = { ...latest, duty_rates: { ...now, ...Object.fromEntries(add.map((c) => [c, got[c]])) } };
  const pick = <T,>(m: Record<string, T> | undefined) => (m ? Object.fromEntries(Object.entries(m).filter(([c]) => add.includes(c))) : undefined);
  const extra = pick(result?.duty_additional);
  if (extra && Object.keys(extra).length) out.duty_additional = { ...(latest.duty_additional ?? {}), ...extra };
  for (const k of ["duty_rates_from_table", "duty_rates_live_mfn"] as const) {
    const more = (result?.[k] ?? []).filter((c) => add.includes(c));
    if (more.length) out[k] = [...new Set([...(latest[k] ?? []), ...more])];
  }
  // The note describes where the rates came from: keep it only when no rate the
  // user owns disagrees with what the lookup was describing.
  const clash = Object.keys(got).some((c) => now[c] !== undefined && now[c] !== got[c]);
  if (result?.duty_rates_note && !clash && !latest.duty_rates_note) out.duty_rates_note = result.duty_rates_note;
  return out;
}

// PrefillBar sits above a money form. Once ready, a product with an HTS
// number and no duty rates yet is priced on its own (no model call); without
// an HTS the button asks for a suggestion first (one model call).
export function PrefillBar({ product, economics, ready, onFill }: { product?: Product; economics: Economics; ready: boolean; onFill: (e: Economics) => void }) {
  const [busy, setBusy] = useState(false);
  const [filled, setFilled] = useState<Filled | null>(null);
  const ran = useRef(false);
  // The inputs as they are now, and which product/form this is: the lookup
  // lands on those, not on what they were when it started (#349).
  const now = useRef({ economics, product, onFill });
  const live = useRef(true);
  useEffect(() => {
    now.current = { economics, product, onFill };
  });
  useEffect(() => {
    live.current = true;
    return () => {
      live.current = false;
    };
  }, []);
  const canPrice = Boolean(product?.plants?.length);
  const noRates = !Object.keys(economics.duty_rates ?? {}).length;
  const run = async () => {
    if (!product || busy) return;
    ran.current = true;
    setBusy(true);
    const started = economics;
    const r = await prefillMoney(product, started);
    if (!live.current) return; // dismissed
    if (now.current.product !== product) {
      setBusy(false); // superseded: the result is for another product
      return;
    }
    if (r.economics) now.current.onFill(mergeDutyPrefill(now.current.economics, started, r.economics));
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
  if (!f.fromFile.length && !auto.length && !f.failed && !f.htsNote) {
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
        {f.htsNote && <li>{f.htsNote}</li>}
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

// mergeEstimates lands AI estimates on the economics as they are now (#349).
// A cell is filled only when it is blank now and was blank when the request
// started; a figure typed (zero included) stays, and so does a cell the user
// cleared meanwhile. Nothing else in `latest` changes.
export function mergeEstimates(
  latest: Economics,
  started: Economics,
  estimates: { group: string; plant: string; usd: number; low: number; high: number; reason: string }[],
): { economics: Economics; cells: EstimatedCells } {
  const costs = { ...(latest.group_costs ?? {}) };
  const cells: EstimatedCells = {};
  for (const x of estimates) {
    if (started.group_costs?.[x.group]?.[x.plant] !== undefined) continue;
    const row = { ...(costs[x.group] ?? {}) };
    if (row[x.plant] !== undefined) continue;
    row[x.plant] = Math.round(x.usd);
    costs[x.group] = row;
    cells[cellKey(x.group, x.plant)] = { low: x.low, high: x.high, reason: x.reason };
  }
  return { economics: Object.keys(cells).length ? { ...latest, illustrative: true, group_costs: costs } : latest, cells };
}

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
  // Busy and the message belong to the product they were for: on another
  // product they read as idle and empty (#349).
  const [busyOn, setBusyOn] = useState<Product | undefined>(undefined);
  const [note, setNote] = useState<{ for: Product | undefined; tone: "ok" | "ask"; text: string } | null>(null);
  const busy = busyOn !== undefined && busyOn === product;
  const msg = note && note.for === product ? note : null;
  // The inputs as they are now: a response merges into these, not into what the
  // request was sent with. `seq` names the latest request; it moves on when the
  // product changes or the form goes away, so older results are ignored.
  const now = useRef({ economics, product, onFill });
  const seq = useRef(0);
  useEffect(() => {
    now.current = { economics, product, onFill };
  });
  useEffect(() => {
    const token = seq;
    return () => {
      token.current++;
    };
  }, []);
  useEffect(() => {
    seq.current++;
  }, [product]);
  if (!product?.plants?.length) return null;
  const run = async () => {
    const mine = ++seq.current;
    const started = economics;
    setBusyOn(product);
    setNote(null);
    try {
      const r = await api.costEstimate({ product, economics });
      if (seq.current !== mine) return;
      const { economics: next, cells } = mergeEstimates(now.current.economics, started, r.estimates ?? []);
      const n = Object.keys(cells).length;
      if (n) now.current.onFill(next, cells);
      setNote(
        n
          ? { for: product, tone: "ok", text: `Filled ${n} blank cost${n === 1 ? "" : "s"} with AI estimates (dashed; hover for the range and why). Illustrative: check them.` }
          : { for: product, tone: "ok", text: r.note || "Every cell already has a cost." },
      );
    } catch (e) {
      if (seq.current !== mine) return;
      setNote({ for: product, tone: "ask", text: e instanceof ApiError && e.status === 400 ? e.message.replace(/^.*?:\s*/, "") : "Couldn't estimate just now. Try again, or enter the costs by hand." });
    } finally {
      if (seq.current === mine) setBusyOn(undefined);
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
