"use client";

import { useId } from "react";
import { DutyBreakdown, DutySources } from "./duty-breakdown";
import { EstimateCosts } from "./money-prefill";
import type { Economics, Plan, Product } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { money, pct } from "@/lib/format";
import { groupName } from "@/lib/families";
import { outcomeLabels } from "@/lib/plan-check";
import { Chip, Country, Flag, Panel, PanelHead, cx } from "./ui";
import { useGroupName } from "./grouping";

export const moveKey = (p: Plan) => (p.move ?? []).map((m) => `${m.group}@${m.to_plant}`).join("+");


// The one label helper lives with the template mirror in families.ts (#391);
// screens import it from here so every group name has a single source.
export { groupName };

// Lower-cases a leading capital only when it starts an ordinary word, so
// acronyms ("LED PCBA (SMT)", "PCBA (SMT)") keep their case, as they do in
// planner.Describe (D220).
const lowerFirst = (s: string) => (/^[A-Z][a-z]/.test(s) ? s[0].toLowerCase() + s.slice(1) : s);

export function movePhrase(p: Plan, family?: string): string {
  // The backend's description, "Move the X and the Y to Thailand" (D220),
  // carries each family's own group names; a one-country move gives its
  // middle. Anything else builds the same phrase from the template names,
  // with no hard-coded group list to drift (#391).
  const d = /^Move (.+) to ([A-Z][^,;]*)$/.exec(p.description ?? "");
  if (d && !/ to [A-Z]/.test(d[1])) return d[1];
  const parts = (p.move ?? []).map((m) => `the ${lowerFirst(groupName(m.group, family))}`);
  if (parts.length === 0) return (p.description ?? "").replace(/^(Make|Move) /, "");
  return parts.length === 1 ? parts[0] : `${parts.slice(0, -1).join(", ")} and ${parts[parts.length - 1]}`;
}

// What a move takes with it (D220): for each moved group, the parts of this
// product in that group that are not already at the plant it moves to, by
// the names the parts list gives them. The group's steps are listed only when
// none of its parts move, so the line names parts, not process sentences
// (judge panel round 3, D270; backend planner.MovedItems does the same).
export function movedParts(product: Product | null | undefined, plan: Plan): { group: string; items: string[] }[] {
  if (!product) return [];
  const names = (xs: { name?: string; group?: string; plant?: string }[], m: { group?: string; to_plant?: string }) =>
    xs
      .filter((x) => x.group === m.group && x.plant !== m.to_plant)
      .map((x) => x.name)
      .filter((n): n is string => Boolean(n));
  return (plan.move ?? []).map((m) => {
    const parts = names(product.components ?? [], m);
    const items = parts.length > 0 ? parts : names(product.operations ?? [], m);
    return { group: m.group ?? "", items: [...new Set(items)] };
  });
}

export function MoveIncludes({ product, plan, className }: { product: Product | null | undefined; plan: Plan; className?: string }) {
  const groupLabel = useGroupName();
  const groups = movedParts(product, plan).filter((g) => g.items.length > 0);
  if (groups.length === 0) return null;
  return (
    <p className={cx("duo text-xs text-dim", className)}>
      <span className="text-body">What moves:</span>{" "}
      {groups.map((g, i) => (
        <span key={g.group}>
          {i > 0 ? " · " : ""}
          <span className="text-body">{groupLabel(g.group, product?.family, product)}</span> ({g.items.join("; ")})
        </span>
      ))}
    </p>
  );
}

// A backtest read for people (judge panel round 5, D310). The comparison is
// between the parts whose origin CBP's real change switched (made in the old
// country before, in the target after) and the parts the plan moves:
// - changed: parts the real change moved, less any the later ruling lists in
//   the target that were already made there (the heat gun's outer shells);
// - moved / missed: which of those the plan moves, and which it doesn't;
// - alsoKept: parts the real change kept in the old country that the plan
//   moves anyway (a smaller move may have been enough).
// Older stored backtests lack already_in_target and plan_also_moves_kept, so
// both are also worked out from the demo's actual_change and the product.
export type ActualChangeExtras = {
  already_in_target?: string[];
  still_from_china?: string[];
  still_from_china_as?: Record<string, string>;
};

// The product component that holds a part the real change kept in the old
// country: the still_from_china_as entry, else a component of the same name,
// ignoring case (#519). undefined when neither.
export function keptPart(change: ActualChangeExtras | null | undefined, product: Product | null | undefined, word: string): string | undefined {
  const mapped = change?.still_from_china_as?.[word];
  if (mapped) return mapped;
  const w = word.trim().toLowerCase();
  return (product?.components ?? []).find((c) => (c.name ?? "").trim().toLowerCase() === w)?.name;
}
export type BacktestView = { changed: string[]; moved: string[]; missed: string[]; already: string[]; alsoKept: string[]; keptGroups: string[]; ruling?: string; matches: boolean };

export function backtestView(
  bt: unknown,
  opts: { change?: ActualChangeExtras | null; product?: Product | null; move?: Plan["move"]; recount?: boolean; madeWith?: string } = {},
): BacktestView | undefined {
  const b = (bt ?? {}) as Record<string, unknown>;
  const list = (k: string) => (Array.isArray(b[k]) ? (b[k] as string[]) : []);
  const already = Array.isArray(b.already_in_target) ? list("already_in_target") : (opts.change?.already_in_target ?? []);
  const changed = list("actual_moved").filter((a) => !already.includes(a));
  if (changed.length === 0) return undefined;
  const groups = new Set((opts.move ?? []).map((m) => m.group));
  const comps = opts.product?.components ?? [];
  const inMove = (name: string) => comps.some((c) => c.name === name && c.group !== undefined && groups.has(c.group));
  // With no plan over the bar the backend compares CBP's change with nothing,
  // so the closest move is compared from its groups instead.
  const moved = opts.recount ? changed.filter(inMove) : list("moved_by_plan").filter((m) => changed.includes(m));
  const missed = changed.filter((a) => !moved.includes(a));
  // A kept part is found through the demo's still_from_china_as map, else by
  // a component of the same name (#519), as planner.ActualChange.KeptPart does.
  const kept = (word: string) => keptPart(opts.change, opts.product, word);
  // A stored backtest that lists none could predate the generic match, so a
  // non-empty list is trusted and an empty one is worked out again.
  const stored = !opts.recount && Array.isArray(b.plan_also_moves_kept) ? list("plan_also_moves_kept") : [];
  const alsoKept = stored.length
    ? stored
    : (opts.change?.still_from_china ?? []).filter((w) => {
        const part = kept(w);
        return part !== undefined && inMove(part);
      });
  // The template groups that hold those parts, for "they sit in one group".
  const keptGroups = [
    ...new Set(
      alsoKept
        .map((w) => comps.find((c) => c.name === kept(w))?.group)
        .filter((g): g is string => Boolean(g))
        .map((g) => groupName(g, opts.product?.family, opts.product, opts.madeWith)),
    ),
  ];
  const ruling = typeof b.source === "string" ? b.source.match(/\b[NH]\d{6}\b/)?.[0] : undefined;
  return { changed, moved, missed, already, alsoKept, keptGroups, ruling, matches: missed.length === 0 && alsoKept.length === 0 };
}

// Whether the backend's backtest says the top plan moves exactly the parts
// the manufacturer really moved, and nothing CBP's change kept where it was.
// The cases list passes the demo's change, product and top move when it has
// them, so a stored backtest that predates plan_also_moves_kept gets the same
// answer as the plan step (#519).
export function backtestMatch(
  bt: unknown,
  opts: { change?: ActualChangeExtras | null; product?: Product | null; move?: Plan["move"] } = {},
): { matches: boolean; ruling?: string } {
  const v = backtestView(bt, opts);
  const ruling = typeof (bt as Record<string, unknown> | undefined)?.source === "string" ? String((bt as Record<string, unknown>).source).match(/\b[NH]\d{6}\b/)?.[0] : undefined;
  return { matches: Boolean(v?.matches), ruling };
}

// Ranking tiers (D340, D341). A shown plan is "clear_margin" when its check
// is at least the bar plus the clear margin and names no counter-ruling, and
// "at_bar" otherwise. New results carry the tier and the threshold; a result
// stored before D341 has neither, so the tier is worked out with the same
// rule and its order is left as stored (net alone).
export const CLEAR_FROM = 0.93;
export type Tier = "clear_margin" | "at_bar";
type Ranked = { ranking?: { clear_from?: number } | null };

export function clearFrom(result?: Ranked | null): number {
  return result?.ranking?.clear_from ?? CLEAR_FROM;
}

export function planTier(p: Plan, result?: Ranked | null): Tier | undefined {
  if (!p.shown) return undefined;
  if (p.tier === "clear_margin" || p.tier === "at_bar") return p.tier;
  const v = p.verdict;
  return (v?.confidence ?? 0) >= clearFrom(result) - 1e-9 && !v?.counter_ruling ? "clear_margin" : "at_bar";
}

// How many of the product's components a move changes the plant of: the
// stored parts_moved, else counted from the product (planner.PartsMoved).
export function partsMoved(p: Plan, product?: Product | null): number {
  if (typeof p.parts_moved === "number") return p.parts_moved;
  const to = new Map((p.move ?? []).map((m) => [m.group, m.to_plant]));
  return (product?.components ?? []).filter((c) => c.group !== undefined && to.has(c.group) && c.plant !== to.get(c.group)).length;
}

// The shown plan that moves the fewest parts, when two or more are shown
// (D340): the stored mark, else the same rule (ties: higher check, higher
// net, earlier rank). A label, never the sort key.
export function smallestSupported(plans: Plan[], product?: Product | null): Plan | undefined {
  if (plans.length < 2) return undefined;
  const marked = plans.find((p) => p.smallest_supported);
  if (marked || plans.some((p) => p.tier)) return marked;
  let best = plans[0];
  for (const p of plans.slice(1)) {
    const [a, b] = [partsMoved(p, product), partsMoved(best, product)];
    const [ca, cb] = [p.verdict?.confidence ?? 0, best.verdict?.confidence ?? 0];
    if (a < b || (a === b && (ca > cb || (ca === cb && (p.net_per_year ?? 0) > (best.net_per_year ?? 0))))) best = p;
  }
  return best;
}

export function TierChip({ plan, result }: { plan: Plan; result?: Ranked | null }) {
  const t = planTier(plan, result);
  if (!t) return null;
  const T = copy.plan.tier;
  return t === "clear_margin" ? (
    <Chip tone="ok" className="whitespace-normal">
      {T.clear} · {T.clearWhy(clearFrom(result).toFixed(2))}
    </Chip>
  ) : (
    <Chip tone="ask" className="whitespace-normal">
      {T.atBar} · {T.atBarWhy}
    </Chip>
  );
}

// A list said in words: "a, b and c".
export function andList(xs: string[]): string {
  return xs.length <= 1 ? (xs[0] ?? "") : `${xs.slice(0, -1).join(", ")} and ${xs[xs.length - 1]}`;
}

// The precedent check said as a judgement: exactly at the bar is never a
// confident pass (DESIGN.md, demo seed run 2).
export function checkWords(v: number | null | undefined): string {
  if (v === null || v === undefined) return "not checked";
  if (Math.abs(v - 0.92) < 0.0005) return "exactly at the 0.92 precedent-check bar";
  return v > 0.92 ? "above the 0.92 precedent-check bar" : "below the 0.92 precedent-check bar";
}

// verdict.agreement ("2/3") is present only when the three-view panel ran,
// near the bar (D83): "3 views, 2 agree". Anything else shows nothing.
export function agreementWords(a: string | null | undefined): string | null {
  const m = /^(\d+)\s*\/\s*(\d+)$/.exec(a ?? "");
  return m ? copy.plan.agreement(m[2], m[1]) : null;
}

// One table of outcome labels for Plan and Scenarios (lib/plan-check.ts, D220).
export const rejectionLabels = outcomeLabels;

// fig.2: one column per plant, operation groups from the product, moved
// groups in blue (or amber for a below-bar move) with "moved".
export function PlantDiagram({ product, plan, below }: { product: Product; plan: Plan; below?: boolean }) {
  const groupLabel = useGroupName();
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
                  {groupLabel(g, product.family, product)}
                </li>
              ))}
              {own.map((g) => {
                const leaves = moved.has(g) && moved.get(g) !== pl.id;
                return (
                  <li key={g} className={cx("border px-2.5 py-1.5 text-[12.5px]", leaves ? "border-dashed border-line text-dim line-through" : "border-line bg-white")}>
                    {groupLabel(g, product.family, product)}
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
  const groupLabel = useGroupName();
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
      {groups.length > 0 && plants.length > 0 && <EstimateCosts product={product} economics={value} onFill={(next) => onChange(next)} className="mt-4" />}
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
                    {groupLabel(g, product?.family, product)}
                  </th>
                  {plants.map((p) => (
                    <td key={p.id} className="py-1 pr-2">
                      <input
                        aria-label={`${groupLabel(g, product?.family, product)} at ${p.name || p.id}, USD a year`}
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
              <legend className="text-[13px] font-medium">{groupLabel(g, product?.family, product)}</legend>
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
