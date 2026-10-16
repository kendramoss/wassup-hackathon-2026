"use client";

import { useId } from "react";
import type { Economics, Plan, Product } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { money, pct } from "@/lib/format";
import { Country, Panel, PanelHead, cx } from "./ui";

export const moveKey = (p: Plan) => (p.move ?? []).map((m) => `${m.group}@${m.to_plant}`).join("+");

const groupNames: Record<string, string> = {
  motor_fan_assembly: "Motor-fan assembly",
  pcba: "PCBA (SMT)",
  heating_element: "Heating element",
  housing_moulding: "Housing moulding",
  switch_cord_minor_parts: "Switch, cord and minor parts",
  final_assembly: "Final assembly",
};

export function groupName(id: string | undefined): string {
  if (!id) return "Other";
  return groupNames[id] ?? id.replace(/_/g, " ").replace(/^\w/, (c) => c.toUpperCase());
}

export const rejectionLabels: Record<string, string> = {
  current_setup: "Current setup",
  below_confidence_bar: "Below the bar",
  no_supporting_precedent: "No precedent",
  engine_other_origin: "Other origin",
  check_failed: "Check failed",
};

// fig.2: one column per plant, operation groups from the product, moved
// groups in blue (or amber for a below-bar move) with "moved".
export function PlantDiagram({ product, plan, below }: { product: Product; plan: Plan; below?: boolean }) {
  const plants = product.plants ?? [];
  const moved = new Map((plan.move ?? []).map((m) => [m.group, m.to_plant]));
  const groupsAt = new Map<string, Set<string>>();
  for (const x of [...product.components, ...(product.operations ?? [])]) {
    const plant = "plant" in x && x.plant ? x.plant : undefined;
    if (!plant || !x.group) continue;
    if (!groupsAt.has(plant)) groupsAt.set(plant, new Set());
    groupsAt.get(plant)!.add(x.group);
  }
  return (
    <div className="flex flex-wrap items-start gap-4">
      {plants.map((pl) => {
        const own = [...(groupsAt.get(pl.id) ?? [])];
        const incoming = [...moved].filter(([, to]) => to === pl.id).map(([g]) => g);
        return (
          <div key={pl.id} className="min-w-[180px] flex-1">
            <h3 className="mb-2 flex items-center gap-1.5 text-[13px] font-medium">
              <Country name={pl.country} />
              <span className="font-normal text-dim">· {pl.name || pl.id}</span>
            </h3>
            <ul className="flex flex-col gap-1.5">
              {incoming.map((g) => (
                <li key={`in-${g}`} className={cx("border px-2.5 py-1.5 text-[12.5px]", below ? "border-ask-line bg-ask-wash" : "border-ok bg-ok-wash")}>
                  <b className={cx("mr-1.5 text-[10px] font-medium uppercase tracking-[0.06em]", below ? "text-ask" : "text-ok")}>moved</b>
                  {groupName(g)}
                </li>
              ))}
              {own.map((g) => {
                const leaves = moved.has(g) && moved.get(g) !== pl.id;
                return (
                  <li key={g} className={cx("border px-2.5 py-1.5 text-[12.5px]", leaves ? "border-dashed border-line text-dim line-through" : "border-line bg-white")}>
                    {groupName(g)}
                  </li>
                );
              })}
            </ul>
          </div>
        );
      })}
    </div>
  );
}

// Money inputs: illustrative — editable. Group costs are yearly USD per
// group and plant.
export function EconomicsEditor({ value, onChange, product, countries }: { value: Economics; onChange: (e: Economics) => void; product?: Product; countries: string[] }) {
  const id = useId();
  const rates = value.duty_rates ?? {};
  const costs = value.group_costs ?? {};
  const plants = product?.plants ?? [];
  const groups = [...new Set([...(product?.components ?? []), ...(product?.operations ?? [])].map((x) => x.group).filter((g): g is string => Boolean(g)))];
  const num = (s: string) => (s.trim() === "" ? undefined : Number(s));
  return (
    <Panel>
      <PanelHead right={value.illustrative === false ? "sourced" : "illustrative"}>{copy.money}</PanelHead>
      <div className="grid gap-4 sm:grid-cols-[220px_minmax(0,1fr)]">
        <div>
          <label htmlFor={`${id}-v`} className="eyebrow block">
            Imports a year (USD)
          </label>
          <input
            id={`${id}-v`}
            inputMode="numeric"
            className="mt-1.5 w-full rounded-[2px] border border-line bg-white px-3 py-2 text-sm"
            value={value.annual_import_value_usd ?? ""}
            onChange={(e) => onChange({ ...value, annual_import_value_usd: num(e.target.value) })}
          />
        </div>
        <fieldset>
          <legend className="eyebrow">Duty rate by origin (%)</legend>
          <div className="mt-1.5 flex flex-wrap gap-2">
            {countries.map((c) => (
              <label key={c} className="flex items-center gap-1.5 border border-line bg-white px-2 py-1 text-[13px]">
                <Country name={c} />
                <input
                  aria-label={`Duty rate for ${c}, percent`}
                  inputMode="decimal"
                  className="w-14 bg-transparent text-right"
                  value={rates[c] === undefined ? "" : String(Math.round(rates[c] * 1000) / 10)}
                  onChange={(e) => {
                    const n = num(e.target.value);
                    const next = { ...rates };
                    if (n === undefined || Number.isNaN(n)) delete next[c];
                    else next[c] = n / 100;
                    onChange({ ...value, duty_rates: next });
                  }}
                />
                %
              </label>
            ))}
          </div>
        </fieldset>
      </div>
      {groups.length > 0 && plants.length > 0 && (
        <div className="mt-4 overflow-x-auto">
          <table className="w-full min-w-[480px] text-[13px]">
            <caption className="eyebrow mb-1.5 text-left">Yearly cost of each group, by plant (USD)</caption>
            <thead>
              <tr>
                <th scope="col" className="py-1 text-left font-normal text-dim">
                  Group
                </th>
                {plants.map((p) => (
                  <th key={p.id} scope="col" className="py-1 text-left font-normal text-dim">
                    {p.name || p.id}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {groups.map((g) => (
                <tr key={g} className="border-t border-line">
                  <th scope="row" className="py-1 pr-2 text-left font-normal">
                    {groupName(g)}
                  </th>
                  {plants.map((p) => (
                    <td key={p.id} className="py-1 pr-2">
                      <input
                        aria-label={`${groupName(g)} at ${p.name || p.id}, USD a year`}
                        inputMode="numeric"
                        className="w-28 rounded-[2px] border border-line bg-white px-2 py-1"
                        value={costs[g]?.[p.id] ?? ""}
                        onChange={(e) => {
                          const n = num(e.target.value);
                          const row = { ...(costs[g] ?? {}) };
                          if (n === undefined || Number.isNaN(n)) delete row[p.id];
                          else row[p.id] = n;
                          onChange({ ...value, group_costs: { ...costs, [g]: row } });
                        }}
                      />
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {value.duty_rates_note && <p className="duo mt-3 text-xs text-dim">{value.duty_rates_note}</p>}
      <p className="mt-2 text-xs text-dim">
        {Object.entries(rates)
          .map(([c, r]) => `${c} ${pct(r)}`)
          .join(" · ")}
        {value.annual_import_value_usd ? ` · ${money(value.annual_import_value_usd)} imports a year` : ""}
      </p>
    </Panel>
  );
}
