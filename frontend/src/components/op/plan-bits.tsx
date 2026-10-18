"use client";

import { useId } from "react";
import { DutyBreakdown, DutySources } from "./duty-breakdown";
import type { Economics, Plan, Product } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { money, pct } from "@/lib/format";
import { Country, Flag, Panel, PanelHead, cx } from "./ui";

export const moveKey = (p: Plan) => (p.move ?? []).map((m) => `${m.group}@${m.to_plant}`).join("+");

const groupNames: Record<string, string> = {
  motor_fan_assembly: "Motor-fan assembly",
  pcba: "PCBA (SMT)",
  heating_element: "Heating element",
  housing_moulding: "Housing moulding",
  switch_cord_minor_parts: "Switch, cord and minor parts",
  final_assembly: "Final assembly",
  // LED tub light demo
  led_pcba: "LED PCBA (SMT)",
  lens: "Lens moulding",
  base_harness_seals: "Base, harness and seals",
};

export function groupName(id: string | undefined): string {
  if (!id) return "Other";
  return groupNames[id] ?? id.replace(/_/g, " ").replace(/^\w/, (c) => c.toUpperCase());
}

// "motor_fan_assembly" → "the motor-fan assembly"; a move's groups joined
// as a sentence: "the motor-fan assembly and the PCBA".
const phrases: Record<string, string> = {
  motor_fan_assembly: "the motor-fan assembly",
  pcba: "the PCBA",
  heating_element: "the heating element",
  housing_moulding: "the housing moulding",
  switch_cord_minor_parts: "the switch, cord and minor parts",
  final_assembly: "final assembly",
  led_pcba: "the LED PCBA",
  lens: "the lens moulding",
  base_harness_seals: "the base, harness and seals",
};

// Lower-cases a leading capital only when it starts an ordinary word, so
// acronyms ("LED PCBA", "PCBA") keep their case.
const lowerFirst = (s: string) => (/^[A-Z][a-z]/.test(s) ? s[0].toLowerCase() + s.slice(1) : s);

export function movePhrase(p: Plan): string {
  const parts = (p.move ?? []).map((m) => phrases[m.group ?? ""] ?? `the ${lowerFirst(groupName(m.group))}`);
  if (parts.length === 0) return (p.description ?? "").replace(/^Make /, "");
  return parts.length === 1 ? parts[0] : `${parts.slice(0, -1).join(", ")} and ${parts[parts.length - 1]}`;
}

// Whether the backend's backtest says the top plan moves exactly the parts
// the manufacturer really moved, and the ruling that recorded the change.
export function backtestMatch(bt: unknown): { matches: boolean; ruling?: string } {
  const b = (bt ?? {}) as Record<string, unknown>;
  const list = (k: string) => (Array.isArray(b[k]) ? (b[k] as string[]) : []);
  const actual = list("actual_moved"), moved = list("moved_by_plan");
  const ruling = typeof b.source === "string" ? b.source.match(/\b[NH]\d{6}\b/)?.[0] : undefined;
  const matches = actual.length > 0 && list("in_actual_change_not_in_plan").length === 0 && actual.every((a) => moved.includes(a));
  return { matches, ruling };
}

// The precedent check said as a judgement: exactly at the bar is never a
// confident pass (DESIGN.md, demo seed run 2).
export function checkWords(v: number | null | undefined): string {
  if (v === null || v === undefined) return "not checked";
  if (Math.abs(v - 0.92) < 0.0005) return "exactly at the 0.92 bar";
  return v > 0.92 ? "above the 0.92 bar" : "below the 0.92 bar";
}

// verdict.agreement ("2/3") is present only when the three-view panel ran,
// near the bar (D83): "3 views, 2 agree". Anything else shows nothing.
export function agreementWords(a: string | null | undefined): string | null {
  const m = /^(\d+)\s*\/\s*(\d+)$/.exec(a ?? "");
  return m ? copy.plan.agreement(m[2], m[1]) : null;
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
                  <b className={cx("mr-1.5 text-[11px] font-medium uppercase tracking-[0.06em]", below ? "text-ask" : "text-ok")}>moved</b>
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

// priced: the case's economics_priced, for the Section 301 breakdown of
// live rates (D90); the inputs stay the user's own.
export function EconomicsEditor({ value, onChange, product, countries, priced }: { value: Economics; onChange: (e: Economics) => void; product?: Product; countries: string[]; priced?: Economics | null }) {
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
        // Columns share the width: no sideways scroll (18 Oct).
        <div className="mt-4 max-sm:hidden">
          <table className="w-full table-fixed text-[13px]">
            <caption className="eyebrow mb-1.5 text-left">Yearly cost of each group, by plant (USD)</caption>
            <thead>
              <tr>
                <th scope="col" className="py-1 text-left font-normal text-dim">
                  Group
                </th>
                {plants.map((p) => (
                  <th key={p.id} scope="col" className="py-1 text-left font-normal text-dim">
                    <Flag country={p.country} className="mr-1" />
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
                        className="w-full min-w-0 rounded-[2px] border border-line bg-white px-2 py-1"
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
      {/* Phones: one block per group with a labelled field per plant, so no
          plant column hides off the right edge (#154). */}
      {groups.length > 0 && plants.length > 0 && (
        <div className="mt-4 flex flex-col gap-3 sm:hidden">
          <p className="eyebrow">Yearly cost of each group, by plant (USD)</p>
          {groups.map((g) => (
            <fieldset key={g} className="border-t border-line pt-2">
              <legend className="text-[13px] font-medium">{groupName(g)}</legend>
              <div className="mt-1 grid grid-cols-2 gap-2">
                {plants.map((p) => (
                  <label key={p.id} className="flex flex-col gap-0.5 text-[11.5px] text-dim">
                    <span>
                      <Flag country={p.country} className="mr-1" />
                      {p.name || p.id}
                    </span>
                    <input
                      inputMode="numeric"
                      className="w-full rounded-[2px] border border-line bg-white px-2 py-1.5 text-[13px] text-ink"
                      value={costs[g]?.[p.id] ?? ""}
                      onChange={(e) => {
                        const n = num(e.target.value);
                        const row = { ...(costs[g] ?? {}) };
                        if (n === undefined || Number.isNaN(n)) delete row[p.id];
                        else row[p.id] = n;
                        onChange({ ...value, group_costs: { ...costs, [g]: row } });
                      }}
                    />
                  </label>
                ))}
              </div>
            </fieldset>
          ))}
        </div>
      )}
      <DutyBreakdown e={priced ?? value} className="mt-3" />
      <DutySources note={value.duty_rates_note} className="mt-2" />
      <p className="mt-2 text-xs text-dim">
        {Object.entries(rates)
          .map(([c, r]) => `${c} ${pct(r)}`)
          .join(" · ")}
        {value.annual_import_value_usd ? ` · ${money(value.annual_import_value_usd)} imports a year` : ""}
      </p>
    </Panel>
  );
}
