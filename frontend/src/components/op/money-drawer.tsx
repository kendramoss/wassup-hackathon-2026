"use client";

import { useId, useState } from "react";
import type { Economics, Product } from "@/lib/api/client";
import { money } from "@/lib/format";
import { groupName } from "./plan-bits";
import { Button, Country, Drawer, cx } from "./ui";

// Money inputs (DESIGN.md step 1, locked 16 Oct, option C of c-money.html;
// frames p-money-quick, p-money-each): imports a year, a duty rate per
// country, then cost per group a year, either as a quick estimate or group
// by group. Everything is illustrative and editable.

// Rough share of a product's cost per group, for the quick estimate only
// (heat-gun proportions; unknown groups weigh like an average one).
const weight: Record<string, number> = {
  motor_fan_assembly: 300,
  pcba: 150,
  heating_element: 75,
  housing_moulding: 75,
  switch_cord_minor_parts: 70,
};

const num = (s: string) => {
  const n = Number(s.replace(/[$,%\s]/g, ""));
  return s.trim() === "" || Number.isNaN(n) ? undefined : n;
};
const usd = (n: number | undefined) => (n === undefined ? "" : `$${Math.round(n).toLocaleString("en-US")}`);

export function productGroups(product?: Product): string[] {
  return [...new Set([...(product?.components ?? []), ...(product?.operations ?? [])].map((x) => x.group).filter((g): g is string => Boolean(g)))].filter(
    (g) => g !== "final_assembly",
  );
}

export function hasMoney(e: Economics | undefined): boolean {
  return Boolean(e?.annual_import_value_usd && Object.keys(e.duty_rates ?? {}).length && Object.keys(e.group_costs ?? {}).length);
}

// Something to edit, even when a duty rate or a cost is still blank. The
// money tier stays incomplete until hasMoney; this only decides whether the
// saved numbers go with the case.
export function hasAnyMoney(e: Economics | undefined): boolean {
  return Boolean(e?.annual_import_value_usd || Object.keys(e?.duty_rates ?? {}).length || Object.keys(e?.group_costs ?? {}).length);
}

// "$2.0M imports a year · China 41.7%, Thailand 16.7% · 5 groups costed"
export function moneySummary(e: Economics | undefined): string {
  if (!e) return "";
  const parts = [];
  if (e.annual_import_value_usd) parts.push(`${money(e.annual_import_value_usd)} imports a year`);
  const rates = Object.entries(e.duty_rates ?? {});
  if (rates.length) parts.push(rates.map(([c, r]) => `${c} ${Math.round(r * 1000) / 10}%`).join(", "));
  const n = Object.keys(e.group_costs ?? {}).length;
  if (n) parts.push(`${n} group${n === 1 ? "" : "s"} costed`);
  return parts.join(" · ");
}

export function MoneyDrawer({
  value,
  product,
  countries,
  onSave,
  onClose,
}: {
  value: Economics | undefined;
  product?: Product;
  countries: string[];
  onSave: (e: Economics) => void;
  onClose: () => void;
}) {
  const id = useId();
  const plants = product?.plants ?? [];
  const groups = productGroups(product);
  // The plant most parts come from is the base; the others cost more.
  const counts = new Map<string, number>();
  for (const c of product?.components ?? []) if (c.plant) counts.set(c.plant, (counts.get(c.plant) ?? 0) + 1);
  const base = [...plants].sort((a, b) => (counts.get(b.id) ?? 0) - (counts.get(a.id) ?? 0))[0];
  const others = plants.filter((p) => p.id !== base?.id);

  const [e, setE] = useState<Economics>(() => ({ illustrative: true, ...(value ?? {}) }));
  const [mode, setMode] = useState<"quick" | "each">(Object.keys(value?.group_costs ?? {}).length ? "each" : "quick");
  const start = base ? groups.reduce((s, g) => s + (value?.group_costs?.[g]?.[base.id] ?? 0), 0) : 0;
  const [total, setTotal] = useState<number | undefined>(start || undefined);
  const [more, setMore] = useState(40);

  const w = (g: string) => weight[g] ?? 100;
  const sumW = groups.reduce((s, g) => s + w(g), 0) || 1;
  const quick = (t: number | undefined, pctMore: number): Economics["group_costs"] => {
    if (!t || !base) return {};
    return Object.fromEntries(
      groups.map((g) => {
        const share = (t * w(g)) / sumW;
        return [g, { [base.id]: Math.round(share), ...Object.fromEntries(others.map((p) => [p.id, Math.round(share * (1 + pctMore / 100))])) }];
      }),
    );
  };
  const costs = mode === "quick" ? quick(total, more) : (e.group_costs ?? {});
  const rates = e.duty_rates ?? {};
  const field = "h-10 w-36 rounded-[2px] border border-edge bg-white px-3 text-right text-sm tabular-nums focus:border-ok";

  const save = () => {
    const out: Economics = { ...e, group_costs: costs };
    if (!Object.keys(out.group_costs ?? {}).length) delete out.group_costs;
    onSave(out);
  };

  return (
    <Drawer
      label="Money inputs"
      width={600}
      onClose={onClose}
      header={
        <>
          <b className="font-medium text-ink">Money inputs</b>
          <span className="ml-3 border border-ask-line px-1.5 py-0.5 text-[10.5px] text-ask">illustrative — editable</span>
        </>
      }
      footer={
        <div className="flex items-center gap-3 border-t border-line px-6 py-3.5">
          <p className="duo mr-auto min-w-0 truncate whitespace-nowrap text-xs text-dim">A rough number is enough to rank moves.</p>
          <Button variant="ghost" className="shrink-0 whitespace-nowrap" onClick={onClose}>
            Cancel
          </Button>
          <Button className="shrink-0 whitespace-nowrap" onClick={save}>
            Save money inputs
          </Button>
        </div>
      }
    >
      <div className="flex flex-col gap-7 px-6 py-5 text-[13px]">
        <section>
          <h3 className="eyebrow mb-2.5">Imports a year</h3>
          <label className="flex items-center justify-between gap-4">
            <span>Value imported (USD)</span>
            <input
              className={field}
              inputMode="numeric"
              placeholder="$2,000,000"
              value={usd(e.annual_import_value_usd)}
              onChange={(ev) => setE({ ...e, annual_import_value_usd: num(ev.target.value) })}
            />
          </label>
        </section>

        <section>
          <h3 className="eyebrow mb-2.5">Duty rate by origin</h3>
          {countries.length === 0 && <p className="duo text-dim">Add plants with countries first; each country gets a rate.</p>}
          <div className="flex flex-col gap-2">
            {countries.map((c) => (
              <label key={c} className="flex items-center justify-between gap-4">
                <Country name={c} />
                <span className="relative">
                  <input
                    className={cx(field, "pr-7")}
                    inputMode="decimal"
                    aria-label={`Duty rate for ${c}, percent`}
                    placeholder="0"
                    value={rates[c] === undefined ? "" : String(Math.round(rates[c] * 1000) / 10)}
                    onChange={(ev) => {
                      const n = num(ev.target.value);
                      const next = { ...rates };
                      if (n === undefined) delete next[c];
                      else next[c] = n / 100;
                      setE({ ...e, duty_rates: next });
                    }}
                  />
                  <span className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-dim">%</span>
                </span>
              </label>
            ))}
          </div>
          {e.duty_rates_note && <p className="duo mt-2 text-xs text-dim">{e.duty_rates_note}</p>}
          <p className="duo mt-1.5 text-xs text-dim">Rates are typed in %; saved as fractions (41.7% → 0.417).</p>
        </section>

        <section>
          <h3 className="eyebrow mb-2.5">Cost per group a year</h3>
          {groups.length === 0 || !base ? (
            <p className="duo text-dim">Add plants, and parts with a group, to cost each move.</p>
          ) : (
            <>
              <div role="tablist" aria-label="How to enter costs" className="mb-4 inline-flex border border-edge">
                {(["quick", "each"] as const).map((m) => (
                  <button
                    key={m}
                    role="tab"
                    type="button"
                    aria-selected={mode === m}
                    onClick={() => {
                      if (m === "each" && mode === "quick") setE({ ...e, group_costs: quick(total, more) });
                      setMode(m);
                    }}
                    className={cx("px-3 py-1.5 text-[12.5px]", mode === m ? "bg-ink text-paper" : "bg-sheet text-body hover:bg-wash")}
                  >
                    {m === "quick" ? "Quick estimate" : "Each group"}
                  </button>
                ))}
              </div>
              {mode === "quick" ? (
                <div className="flex flex-col gap-3">
                  <label className="flex items-center justify-between gap-4">
                    <span>
                      {base.name || "Base plant"} costs, all groups <span className="text-dim">(USD)</span>
                    </span>
                    <input className={field} inputMode="numeric" placeholder="$670,000" value={usd(total)} onChange={(ev) => setTotal(num(ev.target.value))} />
                  </label>
                  {others.length > 0 && (
                    <>
                      <label htmlFor={`${id}-more`} className="flex items-center justify-between gap-4">
                        <span>{others.map((p) => p.name || p.country).join(", ")} costs more by</span>
                        <span className="relative">
                          <input
                            id={`${id}-more`}
                            className={cx(field, "pr-7")}
                            inputMode="numeric"
                            value={more}
                            onChange={(ev) => setMore(Math.max(0, Math.min(100, num(ev.target.value) ?? 0)))}
                          />
                          <span className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-dim">%</span>
                        </span>
                      </label>
                      <input
                        type="range"
                        min={0}
                        max={100}
                        step={5}
                        value={more}
                        aria-label="How much more the other plants cost, percent"
                        onChange={(ev) => setMore(Number(ev.target.value))}
                        className="w-full accent-[var(--ok)]"
                      />
                      <div className="-mt-1 flex justify-between text-[10px] text-dimension">
                        {["0%", "+25%", "+50%", "+75%", "+100%"].map((t) => (
                          <span key={t}>{t}</span>
                        ))}
                      </div>
                    </>
                  )}
                  {total ? (
                    <p className="duo text-xs text-dim">
                      Fills every group in proportion:{" "}
                      {groups
                        .map((g) => {
                          const row = costs?.[g] ?? {};
                          const o = others[0];
                          return `${groupName(g).toLowerCase()} ${money(row[base.id])}${o ? ` → ${money(row[o.id])}` : ""}`;
                        })
                        .join(", ")}
                      . Rough; switch to &ldquo;Each group&rdquo; to set them one by one.
                    </p>
                  ) : null}
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full min-w-[420px] border-collapse">
                    <thead>
                      <tr>
                        <th scope="col" className="pb-2 text-left text-[10.5px] font-medium uppercase tracking-[0.08em] text-dim">
                          Group
                        </th>
                        {plants.map((p) => (
                          <th key={p.id} scope="col" className="pb-2 text-right text-[10.5px] font-medium uppercase tracking-[0.08em] text-dim">
                            {p.name || p.country}
                          </th>
                        ))}
                        {others.length > 0 && (
                          <th scope="col" className="pb-2 pl-3 text-right text-[10.5px] font-medium uppercase tracking-[0.08em] text-dim">
                            Moving adds
                          </th>
                        )}
                      </tr>
                    </thead>
                    <tbody>
                      {groups.map((g) => {
                        const row = costs?.[g] ?? {};
                        const extra = others[0] && row[others[0].id] !== undefined && row[base.id] !== undefined ? row[others[0].id] - row[base.id] : undefined;
                        return (
                          <tr key={g} className="border-t border-line">
                            <th scope="row" className="py-1.5 pr-2 text-left font-normal">
                              {groupName(g)}
                            </th>
                            {plants.map((p) => (
                              <td key={p.id} className="py-1.5 pl-2 text-right">
                                <input
                                  aria-label={`${groupName(g)} at ${p.name || p.id}, USD a year`}
                                  inputMode="numeric"
                                  className={cx(field, "h-9 w-28")}
                                  value={usd(row[p.id])}
                                  onChange={(ev) => {
                                    const n = num(ev.target.value);
                                    const next = { ...row };
                                    if (n === undefined) delete next[p.id];
                                    else next[p.id] = n;
                                    setE({ ...e, group_costs: { ...(e.group_costs ?? {}), [g]: next } });
                                  }}
                                />
                              </td>
                            ))}
                            {others.length > 0 && <td className="py-1.5 pl-3 text-right text-dim">{extra === undefined ? "—" : `${extra >= 0 ? "+" : ""}${money(extra)}`}</td>}
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              )}
            </>
          )}
        </section>
      </div>
    </Drawer>
  );
}
